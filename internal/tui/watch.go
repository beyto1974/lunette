package tui

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// changeSource reports that a followed file has changed. Notifications carry
// no detail: the reader works out what is new from the file itself.
type changeSource interface {
	changes() <-chan struct{}
	close()
}

// coalesceWindow is how long a burst of writes is allowed to settle before it
// is reported. A harvest writes a page of records in many small writes, and
// reading after each one would decode the same half-finished record repeatedly.
const coalesceWindow = 150 * time.Millisecond

// fileWatcher reports writes to one file through the platform's own mechanism -
// inotify on Linux, kqueue on BSD and macOS - rather than by polling.
type fileWatcher struct {
	w    *fsnotify.Watcher
	ch   chan struct{}
	done chan struct{}
}

// newFileWatcher watches the file's directory rather than the file itself. A
// writer that replaces a file instead of appending to it - which is what an
// atomic rename does - leaves a watch on the old inode pointing at nothing.
func newFileWatcher(path string) (changeSource, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}

	fw := &fileWatcher{
		w:  w,
		ch: make(chan struct{}, 1),
		// done is closed by close(), which stops the forwarding goroutine.
		done: make(chan struct{}),
	}
	go fw.forward(filepath.Clean(path))
	return fw, nil
}

func (f *fileWatcher) changes() <-chan struct{} { return f.ch }

func (f *fileWatcher) close() {
	select {
	case <-f.done:
		return // already closed
	default:
	}
	close(f.done)
	f.w.Close()
}

// forward turns a stream of filesystem events into at most one pending
// notification, so a burst of writes wakes the reader once.
func (f *fileWatcher) forward(path string) {
	var settle <-chan time.Time
	for {
		select {
		case <-f.done:
			return

		case event, ok := <-f.w.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) != path {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}
			settle = time.After(coalesceWindow)

		case <-f.w.Errors:
			// An error on the watch is not worth interrupting the browser for;
			// the safety tick will catch anything missed.
			continue

		case <-settle:
			settle = nil
			select {
			case f.ch <- struct{}{}:
			default: // a notification is already pending
			}
		}
	}
}
