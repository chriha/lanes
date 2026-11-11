package trust

import (
	"errors"
	"io/fs"
	"time"
)

// racyWindow is how close to the cache write a directory's mtime may be and still be trusted unchanged: an entry
// created in the same timestamp tick as the stat would leave the mtime as cached, so such a directory is re-read.
const racyWindow = 2 * time.Second

// walkCache is what HashRoot learned about the directory structure under a root. It never holds a trust decision
// or file contents: it only lets the next walk skip reading directories that did not change.
type walkCache struct {
	Written int64              `json:"written"` // Unix nanoseconds the walk that produced it started
	Dirs    map[string]walkDir `json:"dirs"`    // slash path relative to the root ("." for the root itself)
}

// walkDir is one directory in a walkCache.
type walkDir struct {
	ModTime int64    `json:"mtime"`            // Unix nanoseconds
	Dirs    []string `json:"dirs,omitempty"`   // names of child directories that are walked
	Config  bool     `json:"config,omitempty"` // holds a regular .lanes.toml
}

type readDirFunc func(string) ([]fs.DirEntry, error)

// hashRootWith is HashRoot driven by the cache prev (nil for none), returning the cache for the tree as found at now.
func hashRootWith(root string, prev *walkCache, now time.Time, readDir readDirFunc) (string, *walkCache, error) {
	return "", nil, errors.New("not implemented")
}
