package marcio

import (
	"os"
	"path/filepath"
	"testing"
)

// A file still being written ends mid-record; only whole records may be read,
// and the rest must wait for the writer to catch up.
func TestCompletePrefix(t *testing.T) {
	data := fixture(t, "sample.mrc")

	if got := CompletePrefix(data); got != len(data) {
		t.Errorf("CompletePrefix of a whole file = %d, want %d", got, len(data))
	}
	// Record 1 is 297 bytes, record 2 is 209.
	if got := CompletePrefix(data[:400]); got != 297 {
		t.Errorf("CompletePrefix mid-record-2 = %d, want 297", got)
	}
	if got := CompletePrefix(data[:297]); got != 297 {
		t.Errorf("CompletePrefix at a boundary = %d, want 297", got)
	}
	if got := CompletePrefix(data[:10]); got != 0 {
		t.Errorf("CompletePrefix of a partial first record = %d, want 0", got)
	}
	if got := CompletePrefix(nil); got != 0 {
		t.Errorf("CompletePrefix of nothing = %d, want 0", got)
	}
	// Garbage cannot be walked, so nothing is complete.
	if got := CompletePrefix([]byte("not a marc file at all")); got != 0 {
		t.Errorf("CompletePrefix of garbage = %d, want 0", got)
	}
}

// LoadFrom reads the records appended since a previous read and reports where
// to resume, which is what makes following a growing harvest possible.
func TestLoadFrom(t *testing.T) {
	data := fixture(t, "sample.mrc")
	path := filepath.Join(t.TempDir(), "growing.mrc")

	// Start with one and a half records, as a harvest in progress would be.
	if err := os.WriteFile(path, data[:400], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	res, offset, err := LoadFrom(path, 0)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("read %d records, want the 1 complete one", len(res.Records))
	}
	if offset != 297 {
		t.Fatalf("resume offset = %d, want 297", offset)
	}
	if len(res.Issues) != 0 {
		t.Errorf("a half-written record was reported as damage: %v", res.Issues)
	}

	// Nothing new yet.
	res, next, err := LoadFrom(path, offset)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(res.Records) != 0 || next != offset {
		t.Errorf("got %d records and offset %d, want none and %d", len(res.Records), next, offset)
	}

	// The writer finishes the file.
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	res, next, err = LoadFrom(path, offset)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(res.Records) != 2 {
		t.Errorf("read %d records after the file completed, want 2", len(res.Records))
	}
	if next != int64(len(data)) {
		t.Errorf("resume offset = %d, want %d", next, len(data))
	}
	if got := Title(res.Records[0]); got == "" {
		t.Error("the record read after resuming has no title")
	}
}

// Following only makes sense for the binary format; a MARCXML document is not
// complete until its closing tag.
func TestLoadFromRejectsXML(t *testing.T) {
	if _, _, err := LoadFrom("../../testdata/sample.marcxml", 0); err == nil {
		t.Error("LoadFrom accepted MARCXML")
	}
}
