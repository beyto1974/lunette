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

// fileInfo is how one input was read, which is how its records have to be read
// back. A set of files need not all be the same format, and a leader the bytes
// contradict is a per-file decision: read back any other way than it was read
// in, a record decodes differently or not at all.
type fileInfo struct {
	format     marcio.Format
	forcedUTF8 bool
	known      bool
}

// noteFile records how a file was read, from the first batch that says.
func (m *Model) noteFile(i int, format marcio.Format, forcedUTF8 bool) {
	if i < 0 || format == marcio.FormatUnknown {
		return
	}
	for len(m.fileInfo) <= i {
		m.fileInfo = append(m.fileInfo, fileInfo{})
	}
	if !m.fileInfo[i].known {
		m.fileInfo[i] = fileInfo{format: format, forcedUTF8: forcedUTF8, known: true}
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
	var rec *marc.Record
	err := m.withReader(e.file, func(rr *marcio.RecordReader) error {
		var err error
		rec, err = rr.At(e.ext)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.rereads++
	m.cached, m.cachedIdx = rec, i
	return rec, nil
}

// forgetCached drops the record being held, for when the file it came from may
// no longer say the same thing.
func (m *Model) forgetCached() { m.cached, m.cachedIdx = nil, -1 }

// withReader opens the file a record came from, hands a reader for it to fn,
// and closes it again.
//
// The descriptor is deliberately not kept. A browser that held one open for as
// long as it ran would stop a harvester replacing the file at all on Windows,
// where an open file cannot be renamed over, and on Unix would go on reading
// the unlinked inode after it had been. Opening costs microseconds, which a
// keystroke can afford; a sweep that wants every record holds one reader open
// for the whole sweep instead.
func (m *Model) withReader(i int, fn func(*marcio.RecordReader) error) error {
	if i < 0 || i >= len(m.paths) {
		return fmt.Errorf("this record does not name a file it came from")
	}
	if i >= len(m.fileInfo) || !m.fileInfo[i].known {
		return fmt.Errorf("%s was not read as either format", m.paths[i])
	}

	f, err := os.Open(m.paths[i])
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(marcio.NewRecordReader(f, m.fileInfo[i].format, m.fileInfo[i].forcedUTF8))
}

// pathIndex maps the file a batch came from to its place in the browser's
// paths. A batch with no source is the first file, which the browser reads
// itself so that -follow can bound it. A source naming no file it knows is -1:
// a record fetched from the wrong file would be worse than one that says it
// cannot be fetched at all.
func (m *Model) pathIndex(source string) int {
	if source == "" {
		return 0
	}
	for i, p := range m.paths {
		if p == source {
			return i
		}
	}
	return -1
}
