// Package ui is the Bubble Tea front end: views, list navigation, search, command line.
package ui

import (
	"context"
	"fmt"
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
	"github.com/chriha/lanes/internal/session"
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
)

type view struct {
	cfg    config.View
	filter *filter.Filter
	err    error
	items  []item.Item
	sort   []item.SortKey
	edited bool // filter overridden from inside the TUI
}

type fetchedMsg struct {
	name    string
	items   []item.Item
	err     error
	partial bool // quick result; a full one follows
}
type tickMsg time.Time
type usageMsg struct {
	text string
	at   time.Time
}

// usageStale is the age after which the Claude usage is dimmed and shows how old it is.
const usageStale = 10 * time.Minute

type Model struct {
	cfg       *config.Config
	cfgMod    time.Time
	providers map[string]provider.Provider
	data      map[string][]item.Item
	fetchedAt map[string]time.Time
	loading   map[string]bool
	provErr   map[string]error
	fails     map[string]int       // consecutive failed fetches per provider
	retryAt   map[string]time.Time // no refetch before this

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

	sess       *session.Manager
	settings   string
	sessions   []session.Info
	sessByItem map[string][]session.Info
	polling    bool
	confirm    *confirmState
	histTitle  string
	hist       []string
	helpOff    int
	sessCursor int    // cursor of the sessions screen
	lastAttn   string // session the n key jumped to last
	ov         *config.Overrides
	notes      notes.Store
	preview_   bool
	usage      usageMsg // Claude usage from [settings] claude_dir, shown at the right of the header

	width, height int
}

func New(cfg *config.Config) *Model {
	m := &Model{
		data:      map[string][]item.Item{},
		fetchedAt: map[string]time.Time{},
		loading:   map[string]bool{},
		provErr:   map[string]error{},
		fails:     map[string]int{},
		retryAt:   map[string]time.Time{},
		marks:     map[string]bool{},
		search:    textinput.New(),
		cmdline:   textinput.New(),
		preview_:  cfg.Settings.Preview,
	}
	m.search.Prompt, m.cmdline.Prompt = "/", ":"
	m.search.PromptStyle, m.cmdline.PromptStyle = sKey, sKey
	var err error
	if m.ov, err = config.LoadOverrides(); err != nil {
		m.notice = err.Error()
	}
	if m.sess, err = session.NewManager(); err != nil {
		m.notice = strings.TrimSpace(m.notice + " " + err.Error())
	}
	if m.notes, err = notes.Load(); err != nil {
		m.notice = strings.TrimSpace(m.notice + " notes: " + err.Error())
	}
	m.settings, _ = session.SettingsFile()
	m.sessByItem = map[string][]session.Info{}
	m.applyConfig(cfg)
	return m
}

func (m *Model) applyConfig(cfg *config.Config) {
	m.cfg = cfg
	applyTheme(cfg.Theme)
	m.cfgMod = config.Mtime()
	m.providers = map[string]provider.Provider{}
	for name, pc := range cfg.Provider {
		p, err := provider.New(pc)
		if err != nil {
			m.provErr[name] = err
			continue
		}
		m.providers[name] = p
		if _, ok := m.data[name]; !ok {
			m.data[name] = cache.Load(name)
		}
	}
	prev := ""
	if m.cur < len(m.views) {
		prev = m.views[m.cur].cfg.Name
	}
	m.views = m.views[:0]
	m.cur = 0
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
		m.views = append(m.views, &view{cfg: vc, filter: f, err: err, sort: keys, edited: edited})
		if vc.Name == prev {
			m.cur = i
		}
	}
	m.rebuild()
}

// rebuild re-filters and re-sorts every view; the cursor sticks to the same item ID when it survives.
func (m *Model) rebuild() {
	selected := m.selectedID()
	q := strings.ToLower(m.query)
	for i, v := range m.views {
		v.items = v.items[:0]
		if v.err != nil {
			continue
		}
		for _, it := range m.data[v.cfg.Provider] {
			it = m.withNote(it)
			ok, err := v.filter.Match(it.Env())
			if err != nil {
				v.err = err
				break
			}
			if !ok {
				continue
			}
			if i == m.cur && q != "" && !matches(it, q) {
				continue
			}
			v.items = append(v.items, it)
		}
		item.Sort(v.items, v.sort)
	}
	m.restoreCursor(selected)
}

