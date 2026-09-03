// Package tui is the dual-pane record browser: a record list on the left, the
// selected record's content on the right.
package tui

import (
	"errors"
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

// errNoSelection is "the cursor is on nothing", as against a record that could
// not be read: an empty pane rather than an explanation.
var errNoSelection = errors.New("no record selected")

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
	// entries are what is kept for every record loaded; the records
	// themselves are fetched from the file when one is wanted. See store.go.
	entries []entry
	issues  []marcio.Issue
	// files and readers are opened on demand, one per path, and stay open for
	// as long as the browser does; fileInfo remembers how each was read, since
	// that is how its records have to be read back.
	files    []*os.File
	readers  []*marcio.RecordReader
	fileInfo []fileInfo
	// cached is the record last fetched, kept because the browser asks for the
	// same one over and over as it redraws.
	cached    *marc.Record
	cachedIdx int
	// rereads counts records fetched back from a file, which is what the
	// store tests assert on.
	rereads int

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
	// items are the list rows, kept here so that records arriving during a
	// load can be appended rather than sending the list a fresh slice built
	// from every record so far.
	items []list.Item
	// rowsBuilt counts the rows ever built, which is what the load tests
	// assert on: one per record, not one per record per batch.
	rowsBuilt int
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
		paths:     paths,
		path:      paths[0],
		list:      l,
		vp:        viewport.New(),
		input:     in,
		help:      help.New(),
		keys_:     defaultKeyMap(),
		st:        st,
		mode:      render.Annotated,
		loading:   true,
		cachedIdx: -1,
		ch:        make(chan loadMsg, 8),
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
		// The files after the first are streamed in turn; each batch says
		// which file it came from, which is how a record is fetched back from
		// the right one.
		if err == nil && len(rest) > 0 {
			_, err = marcio.StreamFiles(rest, batchSize, func(b marcio.Batch) error {
				m.ch <- loadMsg{batch: b}
				return nil
			})
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
	defer m.closeFiles()
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

// row builds one list row from what was kept for the record. Everything it
// shows was worked out when the record arrived, which is why the list is
// appended to rather than rebuilt as records load.
func (m *Model) row(idx int) item {
	m.rowsBuilt++
	e := m.entries[idx]
	return item{
		ordinal: idx + 1,
		index:   idx,
		title:   e.title,
		year:    e.year,
		key:     e.key,
	}
}

// rebuildItems refreshes the list from the current records and filter. It is
// for changes that can move any row - a new filter, a new scope - and costs a
// pass over every record, so loading does not use it.
func (m *Model) rebuildItems() tea.Cmd {
	indices := m.visibleIndices()
	m.items = make([]list.Item, len(indices))
	for pos, idx := range indices {
		m.items[pos] = m.row(idx)
	}
	return m.list.SetItems(m.items)
}

// appendItems adds the rows for records[from:] that pass the current filter,
// leaving the rows already built alone.
//
// Rebuilding the whole list on every batch is quadratic, and the files this
// browser exists for arrive in thousands of batches: a 1.1 million record
// harvest would build the best part of a billion rows on the way in, which
// looks exactly like a hang.
func (m *Model) appendItems(from int) tea.Cmd {
	for idx := from; idx < m.count(); idx++ {
		if !m.visible(idx) {
			continue
		}
		m.items = append(m.items, m.row(idx))
	}
	return m.list.SetItems(m.items)
}

// current returns the record under the cursor, fetching it from the file it
// came from.
func (m *Model) current() (*marc.Record, item, bool) {
	rec, it, err := m.currentRecord()
	if err != nil {
		return nil, item{}, false
	}
	return rec, it, true
}

// currentOrExplain is current() for something the user asked for. A record
// that cannot be read back says so in the status line: doing nothing silently
// reads as a broken key rather than a broken file.
func (m *Model) currentOrExplain() (*marc.Record, item, bool) {
	rec, it, err := m.currentRecord()
	if err != nil {
		if !errors.Is(err, errNoSelection) {
			m.status = unreadable + err.Error()
		}
		return nil, item{}, false
	}
	return rec, it, true
}

// unreadable prefixes every report of a record that could not be fetched back.
const unreadable = "this record could not be read back: "

// currentRecord is current with the reason it failed, which the record pane
// shows rather than drawing a blank.
func (m *Model) currentRecord() (*marc.Record, item, error) {
	it, ok := m.list.SelectedItem().(item)
	if !ok || it.index >= m.count() {
		return nil, item{}, errNoSelection
	}
	rec, err := m.record(it.index)
	if err != nil {
		return nil, it, err
	}
	return rec, it, nil
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
	rec, _, err := m.currentRecord()
	if errors.Is(err, errNoSelection) {
		m.vp.SetContent("")
		m.fields = nil
		return
	}
	if err != nil {
		m.vp.SetContent(unreadable + err.Error())
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
