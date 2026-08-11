// Package render turns a MARC record into text for display: an annotated form
// with decoded leader and field labels, the raw mnemonic form, MARC-in-JSON, or
// MARCXML. Colour is opt-in so the same code serves both the TUI and piped
// output.
package render

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	marc "github.com/beyto1974/gomarc"
)

// Mode is one of the four ways a record can be shown.
type Mode int

const (
	Annotated Mode = iota
	Raw
	JSON
	XML
)

func (m Mode) String() string {
	switch m {
	case Annotated:
		return "annotated"
	case Raw:
		return "raw"
	case JSON:
		return "json"
	case XML:
		return "xml"
	default:
		return "unknown"
	}
}

// ParseMode maps a mode name to a Mode.
func ParseMode(s string) (Mode, error) {
	for _, m := range []Mode{Annotated, Raw, JSON, XML} {
		if m.String() == strings.ToLower(strings.TrimSpace(s)) {
			return m, nil
		}
	}
	return Annotated, fmt.Errorf("unknown render mode %q (want annotated, raw, json or xml)", s)
}

// Modes lists every mode in toggle order.
func Modes() []Mode { return []Mode{Annotated, Raw, JSON, XML} }

// Options controls a single render.
type Options struct {
	// Color emits ANSI escapes: lipgloss styling for the MARC forms, chroma
	// syntax highlighting for JSON and XML.
	Color bool
	// Match is a search term to highlight in the annotated and raw forms.
	// Ignored when Color is false.
	Match string
	// ChromaStyle names the chroma style for JSON and XML. Defaults to a
	// style that reads acceptably on both light and dark terminals.
	ChromaStyle string
}

const defaultChromaStyle = "nord"

// Render produces the text for one record.
func Render(rec *marc.Record, mode Mode, o Options) (string, error) {
	if rec == nil {
		return "", fmt.Errorf("nil record")
	}
	switch mode {
	case Annotated:
		return annotated(rec, o), nil
	case Raw:
		return raw(rec, o), nil
	case JSON:
		s, err := rec.AsJSON()
		if err != nil {
			return "", err
		}
		return maybeHighlight(s, "json", o), nil
	case XML:
		s, err := recordXML(rec)
		if err != nil {
			return "", err
		}
		return maybeHighlight(s, "xml", o), nil
	default:
		return "", fmt.Errorf("unknown render mode %d", mode)
	}
}

// recordXML serialises one record as a standalone MARCXML document. gomarc
// only exposes XML through a collection writer, so the record is wrapped in a
// one-record collection.
func recordXML(rec *marc.Record) (string, error) {
	var buf bytes.Buffer
	w, err := marc.NewXMLWriter(&buf)
	if err != nil {
		return "", err
	}
	if err := w.Write(rec); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// maybeHighlight runs source through chroma when colour is on.
func maybeHighlight(source, lexer string, o Options) string {
	if !o.Color {
		return source
	}
	style := o.ChromaStyle
	if style == "" {
		style = defaultChromaStyle
	}
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, source, lexer, "terminal256", style); err != nil {
		// Highlighting is cosmetic; fall back to the plain text.
		return source
	}
	return buf.String()
}

// raw is the pymarc/yaz-marcdump mnemonic form, with the tag and subfield
// delimiters tinted when colour is on.
func raw(rec *marc.Record, o Options) string {
	s := rec.String()
	if !o.Color {
		return s
	}
	p := newPalette()
	var b strings.Builder
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		// Breaker lines are "=245  10$aTitle"; the leader is "=LDR  ...".
		if len(line) >= 4 && line[0] == '=' && isTag(line[1:4]) {
			b.WriteString(p.tag.Render(line[:4]))
			b.WriteString(p.value(line[4:], o.Match))
			continue
		}
		b.WriteString(p.leader.Render(line))
	}
	return b.String()
}

