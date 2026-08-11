package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/marcview/internal/marcio"
)

// pollInterval is how often a followed file is checked. A harvest writes a
// page of records every second or two, so this keeps the list close to live
// without spending the terminal's time on stat calls.
const pollInterval = time.Second

// followMsg carries whatever a poll found.
type followMsg struct {
	batch marcio.Batch
	next  int64
	err   error
}

// pollFile reads whatever has been appended since the last poll. It is a
// method returning a command so that tests can run one poll synchronously
// rather than waiting on a timer.
func (m *Model) pollFile() tea.Cmd {
	path, offset := m.path, m.followOffset
	return func() tea.Msg {
		res, next, err := marcio.LoadFrom(path, offset)
		if err != nil {
			return followMsg{err: err}
		}
		return followMsg{
			batch: marcio.Batch{
				Format:     res.Format,
				ForcedUTF8: res.ForcedUTF8,
				Records:    res.Records,
				Issues:     res.Issues,
			},
			next: next,
		}
	}
}

// schedulePoll waits a tick and polls again.
func (m *Model) schedulePoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return m.pollFile()() })
}

// handleFollow folds a poll result into the model.
func (m *Model) handleFollow(msg followMsg) tea.Cmd {
	if msg.err != nil {
		// A file that shrank was replaced rather than appended to. Say so and
		// stop following rather than reading a new file as if it continued the
		// old one.
		m.status = "stopped following: " + msg.err.Error()
		m.following = false
		return nil
	}

	m.followOffset = msg.next
	if len(msg.batch.Records) == 0 && len(msg.batch.Issues) == 0 {
		return m.schedulePoll()
	}

	before := len(m.records)
	m.appendBatch(msg.batch)
	cmd := m.rebuildItems()
	if before == 0 {
		m.refreshDetail()
	}
	m.status = fmt.Sprintf("following: %d records", len(m.records))
	return tea.Batch(cmd, m.schedulePoll())
}
