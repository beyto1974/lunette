package marcio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	marc "github.com/beyto1974/gomarc"
)

// bigXML builds a MARCXML document holding n copies of the fixture's records,
// which is enough input to be cut into several blocks.
func bigXML(t *testing.T, n int) []byte {
	t.Helper()
	data, err := os.ReadFile(testdata(t, "sample.marcxml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	body := string(data)
	start := strings.Index(body, "<record")
	end := strings.LastIndex(body, "</record>") + len("</record>")
	if start < 0 || end < start {
		t.Fatalf("the fixture does not look like MARCXML")
	}

	var b strings.Builder
	b.WriteString("<?xml version='1.0' encoding='UTF-8'?>\n<collection xmlns=\"http://www.loc.gov/MARC21/slim\">\n")
	for i := 0; i < n; i++ {
		b.WriteString(body[start:end])
		b.WriteString("\n")
	}
	b.WriteString("</collection>\n")
	return []byte(b.String())
}

// breaker is how a record compares to another: gomarc's pymarc-style dump of
// everything the record holds.
func breaker(recs []*marc.Record) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.String()
	}
	return out
}

// decodeVia reads a document through the block splitter with the given
// concurrency, which is how a test gets many blocks out of a small fixture.
func decodeVia(t *testing.T, doc []byte, workers, blockSize int) ([]*marc.Record, []error) {
	t.Helper()
	p := newParallelXML(bytes.NewReader(doc), workers, blockSize)
	defer p.close()

	var recs []*marc.Record
	var errs []error
	for {
		rec, err := p.next()
		if errors.Is(err, io.EOF) || (err == nil && rec == nil) {
			return recs, errs
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		recs = append(recs, rec)
	}
}

// Decoding blocks in parallel must give exactly what one decoder reading the
// whole document gives, in the same order. gomarc's own reader is the
// reference, since it is the thing being parallelised. The block size is small
// so that a fixture-sized document is cut many times over.
func TestParallelXMLMatchesOneDecoder(t *testing.T) {
	doc := bigXML(t, 400)

	want, err := marc.ParseXML(bytes.NewReader(doc))
	if err != nil {
		t.Fatalf("ParseXML: %v", err)
	}
	wantStr := breaker(want)

	for _, workers := range []int{1, 2, 8} {
		for _, size := range []int{512, 4 << 10, 1 << 20} {
			recs, errs := decodeVia(t, doc, workers, size)
			if len(errs) != 0 {
				t.Fatalf("workers=%d block=%d: clean input failed: %v", workers, size, errs)
			}
			got := breaker(recs)
			if len(got) != len(wantStr) {
				t.Fatalf("workers=%d block=%d: decoded %d records, want %d", workers, size, len(got), len(wantStr))
			}
			for i := range got {
				if got[i] != wantStr[i] {
					t.Fatalf("workers=%d block=%d: record %d differs:\n got %q\nwant %q", workers, size, i+1, got[i], wantStr[i])
				}
			}
		}
	}
}

// Damage inside one block must not cost the blocks around it, whatever the
// cut happens to fall on.
func TestDamageIsConfinedToItsBlock(t *testing.T) {
	doc := string(bigXML(t, 60))
	i := strings.Index(doc[len(doc)/2:], "<leader>") + len(doc)/2
	damaged := []byte(doc[:i] + "<leader>&nope;" + doc[i+len("<leader>"):])

	for _, size := range []int{512, 4 << 10, 1 << 20} {
		recs, errs := decodeVia(t, damaged, 4, size)
		if len(errs) != 1 {
			t.Errorf("block=%d: got %d failures, want 1: %v", size, len(errs), errs)
		}
		if want := 180 - 1; len(recs) != want {
			t.Errorf("block=%d: decoded %d records, want %d", size, len(recs), want)
		}
	}
}

// A record that was never closed - a harvest interrupted, a repository that
// writes a record in pieces - costs itself and nothing else. Taking the next
// record's closing tag as its own would swallow that record silently, which is
// the one thing this must not do.
func TestUnterminatedRecordKeepsItsNeighbours(t *testing.T) {
	const doc = `<collection>
<record><leader>     nam  22        4500</leader><datafield tag="245" ind1="1" ind2="0"><subfield code="a">One</subfield></datafield></record>
<record><leader>     nam  22        4500</leader><datafield tag="245" ind1="1" ind2="0"><subfield code="a">Two</subfield></datafield>
<record><leader>     nam  22        4500</leader><datafield tag="245" ind1="1" ind2="0"><subfield code="a">Three</subfield></datafield></record>
<record><leader>     nam  22        4500</leader><datafield tag="245" ind1="1" ind2="0"><subfield code="a">Four</subfield></datafield></record>
</collection>`

	res, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var titles []string
	for _, rec := range res.Records {
		titles = append(titles, Title(rec))
	}
	want := []string{"One", "Three", "Four"}
	if len(titles) != len(want) {
		t.Fatalf("decoded %q, want %q", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Errorf("record %d = %q, want %q", i+1, titles[i], want[i])
		}
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(res.Issues), res.Issues)
	}
	// The unterminated record is the second in the file, and the records after
	// it keep their own numbers.
	if res.Issues[0].Ordinal != 2 {
		t.Errorf("issue ordinal = %d, want 2", res.Issues[0].Ordinal)
	}
}

