// Package watch turns OS file notifications into bounded, coalesced hints.
// Full read-only rescans are the recovery path for overflow or watch limits.
package watch

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

const (
	directoryLimit = 512
	depthLimit     = 8
	entryLimit     = 4096
)

// Watcher watches real directories without following symbolic links. Only the
// scheduler calls Rebuild; the collector drains OS events independently.
type Watcher struct {
	native *fsnotify.Watcher
	hints  chan struct{}
	done   chan struct{}
}

// New starts a watcher. On failure callers use periodic scans alone.
func New() (*Watcher, error) {
	native, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{native: native, hints: make(chan struct{}, 1), done: make(chan struct{})}
	go w.collect()
	return w, nil
}

// Hints includes changes, watcher failures and overflow; delivery never blocks.
func (w *Watcher) Hints() <-chan struct{} { return w.hints }

func (w *Watcher) collect() {
	defer close(w.done)
	defer close(w.hints)
	events, failures := w.native.Events, w.native.Errors
	for events != nil {
		select {
		case _, open := <-events:
			if !open {
				events = nil
				continue
			}
		case failure, open := <-failures:
			if failure == nil && open {
				continue
			}
			if !open {
				failures = nil
				continue
			}
		}
		select {
		case w.hints <- struct{}{}:
		default:
		}
	}
}

// Close releases native resources and waits for the collector.
func (w *Watcher) Close() error { err := w.native.Close(); <-w.done; return err }

// Rebuild refreshes at most 512 directories, eight levels and 4096 entries.
// Missing destinations watch their nearest existing ancestor; parents detect
// atomic replacement, directory deletion and rename. Links are never traversed.
func (w *Watcher) Rebuild(host platform.FS, roots []string) error {
	desired, err := directories(host, roots)
	for _, path := range w.native.WatchList() {
		if !desired[path] {
			err = errors.Join(err, w.native.Remove(path))
		}
	}
	for path := range desired {
		err = errors.Join(err, w.native.Add(path))
	}
	return err
}

type pending struct {
	path  string
	depth int
}

func directories(host platform.FS, roots []string) (map[string]bool, error) {
	dirs := map[string]bool{}
	queue := make([]pending, 0, len(roots))
	for _, root := range roots {
		if len(dirs) >= directoryLimit {
			return dirs, watchLimit()
		}
		queue = append(queue, pending{root, 0})
		if parent := nearestParent(host, root); parent != "" {
			dirs[parent] = true
		}
	}
	visited := map[string]bool{}
	entries := 0
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if visited[item.path] {
			continue
		}
		visited[item.path] = true
		children, err := descendants(host, item, dirs, &entries)
		if err != nil {
			return dirs, err
		}
		queue = append(queue, children...)
	}
	return dirs, nil
}

func watchLimit() error { return errors.New("watch limit reached; periodic rescan enabled") }

func nearestParent(host platform.FS, path string) string {
	parent := filepath.Dir(path)
	for {
		info, err := host.Lstat(parent)
		if err == nil && info.IsDir() {
			return parent
		}
		if !errors.Is(err, fs.ErrNotExist) || parent == filepath.Dir(parent) {
			return ""
		}
		parent = filepath.Dir(parent)
	}
}

func descendants(host platform.FS, item pending, dirs map[string]bool, entries *int) ([]pending, error) {
	info, err := host.Lstat(item.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	if len(dirs) >= directoryLimit || item.depth > depthLimit {
		return nil, watchLimit()
	}
	dirs[item.path] = true
	children, err := host.ReadDir(item.path)
	if err != nil {
		return nil, err
	}
	*entries += len(children)
	if *entries > entryLimit {
		return nil, watchLimit()
	}
	return childDirectories(item, children), nil
}

func childDirectories(item pending, children []fs.DirEntry) []pending {
	queue := make([]pending, 0, len(children))
	for _, child := range children {
		if child.IsDir() && child.Name() != ".git" {
			queue = append(queue, pending{filepath.Join(item.path, child.Name()), item.depth + 1})
		}
	}
	return queue
}
