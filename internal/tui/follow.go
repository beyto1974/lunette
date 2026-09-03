package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/beyto1974/lunette/internal/marcio"
)

// pollInterval is how often a followed file is checked when the platform
// cannot watch it, and how often it is checked anyway as a safety net: a watch
// on a network filesystem can miss writes made by another host.
const pollInterval = time.Second

// safetyInterval is the slow re-check used while watching, which costs one
// stat call and covers events the watcher never saw.
const safetyInterval = 15 * time.Second

// trigger says what caused a read, which decides what to arm afterwards.
type trigger int

const (
	// triggerWatch is a notification from the filesystem; the watch has to be
	// re-armed after it.
	triggerWatch trigger = iota
	// triggerPoll is the fallback timer, which re-arms itself.
	triggerPoll
	// triggerSafety is the slow re-check that runs alongside a watch. It has
	// its own timer, so arming anything else here would leave a second reader
	// waiting on the same channel after every tick.
	triggerSafety
)

// followMsg carries whatever a read found.
type followMsg struct {
	batch marcio.Batch
	next  int64
	err   error
	from  trigger
}

// pollFile reads whatever has been appended since the last poll. It is a
// method returning a command so that tests can run one poll synchronously
// rather than waiting on a timer.
func (m *Model) pollFile() tea.Cmd { return m.readFile(triggerPoll) }

// readFile reads whatever has been appended since the last read, tagging the
// result with what asked for it.
func (m *Model) readFile(from trigger) tea.Cmd {
	path, offset := m.path, m.followOffset
	return func() tea.Msg {
		res, next, err := marcio.LoadFrom(path, offset)
		if err != nil {
			return followMsg{err: err, from: from}
		}
		return followMsg{
			batch: marcio.Batch{
				Format:     res.Format,
				ForcedUTF8: res.ForcedUTF8,
				Records:    res.Records,
				Extents:    res.Extents,
				Issues:     res.Issues,
			},
			next: next,
			from: from,
		}
	}
}

// startFollowing begins watching the file, falling back to a timer where the
// platform cannot: an unsupported filesystem, or an inotify limit already
// reached by other programs.
func (m *Model) startFollowing() tea.Cmd {
	newWatcher := m.newWatcher
	if newWatcher == nil {
		newWatcher = newFileWatcher
	}

	w, err := newWatcher(m.path)
	if err != nil {
		m.watching = false
		m.status = "following by polling: " + err.Error()
		return m.schedulePoll()
	}

	m.watcher = w
	m.watching = true
	// The safety tick runs alongside the watch rather than instead of it.
	return tea.Batch(m.waitForChange(), m.scheduleSafetyPoll())
}

// waitForChange blocks until the watcher reports a write, then reads.
func (m *Model) waitForChange() tea.Cmd {
	ch := m.watcher.changes()
	read := m.readFile(triggerWatch)
	return func() tea.Msg {
		<-ch
		return read()
	}
}

// schedulePoll waits a tick and polls again. This is the fallback path.
func (m *Model) schedulePoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return m.pollFile()() })
}

// scheduleSafetyPoll re-reads occasionally even while watching.
func (m *Model) scheduleSafetyPoll() tea.Cmd {
	return tea.Tick(safetyInterval, func(time.Time) tea.Msg { return safetyTickMsg{} })
}

// safetyTickMsg is the slow re-check while watching.
type safetyTickMsg struct{}

// handleFollow folds a poll result into the model.
func (m *Model) handleFollow(msg followMsg) tea.Cmd {
	if msg.err != nil {
		// A file that shrank was replaced rather than appended to. Say so and
		// stop following rather than reading a new file as if it continued the
		// old one.
		m.status = "stopped following: " + msg.err.Error()
		m.stopFollowing()
		return nil
	}

	m.followOffset = msg.next
	if len(msg.batch.Records) == 0 && len(msg.batch.Issues) == 0 {
		return m.waitForNext(msg.from)
	}

	// The file may have been replaced rather than appended to since the last
	// read, so what is held from before it may no longer be what is there.
	m.forgetCached()

	before := m.appendBatch(msg.batch)
	cmd := m.appendItems(before)
	if before == 0 {
		m.refreshDetail()
	}
	m.status = fmt.Sprintf("following: %d records", m.count())
	return tea.Batch(cmd, m.waitForNext(msg.from))
}

// waitForNext arms whichever mechanism this session is using. A safety tick
// arms nothing: its own timer is already running, and adding a watch waiter per
// tick would leave a queue of readers behind.
func (m *Model) waitForNext(from trigger) tea.Cmd {
	if !m.following || from == triggerSafety {
		return nil
	}
	if m.watching {
		return m.waitForChange()
	}
	return m.schedulePoll()
}

// stopFollowing releases the watch.
func (m *Model) stopFollowing() {
	m.following = false
	m.watching = false
	if m.watcher != nil {
		m.watcher.close()
		m.watcher = nil
	}
}
