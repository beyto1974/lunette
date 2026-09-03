package marcio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
// follower learns where the records it has already read end.
//
// It seeks from one record to the next reading only the five length bytes,
// rather than loading the file: a harvest can be gigabytes, and this runs
// every time the file changes.
func CompletePrefixFile(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()

	var offset int64
	header := make([]byte, 5)
	for offset+LeaderLength <= size {
		if _, err := f.ReadAt(header, offset); err != nil {
			return offset, nil
		}
		length, err := strconv.Atoi(string(header))
		if err != nil || length < LeaderLength || offset+int64(length) > size {
			return offset, nil
		}
		offset += int64(length)
	}
	return offset, nil
}

// maxRead bounds a single incremental read. A followed file can grow faster
// than it is read - a harvest writing flat out, or one left running overnight -
// and without a cap the reader would allocate whatever it found. Anything
// beyond the cap is picked up by the read after it.
const maxRead = 64 << 20 // 64 MB

// LoadFrom reads the records appended to a binary MARC21 file after offset,
// and returns the offset to resume from. A partially written record at the end
// is left for the next call, as is anything past maxRead.
func LoadFrom(path string, offset int64) (*Result, int64, error) {
	return loadFromLimit(path, offset, maxRead)
}

// loadFromLimit is LoadFrom with the cap as an argument, so that tests can
// exercise the boundary without writing 64 MB.
func loadFromLimit(path string, offset, limit int64) (*Result, int64, error) {
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
	available := info.Size() - offset
	if available > limit {
		available = limit
	}
	data := make([]byte, available)
	n, err := io.ReadFull(f, data)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
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
	// The records were read out of a slice starting at offset, so everything
	// they say about where they sit counts from there. Both extents and issue
	// offsets have to name a place in the file: one so a reader can come back
	// for the record, the other so the number in the message is the one a hex
	// editor shows.
	for i := range res.Extents {
		if res.Extents[i].Offset >= 0 {
			res.Extents[i].Offset += offset
		}
	}
	for i := range res.Issues {
		if res.Issues[i].Offset >= 0 {
			res.Issues[i].Offset += offset
		}
	}
	return res, offset + int64(complete), nil
}
