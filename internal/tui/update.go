package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/marcview/internal/marcio"
	"github.com/beyto1974/marcview/internal/render"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		// Re-wrap at the new width, but leave the cursor where the user put it.
		m.reflowDetail()
		return m, nil

	case loadMsg:
		return m, m.handleLoad(msg)

	case tea.KeyPressMsg:
		if m.inputMode != inputNone {
			return m, m.handleInputKey(msg)
		}
		return m, m.handleKey(msg)

	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	}

	return m, nil
}

// handleLoad folds one streamed batch into the model.
func (m *Model) handleLoad(msg loadMsg) tea.Cmd {
	// The final message carries no batch, so keep what earlier ones reported.
	if msg.batch.Format != marcio.FormatUnknown {
		m.format = msg.batch.Format
	}
	m.forcedUTF8 = m.forcedUTF8 || msg.batch.ForcedUTF8
	for _, rec := range msg.batch.Records {
		m.records = append(m.records, rec)
		m.keys = append(m.keys, marcio.SearchKey(rec))
	}
	m.issues = append(m.issues, msg.batch.Issues...)

	if msg.done {
		m.loading = false
		m.loadErr = msg.err
		cmd := m.rebuildItems()
		m.refreshDetail()
		return cmd
	}

	first := len(m.records) == len(msg.batch.Records)
	cmd := m.rebuildItems()
	if first {
		m.refreshDetail()
	}
	return tea.Batch(cmd, waitFor(m.ch))
}

// handleInputKey drives the filter and jump prompts.
func (m *Model) handleInputKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		value := m.input.Value()
		mode := m.inputMode
		m.closeInput()
		if mode == inputJump {
			m.jumpTo(value)
			return nil
		}
		return m.setFilter(value)

	case "esc":
		m.closeInput()
		return nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

// handleKey dispatches a key press outside of the prompts.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys_

	switch {
	case key.Matches(msg, k.Quit):
		return tea.Quit

	case key.Matches(msg, k.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
		return nil

	case key.Matches(msg, k.Switch):
		if m.focus == paneList {
			m.focus = paneDetail
		} else {
			m.focus = paneList
		}
		return nil

	case key.Matches(msg, k.Filter):
		m.openInput(inputFilter)
		return nil

	case key.Matches(msg, k.Jump):
		m.openInput(inputJump)
		return nil

	// In one pane, esc is the way back to the list; only once there does it
	// clear the filter.
	case key.Matches(msg, k.Clear):
		if m.singlePane && m.focus == paneDetail {
			m.focus = paneList
			return nil
		}
		if m.filter.empty() {
			return nil
		}
		return m.setFilter("")

	case key.Matches(msg, k.Zoom):
		m.zoom = !m.zoom
		m.layout()
		m.reflowDetail()
		m.status = "one pane: " + onOff(m.zoom)
		return nil

	case key.Matches(msg, k.Annotated):
		return m.setMode(render.Annotated)
	case key.Matches(msg, k.Compact):
		return m.setMode(render.Compact)
	case key.Matches(msg, k.Raw):
		return m.setMode(render.Raw)
	case key.Matches(msg, k.JSON):
		return m.setMode(render.JSON)
	case key.Matches(msg, k.XML):
		return m.setMode(render.XML)

	// n and N follow the focus: through the matching lines of one record on
	// the right, and from matching record to matching record on the left.
	case key.Matches(msg, k.NextMatch):
		if m.focus == paneList {
			m.stepMatchingRecord(1)
		} else {
			m.nextMatch()
		}
		return nil
	case key.Matches(msg, k.PrevMatch):
		if m.focus == paneList {
			m.stepMatchingRecord(-1)
		} else {
			m.previousMatch()
		}
		return nil

	case key.Matches(msg, k.Scope):
		return m.cycleScope()

	case key.Matches(msg, k.Copy):
		return m.copyCurrent()
	}

	// Anything else drives the focused pane. With a record focused the up and
	// down keys move the field cursor; the structured views have no fields, so
	// there they scroll as usual.
	if m.focus == paneDetail {
		if m.fieldCount() > 0 {
			switch {
			case key.Matches(msg, k.Up):
				m.moveField(-1)
				return nil
			case key.Matches(msg, k.Down):
				m.moveField(1)
				return nil
			case key.Matches(msg, k.Top):
				m.moveField(-len(m.fields))
				return nil
			case key.Matches(msg, k.Bottom):
				m.moveField(len(m.fields))
				return nil
			}
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return cmd
	}

	before := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if m.list.Index() != before {
		m.refreshDetail()
	}
	return cmd
}

func (m *Model) setMode(mode render.Mode) tea.Cmd {
	m.mode = mode
	m.refreshDetail()
	m.status = "view: " + mode.String()
	return nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// copyCurrent puts the uncoloured rendering on the system clipboard via OSC 52,
// which works over SSH too: the selected field when the record pane has focus,
// the whole record otherwise.
func (m *Model) copyCurrent() tea.Cmd {
	rec, it, ok := m.current()
	if !ok {
		return nil
	}

	if m.focus == paneDetail && m.fieldCount() > 0 {
		if text, ok := m.selectedFieldText(); ok {
			m.status = fmt.Sprintf("copied field %s of record %d",
				m.fields[m.fieldCursor].Tag, it.ordinal)
			return tea.SetClipboard(text)
		}
	}
	// Copy unwrapped: the pane width is a display choice, not part of the
	// record.
	out, err := render.Render(rec, m.mode, render.Options{})
	if err != nil {
		m.status = "copy failed: " + err.Error()
		return nil
	}
	m.status = fmt.Sprintf("copied record %d as %s", it.ordinal, m.mode)
	return tea.SetClipboard(out)
}

func (m *Model) openInput(mode inputMode) {
	m.inputMode = mode
	m.status = ""
	m.input.Reset()
	if mode == inputFilter {
		m.input.SetValue(m.filterExpression())
		m.input.CursorEnd()
	}
	m.input.Focus()
}

func (m *Model) closeInput() {
	m.inputMode = inputNone
	m.input.Blur()
	m.input.Reset()
}

// filterExpression reconstructs what the user typed, so reopening the prompt
// starts from the active filter.
func (m *Model) filterExpression() string {
	s := ""
	if m.filter.tag != "" {
		s = "tag:" + m.filter.tag
	}
	if m.filter.query != "" {
		if s != "" {
			s += " "
		}
		switch m.filter.scope {
		case marcio.ScopeBoth:
			s += "all:"
		case marcio.ScopeRecord:
			s += "rec:"
		}
		s += m.filter.query
	}
	return s
}
