// The field cursor: which field of the record on show is selected, and how
// it is marked and reached.
package tui

import (
	"strings"

	"github.com/beyto1974/lunette/internal/render"
)

// fieldMarker is the gutter drawn beside the selected field. Every line gets a
// gutter, marked or not, so the text does not shift as the cursor moves.
const fieldMarker = "▌"

// withFieldMarker adds the cursor gutter to a rendering.
func (m *Model) withFieldMarker(lay render.Layout) string {
	if len(lay.Fields) == 0 {
		return lay.Text
	}
	selected := render.FieldSpan{Start: -1, End: -1}
	if m.fieldCursor < len(lay.Fields) {
		selected = lay.Fields[m.fieldCursor]
	}

	lines := strings.Split(lay.Text, "\n")
	for i, line := range lines {
		if i >= selected.Start && i < selected.End {
			lines[i] = m.st.fieldCursor.Render(fieldMarker) + " " + line
			continue
		}
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}

// fieldCount is how many fields the current rendering exposes.
func (m *Model) fieldCount() int { return len(m.fields) }

// moveField moves the field cursor, clamping at the ends: a cursor that wraps
// makes it impossible to tell the top of a record from the bottom.
func (m *Model) moveField(delta int) {
	if len(m.fields) == 0 {
		return
	}
	next := clamp(m.fieldCursor+delta, 0, len(m.fields)-1)
	if next == m.fieldCursor {
		return
	}
	m.fieldCursor = next
	m.redrawDetail()
	m.scrollToField()
}

// scrollToField brings the selected field into view, without recentring when
// it is already on screen.
func (m *Model) scrollToField() {
	if m.fieldCursor >= len(m.fields) {
		return
	}
	span := m.fields[m.fieldCursor]
	top, bottom := m.vp.YOffset(), m.vp.YOffset()+m.vp.Height()
	switch {
	case span.Start < top:
		m.vp.SetYOffset(span.Start)
	case span.End > bottom:
		m.vp.SetYOffset(span.End - m.vp.Height())
	}
}

// selectedFieldText is the plain text of the field under the cursor.
func (m *Model) selectedFieldText() (string, bool) {
	rec, _, ok := m.current()
	if !ok || m.fieldCursor >= len(m.fields) {
		return "", false
	}
	lay, err := render.RenderLayout(rec, m.mode, m.renderOptions(false))
	if err != nil || m.fieldCursor >= len(lay.Fields) {
		return "", false
	}
	span := lay.Fields[m.fieldCursor]
	lines := strings.Split(lay.Text, "\n")
	if span.End > len(lines) {
		return "", false
	}
	return strings.Join(lines[span.Start:span.End], "\n"), true
}
