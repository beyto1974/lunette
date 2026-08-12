package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// typeIn sends each character of s as its own key press, the way a user types.
func typeIn(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func enter(m *Model)  { m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) }
func escape(m *Model) { m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) }

// The filter prompt is the one interactive path the other tests reach past by
// calling setFilter directly: they would not notice if typing stopped working.
func TestFilterPrompt(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	press(m, "/")
	if m.inputMode != inputFilter {
		t.Fatal("/ did not open the filter prompt")
	}
	if !strings.Contains(stripANSI(m.footer()), "filter") {
		t.Errorf("the prompt is not showing:\n%s", stripANSI(m.footer()))
	}

	typeIn(m, "transmission")
	if got := m.input.Value(); got != "transmission" {
		t.Fatalf("the prompt holds %q, want the typed text", got)
	}
	// Nothing is filtered until the prompt is submitted.
	if len(m.list.Items()) != 3 {
		t.Errorf("the list narrowed while still typing: %d items", len(m.list.Items()))
	}

	enter(m)
	if m.inputMode != inputNone {
		t.Error("enter did not close the prompt")
	}
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("after enter the list has %d records, want 1", got)
	}
	if m.filter.query != "transmission" {
		t.Errorf("filter query = %q", m.filter.query)
	}
}

// esc abandons what was typed and leaves the previous result alone.
func TestFilterPromptCancelled(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	press(m, "/")
	typeIn(m, "transmission")
	escape(m)

	if m.inputMode != inputNone {
		t.Error("esc did not close the prompt")
	}
	if !m.filter.empty() {
		t.Errorf("esc applied the filter anyway: %+v", m.filter)
	}
	if got := len(m.list.Items()); got != 3 {
		t.Errorf("the list has %d records, want all 3", got)
	}
}

// Reopening the prompt starts from the filter in force, so it can be edited
// rather than retyped.
func TestFilterPromptReopensWithCurrentFilter(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.setFilter("tag:856 rec:brussels")
	press(m, "/")
	if got := m.input.Value(); got != "tag:856 rec:brussels" {
		t.Errorf("the prompt opened with %q, want the active filter", got)
	}
}

// The prompt swallows keys that are shortcuts outside it: typing "q" into a
// filter must not quit.
func TestPromptSwallowsShortcuts(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	press(m, "/")
	typeIn(m, "qcz")
	if got := m.input.Value(); got != "qcz" {
		t.Errorf("the prompt holds %q; a shortcut fired instead of typing", got)
	}
	if m.zoom {
		t.Error("z zoomed while typing a filter")
	}
}

// The jump prompt takes a record number, and says so when it is given
// something else.
func TestJumpPrompt(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	press(m, ":")
	if m.inputMode != inputJump {
		t.Fatal(": did not open the jump prompt")
	}
	typeIn(m, "3")
	enter(m)

	if _, it, ok := m.current(); !ok || it.ordinal != 3 {
		t.Errorf("jumped to %+v, want record 3", it)
	}

	press(m, ":")
	typeIn(m, "nonsense")
	enter(m)
	if !strings.Contains(m.status, "not a record number") {
		t.Errorf("status = %q, want it to reject the input", m.status)
	}
}
