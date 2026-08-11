// Package tui is the dual-pane record browser: a record list on the left, the
// selected record's content on the right.
package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/marcview/internal/marcio"
	"github.com/beyto1974/marcview/internal/render"
)

type pane int

const (
	paneList pane = iota
	paneDetail
)

type inputMode int

const (
	inputNone inputMode = iota
	inputFilter
	inputJump
)

// batchSize balances first-paint latency against message overhead: the list is
// usable after the first few hundred records while the rest still load.
const batchSize = 256

// loadMsg carries one streamed batch, or the end of the stream.
type loadMsg struct {
	batch marcio.Batch
	err   error
	done  bool
}

// Model is the browser state.
type Model struct {
	path       string
	format     marcio.Format
	forcedUTF8 bool
	records    []*marc.Record
	keys       []string
	issues     []marcio.Issue

	list  list.Model
	vp    viewport.Model
	input textinput.Model
	help  help.Model
	keys_ KeyMap
	st    styles

	focus     pane
	mode      render.Mode
	inputMode inputMode
	filter    filterSpec
	// fullKeys backs the "all:" filter and is built on first use, since it
	// roughly doubles the memory the records occupy.
	fullKeys []string
	// matchLines are the lines of the rendered record holding a filter match,
	// and matchIdx is the one the viewport is parked on.
	matchLines []int
	matchIdx   int
	status     string

	width, height int
	bodyHeight    int    // rows the panes occupy, borders included
	singlePane    bool   // terminal too narrow for both panes
	helpView      string // rendered in layout, since its height drives the body
	loading       bool
	loadErr       error

	ch chan loadMsg
}

// New builds a browser over path and starts loading it in the background.
func New(path string) (*Model, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	st := newStyles()
	l := list.New(nil, compactDelegate{styles: st}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	// Filtering is handled by this model so that the "tag:NNN" syntax and the
	// detail-pane match highlighting stay in step with the list.
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	in := textinput.New()
	in.Prompt = ""

	m := &Model{
		path:    path,
		list:    l,
		vp:      viewport.New(),
		input:   in,
		help:    help.New(),
		keys_:   defaultKeyMap(),
		st:      st,
		mode:    render.Annotated,
		loading: true,
		ch:      make(chan loadMsg, 8),
	}

	go func() {
		defer f.Close()
		err := marcio.Stream(f, batchSize, func(b marcio.Batch) error {
			m.ch <- loadMsg{batch: b}
			return nil
		})
		m.ch <- loadMsg{err: err, done: true}
	}()

	return m, nil
}

// Run opens path in the browser and blocks until the user quits.
func Run(path string) error {
	m, err := New(path)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(waitFor(m.ch), textinput.Blink)
}

// waitFor turns the loader channel into a Bubble Tea message.
func waitFor(ch chan loadMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// filterSpec is a parsed filter expression.
type filterSpec struct {
	query string       // lowercased free text
	tag   string       // field that must be present
	scope marcio.Scope // which index the query is matched against
}

func (f filterSpec) empty() bool { return f.query == "" && f.tag == "" }

// visibleIndices reports the records currently passing the filter, as indices
// into m.records.
func (m *Model) visibleIndices() []int {
	out := make([]int, 0, len(m.records))
	for i, rec := range m.records {
		if m.filter.tag != "" && !marcio.HasTag(rec, m.filter.tag) {
			continue
		}
		if !m.recordMatches(i) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// setFilter applies a filter expression and moves the cursor back to the top
// of the result.
func (m *Model) setFilter(expr string) tea.Cmd {
	m.filter = parseFilter(expr)
	if m.filter.scope.NeedsFullText() {
		m.buildFullKeys()
	}
	m.list.SetDelegate(compactDelegate{styles: m.st, match: m.filter.query})
	cmd := m.rebuildItems()
	m.list.Select(0)
	m.refreshDetail()
	return cmd
}

// buildFullKeys fills the full-text index for any records that do not have one
// yet, which also covers records that arrived after the last "all:" search.
func (m *Model) buildFullKeys() {
	for i := len(m.fullKeys); i < len(m.records); i++ {
		m.fullKeys = append(m.fullKeys, marcio.FullTextKey(m.records[i]))
	}
}

// rebuildItems refreshes the list from the current records and filter.
func (m *Model) rebuildItems() tea.Cmd {
	indices := m.visibleIndices()
	items := make([]list.Item, len(indices))
	for pos, idx := range indices {
		rec := m.records[idx]
		items[pos] = item{
			ordinal: idx + 1,
			index:   idx,
			title:   marcio.Title(rec),
			year:    marcio.Year(rec),
			key:     m.keys[idx],
		}
	}
	return m.list.SetItems(items)
}

// current returns the record under the cursor.
func (m *Model) current() (*marc.Record, item, bool) {
	it, ok := m.list.SelectedItem().(item)
	if !ok || it.index >= len(m.records) {
		return nil, item{}, false
	}
	return m.records[it.index], it, true
}

// refreshDetail re-renders the right pane for the current selection.
func (m *Model) refreshDetail() {
	rec, _, ok := m.current()
	if !ok {
		m.vp.SetContent("")
		return
	}
	out, err := render.Render(rec, m.mode, m.renderOptions(true))
	if err != nil {
		out = "render error: " + err.Error()
	}
	m.vp.SetContent(out)
	m.vp.SetYOffset(0)
	m.findMatches(rec)
}

// renderOptions describes the current view. The coloured and plain renderings
// must agree on width, or the line numbers findMatches computes would not line
// up with what the viewport shows.
func (m *Model) renderOptions(color bool) render.Options {
	return render.Options{
		Color: color,
		Match: m.filter.query,
		Width: m.vp.Width(),
		// A record on one line is unreadable in a pane, so the structured
		// views are always indented here. Export leaves them as they are.
		Indent: true,
	}
}

// findMatches records which lines of the current rendering hold a match, so
// that n and N can step between them. The plain rendering is searched rather
// than the coloured one, whose escape sequences would break the offsets.
func (m *Model) findMatches(rec *marc.Record) {
	m.matchLines, m.matchIdx = nil, 0
	if m.filter.query == "" {
		return
	}
	plain, err := render.Render(rec, m.mode, m.renderOptions(false))
	if err != nil {
		return
	}
	for i, line := range strings.Split(plain, "\n") {
		if strings.Contains(strings.ToLower(line), m.filter.query) {
			m.matchLines = append(m.matchLines, i)
		}
	}
	m.scrollToMatch()
}

// stepMatchingRecord moves the list cursor to the next record matching the
// search term, wrapping at the ends. When a filter is narrowing the list every
// visible record matches, so this steps one row; when it is not, it skips the
// records that do not.
func (m *Model) stepMatchingRecord(dir int) {
	items := m.list.Items()
	if m.filter.query == "" || len(items) == 0 {
		return
	}

	start := m.list.Index()
	for step := 1; step <= len(items); step++ {
		i := ((start+dir*step)%len(items) + len(items)) % len(items)
		it, ok := items[i].(item)
		if !ok {
			continue
		}
		if !m.recordMatches(it.index) {
			continue
		}
		m.list.Select(i)
		m.refreshDetail()
		return
	}
}

// recordMatches reports whether a record matches the current search term in
// the active scope.
func (m *Model) recordMatches(index int) bool {
	if index < 0 || index >= len(m.keys) {
		return false
	}
	var full string
	if index < len(m.fullKeys) {
		full = m.fullKeys[index]
	}
	return m.filter.scope.Matches(m.keys[index], full, m.filter.query)
}

// cycleScope moves the search to the next scope and re-runs the filter.
func (m *Model) cycleScope() tea.Cmd {
	m.filter.scope = m.filter.scope.Next()
	if m.filter.scope.NeedsFullText() {
		m.buildFullKeys()
	}
	cmd := m.rebuildItems()
	m.list.Select(0)
	m.refreshDetail()
	m.status = "search scope: " + m.filter.scope.String()
	return cmd
}

// matchCount is how many lines of the current record match the filter.
func (m *Model) matchCount() int { return len(m.matchLines) }

// matchIndex is the zero-based position within those matches.
func (m *Model) matchIndex() int { return m.matchIdx }

func (m *Model) nextMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx + 1) % len(m.matchLines)
	m.scrollToMatch()
}

func (m *Model) previousMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx - 1 + len(m.matchLines)) % len(m.matchLines)
	m.scrollToMatch()
}

