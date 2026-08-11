package render

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// palette holds the styles used by the annotated and raw views. A zero-value
// lipgloss.Style renders text unchanged, so plainPalette gives byte-identical
// output to a hand-built plain string.
type palette struct {
	tag    lipgloss.Style
	ind    lipgloss.Style
	code   lipgloss.Style
	label  lipgloss.Style
	leader lipgloss.Style
	dim    lipgloss.Style
	match  lipgloss.Style
	plain  bool
}

// Colours are ANSI 256 indices rather than hex so they follow the terminal's
// own theme instead of fighting it.
func newPalette() palette {
	return palette{
		tag:    lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		ind:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		code:   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		label:  lipgloss.NewStyle().Foreground(lipgloss.Color("108")),
		leader: lipgloss.NewStyle().Foreground(lipgloss.Color("140")),
		dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		match:  lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("220")),
	}
}

func plainPalette() palette { return palette{plain: true} }

// value renders field text, highlighting every case-insensitive occurrence of
// match. With no match, or in plain mode, the text passes through untouched.
func (p palette) value(s, match string) string {
	if p.plain || match == "" {
		return s
	}
	lowerS, lowerM := strings.ToLower(s), strings.ToLower(match)

	var b strings.Builder
	for {
		i := strings.Index(lowerS, lowerM)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString(p.match.Render(s[i : i+len(match)]))
		s, lowerS = s[i+len(match):], lowerS[i+len(match):]
	}
}