func matches(it item.Item, q string) bool {
	hay := strings.ToLower(it.ID + " " + it.Title + " " + it.Author)
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
	cmds := []tea.Cmd{tick(), m.pollSessions()}
	for name := range m.providers {
		cmds = append(cmds, m.fetch(name))
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) fetch(name string) tea.Cmd {
	if m.loading[name] {
		return nil
	}
	m.loading[name] = true
	p := m.providers[name]
	full := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		items, err := p.Fetch(ctx)
		return fetchedMsg{name: name, items: items, err: err}
	}
	q, ok := p.(provider.Quicker)
	if !ok {
		return full
	}
	quick := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		items, err := q.Quick(ctx)
		return fetchedMsg{name: name, items: items, err: err, partial: true}
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

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case fetchedMsg:
		if !msg.partial {
			m.loading[msg.name] = false
		}
		if msg.err != nil {
			if !msg.partial {
				m.provErr[msg.name] = msg.err
				m.fails[msg.name]++
				m.retryAt[msg.name] = time.Now().Add(retryDelay(m.cfg.Poll(), m.fails[msg.name]))
			}
			return m, nil
		}
		delete(m.provErr, msg.name)
		if !msg.partial {
			delete(m.fails, msg.name)
			delete(m.retryAt, msg.name)
		}
		items := inherit(m.data[msg.name], msg.items)
		m.data[msg.name] = items
		if !msg.partial {
			m.fetchedAt[msg.name] = time.Now()
		}
		cache.Save(msg.name, items)
		for _, v := range m.views {
			v.err = nil // filter errors may have been transient (stale data shape)
		}
		m.rebuild()
	case tickMsg:
		return m, tea.Batch(tick(), m.onTick(), m.pollSessions(), m.pollUsage())
	case usageMsg:
		m.usage = msg
	case sessionsMsg:
		m.onSessions(msg)
	case doneMsg:
		m.notice = msg.notice
		if msg.refresh {
			return m, tea.Batch(m.pollSessions(), m.refreshAll())
		}
		return m, m.pollSessions()
	case editorDone:
		return m, m.reload()
	case noteEdited:
		if msg.err != nil {
			m.notice = "note: " + msg.err.Error()
			return m, nil
		}
		m.setNote(msg.id, msg.text)
	case pollNow:
		return m, m.pollSessions()
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
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
	for name := range m.providers {
		if time.Since(m.fetchedAt[name]) >= m.cfg.Poll() && time.Now().After(m.retryAt[name]) {
			cmds = append(cmds, m.fetch(name))
		}
	}
	return tea.Batch(cmds...)
}

// builtinKeys cannot be rebound through [keymap].
const builtinKeys = "jkgGhlVqRoixpnNFS/:? "

