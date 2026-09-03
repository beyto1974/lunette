package marcio

import (
	"strings"
	"testing"
)

// A block ends after a complete </record>, so the block after it starts on a
// record boundary and can be handed to a decoder of its own.
func TestLastRecordEnd(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // what precedes the cut, or "" when none can be made yet
	}{
		{"one record", "<record>a</record>", "<record>a</record>"},
		{"two records", "<record>a</record><record>b</record>", "<record>a</record><record>b</record>"},
		{"trailing partial", "<record>a</record><record>b", "<record>a</record>"},
		{"no end tag yet", "<collection><record>a", ""},
		{"prefixed", "<marc:record>a</marc:record>", "<marc:record>a</marc:record>"},
		{"space before the bracket", "<record>a</record >", "<record>a</record >"},
		{"another element", "<collection><leader>a</leader>", ""},

		// The literal text </record> can only reach a file inside a comment or
		// a CDATA section - anywhere else in XML it has to be escaped - and
		// cutting there would tear a record in half.
		{"comment", "<!-- </record> --><record>a</record>", "<!-- </record> --><record>a</record>"},
		{"comment only", "<!-- </record> -->", ""},
		{"cdata", "<record><subfield><![CDATA[</record>]]></subfield></record>", "<record><subfield><![CDATA[</record>]]></subfield></record>"},
		{"cdata unclosed", "<record><![CDATA[</record>", ""},
		{"cdata then a real end", "<record><![CDATA[</record>]]>x</record><record>b", "<record><![CDATA[</record>]]>x</record>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := lastRecordEnd([]byte(tt.in))
			got := ""
			if n > 0 {
				got = tt.in[:n]
			}
			if got != tt.want {
				t.Errorf("lastRecordEnd(%q) cut %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFirstRecordStart(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"bare", "<record>a</record>", 0},
		{"after a prologue", "<?xml version='1.0'?>\n<collection>\n<record/>", 22 + 13},
		{"prefixed", "<collection><marc:record>", 12},
		{"none", "<collection>\n", -1},
		{"not a record", "<collection><recordset>", -1},
		{"inside a comment", "<!-- <record> --><record>", 17},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstRecordStart([]byte(tt.in)); got != tt.want {
				t.Errorf("firstRecordStart(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// Every record in the input has to come out in exactly one block: a splitter
// that loses one would silently shrink a harvest.
func TestBlocksHoldEveryRecord(t *testing.T) {
	const n = 500
	body := strings.Repeat("<record><leader>x</leader></record>\n", n)
	in := "<?xml version='1.0'?>\n<collection>\n" + body + "</collection>\n"

	br := newBlockReader(strings.NewReader(in), 256)
	got, blocks := 0, 0
	for {
		block, err := br.next()
		if len(block) > 0 {
			blocks++
			got += strings.Count(string(block), "<record>")
			if !strings.HasPrefix(string(block), "<record") {
				t.Fatalf("block %d does not start on a record: %.40q", blocks, block)
			}
		}
		if err != nil {
			break
		}
	}
	if got != n {
		t.Errorf("the blocks hold %d records, want %d", got, n)
	}
	if blocks < 2 {
		t.Errorf("a %d byte input split into %d block(s); the block size is not being honoured", len(in), blocks)
	}
}

// A harvest cut off mid-record must still reach a decoder, which is what turns
// it into a reported issue rather than a record that quietly vanishes.
func TestBlocksKeepATruncatedTail(t *testing.T) {
	const in = "<collection><record>a</record><record>b"

	br := newBlockReader(strings.NewReader(in), 8)
	var blocks []string
	for {
		block, err := br.next()
		if len(block) > 0 {
			blocks = append(blocks, string(block))
		}
		if err != nil {
			break
		}
	}
	if len(blocks) != 2 || blocks[1] != "<record>b" {
		t.Errorf("blocks = %q, want the truncated record kept", blocks)
	}
}

// Nothing follows the last record but the closing tag of the collection, which
// belongs to no record and would only make a decoder complain.
func TestBlocksDropTheTrailingWrapper(t *testing.T) {
	const in = "<collection><record>a</record>\n</collection>\n"

	br := newBlockReader(strings.NewReader(in), 8)
	var blocks []string
	for {
		block, err := br.next()
		if len(block) > 0 {
			blocks = append(blocks, string(block))
		}
		if err != nil {
			break
		}
	}
	if len(blocks) != 1 || blocks[0] != "<record>a</record>" {
		t.Errorf("blocks = %q, want just the record", blocks)
	}
}
