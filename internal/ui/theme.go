package ui

import (
	"cmp"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/config"
)

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#6C3FD1", Dark: "#A78BFA"}
	cText   = lipgloss.AdaptiveColor{Light: "#1F2430", Dark: "#E6E8EE"}
	cDim    = lipgloss.AdaptiveColor{Light: "#8A90A0", Dark: "#6B7280"}
	cOK     = lipgloss.AdaptiveColor{Light: "#1E8E4E", Dark: "#4ADE80"}
	cWarn   = lipgloss.AdaptiveColor{Light: "#B7791F", Dark: "#FBBF24"}
	cBad    = lipgloss.AdaptiveColor{Light: "#C53030", Dark: "#F87171"}
	cInfo   = lipgloss.AdaptiveColor{Light: "#2B6CB0", Dark: "#60A5FA"}
	cSelBg  = lipgloss.AdaptiveColor{Light: "#DDD6F5", Dark: "#3A3560"}

	sTitle, sTab, sTabOn, sDim, sText, sID, sMark, sErr, sRule, sKey, sWarn lipgloss.Style
)

var base [8]lipgloss.AdaptiveColor

// detectedDark is the terminal background lipgloss detected before the first forced mode; nil = never forced.
var detectedDark *bool

func init() {
	base = [8]lipgloss.AdaptiveColor{cAccent, cOK, cWarn, cBad, cInfo, cText, cDim, cSelBg}
	buildStyles()
}

func buildStyles() {
	sTitle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sTab = lipgloss.NewStyle().Foreground(cDim).Padding(0, 1)
	sTabOn = lipgloss.NewStyle().Bold(true).Foreground(cText).Background(cSelBg).Padding(0, 1)
	sDim = lipgloss.NewStyle().Foreground(cDim)
	sText = lipgloss.NewStyle().Foreground(cText)
	sID = lipgloss.NewStyle().Foreground(cInfo)
	sMark = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sErr = lipgloss.NewStyle().Foreground(cBad)
	sRule = lipgloss.NewStyle().Foreground(cDim)
	sKey = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sWarn = lipgloss.NewStyle().Foreground(cWarn).Bold(true)
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
	applyMode(t.Mode)
	buildStyles()
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
