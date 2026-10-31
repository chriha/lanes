package ui

import (
	"fmt"
	"maps"
	"time"

	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/snooze"
)

// withSnooze returns the item with snoozed = true in a copy of its fields when it is snoozed (item.Env defaults
// it to false). The provider's data is left alone (it is cached).
func (m *Model) withSnooze(it item.Item) item.Item {
	if _, ok := m.snoozes[it.ID]; !ok {
		return it
	}
	if _, ok := it.Fields["snoozed"]; ok {
		return it // a provider field called snoozed wins, as with note
	}
	f := make(map[string]any, len(it.Fields)+1)
	maps.Copy(f, it.Fields)
	f["snoozed"] = true
	it.Fields = f
	return it
}

// fingerprint is the item's fingerprint as a full fetch would compute it: a partial result's pending fields hold
// inherited values, so they count like any other field.
func fingerprint(it item.Item) string {
	it.Pending = nil
	return it.Fingerprint()
}

// toggleSnooze is the z key: it unsnoozes the targets when all of them are snoozed, otherwise opens the prompt.
func (m *Model) toggleSnooze() bool {
	ts := m.targets()
	if len(ts) == 0 {
		return false
	}
	ids := make([]string, len(ts))
	for i, it := range ts {
		if _, ok := m.snoozes[it.ID]; !ok {
			return true
		}
		ids[i] = it.ID
	}
	if err := m.snoozes.Delete(ids...); err != nil {
		m.say(lvErr, "unsnooze: "+err.Error())
		return false
	}
	m.marks = map[string]bool{}
	m.say(lvInfo, "unsnoozed "+targetsLabel(ids))
	m.rebuild()
	return false
}

// snoozeTargets snoozes the marked items, or the cursor item, until the time input names, or until they change
// when it is empty (:snooze, the z prompt).
func (m *Model) snoozeTargets(input string) {
	until, err := snooze.Parse(input, time.Now())
	if err != nil {
		m.say(lvErr, err.Error())
		return
	}
	ts := m.targets()
	if len(ts) == 0 {
		m.say(lvWarn, "no item to snooze")
		return
	}
	ids := make([]string, len(ts))
	for i, it := range ts {
		ids[i] = it.ID
		if until.IsZero() {
			m.snoozes[it.ID] = snooze.Entry{Fingerprint: fingerprint(it)} // per item: saved below
		}
	}
	if until.IsZero() {
		err = m.snoozes.Delete() // no IDs: just writes the entries set above
	} else {
		err = m.snoozes.Set(snooze.Entry{Until: until}, ids...)
	}
	if err != nil {
		m.say(lvErr, "saving snoozes: "+err.Error())
		m.rebuild()
		return
	}
	m.marks = map[string]bool{}
	m.say(lvInfo, "snoozed "+targetsLabel(ids)+" until it changes")
	if !until.IsZero() {
		m.say(lvInfo, "snoozed "+targetsLabel(ids)+" until "+until.Format("Mon 2006-01-02 15:04"))
	}
	m.rebuild()
}

func targetsLabel(ids []string) string {
	if len(ids) == 1 {
		return ids[0]
	}
	return fmt.Sprintf("%d items", len(ids))
}

// wake removes the snoozes that are over: timed ones whose time has come and, with fetched (a full result),
// change ones whose item came back with a different fingerprint. It reports whether any woke.
// ponytail: a change snooze on an item that never comes back stays in snoozes.json; prune by age if it grows.
func (m *Model) wake(now time.Time, fetched []item.Item) bool {
	var woke []string
	for id, e := range m.snoozes {
		if !e.Until.IsZero() && e.Awake(now, "") {
			woke = append(woke, id)
		}
	}
	for _, it := range fetched {
		if e, ok := m.snoozes[it.ID]; ok && e.Until.IsZero() && e.Awake(now, fingerprint(it)) {
			woke = append(woke, it.ID)
		}
	}
	if len(woke) == 0 {
		return false
	}
	if err := m.snoozes.Delete(woke...); err != nil {
		m.say(lvErr, "saving snoozes: "+err.Error())
	}
	return true
}