// scrollToMatch parks the current match a third of the way down the pane, so
// there is context above it as well as below.
func (m *Model) scrollToMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	offset := m.matchLines[m.matchIdx] - m.vp.Height()/3
	if offset < 0 {
		offset = 0
	}
	m.vp.SetYOffset(offset)
}

// parseFilter reads a filter expression. "tag:856 brussels" keeps records that
// carry an 856 field and match "brussels" in the list titles; a "rec:" prefix
// searches the record body instead, and "all:" searches both.
func parseFilter(s string) filterSpec {
	f := filterSpec{scope: marcio.ScopeTitles}
	for _, word := range strings.Fields(s) {
		lower := strings.ToLower(word)

		for prefix, scope := range map[string]marcio.Scope{
			"all:": marcio.ScopeBoth,
			"rec:": marcio.ScopeRecord,
		} {
			if rest, ok := strings.CutPrefix(lower, prefix); ok {
				f.scope = scope
				lower, word = rest, rest
			}
		}
		if word == "" {
			continue
		}

		if rest, ok := strings.CutPrefix(lower, "tag:"); ok {
			f.tag = rest
			continue
		}
		if f.query != "" {
			f.query += " "
		}
		f.query += word
	}
	f.query = strings.ToLower(f.query)
	return f
}

// jumpTo moves the cursor to the record with the given 1-based ordinal.
func (m *Model) jumpTo(s string) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 {
		m.status = fmt.Sprintf("not a record number: %q", s)
		return
	}
	for pos, li := range m.list.Items() {
		if it, ok := li.(item); ok && it.ordinal == n {
			m.list.Select(pos)
			m.refreshDetail()
			m.status = ""
			return
		}
	}
	m.status = fmt.Sprintf("record %d is not in the current filter", n)
}
