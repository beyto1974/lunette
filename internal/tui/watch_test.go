package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The watcher reports a real append quickly, without anyone polling.
func TestFileWatcherSeesAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harvest.mrc")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := newFileWatcher(path)
	if err != nil {
		t.Skipf("no file watching on this platform: %v", err)
	}
	defer w.close()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString(" more"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	f.Close()

	select {
	case <-w.changes():
	case <-time.After(5 * time.Second):
		t.Fatal("no change reported within 5s of appending")
	}
}

// A burst of writes must not produce a burst of reads.
func TestFileWatcherCoalesces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "harvest.mrc")
	if err := os.WriteFile(path, []byte("start"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w, err := newFileWatcher(path)
	if err != nil {
		t.Skipf("no file watching on this platform: %v", err)
	}
	defer w.close()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	for i := 0; i < 50; i++ {
		if _, err := f.WriteString("x"); err != nil {
			t.Fatalf("WriteString: %v", err)
		}
	}
	f.Close()

	select {
	case <-w.changes():
	case <-time.After(5 * time.Second):
		t.Fatal("no change reported")
	}

	// 50 writes must not queue 50 notifications.
	time.Sleep(300 * time.Millisecond)
	queued := 0
	for {
		select {
		case <-w.changes():
			queued++
			continue
		default:
		}
		break
	}
	if queued > 2 {
		t.Errorf("%d notifications queued for one burst, want them coalesced", queued)
	}
}

// Where the platform cannot watch, following still works on a timer.
func TestFollowFallsBackToPolling(t *testing.T) {
	path, whole := growing(t, 400)

	m, err := New(path, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.newWatcher = func(string) (changeSource, error) { return nil, errors.New("no watches left") }
	drainLoad(t, m)

	if m.watching {
		t.Error("model claims to be watching after the watcher failed")
	}

	// The polling path still picks the records up.
	if err := os.WriteFile(path, whole, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	m.Update(m.pollFile()())
	if len(m.records) != 3 {
		t.Errorf("polling fallback loaded %d records, want 3", len(m.records))
	}
}

// With a watcher, a change notification drives the read.
func TestFollowReadsOnNotification(t *testing.T) {
	path, whole := growing(t, 400)

	fake := &fakeWatcher{ch: make(chan struct{}, 1)}
	m, err := New(path, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.newWatcher = func(string) (changeSource, error) { return fake, nil }
	drainLoad(t, m)

	if !m.watching {
		t.Fatal("model is not watching although the watcher started")
	}
	if len(m.records) != 1 {
		t.Fatalf("loaded %d records, want 1", len(m.records))
	}

	if err := os.WriteFile(path, whole, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fake.ch <- struct{}{}

	// The command waits for the notification and then reads.
	m.Update(m.waitForChange()())
	if len(m.records) != 3 {
		t.Errorf("after the notification there are %d records, want 3", len(m.records))
	}
	if fake.closed {
		t.Error("the watcher was closed while still following")
	}
}

// The safety tick must not leave an extra reader waiting on the watch after
// every tick.
func TestSafetyTickDoesNotStackWaiters(t *testing.T) {
	path, _ := growing(t, 400)

	fake := &fakeWatcher{ch: make(chan struct{}, 1)}
	m, err := New(path, WithFollow())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.newWatcher = func(string) (changeSource, error) { return fake, nil }
	drainLoad(t, m)

	if cmd := m.handleFollow(followMsg{from: triggerSafety, next: m.followOffset}); cmd != nil {
		t.Error("a safety-tick read armed another watch waiter")
	}
	if cmd := m.handleFollow(followMsg{from: triggerWatch, next: m.followOffset}); cmd == nil {
		t.Error("a watch read did not re-arm the watch")
	}
}

type fakeWatcher struct {
	ch     chan struct{}
	closed bool
}

func (f *fakeWatcher) changes() <-chan struct{} { return f.ch }
func (f *fakeWatcher) close()                   { f.closed = true }
