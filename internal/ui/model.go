// Package ui is the Bubble Tea front end: views, list navigation, search, command line.
package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/lanes/internal/app"
	"github.com/chriha/lanes/internal/cache"
	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
	"github.com/chriha/lanes/internal/notes"
	"github.com/chriha/lanes/internal/provider"
	"github.com/chriha/lanes/internal/seen"
	"github.com/chriha/lanes/internal/session"
	"github.com/chriha/lanes/internal/snooze"
	"github.com/chriha/lanes/internal/termbg"
	"github.com/chriha/lanes/internal/update"
)

type mode int

const (
	modeNormal mode = iota
	modeSearch
	modeCommand
	modeHelp
	modeConfirm
	modeHistory
	modeSessions
	modeAgents
	modeDetail
)

type view struct {
	cfg    config.View
	filter *filter.Filter
	err    error
	items  []item.Item
	sort   []item.SortKey
	edited bool             // filter overridden from inside the TUI
	groups []*filter.Filter // compiled cfg.Groups, same order
	heads  []head           // non-empty groups, in items order; nil without groups
	// snoozed counts the snoozed items the filter matches (hidden unless showSnoozed)
	snoozed int
	// members are the items the filter matches, snoozed or not and whatever the search: the items on_enter sees
	members []item.Item
	queue   bool // the built-in queue view: its items are the auto queue's entries
}

// head is a group header in the list: items[at:at+n] belong to the group name.
type head struct {
	at, n int
	name  string
}

type fetchedMsg struct {
	name    string
	gen     int // fetchState.gen when the fetch started
	items   []item.Item
	err     error
	partial bool // quick result; a full one follows
}
type tickMsg time.Time

// spinTickMsg advances the fetch animation; its loop runs only while a provider fetches.
type spinTickMsg time.Time
type usageMsg struct {
	text string
	at   time.Time
}

// usage5hMsg carries the account's 5h usage in percent for the agent view; ok is false when unknown or stale.
type usage5hMsg struct {
	pct int
	ok  bool
}

// usageStale is the age after which the Claude usage shows how old it is.
const usageStale = 10 * time.Minute

type Model struct {
	cfg       *config.Config
	cfgMod    time.Time
	providers map[string]provider.Provider
	data      map[string][]item.Item
	fetchedAt map[string]time.Time
	// fullFetched marks providers with a full result in this run; their polls skip the Quick pass.
	fullFetched map[string]bool
	fetches     map[string]*fetchState // by provider; created on first fetch
	spinning    bool                   // a spinTickMsg loop is running
	provErr     map[string]error
	fails       map[string]int       // consecutive failed fetches per provider
	retryAt     map[string]time.Time // no refetch before this
	// blurred is set while the terminal reports it lost focus; providers then poll at poll_interval_blurred
	blurred bool
	// refetchAfter marks providers whose in-flight fetch started before a refresh run ended: when it lands, fetch again
	refetchAfter map[string]bool
	busy         map[string]busyState // by session.Key of the item
	auto         autoState            // the views' on_enter actions

	views  []*view
	cur    int
	cursor int
	offset int
	marks  map[string]bool

	mode     mode
	search   textinput.Model
	query    string
	cmdline  textinput.Model
	pendingG bool
	notice   string
	level    level                               // how the footer draws notice
	clip     func(string) error                  // nil: the system clipboard (tests fake it)
	bgOut    io.Writer                           // where the background query goes; nil: stdout if a terminal (tests fake it)
	getenv   func(string) string                 // reads TERM and TMUX for the background query; nil: os.Getenv (tests fake it)
	bg       bgCapture                           // the focus-time background query and its reply
	open     func(string) error                  // nil: the system opener (tests fake it)
	sampler  func() func(pid int) []session.Proc // nil: a ps sampler per agent view (tests fake it)

	sess       *session.Manager
	settings   string
	sessions   []session.Info
	sessByItem map[string][]session.Info
	polling    bool
	confirm    *confirmState
	histTitle  string
	hist       []string
	helpOff    int       // scroll offset of the help and detail screens
	detail     item.Item // the item the detail screen shows
	sessCursor int       // cursor of the sessions screen
	lastAttn   string    // session the n key jumped to last
	// interrupted holds the sessions X interrupted in this run; X on one of them again sends SIGKILL
	interrupted map[string]bool
	agents      *agentView // the open agent view, nil when closed
	agentGen    int        // counts the agent views opened, to drop the polls of closed ones
	ov          *config.Overrides
	notes       notes.Store
	snoozes     snooze.Store
	seen        seen.Store
	// seenBase holds, on the first run with seen.json, the providers whose first full result is still to be
	// recorded as seen; their items count as read until then
	seenBase map[string]bool
	// unread holds the IDs of the listed items that changed since the user last saw them (set by rebuild)
	unread map[string]bool
	// dwellID is the unread cursor item a dwell tick is armed for, "" for none
	dwellID string
	// dwellGen counts the dwell ticks armed, to drop the ticks of a cursor that moved on
	dwellGen int
	// showSnoozed lists snoozed items too (:snoozed)
	showSnoozed bool
	preview_    bool
	usage       usageMsg   // Claude usage from [settings] claude_dir, shown at the left of the footer
	usage5h     usage5hMsg // the account's 5h usage from the same file, for the agent view
	// originForge detects the forge for a new .lanes.toml (:edit project); nil = provider.OriginForge
	originForge func() (typ, repo string, err error)
	// updates checks for a newer release at start; nil = no check (see WithUpdateCheck)
	updates *update.Checker
	upgrade string // the upgrade command the update notice shows

	// reloadGen counts the started config reloads; a reloadedMsg of an older one is dropped
	reloadGen int

	// details caches the fetched details by detailKey, in memory only; an entry is reused while its item's
	// fingerprint is unchanged
	details map[string]*detailEntry
	// detailKey is the details key of the item the detail screen shows, "" when its provider fetches none
	detailKey string
	// thread is the thread cursor of the detail screen, an index into the shown detail's threads
	thread int
	// folded holds the threads folded (true) or unfolded by hand, by foldKey, in memory only; the others are
	// folded when resolved
	folded map[string]bool
	// pendingZ is set after z on the detail screen, for za/zA
	pendingZ bool

	width, height int
}

