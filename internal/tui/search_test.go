package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/marcview/internal/marcio"
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

// The filter scope decides which pane the search reads: the list titles, the
// record body, or either.
func TestFilterScopes(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	tests := []struct {
		expr string
		want int
	}{
		// "privacy" is a 650 subject, absent from the list key.
		{"privacy", 0},
		{"rec:privacy", 1},
		{"all:privacy", 1},
		// "2002" is in the list key (year) but not in any subfield of record 1
		// other than 260 $c, which the record scope does see.
		{"kloza", 1},
		{"rec:kloza", 1},
		{"tag:650 rec:privacy", 1},
		{"tag:856 rec:privacy", 0},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			m.setFilter(tt.expr)
			if got := len(m.list.Items()); got != tt.want {
				t.Errorf("%q kept %d records, want %d", tt.expr, got, tt.want)
			}
		})
	}
}

// s cycles the scope and re-applies the current filter.
func TestCycleScope(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.setFilter("privacy")

	if len(m.list.Items()) != 0 {
		t.Fatalf("titles scope kept %d records, want 0", len(m.list.Items()))
	}

	press(m, "s") // titles -> record
	if m.filter.scope != marcio.ScopeRecord {
		t.Fatalf("scope after one press = %v, want record", m.filter.scope)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("record scope kept %d records, want 1", got)
	}
	if !strings.Contains(stripANSI(m.footer()), "record") {
		t.Errorf("the status line does not name the scope:\n%s", stripANSI(m.footer()))
	}

	press(m, "s") // record -> both
	if m.filter.scope != marcio.ScopeBoth {
		t.Errorf("scope after two presses = %v, want both", m.filter.scope)
	}
	press(m, "s") // both -> titles
	if m.filter.scope != marcio.ScopeTitles {
		t.Errorf("scope wrapped to %v, want titles", m.filter.scope)
	}
}

// Reopening the prompt shows the expression that produced the current result.
func TestFilterExpressionKeepsScope(t *testing.T) {
	m := newLoaded(t)
	for _, expr := range []string{"", "brussels", "rec:brussels", "all:brussels", "tag:856 rec:brussels"} {
		m.setFilter(expr)
		if got := m.filterExpression(); got != expr {
			t.Errorf("filterExpression() = %q, want %q", got, expr)
		}
	}
}
