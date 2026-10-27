package provider

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/chriha/lanes/internal/filter"
	"github.com/chriha/lanes/internal/item"
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
	if it.Fields["kind"] != "pr" {
		t.Errorf("kind = %#v, want \"pr\"", it.Fields["kind"])
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

func TestToItemDiverged(t *testing.T) {
	if !strings.Contains(gqlQuery, "divergedFromTargetBranch") {
		t.Error("gqlQuery does not request divergedFromTargetBranch")
	}
	behind := item.Badge{Text: "↓ behind", Kind: "warn"}
	for _, tc := range []struct {
		name      string
		diverged  bool
		conflicts bool
		want      []item.Badge
	}{
		{"in sync", false, false, []item.Badge{{Text: "✓ ci", Kind: "ok"}}},
		{"behind", true, false, []item.Badge{{Text: "✓ ci", Kind: "ok"}, behind}},
		{"behind with conflict", true, true, []item.Badge{{Text: "✓ ci", Kind: "ok"}, behind, {Text: "⚠ conflict", Kind: "bad"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Decode the GraphQL node shape so the test also covers the JSON field mapping.
			raw := fmt.Sprintf(`{"iid":"1","headPipeline":{"status":"SUCCESS"},"conflicts":%t,"divergedFromTargetBranch":%t}`, tc.conflicts, tc.diverged)
			var mr gqlMR
			if err := json.Unmarshal([]byte(raw), &mr); err != nil {
				t.Fatal(err)
			}
			it := toItem(mr, "me", nil)
			if !reflect.DeepEqual(it.Badges, tc.want) {
				t.Errorf("badges = %v, want %v", it.Badges, tc.want)
			}
			if it.Fields["diverged"] != tc.diverged {
				t.Errorf("diverged = %v, want %v", it.Fields["diverged"], tc.diverged)
			}
			f, err := filter.Compile(`diverged && ci == "success"`)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := f.Match(it.Env()); err != nil || got != tc.diverged {
				t.Errorf("filter: got %v err %v, want %v", got, err, tc.diverged)
			}
		})
	}
}
