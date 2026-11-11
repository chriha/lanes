package trust

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// errStaleCache is a cache that disagrees with the tree; the walk is then redone without it.
var errStaleCache = errors.New("stale trust walk cache")

// hashRootWith is HashRoot driven by the cache prev (nil for none), returning the cache for the tree as found at now.
// prev only decides which directories are read: every found .lanes.toml is read fresh, and any error with prev
// redoes the whole walk without it, so a wrong cache can change what is read but never the hash.
func hashRootWith(root string, prev *walkCache, now time.Time, readDir readDirFunc) (string, *walkCache, error) {
	files, next, err := walk(root, prev, now, readDir)
	if err == nil {
		var h string
		if h, err = hashFiles(root, files); err == nil {
			return h, next, nil
		}
	}
	if prev == nil {
		return "", nil, err
	}
	return hashRootWith(root, nil, now, readDir)
}

// walk finds every .lanes.toml under root, skipping .git and node_modules below it. A directory in prev whose mtime
// is unchanged and older than racyWindow before prev was written is not read; its cached entry is used instead.
func walk(root string, prev *walkCache, now time.Time, readDir readDirFunc) ([]string, *walkCache, error) {
	next := &walkCache{Written: now.UnixNano(), Dirs: map[string]walkDir{}}
	if st, err := os.Lstat(root); err != nil {
		return nil, nil, err
	} else if !st.IsDir() {
		return nil, next, nil // filepath.WalkDir does not descend into a symlinked or non-directory root either
	}
	var files []string
	for queue := []string{"."}; len(queue) > 0; {
		rel := queue[0]
		queue = queue[1:]
		dir := filepath.Join(root, filepath.FromSlash(rel))
		st, err := os.Lstat(dir) // before the read, so a change during the read leaves a newer mtime
		if err != nil {
			return nil, nil, err
		}
		if !st.IsDir() {
			return nil, nil, fmt.Errorf("%s: %w", dir, errStaleCache)
		}
		mtime := st.ModTime().UnixNano()
		entry, ok := walkDir{}, false
		if prev != nil {
			var cached walkDir
			cached, ok = prev.Dirs[rel]
			ok = ok && cached.ModTime == mtime && mtime < prev.Written-int64(racyWindow)
			entry = cached
		}
		if !ok {
			entries, err := readDir(dir)
			if err != nil {
				return nil, nil, err
			}
			entry = walkDir{ModTime: mtime}
			for _, e := range entries {
				switch n := e.Name(); {
				case e.IsDir():
					if n != ".git" && n != "node_modules" {
						entry.Dirs = append(entry.Dirs, n)
					}
				case n == ".lanes.toml" && e.Type().IsRegular():
					entry.Config = true
				}
			}
		}
		next.Dirs[rel] = entry
		if entry.Config {
			files = append(files, filepath.Join(dir, ".lanes.toml"))
		}
		for _, n := range entry.Dirs {
			queue = append(queue, filepath.ToSlash(filepath.Join(rel, n)))
		}
	}
	return files, next, nil
}
