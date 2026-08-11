package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// item is one row in the left pane. Everything shown is precomputed at load
// time so that scrolling never touches the MARC record itself.
type item struct {
	// ordinal is the 1-based position among the records that decoded; it
	// diverges from the position in the file when records were skipped, which
	// the title bar reports and `marcview validate` details.
	ordinal int
	index   int // position in Model.records
	title   string
	year    string
	key     string
}

func (i item) FilterValue() string { return i.key }

// compactDelegate renders one record per line: ordinal, title, year.
type compactDelegate struct {
	styles styles
}

func (d compactDelegate) Height() int                         { return 1 }
func (d compactDelegate) Spacing() int                        { return 0 }
func (d compactDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		return
	}

	cursor := "  "
	titleStyle := d.styles.itemNormal
	if index == m.Index() {
		cursor = "▸ "
		titleStyle = d.styles.itemSelected
	}

	ordinal := d.styles.itemOrdinal.Render(fmt.Sprintf("%05d", it.ordinal))
	year := ""
	if it.year != "" {
		year = " " + d.styles.itemYear.Render(it.year)
	}
	// Width budget: cursor + ordinal + space + title + year.
	fixed := 2 + 5 + 1 + lipglossWidth(year)
	titleWidth := m.Width() - fixed
	if titleWidth < 4 {
		titleWidth = 4
	}
	title := ansi.Truncate(it.title, titleWidth, "…")
	if title == "" {
		title = "(no title)"
	}

	fmt.Fprint(w, cursor+ordinal+" "+titleStyle.Render(title)+year)
}

// lipglossWidth measures printable width, ignoring ANSI escapes.
func lipglossWidth(s string) int {
	if s == "" {
		return 0
	}
	return ansi.StringWidth(strings.TrimRight(s, "\n"))
}
