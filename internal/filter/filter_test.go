package filter

import "testing"

func TestMatch(t *testing.T) {
	env := map[string]any{"author": "ann", "me": "bob", "labels": []string{"x"}, "draft": false}
	for src, want := range map[string]bool{
		``:                              true,
		`author != me && !draft`:        true,
		`"x" in labels`:                 true,
		`"y" in labels || author == me`: false,
	} {
		f, err := Compile(src)
		if err != nil {
			t.Fatal(src, err)
		}
		if got, err := f.Match(env); err != nil || got != want {
			t.Errorf("%q: got %v, %v want %v", src, got, err, want)
		}
	}
	if _, err := Compile(`nonsense((`); err == nil {
		t.Error("bad expression should fail to compile")
	}
}