// reload re-reads the config (also triggered by saving the file) and refetches, so edits apply live.
func (m *Model) reload() tea.Cmd {
	cfg, err := config.Load()
	if err != nil {
		m.notice = "config: " + err.Error()
		m.cfgMod = config.Mtime() // do not retry the same broken file every tick
		return nil
	}
	m.applyConfig(cfg)
	m.notice = fmt.Sprintf("config reloaded: %d views, %d actions", len(m.views), len(cfg.Action))
	for k := range cfg.Keymap {
		if len(k) == 1 && strings.Contains(builtinKeys, k) {
			m.notice += fmt.Sprintf(" · key %q is built in and ignored", k)
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
	case modeHistory:
		m.mode = modeNormal
		return m, nil
	case modeSessions:
		return m.keySessions(k)
	}
	m.notice = ""
	s := k.String()
	if s != "g" {
		m.pendingG = false
	}
	n := len(m.items())
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
		m.ask(fmt.Sprintf("quit lanes? (%d sessions keep running) [y/N]", running), func() tea.Cmd { return tea.Quit })
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
		m.notice = "refreshing…"
		return m, m.refreshAll()
	case "o":
		if n > 0 {
			openURL(m.items()[m.cursor].URL)
		}
	case "enter":
		if n > 0 && m.sess != nil {
			return m, m.jump(m.items()[m.cursor])
		}
	case "x":
		if m.sess != nil {
			return m, m.killTargets()
		}
	case "i":
		m.showHistory()
	case "N":
		if n > 0 {
			return m, m.editNote(m.items()[m.cursor].ID)
		}
	case "S":
		m.openSessions()
	case "n":
		if m.sess == nil {
			break
		}
		name, ok := nextAttention(m.sessions, m.lastAttn)
		if !ok {
			m.notice = "nothing needs you"
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
		if name, ok := m.cfg.Keymap[s]; ok {
			return m, m.runAction(name, m.cfg.Action[name])
		}
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
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
		return m.helpView()
	case modeHistory:
		return m.historyView()
	case modeSessions:
		return m.sessionsView()
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
		name := v.cfg.Name
		if v.edited {
			name += "*"
		}
		label := fmt.Sprintf("%s %d", name, len(v.items))
		st := sTab
		if i == m.cur {
			st = sTabOn
		}
		if n := m.attention(v.items); n > 0 {
			// the count carries its own style; the active tab's background must reach it
			cs := badgeStyle("warn")
			if i == m.cur {
				cs = cs.Background(cSelBg)
			}
			tabs = append(tabs, st.PaddingRight(0).Render(label+" ")+cs.Render(fmt.Sprint(n))+st.PaddingLeft(0).Render(""))
			continue
		}
		tabs = append(tabs, st.Render(label))
	}
	left := sTitle.Render(" "+app.Name) + "  " + strings.Join(tabs, "")
	right := m.sessionSummary() + sDim.Render(m.freshness())
	if u := m.usageText(time.Now()); u != "" && m.width-lipgloss.Width(left)-lipgloss.Width(right+u) > 3 {
		right += "   " + u // dropped when the header has no room for it
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
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

// usageText is the Claude usage for the header; once stale it is dimmed and carries its age.
func (m *Model) usageText(now time.Time) string {
	if m.usage.text == "" {
		return ""
	}
	if d := now.Sub(m.usage.at); d > usageStale {
		return sDim.Render(m.usage.text + " (" + age(d) + " ago)")
	}
	return m.usage.text
}

func (m *Model) freshness() string {
	name := m.views[m.cur].cfg.Provider
	switch {
	case m.loading[name]:
		return "⟳ refreshing"
	case m.fetchedAt[name].IsZero():
		return "cached"
	}
	return "updated " + time.Since(m.fetchedAt[name]).Round(time.Second).String() + " ago"
}

func (m *Model) list() string {
	h := m.listHeight()
	items := m.items()
	v := m.views[m.cur]
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	var rows []string
	if len(items) == 0 {
		msg := "nothing here"
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
		case m.loading[v.cfg.Provider]:
			rows = append(rows, "", "  "+sDim.Render("loading…"))
		default:
			rows = append(rows, "", "  "+sDim.Render(msg))
		}
	}
	for i := m.offset; i < len(items) && len(rows) < h; i++ {
		rows = append(rows, m.row(items[i], i == m.cursor))
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n") + "\n"
}

func (m *Model) row(it item.Item, selected bool) string {
	// every segment carries its own background: a nested style's reset would otherwise end the highlight
	st := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Background(cSelBg)
		}
		return s
	}
	plain := st(lipgloss.NewStyle())
	mark := plain.Render("  ")
	if m.marks[it.ID] {
		mark = st(sMark).Render("● ")
	}
	var badges []string
	var all []item.Badge
	if m.notes.Get(it.ID) != "" {
		all = append(all, item.Badge{Text: "✎", Kind: "dim"})
	}
	all = append(all, it.Badges...)
	for i, ss := range m.sessionsOf(it) {
		if i < 3 {
			all = append(all, ss.Badge())
		}
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
	const idW, authW = 7, 16
	id := st(sID).Render(fmt.Sprintf("%-*s", idW, ansi.Truncate(it.ID, idW, "")))
	auth := st(sDim).Render(fmt.Sprintf("%-*s", authW, ansi.Truncate(it.Author, authW, "…")))
	fixed := 1 + 2 + idW + 1 + 1 + authW + 1 + rightW + 1
	titleW := m.width - fixed
	if titleW < 10 {
		titleW = 10
	}
	ts := sText
	if it.Draft {
		ts = sDim
	}
	title := st(ts).Render(fmt.Sprintf("%-*s", titleW, ansi.Truncate(it.Title, titleW, "…")))
	cur := plain.Render(" ")
	if selected {
		cur = st(sMark).Render("▸")
	}
	sp := plain.Render(" ")
	line := cur + mark + id + sp + title + sp + auth + sp + right
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
		left = sText.Render(" " + strings.SplitN(m.notice, "\n", 2)[0])
	case len(m.marks) > 0:
		left = sMark.Render(fmt.Sprintf(" %d marked", len(m.marks)))
	case m.query != "":
		left = sDim.Render(" filter: ") + sText.Render(m.query)
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
		{"space", "mark item (and move down)"},
		{"V", "mark / unmark all"},
		{"esc", "clear marks and filter"},
		{"/", "search items in view (enter keeps, esc clears)"},
		{"F", "edit this view's filter expression (saved outside the config)"},
		{"enter", "jump into the item's newest session"},
		{"x", "close sessions of marked items / cursor item (confirms if running)"},
		{"i", "item history"},
		{"N", "edit the item's note in $EDITOR (save empty to delete)"},
		{"n", "jump into the next session that is asking or waiting"},
		{"S", "sessions: every session, also of items no longer listed (enter jump, x close)"},
		{"p", "toggle preview pane"},
		{"o / u", "open item in browser"},
		{"R / ctrl-r", "refresh data / reload config"},
		{"q", "quit after confirming (sessions keep running)"},
	}
	var b strings.Builder
	b.WriteString(sTitle.Render(" "+app.Name+" — keys") + "\n\n")
	for _, r := range rows {
		b.WriteString("  " + sKey.Render(fmt.Sprintf("%-18s", r[0])) + sText.Render(r[1]) + "\n")
	}
	if len(m.cfg.Keymap) > 0 {
		b.WriteString("\n" + sTitle.Render(" actions") + sDim.Render("  (run on marked items, or the cursor item)") + "\n\n")
		keys := make([]string, 0, len(m.cfg.Keymap))
		for k := range m.cfg.Keymap {
			keys = append(keys, k)
		}
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
		{":filter [expr|reset]", "set / reset this view's filter"},
		{":newview <name> <expr>", "create a view (same provider as the current one)"},
		{":delview <name>", "delete a view created with :newview"},
		{":note [text]", "set the item's note to a line of text / clear it"},
		{":run <action>", "run an action by name"},
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
	b.WriteString(sDim.Render("  state           "+app.StateDir()+"  (history, views, notes, sessions)") + "\n")
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m *Model) helpView() string {
	lines := m.helpLines()
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
		m.helpOff = 1 << 20 // clamped in helpView
	}
	return m, nil
}
