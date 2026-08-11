package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/lunette/internal/marcio"
)

// f filters by the selected field, which is the question "what else looks like
// this" asked of a subject, an author or a year.
func TestFilterBySelectedField(t *testing.T) {
	m := focused(t)
	m.list.Select(1) // the record carrying 650 $a Privacy
	m.refreshDetail()

	// Walk the cursor to the 650.
	for i := 0; i < m.fieldCount(); i++ {
		if m.fields[m.fieldCursor].Tag == "650" {
			break
		}
		m.moveField(1)
	}
	if m.fields[m.fieldCursor].Tag != "650" {
		t.Fatalf("cursor is on %s, want 650", m.fields[m.fieldCursor].Tag)
	}

	press(m, "f")

	if m.filter.tag != "650" {
		t.Errorf("filter tag = %q, want 650", m.filter.tag)
	}
	if m.filter.query != "privacy" {
		t.Errorf("filter query = %q, want privacy", m.filter.query)
	}
	if m.filter.scope != marcio.ScopeRecord {
		t.Errorf("filter scope = %v, want record", m.filter.scope)
	}
	if got := len(m.list.Items()); got != 1 {
		t.Errorf("filtering by the field kept %d records, want 1", got)
	}
	if !strings.Contains(stripANSI(m.footer()), "privacy") {
		t.Errorf("status does not show the new filter:\n%s", stripANSI(m.footer()))
	}
}

// A control field has no subfields; filtering by one uses its data.
func TestFilterByControlField(t *testing.T) {
	m := focused(t)
	for i := 0; i < m.fieldCount(); i++ {
		if m.fields[m.fieldCursor].Tag == "001" {
			break
		}
		m.moveField(1)
	}

	press(m, "f")
	if m.filter.query != "rec-0001" {
		t.Errorf("filter query = %q, want rec-0001", m.filter.query)
	}
}

// With the list focused there is no selected field, so f does nothing.
func TestFilterByFieldNeedsTheRecordPane(t *testing.T) {
	m := focused(t)
	m.focus = paneList

	press(m, "f")
	if !m.filter.empty() {
		t.Errorf("f filtered from the list pane: %+v", m.filter)
	}
}

// o opens the electronic location of the selected field, or of the record.
func TestOpenURL(t *testing.T) {
	m := focused(t)
	var opened []string
	m.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	// The first record's 856 carries the URL; the cursor starts elsewhere, so
	// o falls back to the record's own 856.
	press(m, "o")
	if len(opened) != 1 {
		t.Fatalf("opened %v, want one URL", opened)
	}
	if !strings.HasPrefix(opened[0], "https://biblio.vub.ac.be/vubir/") {
		t.Errorf("opened %q, want the 856 $u", opened[0])
	}
	if !strings.Contains(m.status, "opening") {
		t.Errorf("status = %q, want it to mention opening", m.status)
	}
}

// A record with no electronic location says so instead of doing nothing.
func TestOpenURLWithoutOne(t *testing.T) {
	m := focused(t)
	m.list.Select(1) // no 856 on this record
	m.refreshDetail()

	var opened []string
	m.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	press(m, "o")
	if len(opened) != 0 {
		t.Errorf("opened %v, want nothing", opened)
	}
	if !strings.Contains(m.status, "no URL") {
		t.Errorf("status = %q, want it to say there is no URL", m.status)
	}
}

// A failing opener is reported rather than swallowed.
func TestOpenURLFailure(t *testing.T) {
	m := focused(t)
	m.openURL = func(string) error { return errFake }

	press(m, "o")
	if !strings.Contains(m.status, "could not open") {
		t.Errorf("status = %q, want the failure reported", m.status)
	}
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "no browser here" }

// The keys must be reachable from the help.
func TestNewKeysAreDocumented(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.help.ShowAll = true
	m.layout()

	help := stripANSI(m.helpView)
	for _, want := range []string{"filter by field", "open url", "one pane"} {
		if !strings.Contains(help, want) {
			t.Errorf("full help does not mention %q:\n%s", want, help)
		}
	}
}
