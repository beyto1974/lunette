package export

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"path/filepath"
	"testing"

	marc "github.com/beyto1974/gomarc"

	"github.com/everbright/marco/marcview/internal/marcio"
)

func load(t *testing.T) []*marc.Record {
	t.Helper()
	res, err := marcio.LoadFile(filepath.Join("..", "..", "testdata", "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	return res.Records
}

// Exporting to binary MARC and reading the result back must preserve the
// records, which is the round-trip the harvest pipeline depends on.
func TestWriteBinaryRoundTrip(t *testing.T) {
	recs := load(t)
	var buf bytes.Buffer
	if err := Write(&buf, recs, FormatMRC); err != nil {
		t.Fatalf("Write: %v", err)
	}

	back, err := marcio.Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(back.Records) != len(recs) {
		t.Fatalf("round-trip gave %d records, want %d", len(back.Records), len(recs))
	}
	if len(back.Issues) != 0 {
		t.Errorf("round-trip produced issues: %v", back.Issues)
	}
	for i := range recs {
		if got, want := marcio.Title(back.Records[i]), marcio.Title(recs[i]); got != want {
			t.Errorf("record %d title = %q, want %q", i, got, want)
		}
	}
}

func TestWriteXMLRoundTrip(t *testing.T) {
	recs := load(t)
	var buf bytes.Buffer
	if err := Write(&buf, recs, FormatXML); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := xml.Unmarshal(buf.Bytes(), new(any)); err != nil {
		t.Fatalf("output is not well-formed XML: %v", err)
	}

	back, err := marcio.Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(back.Records) != len(recs) {
		t.Fatalf("round-trip gave %d records, want %d", len(back.Records), len(recs))
	}
	if got, want := marcio.Title(back.Records[1]), marcio.Title(recs[1]); got != want {
		t.Errorf("title = %q, want %q (UTF-8 must survive)", got, want)
	}
}

func TestWriteJSON(t *testing.T) {
	recs := load(t)
	var buf bytes.Buffer
	if err := Write(&buf, recs, FormatJSON); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var v []any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, buf.String())
	}
	if len(v) != len(recs) {
		t.Errorf("JSON array has %d entries, want %d", len(v), len(recs))
	}
}

func TestWriteUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, load(t), "csv"); err == nil {
		t.Error("Write accepted an unknown format")
	}
}

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"mrc", "MRC", " xml ", "json"} {
		if _, err := ParseFormat(s); err != nil {
			t.Errorf("ParseFormat(%q): %v", s, err)
		}
	}
	if _, err := ParseFormat("marc21"); err == nil {
		t.Error("ParseFormat accepted an unknown name")
	}
}

func TestFilter(t *testing.T) {
	recs := load(t)

	tests := []struct {
		name  string
		query string
		tag   string
		want  int
	}{
		{"no criteria keeps everything", "", "", 3},
		{"query matches title", "transmission", "", 1},
		{"query is case-insensitive", "TRANSMISSION", "", 1},
		{"query matches author", "kloza", "", 1},
		{"query matches control number", "rec-0003", "", 1},
		{"query matching nothing", "zzzz", "", 0},
		{"tag present", "", "856", 1},
		{"tag absent", "", "245", 3},
		{"query and tag combined", "transmission", "856", 1},
		{"query and tag disagree", "kloza", "856", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filter(recs, tt.query, tt.tag)
			if len(got) != tt.want {
				t.Errorf("Filter(%q, %q) kept %d records, want %d", tt.query, tt.tag, len(got), tt.want)
			}
		})
	}
}
