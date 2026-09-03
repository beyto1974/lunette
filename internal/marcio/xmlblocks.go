package marcio

import (
	"bytes"
	"io"
)

// Cutting MARCXML into blocks of whole records.
//
// A MARCXML document is a flat list of <record> elements, so it can be cut
// wherever one ends and handed to as many decoders as there are cores. The cut
// has to survive what real harvests contain, which is why this is a scan and
// not a call to bytes.LastIndex: the literal text </record> can reach a file
// inside a comment or a CDATA section, where it closes nothing.

var (
	commentOpen  = []byte("<!--")
	commentClose = []byte("-->")
	cdataOpen    = []byte("<![CDATA[")
	cdataClose   = []byte("]]>")
)

// lastRecordEnd returns the offset just past the last complete </record> in
// buf, or 0 if it holds none. buf must begin outside markup.
func lastRecordEnd(buf []byte) int { return recordEnd(buf, false) }

// firstRecordEnd returns the offset just past the first complete </record>.
func firstRecordEnd(buf []byte) int { return recordEnd(buf, true) }

// recordEnd finds a record's closing tag, the first or the last, skipping the
// comments and CDATA sections in which the same text closes nothing.
func recordEnd(buf []byte, first bool) int {
	last := 0
	for i := 0; i < len(buf); {
		if buf[i] != '<' {
			i++
			continue
		}
		if skip, ok := skipHidden(buf, i); ok {
			if skip < 0 {
				// An unterminated comment or CDATA section: nothing after it
				// can be read as markup, so there is no safe cut beyond here.
				return last
			}
			i = skip
			continue
		}
		if !bytes.HasPrefix(buf[i:], []byte("</")) {
			i++
			continue
		}
		close := bytes.IndexByte(buf[i:], '>')
		if close < 0 {
			return last
		}
		end := i + close + 1
		if localName(buf[i+2:i+close]) == "record" {
			last = end
			if first {
				return last
			}
		}
		i = end
	}
	return last
}

// firstRecordStart returns the offset of the first <record> start tag in buf,
// or -1. It is what skips a document's prologue - the XML declaration and the
// opening <collection> - which belongs to no record.
func firstRecordStart(buf []byte) int {
	for i := 0; i < len(buf); {
		if buf[i] != '<' {
			i++
			continue
		}
		if skip, ok := skipHidden(buf, i); ok {
			if skip < 0 {
				return -1
			}
			i = skip
			continue
		}
		// An end tag, a declaration or a processing instruction: skip it
		// whole, since none of them starts a record.
		if bytes.HasPrefix(buf[i:], []byte("</")) || bytes.HasPrefix(buf[i:], []byte("<!")) || bytes.HasPrefix(buf[i:], []byte("<?")) {
			close := bytes.IndexByte(buf[i:], '>')
			if close < 0 {
				return -1
			}
			i += close + 1
			continue
		}
		name := i + 1
		end := name
		for end < len(buf) && !nameEnds(buf[end]) {
			end++
		}
		if end == len(buf) {
			return -1
		}
		if localName(buf[name:end]) == "record" {
			return i
		}
		i = end
	}
	return -1
}

// skipHidden reports whether a '<' at i opens a comment or a CDATA section,
// and where it ends. A negative offset means it never does.
func skipHidden(buf []byte, i int) (int, bool) {
	for _, pair := range []struct{ open, close []byte }{
		{commentOpen, commentClose},
		{cdataOpen, cdataClose},
	} {
		if !bytes.HasPrefix(buf[i:], pair.open) {
			continue
		}
		from := i + len(pair.open)
		end := bytes.Index(buf[from:], pair.close)
		if end < 0 {
			return -1, true
		}
		return from + end + len(pair.close), true
	}
	return 0, false
}

// nameEnds reports the characters that end an element name.
func nameEnds(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '>' || c == '/'
}

// localName drops any namespace prefix and surrounding space from an element
// name, so that <marc:record> and <record> read alike.
func localName(name []byte) string {
	name = bytes.TrimSpace(name)
	if i := bytes.IndexByte(name, ':'); i >= 0 {
		name = name[i+1:]
	}
	return string(name)
}

// blockReader cuts a MARCXML stream into blocks of whole records. Each block
// starts at a <record> and ends after a </record>, so it decodes on its own;
// the last one may hold a record the harvest was cut off in the middle of,
// which is passed on rather than dropped so that a decoder can report it.
type blockReader struct {
	r       io.Reader
	size    int
	pending []byte
	started bool
	eof     bool
	spent   bool
}

func newBlockReader(r io.Reader, size int) *blockReader {
	if size < 1 {
		size = 1
	}
	return &blockReader{r: r, size: size}
}

// next returns the next block, or io.EOF once the input is spent.
func (b *blockReader) next() ([]byte, error) {
	if b.spent {
		return nil, io.EOF
	}
	if !b.started {
		if err := b.skipPrologue(); err != nil {
			return nil, err
		}
	}

	for {
		if err := b.fill(b.size); err != nil {
			return nil, err
		}
		if end := lastRecordEnd(b.pending); end > 0 {
			block := b.pending[:end]
			// The newline that follows a record belongs to no record; leaving
			// it on the front of the next block would only make "starts at a
			// record" untrue.
			b.pending = bytes.TrimLeft(b.pending[end:], " \t\r\n")
			return block, nil
		}
		if b.eof {
			// What is left is either a record the file stops in the middle of
			// or the closing tag of the collection. The first has to reach a
			// decoder; the second belongs to nothing.
			tail := b.pending
			b.pending, b.spent = nil, true
			if firstRecordStart(tail) < 0 {
				return nil, io.EOF
			}
			return tail, nil
		}
		// No record ended within a block's worth of input: a single record
		// larger than the block size. Keep reading until one does.
		if err := b.fill(len(b.pending) + b.size); err != nil {
			return nil, err
		}
	}
}

// skipPrologue drops everything before the first record.
func (b *blockReader) skipPrologue() error {
	for {
		if err := b.fill(len(b.pending) + b.size); err != nil {
			return err
		}
		if i := firstRecordStart(b.pending); i >= 0 {
			b.pending = b.pending[i:]
			b.started = true
			return nil
		}
		if b.eof {
			b.pending, b.spent = nil, true
			return io.EOF
		}
	}
}

// fill reads until pending holds want bytes or the input ends.
func (b *blockReader) fill(want int) error {
	for !b.eof && len(b.pending) < want {
		need := want - len(b.pending)
		if need < b.size {
			need = b.size
		}
		start := len(b.pending)
		b.pending = append(b.pending, make([]byte, need)...)
		n, err := io.ReadFull(b.r, b.pending[start:])
		b.pending = b.pending[:start+n]
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			b.eof = true
		case err != nil:
			return err
		}
	}
	return nil
}
