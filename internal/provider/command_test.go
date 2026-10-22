package provider

import (
	"context"
	"strings"
	"testing"
	"time"
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
