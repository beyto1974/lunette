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

// offsetUnknown marks a record whose place in the file could not be worked
// out, which is what an unrecoverably damaged block leaves behind.
const offsetUnknown int64 = -1

// Extent is where a record sits in the file it came from. It is what lets a
// reader come back for one record - see RecordReader - instead of keeping
// every record it has ever decoded.
type Extent struct {
	Offset int64
	Length int64
}

// Known reports whether the extent can be read back.
func (e Extent) Known() bool { return e.Offset >= 0 && e.Length > 0 }

// Issue is a record that failed to decode. Ordinal counts every record
// attempted, so it stays aligned with the record numbering a user sees in a
// hex editor or in yaz-marcdump output.
type Issue struct {
	Ordinal int
	Offset  int64
	Err     error
	// Source names the file the record came from, set when several files are
	// read as one set. Empty for a single input.
	Source string
}

func (i Issue) String() string {
	where := ""
	if i.Source != "" {
		where = " of " + i.Source
	}
	if i.Offset >= 0 {
		return fmt.Sprintf("record %d%s (offset %d): %v", i.Ordinal, where, i.Offset, i.Err)
	}
	return fmt.Sprintf("record %d%s: %v", i.Ordinal, where, i.Err)
}

// Batch is one chunk of a streaming load.
type Batch struct {
	Format Format
	// ForcedUTF8 reports that the records carry UTF-8 bytes despite a leader
	// claiming MARC-8, and were decoded as UTF-8 anyway.
	ForcedUTF8 bool
	Records    []*marc.Record
	Extents    []Extent
	Issues     []Issue
	// Source names the file the batch came from, when the walk was told one.
	Source string
}

// Result is a complete load of a file, or of several read as one set.
type Result struct {
	Format Format
	// MixedFormats reports that the inputs were not all the same format.
	MixedFormats bool
	ForcedUTF8   bool
	Records      []*marc.Record
	Extents      []Extent
	Issues       []Issue
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
	// extent reports where the record just attempted sits in the file.
	extent() Extent
	// close releases whatever the source is holding. A source that decodes on
	// other goroutines needs telling that a walk has ended early.
	close()
}

// binarySource keeps its own running position, since a binary record's place
// in the file is simply the sum of the lengths before it.
type binarySource struct {
	rd  *marc.Reader
	pos int64
	ext Extent
}

func (b *binarySource) next() (*marc.Record, error) {
	rec, err := b.rd.Next()
	size := int64(len(b.rd.CurrentChunk()))
	b.ext = Extent{Offset: b.pos, Length: size}
	if size > 0 {
		b.pos += size
	}
	return rec, err
}

func (b *binarySource) extent() Extent { return b.ext }
func (b *binarySource) close()         {}

// xmlSource is one encoding/xml decoder over a stretch of MARCXML. It is what
// a block decoder uses; where the records sit is worked out by the splitter
// that cut the block, not by the decoder.
type xmlSource struct{ rd *marc.XMLReader }

func (x xmlSource) next() (*marc.Record, error) { return x.rd.Next() }
func (x xmlSource) extent() Extent              { return Extent{Offset: offsetUnknown} }
func (x xmlSource) close()                      {}

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
		return &binarySource{rd: marc.NewReader(br, opts...)}, format, forcedUTF8, nil

	case FormatXML:
		// MARCXML goes through the block splitter whatever the core count:
		// it is what lets a decoder start again after damage, which a single
		// encoding/xml decoder cannot do.
		return newParallelXML(br, xmlWorkers(), xmlBlockSize), format, false, nil

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
// failures as they happen. A non-nil error from fn stops the walk; ErrStop
// stops it without being reported as a failure.
func Stream(r io.Reader, batchSize int, fn func(Batch) error) error {
	_, err := walk(r, batchSize, walkOpts{}, fn)
	return err
}

// walkOpts positions one walk inside a larger set of them: base is how many
// records were decoded before this input, so ordinals run across the whole
// set, and source names the file for the issues it raises.
type walkOpts struct {
	base   int
	source string
}

// walkResult is what a single walk learned, which is everything a Summary
// needs about one input.
type walkResult struct {
	format     Format
	forcedUTF8 bool
	decoded    int
	stopped    bool
}

// walk is Stream's body, told where in a set of inputs it sits.
func walk(r io.Reader, batchSize int, opts walkOpts, fn func(Batch) error) (walkResult, error) {
	if batchSize < 1 {
		batchSize = 1
	}
	src, format, forcedUTF8, err := newSource(r)
	if err != nil {
		return walkResult{}, err
	}
	defer src.close()
	res := walkResult{format: format, forcedUTF8: forcedUTF8}

	batch := Batch{Format: format, ForcedUTF8: forcedUTF8, Source: opts.source}
	ordinal := opts.base

	flush := func() error {
		if len(batch.Records) == 0 && len(batch.Issues) == 0 {
			return nil
		}
		if err := fn(batch); err != nil {
			return err
		}
		batch = Batch{Format: format, ForcedUTF8: forcedUTF8, Source: opts.source}
		return nil
	}

	for {
		ordinal++
		rec, err := safeNext(src)
		ext := src.extent()
		if errors.Is(err, io.EOF) || (err == nil && rec == nil) {
			break
		}
		if err != nil {
			batch.Issues = append(batch.Issues, Issue{Ordinal: ordinal, Offset: ext.Offset, Err: err, Source: opts.source})
			if errors.Is(err, errPanicked) || errors.Is(err, errFatal) {
				break
			}
			continue
		}
		res.decoded++
		batch.Records = append(batch.Records, rec)
		batch.Extents = append(batch.Extents, ext)
		if len(batch.Records) >= batchSize {
			if err := flush(); err != nil {
				return stopOrFail(res, err)
			}
		}
	}
	if err := flush(); err != nil {
		return stopOrFail(res, err)
	}
	return res, nil
}

// stopOrFail separates a caller that has seen enough from one that broke.
func stopOrFail(res walkResult, err error) (walkResult, error) {
	if errors.Is(err, ErrStop) {
		res.stopped = true
		return res, nil
	}
	return res, err
}

// Load reads every record from r.
func Load(r io.Reader) (*Result, error) {
	res := &Result{}
	err := Stream(r, 512, func(b Batch) error {
		res.Format = b.Format
		res.ForcedUTF8 = b.ForcedUTF8
		res.Records = append(res.Records, b.Records...)
		res.Extents = append(res.Extents, b.Extents...)
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
