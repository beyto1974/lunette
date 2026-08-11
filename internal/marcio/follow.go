package marcio

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// CompletePrefix returns the length of the leading run of whole records in
// data. A harvest being written ends mid-record, and decoding that tail would
// report damage that is really just a file in progress.
func CompletePrefix(data []byte) int {
	offset := 0
	for offset+LeaderLength <= len(data) {
		length, err := strconv.Atoi(string(data[offset : offset+5]))
		if err != nil || length < LeaderLength {
			return offset
		}
		if offset+length > len(data) {
			return offset
		}
		offset += length
	}
	return offset
}

// CompletePrefixFile is CompletePrefix over a whole file, which is how a
// follower learns where the records it has already read end. It walks record
// lengths without decoding anything, so it costs a few microseconds per
// megabyte.
func CompletePrefixFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return int64(CompletePrefix(data)), nil
}

// LoadFrom reads the records appended to a binary MARC21 file after offset,
// and returns the offset to resume from. A partially written record at the end
// is left for the next call.
func LoadFrom(path string, offset int64) (*Result, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() <= offset {
		// Nothing new. A file that shrank has been replaced rather than
		// appended to, and the caller should start over.
		if info.Size() < offset {
			return nil, 0, fmt.Errorf("%s shrank from %d to %d bytes", path, offset, info.Size())
		}
		return &Result{Format: FormatBinary}, offset, nil
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset, err
	}
	data := make([]byte, info.Size()-offset)
	n, err := f.Read(data)
	if err != nil && n == 0 {
		return nil, offset, err
	}
	data = data[:n]

	if offset == 0 && DetectFormat(data) != FormatBinary {
		return nil, offset, fmt.Errorf("following needs binary MARC21, not %s", DetectFormat(data))
	}

	complete := CompletePrefix(data)
	if complete == 0 {
		return &Result{Format: FormatBinary}, offset, nil
	}

	res, err := Load(bytes.NewReader(data[:complete]))
	if err != nil {
		return nil, offset, err
	}
	return res, offset + int64(complete), nil
}
