package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// z collapses the browser to one pane on any terminal, for people who would
// rather read one thing at a time.
func TestZoomShowsOnePane(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	both := m.View().Content
	if strings.Count(strings.Split(both, "\n")[1], "╭") != 2 {
		t.Fatalf("expected two panes before zooming:\n%s", both)
	}

	press(m, "z")
	one := m.View().Content
	if strings.Count(strings.Split(one, "\n")[1], "╭") != 1 {
		t.Errorf("expected a single pane after zooming:\n%s", one)
	}
	if !strings.Contains(stripANSI(one), "00001") {
		t.Error("the list is not the pane on show")
	}

	press(m, "z")
	// The status line reports the toggle, so compare the panes, not the frame.
	if got := strings.Count(strings.Split(m.View().Content, "\n")[1], "╭"); got != 2 {
		t.Errorf("z did not restore the two-pane layout, found %d panes", got)
	}
	if m.zoom {
		t.Error("zoom flag is still set after toggling twice")
	}
}

// In one-pane mode enter opens the record and esc goes back to the list.
func TestZoomEnterAndBack(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "z")

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.focus != paneDetail {
		t.Fatal("enter did not open the record")
	}
	if !strings.Contains(stripANSI(m.View().Content), "LEADER") {
		t.Error("the record pane is not the one on show")
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.focus != paneList {
		t.Error("esc did not go back to the list")
	}
}

// esc keeps clearing the filter when there is no record pane to leave.
func TestEscStillClearsTheFilter(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.setFilter("transmission")

	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.filter.empty() {
		t.Error("esc did not clear the filter in the two-pane layout")
	}

	// In one pane, esc leaves the record first, then clears the filter.
	m.setFilter("transmission")
	press(m, "z")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.filter.empty() {
		t.Error("esc cleared the filter instead of leaving the record")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.filter.empty() {
		t.Error("a second esc did not clear the filter")
	}
}