func New(cfg *config.Config, opts ...Option) *Model {
	m := &Model{
		data:         map[string][]item.Item{},
		fetchedAt:    map[string]time.Time{},
		provErr:      map[string]error{},
		fails:        map[string]int{},
		retryAt:      map[string]time.Time{},
		busy:         map[string]busyState{},
		refetchAfter: map[string]bool{},
		marks:        map[string]bool{},
		search:       textinput.New(),
		cmdline:      textinput.New(),
		preview_:     cfg.Settings.Preview,
	}
	m.search.Prompt, m.cmdline.Prompt = "/", ":"
	m.search.PromptStyle, m.cmdline.PromptStyle = sKey, sKey
	var err error
	if m.ov, err = config.LoadOverrides(); err != nil {
		m.say(lvErr, err.Error())
	}
	if m.sess, err = session.NewManager(); err != nil {
		m.say(lvErr, strings.TrimSpace(m.notice+" "+err.Error()))
	}
	if m.notes, err = notes.Load(); err != nil {
		m.say(lvErr, strings.TrimSpace(m.notice+" notes: "+err.Error()))
	}
	if m.snoozes, err = snooze.Load(); err != nil {
		m.say(lvErr, strings.TrimSpace(m.notice+" snoozes: "+err.Error()))
	}
	var first bool
	if m.seen, first, err = seen.Load(); err != nil {
		m.say(lvErr, strings.TrimSpace(m.notice+" seen: "+err.Error()))
	}
	if first {
		m.seenBase = map[string]bool{}
		for name := range cfg.Provider {
			m.seenBase[name] = true
		}
	}
	m.settings, _ = session.SettingsFile()
	m.sessByItem = map[string][]session.Info{}
	m.applyConfig(cfg)
	for _, o := range opts {
		o(m)
	}
	return m
}

// buildProviders builds the providers of cfg; it returns the build error of each one that failed.
func buildProviders(cfg *config.Config) (map[string]provider.Provider, map[string]error) {
	provs, errs := map[string]provider.Provider{}, map[string]error{}
	for name, pc := range cfg.Provider {
		p, err := provider.New(pc)
		if err != nil {
			errs[name] = err
			continue
		}
		provs[name] = p
	}
	return provs, errs
}

func (m *Model) applyConfig(cfg *config.Config) {
	provs, errs := buildProviders(cfg)
	m.applyBuilt(cfg, provs, errs)
	m.cfgMod = config.Mtime()
}

// applyBuilt applies cfg with its providers already built (errs: the providers whose build failed). It leaves
// cfgMod alone: a reload set it when it started, so a change made while it ran is reloaded again.
func (m *Model) applyBuilt(cfg *config.Config, provs map[string]provider.Provider, errs map[string]error) {
	m.cfg = cfg
	applyTheme(cfg.Theme)
	m.providers = map[string]provider.Provider{}
	for name, err := range errs {
		m.provErr[name] = err
	}
	for name, p := range provs {
		m.providers[name] = p
		delete(m.fullFetched, name) // a rebuilt provider has no snapshot: show the partial result first again
		delete(m.provErr, name)     // built again: drop the stale build error until the next fetch
		if _, ok := m.data[name]; !ok {
			m.data[name] = item.Sanitize(cache.Load(name))
		}
	}
	for k, b := range m.busy {
		for name := range b.refetch {
			if _, known := m.providers[name]; !known {
				delete(b.refetch, name) // a provider gone from the config never lands a fetch
			}
		}
		if b.refetch != nil && len(b.refetch) == 0 {
			delete(m.busy, k)
		}
	}
	prev, prevQueue := "", m.onQueue()
	if m.cur < len(m.views) {
		prev = m.views[m.cur].cfg.Name
	}
	m.views = m.views[:0]
	m.cur = 0
	m.auto.seen, m.auto.left = nil, nil // the next full fetch of each view seeds them again
	all := append([]config.View{}, cfg.View...)
	seen := map[string]bool{}
	for _, vc := range all {
		seen[vc.Name] = true
	}
	for _, vc := range m.ov.Views {
		if _, known := cfg.Provider[vc.Provider]; known && !seen[vc.Name] {
			all = append(all, vc)
		}
	}
	for i, vc := range all {
		edited := false
		if f, ok := m.ov.Filters[vc.Name]; ok && i < len(cfg.View) {
			vc.Filter, edited = f, true
		}
		f, err := filter.Compile(vc.Filter)
		keys, serr := item.ParseSort(vc.Sort)
		if err == nil {
			err = serr
		}
		var groups []*filter.Filter
		for _, g := range vc.Groups {
			gf, gerr := filter.Compile(g.Filter)
			if gerr != nil && err == nil {
				err = fmt.Errorf("group %q: %w", g.Name, gerr)
			}
			groups = append(groups, gf)
		}
		m.views = append(m.views, &view{cfg: vc, filter: f, err: err, sort: keys, edited: edited, groups: groups})
		if vc.Name == prev && !prevQueue {
			m.cur = i
		}
	}
	m.views = append(m.views, newQueueView()) // always last
	if prevQueue {
		m.cur = len(m.views) - 1
	}
	m.rebuild()
}

// rebuild re-filters and re-sorts every view; the cursor sticks to the same item ID when it survives.
func (m *Model) rebuild() {
	selected := m.selectedID()
	q := strings.ToLower(m.query)
	m.unread = map[string]bool{}
	stacks := map[string][]item.StackInfo{} // per provider, over its whole list: a base the view hides still counts
	for i, v := range m.views {
		if v.queue {
			continue // syncQueue fills it
		}
		v.items, v.heads, v.snoozed, v.members = v.items[:0], nil, 0, nil
		if v.err != nil {
			continue
		}
		its := m.data[v.cfg.Provider]
		stack, ok := stacks[v.cfg.Provider]
		if !ok {
			stack = item.Stack(its)
			stacks[v.cfg.Provider] = stack
		}
		for j, it := range its {
			it = m.withSnooze(m.withNote(it))
			unread := m.isUnread(v.cfg.Provider, it)
			it = withStack(withUnread(it, unread), stack[j])
			ok, err := v.filter.Match(it.Env())
			if err != nil {
				v.err = err
				break
			}
			if !ok {
				continue
			}
			v.members = append(v.members, it)
			if _, snoozed := m.snoozes[it.ID]; snoozed {
				v.snoozed++
				if !m.showSnoozed {
					continue
				}
			}
			if i == m.cur && q != "" && !matches(it, q) {
				continue
			}
			v.items = append(v.items, it)
			if unread {
				m.unread[it.ID] = true
			}
		}
		item.Sort(v.items, v.sort)
		if v.err == nil && len(v.groups) > 0 {
			if err := v.group(); err != nil {
				v.err, v.items = err, v.items[:0]
			}
		}
	}
	m.syncQueue()
	m.restoreCursor(selected)
}

// group reorders the sorted items by their first matching group, ungrouped ones last, and records the headers.
func (v *view) group() error {
	buckets := make([][]item.Item, len(v.groups)+1) // last: ungrouped
	for _, it := range v.items {
		b, env := len(v.groups), it.Env()
		for i, f := range v.groups {
			ok, err := f.Match(env)
			if err != nil {
				return fmt.Errorf("group %q: %w", v.cfg.Groups[i].Name, err)
			}
			if ok {
				b = i
				break
			}
		}
		buckets[b] = append(buckets[b], it)
	}
	v.items = v.items[:0]
	for i, b := range buckets {
		if i < len(v.groups) && len(b) > 0 {
			v.heads = append(v.heads, head{at: len(v.items), n: len(b), name: v.cfg.Groups[i].Name})
		}
		v.items = append(v.items, b...)
	}
	return nil
}

