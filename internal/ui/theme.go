package ui

import (
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

// applyTheme overrides palette entries from config; empty values keep the default.
func applyTheme(t config.Theme) {
	cAccent, cOK, cWarn, cBad, cInfo, cText, cDim, cSelBg = base[0], base[1], base[2], base[3], base[4], base[5], base[6], base[7]
	set := func(c *lipgloss.AdaptiveColor, hex string) {
		if hex != "" {
			*c = lipgloss.AdaptiveColor{Light: hex, Dark: hex}
		}
	}
	set(&cAccent, t.Accent)
	set(&cOK, t.OK)
	set(&cWarn, t.Warn)
	set(&cBad, t.Bad)
	set(&cInfo, t.Info)
	set(&cText, t.Text)
	set(&cDim, t.Dim)
	set(&cSelBg, t.Selection)
	buildStyles()
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