// A comment or a CDATA section can hold the literal text </record>, which
// closes nothing. Cutting there would split a record in half.
func TestParallelXMLWithHiddenEndTags(t *testing.T) {
	const doc = `<?xml version='1.0'?>
<collection>
<!-- </record> a comment that lies -->
<record><leader>     nam  22        4500</leader>
<datafield tag="245" ind1="1" ind2="0"><subfield code="a"><![CDATA[</record>]]></subfield></datafield>
</record>
<record><leader>     nam  22        4500</leader>
<datafield tag="245" ind1="1" ind2="0"><subfield code="a">second</subfield></datafield>
</record>
</collection>`

	res, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues: %v", res.Issues)
	}
	if len(res.Records) != 2 {
		t.Fatalf("decoded %d records, want 2", len(res.Records))
	}
	if got := Title(res.Records[0]); got != "</record>" {
		t.Errorf("first title = %q, want the CDATA text", got)
	}
	if got := Title(res.Records[1]); got != "second" {
		t.Errorf("second title = %q, want second", got)
	}
}

// Damaged MARCXML used to hang: an encoding/xml decoder returns the same
// syntax error for ever, so the loop recorded an issue and asked again,
// without end. It has to stop, and say what it found.
func TestDamagedXMLTerminates(t *testing.T) {
	const doc = `<collection><record><leader>x</leader></record><record><leader>&broken</record></collection>`

	done := make(chan *Result, 1)
	go func() {
		res, err := Load(strings.NewReader(doc))
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()

	select {
	case res := <-done:
		if len(res.Issues) == 0 {
			t.Error("a syntax error was not reported")
		}
		if len(res.Issues) > 4 {
			t.Errorf("reported %d issues for one damaged record", len(res.Issues))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Load never returned on damaged MARCXML")
	}
}

// A record that fails to decode must not take the rest of the file with it:
// the block it sits in is re-read one record at a time, so the damage costs
// one record and the ordinals stay true.
func TestDamagedXMLKeepsTheRestOfTheFile(t *testing.T) {
	doc := string(bigXML(t, 100))
	// Break the leader of one record in the middle of the document.
	const marker = "<leader>"
	i := strings.Index(doc[len(doc)/2:], marker) + len(doc)/2
	damaged := doc[:i] + "<leader>&nope;" + doc[i+len(marker):]

	res, err := Load(strings.NewReader(damaged))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(res.Issues), res.Issues)
	}
	if want := 300 - 1; len(res.Records) != want {
		t.Errorf("decoded %d records, want %d: one damaged record must not lose its neighbours", len(res.Records), want)
	}
	// The ordinal counts every record attempted, so it says where the damage
	// is in the file rather than where it is in the output.
	if res.Issues[0].Ordinal <= 1 || res.Issues[0].Ordinal >= 300 {
		t.Errorf("issue ordinal = %d, want it inside the file", res.Issues[0].Ordinal)
	}
}
