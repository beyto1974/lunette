package marcio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"

	marc "github.com/beyto1974/gomarc"
)

// Decoding MARCXML on more than one core.
//
// encoding/xml is where nearly two thirds of the time reading a MARCXML
// harvest goes, and it has no fast path worth finding: it decodes a character
// at a time. Since a MARCXML document is a flat list of records, it can be cut
// into blocks of whole records - see xmlblocks.go - and each block given to a
// decoder of its own. Records come back in file order regardless, because the
// blocks are queued in order and each one is waited for in turn.

// xmlBlockSize is how much of the file one decoder is given. It is a trade
// between having enough work per block to be worth dispatching and how much
// undecoded and decoded input sits in memory at once.
const xmlBlockSize = 1 << 20

// maxXMLWorkers caps the concurrency. Past this the gain flattens - the work
// is as much memory traffic as arithmetic - while the input and records in
// flight keep growing.
const maxXMLWorkers = 8

func xmlWorkers() int {
	n := runtime.GOMAXPROCS(0)
	if n > maxXMLWorkers {
		n = maxXMLWorkers
	}
	if n < 1 {
		n = 1
	}
	return n
}

// errFatal ends a walk. It marks a failure the reader cannot get past, as
// against a record that merely failed to decode.
var errFatal = errors.New("cannot continue")

// decoded is one attempt at a record: what came out, or what went wrong. Both
// are carried so that the failures keep their place in the file, which is what
// an issue's ordinal reports.
type decoded struct {
	rec *marc.Record
	err error
}

// parallelXML is a source that decodes blocks of MARCXML concurrently.
type parallelXML struct {
	futures chan chan []decoded
	current []decoded
	pos     int
	done    chan struct{}
	closed  bool
}

func newParallelXML(r io.Reader, workers, blockSize int) *parallelXML {
	p := &parallelXML{
		futures: make(chan chan []decoded, workers),
		done:    make(chan struct{}),
	}
	go p.dispatch(newBlockReader(r, blockSize), workers)
	return p
}

// dispatch cuts the input into blocks and starts a decoder for each. A block's
// future is queued before its decoder starts, which is what keeps the records
// in file order however the decoders finish.
func (p *parallelXML) dispatch(br *blockReader, workers int) {
	defer close(p.futures)

	running := make(chan struct{}, workers)
	for {
		block, err := br.next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				p.queue([]decoded{{err: fmt.Errorf("%w: %v", errFatal, err)}})
			}
			return
		}

		ch := make(chan []decoded, 1)
		if !p.send(ch) {
			return
		}
		select {
		case running <- struct{}{}:
		case <-p.done:
			return
		}
		go func(block []byte) {
			defer func() { <-running }()
			ch <- decodeBlock(block)
		}(block)
	}
}

// queue hands the reader a result that needed no decoder.
func (p *parallelXML) queue(items []decoded) {
	ch := make(chan []decoded, 1)
	ch <- items
	p.send(ch)
}

// send queues a future, or reports that the reader has gone away.
func (p *parallelXML) send(ch chan []decoded) bool {
	select {
	case p.futures <- ch:
		return true
	case <-p.done:
		return false
	}
}

func (p *parallelXML) next() (*marc.Record, error) {
	for p.pos >= len(p.current) {
		ch, ok := <-p.futures
		if !ok {
			return nil, io.EOF
		}
		p.current, p.pos = <-ch, 0
	}
	d := p.current[p.pos]
	p.pos++
	return d.rec, d.err
}

// size is unknown for MARCXML: a record's offset in the file is not something
// the decoder reports, and it means little in a document anyway.
func (p *parallelXML) size() int64 { return offsetUnknown }

// close stops the decoders. A walk that ends early - a filter that has seen
// enough, a fatal error - would otherwise leave them blocked on a queue nobody
// is reading.
func (p *parallelXML) close() {
	if !p.closed {
		p.closed = true
		close(p.done)
	}
}

// decodeBlock reads every record in one block.
//
// A block is decoded in one go, which is the fast path. When that fails the
// block is read again a record at a time: an encoding/xml decoder cannot
// continue past a syntax error - it returns the same one for ever - so the
// only way past damage is a new decoder on the next record. That costs
// nothing on a clean file and confines the damage to the record holding it.
func decodeBlock(block []byte) []decoded {
	out, err := decodeAll(block)
	if err == nil {
		return out
	}
	var each []decoded
	for _, rec := range splitRecords(block) {
		items, _ := decodeAll(rec)
		if len(items) == 0 {
			// A span that yielded neither a record nor a failure would make
			// the record numbering drift, so it is a failure.
			items = []decoded{{err: err}}
		}
		each = append(each, items...)
	}
	return each
}

// decodeAll reads records until the input ends, reporting the first failure.
// Anything decoded before it is kept: those records are sound.
func decodeAll(buf []byte) ([]decoded, error) {
	src := xmlSource{rd: marc.NewXMLReader(bytes.NewReader(buf))}
	var out []decoded
	for {
		rec, err := safeNext(src)
		if errors.Is(err, io.EOF) || (err == nil && rec == nil) {
			return out, nil
		}
		if err != nil {
			return append(out, decoded{err: err}), err
		}
		out = append(out, decoded{rec: rec})
	}
}

// splitRecords cuts a block into its <record> elements. What sits between them
// belongs to no record and is dropped; a record the block stops in the middle
// of is kept, so that it is reported rather than lost.
func splitRecords(block []byte) [][]byte {
	var out [][]byte
	for {
		start := firstRecordStart(block)
		if start < 0 {
			return out
		}
		block = block[start:]
		end := firstRecordEnd(block)
		if end <= 0 {
			return append(out, block)
		}
		out = append(out, block[:end])
		block = block[end:]
	}
}
