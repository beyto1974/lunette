// Filtering: which records the list shows, where the search looks, and
// stepping between what it found.
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	marc "github.com/beyto1974/gomarc"
	"github.com/beyto1974/lunette/internal/marcio"
	"github.com/beyto1974/lunette/internal/render"
)

// filterSpec is a parsed filter expression.
type filterSpec struct {
	query string       // lowercased free text
	tag   string       // field that must be present
	scope marcio.Scope // which index the query is matched against
}

func (f filterSpec) empty() bool { return f.query == "" && f.tag == "" }

// visible reports whether one record passes the current filter. It is answered
// from what was kept at load time: a filter that fetched every record back
// would read the whole file again.
func (m *Model) visible(i int) bool {
	if m.filter.tag != "" && !marcio.TagsContain(m.entries[i].tags, m.filter.tag) {
		return false
	}
	return m.recordMatches(i)
}

// visibleIndices reports the records currently passing the filter, as indices
// into m.records.
func (m *Model) visibleIndices() []int {
	out := make([]int, 0, m.count())
	for i := range m.entries {
		if m.visible(i) {
			out = append(out, i)
		}
	}
	return out
}

// setFilter applies a filter expression and moves the cursor back to the top
// of the result.
func (m *Model) setFilter(expr string) tea.Cmd {
	// The status line falls back to describing the filter, which is more use
	// than whatever message preceded it.
	m.status = ""
	m.filter = parseFilter(expr)
	if m.filter.scope.NeedsFullText() {
		m.buildFullKeys()
	}
	m.list.SetDelegate(compactDelegate{styles: m.st, match: m.filter.query})
	cmd := m.rebuildItems()
	m.list.Select(0)
	m.refreshDetail()
	return cmd
}

// buildFullKeys fills the full-text index for any records that do not have one
// yet, which also covers records that arrived after the last "all:" search.
//
// This is the one thing that does read every record back, because searching
// every subfield is what the "all:" scope means. The reads run in record
// order, so the file is walked forwards rather than seeked around.
func (m *Model) buildFullKeys() {
	for i := len(m.fullKeys); i < m.count(); i++ {
		rec, err := m.record(i)
		if err != nil {
			// A record that cannot be read back matches nothing rather than
			// stopping the search on the records that can.
			m.fullKeys = append(m.fullKeys, "")
			continue
		}
		m.fullKeys = append(m.fullKeys, marcio.FullTextKey(rec))
	}
}

// findMatches records which lines of the current rendering hold a match, so
// that n and N can step between them. The plain rendering is searched rather
// than the coloured one, whose escape sequences would break the offsets.
func (m *Model) findMatches(rec *marc.Record) {
	m.matchLines, m.matchIdx = nil, 0
	if m.filter.query == "" {
		return
	}
	plain, err := render.Render(rec, m.mode, m.renderOptions(false))
	if err != nil {
		return
	}
	for i, line := range strings.Split(plain, "\n") {
		if strings.Contains(strings.ToLower(line), m.filter.query) {
			m.matchLines = append(m.matchLines, i)
		}
	}
	m.scrollToMatch()
}

// stepMatchingRecord moves the list cursor to the next record matching the
// search term, wrapping at the ends. When a filter is narrowing the list every
// visible record matches, so this steps one row; when it is not, it skips the
// records that do not.
func (m *Model) stepMatchingRecord(dir int) {
	items := m.list.Items()
	if m.filter.query == "" || len(items) == 0 {
		return
	}

	start := m.list.Index()
	for step := 1; step <= len(items); step++ {
		i := ((start+dir*step)%len(items) + len(items)) % len(items)
		it, ok := items[i].(item)
		if !ok {
			continue
		}
		if !m.recordMatches(it.index) {
			continue
		}
		m.list.Select(i)
		m.refreshDetail()
		return
	}
}

// recordMatches reports whether a record matches the current search term in
// the active scope.
func (m *Model) recordMatches(index int) bool {
	if index < 0 || index >= len(m.entries) {
		return false
	}
	var full string
	if index < len(m.fullKeys) {
		full = m.fullKeys[index]
	}
	return m.filter.scope.Matches(m.entries[index].key, full, m.filter.query)
}

// cycleScope moves the search to the next scope and re-runs the filter.
func (m *Model) cycleScope() tea.Cmd {
	m.filter.scope = m.filter.scope.Next()
	if m.filter.scope.NeedsFullText() {
		m.buildFullKeys()
	}
	cmd := m.rebuildItems()
	m.list.Select(0)
	m.refreshDetail()
	m.status = "search scope: " + m.filter.scope.String()
	return cmd
}

// matchCount is how many lines of the current record match the filter.
func (m *Model) matchCount() int { return len(m.matchLines) }

// matchIndex is the zero-based position within those matches.
func (m *Model) matchIndex() int { return m.matchIdx }

func (m *Model) nextMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx + 1) % len(m.matchLines)
	m.scrollToMatch()
}

func (m *Model) previousMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx - 1 + len(m.matchLines)) % len(m.matchLines)
	m.scrollToMatch()
}

// scrollToMatch parks the current match a third of the way down the pane, so
// there is context above it as well as below.
func (m *Model) scrollToMatch() {
	if len(m.matchLines) == 0 {
		return
	}
	offset := m.matchLines[m.matchIdx] - m.vp.Height()/3
	if offset < 0 {
		offset = 0
	}
	m.vp.SetYOffset(offset)
}

// parseFilter reads a filter expression. "tag:856 brussels" keeps records that
// carry an 856 field and match "brussels" in the list titles; a "rec:" prefix
// searches the record body instead, and "all:" searches both.
func parseFilter(s string) filterSpec {
	f := filterSpec{scope: marcio.ScopeTitles}
	for _, word := range strings.Fields(s) {
		lower := strings.ToLower(word)

		for prefix, scope := range map[string]marcio.Scope{
			"all:": marcio.ScopeBoth,
			"rec:": marcio.ScopeRecord,
		} {
			if rest, ok := strings.CutPrefix(lower, prefix); ok {
				f.scope = scope
				lower, word = rest, rest
			}
		}
		if word == "" {
			continue
		}

		if rest, ok := strings.CutPrefix(lower, "tag:"); ok {
			f.tag = rest
			continue
		}
		if f.query != "" {
			f.query += " "
		}
		f.query += word
	}
	f.query = strings.ToLower(f.query)
	return f
}
