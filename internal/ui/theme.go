package ui

import (
	"cmp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/config"
	"github.com/chriha/lanes/internal/ui/flowview"
)

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#6C3FD1", Dark: "#A78BFA"}
	cText   = lipgloss.AdaptiveColor{Light: "#1F2430", Dark: "#E6E8EE"}
	cDim    = lipgloss.AdaptiveColor{Light: "#8A90A0", Dark: "#6B7280"}
	cOK     = lipgloss.AdaptiveColor{Light: "#1E8E4E", Dark: "#4ADE80"}
	cWarn   = lipgloss.AdaptiveColor{Light: "#B7791F", Dark: "#FBBF24"}
	cOrange = lipgloss.AdaptiveColor{Light: "#C05621", Dark: "#FB923C"}
	cBad    = lipgloss.AdaptiveColor{Light: "#C53030", Dark: "#F87171"}
	cInfo   = lipgloss.AdaptiveColor{Light: "#2B6CB0", Dark: "#60A5FA"}
	// cSelBg is the selection background: a medium-strength tint per side, so the highlight never flips to a bright block on
	// dark terminals. A side configured empty falls back to the terminal's reverse video.
	cSelBg = lipgloss.AdaptiveColor{Light: "#CFC4F0", Dark: "#404872"}

	sTitle, sTab, sTabOn, sDim, sText, sID, sMark, sErr, sRule, sKey, sWarn lipgloss.Style
)

var base [8]lipgloss.AdaptiveColor

// cAgent is the agent view's resolved palette.
var cAgent = flowview.NeonPalette()

// cLabels maps a lowercased label name to its resolved color; a label without an entry renders in cText.
var cLabels map[string]lipgloss.AdaptiveColor

// detectedDark is the terminal background as last detected: recorded before the first forced mode, or reported on a
// focus regain; nil = lipgloss's own detection stands.
var detectedDark *bool

func init() {
	base = [8]lipgloss.AdaptiveColor{cAccent, cOK, cWarn, cBad, cInfo, cText, cDim, cSelBg}
	buildStyles()
}

func buildStyles() {
	sTitle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sTab = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
	sTabOn = selectStyle(lipgloss.NewStyle().Bold(true).Padding(0, 1), true)
	sDim = lipgloss.NewStyle().Foreground(cDim)
	sText = lipgloss.NewStyle().Foreground(cText)
	sID = lipgloss.NewStyle().Foreground(cInfo)
	sMark = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sErr = lipgloss.NewStyle().Foreground(cBad)
	sRule = lipgloss.NewStyle().Foreground(cDim)
	sKey = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sWarn = lipgloss.NewStyle().Foreground(cWarn).Bold(true)
}

// selectStyle marks s as part of the selected row or active tab: with a configured selection color for the current
// background it paints that background, otherwise it swaps the terminal's own foreground and background. withText
// also sets the text color on the configured side; on the reverse side the terminal's foreground is what gets swapped.
func selectStyle(s lipgloss.Style, withText bool) lipgloss.Style {
	c := cSelBg.Dark
	if !lipgloss.HasDarkBackground() {
		c = cSelBg.Light
	}
	if c == "" {
		return s.Reverse(true)
	}
	if withText {
		s = s.Foreground(cText)
	}
	return s.Background(lipgloss.Color(c))
}

// applyTheme resolves the palette from config: per color and side, the [theme.light] / [theme.dark] value wins
// over the flat [theme] value, which wins over the built-in default. A forced mode pins the background.
func applyTheme(t config.Theme) {
	flat, light, dark := hexes(t.Colors), hexes(t.Light), hexes(t.Dark)
	pal := base
	for i := range pal {
		pal[i].Light = cmp.Or(light[i], flat[i], pal[i].Light)
		pal[i].Dark = cmp.Or(dark[i], flat[i], pal[i].Dark)
	}
	cAccent, cOK, cWarn, cBad, cInfo, cText, cDim, cSelBg = pal[0], pal[1], pal[2], pal[3], pal[4], pal[5], pal[6], pal[7]
	cLabels = labelColors(t)
	cAgent = agentPalette(t)
	applyMode(t.Mode)
	buildStyles()
}

// labelColors resolves each label's color by lowercased name: per side, the [theme.light.labels] /
// [theme.dark.labels] entry wins over the flat [theme.labels] one, which wins over the resolved text color.
func labelColors(t config.Theme) map[string]lipgloss.AdaptiveColor {
	flat, light, dark := lowerKeys(t.Labels), lowerKeys(t.Light.Labels), lowerKeys(t.Dark.Labels)
	out := map[string]lipgloss.AdaptiveColor{}
	for _, table := range []map[string]string{flat, light, dark} {
		for name := range table {
			out[name] = lipgloss.AdaptiveColor{
				Light: cmp.Or(light[name], flat[name], cText.Light),
				Dark:  cmp.Or(dark[name], flat[name], cText.Dark),
			}
		}
	}
	return out
}

// lowerKeys copies labels with lowercased names; config validation rejects names that differ only in case.
func lowerKeys(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for name, hex := range labels {
		out[strings.ToLower(name)] = hex
	}
	return out
}

// labelStyle renders a label in its [theme.labels] color, or in the text color without an entry.
func labelStyle(name string) lipgloss.Style {
	if c, ok := cLabels[strings.ToLower(name)]; ok {
		return lipgloss.NewStyle().Foreground(c)
	}
	return sText
}

