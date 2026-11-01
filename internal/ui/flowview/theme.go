// Package flowview draws the animated agent view of a Claude session on a character-grid canvas.
package flowview

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Role is what a cell is drawn as; the theme turns each role into one precomputed style.
type Role uint8

const (
	RoleNone     Role = iota // unstyled
	RoleText                 // body text
	RoleBold                 // titles and headers
	RoleDim                  // rules, timestamps, idle connectors, bar tracks
	RoleOpus                 // opus-family agents
	RoleSonnet               // sonnet-family agents
	RoleHaiku                // haiku-family agents
	RoleOK                   // done
	RoleBad                  // errors
	RoleSelected             // highlighted row: text on the selection background
	numRoles
)

// Palette holds the colors the roles are drawn in; the caller maps its own theme onto it.
type Palette struct {
	Text, Dim, Opus, Sonnet, Haiku, OK, Bad, Selection lipgloss.AdaptiveColor
}

// Theme holds each role's ANSI prefix and suffix, rendered once so the canvas styles a run by concatenation.
type Theme struct {
	pre, post [numRoles]string
}

// sentinel is the text rendered to split a style's output into its prefix and suffix.
const sentinel = "\x00"

// NewTheme renders the palette's styles with lipgloss's default renderer (its color profile and background).
func NewTheme(p Palette) *Theme {
	fg := func(c lipgloss.AdaptiveColor) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	styles := [numRoles]lipgloss.Style{
		RoleNone:     lipgloss.NewStyle(),
		RoleText:     fg(p.Text),
		RoleBold:     fg(p.Text).Bold(true),
		RoleDim:      fg(p.Dim),
		RoleOpus:     fg(p.Opus),
		RoleSonnet:   fg(p.Sonnet),
		RoleHaiku:    fg(p.Haiku),
		RoleOK:       fg(p.OK),
		RoleBad:      fg(p.Bad),
		RoleSelected: fg(p.Text).Background(p.Selection),
	}
	t := &Theme{}
	for r, s := range styles {
		// The sentinel always survives Render; if it ever did not, the role stays unstyled instead of garbled.
		if pre, post, ok := strings.Cut(s.Render(sentinel), sentinel); ok {
			t.pre[r], t.post[r] = pre, post
		}
	}
	return t
}

// ModelRole picks the role of a model id by family: opus, sonnet, haiku, anything else is text.
func ModelRole(model string) Role {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return RoleOpus
	case strings.Contains(m, "sonnet"):
		return RoleSonnet
	case strings.Contains(m, "haiku"):
		return RoleHaiku
	}
	return RoleText
}
