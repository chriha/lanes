package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/trust"
)

// benchItem is the title of the one item the fake provider lists; the first list frame must show it.
const benchItem = "bench-first-list-item"

// benchProv lists benchItem at once, in place of the configured command provider.
type benchProv struct{}

func (benchProv) Fetch(context.Context) ([]item.Item, error) {
	return []item.Item{{ID: "1", Title: benchItem, Fields: map[string]any{}}}, nil
}

// benchConfig is the global config the benchmark loads: one provider and one view of it.
const benchConfig = `[provider.bench]
type = "command"
command = "false"

[[view]]
name = "bench"
provider = "bench"
`

// benchTree makes a project tree under root for the trust walk: nested source dirs, a few nested .lanes.toml
// files, and a node_modules and .git the walk skips.
func benchTree(b *testing.B, root string) {
	b.Helper()
	write := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	write(filepath.Join(root, config.ProjectFile), "# project\n")
	for d := 0; d < 20; d++ {
		dir := filepath.Join(root, "pkg", fmt.Sprintf("d%02d", d), "sub")
		for f := 0; f < 25; f++ {
			write(filepath.Join(dir, fmt.Sprintf("f%02d.go", f)), "package sub\n")
		}
		if d%5 == 0 {
			write(filepath.Join(root, "pkg", fmt.Sprintf("d%02d", d), config.ProjectFile), "# nested\n")
		}
	}
	for _, skipped := range []string{"node_modules", ".git"} {
		for f := 0; f < 100; f++ {
			write(filepath.Join(root, skipped, fmt.Sprintf("x%03d", f)), "x\n")
		}
	}
}

// benchStartup isolates the benchmark's state, builds and trusts the fixture tree, makes it the working directory
// and returns its root.
func benchStartup(b *testing.B) string {
	b.Helper()
	for _, k := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "HOME"} {
		b.Setenv(k, b.TempDir())
	}
	b.Setenv("SHELL", "/bin/sh")
	cfgPath := config.Path()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(benchConfig), 0o600); err != nil {
		b.Fatal(err)
	}
	root := b.TempDir()
	benchTree(b, root)
	hash, err := trust.HashRoot(root)
	if err != nil {
		b.Fatal(err)
	}
	if err := trust.Trust(root, hash); err != nil { // trusted by hash, not always: Check really walks the tree
		b.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	if err := os.Chdir(root); err != nil { // config.Load finds the project's .lanes.toml from the working directory
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			b.Error(err)
		}
	})
	return root
}

// benchLoad is runTUI's load step (config load and New) with the fake provider in place of the configured one;
// sessions are never polled, so tmux is never run.
func benchLoad() (*Model, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	m := New(cfg)
	m.providers = map[string]provider.Provider{"bench": benchProv{}}
	m.polling = true
	return m, nil
}

// firstList runs one start from Init to the first frame that lists benchItem, executing the commands as the
// program would (each in its own goroutine) and feeding their messages back into Update.
func firstList(b *testing.B, root string) {
	s := NewStartup(root, benchLoad)
	defer s.Close()
	msgs := make(chan tea.Msg, 16)
	done := make(chan struct{})
	defer close(done) // drops the messages of commands still running, such as the ticks
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		go func() {
			msg := cmd()
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, c := range batch {
					run(c)
				}
				return
			}
			select {
			case msgs <- msg:
			case <-done:
			}
		}()
	}

	trusted := s.Init()() // the trust walk; the checking frame is already drawn before it
	s.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	_, cmd := s.Update(trusted)
	if err := s.Err(); err != nil {
		b.Fatalf("startup ended: %v", err)
	}
	run(cmd)
	deadline := time.After(10 * time.Second)
	for !strings.Contains(s.View(), benchItem) {
		select {
		case msg := <-msgs:
			_, cmd := s.Update(msg)
			run(cmd)
		case <-deadline:
			b.Fatalf("the first list never showed %q; last frame:\n%s", benchItem, s.View())
		}
	}
}

// BenchmarkTimeToFirstList measures the startup goal (docs/design.md): from Startup.Init over the trust walk of a
// fixture tree, the config load and New to the first rendered list of a fake provider's item.
func BenchmarkTimeToFirstList(b *testing.B) {
	root := benchStartup(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		firstList(b, root)
	}
}