// annotated is the reading view: decoded leader, then every field with its
// MARC21 label and one subfield per line.
func annotated(rec *marc.Record, o Options) string {
	p := newPalette()
	if !o.Color {
		p = plainPalette()
	}

	var b strings.Builder
	writeLeader(&b, rec, p)

	// 880 fields carry the vernacular form of another field. Render each one
	// under its partner instead of stranding them all at the end.
	linked := map[*marc.Field]bool{}
	for _, f := range rec.Fields {
		if f.Tag == "880" {
			continue
		}
		if partners, err := rec.GetLinkedFields(f); err == nil {
			for _, l := range partners {
				linked[l] = true
			}
		}
	}

	for _, f := range rec.Fields {
		if f.Tag == "880" && linked[f] {
			continue // rendered next to its partner below
		}
		writeField(&b, f, p, o.Match, false)
		if partners, err := rec.GetLinkedFields(f); err == nil {
			for _, l := range partners {
				writeField(&b, l, p, o.Match, true)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeLeader(b *strings.Builder, rec *marc.Record, p palette) {
	if rec.Leader == nil {
		return
	}
	l := rec.Leader
	fmt.Fprintf(b, "%s %s\n", p.label.Render("LEADER"), p.leader.Render(l.String()))
	fmt.Fprintf(b, "       %s\n\n", p.dim.Render(fmt.Sprintf(
		"status=%s  type=%s  biblevel=%s  encoding=%s  desc=%s",
		describe(l.RecordStatus(), recordStatus),
		describe(l.TypeOfRecord(), recordType),
		describe(l.BibliographicLevel(), bibLevel),
		describe(l.EncodingLevel(), encodingLevel),
		describe(l.CatalogingForm(), catalogingForm),
	)))
}

// writeField renders one field: header line, then subfields indented under it.
func writeField(b *strings.Builder, f *marc.Field, p palette, match string, vernacular bool) {
	name := TagName(f.Tag)
	if vernacular && name != "" {
		name += " (vernacular)"
	}

	if f.IsControlField() {
		fmt.Fprintf(b, "%s    %s\n", p.tag.Render(f.Tag), p.label.Render(name))
		fmt.Fprintf(b, "       %s\n", p.value(f.Data, match))
		return
	}

	fmt.Fprintf(b, "%s %s%s %s\n",
		p.tag.Render(f.Tag),
		p.ind.Render(indicator(f.Indicator1())),
		p.ind.Render(indicator(f.Indicator2())),
		p.label.Render(name),
	)
	for _, sf := range f.Subfields {
		fmt.Fprintf(b, "       %s %s\n",
			p.code.Render("$"+sf.Code),
			p.value(sf.Value, match),
		)
	}
}

// indicator renders a blank indicator as '#', which is the convention in MARC
// documentation and the only way to see it in a terminal.
func indicator(s string) string {
	if s == "" || s == " " {
		return "#"
	}
	return s
}

func isTag(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// describe renders a leader byte as "value(meaning)", or just the value when
// the code is not one this tool knows.
func describe(b byte, table map[byte]string) string {
	shown := string(b)
	if b == ' ' {
		shown = "#"
	}
	if meaning, ok := table[b]; ok {
		return shown + "(" + meaning + ")"
	}
	return shown
}

var (
	recordStatus = map[byte]string{
		'a': "increase in encoding level", 'c': "corrected", 'd': "deleted",
		'n': "new", 'p': "increase from prepublication",
	}
	recordType = map[byte]string{
		'a': "language material", 'c': "notated music", 'd': "manuscript notated music",
		'e': "cartographic", 'f': "manuscript cartographic", 'g': "projected medium",
		'i': "nonmusical sound recording", 'j': "musical sound recording",
		'k': "two-dimensional nonprojectable graphic", 'm': "computer file",
		'o': "kit", 'p': "mixed materials", 'r': "three-dimensional object",
		't': "manuscript language material",
	}
	bibLevel = map[byte]string{
		'a': "monographic component part", 'b': "serial component part",
		'c': "collection", 'd': "subunit", 'i': "integrating resource",
		'm': "monograph", 's': "serial",
	}
	encodingLevel = map[byte]string{
		' ': "full", '1': "full, not examined", '2': "less-than-full, not examined",
		'3': "abbreviated", '4': "core", '5': "partial", '7': "minimal",
		'8': "prepublication", 'u': "unknown", 'z': "not applicable",
	}
	catalogingForm = map[byte]string{
		' ': "non-ISBD", 'a': "AACR2", 'c': "ISBD punctuation omitted",
		'i': "ISBD punctuation included", 'n': "non-ISBD punctuation omitted",
		'u': "unknown",
	}
)
