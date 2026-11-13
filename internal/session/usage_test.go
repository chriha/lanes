package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TranscriptUsage sums each assistant message's newest usage once, from the session's start on, and skips the lines
// it doesn't know.
func TestTranscriptUsage(t *testing.T) {
	path := filepath.Join("testdata", "usage.jsonl")
	tests := []struct {
		name  string
		since time.Time
		want  Tokens
	}{
		{"since the session's start", time.Date(2026, 11, 1, 10, 5, 0, 0, time.UTC),
			Tokens{Input: 15, CacheWrite: 150, CacheRead: 6000, Output: 65}},
		{"a cutoff on a message's own time counts it", time.Date(2026, 11, 1, 10, 7, 0, 0, time.UTC),
			Tokens{Input: 3, CacheWrite: 30, CacheRead: 3000, Output: 8}},
		{"no start time counts all", time.Time{},
			Tokens{Input: 1015, CacheWrite: 1150, CacheRead: 7000, Output: 1065}},
		{"after the last message", time.Date(2026, 11, 1, 11, 0, 0, 0, time.UTC), Tokens{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := TranscriptUsage(path, tt.since)
			if !ok || got != tt.want {
				t.Errorf("TranscriptUsage = %+v, %v; want %+v, true", got, ok, tt.want)
			}
		})
	}
}

// A transcript that is missing or can't be read gives no usage.
func TestTranscriptUsageUnreadable(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		path string
	}{
		{"no path", ""},
		{"missing file", filepath.Join(dir, "gone.jsonl")},
		{"a directory", dir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := TranscriptUsage(tt.path, time.Time{}); ok || got != (Tokens{}) {
				t.Errorf("TranscriptUsage = %+v, %v; want no usage", got, ok)
			}
		})
	}
}

// A half-written last line is skipped rather than failing the whole transcript.
func TestTranscriptUsageHalfWrittenLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	line := `{"type":"assistant","timestamp":"2026-11-01T10:00:00Z","message":{"id":"m1","usage":{"input_tokens":1,"cache_creation_input_tokens":2,"cache_read_input_tokens":3,"output_tokens":4}}}`
	if err := os.WriteFile(path, []byte(line+"\n"+`{"type":"assistant","message":{"id":"m2","us`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := TranscriptUsage(path, time.Time{})
	if want := (Tokens{Input: 1, CacheWrite: 2, CacheRead: 3, Output: 4}); !ok || got != want {
		t.Errorf("TranscriptUsage = %+v, %v; want %+v, true", got, ok, want)
	}
}

// A later line of a message without usage keeps what the earlier lines counted; a message seen only on usage-less
// lines counts as zero.
func TestTranscriptUsageLineWithoutUsage(t *testing.T) {
	withUsage := func(id string, in, out int) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":"2026-11-01T10:00:00Z","message":{"id":%q,"usage":{"input_tokens":%d,"output_tokens":%d}}}`, id, in, out)
	}
	noUsage := func(id string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":"2026-11-01T10:00:00Z","message":{"id":%q}}`, id)
	}
	tests := []struct {
		name  string
		lines []string
		want  Tokens
	}{
		{"usage-less line after the usage line", []string{withUsage("m1", 5, 7), noUsage("m1")}, Tokens{Input: 5, Output: 7}},
		{"usage-less line between usage lines", []string{withUsage("m1", 5, 7), noUsage("m1"), withUsage("m1", 6, 9)}, Tokens{Input: 6, Output: 9}},
		{"message only on a usage-less line", []string{noUsage("m1")}, Tokens{}},
		{"usage-less message beside a counted one", []string{withUsage("m1", 5, 7), noUsage("m2")}, Tokens{Input: 5, Output: 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "t.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tt.lines, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, ok := TranscriptUsage(path, time.Time{})
			if !ok || got != tt.want {
				t.Errorf("TranscriptUsage = %+v, %v; want %+v, true", got, ok, tt.want)
			}
		})
	}
}
