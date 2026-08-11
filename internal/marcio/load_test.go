package marcio

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testdata(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", name)
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Format
	}{
		{"binary leader", "00297nam a2200097 a 4500", FormatBinary},
		{"xml declaration", "<?xml version=\"1.0\"?><collection/>", FormatXML},
		{"xml bare root", "<collection/>", FormatXML},
		{"xml after whitespace", "\n\n  <collection/>", FormatXML},
		{"garbage", "hello world", FormatUnknown},
		{"empty", "", FormatUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectFormat([]byte(tt.in)); got != tt.want {
				t.Errorf("DetectFormat(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoadFileBinary(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if res.Format != FormatBinary {
		t.Errorf("Format = %v, want FormatBinary", res.Format)
	}
	if len(res.Records) != 3 {
		t.Fatalf("got %d records, want 3", len(res.Records))
	}
	if len(res.Issues) != 0 {
		t.Errorf("got %d issues, want 0: %v", len(res.Issues), res.Issues)
	}
	if got := ControlNumber(res.Records[0]); got != "rec-0001" {
		t.Errorf("ControlNumber = %q, want rec-0001", got)
	}
	if got := Title(res.Records[1]); !strings.Contains(got, "café-cultuur") {
		t.Errorf("Title = %q, want it to contain café-cultuur (UTF-8 round-trip)", got)
	}
}

func TestLoadFileXML(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.marcxml"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if res.Format != FormatXML {
		t.Errorf("Format = %v, want FormatXML", res.Format)
	}
	if len(res.Records) != 3 {
		t.Fatalf("got %d records, want 3", len(res.Records))
	}
}

// Both encodings of the same data must yield the same logical records.
func TestBinaryAndXMLAgree(t *testing.T) {
	bin, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile binary: %v", err)
	}
	xml, err := LoadFile(testdata(t, "sample.marcxml"))
	if err != nil {
		t.Fatalf("LoadFile xml: %v", err)
	}
	if len(bin.Records) != len(xml.Records) {
		t.Fatalf("record count differs: binary %d, xml %d", len(bin.Records), len(xml.Records))
	}
	for i := range bin.Records {
		b, x := bin.Records[i], xml.Records[i]
		if ControlNumber(b) != ControlNumber(x) {
			t.Errorf("record %d: control number %q != %q", i, ControlNumber(b), ControlNumber(x))
		}
		if Title(b) != Title(x) {
			t.Errorf("record %d: title %q != %q", i, Title(b), Title(x))
		}
		if len(b.Fields) != len(x.Fields) {
			t.Errorf("record %d: field count %d != %d", i, len(b.Fields), len(x.Fields))
		}
	}
}

func TestAccessors(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	first, third := res.Records[0], res.Records[2]

	if got, want := Title(first), "Identification of Transmission Lines"; !strings.HasPrefix(got, want) {
		t.Errorf("Title = %q, want prefix %q", got, want)
	}
	if got, want := Author(first), "Neirinck, Ada"; got != want {
		t.Errorf("Author = %q, want %q", got, want)
	}
	if got, want := Year(first), "2002"; got != want {
		t.Errorf("Year = %q, want %q", got, want)
	}
	if got := Author(third); got != "" {
		t.Errorf("Author of authorless record = %q, want empty", got)
	}
	// Year falls back to 008/07-10 when 260$c is absent.
	if got, want := Year(third), "2020"; got != want {
		t.Errorf("Year from 008 = %q, want %q", got, want)
	}

	key := SearchKey(first)
	for _, want := range []string{"rec-0001", "identification", "neirinck"} {
		if !strings.Contains(key, want) {
			t.Errorf("SearchKey = %q, want it to contain %q", key, want)
		}
	}
	if key != strings.ToLower(key) {
		t.Errorf("SearchKey = %q, want lowercase", key)
	}
}

// SearchKey only covers the fields shown in the list, so a term living in a
// subject or a note needs the full-text key instead.
func TestFullTextKey(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	second := res.Records[1]

	if strings.Contains(SearchKey(second), "privacy") {
		t.Fatal("fixture assumption broken: 650 $a Privacy is already in the search key")
	}
	key := FullTextKey(second)
	for _, want := range []string{"privacy", "vandaele", "café-cultuur", "rec-0002"} {
		if !strings.Contains(key, want) {
			t.Errorf("FullTextKey = %q, want it to contain %q", key, want)
		}
	}
	if key != strings.ToLower(key) {
		t.Errorf("FullTextKey = %q, want lowercase", key)
	}
	// Control-field data counts too.
	if !strings.Contains(FullTextKey(res.Records[0]), "181023") {
		t.Error("FullTextKey should include control-field data")
	}
}

func TestHasTag(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if !HasTag(res.Records[0], "856") {
		t.Error("record 1 should have tag 856")
	}
	if HasTag(res.Records[1], "856") {
		t.Error("record 2 should not have tag 856")
	}
}

// A record with a corrupt length must be reported rather than dropped, and the
// records read before it must survive.
func TestLoadCollectsIssues(t *testing.T) {
	good, err := os.ReadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Record 1 is 297 bytes; corrupt the length field of record 2 so the file
	// still sniffs as binary MARC.
	corrupt := append([]byte(nil), good...)
	copy(corrupt[297:302], "XXXXX")

	res, err := Load(bytes.NewReader(corrupt))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Records) != 1 {
		t.Errorf("got %d records, want the 1 record preceding the corruption", len(res.Records))
	}
	if len(res.Issues) == 0 {
		t.Fatal("want at least one issue for a corrupt length, got none")
	}
	if res.Issues[0].Ordinal != 2 {
		t.Errorf("issue ordinal = %d, want 2", res.Issues[0].Ordinal)
	}
	if res.Issues[0].Offset != 297 {
		t.Errorf("issue offset = %d, want 297", res.Issues[0].Offset)
	}
	if res.Issues[0].Err == nil {
		t.Error("issue has nil error")
	}
}

// A length field shorter than the leader makes gomarc allocate a negative-sized
// slice; the loader must turn that panic into an ordinary issue and stop, since
// a panic that consumed no input would otherwise repeat forever.
func TestLoadSurvivesPanic(t *testing.T) {
	res, err := Load(bytes.NewReader([]byte("00003junk\x1d")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Records) != 0 {
		t.Errorf("got %d records, want 0", len(res.Records))
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(res.Issues))
	}
}

func TestOffsetsAdvance(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(res.Offsets) != len(res.Records) {
		t.Fatalf("got %d offsets for %d records", len(res.Offsets), len(res.Records))
	}
	if res.Offsets[0] != 0 {
		t.Errorf("first offset = %d, want 0", res.Offsets[0])
	}
	// Record lengths come from the leader: 297, 209, 157.
	if res.Offsets[1] != 297 || res.Offsets[2] != 297+209 {
		t.Errorf("offsets = %v, want [0 297 506]", res.Offsets)
	}
}

func TestStreamBatches(t *testing.T) {
	f, err := os.Open(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	var batches, total int
	err = Stream(f, 2, func(b Batch) error {
		batches++
		total += len(b.Records)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if total != 3 {
		t.Errorf("streamed %d records, want 3", total)
	}
	if batches != 2 {
		t.Errorf("got %d batches of size 2 for 3 records, want 2", batches)
	}
}
