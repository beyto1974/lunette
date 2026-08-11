package render

import (
	"encoding/json"
	"encoding/xml"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	marc "github.com/beyto1974/gomarc"
	"github.com/charmbracelet/x/ansi"

	"github.com/beyto1974/marcview/internal/marcio"
)

func load(t *testing.T) []*marc.Record {
	t.Helper()
	res, err := marcio.LoadFile(filepath.Join("..", "..", "testdata", "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	return res.Records
}

func render(t *testing.T, rec *marc.Record, mode Mode, o Options) string {
	t.Helper()
	out, err := Render(rec, mode, o)
	if err != nil {
		t.Fatalf("Render(%v): %v", mode, err)
	}
	return out
}

func TestAnnotated(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Annotated, Options{})

	for _, want := range []string{
		"LEADER",
		"status=n",
		"type=a",
		"245",
		"Title Statement",
		"$a Identification of Transmission Lines",
		"$b from time-domain measurements",
		"856",
		"Electronic Location and Access",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("annotated output missing %q\n---\n%s", want, out)
		}
	}
}

// Blank indicators are invisible in a terminal, so they render as '#'.
func TestAnnotatedBlankIndicators(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Annotated, Options{})
	if !strings.Contains(out, "260 ##") {
		t.Errorf("want blank indicators rendered as '260 ##'\n---\n%s", out)
	}
	if !strings.Contains(out, "245 10") {
		t.Errorf("want indicators '245 10'\n---\n%s", out)
	}
}

// Control fields have no indicators or subfields.
func TestAnnotatedControlField(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Annotated, Options{})
	if !strings.Contains(out, "001    Control Number") {
		t.Errorf("want a control-field line for 001\n---\n%s", out)
	}
	if !strings.Contains(out, "rec-0001") {
		t.Errorf("want the 001 value in the output\n---\n%s", out)
	}
}

// Local and unregistered tags still render, just without a label.
func TestAnnotatedUnknownTag(t *testing.T) {
	recs := load(t)
	out := render(t, recs[2], Annotated, Options{})
	if !strings.Contains(out, "999") || !strings.Contains(out, "local field") {
		t.Errorf("want tag 999 and its value\n---\n%s", out)
	}
}

func TestRaw(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Raw, Options{})
	// gomarc's String() is pymarc's breaker format: "=245  10$aTitle".
	if !strings.Contains(out, "=245  10") || !strings.Contains(out, "$aIdentification") {
		t.Errorf("raw output does not look like MARC breaker form\n---\n%s", out)
	}
	if !strings.Contains(out, "=LDR") {
		t.Errorf("raw output missing the leader line\n---\n%s", out)
	}
	if strings.Contains(out, "Title Statement") {
		t.Error("raw output must not carry annotated field labels")
	}
}

// Compact puts a whole field on one line, keeping the '#' indicator
// convention but dropping the labels and the per-subfield line breaks.
func TestCompact(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Compact, Options{})

	for _, want := range []string{
		"LDR    00297nam a2200097 a 4500",
		"001    rec-0001",
		"245 10 $a Identification of Transmission Lines $b from time-domain measurements",
		"260 ## $c 2002",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compact output missing line %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Title Statement") {
		t.Error("compact output must not carry field labels")
	}

	// One line per field, plus the leader.
	if got, want := len(strings.Split(out, "\n")), len(recs[0].Fields)+1; got != want {
		t.Errorf("compact used %d lines for %d fields, want %d", got, len(recs[0].Fields), want)
	}
}

// Long values wrap under an indent rather than running past the pane edge.
func TestWrapping(t *testing.T) {
	recs := load(t)
	const width = 40

	for _, mode := range []Mode{Annotated, Compact} {
		t.Run(mode.String(), func(t *testing.T) {
			out := render(t, recs[0], mode, Options{Width: width})

			for i, line := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(line); w > width {
					t.Errorf("line %d is %d cells wide, want at most %d: %q", i, w, width, line)
				}
			}

			// Wrapping must not lose or reorder text. The 856 URL is the
			// longest single token in the fixture.
			flat := strings.Join(strings.Fields(out), " ")
			if !strings.Contains(flat, "https://biblio.vub.ac.be/vubir/rec-0001.html") &&
				!strings.Contains(strings.ReplaceAll(flat, " ", ""),
					"https://biblio.vub.ac.be/vubir/rec-0001.html") {
				t.Errorf("the 856 URL did not survive wrapping:\n%s", out)
			}
		})
	}
}

// Continuation lines line up under the value they belong to, so the tag column
// stays readable.
func TestWrapIndentsContinuations(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Compact, Options{Width: 40})

	var continuations int
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "       ") && strings.TrimSpace(line) != "" {
			continuations++
		}
	}
	if continuations == 0 {
		t.Errorf("nothing wrapped at width 40:\n%s", out)
	}
}