// lines maps list lines to items: an item index, or -1-k for header k. Without groups line i is item i.
func (v *view) lines() []int {
	out := make([]int, 0, len(v.items)+len(v.heads))
	k := 0
	for i := range v.items {
		if k < len(v.heads) && v.heads[k].at == i {
			out = append(out, -1-k)
			k++
		}
		out = append(out, i)
	}
	return out
}

// listName is the name shown in a row's name column: the assignees for tickets, the author otherwise.
func listName(it item.Item) string {
	if it.Fields["kind"] != "ticket" {
		return it.Author
	}
	var names []string
	switch v := it.Fields["assignees"].(type) {
	case []string:
		names = v
	case []any: // decoded from the cache or a command provider's JSON
		for _, a := range v {
			if s, ok := a.(string); ok {
				names = append(names, s)
			}
		}
	}
	return strings.Join(names, ", ")
}

// prioStrip is the ▰▱ strip of a ticket's priority (1 urgent .. 4 low) and its style; empty for 0 or other kinds.
func prioStrip(it item.Item) (string, lipgloss.Style) {
	if it.Fields["kind"] != "ticket" {
		return "", sDim
	}
	p := 0
	switch v := it.Fields["priority"].(type) {
	case int:
		p = v
	case float64: // decoded from the cache or a command provider's JSON
		p = int(v)
	}
	fg := lipgloss.NewStyle().Foreground
	switch p {
	case 1:
		return "▰▰▰", fg(cBad)
	case 2:
		return "▰▰▱", fg(cOrange)
	case 3:
		return "▰▱▱", fg(cWarn)
	case 4:
		return "▱▱▱", sDim
	}
	return "", sDim
}

func matches(it item.Item, q string) bool {
	hay := strings.ToLower(it.ID + " " + it.Title + " " + it.Author + " " + listName(it))
	if t, ok := it.Fields["ticket"].(string); ok {
		hay += " " + strings.ToLower(t)
	}
	return strings.Contains(hay, q)
}

func (m *Model) items() []item.Item { return m.views[m.cur].items }

func (m *Model) selectedID() string {
	if len(m.views) == 0 || m.cur >= len(m.views) {
		return ""
	}
	if it := m.items(); m.cursor < len(it) {
		return it[m.cursor].ID
	}
	return ""
}

func (m *Model) restoreCursor(id string) {
	for i, it := range m.items() {
		if it.ID == id {
			m.cursor = i
			return
		}
	}
	m.clamp()
}

func (m *Model) clamp() {
	n := len(m.items())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), m.pollSessions(), m.updateCheck()}
	for name := range m.providers {
		cmds = append(cmds, m.fetch(name))
	}
	return tea.Batch(append(cmds, m.armSpin())...)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetch starts a fetch of provider name unless one is in flight. Its context is cancelled by cancelFetches.
func (m *Model) fetch(name string) tea.Cmd {
	st := m.fetchStateOf(name)
	if st.running {
		return nil
	}
	parent, cancel := context.WithCancel(context.Background())
	st.running, st.started, st.cancel = true, time.Now(), cancel
	gen := st.gen
	p := m.providers[name]
	full := func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 60*time.Second)
		defer cancel()
		items, err := p.Fetch(ctx)
		return fetchedMsg{name: name, gen: gen, items: item.Sanitize(items), err: err}
	}
	q, ok := p.(provider.Quicker)
	if !ok || m.fullFetched[name] {
		return full
	}
	quick := func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		items, err := q.Quick(ctx)
		return fetchedMsg{name: name, gen: gen, items: item.Sanitize(items), err: err, partial: true}
	}
	return tea.Sequence(quick, full)
}

// retryDelay is the wait after the nth consecutive failure: poll, doubling, capped at 5 minutes.
func retryDelay(poll time.Duration, fails int) time.Duration {
	d := poll
	for i := 1; i < fails && d < 5*time.Minute; i++ {
		d *= 2
	}
	return min(d, 5*time.Minute)
}

func (m *Model) refreshAll() tea.Cmd {
	var cmds []tea.Cmd
	for name := range m.providers {
		delete(m.fails, name)
		delete(m.retryAt, name)
		cmds = append(cmds, m.fetch(name))
	}
	return tea.Batch(cmds...)
}

