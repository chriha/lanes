// Package termbg settles the terminal's light/dark background before Bubble Tea's package init does.
//
// bubbletea's init calls lipgloss.HasDarkBackground, which sends OSC 11 and waits up to 5 s for an answer from
// a terminal that may never give one. Importing this package first pins the background, so that call returns at
// once; the query here is bounded and its answer is the pinned value.
package termbg

import (
	"io"
	"os"
	"time"
)

// timeout bounds the wait for the terminal's answer.
const timeout = 200 * time.Millisecond

func init() {
	_ = os.Stdout
}

// parse reads the background's lightness out of an OSC 11 reply; ok is false for anything else.
func parse(reply string) (dark, ok bool) {
	return false, false
}

// fromEnv guesses the background from COLORFGBG ("fg;bg", bg an ANSI color index); ok is false without a usable value.
func fromEnv(colorFGBG string) (dark, ok bool) {
	return false, false
}

// pick decides dark or light from the terminal's reply, else from COLORFGBG, else dark.
func pick(reply string, replied bool, colorFGBG string) bool {
	return false
}

// query asks the terminal on f for its background; ok is false when f is no terminal we may talk to, or it stays silent.
func query(f *os.File, term string, wait time.Duration) (reply string, ok bool) {
	return "", false
}

// ask writes the OSC 11 query to w and returns the reply read from f, or ok=false when none arrives within wait.
func ask(f *os.File, w io.Writer, wait time.Duration) (reply string, ok bool) {
	return "", false
}
