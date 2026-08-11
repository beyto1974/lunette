// Package marcio loads MARC21 records from binary transmission files or
// MARCXML, collecting per-record failures instead of aborting on them.
package marcio

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	marc "github.com/beyto1974/gomarc"
)

// Format identifies how records are encoded in a file.
type Format int

const (
	FormatUnknown Format = iota
	FormatBinary
	FormatXML
)

func (f Format) String() string {
	switch f {
	case FormatBinary:
		return "MARC21"
	case FormatXML:
		return "MARCXML"
	default:
		return "unknown"
	}
}

// declaresMARC8 reports whether the first record's leader/09 is anything other
// than 'a', which is MARC21's way of saying "MARC-8 encoded".
func declaresMARC8(peek []byte) bool {
	const codingScheme = 9
	if len(peek) <= codingScheme {
		return false
	}
	return peek[codingScheme] != 'a'
}

// offsetUnknown marks records whose byte offset cannot be determined, which is
// the case for every MARCXML record.
const offsetUnknown int64 = -1

// Issue is a record that failed to decode. Ordinal counts every record
// attempted, so it stays aligned with the record numbering a user sees in a
// hex editor or in yaz-marcdump output.
type Issue struct {
	Ordinal int
	Offset  int64
	Err     error
}

func (i Issue) String() string {
	if i.Offset >= 0 {
		return fmt.Sprintf("record %d (offset %d): %v", i.Ordinal, i.Offset, i.Err)
	}
	return fmt.Sprintf("record %d: %v", i.Ordinal, i.Err)
}

// Batch is one chunk of a streaming load.
type Batch struct {
	Format Format
	// ForcedUTF8 reports that the records carry UTF-8 bytes despite a leader
	// claiming MARC-8, and were decoded as UTF-8 anyway.
	ForcedUTF8 bool
	Records    []*marc.Record
	Offsets    []int64
	Issues     []Issue
}

// Result is a complete load of a file.
type Result struct {
	Format     Format
	ForcedUTF8 bool
	Records    []*marc.Record
	Offsets    []int64
	Issues     []Issue
}

// DetectFormat inspects the leading bytes of a file. MARCXML starts with '<'
// once leading whitespace is skipped; a binary MARC21 record starts with the
// five ASCII digits of its record length.
func DetectFormat(peek []byte) Format {
	for _, b := range peek {
		switch {
		case b == ' ' || b == '\t' || b == '\n' || b == '\r':
			continue
		case b == '<':
			return FormatXML
		case b >= '0' && b <= '9':
			return FormatBinary
		default:
			return FormatUnknown
		}
	}
	return FormatUnknown
}

// source is the common shape of gomarc's binary and XML readers.
type source interface {
	next() (*marc.Record, error)
	// size reports the byte length of the record just attempted, or
	// offsetUnknown when the source cannot tell.
	size() int64
}

type binarySource struct{ rd *marc.Reader }

func (b binarySource) next() (*marc.Record, error) { return b.rd.Next() }
func (b binarySource) size() int64                 { return int64(len(b.rd.CurrentChunk())) }

type xmlSource struct{ rd *marc.XMLReader }

func (x xmlSource) next() (*marc.Record, error) { return x.rd.Next() }
func (x xmlSource) size() int64                 { return offsetUnknown }

// newSource sniffs the format and encoding and returns a reader for them.
// forcedUTF8 reports that the file's bytes were trusted over its leaders.
func newSource(r io.Reader) (src source, format Format, forcedUTF8 bool, err error) {
	br := bufio.NewReaderSize(r, encodingSample+4096)
	peek, err := br.Peek(encodingSample)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, FormatUnknown, false, err
	}

	switch format = DetectFormat(peek); format {
	case FormatBinary:
		// MARCXML is UTF-8 by definition, so the override only applies here,
		// and only when the leader disagrees with the bytes. Overriding a
		// record that already declares UTF-8 changes nothing but would report
		// a conflict that does not exist.
		forcedUTF8 = declaresMARC8(peek) && LooksUTF8(peek)
		opts := []marc.ReaderOption{marc.WithHideUTF8Warnings(true)}
		if forcedUTF8 {
			opts = append(opts, marc.WithForceUTF8(true))
		}
		return binarySource{rd: marc.NewReader(br, opts...)}, format, forcedUTF8, nil

	case FormatXML:
		return xmlSource{rd: marc.NewXMLReader(br)}, format, false, nil

	default:
		return nil, FormatUnknown, false, fmt.Errorf("unrecognised format: not binary MARC21 or MARCXML")
	}
}

// errPanicked wraps a panic recovered from gomarc. It ends the walk: a panic
// leaves the reader mid-record, and one that consumed no input would repeat on
// every subsequent call.
var errPanicked = errors.New("reader panicked")

// safeNext calls the source, converting a panic into an error. gomarc panics on
// some malformed input - a declared record length below 5, for instance, makes
// it allocate a negative-length slice.
func safeNext(s source) (rec *marc.Record, err error) {
	defer func() {
		if r := recover(); r != nil {
			rec, err = nil, fmt.Errorf("%w: malformed record: %v", errPanicked, r)
		}
	}()
	return s.next()
}

// Stream reads r and hands batchSize records at a time to fn. Issues are
// reported in the batch they occurred in, so a caller can surface parse
// failures as they happen. A non-nil error from fn stops the walk.
func Stream(r io.Reader, batchSize int, fn func(Batch) error) error {
	if batchSize < 1 {
		batchSize = 1
	}
	src, format, forcedUTF8, err := newSource(r)
	if err != nil {
		return err
	}

	batch := Batch{Format: format, ForcedUTF8: forcedUTF8}
	var offset int64
	ordinal := 0

	flush := func() error {
		if len(batch.Records) == 0 && len(batch.Issues) == 0 {
			return nil
		}
		if err := fn(batch); err != nil {
			return err
		}
		batch = Batch{Format: format, ForcedUTF8: forcedUTF8}
		return nil
	}

	for {
		ordinal++
		start := offset
		rec, err := safeNext(src)
		if size := src.size(); size > 0 {
			offset += size
		}
		if errors.Is(err, io.EOF) || (err == nil && rec == nil) {
			break
		}
		if err != nil {
			batch.Issues = append(batch.Issues, Issue{Ordinal: ordinal, Offset: start, Err: err})
			if errors.Is(err, errPanicked) {
				break
			}
			continue
		}
		if src.size() == offsetUnknown {
			start = offsetUnknown
		}
		batch.Records = append(batch.Records, rec)
		batch.Offsets = append(batch.Offsets, start)
		if len(batch.Records) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// Load reads every record from r.
func Load(r io.Reader) (*Result, error) {
	res := &Result{}
	err := Stream(r, 512, func(b Batch) error {
		res.Format = b.Format
		res.ForcedUTF8 = b.ForcedUTF8
		res.Records = append(res.Records, b.Records...)
		res.Offsets = append(res.Offsets, b.Offsets...)
		res.Issues = append(res.Issues, b.Issues...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// LoadFile reads every record from the file at path.
func LoadFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Load(f)
}
