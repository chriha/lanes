# Troubleshooting

Run `lanes doctor` first. Without starting the TUI or touching any tmux server, it checks `tmux` (in `PATH`,
≥ 3.2), `glab` and `glab auth status` (only when a `gitlab` provider is configured) and likewise `gh` and
`gh auth status` for `github` and `github-issues` (an `auto` provider counts as the forge its `origin` resolves to here; without an
`origin` it warns), that the global config and the project `.lanes.toml` each parse (one line per file, with its
path), that the merged config loads, that
every view filter compiles and every action template parses, and that the state and cache directories are
writable (it warns when their mode is not `0700`; fix with `chmod 700 <dir>`). When an action's `run` uses
`{{.settings}}` it also checks that `claude` is in `PATH`, and it always checks that the running `lanes` binary
still exists and is executable, since Claude hooks call it by that path (reinstall lanes if it moved). A `trust`
line names the trusted project root and whether it is "always trust" or trusted by hash. It prints one
`ok` / `warn` / `fail` line per check, naming the view or action, exits 1 on any fail, and puts the fix at the end of
a failing line in parentheses.

- **`glab: …` / `gh: …` error in the list** — run `glab auth status` / `gh auth status`; `lanes` needs a
  logged-in CLI. For a project that cannot be detected from the git remote, set `project` on the provider.
- **`auto: cannot detect the forge …`** — the directory has no `origin` remote; set `type = "github"` or
  `type = "gitlab"` on the provider (in a `.lanes.toml` for that project).
- **`tmux not found in PATH`** — install tmux (≥ 3.2).
- **A key does nothing** — it may be a built-in key; open `?` and look at the actions list, shadowed bindings
  are flagged.
- **An action reports `no field "x"; this item's identifier is …`** — the template uses a field the item does
  not have; check the name against `p`. **`map has no entry for key "x"`** means `{{.vars.x}}` is not defined:
  add it to the action's `vars`.
- **The list is empty** — the filter excludes everything, or its fields don't exist for that provider. Press
  `F` to look at the filter, or `:filter reset`.
- **Start fresh** — delete `~/.local/state/lanes` and `~/.cache/lanes` (and the config if you want the
  defaults back). Running sessions: `tmux -L lanes-<project> kill-server`.
