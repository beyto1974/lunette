package export

import (
	"bytes"
	"testing"

	"github.com/beyto1974/lunette/internal/marcio"
)

// A streaming writer must produce exactly what the slice-at-a-time writer
// produces, or exporting a file too large to hold in memory would give a
// different result from exporting a small one.
func TestWriterMatchesWrite(t *testing.T) {
	for _, format := range []Format{FormatMRC, FormatXML, FormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			var want bytes.Buffer
			if err := Write(&want, load(t), format); err != nil {
				t.Fatalf("Write: %v", err)
			}

			var got bytes.Buffer
			w, err := NewWriter(&got, format)
			if err != nil {
				t.Fatalf("NewWriter: %v", err)
			}
			for _, rec := range load(t) {
				if err := w.Write(rec); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			if got.String() != want.String() {
				t.Errorf("streamed output differs from Write\n got %q\nwant %q", got.String(), want.String())
			}
		})
	}
}

func TestNewWriterUnknownFormat(t *testing.T) {
	if _, err := NewWriter(&bytes.Buffer{}, Format("dvd")); err == nil {
		t.Error("NewWriter accepted an unknown format")
	}
}

// An empty export still has to be a well-formed document in the formats that
// have a wrapper, which is what Close is for.
func TestWriterEmpty(t *testing.T) {
	for _, format := range []Format{FormatMRC, FormatXML, FormatJSON} {
		var got, want bytes.Buffer
		if err := Write(&want, nil, format); err != nil {
			t.Fatalf("Write: %v", err)
		}
		w, err := NewWriter(&got, format)
		if err != nil {
			t.Fatalf("NewWriter: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if got.String() != want.String() {
			t.Errorf("%s: empty stream = %q, want %q", format, got.String(), want.String())
		}
	}
}

// Criteria.Match is what Filter does to one record, and streaming needs it
// one record at a time.
func TestCriteriaMatch(t *testing.T) {
	recs := load(t)
	tests := []struct {
		name string
		c    Criteria
		want int
	}{
		{"everything", Criteria{}, len(recs)},
		{"tag", Criteria{Tag: "856"}, len(Filter(recs, Criteria{Tag: "856"}))},
		{"query", Criteria{Query: "café"}, len(Filter(recs, Criteria{Query: "café"}))},
		{"full text", Criteria{Query: "brussel", Scope: marcio.ScopeBoth}, len(Filter(recs, Criteria{Query: "brussel", Scope: marcio.ScopeBoth}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := 0
			for _, rec := range recs {
				if tt.c.Match(rec) {
					got++
				}
			}
			if got != tt.want {
				t.Errorf("Match kept %d records, Filter kept %d", got, tt.want)
			}
		})
	}
}
