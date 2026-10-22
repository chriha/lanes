package provider

import (
	"testing"

	"github.com/chriha/lanes/internal/filter"
)

func TestToItemAndFilter(t *testing.T) {
	var mr gqlMR
	mr.IID, mr.Title, mr.SourceBranch = "7", "Draft: ABC-12 thing", "fix/abc-12"
	mr.Author.Username = "alex"
	mr.Labels.Nodes = append(mr.Labels.Nodes, struct{ Title string }{"waiting-for-review"})
	it := toItem(mr, "me", &threadInfo{open: 1, byMe: true})
	if it.Title != "ABC-12 thing" || it.Fields["ticket"] != "ABC-12" || it.Fields["open_thread_by_me"] != true {
		t.Fatalf("bad item: %+v", it)
	}
	for src, want := range map[string]bool{
		`"waiting-for-review" in labels && author != me`: true,
		`!open_thread_by_me`:                             false,
		`author == me`:                                   false,
	} {
		f, err := filter.Compile(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Match(it.Env())
		if err != nil || got != want {
			t.Errorf("%s: got %v err %v, want %v", src, got, err, want)
		}
	}
}
