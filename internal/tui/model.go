// Package tui is the dual-pane record browser: a record list on the left, the
// selected record's content on the right.
package tui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/lunette/internal/marcio"
	"github.com/beyto1974/lunette/internal/render"
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
	// paths are the files being browsed, read as one set; path is the first,
	// which is the one -follow watches.
	paths      []string
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

	// fields are the line ranges of the record on show, and fieldCursor is the
	// one selected. Structured views carry no spans, and then the record pane
	// scrolls by line as before.
	fields      []render.FieldSpan
	fieldCursor int
	// zoom forces the single-pane layout on a terminal wide enough for two.
	zoom bool
	// openURL hands a link to the desktop; tests replace it.
	openURL func(string) error
	// following polls the file for records appended after the initial load,
	// which is how a harvest still being written can be browsed live.
	following    bool
	followOffset int64
	// watching is true when the filesystem reports changes; when it is false
	// and following is true, a timer does the work instead.
	watching bool
	watcher  changeSource
	// newWatcher is replaced in tests.
	newWatcher func(string) (changeSource, error)
	status     string

	width, height int
	bodyHeight    int    // rows the panes occupy, borders included
	singlePane    bool   // terminal too narrow for both panes
	helpView      string // rendered in layout, since its height drives the body
	loading       bool
	loadErr       error

	ch chan loadMsg
}

// Option configures a browser.
type Option func(*Model)

// WithFollow keeps reading the file as records are appended to it.
func WithFollow() Option { return func(m *Model) { m.following = true } }

// New builds a browser over one or more files and starts loading in the
// background.
func New(paths []string, opts ...Option) (*Model, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files given")
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
		paths:   paths,
		path:    paths[0],
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
	for _, opt := range opts {
		opt(m)
	}

	// Open the first file here rather than in the goroutine so that a missing
	// file is an error from New, before the browser starts.
	first, err := os.Open(m.path)
	if err != nil {
		return nil, err
	}
	// Bound the first read before the goroutine starts: writing model state
	// from inside it would be a race waiting for its first refactor.
	source := m.initialReader(first)
	rest := paths[1:]

	go func() {
		defer first.Close()
		err := marcio.Stream(source, batchSize, func(b marcio.Batch) error {
			m.ch <- loadMsg{batch: b}
			return nil
		})
		// The files after the first are read whole: only the first can be
		// followed, and streaming them separately buys nothing.
		if err == nil && len(rest) > 0 {
			var res *marcio.Result
			if res, err = marcio.LoadFiles(rest); err == nil {
				m.ch <- loadMsg{batch: marcio.Batch{
					Format:     res.Format,
					ForcedUTF8: res.ForcedUTF8,
					Records:    res.Records,
					Issues:     res.Issues,
				}}
			}
		}
		m.ch <- loadMsg{err: err, done: true}
	}()

	return m, nil
}

// initialReader bounds the first read. When following a file that is still
// being written, the last record is usually half there; reading it would
// report a harvest in progress as a damaged record, and the same bytes would
// then be read again by the first poll.
func (m *Model) initialReader(f *os.File) io.Reader {
	if !m.following {
		return f
	}
	complete, err := marcio.CompletePrefixFile(m.path)
	if err != nil {
		return f
	}
	m.followOffset = complete
	return io.LimitReader(f, complete)
}

// Run opens the files in the browser and blocks until the user quits.
func Run(paths []string, opts ...Option) error {
	m, err := New(paths, opts...)
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

// rebuildItems refreshes the list from the current records and filter.
func (m *Model) rebuildItems() tea.Cmd {
	indices := m.visibleIndices()
	items := make([]list.Item, len(indices))
	for pos, idx := range indices {
		rec := m.records[idx]
		items[pos] = item{
			ordinal: idx + 1,
			index:   idx,
			// Record text on its way to the terminal: see render.Sanitize.
			title: render.Sanitize(marcio.Title(rec)),
			year:  render.Sanitize(marcio.Year(rec)),
			key:   m.keys[idx],
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

// refreshDetail re-renders the right pane for the current selection and puts
// the field cursor back at the top of the record.
func (m *Model) refreshDetail() {
	m.fieldCursor = 0
	m.redrawDetail()

	if rec, _, ok := m.current(); ok {
		m.vp.SetYOffset(0)
		m.findMatches(rec)
	}
}

// reflowDetail re-renders after a width change. The record wraps differently,
// so the match lines have to be found again, but the field cursor belongs to
// the user and stays where it is.
func (m *Model) reflowDetail() {
	m.redrawDetail()
	if rec, _, ok := m.current(); ok {
		wanted := m.matchIdx
		m.findMatches(rec)
		if wanted < len(m.matchLines) {
			m.matchIdx = wanted
		}
	}
	m.scrollToField()
}

// redrawDetail re-renders the record with the current field cursor marked,
// leaving the scroll position alone.
func (m *Model) redrawDetail() {
	rec, _, ok := m.current()
	if !ok {
		m.vp.SetContent("")
		m.fields = nil
		return
	}

	lay, err := render.RenderLayout(rec, m.mode, m.renderOptions(true))
	if err != nil {
		m.vp.SetContent("render error: " + err.Error())
		m.fields = nil
		return
	}
	m.fields = lay.Fields
	if m.fieldCursor >= len(m.fields) {
		m.fieldCursor = max(len(m.fields)-1, 0)
	}
	m.vp.SetContent(m.withFieldMarker(lay))
}

// renderOptions describes the current view. The coloured and plain renderings
// must agree on width, or the line numbers findMatches computes would not line
// up with what the viewport shows.
func (m *Model) renderOptions(color bool) render.Options {
	return render.Options{
		Color: color,
		Match: m.filter.query,
		// Two cells go to the field-cursor gutter.
		Width: max(m.vp.Width()-2, 1),
		// A record on one line is unreadable in a pane, so the structured
		// views are always indented here. Export leaves them as they are.
		Indent: true,
	}
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
