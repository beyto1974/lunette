package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/lunette/internal/render"
)

func focused(t *testing.T) *Model {
	t.Helper()
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.focus = paneDetail
	return m
}

// With the record focused, the cursor moves field by field rather than line by
// line, and the selected field is marked.
func TestFieldCursorMoves(t *testing.T) {
	m := focused(t)

	if m.fieldCount() < 3 {
		t.Fatalf("record has %d fields, want at least 3", m.fieldCount())
	}
	if m.fieldCursor != 0 {
		t.Errorf("cursor starts at %d, want 0", m.fieldCursor)
	}

	press(m, "j")
	if m.fieldCursor != 1 {
		t.Errorf("j moved the cursor to %d, want 1", m.fieldCursor)
	}
	press(m, "k")
	if m.fieldCursor != 0 {
		t.Errorf("k moved the cursor to %d, want 0", m.fieldCursor)
	}

	// A cursor clamps at the ends rather than wrapping.
	press(m, "k")
	if m.fieldCursor != 0 {
		t.Errorf("k at the top moved to %d, want 0", m.fieldCursor)
	}
	for i := 0; i < m.fieldCount()+3; i++ {
		press(m, "j")
	}
	if want := m.fieldCount() - 1; m.fieldCursor != want {
		t.Errorf("cursor ran to %d, want it clamped at %d", m.fieldCursor, want)
	}
}

// The selected field is marked in the pane, and the header says which it is.
func TestFieldCursorIsVisible(t *testing.T) {
	m := focused(t)
	first := m.vp.GetContent()
	if !strings.Contains(first, fieldMarker) {
		t.Fatalf("no field marker in the record pane:\n%s", stripANSI(first))
	}

	header := stripANSI(m.detailPane())
	if !strings.Contains(header, "field 1/") {
		t.Errorf("header does not report the field position:\n%s", header)
	}

	press(m, "j")
	if m.vp.GetContent() == first {
		t.Error("moving the field cursor did not redraw the pane")
	}
}

// Moving the list cursor resets the field cursor: it points into a record.
func TestFieldCursorResetsWithRecord(t *testing.T) {
	m := focused(t)
	press(m, "j")
	press(m, "j")
	if m.fieldCursor == 0 {
		t.Fatal("cursor did not move")
	}

	m.focus = paneList
	press(m, "j") // next record
	if m.fieldCursor != 0 {
		t.Errorf("field cursor is %d after changing record, want 0", m.fieldCursor)
	}
}

// A resize re-wraps the record but must not move the user's cursor.
func TestFieldCursorSurvivesResize(t *testing.T) {
	m := focused(t)
	press(m, "j")
	press(m, "j")
	before := m.fieldCursor
	if before != 2 {
		t.Fatalf("cursor is at %d, want 2", before)
	}

	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if m.fieldCursor != before {
		t.Errorf("resize moved the field cursor from %d to %d", before, m.fieldCursor)
	}

	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	if m.fieldCursor != before {
		t.Errorf("widening moved the field cursor from %d to %d", before, m.fieldCursor)
	}
}

// Zooming to one pane re-wraps the record too, and likewise must not move it.
func TestFieldCursorSurvivesZoom(t *testing.T) {
	m := focused(t)
	press(m, "j")
	before := m.fieldCursor

	press(m, "z")
	if m.fieldCursor != before {
		t.Errorf("zoom moved the field cursor from %d to %d", before, m.fieldCursor)
	}
}

// JSON and XML have no fields to point at, so the keys scroll instead.
func TestFieldCursorAbsentInStructuredViews(t *testing.T) {
	m := focused(t)
	m.setMode(render.XML)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 14})

	if m.fieldCount() != 0 {
		t.Fatalf("XML reported %d fields, want none", m.fieldCount())
	}
	if strings.Contains(m.vp.GetContent(), fieldMarker) {
		t.Error("XML view is showing a field marker")
	}

	before := m.vp.YOffset()
	for i := 0; i < 5; i++ {
		press(m, "j")
	}
	if m.vp.YOffset() <= before {
		t.Error("j did not scroll the XML view")
	}
}

// y copies the selected field when the record is focused, the whole record
// otherwise.
func TestCopyFollowsTheFieldCursor(t *testing.T) {
	m := focused(t)
	press(m, "j") // the 001 control field

	m.copyCurrent()
	if !strings.Contains(m.status, "field") {
		t.Errorf("status after copying a field = %q", m.status)
	}

	m.focus = paneList
	m.copyCurrent()
	if !strings.Contains(m.status, "record") {
		t.Errorf("status after copying a record = %q", m.status)
	}
}

func TestSelectedFieldText(t *testing.T) {
	m := focused(t)
	m.fieldCursor = 0

	text, ok := m.selectedFieldText()
	if !ok {
		t.Fatal("no selected field text")
	}
	if !strings.Contains(text, "LEADER") && !strings.Contains(text, "001") {
		t.Errorf("first field text = %q, want the record's first field", text)
	}
	if strings.Contains(text, fieldMarker) {
		t.Error("copied text carries the cursor marker")
	}
}
