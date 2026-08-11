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
		m.refreshDetail()
		return m, nil

	case loadMsg:
		return m, m.handleLoad(msg)

	case tea.KeyPressMsg:
		if m.inputMode != inputNone {
			return m, m.handleInputKey(msg)
		}
		return m, m.handleKey(msg)
	}

	return m, nil
}

// handleLoad folds one streamed batch into the model.
func (m *Model) handleLoad(msg loadMsg) tea.Cmd {
	m.format = msg.batch.Format
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

	case key.Matches(msg, k.Clear):
		if m.filter.empty() {
			return nil
		}
		return m.setFilter("")

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

	case key.Matches(msg, k.NextMatch):
		m.nextMatch()
		return nil
	case key.Matches(msg, k.PrevMatch):
		m.previousMatch()
		return nil

	case key.Matches(msg, k.Copy):
		return m.copyCurrent()
	}

	// Anything else drives the focused pane.
	if m.focus == paneDetail {
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

// copyCurrent puts the uncoloured rendering of the current record on the
// system clipboard via OSC 52, which works over SSH too.
func (m *Model) copyCurrent() tea.Cmd {
	rec, it, ok := m.current()
	if !ok {
		return nil
	}
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
		if m.filter.fullText {
			s += "all:"
		}
		s += m.filter.query
	}
	return s
}
