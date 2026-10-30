package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/chriha/lanes/internal/config"
)

// trueColorTheme renders styles in true color and restores the palette, the background detection and the
// recorded detection after the test, so forcing a mode cannot leak into other tests.
func trueColorTheme(t *testing.T) {
	t.Helper()
	prevProfile, prevDark, prevDetected := lipgloss.ColorProfile(), lipgloss.HasDarkBackground(), detectedDark
	lipgloss.SetColorProfile(0) // termenv.TrueColor: render the colors the test compares
	t.Cleanup(func() {
		applyTheme(config.Theme{})
		detectedDark = prevDetected
		lipgloss.SetHasDarkBackground(prevDark)
		lipgloss.SetColorProfile(prevProfile)
	})
}

func TestApplyThemeForcedMode(t *testing.T) {
	theme := func(mode string) config.Theme {
		return config.Theme{
			Mode:   mode,
			Colors: config.Colors{Accent: "#010101"},
			Light:  config.Colors{Accent: "#020202"},
			Dark:   config.Colors{Text: "#030303"},
		}
	}
	tests := []struct {
		mode               string
		detectedDark       bool // the terminal reports the opposite background
		wantAccent         string
		wantText           string
		wantAccentReverted string // built-in default once the overrides are removed
	}{
		// light: [theme.light] beats the flat key; text has no override, so the built-in Light #1F2430.
		{"light", true, "2;2;2", "31;36;48", "108;63;209"},
		// dark: no [theme.dark] accent, so the flat key; [theme.dark] text applies.
		{"dark", false, "1;1;1", "3;3;3", "167;139;250"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			trueColorTheme(t)
			detectedDark = nil
			lipgloss.SetHasDarkBackground(tt.detectedDark)

			applyTheme(theme(tt.mode))
			if got := sTitle.Render("x"); !strings.Contains(got, tt.wantAccent) {
				t.Errorf("accent = %q, want %s", got, tt.wantAccent)
			}
			if got := sText.Render("x"); !strings.Contains(got, tt.wantText) {
				t.Errorf("text = %q, want %s", got, tt.wantText)
			}

			applyTheme(config.Theme{Mode: tt.mode}) // reload without the overrides
			if got := sTitle.Render("x"); !strings.Contains(got, tt.wantAccentReverted) {
				t.Errorf("accent after removing the overrides = %q, want %s", got, tt.wantAccentReverted)
			}

			applyTheme(config.Theme{Mode: "system"}) // back to the detected background
			if got := lipgloss.HasDarkBackground(); got != tt.detectedDark {
				t.Errorf("system mode: dark background = %v, want the detected %v", got, tt.detectedDark)
			}
		})
	}
}

func TestApplyThemeSystemPrecedence(t *testing.T) {
	trueColorTheme(t)
	defOK, defWarn := cOK, cWarn
	applyTheme(config.Theme{
		Mode:   "system",
		Colors: config.Colors{Accent: "#010101"},
		Light:  config.Colors{Accent: "#020202"},
		Dark:   config.Colors{OK: "#030303"},
	})
	for _, c := range []struct {
		name      string
		got, want lipgloss.AdaptiveColor
	}{
		{"accent: light table, flat on dark", cAccent, lipgloss.AdaptiveColor{Light: "#020202", Dark: "#010101"}},
		{"ok: default light, dark table", cOK, lipgloss.AdaptiveColor{Light: defOK.Light, Dark: "#030303"}},
		{"warn: defaults", cWarn, defWarn},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

func TestApplyThemeLabelColors(t *testing.T) {
	trueColorTheme(t)
	applyTheme(config.Theme{
		Mode:   "system",
		Colors: config.Colors{Labels: map[string]string{"bug": "#010101", "Docs": "#020202", "flat-only": "#030303"}},
		Light:  config.Colors{Labels: map[string]string{"BUG": "#040404", "light-only": "#050505"}},
		Dark:   config.Colors{Labels: map[string]string{"docs": "#060606"}},
	})
	tests := []struct {
		label string
		want  lipgloss.AdaptiveColor
		ok    bool
	}{
		{"bug", lipgloss.AdaptiveColor{Light: "#040404", Dark: "#010101"}, true},  // light table, flat on dark
		{"docs", lipgloss.AdaptiveColor{Light: "#020202", Dark: "#060606"}, true}, // flat on light, dark table
		{"flat-only", lipgloss.AdaptiveColor{Light: "#030303", Dark: "#030303"}, true},
		{"light-only", lipgloss.AdaptiveColor{Light: "#050505", Dark: cText.Dark}, true}, // dark falls back to text
		{"other", lipgloss.AdaptiveColor{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got, ok := cLabels[tt.label]
			if ok != tt.ok || got != tt.want {
				t.Errorf("cLabels[%q] = %+v, %v; want %+v, %v", tt.label, got, ok, tt.want, tt.ok)
			}
		})
	}
}
