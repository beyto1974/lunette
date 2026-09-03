package export

import (
	"fmt"
	"io"
	"strings"

	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/lunette/internal/marcio"
)

// Writer serialises records one at a time. It is what Write does, without the
// records having to exist all at once: a 4 GB harvest converts in constant
// memory rather than being decoded into the heap first.
//
// Close finishes the document - the closing tag or bracket the wrapped formats
// need - and must be called even when nothing was written.
type Writer struct {
	format Format
	mrc    *marc.Writer
	xml    *marc.XMLWriter
	json   *marc.JSONWriter
	n      int
}

// NewWriter starts a document in the given format.
func NewWriter(w io.Writer, format Format) (*Writer, error) {
	out := &Writer{format: format}
	var err error
	switch format {
	case FormatMRC:
		out.mrc = marc.NewWriter(w)
	case FormatXML:
		out.xml, err = marc.NewXMLWriter(w)
	case FormatJSON:
		out.json, err = marc.NewJSONWriter(w)
	default:
		return nil, fmt.Errorf("unknown format %q (want mrc, xml or json)", format)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Write serialises one record, labelling it as UTF-8 first for the reason
// Write gives: everything this package emits is UTF-8, whatever the leader
// arrived saying.
func (w *Writer) Write(rec *marc.Record) error {
	if err := labelAsUTF8(rec); err != nil {
		return err
	}
	w.n++

	var err error
	switch w.format {
	case FormatMRC:
		err = w.mrc.Write(rec)
	case FormatXML:
		err = w.xml.Write(rec)
	case FormatJSON:
		err = w.json.Write(rec)
	}
	if err != nil {
		return fmt.Errorf("record %d: %w", w.n, err)
	}
	return nil
}

// Close ends the document. Binary MARC21 has no wrapper and needs nothing.
func (w *Writer) Close() error {
	switch w.format {
	case FormatXML:
		return w.xml.Close()
	case FormatJSON:
		return w.json.Close()
	}
	return nil
}

// Match reports whether one record passes the criteria. Filter is this applied
// to a slice; streaming needs it a record at a time.
func (c Criteria) Match(rec *marc.Record) bool {
	return c.Matcher().Match(rec)
}

// Matcher is Criteria prepared for a walk: the query is lowercased and the tag
// trimmed once here rather than once per record, which over a harvest of a
// million records is a million allocations saved.
type Matcher struct {
	query, tag string
	scope      marcio.Scope
}

// Matcher prepares the criteria.
func (c Criteria) Matcher() Matcher {
	return Matcher{
		query: strings.ToLower(strings.TrimSpace(c.Query)),
		tag:   strings.TrimSpace(c.Tag),
		scope: c.Scope,
	}
}

// Everything reports criteria that keep every record, which lets a caller skip
// the work entirely.
func (m Matcher) Everything() bool { return m.query == "" && m.tag == "" }

// Match reports whether one record passes.
func (m Matcher) Match(rec *marc.Record) bool {
	if m.tag != "" && !marcio.HasTag(rec, m.tag) {
		return false
	}
	if m.query == "" {
		return true
	}
	var full string
	if m.scope.NeedsFullText() {
		full = marcio.FullTextKey(rec)
	}
	return m.scope.Matches(marcio.SearchKey(rec), full, m.query)
}
