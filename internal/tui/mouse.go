package tui

import (
	tea "charm.land/bubbletea/v2"
)

// wheelStep is how far one notch scrolls the record pane. Three lines is the
// usual terminal convention and keeps context on screen.
const wheelStep = 3

// handleMouse routes a mouse event to the pane it happened over. Clicks move
// focus and, in the list, the selection; the wheel scrolls whatever is under
// the pointer without stealing focus, which is what makes it usable for a
// glance at the other pane.
func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	mouse := msg.Mouse()
	which, row, ok := m.paneAt(mouse.X, mouse.Y)
	if !ok {
		return nil
	}

	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return nil
		}
		m.focus = which
		if which == paneList {
			m.selectRow(row)
		}
		return nil

	case tea.MouseWheelMsg:
		switch mouse.Button {
		case tea.MouseWheelUp:
			m.scroll(which, -1)
		case tea.MouseWheelDown:
			m.scroll(which, 1)
		}
		return nil
	}
	return nil
}

// paneAt maps terminal coordinates to a pane and, for the list, to the row
// within it. The layout is one title row, then the panes with their borders,
// then the footer.
func (m *Model) paneAt(x, y int) (which pane, row int, ok bool) {
	const titleRows = 1
	if y < titleRows || y >= titleRows+m.bodyHeight {
		return 0, 0, false
	}
	// The pane's own top border takes the first row of the body.
	row = y - titleRows - 1
	if row < 0 || row >= m.bodyHeight-2 {
		return 0, 0, false
	}

	if m.singlePane {
		return m.focus, row, true
	}
	if x < m.list.Width()+2 {
		return paneList, row, true
	}
	return paneDetail, row, true
}

// selectRow moves the cursor to the row-th visible item. The list paginates,
// so the row is an offset from the first item on the current page.
func (m *Model) selectRow(row int) {
	first := m.list.Index() - m.list.Cursor()
	target := first + row
	if target < 0 || target >= len(m.list.Items()) {
		return
	}
	if target == m.list.Index() {
		return
	}
	m.list.Select(target)
	m.refreshDetail()
}

// scroll moves a pane by one wheel notch: delta is -1 for up, 1 for down.
func (m *Model) scroll(which pane, delta int) {
	if which == paneDetail {
		if delta < 0 {
			m.vp.ScrollUp(wheelStep)
		} else {
			m.vp.ScrollDown(wheelStep)
		}
		return
	}

	before := m.list.Index()
	if delta < 0 {
		m.list.CursorUp()
	} else {
		m.list.CursorDown()
	}
	if m.list.Index() != before {
		m.refreshDetail()
	}
}
