// Package export writes record sets back out as binary MARC21, MARCXML or
// MARC-in-JSON, optionally narrowed by the same criteria the TUI filters with.
package export

import (
	"fmt"
	"io"
	"strings"

	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/marcview/internal/marcio"
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
func Write(w io.Writer, recs []*marc.Record, format Format) error {
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
	// Query is matched case-insensitively against the record's search key -
	// control number, title, author and year - or, with FullText, against
	// every subfield value in the record.
	Query string
	// Tag, when set, requires the record to carry that field.
	Tag string
	// FullText widens Query to the whole record. It costs a walk of every
	// field per record, so it is opt-in.
	FullText bool
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
			haystack := marcio.SearchKey(r)
			if c.FullText {
				haystack = marcio.FullTextKey(r)
			}
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
