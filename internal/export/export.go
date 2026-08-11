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
	for _, r := range recs {
		if err := labelAsUTF8(r); err != nil {
			return err
		}
	}

	switch format {
	case FormatMRC:
		writer := marc.NewWriter(w)
		for i, r := range recs {
			if err := writer.Write(r); err != nil {
				return fmt.Errorf("record %d: %w", i+1, err)
			}
		}
		return nil

	case FormatXML:
		writer, err := marc.NewXMLWriter(w)
		if err != nil {
			return err
		}
		for i, r := range recs {
			if err := writer.Write(r); err != nil {
				return fmt.Errorf("record %d: %w", i+1, err)
			}
		}
		return writer.Close()

	case FormatJSON:
		writer, err := marc.NewJSONWriter(w)
		if err != nil {
			return err
		}
		for i, r := range recs {
			if err := writer.Write(r); err != nil {
				return fmt.Errorf("record %d: %w", i+1, err)
			}
		}
		return writer.Close()

	default:
		return fmt.Errorf("unknown format %q (want mrc, xml or json)", format)
	}
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
	query := strings.ToLower(strings.TrimSpace(c.Query))
	tag := strings.TrimSpace(c.Tag)
	if query == "" && tag == "" {
		return recs
	}

	out := make([]*marc.Record, 0, len(recs))
	for _, r := range recs {
		if tag != "" && !marcio.HasTag(r, tag) {
			continue
		}
		if query != "" {
			var full string
			if c.Scope.NeedsFullText() {
				full = marcio.FullTextKey(r)
			}
			if !c.Scope.Matches(marcio.SearchKey(r), full, query) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
