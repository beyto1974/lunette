package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/beyto1974/marcview/internal/render"
)

// newLoaded builds a model over the sample file and drains the background
// loader, so tests see the same state the UI would after startup.
func newLoaded(t *testing.T) *Model {
	t.Helper()
	m, err := New(filepath.Join("..", "..", "testdata", "sample.mrc"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	for {
		msg := <-m.ch
		m.Update(msg)
		if msg.done {
			if msg.err != nil {
				t.Fatalf("load: %v", msg.err)
			}
			return m
		}
	}
}

func TestLoadPopulatesList(t *testing.T) {
	m := newLoaded(t)
	if len(m.records) != 3 {
		t.Fatalf("loaded %d records, want 3", len(m.records))
	}
	if got := len(m.list.Items()); got != 3 {
		t.Errorf("list has %d items, want 3", got)
	}
	if m.loading {
		t.Error("still loading after the done message")
	}
}

func TestDetailFollowsSelection(t *testing.T) {
	m := newLoaded(t)
	if !strings.Contains(m.vp.GetContent(), "Identification") {
		t.Errorf("detail pane does not show the first record:\n%s", m.vp.GetContent())
	}

	m.list.Select(1)
	m.refreshDetail()
	content := m.vp.GetContent()
	if !strings.Contains(content, "Kloza") {
		t.Errorf("detail pane did not follow the cursor:\n%s", content)
	}
	if !strings.Contains(content, "Title Statement") {
		t.Error("detail pane is not in annotated mode by default")
	}
}

func TestModeSwitch(t *testing.T) {
	m := newLoaded(t)
	m.setMode(render.Raw)
	if !strings.Contains(m.vp.GetContent(), "=245") {
		t.Errorf("raw mode not applied:\n%s", m.vp.GetContent())
	}
	m.setMode(render.JSON)
	if !strings.Contains(m.vp.GetContent(), "leader") {
		t.Errorf("json mode not applied:\n%s", m.vp.GetContent())
	}
}

func TestFilterNarrowsList(t *testing.T) {
	m := newLoaded(t)

	tests := []struct {
		expr string
		want int
	}{
		{"", 3},
		{"transmission", 1},
		{"TRANSMISSION", 1},
		{"tag:856", 1},
		{"tag:245", 3},
		{"tag:856 transmission", 1},
		{"tag:856 kloza", 0},
		{"zzzz", 0},
		// "privacy" lives in a 650, which the list key does not cover.
		{"privacy", 0},
		{"all:privacy", 1},
		{"all:transmission", 1},
		{"tag:650 all:privacy", 1},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			m.setFilter(tt.expr)
			m.rebuildItems()
			if got := len(m.list.Items()); got != tt.want {
				t.Errorf("filter %q kept %d records, want %d", tt.expr, got, tt.want)
			}
		})
	}
}

// The ordinal shown in the list must survive filtering, so that a jump to
// record 3 still finds record 3.
func TestJumpTo(t *testing.T) {
	m := newLoaded(t)

	m.jumpTo("3")
	if _, it, ok := m.current(); !ok || it.ordinal != 3 {
		t.Errorf("jump to 3 selected %+v", it)
	}
	if !strings.Contains(m.vp.GetContent(), "Sans auteur") {
		t.Error("jump did not refresh the detail pane")
	}

	m.jumpTo("99")
	if !strings.Contains(m.status, "not in the current filter") {
		t.Errorf("status after an out-of-range jump = %q", m.status)
	}
	m.jumpTo("abc")
	if !strings.Contains(m.status, "not a record number") {
		t.Errorf("status after a non-numeric jump = %q", m.status)
	}

	// A record filtered out cannot be jumped to.
	m.setFilter("tag:856")
	m.jumpTo("3")
	if !strings.Contains(m.status, "not in the current filter") {
		t.Errorf("status = %q, want the filter explanation", m.status)
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		in        string
		wantQuery string
		wantTag   string
		wantAll   bool
	}{
		{"", "", "", false},
		{"brussels", "brussels", "", false},
		{"BRUSSELS", "brussels", "", false},
		{"tag:856", "", "856", false},
		{"tag:856 brussels", "brussels", "856", false},
		{"brussels tag:856", "brussels", "856", false},
		{"two words", "two words", "", false},
		{"all:brussels", "brussels", "", true},
		{"all:tag:856", "", "856", true},
		{"all: brussels", "brussels", "", true},
		{"tag:856 all:brussels", "brussels", "856", true},
	}
	for _, tt := range tests {
		f := parseFilter(tt.in)
		if f.query != tt.wantQuery || f.tag != tt.wantTag || f.fullText != tt.wantAll {
			t.Errorf("parseFilter(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.in, f.query, f.tag, f.fullText, tt.wantQuery, tt.wantTag, tt.wantAll)
		}
	}
}

func TestFilterExpressionRoundTrip(t *testing.T) {
	m := newLoaded(t)
	for _, expr := range []string{"", "brussels", "tag:856", "tag:856 brussels", "all:brussels", "tag:856 all:brussels"} {
		m.setFilter(expr)
		if got := m.filterExpression(); got != expr {
			t.Errorf("filterExpression() = %q, want %q", got, expr)
		}
	}
}

// The view must render at any terminal size without panicking or spilling past
// the terminal width.
func TestViewFitsWidth(t *testing.T) {
	m := newLoaded(t)
	for _, size := range []struct{ w, h int }{{120, 40}, {80, 24}, {40, 12}, {20, 8}} {
		m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		view := m.View()
		for i, line := range strings.Split(view.Content, "\n") {
			if w := lipglossWidth(line); w > size.w {
				t.Errorf("terminal %dx%d: line %d is %d cells wide", size.w, size.h, i, w)
			}
		}
	}
}

// The rendered view must fit the terminal in both directions. It did not:
// toggling full help pushed the footer - the help itself - off the bottom, so
// "?" appeared to do nothing.
func TestViewFitsHeight(t *testing.T) {
	m := newLoaded(t)

	for _, showAll := range []bool{false, true} {
		m.help.ShowAll = showAll
		for _, size := range []struct{ w, h int }{{120, 40}, {100, 30}, {80, 24}, {60, 16}, {40, 12}} {
			m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) > size.h {
				t.Errorf("help=%v terminal %dx%d: view is %d lines, want at most %d",
					showAll, size.w, size.h, len(lines), size.h)
			}
		}
	}
}

