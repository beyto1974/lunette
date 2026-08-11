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
	"github.com/charmbracelet/x/ansi"
)

// Mode is one of the four ways a record can be shown.
type Mode int

const (
	Annotated Mode = iota
	Compact
	Raw
	JSON
	XML
)

func (m Mode) String() string {
	switch m {
	case Annotated:
		return "annotated"
	case Compact:
		return "compact"
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
	for _, m := range Modes() {
		if m.String() == strings.ToLower(strings.TrimSpace(s)) {
			return m, nil
		}
	}
	return Annotated, fmt.Errorf("unknown render mode %q (want annotated, compact, raw, json or xml)", s)
}

// Modes lists every mode in toggle order.
func Modes() []Mode { return []Mode{Annotated, Compact, Raw, JSON, XML} }

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
	// Indent pretty-prints the JSON and XML views, which the library emits as
	// a single line. It does not affect export, whose output is meant for
	// other programs.
	Indent bool
	// Width wraps the annotated and compact views at this many cells,
	// indenting continuation lines under the value they belong to. Zero
	// leaves lines unwrapped, which is what piped output wants.
	Width int
}

const defaultChromaStyle = "nord"

// Render produces the text for one record.
func Render(rec *marc.Record, mode Mode, o Options) (string, error) {
	if rec == nil {
		return "", fmt.Errorf("nil record")
	}
	lay, err := RenderLayout(rec, mode, o)
	return lay.Text, err
}

// FieldSpan is the half-open range of rendered lines one field occupies.
type FieldSpan struct {
	Tag   string
	Start int
	End   int
}

// Layout is a rendering together with the lines each field occupies, which is
// what lets the browser put a cursor on a field rather than on a line.
// JSON and XML carry no spans: they have no field structure to point at.
type Layout struct {
	Text   string
	Fields []FieldSpan
}

