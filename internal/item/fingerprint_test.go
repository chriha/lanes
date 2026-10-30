package item

import "testing"

func TestFingerprint(t *testing.T) {
	base := func() Item {
		return Item{
			ID: "!1", Title: "fix it", URL: "u", Author: "a",
			Badges: []Badge{{"CI", "ok"}},
			Fields: map[string]any{"ci": "ok", "votes": 2.0, "labels": []any{"x"}},
		}
	}
	tests := []struct {
		name   string
		change func(*Item)
		same   bool
	}{
		{"identical data", func(*Item) {}, true},
		{"note is injected, not data", func(i *Item) { i.Fields["note"] = "later" }, true},
		{"snoozed is injected, not data", func(i *Item) { i.Fields["snoozed"] = true }, true},
		{"url and author are not part of it", func(i *Item) { i.URL, i.Author = "v", "b" }, true},
		{"title", func(i *Item) { i.Title = "fix it now" }, false},
		{"draft", func(i *Item) { i.Draft = true }, false},
		{"badge", func(i *Item) { i.Badges[0].Kind = "bad" }, false},
		{"field value", func(i *Item) { i.Fields["ci"] = "failed" }, false},
		{"new field", func(i *Item) { i.Fields["x"] = 1.0 }, false},
		{"nested value", func(i *Item) { i.Fields["labels"] = []any{"y"} }, false},
		{"values don't run together", func(i *Item) { i.Fields = map[string]any{"ci": "ok votes:2"} }, false},
	}
	want := base().Fingerprint()
	pending := base()
	pending.Pending = []string{"ci"}
	if pending.Fingerprint() == want {
		t.Error("marking ci pending did not drop it from the fingerprint")
	}
	pendingWant := pending.Fingerprint()
	t.Run("pending field value is ignored", func(t *testing.T) {
		it := base()
		it.Pending = []string{"ci"}
		it.Fields["ci"] = "failed"
		if it.Fingerprint() != pendingWant {
			t.Error("a pending field's value changed the fingerprint")
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := base()
			tt.change(&it)
			if got := it.Fingerprint(); (got == want) != tt.same {
				t.Errorf("same = %v, want %v", got == want, tt.same)
			}
		})
	}
}
