package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/everbright/marco/marcview/internal/render"
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
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			m.query, m.tagFilter = parseFilter(tt.expr)
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
	m.query, m.tagFilter = parseFilter("tag:856")
	m.rebuildItems()
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
	}{
		{"", "", ""},
		{"brussels", "brussels", ""},
		{"BRUSSELS", "brussels", ""},
		{"tag:856", "", "856"},
		{"tag:856 brussels", "brussels", "856"},
		{"brussels tag:856", "brussels", "856"},
		{"two words", "two words", ""},
	}
	for _, tt := range tests {
		q, tag := parseFilter(tt.in)
		if q != tt.wantQuery || tag != tt.wantTag {
			t.Errorf("parseFilter(%q) = (%q, %q), want (%q, %q)", tt.in, q, tag, tt.wantQuery, tt.wantTag)
		}
	}
}

func TestFilterExpressionRoundTrip(t *testing.T) {
	m := newLoaded(t)
	for _, expr := range []string{"", "brussels", "tag:856", "tag:856 brussels"} {
		m.query, m.tagFilter = parseFilter(expr)
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

func TestClamp(t *testing.T) {
	if clamp(5, 10, 20) != 10 || clamp(25, 10, 20) != 20 || clamp(15, 10, 20) != 15 {
		t.Error("clamp is wrong")
	}
}