// RenderLayout produces the text for one record and the field spans within it.
func RenderLayout(rec *marc.Record, mode Mode, o Options) (Layout, error) {
	if rec == nil {
		return Layout{}, fmt.Errorf("nil record")
	}
	switch mode {
	case Annotated:
		return annotated(rec, o), nil
	case Compact:
		return compact(rec, o), nil
	case Raw:
		return raw(rec, o), nil
	case JSON:
		s, err := rec.AsJSON()
		if err != nil {
			return Layout{}, err
		}
		// Defuse before indenting: the writer emits one line, so any control
		// character in it came from the record, while the newlines added by
		// indenting are structure and must survive.
		s = defuse(s)
		if o.Indent {
			s = indentJSON(s)
		}
		return Layout{Text: structured(s, "json", o)}, nil
	case XML:
		s, err := recordXML(rec)
		if err != nil {
			return Layout{}, err
		}
		s = defuse(s)
		if o.Indent {
			s = indentXML(s)
		}
		return Layout{Text: structured(s, "xml", o)}, nil
	default:
		return Layout{}, fmt.Errorf("unknown render mode %d", mode)
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

// structured syntax-highlights JSON or XML and marks the search term in it.
// Both steps are colour-only: plain output must stay free of escapes so that
// piping it stays useful.
func structured(source, lexer string, o Options) string {
	if !o.Color {
		return source
	}
	return highlightMatches(chromaHighlight(source, lexer, o), o.Match)
}

// chromaHighlight runs source through chroma.
func chromaHighlight(source, lexer string, o Options) string {
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
func raw(rec *marc.Record, o Options) Layout {
	// The line breaks between fields are gomarc's own structure, so defuse
	// each line rather than the whole string, which would turn them into ^J.
	lines := strings.Split(rec.String(), "\n")
	for i, line := range lines {
		lines[i] = Sanitize(line)
	}
	text := strings.Join(lines, "\n")

	// Breaker format puts each field on its own line, after the leader line.
	var spans []FieldSpan
	for i, line := range lines {
		if len(line) < 4 || line[0] != '=' || !isTag(line[1:4]) {
			continue
		}
		spans = append(spans, FieldSpan{Tag: line[1:4], Start: i, End: i + 1})
	}

	if !o.Color {
		return Layout{Text: text, Fields: spans}
	}

	p := newPalette()
	var b strings.Builder
	for i, line := range lines {
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
	return Layout{Text: b.String(), Fields: spans}
}

// builder accumulates rendered lines and counts them, so a caller can record
// which lines a field occupied.
type builder struct {
	b     strings.Builder
	lines int
}

func (bb *builder) writeLine(s string) {
	bb.b.WriteString(s)
	bb.b.WriteByte('\n')
	bb.lines++
}

func (bb *builder) String() string { return bb.b.String() }

// compact is one field per line with its subfields inline. It keeps the tag,
// the '#' convention for blank indicators and the colours of the annotated
// view, but drops the labels and the line breaks - the density you want when
// scanning or comparing records rather than reading one.
func compact(rec *marc.Record, o Options) Layout {
	p := newPalette()
	if !o.Color {
		p = plainPalette()
	}

	var b builder
	var spans []FieldSpan
	if rec.Leader != nil {
		writeWrapped(&b, p.tag.Render("LDR")+"    ", p.leader.Render(Sanitize(rec.Leader.String())), tagIndent, o.Width)
	}

	for _, f := range rec.Fields {
		start := b.lines
		if f.IsControlField() {
			writeWrapped(&b, p.tag.Render(f.Tag)+"    ", p.value(f.Data, o.Match), tagIndent, o.Width)
			spans = append(spans, FieldSpan{Tag: f.Tag, Start: start, End: b.lines})
			continue
		}

		prefix := fmt.Sprintf("%s %s%s ",
			p.tag.Render(f.Tag),
			p.ind.Render(indicator(f.Indicator1())),
			p.ind.Render(indicator(f.Indicator2())),
		)
		parts := make([]string, 0, len(f.Subfields))
		for _, sf := range f.Subfields {
			parts = append(parts, p.code.Render("$"+sf.Code)+" "+p.value(sf.Value, o.Match))
		}
		writeWrapped(&b, prefix, strings.Join(parts, " "), tagIndent, o.Width)
		spans = append(spans, FieldSpan{Tag: f.Tag, Start: start, End: b.lines})
	}
	return Layout{Text: strings.TrimRight(b.String(), "\n"), Fields: spans}
}

// annotated is the reading view: decoded leader, then every field with its
// MARC21 label and one subfield per line.
func annotated(rec *marc.Record, o Options) Layout {
	p := newPalette()
	if !o.Color {
		p = plainPalette()
	}

	var b builder
	var spans []FieldSpan
	writeLeader(&b, rec, p, o.Width)

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
		start := b.lines
		writeField(&b, f, p, o, false)
		if partners, err := rec.GetLinkedFields(f); err == nil {
			for _, l := range partners {
				writeField(&b, l, p, o, true)
			}
		}
		spans = append(spans, FieldSpan{Tag: f.Tag, Start: start, End: b.lines})
	}
	return Layout{Text: strings.TrimRight(b.String(), "\n"), Fields: spans}
}

func writeLeader(b *builder, rec *marc.Record, p palette, width int) {
	if rec.Leader == nil {
		return
	}
	l := rec.Leader
	writeWrapped(b, p.label.Render("LEADER")+" ", p.leader.Render(Sanitize(l.String())), tagIndent, width)
	writeWrapped(b, "       ", p.dim.Render(fmt.Sprintf(
		"status=%s  type=%s  biblevel=%s  encoding=%s  desc=%s",
		describe(l.RecordStatus(), recordStatus),
		describe(l.TypeOfRecord(), recordType),
		describe(l.BibliographicLevel(), bibLevel),
		describe(l.EncodingLevel(), encodingLevel),
		describe(l.CatalogingForm(), catalogingForm),
	)), tagIndent, width)
	b.writeLine("")
}

// Continuation indents: under the label, past "245 10 ", and under a subfield
// value, past "       $a ".
const (
	tagIndent      = 7
	subfieldIndent = 10
)

// writeField renders one field: header line, then subfields indented under it.

func writeField(b *builder, f *marc.Field, p palette, o Options, vernacular bool) {
	name := TagName(f.Tag)
	if vernacular && name != "" {
		name += " (vernacular)"
	}

	if f.IsControlField() {
		writeWrapped(b, p.tag.Render(f.Tag)+"    ", p.label.Render(name), tagIndent, o.Width)
		// A control field's data sits at column 7, so its continuations do too.
		writeWrapped(b, "       ", p.value(f.Data, o.Match), tagIndent, o.Width)
		return
	}

	writeWrapped(b, fmt.Sprintf("%s %s%s ",
		p.tag.Render(f.Tag),
		p.ind.Render(indicator(f.Indicator1())),
		p.ind.Render(indicator(f.Indicator2())),
	), p.label.Render(name), tagIndent, o.Width)
	for _, sf := range f.Subfields {
		writeWrapped(b,
			"       "+p.code.Render("$"+sf.Code)+" ",
			p.value(sf.Value, o.Match),
			subfieldIndent, o.Width)
	}
}

// writeWrapped writes prefix followed by body, wrapping at width and indenting
// every continuation line by indent cells so it sits under the value rather
// than under the tag. Width 0 disables wrapping. Measurement is ANSI-aware, so
// styled text wraps at the same column as plain text - which also keeps the
// line numbers the match navigator computes from the plain rendering valid.
func writeWrapped(b *builder, prefix, body string, indent, width int) {
	if width <= 0 || ansi.StringWidth(prefix)+ansi.StringWidth(body) <= width {
		b.writeLine(prefix + body)
		return
	}

	limit := width - indent
	if limit < 8 {
		// Too narrow to indent usefully; wrap against the full width instead.
		limit, indent = width, 0
	}

	// ansi.Wrap breaks over-long words, which matters for URLs in 856 $u.
	wrapped := ansi.Wrap(body, limit, "")
	pad := strings.Repeat(" ", indent)
	for i, line := range strings.Split(wrapped, "\n") {
		if i == 0 {
			b.writeLine(prefix + line)
		} else {
			b.writeLine(pad + line)
		}
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
