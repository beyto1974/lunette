package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// item is one row in the left pane. Everything shown is precomputed at load
// time so that scrolling never touches the MARC record itself.
type item struct {
	// ordinal is the 1-based position among the records that decoded; it
	// diverges from the position in the file when records were skipped, which
	// the title bar reports and `lunette validate` details.
	ordinal int
	index   int // position in Model.records
	title   string
	year    string
	key     string
}

func (i item) FilterValue() string { return i.key }

// compactDelegate renders one record per line: ordinal, title, year. match is
// the active filter term, highlighted inside the title so a filtered list
// shows where each record matched and not merely that it did.
type compactDelegate struct {
	styles styles
	match  string
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
	// Truncate before styling: cutting a string that already holds escape
	// sequences would leave the terminal mid-sequence.
	title := ansi.Truncate(it.title, titleWidth, "…")
	if title == "" {
		title = "(no title)"
	}

	fmt.Fprint(w, cursor+ordinal+" "+d.renderTitle(title, titleStyle)+year)
}

// renderTitle styles the title, giving the matched span its own colour.
func (d compactDelegate) renderTitle(title string, base lipgloss.Style) string {
	if d.match == "" {
		return base.Render(title)
	}

	lowerTitle, lowerMatch := strings.ToLower(title), strings.ToLower(d.match)
	var b strings.Builder
	for {
		i := strings.Index(lowerTitle, lowerMatch)
		if i < 0 {
			b.WriteString(base.Render(title))
			return b.String()
		}
		b.WriteString(base.Render(title[:i]))
		b.WriteString(d.styles.itemMatch.Render(title[i : i+len(d.match)]))
		title, lowerTitle = title[i+len(d.match):], lowerTitle[i+len(d.match):]
	}
}

// lipglossWidth measures printable width, ignoring ANSI escapes.
func lipglossWidth(s string) int {
	if s == "" {
		return 0
	}
	return ansi.StringWidth(strings.TrimRight(s, "\n"))
}
