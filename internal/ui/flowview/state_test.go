package flowview

import (
	"fmt"
	"testing"
	"time"

	"github.com/chriha/lanes/internal/session"
)

func TestShortModel(t *testing.T) {
	tests := map[string]string{
		"claude-opus-4-1-20250805":  "opus 4.1",
		"claude-sonnet-4-5[1m]":     "sonnet 4.5",
		"claude-3-5-haiku-20241022": "haiku 3.5",
		"claude-haiku-4-5-20251001": "haiku 4.5",
		"gpt-x":                     "gpt-x",
		"":                          "",
		"claude-opus-5-5":           "opus 5.5",
	}
	for in, want := range tests {
		if got := shortModel(in); got != want {
			t.Errorf("shortModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTopTools(t *testing.T) {
	s := State{Snap: session.Snapshot{Lead: session.Lead{
		Calls:     10,
		ToolCalls: map[string]int{"Read": 4, "Bash": 4, "Edit": 1, "Grep": 1},
	}}}
	if got := fmt.Sprint(s.topTools(3)); got != "[{Bash 0.4} {Read 0.4} {Edit 0.1}]" {
		t.Errorf("topTools = %s", got)
	}
	if got := (State{}).topTools(3); len(got) != 0 {
		t.Errorf("topTools without calls = %v, want none", got)
	}
}

func TestFormats(t *testing.T) {
	durs := map[time.Duration]string{
		-time.Second:                   "0s",
		42*time.Second + 900e6:         "42s",
		12*time.Minute + 3*time.Second: "12m03s",
		62 * time.Minute:               "1h02m",
	}
	for d, want := range durs {
		if got := fmtDur(d); got != want {
			t.Errorf("fmtDur(%v) = %q, want %q", d, got, want)
		}
	}
	counts := map[int]string{950: "950", 12_345: "12.3k", 1_234_567: "1.2M"}
	for n, want := range counts {
		if got := fmtCount(n); got != want {
			t.Errorf("fmtCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSinceUnknownTimes(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		start, end time.Time
		want       string
	}{
		{"both known", t0, t0.Add(90 * time.Second), "1m30s"},
		{"no start", time.Time{}, t0, ""},
		{"no end", t0, time.Time{}, ""},
	}
	for _, tt := range tests {
		if got := since(tt.start, tt.end); got != tt.want {
			t.Errorf("%s: since = %q, want %q", tt.name, got, tt.want)
		}
	}
}
