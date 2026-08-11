package render

import (
	"strings"
	"testing"
)

// A field cursor needs to know which rendered lines belong to which field.
func TestLayoutSpans(t *testing.T) {
	recs := load(t)

	for _, mode := range []Mode{Annotated, Compact, Raw} {
		t.Run(mode.String(), func(t *testing.T) {
			lay, err := RenderLayout(recs[0], mode, Options{})
			if err != nil {
				t.Fatalf("RenderLayout: %v", err)
			}

			if got, want := len(lay.Fields), len(recs[0].Fields); got != want {
				t.Fatalf("got %d spans for %d fields", got, want)
			}

			lines := strings.Split(lay.Text, "\n")
			for i, span := range lay.Fields {
				if span.Start < 0 || span.End > len(lines) || span.Start >= span.End {
					t.Fatalf("span %d out of range: %+v (%d lines)", i, span, len(lines))
				}
				if span.Tag != recs[0].Fields[i].Tag {
					t.Errorf("span %d is tagged %q, want %q", i, span.Tag, recs[0].Fields[i].Tag)
				}
				// The first line of a span must mention its tag.
				if !strings.Contains(lines[span.Start], span.Tag) {
					t.Errorf("span %d starts on %q, which does not name tag %s",
						i, lines[span.Start], span.Tag)
				}
			}

			// Spans must not overlap and must run in order.
			for i := 1; i < len(lay.Fields); i++ {
				if lay.Fields[i].Start < lay.Fields[i-1].End {
					t.Errorf("span %d starts at %d, before span %d ends at %d",
						i, lay.Fields[i].Start, i-1, lay.Fields[i-1].End)
				}
			}
		})
	}
}

// Wrapped fields span every line they occupy, not just the first.
func TestLayoutSpansCoverWrappedLines(t *testing.T) {
	recs := load(t)
	lay, err := RenderLayout(recs[0], Compact, Options{Width: 40})
	if err != nil {
		t.Fatalf("RenderLayout: %v", err)
	}

	// The 856 URL is far longer than 40 cells, so its span must be multi-line.
	var found bool
	for _, span := range lay.Fields {
		if span.Tag != "856" {
			continue
		}
		found = true
		if span.End-span.Start < 2 {
			t.Errorf("856 span covers %d line(s), want it to wrap", span.End-span.Start)
		}
	}
	if !found {
		t.Fatal("no 856 span")
	}
}

// The text must be identical to what Render produces, so the two cannot drift.
func TestLayoutTextMatchesRender(t *testing.T) {
	recs := load(t)
	for _, mode := range []Mode{Annotated, Compact, Raw} {
		lay, err := RenderLayout(recs[0], mode, Options{Width: 60})
		if err != nil {
			t.Fatalf("RenderLayout: %v", err)
		}
		plain := render(t, recs[0], mode, Options{Width: 60})
		if lay.Text != plain {
			t.Errorf("%v: layout text differs from Render output", mode)
		}
	}
}

// JSON and XML have no field structure to point a cursor at.
func TestLayoutStructuredHasNoSpans(t *testing.T) {
	recs := load(t)
	for _, mode := range []Mode{JSON, XML} {
		lay, err := RenderLayout(recs[0], mode, Options{})
		if err != nil {
			t.Fatalf("RenderLayout: %v", err)
		}
		if len(lay.Fields) != 0 {
			t.Errorf("%v: got %d spans, want none", mode, len(lay.Fields))
		}
		if lay.Text == "" {
			t.Errorf("%v: no text", mode)
		}
	}
}
