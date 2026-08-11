package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	marc "github.com/beyto1974/gomarc"
)

// filterBySelectedField answers "what else looks like this" for the field under
// the cursor: the same tag carrying the same text. The scope is the record
// body, since a subject or a note is not in the list key.
func (m *Model) filterBySelectedField() tea.Cmd {
	field, ok := m.selectedField()
	if !ok {
		m.status = "no field selected - focus the record pane first"
		return nil
	}

	value := fieldText(field)
	if value == "" {
		m.status = "field " + field.Tag + " has no text to filter by"
		return nil
	}

	return m.setFilter(fmt.Sprintf("tag:%s rec:%s", field.Tag, value))
}

// selectedField is the record field the cursor is on. The rendering and the
// record share an order, so the cursor indexes both.
func (m *Model) selectedField() (*marc.Field, bool) {
	rec, _, ok := m.current()
	if !ok || m.focus != paneDetail || m.fieldCursor >= len(m.fields) {
		return nil, false
	}
	if m.fieldCursor >= len(rec.Fields) {
		return nil, false
	}
	return rec.Fields[m.fieldCursor], true
}

// fieldText is what a field is worth searching for: its first subfield value,
// or the data of a control field.
func fieldText(f *marc.Field) string {
	if f.IsControlField() {
		return strings.TrimSpace(f.Data)
	}
	for _, sf := range f.Subfields {
		if v := strings.TrimSpace(sf.Value); v != "" {
			return v
		}
	}
	return ""
}

// openSelectedURL opens the electronic location of the selected field, falling
// back to the record's own 856. Records in an institutional repository are
// mostly links, so this is the difference between reading about a resource and
// reaching it.
func (m *Model) openSelectedURL() tea.Cmd {
	rec, _, ok := m.current()
	if !ok {
		return nil
	}

	url := ""
	if field, ok := m.selectedField(); ok {
		url = fieldURL(field)
	}
	if url == "" {
		for _, f := range rec.GetFields("856") {
			if url = fieldURL(f); url != "" {
				break
			}
		}
	}
	if url == "" {
		m.status = "no URL on this record"
		return nil
	}

	if err := openable(url); err != nil {
		m.status = "will not open this link: " + err.Error()
		return nil
	}

	open := m.openURL
	if open == nil {
		open = openInBrowser
	}
	if err := open(url); err != nil {
		m.status = "could not open " + url + ": " + err.Error()
		return nil
	}
	m.status = "opening " + url
	return nil
}

// fieldURL is the $u of a field, or a $y or $z that happens to hold one.
func fieldURL(f *marc.Field) string {
	if f.IsControlField() {
		return ""
	}
	for _, code := range []string{"u", "y", "z"} {
		if v, ok := f.Subfield(code); ok {
			if v = strings.TrimSpace(v); strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
				return v
			}
		}
	}
	return ""
}

// maxURL is a generous bound on a link worth opening; a longer one is a sign
// of a record carrying something other than a URL.
const maxURL = 2048

// openable rejects a URL that should not be handed to another program. The
// http and https prefixes are checked when the URL is pulled out of the
// record, which already rules out file:// and a leading dash; this rules out
// the control characters that a terminal, a shell history or a desktop handler
// might act on.
func openable(url string) error {
	if len(url) > maxURL {
		return fmt.Errorf("link is %d characters long", len(url))
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only http and https links are opened")
	}
	for _, r := range url {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return fmt.Errorf("link contains a control character")
		}
	}
	return nil
}

// openInBrowser hands a URL to the desktop. It does not wait: the browser
// outliving the terminal session is the normal case.
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
