package ui

import (
	"testing"

	"github.com/chriha/lanes/internal/config"
)

func TestRunActionTemplateErrors(t *testing.T) {
	tests := []struct {
		name       string
		fields     map[string]any
		act        config.Action
		wantNotice string
	}{
		{
			name: "gitlab identifier", fields: map[string]any{"iid": 7},
			act:        config.Action{Type: "run", Run: "echo {{.nope}}"},
			wantNotice: `action a: no field "nope"; this item's identifier is "iid" (see fields with p)`,
		},
		{
			name: "github identifier", fields: map[string]any{"number": 7},
			act:        config.Action{Type: "run", Run: "echo {{.iid}}"},
			wantNotice: `action a: no field "iid"; this item's identifier is "number" (see fields with p)`,
		},
		{
			name: "linear identifier", fields: map[string]any{"key": "ENG-1"},
			act:        config.Action{Type: "run", Run: "echo {{.iid}}"},
			wantNotice: `action a: no field "iid"; this item's identifier is "key" (see fields with p)`,
		},
		{
			name: "iid wins over number", fields: map[string]any{"iid": 1, "number": 2},
			act:        config.Action{Type: "run", Run: "echo {{.nope}}"},
			wantNotice: `action a: no field "nope"; this item's identifier is "iid" (see fields with p)`,
		},
		{
			name: "command provider falls back to id", fields: map[string]any{},
			act:        config.Action{Type: "run", Run: "echo {{.iid}}"},
			wantNotice: `action a: no field "iid"; this item's identifier is "id" (see fields with p)`,
		},
		{
			name: "cwd keeps its prefix", fields: map[string]any{"number": 7},
			act:        config.Action{Type: "run", Run: "true", Cwd: "/tmp/{{.iid}}"},
			wantNotice: `action a cwd: no field "iid"; this item's identifier is "number" (see fields with p)`,
		},
		{
			name: "missing var keeps the raw error", fields: map[string]any{"iid": 7},
			act:        config.Action{Type: "run", Run: "echo {{.vars.x}}"},
			wantNotice: `action a: template: :1:12: executing "" at <.vars.x>: map has no entry for key "x"`,
		},
		{
			name: "other errors are unchanged", fields: map[string]any{},
			act:        config.Action{Type: "run", Run: "echo {{.id | nofunc}}"},
			wantNotice: `action a: template: :1: function "nofunc" not defined`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := whenModel(t)
			m.cursor = 0
			f := m.items()[0].Fields // shared with the view's copy
			clear(f)
			for k, v := range tt.fields {
				f[k] = v
			}
			m.runAction("a", tt.act)
			if m.notice != tt.wantNotice {
				t.Errorf("notice = %q, want %q", m.notice, tt.wantNotice)
			}
		})
	}
}
