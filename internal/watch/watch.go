package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDelay = 300 * time.Millisecond

// Handler is called with the set of changed file paths after debouncing.
type Handler func(changed []string) error

// Watcher watches a directory tree for file changes and calls handler after
// a debounce window. Only files matching the tracked extensions are reported.
type Watcher struct {
	root    string
	exts    map[string]bool
	handler Handler
	fw      *fsnotify.Watcher
}

// New creates a Watcher rooted at dir, watching files with the given extensions.
func New(dir string, exts []string, handler Handler) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	extSet := make(map[string]bool, len(exts))
	for _, e := range exts {
		extSet[e] = true
	}

	w := &Watcher{root: dir, exts: extSet, handler: handler, fw: fw}

	// Walk and add all directories recursively
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return fw.Add(path)
	}); err != nil {
		fw.Close()
		return nil, fmt.Errorf("setting up watches: %w", err)
	}

	return w, nil
}

// Run starts the watch loop. It blocks until ctx is cancelled or an error
// occurs. Uses a debounce window to coalesce rapid saves.
func (w *Watcher) Run(stop <-chan struct{}) error {
	defer w.fw.Close()

	pending := make(map[string]struct{})
	var timer *time.Timer

	flush := func() {
		if len(pending) == 0 {
			return
		}
		changed := make([]string, 0, len(pending))
		for f := range pending {
			changed = append(changed, f)
		}
		pending = make(map[string]struct{})
		if err := w.handler(changed); err != nil {
			fmt.Fprintf(os.Stderr, "[watch] handler error: %v\n", err)
		}
	}

	for {
		select {
		case <-stop:
			if timer != nil {
				timer.Stop()
			}
			return nil

		case event, ok := <-w.fw.Events:
			if !ok {
				return nil
			}
			if !w.relevant(event.Name) {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				pending[event.Name] = struct{}{}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounceDelay, flush)
			}
			// If a new directory is created, watch it
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.fw.Add(event.Name)
				}
			}

		case err, ok := <-w.fw.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "[watch] fsnotify error: %v\n", err)
		}
	}
}

// relevant returns true if the file path has a tracked extension.
func (w *Watcher) relevant(path string) bool {
	if len(w.exts) == 0 {
		return true
	}
	return w.exts[filepath.Ext(path)]
}
