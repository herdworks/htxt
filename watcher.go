package htxt

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Watcher recursively polls a source tree and coalesces file changes.
type Watcher struct {
	Interval time.Duration

	mu                sync.RWMutex
	website           *Website
	sourceDirectories []string
	changes           chan struct{}
}

type watchState struct {
	Modified time.Time
	Mode     fs.FileMode
	Size     int64
}

const (
	// DefaultWatcherInterval is the quiet period before a file change triggers a rebuild.
	DefaultWatcherInterval = time.Second

	watcherPollInterval = 100 * time.Millisecond
)

// NewWatcher creates a watcher for website's managed files.
func NewWatcher(website *Website, sourceDirectories ...string) Watcher {
	return Watcher{
		website:           website,
		Interval:          DefaultWatcherInterval,
		sourceDirectories: slices.Clone(sourceDirectories),
		changes:           make(chan struct{}, 1),
	}
}

// SetWebsite updates the website whose managed files are watched.
func (watcher *Watcher) SetWebsite(website *Website) {
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	watcher.website = website
}

// Changes returns a signal each time the source tree becomes stable after a change.
func (watcher *Watcher) Changes() <-chan struct{} {
	return watcher.changes
}

// Run polls until the context is canceled.
func (watcher *Watcher) Run(ctx context.Context) error {
	if watcher.currentWebsite() == nil {
		return fmt.Errorf("watcher.website is required")
	}
	if watcher.changes == nil {
		watcher.changes = make(chan struct{}, 1)
	}
	interval := watcher.Interval
	if interval == 0 {
		interval = DefaultWatcherInterval
	}
	previous, err := watcher.snapshot()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(watcherPollInterval)
	defer ticker.Stop()
	var changedAt time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			current, err := watcher.snapshot()
			if err != nil {
				return err
			}
			if !sameWatchState(previous, current) {
				previous = current
				changedAt = now
				continue
			}
			if !changedAt.IsZero() && now.Sub(changedAt) >= interval {
				watcher.notify()
				changedAt = time.Time{}
			}
		}
	}
}

func (watcher *Watcher) notify() {
	select {
	case watcher.changes <- struct{}{}:
	default:
	}
}

func (watcher *Watcher) snapshot() (map[string]watchState, error) {
	states := map[string]watchState{}
	website := watcher.currentWebsite()
	if website == nil {
		return nil, fmt.Errorf("watcher.website is required")
	}
	paths, err := website.FilePaths(watcher.sourceDirectories...)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		info, err := os.Stat(filepath.Join(website.RootDir(), filepath.FromSlash(path)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		states[path] = watchState{
			Modified: info.ModTime(),
			Mode:     info.Mode(),
			Size:     info.Size(),
		}
	}
	return states, nil
}

func (watcher *Watcher) currentWebsite() *Website {
	watcher.mu.RLock()
	defer watcher.mu.RUnlock()
	return watcher.website
}

func sameWatchState(first, second map[string]watchState) bool {
	if len(first) != len(second) {
		return false
	}
	for path, firstState := range first {
		secondState, ok := second[path]
		if !ok || firstState != secondState {
			return false
		}
	}
	return true
}
