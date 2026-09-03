package marcio

import (
	"strings"
	"testing"
)

// The encoding report used to read the whole document into memory, which on a
// 3.7 GB harvest is 3.7 GB. Reading it in chunks has to give the same answer,
// whatever a chunk boundary happens to fall on: the middle of a <record> tag,
// or the middle of a character.
func TestXMLReportIsChunkIndependent(t *testing.T) {
	docs := map[string]string{
		"ascii":            "<collection>" + strings.Repeat("<record>x</record>", 200) + "</collection>",
		"utf8":             "<collection>" + strings.Repeat("<record>café-cultuur — dépôt</record>", 200) + "</collection>",
		"latin1":           "<collection><record>caf\xe9</record>" + strings.Repeat("<record>x</record>", 200) + "</collection>",
		"marker at an end": strings.Repeat("<record>xxxxxx</record>", 300),
	}

	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			want := &EncodingReport{Format: FormatXML}
			if err := analyzeXMLChunked(strings.NewReader(doc), want, len(doc)+16); err != nil {
				t.Fatalf("whole document: %v", err)
			}

			// Every size from a few bytes upwards, so that a boundary lands
			// inside every construct in turn.
			for _, size := range []int{1, 2, 3, 5, 7, 8, 13, 64, 1024} {
				got := &EncodingReport{Format: FormatXML}
				if err := analyzeXMLChunked(strings.NewReader(doc), got, size); err != nil {
					t.Fatalf("chunk %d: %v", size, err)
				}
				if got.Records != want.Records || got.WithNonASCII != want.WithNonASCII || got.InvalidUTF8 != want.InvalidUTF8 {
					t.Errorf("chunk %d: report = %+v, want %+v", size, got, want)
				}
			}
		})
	}
}

// The count is of <record> tags, and a document holding none has none.
func TestXMLReportCounts(t *testing.T) {
	rep := &EncodingReport{Format: FormatXML}
	if err := analyzeXMLChunked(strings.NewReader("<collection></collection>"), rep, 4); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if rep.Records != 0 {
		t.Errorf("Records = %d, want 0", rep.Records)
	}
	if rep.WithNonASCII != 0 || rep.InvalidUTF8 != 0 {
		t.Errorf("report = %+v, want a clean ASCII document", rep)
	}
}
