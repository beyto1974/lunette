package marcio

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return b
}

// relabel rewrites leader/09 of every record in a binary MARC file.
func relabel(t *testing.T, data []byte, scheme byte) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	for off := 0; off+LeaderLength <= len(out); {
		length := 0
		for i := 0; i < 5; i++ {
			length = length*10 + int(out[off+i]-'0')
		}
		if length <= 0 {
			break
		}
		out[off+9] = scheme
		off += length
	}
	return out
}

// A file whose leaders and bytes agree needs no explaining.
func TestAnalyzeEncodingConsistent(t *testing.T) {
	rep, err := AnalyzeEncoding(bytes.NewReader(fixture(t, "sample.mrc")))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}

	if rep.Format != FormatBinary {
		t.Errorf("Format = %v, want MARC21", rep.Format)
	}
	if rep.Records != 3 {
		t.Errorf("Records = %d, want 3", rep.Records)
	}
	if rep.DeclaredUTF8 != 3 || rep.DeclaredMARC8 != 0 {
		t.Errorf("declared utf-8/marc-8 = %d/%d, want 3/0", rep.DeclaredUTF8, rep.DeclaredMARC8)
	}
	if rep.WithNonASCII != 1 {
		t.Errorf("WithNonASCII = %d, want 1 (the café record)", rep.WithNonASCII)
	}
	if rep.WithEscapes != 0 {
		t.Errorf("WithEscapes = %d, want 0", rep.WithEscapes)
	}
	if len(rep.Mismatched) != 0 {
		t.Errorf("Mismatched = %v, want none", rep.Mismatched)
	}
	if rep.Conflict() {
		t.Error("Conflict() is true for a consistent file")
	}
}

// The case this tool exists for: UTF-8 bytes behind a MARC-8 leader.
func TestAnalyzeEncodingMislabeled(t *testing.T) {
	data := relabel(t, fixture(t, "sample.mrc"), ' ')

	rep, err := AnalyzeEncoding(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}
	if rep.DeclaredMARC8 != 3 {
		t.Errorf("DeclaredMARC8 = %d, want 3", rep.DeclaredMARC8)
	}
	if !rep.Conflict() {
		t.Error("Conflict() is false for UTF-8 bytes behind a MARC-8 leader")
	}
	// Only the record carrying non-ASCII can be shown to be mislabelled.
	if len(rep.Mismatched) != 1 || rep.Mismatched[0] != 2 {
		t.Errorf("Mismatched = %v, want the second record", rep.Mismatched)
	}
	if rep.MismatchedTotal != 1 {
		t.Errorf("MismatchedTotal = %d, want 1", rep.MismatchedTotal)
	}
	if !strings.Contains(strings.ToLower(rep.Verdict()), "utf-8") {
		t.Errorf("Verdict = %q, want it to name the encoding", rep.Verdict())
	}
}

// A record using real MARC-8 escape sequences must not be reported as
// mislabelled UTF-8.
func TestAnalyzeEncodingGenuineMARC8(t *testing.T) {
	data := fixture(t, "sample.mrc")
	// Replace two ASCII bytes of the first record with a MARC-8 escape
	// sequence: ESC ( B selects the ASCII graphic set.
	marc8 := append([]byte(nil), data...)
	i := bytes.Index(marc8, []byte("Neirinck"))
	if i < 0 {
		t.Fatal("fixture changed")
	}
	copy(marc8[i:], []byte{0x1b, '(', 'B'})
	marc8 = relabel(t, marc8, ' ')

	rep, err := AnalyzeEncoding(bytes.NewReader(marc8))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}
	if rep.WithEscapes != 1 {
		t.Errorf("WithEscapes = %d, want 1", rep.WithEscapes)
	}
	if rep.Conflict() {
		t.Error("a file containing MARC-8 escapes should not be called mislabelled UTF-8")
	}
}

// Bytes that are neither ASCII nor valid UTF-8 are worth flagging: they are
// probably Latin-1, which neither decoder handles correctly.
func TestAnalyzeEncodingInvalidUTF8(t *testing.T) {
	data := fixture(t, "sample.mrc")
	latin1 := append([]byte(nil), data...)
	i := bytes.Index(latin1, []byte("Ada"))
	if i < 0 {
		t.Fatal("fixture changed")
	}
	latin1[i] = 0xe9 // é in Latin-1, invalid on its own in UTF-8

	rep, err := AnalyzeEncoding(bytes.NewReader(latin1))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}
	if rep.InvalidUTF8 != 1 {
		t.Errorf("InvalidUTF8 = %d, want 1", rep.InvalidUTF8)
	}
}

// MARCXML is UTF-8 by definition, so there is nothing to reconcile.
func TestAnalyzeEncodingXML(t *testing.T) {
	rep, err := AnalyzeEncoding(bytes.NewReader(fixture(t, "sample.marcxml")))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}
	if rep.Format != FormatXML {
		t.Errorf("Format = %v, want MARCXML", rep.Format)
	}
	if rep.Conflict() {
		t.Error("MARCXML cannot conflict with its own leaders")
	}
	if !strings.Contains(rep.Verdict(), "MARCXML") {
		t.Errorf("Verdict = %q, want it to mention MARCXML", rep.Verdict())
	}
}

// A truncated record is reported rather than silently ending the walk.
func TestAnalyzeEncodingTruncated(t *testing.T) {
	data := fixture(t, "sample.mrc")
	rep, err := AnalyzeEncoding(bytes.NewReader(data[:len(data)-40]))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}
	if rep.Truncated != 1 {
		t.Errorf("Truncated = %d, want 1", rep.Truncated)
	}
	if rep.Records != 2 {
		t.Errorf("Records = %d, want the 2 complete records", rep.Records)
	}
}

// The listed record numbers are capped, but the count must be the real one:
// reporting the cap as the total understates the problem.
func TestAnalyzeEncodingCountsBeyondTheCap(t *testing.T) {
	one := fixture(t, "sample.mrc")
	// The second record is the one carrying non-ASCII; repeat the whole file
	// until well past the example cap.
	var many []byte
	for i := 0; i < maxExamples+5; i++ {
		many = append(many, one...)
	}
	many = relabel(t, many, ' ')

	rep, err := AnalyzeEncoding(bytes.NewReader(many))
	if err != nil {
		t.Fatalf("AnalyzeEncoding: %v", err)
	}
	if len(rep.Mismatched) != maxExamples {
		t.Errorf("listed %d examples, want the cap of %d", len(rep.Mismatched), maxExamples)
	}
	if rep.MismatchedTotal != maxExamples+5 {
		t.Errorf("MismatchedTotal = %d, want %d", rep.MismatchedTotal, maxExamples+5)
	}
	if !strings.Contains(rep.Verdict(), strconv.Itoa(maxExamples+5)) {
		t.Errorf("verdict quotes the cap rather than the total: %q", rep.Verdict())
	}
}
