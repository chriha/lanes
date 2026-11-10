// Package cache persists provider results for an instant first paint.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/item"
)

func path(name string) string { return filepath.Join(app.CacheDir(), name+".json") }

func Load(name string) []item.Item {
	b, err := os.ReadFile(path(name))
	if err != nil {
		return nil
	}
	var items []item.Item
	if json.Unmarshal(b, &items) != nil {
		return nil
	}
	return items
}

var saveMu sync.Mutex // Save runs from tea.Cmds, so two writes of one file must not interleave

func Save(name string, items []item.Item) {
	b, err := json.Marshal(items)
	saveMu.Lock()
	defer saveMu.Unlock()
	if err != nil || os.MkdirAll(app.CacheDir(), app.DirMode) != nil {
		return
	}
	_ = app.WriteFile(path(name), b)
}
