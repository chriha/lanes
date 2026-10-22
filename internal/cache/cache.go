// Package cache persists provider results for an instant first paint.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"

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

func Save(name string, items []item.Item) {
	b, err := json.Marshal(items)
	if err != nil || os.MkdirAll(app.CacheDir(), 0o755) != nil {
		return
	}
	_ = os.WriteFile(path(name), b, 0o644)
}
