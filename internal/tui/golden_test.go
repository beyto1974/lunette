package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/marcview/internal/render"
)

// update rewrites the golden files instead of comparing against them:
//
//	go test ./internal/tui/ -run TestGolden -update
var update = flag.Bool("update", false, "rewrite the golden frames")

// Golden frames guard the layout arithmetic. Three sizing bugs shipped in this
// package - panes two rows too tall, list rows two cells too wide so the year
// wrapped onto its own line, and the full help ellipsised off the right edge -
// and none of them failed a test, because each assertion checked a property
// rather than looking at the frame. A whole rendered frame catches all three,
// and makes deliberate layout changes visible in review.
//
// Frames are stored without ANSI escapes: this is a test of layout, not of
// colour, and escape sequences would make every diff unreadable.
func TestGoldenFrames(t *testing.T) {
	cases := []struct {
		name  string
		w, h  int
		setup func(*Model)
	}{
		{name: "initial-120x40", w: 120, h: 40},
		{name: "initial-80x24", w: 80, h: 24},
		{name: "narrow-40x14", w: 40, h: 14},
		{name: "filtered-100x30", w: 100, h: 30, setup: func(m *Model) {
			m.setFilter("transmission")
		}},
		{name: "fulltext-filter-100x30", w: 100, h: 30, setup: func(m *Model) {
			m.setFilter("all:privacy")
		}},
		{name: "help-100x30", w: 100, h: 30, setup: func(m *Model) {
			m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
		}},
		{name: "compact-detail-focus-100x30", w: 100, h: 30, setup: func(m *Model) {
			m.list.Select(1)
			m.setMode(render.Compact)
			m.focus = paneDetail
		}},
		{name: "no-match-100x30", w: 100, h: 30, setup: func(m *Model) {
			m.setFilter("zzzz")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newLoaded(t)
			m.Update(tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
			if tc.setup != nil {
				tc.setup(m)
				// A setup step may change how much room the footer needs.
				m.Update(tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
			}

			got := stripANSI(m.View().Content)
			path := filepath.Join("testdata", "golden", tc.name+".txt")

			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return
			}

			wantBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run: go test ./internal/tui/ -run TestGolden -update)", err)
			}
			if want := string(wantBytes); got != want {
				t.Errorf("frame differs from %s\n--- want ---\n%s\n--- got ---\n%s\n%s",
					path, want, got, firstDifference(want, got))
			}

			// The frame must also fit the terminal it claims to be drawn for.
			lines := strings.Split(got, "\n")
			if len(lines) > tc.h {
				t.Errorf("frame is %d lines, terminal is %d", len(lines), tc.h)
			}
			for i, line := range lines {
				if w := lipglossWidth(line); w > tc.w {
					t.Errorf("line %d is %d cells wide, terminal is %d", i, w, tc.w)
				}
			}
		})
	}
}

// firstDifference points at the line where two frames diverge, which is easier
// to read than two full frames side by side.
func firstDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return "first difference on line " + itoa(i) + ":\nwant: " + wl + "\ngot:  " + gl
		}
	}
	return ""
}