// hexes lists the colors in base's order.
func hexes(c config.Colors) [8]string {
	return [8]string{c.Accent, c.OK, c.Warn, c.Bad, c.Info, c.Text, c.Dim, c.Selection}
}

// applyMode forces light/dark via lipgloss; system restores the background lipgloss detected before the first
// forced mode, since lipgloss never re-detects once a value is set.
func applyMode(mode string) {
	switch mode {
	case "light", "dark":
		if detectedDark == nil {
			d := lipgloss.HasDarkBackground()
			detectedDark = &d
		}
		lipgloss.SetHasDarkBackground(mode == "dark")
	default:
		if detectedDark != nil {
			lipgloss.SetHasDarkBackground(*detectedDark)
		}
	}
}

// agentPalette resolves the agent view's palette from config: per color and side, the [theme.light.agent_view] /
// [theme.dark.agent_view] value wins over the flat [theme.agent_view] value, which wins over the neon default.
func agentPalette(t config.Theme) flowview.Palette {
	p := flowview.NeonPalette()
	f, l, d := t.AgentView, t.Light.AgentView, t.Dark.AgentView
	for _, c := range []struct {
		dst               *lipgloss.AdaptiveColor
		flat, light, dark string
	}{
		{&p.Text, f.Text, l.Text, d.Text},
		{&p.Dim, f.Dim, l.Dim, d.Dim},
		{&p.Accent, f.Accent, l.Accent, d.Accent},
		{&p.Track, f.Track, l.Track, d.Track},
		{&p.Opus, f.Opus, l.Opus, d.Opus},
		{&p.Sonnet, f.Sonnet, l.Sonnet, d.Sonnet},
		{&p.Haiku, f.Haiku, l.Haiku, d.Haiku},
		{&p.OK, f.OK, l.OK, d.OK},
		{&p.Bad, f.Bad, l.Bad, d.Bad},
		{&p.Selection, f.Selection, l.Selection, d.Selection},
	} {
		c.dst.Light = cmp.Or(c.light, c.flat, c.dst.Light)
		c.dst.Dark = cmp.Or(c.dark, c.flat, c.dst.Dark)
	}
	return p
}

func badgeStyle(kind string) lipgloss.Style {
	c := cDim
	switch kind {
	case "ok":
		c = cOK
	case "warn":
		c = cWarn
	case "bad":
		c = cBad
	case "info":
		c = cInfo
	}
	return lipgloss.NewStyle().Foreground(c)
}

// redetected applies a background the terminal reported after start: in system mode a changed one re-pins lipgloss
// and rebuilds the styles; a forced mode or an unchanged background leaves the palette alone.
func redetected(mode string, dark bool) {
	if forcedMode(mode) || lipgloss.HasDarkBackground() == dark {
		return
	}
	lipgloss.SetHasDarkBackground(dark)
	detectedDark = &dark
	buildStyles()
}

// forcedMode reports whether mode pins the palette instead of following the terminal.
func forcedMode(mode string) bool { return mode == "light" || mode == "dark" }

// bgReplyWait bounds the wait for the terminal's answer to a focus-time background query.
const bgReplyWait = 200 * time.Millisecond

// oscBG starts the body of an OSC 11 reply, after its ESC ].
const oscBG = "11;rgb:"

// bgCapture catches the terminal's answer to a background query among the key messages: bubbletea v1 owns stdin and
// has no OSC parsing, so "\x1b]11;rgb:…" arrives as alt+], the body as runes, then alt+\ (ESC \) or ctrl+g (BEL).
type bgCapture struct {
	gen    int          // counts the queries written, to drop the messages of an ended capture
	active bool         // a query is out and its reply not yet complete
	keys   []tea.KeyMsg // the keys swallowed so far, replayed when the pattern breaks
	body   string       // the reply's runes after ESC ]
}

// bgVerdict is what a key means to an active capture.
type bgVerdict int

const (
	bgPass  bgVerdict = iota // no reply has started: the key is the user's
	bgHold                   // continues the reply: swallowed for now
	bgDone                   // ends the reply
	bgBreak                  // breaks the reply: the swallowed keys and this one are replayed
)

// feed classifies k against the reply seen so far and takes a continuing body chunk into body.
func (c *bgCapture) feed(k tea.KeyMsg) bgVerdict {
	switch {
	case len(c.keys) == 0:
		if k.Type == tea.KeyRunes && k.Alt && string(k.Runes) == "]" {
			return bgHold
		}
		return bgPass
	case k.Type == tea.KeyRunes && !k.Alt && !k.Paste:
		if body := c.body + string(k.Runes); replyBody(body) {
			c.body = body
			return bgHold
		}
	case len(c.body) > len(oscBG) && (k.Type == tea.KeyCtrlG || k.Type == tea.KeyRunes && k.Alt && string(k.Runes) == `\`):
		return bgDone
	}
	return bgBreak
}

// replyBody reports whether s can be the start of an OSC 11 reply body: a prefix of "11;rgb:", or that followed by hex
// digits and slashes.
func replyBody(s string) bool {
	if strings.HasPrefix(oscBG, s) {
		return true
	}
	rest, ok := strings.CutPrefix(s, oscBG)
	return ok && strings.Trim(rest, "0123456789abcdefABCDEF/") == ""
}

// bgSentMsg reports that the focus-time background query of capture gen was written; err is why it was not.
type bgSentMsg struct {
	gen int
	err error
}

// bgTimeoutMsg ends capture gen when the terminal has not answered in time.
type bgTimeoutMsg struct{ gen int }
