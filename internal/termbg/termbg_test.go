package termbg

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseDark(t *testing.T) {
	tests := []struct {
		name     string
		reply    string
		dark, ok bool
	}{
		{"black, ST", "\x1b]11;rgb:0000/0000/0000\x1b\\", true, true},
		{"white, BEL", "\x1b]11;rgb:ffff/ffff/ffff\a", false, true},
		{"two-digit channels", "\x1b]11;rgb:1e/1e/2e\x1b\\", true, true},
		{"light grey", "\x1b]11;rgb:f5f5/f5f5/f5f5\x1b\\", false, true},
		{"cursor position report", "\x1b[42;1R", false, false},
		{"garbage", "hello", false, false},
		{"empty", "", false, false},
		{"bad hex", "\x1b]11;rgb:zz/00/00\x1b\\", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dark, ok := ParseDark(tt.reply)
			if dark != tt.dark || ok != tt.ok {
				t.Errorf("ParseDark(%q) = %v, %v; want %v, %v", tt.reply, dark, ok, tt.dark, tt.ok)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	tests := []struct {
		env      string
		dark, ok bool
	}{
		{"15;0", true, true},
		{"0;15", false, true},
		{"0;default;0", true, true},
		{"7;8", true, true},
		{"", false, false},
		{"nonsense", false, false},
		{"0;x", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			dark, ok := fromEnv(tt.env)
			if dark != tt.dark || ok != tt.ok {
				t.Errorf("fromEnv(%q) = %v, %v; want %v, %v", tt.env, dark, ok, tt.dark, tt.ok)
			}
		})
	}
}

func TestPick(t *testing.T) {
	const light, dark = "\x1b]11;rgb:ffff/ffff/ffff\a", "\x1b]11;rgb:0000/0000/0000\a"
	tests := []struct {
		name     string
		reply    string
		replied  bool
		fgbg     string
		wantDark bool
	}{
		{"reply wins over COLORFGBG", light, true, "15;0", false},
		{"dark reply", dark, true, "0;15", true},
		{"no reply uses COLORFGBG light", "", false, "0;15", false},
		{"no reply uses COLORFGBG dark", "", false, "15;0", true},
		{"unparsable reply falls back to COLORFGBG", "junk", true, "0;15", false},
		{"nothing known is dark", "", false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pick(tt.reply, tt.replied, tt.fgbg); got != tt.wantDark {
				t.Errorf("pick = %v, want %v", got, tt.wantDark)
			}
		})
	}
}

func TestQuerySkipsWhatItMustNotTalkTo(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	for _, term := range []string{"xterm-256color", "screen", "tmux-256color", "dumb"} {
		t.Run(term, func(t *testing.T) {
			// a pipe is no terminal, so the query must not even be written to it
			start := time.Now()
			if reply, ok := query(w, term, "", time.Second); ok || reply != "" {
				t.Errorf("query = %q, %v; want no reply", reply, ok)
			}
			if time.Since(start) > 500*time.Millisecond {
				t.Errorf("query waited %v on something that is no terminal", time.Since(start))
			}
		})
	}
}

func TestAsk(t *testing.T) {
	t.Run("returns the reply", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		defer w.Close()
		const reply = "\x1b]11;rgb:ffff/ffff/ffff\x1b\\"
		if _, err := w.WriteString(reply); err != nil {
			t.Fatal(err)
		}
		sent, err := os.CreateTemp(t.TempDir(), "query")
		if err != nil {
			t.Fatal(err)
		}
		defer sent.Close()
		got, ok := ask(r, sent, queryOSC11, time.Second)
		if !ok || got != reply {
			t.Errorf("ask = %q, %v; want %q", got, ok, reply)
		}
		out, _ := os.ReadFile(sent.Name())
		if string(out) != "\x1b]11;?\x1b\\" {
			t.Errorf("query written = %q", out)
		}
	})
	t.Run("a silent terminal costs the wait, not seconds", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		defer w.Close()
		start := time.Now()
		got, ok := ask(r, io.Discard, queryOSC11, 100*time.Millisecond)
		if ok || got != "" {
			t.Errorf("ask = %q, %v; want no reply", got, ok)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("ask took %v, want about the 100ms wait", d)
		}
	})
}

func TestAskLeavesNoInputBehind(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	// a reply that never ends overruns the read limit: what is left in the queue would reach the TUI as keystrokes
	if _, err := w.WriteString("\x1b]11;rgb:" + strings.Repeat("f", 100)); err != nil {
		t.Fatal(err)
	}
	if got, ok := ask(r, io.Discard, queryOSC11, time.Second); ok {
		t.Fatalf("ask = %q, true; want no reply", got)
	}
	if b, ok := readByte(r, 10*time.Millisecond); ok {
		t.Errorf("input queue still holds %q after ask", b)
	}
}

func TestQuery(t *testing.T) {
	tests := []struct {
		name     string
		term     string
		tmux     string
		want     string
		wantSent bool
	}{
		{"plain terminal", "xterm-256color", "", "\x1b]11;?\x1b\\", true},
		{"plain terminal ignores TMUX", "xterm-256color", "/tmp/tmux-1/default,1,0", "\x1b]11;?\x1b\\", true},
		{"tmux term inside tmux is wrapped", "tmux-256color", "/tmp/tmux-1/default,1,0", "\x1bPtmux;\x1b\x1b]11;?\x1b\x1b\\\x1b\\", true},
		{"screen term inside tmux is wrapped", "screen-256color", "/tmp/tmux-1/default,1,0", "\x1bPtmux;\x1b\x1b]11;?\x1b\x1b\\\x1b\\", true},
		{"tmux term without TMUX is skipped", "tmux-256color", "", "", false},
		{"screen term without TMUX is skipped", "screen", "", "", false},
		{"dumb is skipped", "dumb", "/tmp/tmux-1/default,1,0", "", false},
		{"empty TERM", "", "", "\x1b]11;?\x1b\\", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Query(tt.term, tt.tmux)
			if ok != tt.wantSent || got != tt.want {
				t.Errorf("Query(%q, %q) = %q, %v; want %q, %v", tt.term, tt.tmux, got, ok, tt.want, tt.wantSent)
			}
		})
	}
}
