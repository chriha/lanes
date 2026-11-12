package ui

import (
	"fmt"
	"strings"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/item"
)

// queueName is the name of the built-in view that lists the auto queue. It is always the last view, is not
// configurable and is reached with Q.
const queueName = "queue"

// newQueueView is the built-in view of the auto queue; its items are the queue's entries, see syncQueue.
func newQueueView() *view { return &view{cfg: config.View{Name: queueName}, queue: true} }

// onQueue says whether the current view is the queue view.
func (m *Model) onQueue() bool { return m.cur < len(m.views) && m.views[m.cur].queue }

// syncQueue lists the entries of the auto queue in the queue view, oldest first. On the queue view the search
// applies; on_enter entries whose item is no longer listed by their view are left out (autoFetched drops them).
func (m *Model) syncQueue() {
	for i, v := range m.views {
		if !v.queue {
			continue
		}
		v.items, v.members = v.items[:0], nil
		for _, e := range m.auto.queue {
			it, ok := m.autoLookup(e)
			if !ok {
				continue
			}
			v.members = append(v.members, it)
			if i == m.cur && m.query != "" && !matches(it, strings.ToLower(m.query)) {
				continue
			}
			v.items = append(v.items, it)
		}
	}
}

// refreshQueue is syncQueue for a queue change outside rebuild: the cursor sticks to its item.
func (m *Model) refreshQueue() {
	selected := ""
	if m.onQueue() {
		selected = m.selectedID()
	}
	m.syncQueue()
	if m.onQueue() {
		m.restoreCursor(selected)
	}
}

// dropQueued removes the queue entries of the marked items, or of the item under the cursor. It leaves
// sessions alone and does not ask.
func (m *Model) dropQueued() {
	drop := map[string]bool{}
	for _, it := range m.targets() {
		drop[it.ID] = true
	}
	keep := m.auto.queue[:0]
	n := 0
	for _, e := range m.auto.queue {
		if drop[e.id] {
			n++
			delete(m.marks, e.id)
			continue
		}
		keep = append(keep, e)
	}
	m.auto.queue = keep
	m.refreshQueue()
	if n == 0 {
		m.say(lvWarn, "nothing to drop")
		return
	}
	m.say(lvInfo, fmt.Sprintf("dropped %d from the queue", n))
}

// queueProvider is the provider of the view that queued the item with id, "" when it is not queued.
func (m *Model) queueProvider(it item.Item) string {
	for _, e := range m.auto.queue {
		if e.id != it.ID {
			continue
		}
		if e.manual {
			return e.prov
		}
		for _, v := range m.views {
			if !v.queue && v.cfg.Name == e.view {
				return v.cfg.Provider
			}
		}
	}
	return ""
}