// Pressing "?" must actually reveal the full help.
func TestHelpToggleShowsFullHelp(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if strings.Contains(stripANSI(m.View().Content), "previous match") {
		t.Fatal("full help is visible before it was asked for")
	}

	m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})

	content := stripANSI(m.View().Content)
	for _, want := range []string{"previous match", "compact", "jump to record"} {
		if !strings.Contains(content, want) {
			t.Errorf("full help does not mention %q after '?':\n%s", want, content)
		}
	}

	m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if strings.Contains(stripANSI(m.View().Content), "previous match") {
		t.Error("'?' did not toggle the full help back off")
	}
}

// Every binding must be reachable from the full help; the bubble ellipsises
// columns that do not fit, which silently hid three of them.
func TestFullHelpListsEveryBinding(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.help.ShowAll = true
	m.layout()

	help := stripANSI(m.helpView)
	for _, group := range m.keys_.FullHelp() {
		for _, b := range group {
			if !strings.Contains(help, b.Help().Desc) {
				t.Errorf("full help at 100 columns omits %q:\n%s", b.Help().Desc, help)
			}
		}
	}
}

func TestClamp(t *testing.T) {
	if clamp(5, 10, 20) != 10 || clamp(25, 10, 20) != 20 || clamp(15, 10, 20) != 15 {
		t.Error("clamp is wrong")
	}
}

// With a filter active the list must show where each record matched, not just
// that it did.
func TestListHighlightsMatches(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.setFilter("")
	plain := m.list.View()
	m.setFilter("transmission")
	highlighted := m.list.View()

	if highlighted == plain {
		t.Error("the list looks the same with and without a filter")
	}
	// The list pane truncates, so compare against what fits.
	if !strings.Contains(stripANSI(highlighted), "Identification of Trans") {
		t.Errorf("highlighting mangled the title:\n%s", stripANSI(highlighted))
	}
	// Truncation must not have cut an escape sequence in half: after stripping
	// complete sequences, no escape byte may remain.
	if strings.ContainsRune(stripANSI(highlighted), 0x1b) {
		t.Error("highlighted list output has an unterminated escape sequence")
	}
}

func TestMatchNavigation(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// No filter, no matches to step through.
	if got := m.matchCount(); got != 0 {
		t.Errorf("matchCount without a filter = %d, want 0", got)
	}

	m.setFilter("transmission")
	if got := m.matchCount(); got < 1 {
		t.Fatalf("matchCount = %d, want at least 1 in the selected record", got)
	}

	// "identification" appears in both 245 $a and the search key; use a term
	// with several hits in the record to exercise wrapping.
	m.setFilter("de")
	n := m.matchCount()
	if n < 2 {
		t.Skipf("fixture has only %d matches for 'de'", n)
	}
	first := m.matchIndex()
	m.nextMatch()
	if m.matchIndex() == first {
		t.Error("nextMatch did not move")
	}
	for i := 0; i < n; i++ {
		m.nextMatch()
	}
	if m.matchIndex() != m.matchIndex()%n {
		t.Error("nextMatch did not wrap")
	}
	m.previousMatch()
	if m.matchIndex() < 0 || m.matchIndex() >= n {
		t.Errorf("previousMatch left the index at %d, outside 0..%d", m.matchIndex(), n-1)
	}
}

// The detail header reports the match position so the user knows how far
// through the record they are.
func TestMatchCounterInHeader(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.setFilter("transmission")

	header := stripANSI(m.detailPane())
	if !strings.Contains(header, "match 1/") {
		t.Errorf("detail header does not report the match position:\n%s", header)
	}
}

// stripANSI removes CSI sequences so tests can compare visible text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// The record pane wraps to its own width, so a long 856 URL stays readable
// instead of being cut at the border.
func TestDetailWrapsToPaneWidth(t *testing.T) {
	m := newLoaded(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.setMode(render.Compact)

	content := m.vp.GetContent()
	for i, line := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(line); w > m.vp.Width() {
			t.Errorf("line %d is %d cells wide, pane is %d: %q", i, w, m.vp.Width(), line)
		}
	}

	// The URL is longer than the pane, so it must appear broken across lines
	// rather than truncated away.
	flat := strings.ReplaceAll(stripANSI(content), "\n", "")
	flat = strings.ReplaceAll(flat, " ", "")
	if !strings.Contains(flat, "https://biblio.vub.ac.be/vubir/rec-0001.html") {
		t.Errorf("the 856 URL was lost:\n%s", stripANSI(content))
	}
}
