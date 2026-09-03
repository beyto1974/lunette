package marcio

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// Every record has to say where it sits, in both formats: it is what lets a
// reader come back for one record without holding the other million.
func TestExtentsLocateEveryRecord(t *testing.T) {
	for _, name := range []string{"sample.mrc", "sample.marcxml"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(testdata(t, name))
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			res, err := Load(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if len(res.Extents) != len(res.Records) {
				t.Fatalf("got %d extents for %d records", len(res.Extents), len(res.Records))
			}

			rr := NewRecordReader(bytes.NewReader(data), res.Format, res.ForcedUTF8)
			for i, ext := range res.Extents {
				if ext.Offset < 0 || ext.Length <= 0 {
					t.Fatalf("record %d has no extent: %+v", i+1, ext)
				}
				got, err := rr.At(ext)
				if err != nil {
					t.Fatalf("record %d: re-reading it failed: %v", i+1, err)
				}
				if got.String() != res.Records[i].String() {
					t.Errorf("record %d re-read differently:\n got %q\nwant %q", i+1, got.String(), res.Records[i].String())
				}
			}
		})
	}
}

// A record that makes the reader panic still has to say where it starts: the
// offset in the message is what a reader takes to a hex editor, and naming the
// record before it sends them to the wrong place.
func TestPanickingRecordReportsItsOwnOffset(t *testing.T) {
	good, err := os.ReadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// The first record is 297 bytes; a declared length below the leader's is
	// what makes gomarc allocate a negative slice.
	damaged := append(append([]byte(nil), good[:297]...), []byte("00003whatever")...)

	res, err := Load(bytes.NewReader(damaged))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(res.Issues), res.Issues)
	}
	if res.Issues[0].Offset != 297 {
		t.Errorf("issue offset = %d, want 297", res.Issues[0].Offset)
	}
}

// A tag list stands in for the record when a filter asks what fields it holds.
func TestTags(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	for i, rec := range res.Records {
		tags := Tags(rec)
		for _, f := range rec.Fields {
			if !TagsContain(tags, f.Tag) {
				t.Errorf("record %d: tags %q do not hold %q", i+1, tags, f.Tag)
			}
		}
		if TagsContain(tags, "998") {
			t.Errorf("record %d: tags %q claim a field it does not carry", i+1, tags)
		}
		if TagsContain(tags, "") {
			t.Errorf("record %d: an empty tag matched", i+1)
		}
		// A tag appears once however many times the record repeats it.
		if n := strings.Count(tags, " 650 "); n > 1 {
			t.Errorf("record %d: tags %q repeat 650 %d times", i+1, tags, n)
		}
	}
}

// The offsets of a binary file are the record boundaries a hex editor would
// show, which is what an issue reports.
func TestBinaryExtentsAreRecordBoundaries(t *testing.T) {
	res, err := LoadFile(testdata(t, "sample.mrc"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	want := []int64{0, 297, 297 + 209}
	for i, ext := range res.Extents {
		if ext.Offset != want[i] {
			t.Errorf("record %d offset = %d, want %d", i+1, ext.Offset, want[i])
		}
	}
}

// A record whose block had to be read one record at a time still knows where
// it sits, so damage does not cost the records around it their extents.
func TestExtentsSurviveDamage(t *testing.T) {
	doc := string(bigXML(t, 40))
	i := strings.Index(doc[len(doc)/2:], "<leader>") + len(doc)/2
	damaged := doc[:i] + "<leader>&nope;" + doc[i+len("<leader>"):]

	res, err := Load(strings.NewReader(damaged))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(res.Issues))
	}

	rr := NewRecordReader(strings.NewReader(damaged), res.Format, res.ForcedUTF8)
	for i, ext := range res.Extents {
		if ext.Offset < 0 {
			t.Fatalf("record %d lost its extent", i+1)
		}
		got, err := rr.At(ext)
		if err != nil {
			t.Fatalf("record %d: %v", i+1, err)
		}
		if got.String() != res.Records[i].String() {
			t.Errorf("record %d re-read differently", i+1)
		}
	}
}
