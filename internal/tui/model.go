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

	"github.com/everbright/marco/marcview/internal/marcio"
	"github.com/everbright/marco/marcview/internal/render"
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
	query     string
	tagFilter string
	status    string

	width, height int
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

// visible reports the records currently passing the filter, as indices into
// m.records.
func (m *Model) visibleIndices() []int {
	out := make([]int, 0, len(m.records))
	for i, rec := range m.records {
		if m.tagFilter != "" && !marcio.HasTag(rec, m.tagFilter) {
			continue
		}
		if m.query != "" && !strings.Contains(m.keys[i], m.query) {
			continue
		}
		out = append(out, i)
	}
	return out
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
	out, err := render.Render(rec, m.mode, render.Options{Color: true, Match: m.query})
	if err != nil {
		out = "render error: " + err.Error()
	}
	m.vp.SetContent(out)
	m.vp.SetYOffset(0)
}

// parseFilter splits a filter expression into a free-text query and a tag
// restriction. "tag:856 brussels" keeps records that carry an 856 field and
// match "brussels".
func parseFilter(s string) (query, tag string) {
	for _, word := range strings.Fields(s) {
		if rest, ok := strings.CutPrefix(strings.ToLower(word), "tag:"); ok {
			tag = rest
			continue
		}
		if query != "" {
			query += " "
		}
		query += word
	}
	return strings.ToLower(query), tag
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
