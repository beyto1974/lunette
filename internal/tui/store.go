// What the browser keeps for each record, and how it gets the record itself
// back when it needs one.
package tui

import (
	"fmt"
	"os"

	marc "github.com/beyto1974/gomarc"

	"github.com/beyto1974/lunette/internal/marcio"
	"github.com/beyto1974/lunette/internal/render"
)

// entry is everything the browser keeps for one record: what its list row
// shows, what a filter is answered from, and where the record sits.
//
// A decoded record costs several kilobytes; this costs a few hundred bytes. On
// the harvests this exists to read - 1.1 million records, 3.7 GB - that is the
// difference between a browser that opens and one that exhausts the machine.
type entry struct {
	title string
	year  string
	// key is the list-scope search index: control number, title, author, year.
	key string
	// tags are the field tags the record carries, so that a tag filter is
	// answered without reading the record back.
	tags string
	ext  marcio.Extent
	// file is which of the browser's paths the record came from.
	file int
}

// newEntry reduces a record to what is kept.
func newEntry(rec *marc.Record, ext marcio.Extent, file int) entry {
	return entry{
		// Record text on its way to the terminal: see render.Sanitize. It is
		// done here so that a row is not sanitised again every time it is
		// drawn.
		title: render.Sanitize(marcio.Title(rec)),
		year:  render.Sanitize(marcio.Year(rec)),
		key:   marcio.SearchKey(rec),
		tags:  marcio.Tags(rec),
		ext:   ext,
		file:  file,
	}
}

// count is how many records have been loaded so far.
func (m *Model) count() int { return len(m.entries) }

// record fetches one record from the file it came from. The last one fetched
// is kept, since the browser asks for the same record repeatedly - a redraw, a
// mode switch, a resize - and re-reading it each time would be waste.
func (m *Model) record(i int) (*marc.Record, error) {
	if i < 0 || i >= len(m.entries) {
		return nil, fmt.Errorf("there is no record %d", i+1)
	}
	if m.cached != nil && m.cachedIdx == i {
		return m.cached, nil
	}

	e := m.entries[i]
	rr, err := m.readerFor(e.file)
	if err != nil {
		return nil, err
	}
	rec, err := rr.At(e.ext)
	if err != nil {
		return nil, err
	}
	m.rereads++
	m.cached, m.cachedIdx = rec, i
	return rec, nil
}

// readerFor opens a file the first time a record is wanted from it and keeps
// it open, so that browsing a set of harvest files is one descriptor each
// rather than one open per keystroke.
func (m *Model) readerFor(i int) (*marcio.RecordReader, error) {
	if i < 0 || i >= len(m.paths) {
		return nil, fmt.Errorf("no file %d", i)
	}
	if i < len(m.readers) && m.readers[i] != nil {
		return m.readers[i], nil
	}

	f, err := os.Open(m.paths[i])
	if err != nil {
		return nil, err
	}
	for len(m.files) <= i {
		m.files = append(m.files, nil)
		m.readers = append(m.readers, nil)
	}
	m.files[i] = f
	m.readers[i] = marcio.NewRecordReader(f, m.format, m.forcedUTF8)
	return m.readers[i], nil
}

// closeFiles releases the descriptors the browser opened to fetch records.
func (m *Model) closeFiles() {
	for _, f := range m.files {
		if f != nil {
			f.Close()
		}
	}
	m.files, m.readers = nil, nil
}

// pathIndex maps the file a batch came from to its place in the browser's
// paths. A batch with no source is the first file, which the browser reads
// itself so that -follow can bound it.
func (m *Model) pathIndex(source string) int {
	if source == "" {
		return 0
	}
	for i, p := range m.paths {
		if p == source {
			return i
		}
	}
	return 0
}
