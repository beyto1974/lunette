package render

import "strings"

// Sanitize replaces control characters with caret notation, so that text taken
// from a record cannot act on the terminal it is displayed in.
//
// MARC files come from other people's repositories. A record may carry ESC
// sequences - MARC-8 uses them legitimately, and a mislabelled record hands
// them over intact, since ESC is valid UTF-8. Written to a terminal unfiltered
// they would clear the screen, rewrite what is already there, retitle the
// window, or copy attacker-chosen text into the user's clipboard through
// OSC 52.
//
// Caret notation keeps the text visible - "^[" rather than nothing - so a
// reader can see what the record actually contains, and it keeps one byte one
// cell, which the wrapping and field-span arithmetic depends on. Newlines and
// tabs are substituted too: a newline inside a field value would otherwise
// shift every line number the field cursor and the match navigator rely on.
func Sanitize(s string) string {
	if !needsSanitizing(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case r < 0x20:
			// C0: ^@ through ^_
			b.WriteByte('^')
			b.WriteRune(r + 0x40)
		case r == 0x7f:
			b.WriteString("^?")
		case r >= 0x80 && r <= 0x9f:
			// C1: show the 7-bit equivalent, so 0x9b (CSI) reads as ^[.
			b.WriteByte('^')
			b.WriteRune(r - 0x40)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// needsSanitizing keeps the common case allocation-free: almost every value in
// a catalogue is ordinary text.
func needsSanitizing(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return true
		}
	}
	return false
}
