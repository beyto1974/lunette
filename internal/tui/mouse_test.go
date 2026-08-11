package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/lunette/internal/render"
)

func sized(t *testing.T) *Model {
	t.Helper()
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func click(m *Model, x, y int) {
	m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func wheel(m *Model, x, y int, up bool) {
	button := tea.MouseWheelDown
	if up {
		button = tea.MouseWheelUp
	}
	m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
}

// Clicking a row selects that record and moves focus to the list.
func TestMouseClickSelectsRow(t *testing.T) {
	m := sized(t)
	m.focus = paneDetail

	// Row 0 of the list sits below the title bar and the pane's top border.
	click(m, 4, listRowY(m, 2))

	if m.focus != paneList {
		t.Error("clicking the list did not move focus to it")
	}
	if _, it, ok := m.current(); !ok || it.ordinal != 3 {
		t.Errorf("clicked row 2, selected %+v, want ordinal 3", it)
	}
	if !strings.Contains(m.vp.GetContent(), "Sans auteur") {
		t.Error("the detail pane did not follow the click")
	}
}

// Clicking past the last record must not move the cursor or panic.
func TestMouseClickBelowLastRow(t *testing.T) {
	m := sized(t)
	m.list.Select(1)
	before := m.list.Index()

	click(m, 4, listRowY(m, 25))

	if m.list.Index() != before {
		t.Errorf("clicking empty space moved the cursor from %d to %d", before, m.list.Index())
	}
}

// Clicking the record pane focuses it without disturbing the selection.
func TestMouseClickFocusesDetail(t *testing.T) {
	m := sized(t)
	before := m.list.Index()

	click(m, m.width-5, 5)

	if m.focus != paneDetail {
		t.Error("clicking the record pane did not focus it")
	}
	if m.list.Index() != before {
		t.Error("clicking the record pane changed the selection")
	}
}

// Clicks on the title bar and the footer are ignored.
func TestMouseClickOutsidePanes(t *testing.T) {
	m := sized(t)
	m.focus = paneList
	before := m.list.Index()

	click(m, 10, 0)          // title bar
	click(m, 10, m.height-1) // help line

	if m.focus != paneList || m.list.Index() != before {
		t.Error("a click outside the panes changed the state")
	}
}

// The wheel scrolls whichever pane it is over, without changing focus.
func TestMouseWheel(t *testing.T) {
	m := sized(t)
	m.focus = paneList

	// XML is the longest rendering; shrink the terminal so it overflows the
	// pane and there is something to scroll.
	m.list.Select(0)
	m.setMode(render.XML)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 14})
	if m.vp.TotalLineCount() <= m.vp.Height() {
		t.Fatalf("record fits the pane (%d lines in %d rows), nothing to scroll",
			m.vp.TotalLineCount(), m.vp.Height())
	}
	beforeOffset, beforeIndex := m.vp.YOffset(), m.list.Index()
	wheel(m, m.width-5, 5, false)
	if m.vp.YOffset() <= beforeOffset {
		t.Errorf("wheel over the record pane did not scroll it (%d -> %d)", beforeOffset, m.vp.YOffset())
	}
	if m.list.Index() != beforeIndex {
		t.Error("wheel over the record pane moved the list cursor")
	}
	if m.focus != paneList {
		t.Error("the wheel changed focus")
	}

	// Over the list: move the cursor.
	wheel(m, 4, listRowY(m, 0), false)
	if m.list.Index() == beforeIndex {
		t.Error("wheel over the list did not move the cursor")
	}
	wheel(m, 4, listRowY(m, 0), true)
	if m.list.Index() != beforeIndex {
		t.Error("wheel up did not undo wheel down")
	}
}

// The published view must ask the terminal for mouse events, or none arrive.
func TestViewEnablesMouse(t *testing.T) {
	m := sized(t)
	if m.View().MouseMode == tea.MouseModeNone {
		t.Error("the view does not enable mouse reporting")
	}
}

// listRowY is the terminal row of the nth record row: title bar, then the
// pane's top border.
func listRowY(m *Model, n int) int { return 1 + 1 + n }
