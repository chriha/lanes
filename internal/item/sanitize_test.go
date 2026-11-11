package item

import (
	"reflect"
	"testing"
)

func TestSanitizeTitle(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "fix the thing", "fix the thing"},
		{"non-ascii intact", "Größe 日本語 😀 é", "Größe 日本語 😀 é"},
		{"clear screen csi", "a\x1b[2Jb", "a[2Jb"},
		{"osc 52 clipboard", "a\x1b]52;c;aGk=\x07b", "a]52;c;aGk=b"},
		{"osc 8 hyperlink", "\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\", "]8;;http://x\\link]8;;\\"},
		{"c1 csi", "a\u009bb", "ab"},
		{"bell", "a\x07b", "ab"},
		{"del", "a\x7fb", "ab"},
		{"nul", "a\x00b", "ab"},
		{"newline becomes space", "a\nb", "a b"},
		{"tab becomes space", "a\tb", "a b"},
		{"crlf becomes one space", "a\r\nb", "a b"},
		{"run of whitespace controls collapses", "a\n\n\tb", "a b"},
		{"real spaces kept", "a  b", "a  b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sanitize([]Item{{Title: tt.in}})[0].Title; got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	in := []Item{{
		ID: "!1\x1b[2J", Title: "t\x07\nx", URL: "u\x9b", Author: "a\x1b]52;c;x\x07",
		Badges:  []Badge{{Text: "ok\x07", Kind: "ok\x1b"}},
		Pending: []string{"ci\x07"},
		Fields: map[string]any{
			"k\x07":  "v\x1b[2J",
			"n":      2.0,
			"b":      true,
			"nil":    nil,
			"list":   []any{"x\x07", 1.0, map[string]any{"deep": "d\x9b"}},
			"nested": map[string]any{"a": []any{"\x1b[H"}},
			"strs":   []string{"s\x07"},
		},
	}}
	want := []Item{{
		ID: "!1[2J", Title: "t x", URL: "u", Author: "a]52;c;x",
		Badges:  []Badge{{Text: "ok", Kind: "ok"}},
		Pending: []string{"ci"},
		Fields: map[string]any{
			"k":      "v[2J",
			"n":      2.0,
			"b":      true,
			"nil":    nil,
			"list":   []any{"x", 1.0, map[string]any{"deep": "d"}},
			"nested": map[string]any{"a": []any{"[H"}},
			"strs":   []string{"s"},
		},
	}}
	if got := Sanitize(in); !reflect.DeepEqual(got, want) {
		t.Errorf("Sanitize =\n%#v\nwant\n%#v", got, want)
	}
	if Sanitize(nil) != nil {
		t.Error("Sanitize(nil) is not nil")
	}
}

func TestSanitizeDetailText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "fix the thing", "fix the thing"},
		{"newlines kept", "a\nb\n\nc", "a\nb\n\nc"},
		{"tab becomes a space", "a\tb", "a b"},
		{"carriage return dropped", "a\r\nb", "a\nb"},
		{"osc 52 clipboard", "a\x1b]52;c;aGk=\x07b", "a]52;c;aGk=b"},
		{"osc 8 hyperlink", "\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\", "]8;;http://x\\link]8;;\\"},
		{"csi cursor move", "a\x1b[2;3Hb", "a[2;3Hb"},
		{"raw 0x9b byte", "a\x9bb", "ab"},
		{"c1 csi rune", "a\u009bb", "ab"},
		{"del and nul", "a\x7f\x00b", "ab"},
		{"non-ascii intact", "Größe 日本語 😀", "Größe 日本語 😀"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Detail{
				Description: tt.in,
				Threads:     []Thread{{Comments: []Comment{{Body: tt.in}}}},
				Comments:    []Comment{{Body: tt.in}},
			}
			got := SanitizeDetail(d)
			for field, g := range map[string]string{
				"description": got.Description, "thread body": got.Threads[0].Comments[0].Body, "comment body": got.Comments[0].Body,
			} {
				if g != tt.want {
					t.Errorf("%s = %q, want %q", field, g, tt.want)
				}
			}
		})
	}
}

func TestSanitizeDetail(t *testing.T) {
	const dirty = "x\x1b]52;c;aGk=\x07\x9by"
	const clean = "x]52;c;aGk=y"
	in := Detail{
		Description: "d\n" + dirty,
		Pipeline: &Pipeline{
			ID: dirty, Status: dirty, Duration: 3,
			Jobs: []Job{{Name: dirty, Stage: dirty, Status: dirty, Duration: 5}},
		},
		Threads: []Thread{{
			File: dirty + "\n", Line: 4, Resolved: true, URL: "https://h/" + dirty,
			Comments: []Comment{{Author: dirty + "\t", Body: "b\n" + dirty}},
		}},
		Comments: []Comment{{Author: dirty, Body: dirty}},
	}
	want := Detail{
		Description: "d\n" + clean,
		Pipeline: &Pipeline{
			ID: clean, Status: clean, Duration: 3,
			Jobs: []Job{{Name: clean, Stage: clean, Status: clean, Duration: 5}},
		},
		Threads: []Thread{{
			File: clean + " ", Line: 4, Resolved: true, URL: "https://h/" + clean,
			Comments: []Comment{{Author: clean + " ", Body: "b\n" + clean}},
		}},
		Comments: []Comment{{Author: clean, Body: clean}},
	}
	got := SanitizeDetail(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SanitizeDetail =\n%+v\nwant\n%+v", got, want)
	}
	if in.Pipeline.ID != dirty || in.Threads[0].Comments[0].Body != "b\n"+dirty {
		t.Error("SanitizeDetail changed its argument")
	}
	if z := SanitizeDetail(Detail{}); !reflect.DeepEqual(z, Detail{}) {
		t.Errorf("zero Detail = %+v, want zero", z)
	}
}
