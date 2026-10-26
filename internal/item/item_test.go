package item

import "testing"

func TestEnvNote(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		want   any
	}{
		{"no note is empty", nil, ""},
		{"injected note", map[string]any{"note": "waiting"}, "waiting"},
		{"provider field wins", map[string]any{"note": 3}, 3},
	}
	for _, tt := range tests {
		if got := (Item{Fields: tt.fields}).Env()["note"]; got != tt.want {
			t.Errorf("%s: note = %v, want %v", tt.name, got, tt.want)
		}
	}
}
