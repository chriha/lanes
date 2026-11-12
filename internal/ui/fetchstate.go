package ui

import (
	"context"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// fetchStatus is the status line of provider name at now: its phase (cached, fetching with elapsed time, failed,
// updated) and the duration of its last finished fetch.
func (m *Model) fetchStatus(name string, now time.Time) string {
	st := m.fetches[name]
	var s string
	switch {
	case st != nil && st.running:
		el := now.Sub(st.started)
		s = name + " " + scanFrames[int(el/spinEvery)%len(scanFrames)] + " fetching " + el.Round(time.Second).String()
		if st.last > 0 {
			return s + " · last " + fetchDur(st.last)
		}
		return s
	case m.provErr[name] != nil:
		s = name + " ✗ failed"
	case m.fetchedAt[name].IsZero():
		return name + " cached"
	default:
		s = name + " updated " + now.Sub(m.fetchedAt[name]).Round(time.Second).String() + " ago"
	}
	if st != nil && st.last > 0 {
		s += " · " + fetchDur(st.last)
	}
	return s
}

// scanFrames animate a running fetch: a dot sweeping back and forth in a fixed-width track, one frame per spinEvery
// of the fetch.
var scanFrames = []string{"[●    ]", "[ ●   ]", "[  ●  ]", "[   ● ]", "[    ●]", "[   ● ]", "[  ●  ]", "[ ●   ]"}

// spinEvery is how long a scanFrames frame shows, and the interval of the spinTickMsg loop that redraws it.
const spinEvery = 100 * time.Millisecond

// armSpin starts the animation tick loop when a provider fetches and no loop runs yet; nil otherwise. The loop
// ends at the first spinTickMsg that finds every provider idle, so at most one runs and none while all are idle.
func (m *Model) armSpin() tea.Cmd {
	if m.spinning {
		return nil
	}
	for _, st := range m.fetches {
		if st.running {
			m.spinning = true
			return tea.Tick(spinEvery, func(t time.Time) tea.Msg { return spinTickMsg(t) })
		}
	}
	return nil
}

// fetchDur is a fetch duration for the status: tenths of a second from 1s, milliseconds below.
func fetchDur(d time.Duration) string {
	if d >= time.Second {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Millisecond).String()
}

// providerStatuses renders the status of every provider, sorted by name; failed ones in the error style. A view's
// provider that failed to build counts too.
func (m *Model) providerStatuses(now time.Time) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for name := range m.providers {
		add(name)
	}
	for _, v := range m.views {
		if !v.queue {
			add(v.cfg.Provider)
		}
	}
	sort.Strings(names)
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = m.styledStatus(name, now)
	}
	return out
}

// styledStatus is fetchStatus in the dim style, or the error style while the provider's last fetch failed.
func (m *Model) styledStatus(name string, now time.Time) string {
	if !m.fetching(name) && m.provErr[name] != nil {
		return sErr.Render(m.fetchStatus(name, now))
	}
	return sDim.Render(m.fetchStatus(name, now))
}
