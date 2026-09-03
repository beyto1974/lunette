package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/lunette/internal/marcio"
)

// growing writes the first n bytes of the fixture to a temporary file.
func growing(t *testing.T, n int) (path string, whole []byte) {
	t.Helper()
	whole, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path = filepath.ToSlash(filepath.Join(t.TempDir(), "harvest.mrc"))
	if err := os.WriteFile(path, whole[:n], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path, whole
}

// Following a harvest still being written picks up records as they land.
func TestFollowPicksUpNewRecords(t *testing.T) {
	path, whole := growing(t, 400) // one whole record and half of the next

	m, err := New([]string{path}, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	drainLoad(t, m)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if m.count() != 1 {
		t.Fatalf("loaded %d records, want the 1 complete one", m.count())
	}
	if len(m.issues) != 0 {
		t.Errorf("the half-written record was reported as damage: %v", m.issues)
	}
	if !strings.Contains(stripANSI(m.titleBar()), "following") {
		t.Errorf("title bar does not say it is following:\n%s", stripANSI(m.titleBar()))
	}

	// The harvest writes the rest.
	if err := os.WriteFile(path, whole, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	m.Update(m.pollFile()())

	if m.count() != 3 {
		t.Errorf("after the file grew there are %d records, want 3", m.count())
	}
	if got := len(m.list.Items()); got != 3 {
		t.Errorf("the list shows %d records, want 3", got)
	}
}

// Records arriving while following have to be fetched back from the file as it
// is now. A writer that replaces the file by renaming over it leaves the old
// descriptor on an unlinked inode, and a record read through it would be
// whatever used to be at that offset.
func TestFollowedRecordsComeFromTheCurrentFile(t *testing.T) {
	path, whole := growing(t, 400)

	m, err := New([]string{path}, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	drainLoad(t, m)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Read the first record, so anything the browser might hold on to the
	// original file, it is holding.
	if _, err := m.record(0); err != nil {
		t.Fatalf("record 0: %v", err)
	}

	// The writer replaces the file rather than appending to it.
	replacement := filepath.Join(filepath.Dir(path), "replacement.mrc")
	if err := os.WriteFile(replacement, whole, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	m.Update(m.pollFile()())

	if m.count() != 3 {
		t.Fatalf("after the file was replaced there are %d records, want 3", m.count())
	}
	for i := 0; i < m.count(); i++ {
		rec, err := m.record(i)
		if err != nil {
			t.Fatalf("record %d: %v", i+1, err)
		}
		if got := marcio.Title(rec); got != m.entries[i].title {
			t.Errorf("record %d reads back as %q, but its row says %q", i+1, got, m.entries[i].title)
		}
	}
}

// A poll that finds nothing new must change nothing.
func TestFollowIdlePoll(t *testing.T) {
	path, _ := growing(t, 400)

	m, err := New([]string{path}, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	drainLoad(t, m)

	before := m.count()
	for i := 0; i < 3; i++ {
		m.Update(m.pollFile()())
	}
	if m.count() != before {
		t.Errorf("idle polling changed the record count from %d to %d", before, m.count())
	}
	if len(m.issues) != 0 {
		t.Errorf("idle polling produced issues: %v", m.issues)
	}
}

// Without the option there is no polling at all.
func TestFollowIsOptIn(t *testing.T) {
	m := newLoaded(t)
	if m.following {
		t.Error("the browser is following without being asked to")
	}
	if strings.Contains(stripANSI(m.titleBar()), "following") {
		t.Error("title bar claims to be following")
	}
}

// A file replaced rather than appended to is reported, not silently ignored.
func TestFollowDetectsTruncation(t *testing.T) {
	path, whole := growing(t, len(mustRead(t)))
	_ = whole

	m, err := New([]string{path}, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	drainLoad(t, m)

	if err := os.WriteFile(path, []byte("00297"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	m.Update(m.pollFile()())

	if m.status == "" || !strings.Contains(m.status, "shrank") {
		t.Errorf("status = %q, want it to report the file shrinking", m.status)
	}
}

func mustRead(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return b
}

// drainLoad runs the initial background load to completion.
func drainLoad(t *testing.T, m *Model) {
	t.Helper()
	for {
		msg := <-m.ch
		m.Update(msg)
		if msg.done {
			if msg.err != nil {
				t.Fatalf("load: %v", msg.err)
			}
			return
		}
	}
}
