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

// finish ends the running fetch: it records how long it took and releases its context.
func (st *fetchState) finish() {
	if st.running {
		st.last = time.Since(st.started)
	}
	if st.cancel != nil {
		st.cancel()
	}
	st.running, st.cancel = false, nil
}

// fetchStateOf returns the fetch status of provider name, creating it on first use.
func (m *Model) fetchStateOf(name string) *fetchState {
	if m.fetches == nil {
		m.fetches = map[string]*fetchState{}
	}
	st := m.fetches[name]
	if st == nil {
		st = &fetchState{}
		m.fetches[name] = st
	}
	return st
}

// fetching reports whether a fetch of provider name is in flight.
func (m *Model) fetching(name string) bool {
	st := m.fetches[name]
	return st != nil && st.running
}

// cancelFetches cancels every running fetch; their results are dropped when they land.
func (m *Model) cancelFetches() {
	for name, st := range m.fetches {
		if !st.running {
			continue
		}
		if st.cancel != nil {
			st.cancel()
		}
		st.running, st.cancel = false, nil
		st.gen++
		delete(m.refetchAfter, name) // the fetch replacing it starts after any refresh run that waited on this one
	}
}

// Close cancels the running fetches. Call it once the program has quit.
func (m *Model) Close() { m.cancelFetches() }
