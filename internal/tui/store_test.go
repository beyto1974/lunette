package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The browser keeps a list row and an extent per record, not the record. A
// harvest of 1.1 million records is several gigabytes decoded and a few
// hundred megabytes as rows; the record itself is fetched when the cursor
// lands on it, which is a seek and a couple of kilobytes.
func TestRecordsAreFetchedOnDemand(t *testing.T) {
	m := newLoaded(t)

	if m.rereads == 0 {
		t.Error("nothing was fetched; the detail pane should have asked for the selected record")
	}
	before := m.rereads

	m.list.Select(1)
	m.refreshDetail()
	if m.rereads <= before {
		t.Error("moving the cursor did not fetch the record it landed on")
	}
	if !strings.Contains(stripANSI(m.vp.GetContent()), "café-cultuur") {
		t.Errorf("the second record did not render:\n%s", stripANSI(m.vp.GetContent()))
	}
}

// Re-rendering the same record - a mode switch, a resize - must not go back to
// the file every time.
func TestTheSelectedRecordIsCached(t *testing.T) {
	m := newLoaded(t)
	before := m.rereads
	m.redrawDetail()
	m.redrawDetail()
	if m.rereads != before {
		t.Errorf("redrawing fetched the record again: %d fetches, want %d", m.rereads, before)
	}
}

// Filtering by tag must not read a record: it is answered from what was kept
// at load time, or a filter would re-read the whole file.
func TestTagFilterReadsNothing(t *testing.T) {
	m := newLoaded(t)
	before := m.rereads

	m.setFilter("tag:856")
	if n := len(m.list.Items()); n != 1 {
		t.Errorf("tag:856 kept %d records, want 1", n)
	}
	// One fetch is the newly selected record the detail pane draws.
	if m.rereads > before+1 {
		t.Errorf("filtering by tag fetched %d records", m.rereads-before)
	}
}

// The "all:" scope searches every subfield, which does mean reading the
// records again. It has to still work.
func TestFullTextFilterFetchesRecords(t *testing.T) {
	m := newLoaded(t)
	m.setFilter("all:privacy")
	if n := len(m.list.Items()); n != 1 {
		t.Errorf("all:privacy kept %d records, want 1", n)
	}
}

// A file whose records cannot be read back says so in the pane rather than
// showing an empty one.
func TestUnreadableRecordExplainsItself(t *testing.T) {
	m := newLoaded(t)
	m.closeFiles()
	// A reader over a closed file fails; the model must not pretend otherwise.
	m.readers = nil
	m.paths = []string{"no-such-file.mrc"}
	m.cached, m.cachedIdx = nil, -1

	m.redrawDetail()
	if got := stripANSI(m.vp.GetContent()); !strings.Contains(got, "could not") {
		t.Errorf("pane = %q, want it to explain the failure", got)
	}
}

// Several files are read as one set, so a record has to remember which file it
// came from or it would be fetched from the wrong one.
func TestRecordsRememberTheirFile(t *testing.T) {
	m, err := New([]string{fixture("sample.mrc"), fixture("sample.marcxml")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(t, m)

	if len(m.entries) != 6 {
		t.Fatalf("loaded %d records, want 6", len(m.entries))
	}
	if m.entries[0].file != 0 || m.entries[5].file != 1 {
		t.Errorf("records came from files %d and %d, want 0 and 1", m.entries[0].file, m.entries[5].file)
	}
	for i := range m.entries {
		if _, err := m.record(i); err != nil {
			t.Errorf("record %d: %v", i+1, err)
		}
	}
}

// A set of files need not all be the same format, and a record has to be read
// back the way it was read in. Deciding that from whatever the last file said
// would mean a binary reader turned loose on MARCXML.
func TestMixedFormatsAreReadBackPerFile(t *testing.T) {
	paths := []string{fixture("sample.mrc"), fixture("sample.marcxml"), fixture("sample.mrc")}
	m, err := New(paths)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(t, m)

	if len(m.entries) != 9 {
		t.Fatalf("loaded %d records, want 9", len(m.entries))
	}
	for i := range m.entries {
		rec, err := m.record(i)
		if err != nil {
			t.Fatalf("record %d (file %d): %v", i+1, m.entries[i].file, err)
		}
		// Every fixture record has a title; a record read with the wrong
		// reader would not decode at all.
		if got := m.entries[i].title; got == "" {
			t.Errorf("record %d has no title", i+1)
		} else if rec == nil {
			t.Errorf("record %d came back empty", i+1)
		}
	}
}