// Update handles msg, then starts the fetch animation's tick loop if a fetch is running and no loop is.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	if spin := m.armSpin(); spin != nil {
		cmd = tea.Batch(cmd, spin)
	}
	if d := m.armDwell(); d != nil {
		cmd = tea.Batch(cmd, d)
	}
	return model, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.agents != nil {
			m.agents.fv.SetSize(msg.Width, msg.Height)
		}
	case fetchedMsg:
		st := m.fetchStateOf(msg.name)
		if msg.gen != st.gen {
			return m, nil // a cancelled fetch: the fetch that replaced it delivers newer data
		}
		var again tea.Cmd
		if !msg.partial {
			st.finish()
			again = m.fetchLanded(msg.name)
		}
		if msg.err != nil {
			if !msg.partial {
				m.provErr[msg.name] = msg.err
				m.fails[msg.name]++
				m.retryAt[msg.name] = time.Now().Add(retryDelay(m.cfg.Poll(), m.fails[msg.name]))
			}
			return m, again
		}
		delete(m.provErr, msg.name)
		if !msg.partial {
			if m.fullFetched == nil {
				m.fullFetched = map[string]bool{}
			}
			m.fullFetched[msg.name] = true
			delete(m.fails, msg.name)
			delete(m.retryAt, msg.name)
		}
		items := inherit(m.data[msg.name], msg.items)
		m.data[msg.name] = items
		if !msg.partial {
			m.fetchedAt[msg.name] = time.Now()
			m.wake(time.Now(), items) // a partial result lacks pending fields: only a full one can tell a change
			if m.seenBase[msg.name] {
				m.markRead(items...)
				delete(m.seenBase, msg.name)
			}
		}
		// landed items are never mutated afterwards (the next landing builds a new slice), so the command may read them
		save := func() tea.Msg { cache.Save(msg.name, items); return nil }
		for _, v := range m.views {
			v.err = nil // filter errors may have been transient (stale data shape)
		}
		m.rebuild()
		var auto tea.Cmd
		if !msg.partial {
			auto = m.autoFetched(msg.name) // a partial result may lack items: only a full one tells who arrived or left
		}
		return m, tea.Batch(save, again, auto)
	case tea.BlurMsg:
		m.blurred = true
	case tea.FocusMsg:
		m.blurred = false
		return m, tea.Batch(m.fetchStale(m.cfg.Poll()), m.queryBackground())
	case bgSentMsg:
		return m.onBgSent(msg)
	case bgTimeoutMsg:
		if msg.gen == m.bg.gen && m.bg.active {
			return m.endCapture()
		}
	case dwellMsg:
		m.dwelled(msg)
		return m, nil
	case spinTickMsg:
		m.spinning = false // this loop ends; Update re-arms it while a provider fetches
		return m, nil
	case tickMsg:
		if m.wake(time.Time(msg), nil) {
			m.rebuild()
		}
		return m, tea.Batch(tick(), m.onTick(), m.pollSessions(), m.pollUsage(), m.pollUsage5h())
	case usageMsg:
		m.usage = msg
	case usage5hMsg:
		m.usage5h = msg
		m.syncUsage5h()
	case updateMsg:
		m.onUpdate(msg)
	case sessionsMsg:
		chains := m.onSessions(msg)
		if msg.err == nil {
			m.syncAgents()
		}
		return m, tea.Batch(chains, m.drainAuto()) // a session that ended frees room for a queued on_enter start
	case agentPollMsg:
		return m, m.onAgentPoll(msg)
	case agentSnapMsg:
		return m, m.onAgentSnap(msg)
	case agentProcPollMsg:
		return m, m.onAgentProcPoll(msg)
	case agentProcsMsg:
		return m, m.onAgentProcs(msg)
	case interruptedMsg:
		if m.interrupted == nil {
			m.interrupted = map[string]bool{}
		}
		for _, n := range msg.names {
			m.interrupted[n] = true
		}
		return m.Update(doneMsg{notice: msg.notice, level: msg.level})
	case doneMsg:
		m.say(msg.level, msg.notice)
		if msg.refresh {
			m.holdUntilRefetched(msg.keys, msg.ok)
		} else {
			m.unlock(msg.keys)
		}
		var chains []tea.Cmd
		for i, exit := range msg.exits {
			if next, _ := followUp(m.cfg.Action[msg.action], exit); next != "" {
				delete(m.busy, msg.keys[i]) // the follow-up starts now, not after a refresh run's refetch
				chains = append(chains, m.chain(msg.action, msg.keys[i], exit))
			}
		}
		if msg.refresh {
			return m, tea.Batch(append(chains, m.pollSessions(), m.refreshAll())...)
		}
		if msg.jump != "" {
			return m, tea.Batch(m.pollSessions(), m.jumpTo(msg.jump))
		}
		return m, tea.Batch(append(chains, m.pollSessions())...)
	case editorDone:
		// :edit project: the user wrote it through lanes, so the reload trusts its new content first
		return m, m.startReload(msg.project)
	case reloadedMsg:
		return m, m.onReloaded(msg)
	case noteEdited:
		if msg.err != nil {
			m.say(lvErr, "note: "+msg.err.Error())
			return m, nil
		}
		m.setNote(msg.id, msg.text)
	case pollNow:
		return m, m.pollSessions()
	case detailMsg:
		m.onDetail(msg)
	case tea.KeyMsg:
		return m.keyIn(msg)
	default:
		if m.agents != nil { // the agent view's own tick
			var cmd tea.Cmd
			m.agents.fv, cmd = m.agents.fv.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// queryBackground writes the terminal background query in system mode and arms the capture of its reply; nil in a
// forced mode, while a query is unanswered, or when the terminal must not be asked.
func (m *Model) queryBackground() tea.Cmd {
	if forcedMode(m.cfg.Theme.Mode) || m.bg.active {
		return nil
	}
	getenv := m.getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	q, ok := termbg.Query(getenv("TERM"), getenv("TMUX"))
	if !ok {
		return nil
	}
	out := m.bgOut
	if out == nil {
		if fi, err := os.Stdout.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return nil // no terminal to ask
		}
		out = os.Stdout
	}
	m.bg = bgCapture{gen: m.bg.gen + 1, active: true}
	gen := m.bg.gen
	return func() tea.Msg {
		_, err := io.WriteString(out, q)
		return bgSentMsg{gen: gen, err: err}
	}
}

// onBgSent starts the wait for the reply of a written query; a query that could not be written ends its capture,
// which keeps the palette as for a terminal that never answers.
func (m *Model) onBgSent(msg bgSentMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.bg.gen || !m.bg.active {
		return m, nil
	}
	if msg.err != nil {
		return m.endCapture()
	}
	return m, tea.Tick(bgReplyWait, func(time.Time) tea.Msg { return bgTimeoutMsg{gen: msg.gen} })
}

// keyIn hands k to the key handling unless it is part of the reply to a background query.
func (m *Model) keyIn(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.bg.active {
		return m.key(k)
	}
	switch m.bg.feed(k) {
	case bgPass:
		return m.key(k)
	case bgHold:
		m.bg.keys = append(m.bg.keys, k)
		return m, nil
	case bgDone:
		dark, ok := termbg.ParseDark("\x1b]" + m.bg.body + "\a")
		m.bg = bgCapture{gen: m.bg.gen}
		if ok {
			redetected(m.cfg.Theme.Mode, dark)
		}
		return m, nil
	}
	return m.endCapture(k)
}

