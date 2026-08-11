package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/beyto1974/marcview/internal/marcio"
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
	m.singlePane = m.width < listMinWidth+detailMinimum

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

	// Each pane spends two rows and two columns on its border, and the detail
	// pane one more row on its header.
	m.list.SetSize(max(listWidth-2, 1), max(bodyHeight-2, 1))
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
	v.WindowTitle = "marcview - " + m.path
	return v
}

func (m *Model) titleBar() string {
	parts := []string{
		m.st.titleBar.Render("marcview"),
		m.st.fileName.Render(m.path),
	}

	meta := []string{m.format.String()}
	if m.loading {
		meta = append(meta, fmt.Sprintf("loading… %d records", len(m.records)))
	} else {
		meta = append(meta, fmt.Sprintf("%d records", len(m.records)))
	}
	if shown := len(m.list.Items()); shown != len(m.records) {
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

	return truncate(strings.Join(parts, m.st.meta.Render(" · ")), m.width)
}

func (m *Model) listPane() string {
	width := m.list.Width() + 2
	style := m.st.paneIdle
	if m.focus == paneList {
		style = m.st.paneActive
	}
	return style.Width(width - 2).Render(m.list.View())
}

func (m *Model) detailPane() string {
	style := m.st.paneIdle
	if m.focus == paneDetail {
		style = m.st.paneActive
	}

	header := m.st.meta.Render("no record")
	if _, it, ok := m.current(); ok {
		label := fmt.Sprintf("record %d/%d", it.ordinal, len(m.records))
		if cn := m.currentControlNumber(); cn != "" {
			label += " · " + cn
		}
		meta := " · " + m.mode.String()
		if n := m.matchCount(); n > 0 {
			meta += fmt.Sprintf(" · match %d/%d", m.matchIdx+1, n)
		}
		meta += " · " + scrollHint(m.vp.ScrollPercent())
		header = m.st.paneTitle.Render(label) + m.st.meta.Render(meta)
	}

	inner := strings.Join([]string{truncate(header, m.vp.Width()), m.vp.View()}, "\n")
	return style.Width(m.vp.Width()).Render(inner)
}

func (m *Model) currentControlNumber() string {
	rec, _, ok := m.current()
	if !ok {
		return ""
	}
	return marcio.ControlNumber(rec)
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
		scope := "match:"
		if m.filter.fullText {
			scope = "all:"
		}
		parts = append(parts, scope+m.filter.query)
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
