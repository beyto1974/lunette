package marcio

import (
	"os"
	"path/filepath"
	"testing"
)

// split writes the fixture as two files, cut at a record boundary, the way a
// harvest arrives: one file per request window.
func split(t *testing.T) (first, second string) {
	t.Helper()
	data := fixture(t, "sample.mrc")
	dir := t.TempDir()

	// Record 1 is 297 bytes; the rest is records 2 and 3.
	first = filepath.Join(dir, "part-1.mrc")
	second = filepath.Join(dir, "part-2.mrc")
	if err := os.WriteFile(first, data[:297], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(second, data[297:], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return first, second
}

func TestLoadFiles(t *testing.T) {
	first, second := split(t)

	res, err := LoadFiles([]string{first, second})
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	if len(res.Records) != 3 {
		t.Fatalf("loaded %d records, want 3", len(res.Records))
	}
	if res.Format != FormatBinary {
		t.Errorf("Format = %v, want MARC21", res.Format)
	}
	if got := ControlNumber(res.Records[0]); got != "rec-0001" {
		t.Errorf("first record is %q, want rec-0001", got)
	}
	if got := ControlNumber(res.Records[2]); got != "rec-0003" {
		t.Errorf("last record is %q, want rec-0003", got)
	}
}

// One file is the same as calling Load.
func TestLoadFilesSingle(t *testing.T) {
	res, err := LoadFiles([]string{testdata(t, "sample.mrc")})
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	if len(res.Records) != 3 {
		t.Errorf("loaded %d records, want 3", len(res.Records))
	}
}

// Formats may be mixed: a harvest kept as MARCXML alongside a converted file.
func TestLoadFilesMixedFormats(t *testing.T) {
	res, err := LoadFiles([]string{testdata(t, "sample.mrc"), testdata(t, "sample.marcxml")})
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	if len(res.Records) != 6 {
		t.Errorf("loaded %d records, want 6", len(res.Records))
	}
	if !res.MixedFormats {
		t.Error("MixedFormats should be set when the inputs disagree")
	}
}

// Record numbering has to keep running across files, or an issue reported
// against "record 2" is ambiguous.
func TestLoadFilesContinuesOrdinals(t *testing.T) {
	first, second := split(t)

	// Damage the second record inside the second file. Corrupting its first
	// five bytes instead would break format detection and lose the whole file,
	// which is a different failure.
	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	copy(data[209:214], "XXXXX")
	if err := os.WriteFile(second, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	res, err := LoadFiles([]string{first, second})
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(res.Issues), res.Issues)
	}
	// One record came from the first file and one good record precedes the
	// damaged one here, so it is the third record overall.
	if res.Issues[0].Ordinal != 3 {
		t.Errorf("issue ordinal = %d, want 3", res.Issues[0].Ordinal)
	}
	if res.Issues[0].Source != second {
		t.Errorf("issue source = %q, want %q", res.Issues[0].Source, second)
	}
}

// A file that cannot be opened names itself.
func TestLoadFilesMissing(t *testing.T) {
	_, err := LoadFiles([]string{testdata(t, "sample.mrc"), "no-such-file.mrc"})
	if err == nil {
		t.Fatal("LoadFiles accepted a missing file")
	}
	if got := err.Error(); !contains(got, "no-such-file.mrc") {
		t.Errorf("error = %q, want it to name the file", got)
	}
}

func TestLoadFilesEmpty(t *testing.T) {
	if _, err := LoadFiles(nil); err == nil {
		t.Error("LoadFiles accepted no files at all")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
