package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/item"
)

func TestCommandFetch(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	tests := []struct {
		name    string
		cmd     string
		n       int
		wantErr string
	}{
		{"valid", `printf '[{"id":"1","title":"T","url":"http://x","fields":{"a":2}}]'`, 1, ""},
		{"empty", `printf '[]'`, 0, ""},
		{"bad json", `printf 'nope'`, 0, "bad JSON"},
		{"failing", `echo boom >&2; exit 3`, 0, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := (&Command{Cmd: tt.cmd}).Fetch(context.Background())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != tt.n {
				t.Fatalf("got %d items, want %d", len(items), tt.n)
			}
			if tt.n == 1 {
				it := items[0]
				if it.ID != "1" || it.Title != "T" || it.URL != "http://x" || it.Fields["a"] != float64(2) {
					t.Fatalf("item = %+v", it)
				}
			}
		})
	}
}

func TestCommandFetchCancelled(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (&Command{Cmd: "exec sleep 5"}).Fetch(ctx)
	if err == nil {
		t.Fatal("want error")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %s, want prompt return", d)
	}
}

func TestCommandDetail(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	created := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	full := `{
  "description": "Fix the thing",
  "jobs": [{"name": "unit", "stage": "test", "status": "failed", "duration": 42}],
  "threads": [{"file": "a.go", "line": 7, "resolved": true,
    "comments": [{"author": "sam", "body": "why?", "created": "2026-11-01T10:00:00Z"}, {"author": "lee", "body": "because"}]}],
  "comments": [{"author": "alex", "body": "lgtm", "created": "2026-11-01T10:00:00Z"}]
}`
	tests := []struct {
		name    string
		script  string // body of the detail script; "" = no detail command
		want    item.Detail
		wantErr string
		noDet   bool
	}{
		{name: "no detail command", noDet: true},
		{name: "full", script: "cat <<'J'\n" + full + "\nJ\n", want: item.Detail{
			Description: "Fix the thing",
			Pipeline:    &item.Pipeline{Jobs: []item.Job{{Name: "unit", Stage: "test", Status: "failed", Duration: 42 * time.Second}}},
			Threads: []item.Thread{{File: "a.go", Line: 7, Resolved: true, Comments: []item.Comment{
				{Author: "sam", Body: "why?", Created: created}, {Author: "lee", Body: "because"}}}},
			Comments: []item.Comment{{Author: "alex", Body: "lgtm", Created: created}},
		}},
		{name: "item id in env", script: `printf '{"description":"%s"}' "$LANES_ITEM_ID"`, want: item.Detail{Description: "T-1"}},
		{name: "no jobs no pipeline", script: `printf '{"jobs":[]}'`, want: item.Detail{}},
		{name: "invalid json", script: `printf 'nope'`, wantErr: "bad JSON"},
		{name: "bad time", script: `printf '{"comments":[{"created":"yesterday"}]}'`, wantErr: "bad JSON"},
		{name: "failing", script: `echo boom >&2; exit 3`, wantErr: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Command{Cmd: "false"}
			if tt.script != "" {
				path := filepath.Join(t.TempDir(), "detail.sh")
				if err := os.WriteFile(path, []byte(tt.script+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				c.DetailCmd = path
			}
			d, err := c.Detail(context.Background(), item.Item{ID: "T-1"})
			switch {
			case tt.noDet:
				if !errors.Is(err, ErrNoDetail) {
					t.Fatalf("err = %v, want ErrNoDetail", err)
				}
				return
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			if !reflect.DeepEqual(d, tt.want) {
				t.Fatalf("detail = %+v\nwant     %+v", d, tt.want)
			}
		})
	}
}
