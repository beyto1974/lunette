package render

import (
	"path/filepath"
	"strings"
	"testing"

	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/lunette/internal/marcio"
)

// loadEscapes returns the record from testdata/escapes.mrc, whose 245 $a
// carries a screen clear, an OSC 52 clipboard write and a colour change.
func loadEscapes(t *testing.T) *marc.Record {
	t.Helper()
	res, err := marcio.LoadFile(filepath.Join("..", "..", "testdata", "escapes.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("loaded %d records, want 1", len(res.Records))
	}
	return res.Records[0]
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "Identification of Lines", "Identification of Lines"},
		{"accents are untouched", "café-cultuur — données", "café-cultuur — données"},
		{"escape becomes caret notation", "a\x1bb", "a^[b"},
		{"screen clear is defused", "T\x1b[2J", "T^[[2J"},
		{"bell", "x\x07", "x^G"},
		{"carriage return", "a\rb", "a^Mb"},
		{"newline, which would break line accounting", "a\nb", "a^Jb"},
		{"tab", "a\tb", "a^Ib"},
		{"delete", "a\x7fb", "a^?b"},
		{"c1 control reads as its 7-bit form", "ab", "a^[b"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sanitize(tt.in); got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The whole point: nothing a record carries may reach the terminal as a live
// control sequence, in any view.
func TestNoEscapesEscapeAnyRenderMode(t *testing.T) {
	rec := loadEscapes(t)

	for _, mode := range Modes() {
		t.Run(mode.String(), func(t *testing.T) {
			plain := render(t, rec, mode, Options{})
			if i := strings.IndexAny(plain, "\x1b\x07"); i >= 0 {
				t.Errorf("plain %v output carries a control character at %d: %q", mode, i, plain)
			}

			// With colour on, the only escapes may be the ones this package
			// wrote itself; the record's own must still be defused.
			colored := render(t, rec, mode, Options{Color: true, Match: "harmless"})
			if strings.Contains(colored, "\x1b[2J") {
				t.Errorf("%v: the record's screen clear survived colouring", mode)
			}
			if strings.Contains(colored, "\x1b]52;") {
				t.Errorf("%v: the record's clipboard write survived colouring", mode)
			}
			if strings.Contains(colored, "\x07") {
				t.Errorf("%v: a bell survived colouring", mode)
			}
		})
	}
}

// The defused text stays readable rather than disappearing.
func TestSanitizedTextIsStillVisible(t *testing.T) {
	out := render(t, loadEscapes(t), Annotated, Options{})
	if !strings.Contains(out, "Harmless Title^[[2J") {
		t.Errorf("want the sequence shown in caret notation:\n%s", out)
	}
}

// Wrapping and field spans count cells, so a sanitized value must be measured
// after substitution, not before.
func TestSanitizedValueWrapsCorrectly(t *testing.T) {
	lay, err := RenderLayout(loadEscapes(t), Compact, Options{Width: 40})
	if err != nil {
		t.Fatalf("RenderLayout: %v", err)
	}
	for i, line := range strings.Split(lay.Text, "\n") {
		if len(line) > 40 {
			t.Errorf("line %d is %d columns wide, want at most 40: %q", i, len(line), line)
		}
	}
	for _, span := range lay.Fields {
		if span.Start >= span.End {
			t.Errorf("empty span %+v", span)
		}
	}
}
