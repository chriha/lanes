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

func TestFmtThousands(t *testing.T) {
	tests := map[int]string{0: "0", 999: "999", 1000: "1,000", 12_345: "12,345", 1_234_567: "1,234,567", -4200: "-4,200"}
	for n, want := range tests {
		if got := fmtThousands(n); got != want {
			t.Errorf("fmtThousands(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestActivity(t *testing.T) {
	ago := func(d time.Duration) time.Time { return t0.Add(-d) }
	at := func(idx, n int) []int { // sparkMinutes counts, n at idx
		c := make([]int, sparkMinutes)
		c[idx] = n
		return c
	}
	tests := []struct {
		name  string
		times []time.Time
		want  []int
	}{
		{"no calls", nil, make([]int, sparkMinutes)},
		{"calls within the last minute count in the newest cell", []time.Time{ago(0), ago(10 * time.Second), ago(59 * time.Second)}, at(14, 3)},
		{"one minute ago is the cell before", []time.Time{ago(time.Minute), ago(119 * time.Second)}, at(13, 2)},
		{"the oldest minute is the first cell", []time.Time{ago(14*time.Minute + 59*time.Second)}, at(0, 1)},
		{"calls 15 minutes ago or older are dropped", []time.Time{ago(15 * time.Minute), ago(time.Hour)}, make([]int, sparkMinutes)},
		{"calls after now count as now", []time.Time{t0.Add(5 * time.Second)}, at(14, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := activity(tt.times, t0); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("activity = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShare5h(t *testing.T) {
	agents := func(tokens ...int) []session.Agent {
		out := make([]session.Agent, len(tokens))
		for i, k := range tokens {
			out[i] = session.Agent{ID: fmt.Sprint(i), Tokens: k}
		}
		return out
	}
	tests := []struct {
		name   string
		pct    int
		known  bool
		lead   int
		agents []session.Agent
		want   []int // the lead's share, then each agent's
		wantOK bool
	}{
		{"unknown usage", 40, false, 100, agents(100), []int{0, 0}, false},
		{"shares by tokens", 40, true, 300, agents(100), []int{30, 10}, true},
		{"lead alone has it all", 12, true, 5_000, nil, []int{12}, true},
		{"rounded to whole percent", 10, true, 2, agents(1), []int{7, 3}, true},
		{"no tokens yet is 0, not a division by zero", 40, true, 0, agents(0, 0), []int{0, 0, 0}, true},
		{"zero usage", 0, true, 300, agents(100), []int{0, 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := State{Usage5h: tt.pct, Usage5hKnown: tt.known, Snap: session.Snapshot{Lead: session.Lead{Tokens: tt.lead}, Agents: tt.agents}}
			got := make([]int, 0, len(tt.want))
			pct, ok := s.share5h(tt.lead)
			got = append(got, pct)
			for _, a := range tt.agents {
				p, aok := s.share5h(a.Tokens)
				if aok != ok {
					t.Fatalf("ok differs between actors: lead %v, agent %v", ok, aok)
				}
				got = append(got, p)
			}
			if ok != tt.wantOK || fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("shares = %v, %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestShare5hSumsToTheAccountUsage(t *testing.T) {
	// Each share is rounded on its own: the sum is the account's usage within half a percent per actor.
	for _, tokens := range [][]int{{1_234_567, 412_345, 23_456, 98_765, 5_432}, {1, 1, 1}, {10, 20, 30, 40}, {999, 1}} {
		for _, pct := range []int{0, 1, 7, 33, 100} {
			s := State{Usage5h: pct, Usage5hKnown: true, Snap: session.Snapshot{Lead: session.Lead{Tokens: tokens[0]}}}
			for i, k := range tokens[1:] {
				s.Snap.Agents = append(s.Snap.Agents, session.Agent{ID: fmt.Sprint(i), Tokens: k})
			}
			sum := 0
			for _, k := range tokens {
				p, _ := s.share5h(k)
				sum += p
			}
			if d := float64(sum - pct); d > float64(len(tokens))/2 || d < -float64(len(tokens))/2 {
				t.Errorf("tokens %v at %d%%: shares sum to %d", tokens, pct, sum)
			}
		}
	}
}
