package server

import (
	"context"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type watcher struct {
	root   string
	w      *fsnotify.Watcher
	logger *log.Logger
	hub    *sseHub
}

func newWatcher(root string, logger *log.Logger, hub *sseHub) (*watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	wat := &watcher{root: root, w: w, logger: logger, hub: hub}
	if err := wat.addRecursive(root); err != nil {
		_ = w.Close()
		return nil, err
	}
	return wat, nil
}

func (wt *watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			return fs.SkipDir
		}
		return wt.w.Add(p)
	})
}

func (wt *watcher) close() {
	_ = wt.w.Close()
}

func (wt *watcher) run(ctx context.Context) {
	var (
		debounce = 150 * time.Millisecond
		timer    *time.Timer
		pending  = map[string]struct{}{}
	)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		// Combine: notify generic reload; clients refetch tree + current view.
		wt.hub.broadcast("reload", "fs")
		pending = map[string]struct{}{}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-wt.w.Events:
			if !ok {
				return
			}
			name := filepath.Base(ev.Name)
			if strings.HasPrefix(name, ".") {
				continue
			}
			// If a new directory is created, watch it.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := osStat(ev.Name); err == nil && info.IsDir() {
					_ = wt.addRecursive(ev.Name)
				}
			}
			pending[ev.Name] = struct{}{}
			if timer == nil {
				timer = time.AfterFunc(debounce, func() { flush() })
			} else {
				timer.Reset(debounce)
			}
		case err, ok := <-wt.w.Errors:
			if !ok {
				return
			}
			if wt.logger != nil {
				wt.logger.Printf("watcher error: %v", err)
			}
		}
	}
}
