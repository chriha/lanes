package session

import "time"

// Tokens are the token counts of a Claude session's assistant messages.
type Tokens struct {
	Input      int
	CacheWrite int
	CacheRead  int
	Output     int
}

// TranscriptUsage sums the token usage of the assistant messages in the transcript at path written at or after
// since. ok is false when the transcript is missing or unreadable.
func TranscriptUsage(path string, since time.Time) (tok Tokens, ok bool) {
	return Tokens{}, false
}
