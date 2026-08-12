package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// loadedEscapes opens the terminal-injection fixture in a sized browser.
func loadedEscapes(t *testing.T) *Model {
	t.Helper()
	m, err := New([]string{fixture("escapes.mrc")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	drainLoad(t, m)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

// liveEscapes reports control characters that a record put on the screen. The
// browser writes plenty of escapes of its own, so only the sequences from the
// fixture are looked for.
func liveEscapes(s string) []string {
	var found []string
	for _, seq := range []struct{ name, text string }{
		{"screen clear", "\x1b[2J"},
		{"clipboard write", "\x1b]52;"},
		{"bell", "\x07"},
	} {
		if strings.Contains(s, seq.text) {
			found = append(found, seq.name)
		}
	}
	return found
}

// Nothing the record carries may act on the terminal, anywhere in the frame.
func TestRecordCannotDriveTheTerminal(t *testing.T) {
	m := loadedEscapes(t)

	if got := liveEscapes(m.View().Content); got != nil {
		t.Errorf("the frame carries the record's %v", got)
	}
	if got := liveEscapes(m.vp.GetContent()); got != nil {
		t.Errorf("the record pane carries %v", got)
	}
	if got := liveEscapes(m.list.View()); got != nil {
		t.Errorf("the list carries %v", got)
	}
	if got := liveEscapes(stripANSI(m.detailPane())); got != nil {
		t.Errorf("the detail header carries %v", got)
	}

	// The text is defused, not dropped.
	if !strings.Contains(stripANSI(m.vp.GetContent()), "Harmless Title^[") {
		t.Errorf("the sequence is not shown in caret notation:\n%s", stripANSI(m.vp.GetContent()))
	}
}

// Copying must not hand the sequences to the clipboard either, where they
// would act on whatever the text is later pasted into.
func TestCopyIsDefused(t *testing.T) {
	m := loadedEscapes(t)

	m.focus = paneList
	copied, what, ok := m.clipboardPayload()
	if !ok {
		t.Fatal("nothing to copy")
	}
	if !strings.Contains(what, "record") {
		t.Errorf("copy described as %q, want the whole record", what)
	}
	if got := liveEscapes(copied); got != nil {
		t.Errorf("the clipboard would receive %v: %q", got, copied)
	}

	// The same for a single field.
	m.focus = paneDetail
	m.moveField(1)
	copied, what, ok = m.clipboardPayload()
	if !ok {
		t.Fatal("nothing to copy from the record pane")
	}
	if !strings.Contains(what, "field") {
		t.Errorf("copy described as %q, want a field", what)
	}
	if got := liveEscapes(copied); got != nil {
		t.Errorf("copying a field would send %v: %q", got, copied)
	}
}

// A URL is handed to another program, so it must not carry control characters
// however it reached the record.
func TestOpenURLRejectsControlCharacters(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	var opened []string
	m.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	if err := openable("https://example.org/ok"); err != nil {
		t.Errorf("a plain URL was refused: %v", err)
	}
	for _, bad := range []string{
		"https://example.org/\x1b[2J",
		"https://example.org/\nrm -rf",
		"https://example.org/\x00",
	} {
		if err := openable(bad); err == nil {
			t.Errorf("accepted a URL carrying a control character: %q", bad)
		}
	}
	if len(opened) != 0 {
		t.Errorf("opened %v without being asked", opened)
	}
}
