package marcio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// LeaderLength is the fixed size of a MARC21 leader.
const LeaderLength = 24

// codingScheme is leader/09: 'a' means UTF-8, blank means MARC-8.
const codingScheme = 9

// maxExamples caps the record numbers listed for a category, so a report on a
// file where every record is mislabelled stays readable.
const maxExamples = 10

// EncodingReport describes what a file's bytes and leaders say about its
// character encoding, and whether they agree.
//
// The analysis reads raw record bytes rather than decoded records: MARC-8
// escape sequences and invalid UTF-8 are exactly what a decoder removes, so by
// the time a record is parsed the evidence is gone.
type EncodingReport struct {
	Format  Format
	Records int

	// DeclaredUTF8 and DeclaredMARC8 count leader/09.
	DeclaredUTF8  int
	DeclaredMARC8 int

	// WithNonASCII counts records holding any byte above 0x7F, the only ones
	// whose encoding can be told apart.
	WithNonASCII int
	// WithEscapes counts records containing an ESC, which is how MARC-8
	// announces a character set: strong evidence the record really is MARC-8.
	WithEscapes int
	// InvalidUTF8 counts records with non-ASCII bytes that do not form valid
	// UTF-8 and carry no escape sequences - usually Latin-1, which neither
	// decoder reads correctly.
	InvalidUTF8 int

	// Mismatched lists the 1-based numbers of records that declare MARC-8 but
	// hold valid multi-byte UTF-8, capped at maxExamples; MismatchedTotal is
	// how many there really are.
	Mismatched      []int
	MismatchedTotal int
	// Truncated counts records whose declared length ran past the end of the
	// file.
	Truncated int
}

// Conflict reports whether the file's bytes contradict its leaders, which is
// the condition the loader silently corrects.
func (r *EncodingReport) Conflict() bool {
	return r.MismatchedTotal > 0 && r.WithEscapes == 0
}

// Verdict is a one-line summary in plain words.
func (r *EncodingReport) Verdict() string {
	switch {
	case r.Format == FormatXML:
		return "MARCXML, which is UTF-8 by definition; leaders are not consulted"
	case r.Records == 0:
		return "no records to judge"
	case r.WithEscapes > 0:
		return fmt.Sprintf("genuine MARC-8: %d record(s) carry escape sequences", r.WithEscapes)
	case r.Conflict():
		return fmt.Sprintf("UTF-8 bytes behind a MARC-8 leader in %d record(s); marcview decodes them as UTF-8, "+
			"other tools will not", r.MismatchedTotal)
	case r.InvalidUTF8 > 0:
		return fmt.Sprintf("%d record(s) hold bytes that are neither ASCII nor valid UTF-8, probably Latin-1", r.InvalidUTF8)
	case r.WithNonASCII == 0:
		return "pure ASCII: the declared encoding makes no difference"
	default:
		return "consistent: the bytes match what the leaders declare"
	}
}

// AnalyzeEncoding examines a MARC21 or MARCXML file.
func AnalyzeEncoding(r io.Reader) (*EncodingReport, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	peek, err := br.Peek(64)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	rep := &EncodingReport{Format: DetectFormat(peek)}
	switch rep.Format {
	case FormatXML:
		return rep, analyzeXML(br, rep)
	case FormatBinary:
		return rep, analyzeBinary(br, rep)
	default:
		return nil, fmt.Errorf("unrecognised format: not binary MARC21 or MARCXML")
	}
}

// AnalyzeEncodingFile examines the file at path.
func AnalyzeEncodingFile(path string) (*EncodingReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return AnalyzeEncoding(f)
}

// analyzeXML counts records and non-ASCII content. A MARCXML document declares
// its own encoding, so there is no leader to reconcile.
func analyzeXML(r io.Reader, rep *EncodingReport) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	rep.Records = bytes.Count(data, []byte("<record"))
	if !isASCII(data) {
		rep.WithNonASCII = 1 // per document, not per record
		if !utf8.Valid(data) {
			rep.InvalidUTF8 = 1
		}
	}
	return nil
}

// analyzeBinary walks the transmission format by record length, which needs no
// decoding and so survives records gomarc would reject.
func analyzeBinary(r io.Reader, rep *EncodingReport) error {
	for {
		header := make([]byte, 5)
		n, err := io.ReadFull(r, header)
		if n == 0 && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			return nil
		}
		if err != nil {
			rep.Truncated++
			return nil
		}

		length, convErr := strconv.Atoi(string(header))
		if convErr != nil || length < LeaderLength {
			rep.Truncated++
			return nil
		}

		rest := make([]byte, length-5)
		n, err = io.ReadFull(r, rest)
		if err != nil {
			rep.Truncated++
			return nil
		}
		rep.inspect(append(header, rest[:n]...))
	}
}

// inspect classifies one raw record.
func (r *EncodingReport) inspect(record []byte) {
	r.Records++

	declaresUTF8 := len(record) > codingScheme && record[codingScheme] == 'a'
	if declaresUTF8 {
		r.DeclaredUTF8++
	} else {
		r.DeclaredMARC8++
	}

	body := record[min(LeaderLength, len(record)):]

	// Escape sequences are themselves ASCII, so they have to be looked for
	// before the ASCII shortcut: a MARC-8 record whose text happens to be
	// plain ASCII still announces its character sets.
	hasEscape := bytes.IndexByte(body, 0x1b) >= 0
	if hasEscape {
		r.WithEscapes++
	}
	if isASCII(body) {
		return
	}
	r.WithNonASCII++

	switch {
	case hasEscape:
		// Already counted; the record is MARC-8 as declared.
	case !utf8.Valid(body):
		r.InvalidUTF8++
	case !declaresUTF8:
		r.MismatchedTotal++
		if len(r.Mismatched) < maxExamples {
			r.Mismatched = append(r.Mismatched, r.Records)
		}
	}
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// String renders the report for a terminal.
func (r *EncodingReport) String() string {
	var b strings.Builder
	const field = "%-28s%d\n"
	fmt.Fprintf(&b, "%-28s%s\n", "format:", r.Format)
	fmt.Fprintf(&b, field, "records:", r.Records)

	if r.Format == FormatBinary {
		fmt.Fprintf(&b, field, "leader/09 says utf-8:", r.DeclaredUTF8)
		fmt.Fprintf(&b, field, "leader/09 says marc-8:", r.DeclaredMARC8)
	}
	fmt.Fprintf(&b, field, "records w/ non-ascii:", r.WithNonASCII)
	fmt.Fprintf(&b, field, "records w/ marc-8 escapes:", r.WithEscapes)
	fmt.Fprintf(&b, field, "records w/ invalid utf-8:", r.InvalidUTF8)
	if r.Truncated > 0 {
		fmt.Fprintf(&b, field, "truncated records:", r.Truncated)
	}
	if r.MismatchedTotal > 0 {
		fmt.Fprintf(&b, field, "mislabelled records:", r.MismatchedTotal)
		fmt.Fprintf(&b, "%-28s%s", "  for example:", joinInts(r.Mismatched))
		if r.MismatchedTotal > len(r.Mismatched) {
			fmt.Fprintf(&b, ", … (%d more)", r.MismatchedTotal-len(r.Mismatched))
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\n%s\n", r.Verdict())
	return b.String()
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}
