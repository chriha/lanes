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
