package marcio

import (
	"os"
	"path/filepath"
	"testing"
)

// bigFile writes n copies of the fixture, so a test can act on something much
// larger than a read buffer without checking in a large file.
func bigFile(t *testing.T, copies int) (path string, size int64) {
	t.Helper()
	one := fixture(t, "sample.mrc")
	path = filepath.Join(t.TempDir(), "big.mrc")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()
	for i := 0; i < copies; i++ {
		if _, err := f.Write(one); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	return path, info.Size()
}

// CompletePrefixFile must not read the file it measures: it exists to tell a
// follower where the whole records end, and a harvest can be gigabytes.
func TestCompletePrefixFileDoesNotReadEverything(t *testing.T) {
	path, size := bigFile(t, 400) // ~265 KB of whole records

	got, err := CompletePrefixFile(path)
	if err != nil {
		t.Fatalf("CompletePrefixFile: %v", err)
	}
	if got != size {
		t.Errorf("CompletePrefixFile = %d, want %d", got, size)
	}

	// Append half a record; the answer must not move.
	partial := fixture(t, "sample.mrc")[:100]
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.Write(partial); err != nil {
		t.Fatalf("Write: %v", err)
	}
	f.Close()

	got, err = CompletePrefixFile(path)
	if err != nil {
		t.Fatalf("CompletePrefixFile: %v", err)
	}
	if got != size {
		t.Errorf("a half-written record moved the boundary to %d, want %d", got, size)
	}
}

// A single read is capped, so a file that grows without bound does not become
// one allocation without bound. The rest is picked up by the read after it.
func TestLoadFromCapsOneRead(t *testing.T) {
	path, size := bigFile(t, 400)

	res, next, err := loadFromLimit(path, 0, 4096)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if next >= size {
		t.Fatalf("one read consumed %d of %d bytes; the cap did nothing", next, size)
	}
	if len(res.Records) == 0 {
		t.Fatal("a capped read returned no records")
	}

	// Reading on from there eventually reaches the end.
	offset, rounds := next, 0
	for offset < size {
		res, offset, err = loadFromLimit(path, offset, 4096)
		if err != nil {
			t.Fatalf("LoadFrom at %d: %v", offset, err)
		}
		if len(res.Records) == 0 {
			t.Fatalf("no progress at offset %d", offset)
		}
		if rounds++; rounds > 1000 {
			t.Fatal("reading did not converge")
		}
	}
	if offset != size {
		t.Errorf("finished at %d, want %d", offset, size)
	}
}

// The whole file is still read correctly when the cap is not in the way.
func TestLoadFromWholeFile(t *testing.T) {
	path, size := bigFile(t, 3)

	res, next, err := LoadFrom(path, 0)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if next != size {
		t.Errorf("read to %d, want %d", next, size)
	}
	if len(res.Records) != 9 {
		t.Errorf("got %d records, want 9", len(res.Records))
	}
}