// endCapture ends the background capture and replays the keys it swallowed, then extra, in order.
func (m *Model) endCapture(extra ...tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := append(m.bg.keys, extra...)
	m.bg = bgCapture{gen: m.bg.gen}
	cmds := make([]tea.Cmd, 0, len(keys))
	for _, k := range keys {
		_, cmd := m.key(k)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) unlock(keys []string) {
	for _, k := range keys {
		delete(m.busy, k)
	}
}

// holdUntilRefetched keeps the items of a successful refresh run locked until every provider has landed a
// fetch that started after the run; a failed run unlocks them at once. Call it before refreshAll.
func (m *Model) holdUntilRefetched(keys []string, ok bool) {
	if !ok || len(m.providers) == 0 {
		m.unlock(keys)
		return
	}
	for name := range m.providers {
		if m.fetching(name) {
			m.refetchAfter[name] = true // its result predates the run
		}
	}
	for _, k := range keys {
		b, held := m.busy[k]
		if !held {
			continue
		}
		b.refetch = map[string]bool{}
		for name := range m.providers {
			b.refetch[name] = true
		}
		m.busy[k] = b
	}
}

// fetchLanded runs when provider name delivered a full result or an error. A fetch that started before a
// refresh run ended is followed by a fresh one; any other releases the items that waited on it.
func (m *Model) fetchLanded(name string) tea.Cmd {
	if m.refetchAfter[name] {
		delete(m.refetchAfter, name)
		if _, known := m.providers[name]; known {
			return m.fetch(name)
		}
	}
	for k, b := range m.busy {
		if !b.refetch[name] {
			continue
		}
		delete(b.refetch, name)
		if len(b.refetch) == 0 {
			delete(m.busy, k)
		}
	}
	return nil
}

// inherit fills fields a partial result marks as pending from the previous snapshot of the same item.
func inherit(old, fresh []item.Item) []item.Item {
	byID := make(map[string]item.Item, len(old))
	for _, o := range old {
		byID[o.ID] = o
	}
	for i, it := range fresh {
		o, ok := byID[it.ID]
		if !ok {
			continue
		}
		for _, f := range it.Pending {
			if v, has := o.Fields[f]; has {
				fresh[i].Fields[f] = v
			}
		}
	}
	return fresh
}

func (m *Model) onTick() tea.Cmd {
	var cmds []tea.Cmd
	if config.Mtime().After(m.cfgMod) {
		cmds = append(cmds, m.reload())
	}
	interval := m.cfg.Poll()
	if m.blurred {
		interval = m.cfg.PollBlurred()
	}
	return tea.Batch(append(cmds, m.fetchStale(interval))...)
}

// fetchStale fetches every provider whose data is older than interval and that is not waiting out a failure.
func (m *Model) fetchStale(interval time.Duration) tea.Cmd {
	var cmds []tea.Cmd
	for name := range m.providers {
		if time.Since(m.fetchedAt[name]) >= interval && time.Now().After(m.retryAt[name]) {
			cmds = append(cmds, m.fetch(name))
		}
	}
	return tea.Batch(cmds...)
}

// builtinKeys cannot be rebound through [keymap].
const builtinKeys = "jkgGhlVqRoidxXtpnNFSyzmAQ/:? "

// level is how severe a footer notice is: it picks the notice's colour.
type level int

const (
	lvInfo level = iota // plain text
	lvWarn              // the request was not fully carried out: theme warn
	lvErr               // something failed: theme bad
)

func (l level) style() lipgloss.Style {
	switch l {
	case lvErr:
		return sErr
	case lvWarn:
		return sWarn
	}
	return sText
}

// say sets the footer notice. Set it only through say, so every notice has an explicit level.
func (m *Model) say(l level, text string) { m.notice, m.level = text, l }

// reloadedMsg is the result of a config reload run off the UI goroutine. Without cfg the reload was not applied
// and notice says why.
type reloadedMsg struct {
	gen       int
	cfg       *config.Config
	providers map[string]provider.Provider
	buildErr  map[string]error
	notice    string
	level     level
}

// reload re-reads the config (also triggered by saving the file) and refetches, so edits apply live.
func (m *Model) reload() tea.Cmd { return m.startReload(false) }

// startReload runs the trust check, config.Load and the provider build in a command; the old config stays active
// until its reloadedMsg lands. trustFirst records the project's current .lanes.toml as trusted before the check.
func (m *Model) startReload(trustFirst bool) tea.Cmd {
	m.reloadGen++
	gen := m.reloadGen
	m.cfgMod = config.Mtime() // the tick starts no second reload of the same change while this one runs
	return func() tea.Msg { return loadReload(gen, trustFirst) }
}

// loadReload does the blocking part of a reload. It runs in a Bubble Tea command and touches no model state.
func loadReload(gen int, trustFirst bool) reloadedMsg {
	fail := func(l level, notice string) reloadedMsg { return reloadedMsg{gen: gen, level: l, notice: notice} }
	if trustFirst {
		if err := trustProject(); err != nil {
			return fail(lvErr, "trust: "+err.Error())
		}
	}
	if ok, err := projectTrusted(); !ok {
		if err != nil {
			return fail(lvErr, "trust: "+err.Error())
		}
		return fail(lvWarn, ".lanes.toml changed — restart lanes to trust it")
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(lvErr, "config: "+err.Error())
	}
	provs, errs := buildProviders(cfg)
	return reloadedMsg{gen: gen, cfg: cfg, providers: provs, buildErr: errs}
}

// onReloaded applies a landed reload unless a newer one started since.
func (m *Model) onReloaded(msg reloadedMsg) tea.Cmd {
	if msg.gen != m.reloadGen {
		return nil // a newer reload replaces it
	}
	if msg.cfg == nil {
		m.say(msg.level, msg.notice)
		return nil
	}
	m.cancelFetches() // their results belong to the old config; refreshAll below fetches at once
	m.applyBuilt(msg.cfg, msg.providers, msg.buildErr)
	m.say(lvInfo, fmt.Sprintf("config reloaded: %d views, %d actions", len(m.views)-1, len(msg.cfg.Action)))
	for k := range msg.cfg.Keymap {
		if len(k) == 1 && strings.Contains(builtinKeys, k) {
			m.say(lvWarn, m.notice+fmt.Sprintf(" · key %q is built in and ignored", k))
		}
	}
	return m.refreshAll()
}

func (m *Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeSearch:
		return m.keySearch(k)
	case modeCommand:
		return m.keyCommand(k)
	case modeConfirm:
		return m.keyConfirm(k)
	case modeHelp:
		return m.keyHelp(k)
	case modeDetail:
		return m.keyDetail(k)
	case modeHistory:
		m.mode = modeNormal
		return m, nil
	case modeSessions:
		return m.keySessions(k)
	case modeAgents:
		return m.keyAgents(k)
	}
	m.notice = ""
	s := k.String()
	n := len(m.items())
	if m.pendingG && s == "r" { // gr: the g prefix wins over a keymap binding for r
		m.pendingG = false
		if n > 0 {
			return m, m.resume(m.items()[m.cursor])
		}
		return m, nil
	}
	if s != "g" {
		m.pendingG = false
	}
	switch s {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		running := 0
		for _, si := range m.sessions {
			if !si.Dead {
				running++
			}
		}
		keep := "sessions keep"
		if running == 1 {
			keep = "session keeps"
		}
		m.ask(fmt.Sprintf("quit lanes? (%d %s running) [y/N]", running, keep), func() tea.Cmd { return tea.Quit })
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "g":
		if m.pendingG {
			m.cursor, m.pendingG = 0, false
		} else {
			m.pendingG = true
		}
	case "G":
		m.cursor = n - 1
	case "ctrl+d":
		m.cursor += m.listHeight() / 2
	case "ctrl+u":
		m.cursor -= m.listHeight() / 2
	case "l", "tab", "right":
		m.switchView(m.cur + 1)
	case "h", "shift+tab", "left":
		m.switchView(m.cur - 1)
	case " ":
		if n > 0 {
			id := m.items()[m.cursor].ID
			if m.marks[id] {
				delete(m.marks, id)
			} else {
				m.marks[id] = true
			}
			m.cursor++
		}
	case "V":
		all := true
		for _, it := range m.items() {
			all = all && m.marks[it.ID]
		}
		for _, it := range m.items() {
			if all {
				delete(m.marks, it.ID)
			} else {
				m.marks[it.ID] = true
			}
		}
	case "esc":
		m.marks = map[string]bool{}
		if m.query != "" {
			m.query = ""
			m.search.SetValue("")
			m.rebuild()
		}
	case "/":
		m.mode = modeSearch
		m.search.SetValue(m.query)
		m.search.CursorEnd()
		return m, m.search.Focus()
	case ":":
		m.mode = modeCommand
		m.cmdline.SetValue("")
		return m, m.cmdline.Focus()
	case "?":
		m.mode, m.helpOff = modeHelp, 0
	case "R":
		m.say(lvInfo, "refreshing…")
		return m, m.refreshAll()
	case "o":
		if n > 0 {
			m.openItemURL(m.items()[m.cursor].URL)
		}
	case "y":
		m.copyIDs()
	case "enter":
		if n > 0 && m.sess != nil {
			return m, m.jump(m.items()[m.cursor])
		}
	case "Q":
		m.switchView(len(m.views) - 1)
	case "x":
		if m.onQueue() {
			m.dropQueued()
		} else if m.sess != nil {
			return m, m.killTargets()
		}
	case "X":
		if m.sess != nil {
			return m, m.interruptTargets()
		}
	case "t":
		if n > 0 {
			return m, m.shell(m.items()[m.cursor])
		}
	case "i":
		m.showHistory()
	case "d":
		return m, m.showDetail()
	case "z":
		if m.toggleSnooze() {
			m.mode = modeCommand
			m.cmdline.SetValue("snooze ")
			m.cmdline.CursorEnd()
			return m, m.cmdline.Focus()
		}
	case "m":
		m.toggleRead()
	case "N":
		if n > 0 {
			return m, m.editNote(m.items()[m.cursor].ID)
		}
	case "S":
		m.openSessions()
	case "A":
		if n > 0 {
			return m, m.agentsOfItem(m.items()[m.cursor])
		}
	case "n":
		if m.sess == nil {
			break
		}
		name, ok := nextAttention(m.sessions, m.lastAttn)
		if !ok {
			m.say(lvWarn, "nothing needs you")
			break
		}
		m.lastAttn = name
		return m, m.jumpTo(name)
	case "p":
		m.preview_ = !m.preview_
	case "F":
		m.mode = modeCommand
		m.cmdline.SetValue("filter " + m.views[m.cur].cfg.Filter)
		m.cmdline.CursorEnd()
		return m, m.cmdline.Focus()
	case "ctrl+r":
		return m, m.reload()
	default:
		if s == "r" && m.onQueue() { // in the queue view r runs entries now, over a keymap binding for r
			return m, m.runQueued()
		}
		if name, ok := m.cfg.Keymap[s]; ok {
			return m, m.runAction(name, m.cfg.Action[name])
		}
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < len(m.views)-1 { // not the queue view: Q
			m.switchView(int(s[0] - '1'))
		}
	}
	m.clamp()
	return m, nil
}

