package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func press(m *Model, s string) {
	m.Update(tea.KeyPressMsg{Code: rune(s[0]), Text: s})
}

// n and N follow the focused pane: through matches within a record on the
// right, and from matching record to matching record on the left.
func TestNextMatchIsFocusAware(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.setFilter("all:e") // matches every record in the fixture

	if len(m.list.Items()) < 3 {
		t.Fatalf("filter kept %d records, want all 3", len(m.list.Items()))
	}

	m.focus = paneList
	m.list.Select(0)
	press(m, "n")
	if got := m.list.Index(); got != 1 {
		t.Errorf("n in the list moved to index %d, want 1", got)
	}
	press(m, "N")
	if got := m.list.Index(); got != 0 {
		t.Errorf("N in the list moved to index %d, want 0", got)
	}

	// From the first record, N wraps to the last.
	press(m, "N")
	if got, want := m.list.Index(), len(m.list.Items())-1; got != want {
		t.Errorf("N wrapped to %d, want %d", got, want)
	}

	// With the record focused, n steps within the record instead.
	m.list.Select(0)
	m.focus = paneDetail
	before := m.list.Index()
	if m.matchCount() < 2 {
		t.Fatalf("record has %d matching lines, want at least 2", m.matchCount())
	}
	press(m, "n")
	if m.list.Index() != before {
		t.Error("n in the record pane moved the list cursor")
	}
	if m.matchIndex() != 1 {
		t.Errorf("n in the record pane left the match index at %d, want 1", m.matchIndex())
	}
}

// Without a search term there is nothing to step to.
func TestNextMatchWithoutQuery(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.focus = paneList
	m.list.Select(1)

	press(m, "n")
	if m.list.Index() != 1 {
		t.Error("n moved the cursor with no search term active")
	}
}

// The list pane reports where the cursor is in the file.
func TestListHeaderShowsPosition(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.list.Select(0)
	first := stripANSI(m.listPane())
	if !strings.Contains(first, "1/3") {
		t.Errorf("list header does not show the position:\n%s", first)
	}
	if !strings.Contains(first, "33%") {
		t.Errorf("list header does not show progress:\n%s", first)
	}

	m.list.Select(2)
	last := stripANSI(m.listPane())
	if !strings.Contains(last, "3/3") || !strings.Contains(last, "100%") {
		t.Errorf("list header did not follow the cursor:\n%s", last)
	}
}

// When a filter is active the header says how much of the file is showing.
func TestListHeaderWhenFiltered(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.setFilter("transmission")

	header := stripANSI(m.listPane())
	if !strings.Contains(header, "1/1") {
		t.Errorf("filtered header does not show the filtered position:\n%s", header)
	}
	if !strings.Contains(header, "of 3") {
		t.Errorf("filtered header does not say how many records the file holds:\n%s", header)
	}
}
