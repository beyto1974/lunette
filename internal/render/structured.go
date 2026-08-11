package render

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
)

// indentWidth is two spaces: MARC records nest four levels deep, and anything
// wider pushes subfield values off a narrow pane.
const indentWidth = "  "

// indentJSON pretty-prints a JSON document. It returns the input unchanged if
// it does not parse, since a display path should never lose the record.
func indentJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", indentWidth); err != nil {
		return s
	}
	return buf.String()
}

// indentXML reformats a MARCXML document one element per line.
//
// Two things need care. encoding/xml resolves namespaces onto every element
// and re-declares them on output, which would put an xmlns on all fifteen
// elements of a record; the namespace is therefore stripped from every element
// and declared once on the root. And character data is passed through
// untouched: MARC fixed fields are position-significant, so trimming the
// trailing spaces of an 008 would corrupt the record.
//
// The input is the single-line document the writer produces, so there is no
// existing indentation whitespace to discard.
func indentXML(s string) string {
	var buf bytes.Buffer
	dec := xml.NewDecoder(strings.NewReader(s))
	enc := xml.NewEncoder(&buf)
	enc.Indent("", indentWidth)

	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return s
		}

		switch t := tok.(type) {
		case xml.StartElement:
			ns := t.Name.Space
			t.Name.Space = ""
			attrs := t.Attr[:0]
			for _, a := range t.Attr {
				if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
					continue
				}
				a.Name.Space = ""
				attrs = append(attrs, a)
			}
			t.Attr = attrs
			if depth == 0 && ns != "" {
				t.Attr = append(t.Attr, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: ns})
			}
			depth++
			tok = t

		case xml.EndElement:
			depth--
			t.Name.Space = ""
			tok = t
		}

		if err := enc.EncodeToken(tok); err != nil {
			return s
		}
	}
	if err := enc.Close(); err != nil {
		return s
	}
	return buf.String()
}

// highlightMatches marks every case-insensitive occurrence of match by
// inverting it with SGR 7, closed by SGR 27.
//
// Inverting rather than colouring is what makes this safe over chroma output:
// setting a colour would need a reset to undo, and a reset would also wipe the
// syntax colour chroma had in force at that point. SGR 27 turns inversion off
// and leaves every other attribute alone.
func highlightMatches(styled, match string) string {
	if match == "" || styled == "" {
		return styled
	}

	// Map each printable byte to its offset in the styled string, so matches
	// found in the plain text can be located in the escaped one.
	var plain strings.Builder
	offsets := make([]int, 0, len(styled))
	for i := 0; i < len(styled); {
		if styled[i] == 0x1b {
			j := i + 1
			if j < len(styled) && styled[j] == '[' {
				j++
				for j < len(styled) && (styled[j] < '@' || styled[j] > '~') {
					j++
				}
			}
			i = j + 1
			continue
		}
		plain.WriteByte(styled[i])
		offsets = append(offsets, i)
		i++
	}

	haystack, needle := plain.String(), match
	lowerHay, lowerNeedle := strings.ToLower(haystack), strings.ToLower(needle)
	// Lowercasing can change length for a few Unicode cases; fall back to a
	// case-sensitive search rather than splicing at wrong offsets.
	if len(lowerHay) == len(haystack) && len(lowerNeedle) == len(needle) {
		haystack, needle = lowerHay, lowerNeedle
	}

	var out strings.Builder
	out.Grow(len(styled) + 16)
	last, from := 0, 0
	for {
		i := strings.Index(haystack[from:], needle)
		if i < 0 || needle == "" {
			break
		}
		start := from + i
		end := start + len(needle) // exclusive, in plain bytes

		out.WriteString(styled[last:offsets[start]])
		out.WriteString("\x1b[7m")
		out.WriteString(styled[offsets[start] : offsets[end-1]+1])
		out.WriteString("\x1b[27m")

		last = offsets[end-1] + 1
		from = end
	}
	out.WriteString(styled[last:])
	return out.String()
}