func (m *Model) switchView(i int) {
	if i < 0 || i >= len(m.views) || i == m.cur {
		return
	}
	m.cur, m.cursor, m.offset = i, 0, 0
	m.rebuild()
}

func (m *Model) keySearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter":
		m.mode = modeNormal
		m.search.Blur()
		return m, nil
	case "esc":
		m.mode, m.query = modeNormal, ""
		m.search.Blur()
		m.rebuild()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(k)
	m.query = m.search.Value()
	m.rebuild()
	return m, cmd
}

func (m *Model) keyCommand(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.mode = modeNormal
		m.cmdline.Blur()
		return m, nil
	case "enter":
		line := strings.TrimSpace(m.cmdline.Value())
		m.mode = modeNormal
		m.cmdline.Blur()
		return m.runCommand(line)
	}
	var cmd tea.Cmd
	m.cmdline, cmd = m.cmdline.Update(k)
	return m, cmd
}

func (m *Model) listHeight() int {
	h := m.height - 4 // header, rule, rule, footer
	if m.preview_ {
		h -= previewH + 1
	}
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.mode {
	case modeHelp:
		return m.scrollView(m.helpLines())
	case modeDetail:
		return m.scrollView(m.detailLines())
	case modeHistory:
		return m.historyView()
	case modeSessions:
		return m.sessionsView()
	case modeAgents:
		if m.agents != nil {
			return m.agents.fv.View()
		}
	case modeConfirm:
		if m.confirm != nil && m.confirm.back == modeSessions {
			return m.sessionsView()
		}
	}
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString(sRule.Render(strings.Repeat("─", m.width)) + "\n")
	b.WriteString(m.list())
	b.WriteString(sRule.Render(strings.Repeat("─", m.width)) + "\n")
	if m.preview_ {
		b.WriteString(m.preview())
		b.WriteString(sRule.Render(strings.Repeat("─", m.width)) + "\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

// attention counts the items with a live session that is asking or waiting for input.
func (m *Model) attention(items []item.Item) (n int) {
	for _, it := range items {
		for _, ss := range m.sessionsOf(it) {
			if !ss.Dead && (ss.Claude == "asking" || ss.Claude == "waiting") {
				n++
				break
			}
		}
	}
	return n
}

func (m *Model) header() string {
	var tabs []string
	for i, v := range m.views {
		if (v.cfg.Divider || v.queue) && i > 0 { // the built-in queue tab is always set apart
			tabs = append(tabs, sDim.Render("¦"))
		}
		name := v.cfg.Name
		if v.edited {
			name += "*"
		}
		label := fmt.Sprintf("%s %d", name, len(v.items))
		if v.queue {
			label = name // muted, with a count only when entries wait
			if len(m.auto.queue) > 0 {
				label = fmt.Sprintf("%s %d", name, len(m.auto.queue))
			}
		}
		st := sTab
		if i == m.cur {
			st = sTabOn
		}
		if n := m.attention(v.items); n > 0 {
			// the count carries its own style; the active tab's background must reach it
			cs := badgeStyle("warn")
			if i == m.cur {
				cs = selectStyle(cs, false)
			}
			tabs = append(tabs, st.PaddingRight(0).Render(label+" ")+cs.Render(fmt.Sprint(n))+st.PaddingLeft(0).Render(""))
			continue
		}
		tabs = append(tabs, st.Render(label))
	}
	left := sTitle.Render(" "+app.Name) + "  " + strings.Join(tabs, "")
	now := time.Now()
	right := m.sessionSummary() + strings.Join(m.providerStatuses(now), sDim.Render("  "))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		// too narrow for every provider: only the one of the current view
		right = m.sessionSummary()
		if !m.onQueue() {
			right += m.styledStatus(m.views[m.cur].cfg.Provider, now)
		}
		gap = max(m.width-lipgloss.Width(left)-lipgloss.Width(right)-1, 1)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) pollUsage() tea.Cmd {
	dir := m.cfg.Settings.ClaudeDir
	return func() tea.Msg {
		text, at := session.Usage(dir)
		return usageMsg{text, at}
	}
}

func (m *Model) pollUsage5h() tea.Cmd {
	dir := m.cfg.Settings.ClaudeDir
	return func() tea.Msg {
		pct, ok := session.Usage5h(dir, time.Now())
		return usage5hMsg{pct, ok}
	}
}

// usageText is the Claude usage for the footer; once stale it carries its age.
func (m *Model) usageText(now time.Time) string {
	if m.usage.text == "" {
		return ""
	}
	text := "claude usage: " + m.usage.text
	if d := now.Sub(m.usage.at); d > usageStale {
		text += " (" + age(d) + " ago)"
	}
	return text
}

const (
	idMinW = 7
	idMaxW = 20
)

func (m *Model) list() string {
	h := m.listHeight()
	items := m.items()
	v := m.views[m.cur]
	lines := v.lines()
	// offset counts lines; the cursor's line is the cursor plus the headers above it
	cl := m.cursor
	for _, hd := range v.heads {
		if hd.at <= m.cursor {
			cl++
		}
	}
	top := cl
	if cl > 0 && cl-1 < len(lines) && lines[cl-1] < 0 {
		top = cl - 1 // on a group's first item, keep its header in view
	}
	if top < m.offset {
		m.offset = top
	}
	if cl >= m.offset+h {
		m.offset = cl - h + 1
	}
	var rows []string
	if len(items) == 0 {
		msg := "nothing here"
		if v.queue {
			msg = "queue empty: starts held back by max_claude_sessions wait here"
		}
		switch {
		case v.err != nil:
			msg = "filter error: " + v.err.Error()
			rows = append(rows, "", "  "+sErr.Render(msg))
		case m.provErr[v.cfg.Provider] != nil:
			e := m.provErr[v.cfg.Provider].Error()
			if t := m.retryAt[v.cfg.Provider]; !t.IsZero() {
				e += " · retry in " + time.Until(t).Round(time.Second).String()
			}
			rows = append(rows, "", "  "+sErr.Render(e))
		case m.fetching(v.cfg.Provider):
			rows = append(rows, "")
			for _, st := range m.providerStatuses(time.Now()) {
				rows = append(rows, "  "+st)
			}
		default:
			rows = append(rows, "", "  "+sDim.Render(msg))
		}
	}
	idW := idColWidth(items)
	for l := m.offset; l < len(lines) && len(rows) < h; l++ {
		if i := lines[l]; i >= 0 {
			rows = append(rows, m.row(items[i], i == m.cursor, idW))
		} else {
			hd := v.heads[-1-i]
			rows = append(rows, "  "+sDim.Render(fmt.Sprintf("── %s (%d) ──", hd.name, hd.n)))
		}
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n") + "\n"
}

// idColWidth is the width of the ID column for items: the longest ID, within [idMinW, idMaxW].
func idColWidth(items []item.Item) int {
	w := idMinW
	for _, it := range items {
		if iw := lipgloss.Width(it.ID); iw > w {
			w = iw
		}
	}
	return min(w, idMaxW)
}

func (m *Model) row(it item.Item, selected bool, idW int) string {
	// every segment carries its own background: a nested style's reset would otherwise end the highlight
	st := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return selectStyle(s, false)
		}
		return s
	}
	plain := st(lipgloss.NewStyle())
	mark := plain.Render(" ")
	if m.marks[it.ID] {
		mark = st(sMark).Render("▌")
	}
	var badges []string
	var all []item.Badge
	all = append(all, it.Badges...)
	for i, ss := range m.sessionsOf(it) {
		if i < 3 {
			all = append(all, ss.Badge())
		}
	}
	b, busy := m.busy[session.Key(it.ID)]
	if busy {
		all = append(all, item.Badge{Text: "⋯ " + b.action, Kind: "info"})
	}
	if a := m.queued(it.ID); a != "" {
		all = append(all, item.Badge{Text: "queued " + a, Kind: "warn"})
	}
	rightW := 0
	for _, b := range all {
		badges = append(badges, st(badgeStyle(b.Kind)).Render(b.Text))
		rightW += lipgloss.Width(b.Text)
	}
	if len(all) > 1 {
		rightW += 2 * (len(all) - 1)
	}
	right := strings.Join(badges, plain.Render("  "))
	const authW = 16
	id := st(sID).Render(fmt.Sprintf("%-*s", idW, ansi.Truncate(it.ID, idW, "…")))
	fixed := 1 + 1 + idW + 1 + 1 + rightW + 1
	cellW := m.width - fixed // title, author and padding share this cell
	strip, stripSt := prioStrip(it)
	nameW := authW
	if strip != "" {
		nameW -= 4 // the strip and a space stay inside the budget; only the names truncate
	}
	authText := ansi.Truncate(listName(it), nameW, "…")
	authCellW := 0
	if authText != "" {
		authCellW = 2 + ansi.StringWidth(authText) // two spaces, then the author
	}
	if strip != "" {
		if authText == "" {
			authCellW = 2
		} else {
			authCellW++ // a space between the names and the strip
		}
		authCellW += 3
	}
	noteW := 0
	if m.notes.Get(it.ID) != "" {
		noteW = 2 // a space, then the note marker
	}
	_, snoozed := m.snoozes[it.ID]
	snoozeW := 0
	if snoozed {
		snoozeW = 2 // a space, then the snooze marker
	}
	base := stackBase(it)
	baseText := ""
	if base != "" {
		baseText = ansi.Truncate("↑ "+base, idW+2, "…")
	}
	baseW := 0
	if baseText != "" {
		baseW = 1 + ansi.StringWidth(baseText) // a space, then the stack marker
	}
	titleW := cellW - authCellW - noteW - snoozeW - baseW
	if titleW < 10 {
		titleW = 10
	}
	ts := sText
	if m.unread[it.ID] {
		ts = ts.Bold(true)
	}
	if it.Draft || len(b.refetch) > 0 || snoozed || base != "" {
		// drafts, snoozed items (:snoozed), items whose base is not merged yet, and items whose refresh run's
		// result is not fetched yet
		ts = sDim
	}
	titleText := ansi.Truncate(it.Title, titleW, "…")
	used := ansi.StringWidth(titleText)
	title := st(ts).Render(titleText)
	if baseText != "" {
		title += plain.Render(" ") + st(sDim).Render(baseText)
		used += baseW
	}
	if authText != "" {
		title += plain.Render("  ") + st(sDim).Render(authText)
	}
	if strip != "" {
		if authText == "" {
			title += plain.Render("  ")
		} else {
			title += plain.Render(" ")
		}
		title += st(stripSt).Render(strip)
	}
	if authCellW > 0 {
		used += authCellW
	}
	if noteW > 0 {
		title += plain.Render(" ") + st(badgeStyle("info")).Render("✎")
		used += noteW
	}
	if snoozed {
		title += plain.Render(" ") + st(sDim).Render("z")
		used += snoozeW
	}
	if pad := cellW - used; pad > 0 {
		title += plain.Render(strings.Repeat(" ", pad))
	}
	sp := plain.Render(" ")
	line := mark + sp + id + sp + title + sp + right
	if selected {
		return plain.Width(m.width).Render(line)
	}
	return line
}

func (m *Model) footer() string {
	switch m.mode {
	case modeSearch:
		return m.search.View()
	case modeCommand:
		return m.cmdline.View()
	case modeConfirm:
		return " " + sWarn.Render(m.confirm.prompt) + sDim.Render("  y/enter yes · n/esc no")
	}
	left := ""
	switch {
	case m.notice != "":
		left = m.level.style().Render(" " + strings.SplitN(m.notice, "\n", 2)[0])
	case len(m.marks) > 0:
		left = sMark.Render(fmt.Sprintf(" %d marked", len(m.marks)))
	case m.query != "":
		left = sDim.Render(" filter: ") + sText.Render(m.query)
	case m.usage.text != "":
		left = sDim.Render(" " + m.usageText(time.Now()))
	}
	for _, seg := range []struct {
		n      int
		format string
	}{{m.views[m.cur].snoozed, "%d snoozed"}, {len(m.auto.queue), "queued %d"}} {
		if seg.n == 0 {
			continue
		}
		sep := " · "
		if left == "" {
			sep = " "
		}
		left += sDim.Render(sep + fmt.Sprintf(seg.format, seg.n))
	}
	hint := sDim.Render("j/k move · h/l view · space mark · enter jump · x close · p preview · ? help")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(hint) - 1
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + hint
}

func (m *Model) helpLines() []string {
	rows := [][2]string{
		{"j / k", "move down / up"},
		{"gg / G", "top / bottom"},
		{"ctrl-d / ctrl-u", "half page down / up"},
		{"h / l, tab", "previous / next view"},
		{"1-9", "jump to view"},
		{"Q", "queue view: starts waiting for a free session (r starts entries now, past the limit; x drops them)"},
		{"space", "mark item (and move down)"},
		{"V", "mark / unmark all"},
		{"esc", "clear marks and filter"},
		{"/", "search items in view (enter keeps, esc clears)"},
		{"F", "edit this view's filter expression (saved outside the config)"},
		{"enter", "jump into the item's newest session"},
		{"x", "close sessions of marked items / cursor item (after confirming)"},
		{"X", "interrupt the running commands of marked items / cursor item; keeps output (again: SIGKILL)"},
		{"t", "shell for the cursor item: jump into it (started, or restarted if finished)"},
		{"gr", "resume the cursor item's newest finished Claude session (actions with resume)"},
		{"i", "item history"},
		{"d", "item details: fields, description, pipeline, threads, comments (j/k scroll, J/K thread, enter/space fold it, za/zA fold/unfold all, q close)"},
		{"N", "edit the item's note in $EDITOR (save empty to delete)"},
		{"m", "mark item(s) unread; when all of them are unread: read"},
		{"z", "snooze item(s): 2h, 3d, mon, 2026-11-03, empty = until it changes; on a snoozed one: unsnooze"},
		{"n", "jump into the next session that is asking or waiting"},
		{"A", "agent view of the item's newest Claude session: agents, tools, context (esc / q / A back)"},
		{"S", "sessions: every session, also of items no longer listed (enter jump, X interrupt, x close)"},
		{"p", "toggle preview pane"},
		{"o / u", "open item in browser"},
		{"y", "copy the ID of the cursor item / marked items to the clipboard"},
		{"R / ctrl-r", "refresh data / reload config"},
		{"q", "quit after confirming (sessions keep running)"},
	}
	var b strings.Builder
	b.WriteString(sTitle.Render(" "+app.Name+" — keys") + "\n\n")
	for _, r := range rows {
		b.WriteString("  " + sKey.Render(fmt.Sprintf("%-18s", r[0])) + sText.Render(r[1]) + "\n")
	}
	// only the actions whose when matches the cursor item; all of them when the list is empty
	keys := make([]string, 0, len(m.cfg.Keymap))
	for k, name := range m.cfg.Keymap {
		if its := m.items(); m.cursor < len(its) {
			if match, _, _ := matchWhen(m.cfg.Action[name], its[m.cursor:m.cursor+1]); len(match) == 0 {
				continue
			}
		}
		keys = append(keys, k)
	}
	if len(keys) > 0 {
		b.WriteString("\n" + sTitle.Render(" actions") + sDim.Render("  (run on marked items, or the cursor item)") + "\n\n")
		sort.Strings(keys)
		for _, k := range keys {
			name := m.cfg.Keymap[k]
			act := m.cfg.Action[name]
			kind := "session"
			if act.Type == "run" {
				kind = "run"
			}
			note := sDim.Render(fmt.Sprintf("[%s] ", kind)) + sDim.Render(ansi.Truncate(act.Run, m.width-40, "…"))
			if len(k) == 1 && strings.Contains(builtinKeys, k) {
				note = sErr.Render("shadowed by a built-in key")
			}
			b.WriteString("  " + sKey.Render(fmt.Sprintf("%-18s", k)) + sText.Render(fmt.Sprintf("%-18s", name)) + note + "\n")
		}
	}
	b.WriteString("\n" + sTitle.Render(" commands") + "\n\n")
	for _, c := range [][2]string{
		{":view <name>", "switch view"},
		{":filter [expr|reset|save]", "set / reset this view's filter / save it into the config"},
		{":newview <name> <expr>", "create a view (same provider as the current one)"},
		{":delview <name>", "delete a view created with :newview"},
		{":note [text]", "set the item's note to a line of text / clear it"},
		{":snooze [when]", "snooze marked items / cursor item until when (empty: until it changes)"},
		{":snoozed", "show / hide snoozed items (dimmed, marked z)"},
		{":run <action>", "run an action by name"},
		{":auto [on|off]", "pause / resume the views' on_enter actions (show their state)"},
		{":resume", "resume the cursor item's newest finished Claude session (same as gr)"},
		{":sessions", "list every session (same as S)"},
		{":prune [failed]", "kill every finished session (only failed ones)"},
		{":refresh  :reload  :edit  :config  :quit", ""},
	} {
		b.WriteString("  " + sKey.Render(fmt.Sprintf("%-26s", c[0])) + sText.Render(c[1]) + "\n")
	}
	b.WriteString("\n" + sDim.Render("  global config   "+config.Path()) + "\n")
	if pp := config.ProjectPath(); pp != "" {
		b.WriteString(sDim.Render("  project config  "+pp) + "\n")
	} else {
		b.WriteString(sDim.Render("  project config  none (create "+config.ProjectFile+" in the project, :edit project)") + "\n")
	}
	b.WriteString(sDim.Render("  state           "+app.StateDir()+"  (history, views, notes, snoozes, sessions)") + "\n")
	b.WriteString(m.fieldsHelp())
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

// scrollView renders lines as a full screen scrolled by helpOff (the help and detail screens).
func (m *Model) scrollView(lines []string) string {
	h := m.height - 1
	if h < 3 {
		h = 3
	}
	if max := len(lines) - h; m.helpOff > max {
		m.helpOff = max
	}
	if m.helpOff < 0 {
		m.helpOff = 0
	}
	end := m.helpOff + h
	if end > len(lines) {
		end = len(lines)
	}
	out := lines[m.helpOff:end]
	for len(out) < h {
		out = append(out, "")
	}
	pos := ""
	if len(lines) > h {
		pos = fmt.Sprintf("  %d–%d of %d", m.helpOff+1, end, len(lines))
	}
	return strings.Join(out, "\n") + "\n" + sDim.Render(" j/k scroll · ctrl-d/u page · g/G top/bottom · q close"+pos)
}

func (m *Model) keyHelp(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := m.height / 2
	switch k.String() {
	case "q", "esc", "?", "ctrl+c":
		m.mode = modeNormal
	case "j", "down", "enter":
		m.helpOff++
	case "k", "up":
		m.helpOff--
	case "ctrl+d", "pgdown", " ":
		m.helpOff += page
	case "ctrl+u", "pgup":
		m.helpOff -= page
	case "g", "home":
		m.helpOff = 0
	case "G", "end":
		m.helpOff = 1 << 20 // clamped in scrollView
	}
	return m, nil
}
