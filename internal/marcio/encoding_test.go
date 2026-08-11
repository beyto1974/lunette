package marcio

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestLooksUTF8(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"pure ascii is not evidence either way", []byte("plain ascii record"), false},
		{"valid utf-8 with accents", []byte("données café"), true},
		{"latin-1 high bytes", []byte{'d', 'o', 'n', 0xe9, 'e', 's'}, false},
		{"marc-8 escape sequence", []byte{0x1b, '(', 'B', 'a', 0xe9}, false},
		{"truncated multibyte at the end still counts", append([]byte("café"), 0xc3), true},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksUTF8(tt.in); got != tt.want {
				t.Errorf("LooksUTF8(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// Records that carry UTF-8 bytes while their leader still says MARC-8 are
// common in OAI-PMH harvests. Decoding them as MARC-8 turns "données" into
// "donn©♭es", so the loader trusts the bytes over the leader.
func TestLoadMislabeledUTF8(t *testing.T) {
	good, err := os.ReadFile("../../testdata/sample.mrc")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Clear the "a" in leader position 9 of every record, leaving the UTF-8
	// bytes in place.
	mislabeled := append([]byte(nil), good...)
	for off := 0; off < len(mislabeled); {
		length := 0
		for i := 0; i < 5; i++ {
			length = length*10 + int(mislabeled[off+i]-'0')
		}
		mislabeled[off+9] = ' '
		off += length
	}
	if bytes.Equal(good, mislabeled) {
		t.Fatal("test fixture was not modified")
	}

	res, err := Load(bytes.NewReader(mislabeled))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(res.Records) != 3 {
		t.Fatalf("got %d records, want 3", len(res.Records))
	}
	if got := Title(res.Records[1]); !strings.Contains(got, "café-cultuur") {
		t.Errorf("title = %q, want it to contain café-cultuur", got)
	}
	if !res.ForcedUTF8 {
		t.Error("Result.ForcedUTF8 should record that the leader was overridden")
	}
}

// A file whose leader is honest must not be flagged as overridden.
func TestLoadWellLabeledUTF8(t *testing.T) {
	res, err := LoadFile("../../testdata/sample.mrc")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := Title(res.Records[1]); !strings.Contains(got, "café-cultuur") {
		t.Errorf("title = %q, want it to contain café-cultuur", got)
	}
}
