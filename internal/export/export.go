// Package export writes record sets back out as binary MARC21, MARCXML or
// MARC-in-JSON, optionally narrowed by the same criteria the TUI filters with.
package export

import (
	"fmt"
	"io"
	"strings"

	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/lunette/internal/marcio"
)

// Format names an output encoding.
type Format string

const (
	FormatMRC  Format = "mrc"
	FormatXML  Format = "xml"
	FormatJSON Format = "json"
)

// ParseFormat maps a command-line value to a Format.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case FormatMRC, FormatXML, FormatJSON:
		return f, nil
	default:
		return "", fmt.Errorf("unknown format %q (want mrc, xml or json)", s)
	}
}

// Write serialises recs to w in the given format.
//
// Every format this writes is UTF-8: MARCXML and MARC-in-JSON are UTF-8 by
// definition, and gomarc emits binary MARC21 as UTF-8 too. So every record is
// labelled as UTF-8 on the way out. Harvested records routinely arrive with
// leader/09 blank, which claims MARC-8; passing that leader through would
// mislabel the output for whoever reads it next.
//
// This edits the leader of the records it is given rather than copying them,
// which matches what they now hold: by this point they have been decoded to
// Unicode.
func Write(w io.Writer, recs []*marc.Record, format Format) error {
	writer, err := NewWriter(w, format)
	if err != nil {
		return err
	}
	for _, r := range recs {
		if err := writer.Write(r); err != nil {
			return err
		}
	}
	return writer.Close()
}

// Criteria narrows a record set. Empty criteria match everything.
type Criteria struct {
	// Query is matched case-insensitively, in the given Scope.
	Query string
	// Tag, when set, requires the record to carry that field.
	Tag string
	// Scope decides which index Query is matched against. The zero value
	// searches the list key alone, which is the cheap one.
	Scope marcio.Scope
}

// labelAsUTF8 sets leader/09 to 'a'. Records without a leader are left alone;
// the writers report those.
func labelAsUTF8(r *marc.Record) error {
	if r == nil || r.Leader == nil || r.Leader.CodingScheme() == 'a' {
		return nil
	}
	return r.Leader.SetCodingScheme("a")
}

// Filter keeps the records matching c, preserving their order.
func Filter(recs []*marc.Record, c Criteria) []*marc.Record {
	m := c.Matcher()
	if m.Everything() {
		return recs
	}

	out := make([]*marc.Record, 0, len(recs))
	for _, r := range recs {
		if m.Match(r) {
			out = append(out, r)
		}
	}
	return out
}
