package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// Tokens are the token counts of a Claude session's assistant messages.
type Tokens struct {
	Input      int
	CacheWrite int
	CacheRead  int
	Output     int
}

// tokenLine is the part of a transcript line TranscriptUsage reads.
type tokenLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage usage  `json:"usage"`
	} `json:"message"`
}

// TranscriptUsage sums the token usage of the assistant messages in the transcript at path written at or after
// since (all of them when since is zero): a resumed session appends to its predecessor's transcript. One message
// spans several lines; its newest line's usage counts, once. ok is false when the transcript is missing or unreadable.
// It reads the file as a stream, so a long transcript costs time but not memory: call it off the UI goroutine.
func TranscriptUsage(path string, since time.Time) (tok Tokens, ok bool) {
	if path == "" {
		return Tokens{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return Tokens{}, false
	}
	defer f.Close()
	msgs := map[string]usage{}
	r := bufio.NewReader(f)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			var l tokenLine
			if json.Unmarshal(b, &l) == nil && l.Type == "assistant" && l.Message.ID != "" &&
				l.Message.Model != "<synthetic>" && !l.Timestamp.Before(since) {
				// A line without usage carries nothing to count: a known message keeps what it has.
				if _, known := msgs[l.Message.ID]; !known || l.Message.Usage != (usage{}) {
					msgs[l.Message.ID] = l.Message.Usage
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Tokens{}, false
		}
	}
	for _, u := range msgs {
		tok.Input += u.Input
		tok.CacheWrite += u.CacheWrite
		tok.CacheRead += u.CacheRead
		tok.Output += u.Output
	}
	return tok, true
}
