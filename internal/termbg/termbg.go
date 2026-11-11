// Package termbg settles the terminal's light/dark background: a bounded query for the real terminal, instead of
// the 5 s wait termenv puts on one that never answers. Bubbletea's init is kept from querying by package quietterm.
package termbg

import (
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// timeout bounds the wait for the terminal's answer. An answering terminal needs a few milliseconds.
const timeout = 200 * time.Millisecond

// queryOSC11 asks for the background color; the reply is "\x1b]11;rgb:RRRR/GGGG/BBBB" ended by BEL or ESC \.
const queryOSC11 = "\x1b]11;?\x1b\\"

var once sync.Once

// queryFor returns the bytes that ask the terminal for its background given TERM and $TMUX; ok is false when the
// terminal must not be asked. Inside tmux the query is wrapped in tmux's passthrough.
func queryFor(termName, tmuxEnv string) (q string, ok bool) {
	if strings.HasPrefix(termName, "dumb") {
		return "", false
	}
	if strings.HasPrefix(termName, "screen") || strings.HasPrefix(termName, "tmux") {
		// screen and tmux front several terminals, so they cannot answer for one, unless tmux relays the query
		// to the outer terminal: its passthrough takes the sequence with every ESC doubled. It needs the tmux
		// option allow-passthrough; without it the query is dropped and we fall back as for a silent terminal.
		if tmuxEnv == "" {
			return "", false
		}
		return "\x1bPtmux;" + strings.ReplaceAll(queryOSC11, "\x1b", "\x1b\x1b") + "\x1b\\", true
	}
	return queryOSC11, true
}

// Settle pins lipgloss's background to what the terminal on stdout reports, the first time it is called. Call it
// after quietterm.Restore and before anything renders an adaptive color. It waits at most timeout.
func Settle() {
	once.Do(func() {
		reply, ok := query(os.Stdout, os.Getenv("TERM"), os.Getenv("TMUX"), timeout)
		lipgloss.SetHasDarkBackground(pick(reply, ok, os.Getenv("COLORFGBG")))
	})
}

// pick decides dark or light from the terminal's reply, else from COLORFGBG, else dark.
func pick(reply string, replied bool, colorFGBG string) bool {
	if replied {
		if dark, ok := parse(reply); ok {
			return dark
		}
	}
	if dark, ok := fromEnv(colorFGBG); ok {
		return dark
	}
	return true
}

// parse reads the background's lightness out of an OSC 11 reply; ok is false for anything else.
func parse(reply string) (dark, ok bool) {
	_, rest, found := strings.Cut(reply, "rgb:")
	if !found {
		return false, false
	}
	rest = strings.TrimRight(rest, "\a\x1b\\")
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		return false, false
	}
	var c [3]float64
	for i, p := range parts {
		if p == "" || len(p) > 4 {
			return false, false
		}
		v, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return false, false
		}
		c[i] = float64(v) / float64(uint64(1)<<(4*len(p))-1)
	}
	hi, lo := max(c[0], c[1], c[2]), min(c[0], c[1], c[2])
	return (hi+lo)/2 < 0.5, true // HSL lightness, as termenv judges it
}

// fromEnv guesses the background from COLORFGBG ("fg;bg", bg an ANSI color index); ok is false without a usable value.
func fromEnv(colorFGBG string) (dark, ok bool) {
	i := strings.LastIndex(colorFGBG, ";")
	if i < 0 {
		return false, false
	}
	n, err := strconv.Atoi(colorFGBG[i+1:])
	if err != nil || n < 0 || n > 15 {
		return false, false
	}
	return n < 7 || n == 8, true
}

// query asks the terminal on f for its background; ok is false when f is no terminal we may talk to, or it stays silent.
func query(f *os.File, termName, tmuxEnv string, wait time.Duration) (reply string, ok bool) {
	q, ok := queryFor(termName, tmuxEnv)
	if !ok {
		return "", false
	}
	fd := f.Fd()
	if !term.IsTerminal(fd) || !foreground(fd) {
		return "", false
	}
	old, err := term.MakeRaw(fd) // no echo of the reply, no line buffering
	if err != nil {
		return "", false
	}
	defer func() {
		if err := term.Restore(fd, old); err != nil {
			reply, ok = "", false // a terminal we cannot put back is not one to trust
		}
	}()
	return ask(f, f, q, wait)
}

// ask writes the query q to w and returns the reply read from f, or ok=false when none arrives within wait.
func ask(f *os.File, w io.Writer, q string, wait time.Duration) (reply string, ok bool) {
	// Whatever is still queued once we stop reading (the rest of an overlong or garbled reply) would reach the
	// TUI as keystrokes. ponytail: a reply that arrives after this point cannot be told from typing and still leaks.
	defer drain(f)
	if _, err := io.WriteString(w, q); err != nil {
		return "", false
	}
	deadline := time.Now().Add(wait)
	var buf []byte
	for len(buf) < 64 {
		b, ok := readByte(f, time.Until(deadline))
		if !ok {
			return "", false
		}
		if len(buf) == 0 && b != 0x1b {
			continue // skip anything typed before the reply
		}
		buf = append(buf, b)
		if b == '\a' || (b == '\\' && len(buf) >= 2 && buf[len(buf)-2] == 0x1b) {
			return string(buf), true
		}
	}
	return "", false
}

// drain discards the input already queued on f, waiting at most a millisecond for the first byte.
func drain(f *os.File) {
	for {
		if _, ok := readByte(f, time.Millisecond); !ok {
			return
		}
	}
}
