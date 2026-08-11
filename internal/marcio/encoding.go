package marcio

import (
	"bytes"
	"unicode/utf8"
)

// encodingSample is how much of a file is inspected to decide its encoding.
// Diacritics tend to appear early in a catalogue, and reading more only slows
// the first paint.
const encodingSample = 64 * 1024

// LooksUTF8 reports whether data carries multi-byte UTF-8 sequences and no
// MARC-8 escape sequences. Pure ASCII returns false: it decodes identically
// either way, so there is nothing to override.
//
// This exists because OAI-PMH repositories routinely emit UTF-8 record bytes
// while leaving leader/09 blank, which claims MARC-8. Trusting that leader
// turns "données" into "donn©♭es".
func LooksUTF8(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// A MARC-8 record announces its character sets with escape sequences.
	if bytes.IndexByte(data, 0x1b) >= 0 {
		return false
	}

	// A sample can cut a multi-byte sequence in half; drop a trailing partial
	// rune before validating.
	trimmed := data
	for i := 0; i < utf8.UTFMax && len(trimmed) > 0; i++ {
		if r, size := utf8.DecodeLastRune(trimmed); r == utf8.RuneError && size <= 1 {
			trimmed = trimmed[:len(trimmed)-1]
			continue
		}
		break
	}
	if len(trimmed) == 0 || !utf8.Valid(trimmed) {
		return false
	}

	// Only non-ASCII content proves anything.
	for _, b := range trimmed {
		if b >= utf8.RuneSelf {
			return true
		}
	}
	return false
}