// A continuation must line up under the text it continues, which for a control
// field is column 7, not the column a subfield value would start at.
func TestControlFieldWrapAlignment(t *testing.T) {
	recs := load(t)
	lines := strings.Split(render(t, recs[0], Annotated, Options{Width: 40}), "\n")

	found := false
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "181023") {
			continue
		}
		found = true
		if got := indentOf(line); got != 7 {
			t.Errorf("008 value starts at column %d, want 7", got)
		}
		for _, cont := range lines[i+1:] {
			if !strings.HasPrefix(cont, " ") {
				break
			}
			if got := indentOf(cont); got != 7 {
				t.Errorf("008 continuation is indented %d, want 7: %q", got, cont)
			}
		}
		break
	}
	if !found {
		t.Fatalf("008 field not found in:\n%s", strings.Join(lines, "\n"))
	}
}

func indentOf(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// Width 0 means no wrapping, which is what piped output wants.
func TestNoWrapByDefault(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Compact, Options{})
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "       ") {
			t.Errorf("unwrapped output has a continuation line: %q", line)
		}
	}
	if !strings.Contains(out, "$u https://biblio.vub.ac.be/vubir/rec-0001.html") {
		t.Error("the 856 URL should be on one line when wrapping is off")
	}
}

// Wrapping measures printable width, so colour must not shorten the lines.
func TestWrappingWithColor(t *testing.T) {
	recs := load(t)
	const width = 48
	out := render(t, recs[0], Annotated, Options{Width: width, Color: true, Match: "transmission"})
	for i, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("coloured line %d is %d cells wide, want at most %d", i, w, width)
		}
	}
}

func TestJSONIsValid(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], JSON, Options{})
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("JSON mode produced invalid JSON: %v\n---\n%s", err, out)
	}
}

func TestXMLIsValid(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], XML, Options{})
	if err := xml.Unmarshal([]byte(out), new(any)); err != nil {
		t.Fatalf("XML mode produced invalid XML: %v\n---\n%s", err, out)
	}
}

// The structured views are one long line as the library emits them, which is
// unreadable in a pane; Indent makes them navigable.
func TestIndentJSON(t *testing.T) {
	recs := load(t)

	flat := render(t, recs[0], JSON, Options{})
	if strings.Contains(flat, "\n") {
		t.Error("JSON should be a single line without Indent")
	}

	pretty := render(t, recs[0], JSON, Options{Indent: true})
	if !strings.Contains(pretty, "\n") {
		t.Fatalf("Indent produced no line breaks:\n%s", pretty)
	}
	var a, b any
	if err := json.Unmarshal([]byte(pretty), &a); err != nil {
		t.Fatalf("indented JSON is invalid: %v", err)
	}
	if err := json.Unmarshal([]byte(flat), &b); err != nil {
		t.Fatalf("flat JSON is invalid: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("indenting changed the JSON structure")
	}
}

func TestIndentXML(t *testing.T) {
	recs := load(t)

	flat := render(t, recs[0], XML, Options{})
	pretty := render(t, recs[0], XML, Options{Indent: true})

	if strings.Count(pretty, "\n") < 5 {
		t.Fatalf("Indent produced too few line breaks:\n%s", pretty)
	}
	if err := xml.Unmarshal([]byte(pretty), new(any)); err != nil {
		t.Fatalf("indented XML is invalid: %v", err)
	}
	for _, want := range []string{"rec-0001", "Identification of Transmission Lines", "856"} {
		if !strings.Contains(pretty, want) {
			t.Errorf("indented XML lost %q", want)
		}
	}
	if strings.Count(flat, "\n") >= strings.Count(pretty, "\n") {
		t.Error("flat XML should have fewer lines than indented XML")
	}
}

// MARC fixed fields are position-significant: 008 is padded to 40 characters
// and its trailing spaces carry meaning, so indenting must not trim them.
func TestIndentXMLPreservesFixedFields(t *testing.T) {
	recs := load(t)
	pretty := render(t, recs[0], XML, Options{Indent: true})

	back, err := marcio.Load(strings.NewReader(pretty))
	if err != nil {
		t.Fatalf("indented XML does not load: %v", err)
	}
	if len(back.Records) != 1 {
		t.Fatalf("loaded %d records, want 1", len(back.Records))
	}

	original, reloaded := recs[0].Get("008"), back.Records[0].Get("008")
	if original == nil || reloaded == nil {
		t.Fatal("008 missing")
	}
	if reloaded.Data != original.Data {
		t.Errorf("008 changed through indenting:\n got %q\nwant %q", reloaded.Data, original.Data)
	}
}

// The namespace belongs on the collection element, not on every element in it.
func TestIndentXMLNamespace(t *testing.T) {
	recs := load(t)
	pretty := render(t, recs[0], XML, Options{Indent: true})

	if n := strings.Count(pretty, "xmlns="); n != 1 {
		t.Errorf("found %d xmlns declarations, want 1:\n%s", n, pretty)
	}
	if !strings.Contains(pretty, `<collection xmlns="http://www.loc.gov/MARC21/slim">`) {
		t.Errorf("collection element is not declared as MARCXML:\n%s", pretty)
	}
}

// Chroma colours JSON and XML with its own palette, so the match is marked by
// inverting the cell rather than by setting a colour that would fight it.
func TestMatchHighlightInStructuredViews(t *testing.T) {
	recs := load(t)

	for _, mode := range []Mode{JSON, XML} {
		t.Run(mode.String(), func(t *testing.T) {
			plain := render(t, recs[0], mode, Options{Color: true, Indent: true})
			marked := render(t, recs[0], mode, Options{Color: true, Indent: true, Match: "transmission"})

			if marked == plain {
				t.Fatal("Match had no effect")
			}
			if !strings.Contains(marked, "\x1b[7m") || !strings.Contains(marked, "\x1b[27m") {
				t.Error("want the match wrapped in reverse video (SGR 7/27)")
			}
			// Inverting must not disturb the text or chroma's own colours.
			if stripANSI(marked) != stripANSI(plain) {
				t.Errorf("highlighting changed the text:\n%s", stripANSI(marked))
			}
		})
	}
}

// Highlighting is case-insensitive and works on uncoloured output too.
func TestMatchHighlightPlainStructured(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], JSON, Options{Match: "TRANSMISSION"})
	if strings.Contains(out, "\x1b[") {
		t.Error("plain output must stay free of escapes even with a match")
	}
}

