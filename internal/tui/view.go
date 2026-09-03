package tui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/beyto1974/lunette/internal/marcio"
	"github.com/beyto1974/lunette/internal/render"
)

// Pane geometry. The list is wide enough for an ordinal plus a readable slice
// of title, and never so wide that the record pane cannot show a full subfield.
const (
	listMinWidth  = 28
	listMaxWidth  = 60
	listFraction  = 0.38
	detailMinimum = 30
)

// layout resizes the bubbles to the current terminal size.
func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Below the combined minimum there is no room for two panes, so the
	// terminal shows whichever one has focus.
	m.singlePane = m.zoom || m.width < listMinWidth+detailMinimum

	listWidth := clamp(int(float64(m.width)*listFraction), listMinWidth, listMaxWidth)
	detailWidth := m.width - listWidth
	switch {
	case m.singlePane:
		listWidth, detailWidth = m.width, m.width
	case detailWidth < detailMinimum:
		listWidth = m.width - detailMinimum
		detailWidth = detailMinimum
	}

	// The help view wraps, so its height is only known once it is rendered.
	m.help.SetWidth(m.width)
	m.helpView = m.clipLines(m.help.View(m.keys_))

	bodyHeight := m.height - 1 - m.footerHeight()
	if bodyHeight < 3 {
		bodyHeight = 3
	}
	m.bodyHeight = bodyHeight

	// Each pane spends two rows and two columns on its border and one more row
	// on its header.
	m.list.SetSize(max(listWidth-2, 1), max(bodyHeight-3, 1))
	m.vp.SetWidth(max(detailWidth-2, 1))
	m.vp.SetHeight(max(bodyHeight-3, 1))
	m.input.SetWidth(max(m.width-12, 10))
}

// footerHeight is the status line plus the rendered help.
func (m *Model) footerHeight() int {
	return 1 + lipgloss.Height(m.helpView)
}

// clipLines truncates every line of s to the terminal width.
func (m *Model) clipLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = truncate(line, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("loading " + m.path + "…")
		v.AltScreen = true
		return v
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, m.listPane(), m.detailPane())
	if m.singlePane {
		body = m.listPane()
		if m.focus == paneDetail {
			body = m.detailPane()
		}
	}
	content := strings.Join([]string{m.titleBar(), body, m.footer()}, "\n")

	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "lunette - " + m.path
	// Cell motion is enough: clicks and wheel notches, no drag tracking.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) titleBar() string {
	full := m.buildTitleBar(marcio.Describe(m.paths))
	if m.width > 0 && lipglossWidth(full) > m.width {
		// The counts matter more than where the file lives.
		return truncate(m.buildTitleBar(filepath.Base(m.path)), m.width)
	}
	return full
}

func (m *Model) buildTitleBar(path string) string {
	parts := []string{
		m.st.titleBar.Render("lunette"),
		m.st.fileName.Render(path),
	}

	meta := []string{m.format.String()}
	switch {
	case m.loading:
		meta = append(meta, fmt.Sprintf("loading… %d records", m.count()))
	case m.following:
		meta = append(meta, fmt.Sprintf("%d records · following", m.count()))
	default:
		meta = append(meta, fmt.Sprintf("%d records", m.count()))
	}
	if shown := len(m.list.Items()); shown != m.count() {
		meta = append(meta, fmt.Sprintf("%d shown", shown))
	}
	parts = append(parts, m.st.meta.Render(strings.Join(meta, " · ")))

	if m.forcedUTF8 {
		parts = append(parts, m.st.warn.Render("UTF-8 (leader says MARC-8)"))
	}
	if n := len(m.issues); n > 0 {
		parts = append(parts, m.st.warn.Render(fmt.Sprintf("%d unreadable", n)))
	}
	if m.loadErr != nil {
		parts = append(parts, m.st.warn.Render(m.loadErr.Error()))
	}

	return strings.Join(parts, m.st.meta.Render(" · "))
}

func (m *Model) listPane() string {
	return m.pane(paneList, m.list.Width(), m.listHeader(), m.listBody())
}

// listBody is the list, or an explanation when there is nothing to list. The
// bubble's own "No items." says nothing about the filter that emptied it.
func (m *Model) listBody() string {
	if len(m.list.Items()) > 0 {
		return m.list.View()
	}
	if m.filter.empty() {
		return "\n  This file holds no records."
	}
	return "\n  No record matches.\n\n  " + m.st.meta.Render("esc clears the filter")
}

