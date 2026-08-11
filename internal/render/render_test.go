package render

import (
	"encoding/json"
	"encoding/xml"
	"path/filepath"
	"strings"
	"testing"

	marc "github.com/beyto1974/gomarc"

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
