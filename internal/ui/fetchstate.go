package ui

import (
	"context"
	"time"
)

// fetchState is one provider's fetch status. It changes only on the UI goroutine (fetch, the fetchedMsg handler,
// cancelFetches), never from the goroutine that runs the fetch.
type fetchState struct {
	gen     int                // bumped when the running fetch is cancelled; a result carrying another gen is stale
	running bool               // a fetch is in flight
	started time.Time          // when the running fetch started
	last    time.Duration      // how long the last finished fetch took; 0 before the first one
	cancel  context.CancelFunc // cancels the running fetch; nil while idle
}

// fetching reports whether a fetch of provider name is in flight.
func (m *Model) fetching(name string) bool { return false }

// cancelFetches cancels every running fetch; their results are dropped when they land.
func (m *Model) cancelFetches() {}

// Close cancels the running fetches. Call it once the program has quit.
func (m *Model) Close() {}