// listHeader reports where the cursor sits in the file: position, how many
// records are showing when a filter hides some, and how far through the set
// the cursor is.
func (m *Model) listHeader() string {
	total, shown := m.count(), len(m.list.Items())
	if shown == 0 {
		if m.filter.empty() {
			return m.st.meta.Render("no records")
		}
		return m.st.meta.Render("0 of " + strconv.Itoa(total))
	}

	position := m.list.Index() + 1
	label := fmt.Sprintf("%d/%d", position, shown)
	if shown != total {
		label += fmt.Sprintf(" of %d", total)
	}
	return m.st.paneTitle.Render(label) +
		m.st.meta.Render(fmt.Sprintf(" · %d%%", position*100/shown))
}

func (m *Model) detailPane() string {
	header := m.st.meta.Render("no record")
	if _, it, ok := m.current(); ok {
		header = m.detailHeader(it)
	}

	return m.pane(paneDetail, m.vp.Width(), header, m.vp.View())
}

// pane frames a header and body in a border.
//
// Both dimensions are pinned rather than left to lipgloss. Its Width counts the
// border, so passing the content width would squeeze every line by two cells -
// which wrapped each list row onto a second line and put the year underneath
// the title. The body is padded or cut to an exact row count for the same
// reason: a bubble returning more lines than it was sized for would push the
// footer, and with it the help, off the bottom of the terminal.
func (m *Model) pane(which pane, contentWidth int, header, body string) string {
	style := m.st.paneIdle
	if m.focus == which {
		style = m.st.paneActive
	}

	rows := m.bodyHeight - 2 // less the top and bottom border
	if rows < 1 {
		rows = 1
	}
	lines := append([]string{truncate(header, contentWidth)}, strings.Split(body, "\n")...)
	return style.Width(contentWidth + 2).Render(strings.Join(fitLines(lines, rows), "\n"))
}

// fitLines pads or truncates lines to exactly n entries.
func fitLines(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines[:n]
}

// detailHeader packs as much of the record's status into the pane width as
// fits, dropping the least useful part first. Truncating instead would cut the
// scroll position off mid-digit on a narrow terminal.
func (m *Model) detailHeader(it item) string {
	position := fmt.Sprintf("record %d/%d", it.ordinal, m.count())

	var rest []string
	if n := m.fieldCount(); n > 0 {
		rest = append(rest, fmt.Sprintf("field %d/%d", m.fieldCursor+1, n))
	}
	if n := m.matchCount(); n > 0 {
		rest = append(rest, fmt.Sprintf("match %d/%d", m.matchIdx+1, n))
	}
	rest = append(rest, m.mode.String(), scrollHint(m.vp.ScrollPercent()))
	if cn := m.currentControlNumber(); cn != "" {
		rest = append(rest, cn)
	}

	const separator = " · "
	width := m.vp.Width()
	used := lipglossWidth(position)
	kept := rest[:0]
	for _, part := range rest {
		if used+len(separator)+lipglossWidth(part) > width {
			continue
		}
		used += len(separator) + lipglossWidth(part)
		kept = append(kept, part)
	}

	header := m.st.paneTitle.Render(position)
	if len(kept) > 0 {
		header += m.st.meta.Render(separator + strings.Join(kept, separator))
	}
	return header
}

func (m *Model) currentControlNumber() string {
	rec, _, ok := m.current()
	if !ok {
		return ""
	}
	return render.Sanitize(marcio.ControlNumber(rec))
}

func scrollHint(pct float64) string {
	return fmt.Sprintf("%3.0f%%", pct*100)
}

func (m *Model) footer() string {
	var top string
	switch m.inputMode {
	case inputFilter:
		top = m.st.prompt.Render("filter ") + m.input.View() +
			m.st.meta.Render("   (tag:856 narrows by field)")
	case inputJump:
		top = m.st.prompt.Render("record # ") + m.input.View()
	default:
		top = m.st.status.Render(m.statusLine())
	}

	return truncate(top, m.width) + "\n" + m.helpView
}

func (m *Model) statusLine() string {
	if m.status != "" {
		return m.status
	}
	var parts []string
	if m.filter.tag != "" {
		parts = append(parts, "tag:"+m.filter.tag)
	}
	if m.filter.query != "" {
		parts = append(parts, m.filter.query+" in "+m.filter.scope.String())
	}
	if len(parts) == 0 {
		return "no filter"
	}
	return "filter " + strings.Join(parts, " ")
}

// truncate cuts a rendered string to width, keeping ANSI styling intact.
func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