// Colour is opt-in: the default is plain text so that export and tests stay
// byte-comparable.
func TestPlainOutputHasNoEscapes(t *testing.T) {
	recs := load(t)
	for _, mode := range Modes() {
		out := render(t, recs[0], mode, Options{})
		if strings.Contains(out, "\x1b[") {
			t.Errorf("%v: plain output contains ANSI escapes", mode)
		}
	}
}

func TestColorOutputHasEscapes(t *testing.T) {
	recs := load(t)
	for _, mode := range Modes() {
		out := render(t, recs[0], mode, Options{Color: true})
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("%v: coloured output contains no ANSI escapes\n---\n%s", mode, out)
		}
	}
}

// Colour must not alter the text itself, only wrap it.
func TestColorPreservesText(t *testing.T) {
	recs := load(t)
	plain := render(t, recs[1], Annotated, Options{})
	colored := render(t, recs[1], Annotated, Options{Color: true})
	if !strings.Contains(stripANSI(colored), "café-cultuur") {
		t.Errorf("colour changed the text\nplain:\n%s\ncolored:\n%s", plain, stripANSI(colored))
	}
}

func TestMatchHighlight(t *testing.T) {
	recs := load(t)
	out := render(t, recs[0], Annotated, Options{Color: true, Match: "transmission"})
	if !strings.Contains(stripANSI(out), "Identification of Transmission Lines") {
		t.Errorf("highlighting mangled the text\n---\n%s", stripANSI(out))
	}
	plain := render(t, recs[0], Annotated, Options{Color: true})
	if out == plain {
		t.Error("Match had no effect on the rendered output")
	}
}

func TestTagName(t *testing.T) {
	tests := map[string]string{
		"245": "Title Statement",
		"100": "Main Entry - Personal Name",
		"856": "Electronic Location and Access",
		"999": "",
	}
	for tag, want := range tests {
		if got := TagName(tag); got != want {
			t.Errorf("TagName(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestModeString(t *testing.T) {
	if Annotated.String() != "annotated" || Raw.String() != "raw" {
		t.Error("Mode.String is wrong")
	}
	if got, err := ParseMode("json"); err != nil || got != JSON {
		t.Errorf("ParseMode(json) = %v, %v", got, err)
	}
	if got, err := ParseMode("compact"); err != nil || got != Compact {
		t.Errorf("ParseMode(compact) = %v, %v", got, err)
	}
	if Compact.String() != "compact" {
		t.Errorf("Compact.String() = %q", Compact.String())
	}
	if len(Modes()) != 5 {
		t.Errorf("Modes() has %d entries, want 5", len(Modes()))
	}
	if _, err := ParseMode("nope"); err == nil {
		t.Error("ParseMode accepted an unknown mode")
	}
}

// stripANSI removes CSI sequences so tests can compare visible text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
