package export

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/lunette/internal/marcio"
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
		scope marcio.Scope
		want  int
	}{
		{"no criteria keeps everything", "", "", marcio.ScopeTitles, 3},
		{"query matches title", "transmission", "", marcio.ScopeTitles, 1},
		{"query is case-insensitive", "TRANSMISSION", "", marcio.ScopeTitles, 1},
		{"query matches author", "vandaele", "", marcio.ScopeTitles, 1},
		{"query matches control number", "rec-0003", "", marcio.ScopeTitles, 1},
		{"query matching nothing", "zzzz", "", marcio.ScopeTitles, 0},
		{"tag present", "", "856", marcio.ScopeTitles, 1},
		{"tag absent", "", "245", marcio.ScopeTitles, 3},
		{"query and tag combined", "transmission", "856", marcio.ScopeTitles, 1},
		{"query and tag disagree", "vandaele", "856", marcio.ScopeTitles, 0},
		// "Privacy" is a 650 subject, outside the list key.
		{"subject misses the key", "privacy", "", marcio.ScopeTitles, 0},
		{"subject found in the record scope", "privacy", "", marcio.ScopeRecord, 1},
		{"subject found in both", "privacy", "", marcio.ScopeBoth, 1},
		{"the record scope still honours the tag", "privacy", "856", marcio.ScopeRecord, 0},
		{"the record scope finds control-field data", "181023", "", marcio.ScopeRecord, 1},
		{"the titles scope does not", "181023", "", marcio.ScopeTitles, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filter(recs, Criteria{Query: tt.query, Tag: tt.tag, Scope: tt.scope})
			if len(got) != tt.want {
				t.Errorf("Filter(%q, %q, scope=%v) kept %d records, want %d",
					tt.query, tt.tag, tt.scope, len(got), tt.want)
			}
		})
	}
}

// mislabelled loads the fixture with leader/09 blanked, which is how OAI-PMH
// harvests usually arrive: UTF-8 bytes behind a MARC-8 leader.
func mislabelled(t *testing.T) []*marc.Record {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for off := 0; off+24 <= len(data); {
		length := 0
		for i := 0; i < 5; i++ {
			length = length*10 + int(data[off+i]-'0')
		}
		if length <= 0 {
			break
		}
		data[off+9] = ' '
		off += length
	}
	res, err := marcio.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return res.Records
}

// Everything this tool writes is UTF-8, so every record it writes must say so.
// MARCXML and MARC-in-JSON are UTF-8 by definition, and a leader inside them
// claiming MARC-8 mislabels the record for whoever converts it back.
func TestWriteFixesTheLeader(t *testing.T) {
	recs := mislabelled(t)
	if recs[0].Leader.CodingScheme() != ' ' {
		t.Fatalf("fixture is not mislabelled: leader/09 = %q", recs[0].Leader.CodingScheme())
	}

	for _, format := range []Format{FormatMRC, FormatXML, FormatJSON} {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(&buf, mislabelled(t), format); err != nil {
				t.Fatalf("Write: %v", err)
			}
			for i, leader := range leadersIn(t, buf.String(), format) {
				if leader[9] != 'a' {
					t.Errorf("record %d exported with leader/09 = %q, want 'a': %q",
						i+1, leader[9], leader)
				}
			}
		})
	}
}

// Fixing the leader must not disturb the record itself.
func TestWriteFixesLeaderWithoutChangingContent(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, mislabelled(t), FormatMRC); err != nil {
		t.Fatalf("Write: %v", err)
	}
	back, err := marcio.Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if back.ForcedUTF8 {
		t.Error("the exported file still needs an encoding override")
	}
	if len(back.Records) != 3 {
		t.Fatalf("reloaded %d records, want 3", len(back.Records))
	}
	if got := marcio.Title(back.Records[1]); !strings.Contains(got, "café-cultuur") {
		t.Errorf("title = %q, want the diacritics intact", got)
	}
	original := mislabelled(t)[0].Get("008")
	if got := back.Records[0].Get("008"); got == nil || got.Data != original.Data {
		t.Errorf("008 changed through export:\n got %#v\nwant %q", got, original.Data)
	}
}

// leadersIn pulls the leader of every record out of an exported document.
func leadersIn(t *testing.T, out string, format Format) []string {
	t.Helper()
	switch format {
	case FormatXML:
		var found []string
		for _, part := range strings.Split(out, "<leader>")[1:] {
			found = append(found, part[:strings.Index(part, "</leader>")])
		}
		return found

	case FormatJSON:
		var recs []struct {
			Leader string `json:"leader"`
		}
		if err := json.Unmarshal([]byte(out), &recs); err != nil {
			t.Fatalf("output is not JSON: %v", err)
		}
		found := make([]string, len(recs))
		for i, r := range recs {
			found[i] = r.Leader
		}
		return found

	default: // binary: each record starts with its 24-byte leader
		var found []string
		for off := 0; off+24 <= len(out); {
			length := 0
			for i := 0; i < 5; i++ {
				length = length*10 + int(out[off+i]-'0')
			}
			if length <= 0 {
				break
			}
			found = append(found, out[off:off+24])
			off += length
		}
		return found
	}
}
