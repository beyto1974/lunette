package marcio

import (
	"bytes"
	"fmt"
	"io"

	marc "github.com/beyto1974/gomarc"
)

// RecordReader reads single records back out of a file that has already been
// walked, using the extents that walk reported.
//
// It is what lets the browser hold a million records without holding a million
// decoded records: what a list row shows is a few short strings, and the
// record itself is fetched when the cursor lands on it. A seek and a couple of
// kilobytes are far cheaper than the several kilobytes per record that a
// decoded record costs for as long as the browser is open.
type RecordReader struct {
	r          io.ReaderAt
	format     Format
	forcedUTF8 bool
}

// NewRecordReader reads records of the given format out of r. forcedUTF8 has
// to match what the walk decided, or a re-read record would decode differently
// from the one the walk produced.
func NewRecordReader(r io.ReaderAt, format Format, forcedUTF8 bool) *RecordReader {
	return &RecordReader{r: r, format: format, forcedUTF8: forcedUTF8}
}

// At decodes the record at ext.
func (rr *RecordReader) At(ext Extent) (*marc.Record, error) {
	if !ext.Known() {
		return nil, fmt.Errorf("record has no known place in the file")
	}
	buf := make([]byte, ext.Length)
	if _, err := rr.r.ReadAt(buf, ext.Offset); err != nil && err != io.EOF {
		return nil, err
	}

	switch rr.format {
	case FormatXML:
		return first(xmlSource{rd: marc.NewXMLReader(bytes.NewReader(buf))})
	case FormatBinary:
		opts := []marc.ReaderOption{marc.WithHideUTF8Warnings(true)}
		if rr.forcedUTF8 {
			opts = append(opts, marc.WithForceUTF8(true))
		}
		return first(&binarySource{rd: marc.NewReader(bytes.NewReader(buf), opts...)})
	default:
		return nil, fmt.Errorf("cannot re-read a record of unknown format")
	}
}

// first decodes one record, turning "the bytes held none" into an error rather
// than a nil record the caller has to test for.
func first(src source) (*marc.Record, error) {
	rec, err := safeNext(src)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, fmt.Errorf("no record at that offset")
	}
	return rec, nil
}
