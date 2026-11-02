package termbg

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
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
			dark, ok := parse(tt.reply)
			if dark != tt.dark || ok != tt.ok {
				t.Errorf("parse(%q) = %v, %v; want %v, %v", tt.reply, dark, ok, tt.dark, tt.ok)
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
			if reply, ok := query(w, term, time.Second); ok || reply != "" {
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
		got, ok := ask(r, sent, time.Second)
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
		got, ok := ask(r, io.Discard, 100*time.Millisecond)
		if ok || got != "" {
			t.Errorf("ask = %q, %v; want no reply", got, ok)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("ask took %v, want about the 100ms wait", d)
		}
	})
}
