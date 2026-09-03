package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// bigFixture writes the sample repeated enough times to arrive in many
// batches, which is the only way the cost of loading shows up at all.
func bigFixture(t *testing.T, times int) string {
	t.Helper()
	one, err := os.ReadFile(fixture("sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	path := filepath.Join(t.TempDir(), "big.mrc")
	// Binary MARC21 concatenates, so a long file is the fixture end to end.
	if err := os.WriteFile(path, []byte(strings.Repeat(string(one), times)), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// drain runs the loader to completion, as the program's event loop would.
func drain(t *testing.T, m *Model) {
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

// Loading must build each row once. The list used to be rebuilt from scratch
// on every batch, which is quadratic in the number of records: the 3.7 GB
// harvest this is meant to browse is about 1.1 million of them, arriving in
// four thousand batches, so the browser built billions of rows before it
// finished loading and appeared to hang.
func TestLoadBuildsEachRowOnce(t *testing.T) {
	const times = 200 // 600 records, several batches

	m, err := New([]string{bigFixture(t, times)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(t, m)

	want := times * 3
	if len(m.records) != want {
		t.Fatalf("loaded %d records, want %d", len(m.records), want)
	}
	if got := len(m.list.Items()); got != want {
		t.Errorf("list has %d items, want %d", got, want)
	}
	if m.rowsBuilt != want {
		t.Errorf("built %d rows for %d records; loading must build each row once", m.rowsBuilt, want)
	}
}

// A filter still rebuilds, because which records are visible has changed. That
// is one pass over the records, not one per batch.
func TestFilterRebuildsOnce(t *testing.T) {
	m, err := New([]string{bigFixture(t, 200)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(t, m)

	before := m.rowsBuilt
	m.setFilter("vandaele")
	built := m.rowsBuilt - before
	if built != len(m.list.Items()) {
		t.Errorf("filtering built %d rows for %d visible records", built, len(m.list.Items()))
	}
	if len(m.list.Items()) != 200 {
		t.Errorf("filter kept %d records, want 200", len(m.list.Items()))
	}
}
