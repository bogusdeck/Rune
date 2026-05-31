package main

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// watchFiles starts an fsnotify watcher on the topic's workDir and returns a
// tea.Cmd that delivers the next .md file change as a fileChangedMsg. After
// each event, the caller is expected to call watchFiles() again to re-subscribe.
func (m *model) watchFiles() tea.Cmd {
	dir := m.workDir
	if dir == "" {
		dir = "."
	}
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}
		defer watcher.Close()
		if err := addWatchDirs(watcher, dir); err != nil {
			return nil
		}
		for event := range watcher.Events {
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			base := filepath.Base(event.Name)
			// Ignore hidden / dot files (".chat.json", swap files, etc.) so
			// the preview pane doesn't flicker on session saves.
			if strings.HasPrefix(base, ".") {
				continue
			}
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				_ = addWatchDirs(watcher, event.Name)
				continue
			}
			return fileChangedMsg(event.Name)
		}
		return nil
	}
}

func addWatchDirs(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}
