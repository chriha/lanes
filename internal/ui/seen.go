package ui

import (
	"maps"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chriha/lanes/internal/item"
)

// dwell is how long the cursor rests on an unread item before it counts as read.
const dwell = time.Second

// dwellMsg fires once the cursor rested on an unread item for dwell; gen drops the ticks of a cursor that moved on.
type dwellMsg struct {
	id  string
	gen int
}

// isUnread reports whether the item of provider changed since the user last saw it. Items of a provider whose
// first full result on the first run has not landed yet count as read: that result records them as seen.
// Without a loaded store (a model built without New) nothing is unread.
func (m *Model) isUnread(prov string, it item.Item) bool {
	return m.seen != nil && !m.seenBase[prov] && m.seen.Unread(it.ID, fingerprint(it))
}

// withUnread returns the item with unread = true in a copy of its fields (item.Env defaults it to false).
// A provider field called unread wins, as with note.
func withUnread(it item.Item, unread bool) item.Item {
	if !unread {
		return it
	}
	if _, ok := it.Fields["unread"]; ok {
		return it
	}
	f := make(map[string]any, len(it.Fields)+1)
	maps.Copy(f, it.Fields)
	f["unread"] = true
	it.Fields = f
	return it
}

// markRead records the items as seen at their current fingerprint.
func (m *Model) markRead(its ...item.Item) {
	fps := make(map[string]string, len(its))
	for _, it := range its {
		fps[it.ID] = fingerprint(it)
	}
	if err := m.seen.MarkRead(fps); err != nil {
		m.say(lvErr, "saving seen items: "+err.Error())
	}
}

// armDwell starts a dwell tick when the cursor came to rest on an unread item (a new one, or the same one
// turning unread); a tick armed before is dropped by its generation.
func (m *Model) armDwell() tea.Cmd {
	id := m.selectedID()
	if !m.unread[id] {
		id = ""
	}
	if id == m.dwellID {
		return nil
	}
	m.dwellID = id
	m.dwellGen++
	if id == "" {
		return nil
	}
	gen := m.dwellGen
	return tea.Tick(dwell, func(time.Time) tea.Msg { return dwellMsg{id: id, gen: gen} })
}

// dwelled marks the cursor item read when the tick is the current one and the cursor is still on it.
func (m *Model) dwelled(msg dwellMsg) {
	if msg.gen != m.dwellGen || m.selectedID() != msg.id || !m.unread[msg.id] {
		return
	}
	m.markRead(m.items()[m.cursor])
	m.rebuild()
}

// toggleRead is the m key: it marks the targets read when all of them are unread, otherwise unread. The cursor
// item counts as dwelled on, so resting on it doesn't undo the choice.
func (m *Model) toggleRead() {
	ts := m.targets()
	if len(ts) == 0 || m.seen == nil {
		return
	}
	ids := make([]string, len(ts))
	allUnread := true
	for i, it := range ts {
		ids[i] = it.ID
		allUnread = allUnread && m.unread[it.ID]
	}
	if allUnread {
		m.markRead(ts...)
		m.say(lvInfo, "read "+targetsLabel(ids))
	} else {
		if err := m.seen.MarkUnread(ids...); err != nil {
			m.say(lvErr, "saving seen items: "+err.Error())
		} else {
			m.say(lvInfo, "unread "+targetsLabel(ids))
		}
	}
	m.marks = map[string]bool{}
	m.rebuild()
	m.dwellID = m.selectedID()
	m.dwellGen++ // drops a tick armed before
}
