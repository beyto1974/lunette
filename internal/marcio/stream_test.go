package marcio

import (
	"os"
	"strings"
	"testing"
)

// countingReader reports how much of an input a walk actually consumed, which
// is what tells an early stop apart from a full read that threw records away.
type countingReader struct {
	r    *strings.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

func TestStreamFilesSummary(t *testing.T) {
	first, second := split(t)

	var got int
	sum, err := StreamFiles([]string{first, second}, 1, func(b Batch) error {
		got += len(b.Records)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamFiles: %v", err)
	}
	if got != 3 {
		t.Errorf("streamed %d records, want 3", got)
	}
	if sum.Records != 3 {
		t.Errorf("Summary.Records = %d, want 3", sum.Records)
	}
	if sum.Format != FormatBinary {
		t.Errorf("Summary.Format = %v, want FormatBinary", sum.Format)
	}
	if sum.MixedFormats {
		t.Error("Summary.MixedFormats set for two binary files")
	}
	if len(sum.Issues) != 0 {
		t.Errorf("Summary.Issues = %v, want none", sum.Issues)
	}
}

// StreamFiles must number records across the whole set exactly as LoadFiles
// does, or an issue reported against "record 3" would mean two things.
func TestStreamFilesContinuesOrdinals(t *testing.T) {
	first, second := split(t)

	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	copy(data[209:214], "XXXXX")
	if err := os.WriteFile(second, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	want, err := LoadFiles([]string{first, second})
	if err != nil {
		t.Fatalf("LoadFiles: %v", err)
	}
	sum, err := StreamFiles([]string{first, second}, 1, func(Batch) error { return nil })
	if err != nil {
		t.Fatalf("StreamFiles: %v", err)
	}

	if len(sum.Issues) != len(want.Issues) {
		t.Fatalf("got %d issues, want %d", len(sum.Issues), len(want.Issues))
	}
	for i, issue := range sum.Issues {
		if issue.Ordinal != want.Issues[i].Ordinal || issue.Source != want.Issues[i].Source {
			t.Errorf("issue %d = %+v, want %+v", i, issue, want.Issues[i])
		}
	}
	if sum.Records != len(want.Records) {
		t.Errorf("Summary.Records = %d, want %d", sum.Records, len(want.Records))
	}
}

func TestStreamFilesMixedFormats(t *testing.T) {
	sum, err := StreamFiles([]string{testdata(t, "sample.mrc"), testdata(t, "sample.marcxml")}, 2, func(Batch) error { return nil })
	if err != nil {
		t.Fatalf("StreamFiles: %v", err)
	}
	if !sum.MixedFormats {
		t.Error("MixedFormats not reported for a binary and an XML file")
	}
	if sum.Records != 6 {
		t.Errorf("Summary.Records = %d, want 6", sum.Records)
	}
}

func TestStreamFilesMissing(t *testing.T) {
	_, err := StreamFiles([]string{testdata(t, "sample.mrc"), "no-such-file.mrc"}, 8, func(Batch) error { return nil })
	if err == nil {
		t.Fatal("StreamFiles accepted a missing file")
	}
	if !strings.Contains(err.Error(), "no-such-file.mrc") {
		t.Errorf("error = %q, want it to name the file", err)
	}
}

func TestStreamFilesEmpty(t *testing.T) {
	if _, err := StreamFiles(nil, 8, func(Batch) error { return nil }); err == nil {
		t.Error("StreamFiles accepted no files at all")
	}
}

// ErrStop is how a caller that has seen enough - `show -n 1` on a harvest of
// millions - stops the walk without the walk calling it a failure.
func TestStreamStopsEarly(t *testing.T) {
	one, err := os.ReadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Binary MARC21 concatenates, so a long input is three records repeated.
	// It has to outgrow the sniffing buffer, or the first Peek would read the
	// whole file and an early stop would save nothing measurable.
	data := strings.Repeat(string(one), 200)
	cr := &countingReader{r: strings.NewReader(data)}

	seen := 0
	if err := Stream(cr, 1, func(b Batch) error {
		seen += len(b.Records)
		return ErrStop
	}); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if seen != 1 {
		t.Errorf("streamed %d records before stopping, want 1", seen)
	}
	if cr.read >= len(data) {
		t.Errorf("read %d of %d bytes; an early stop must not read the whole input", cr.read, len(data))
	}
}

func TestStreamFilesStopsEarly(t *testing.T) {
	first, second := split(t)

	seen := 0
	sum, err := StreamFiles([]string{first, second}, 1, func(b Batch) error {
		seen += len(b.Records)
		return ErrStop
	})
	if err != nil {
		t.Fatalf("StreamFiles: %v", err)
	}
	if seen != 1 {
		t.Errorf("streamed %d records, want 1", seen)
	}
	if !sum.Stopped {
		t.Error("Summary.Stopped not set after an early stop")
	}
}

// A failure from the callback is the caller's own, and must not be swallowed.
func TestStreamFilesReportsCallbackError(t *testing.T) {
	boom := errFake("boom")
	_, err := StreamFiles([]string{testdata(t, "sample.mrc")}, 1, func(Batch) error { return boom })
	if err != boom {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
