package marcio

import (
	"strings"

	marc "github.com/beyto1974/gomarc"
)

// ControlNumber returns field 001, the record's identifier within the
// originating system. Harvested records do not always carry one.
func ControlNumber(r *marc.Record) string {
	if f := r.Get("001"); f != nil {
		return strings.TrimSpace(f.Data)
	}
	return ""
}

// Title returns 245 $a $b joined, with the trailing ISBD punctuation that
// cataloguing rules put before $b left in place.
func Title(r *marc.Record) string {
	f := r.Get("245")
	if f == nil {
		return ""
	}
	parts := f.GetSubfields("a", "b")
	return strings.TrimSpace(strings.Join(parts, " "))
}

// Author returns the main entry: 100, 110 or 111 $a, whichever is present.
func Author(r *marc.Record) string {
	for _, tag := range []string{"100", "110", "111"} {
		if f := r.Get(tag); f != nil {
			if v, ok := f.Subfield("a"); ok {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// Year returns the publication year: 264 $c or 260 $c reduced to their first
// four consecutive digits, falling back to 008/07-10 (date 1) when neither
// carries a usable date.
func Year(r *marc.Record) string {
	for _, tag := range []string{"264", "260"} {
		for _, f := range r.GetFields(tag) {
			if v, ok := f.Subfield("c"); ok {
				if y := firstYear(v); y != "" {
					return y
				}
			}
		}
	}
	if f := r.Get("008"); f != nil && len(f.Data) >= 11 {
		if y := firstYear(f.Data[7:11]); y != "" {
			return y
		}
	}
	return ""
}

// firstYear pulls the first run of four digits out of s.
func firstYear(s string) string {
	run := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			run++
			if run == 4 {
				return s[i-3 : i+1]
			}
			continue
		}
		run = 0
	}
	return ""
}

// HasTag reports whether the record carries at least one field with tag.
func HasTag(r *marc.Record, tag string) bool {
	return r.Get(tag) != nil
}

// FullTextKey is every value in the record, lowercased: control-field data and
// all subfield values. It backs the "all:" filter, and is built on demand
// rather than at load time because it costs roughly as much memory as the
// records themselves.
func FullTextKey(r *marc.Record) string {
	var b strings.Builder
	for _, f := range r.Fields {
		if f.IsControlField() {
			b.WriteString(f.Data)
			b.WriteByte(' ')
			continue
		}
		for _, sf := range f.Subfields {
			b.WriteString(sf.Value)
			b.WriteByte(' ')
		}
	}
	return strings.ToLower(b.String())
}

// SearchKey is the lowercased haystack the TUI filters against. It is built
// once at load time so that filtering never walks the record's fields again.
func SearchKey(r *marc.Record) string {
	var b strings.Builder
	b.WriteString(ControlNumber(r))
	b.WriteByte(' ')
	b.WriteString(Title(r))
	b.WriteByte(' ')
	b.WriteString(Author(r))
	b.WriteByte(' ')
	b.WriteString(Year(r))
	return strings.ToLower(b.String())
}
