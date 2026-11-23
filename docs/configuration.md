# Configuration

## Global and project config

- **Global** config: `~/.config/lanes/config.toml` (honours `$XDG_CONFIG_HOME`). Created with defaults on first
  start. Put *your* views, actions and keys here.
- **Project** config: `.lanes.toml` in the project directory — `lanes` uses the nearest one in the working
  directory or any parent (the search stops at your home directory). Use it for project-specific settings such
  as the GitLab project, an extra view, or a project-specific action. Create it with `lanes init` or
  `:edit project` (which also opens it in your editor). Both write the same file to the current directory:
  every option, explained and commented out, except a `[provider.forge]` whose `type` and `project` are taken
  from the git `origin` (it replaces the global `forge` provider the default views use; with no `origin` it is
  commented out too). If a `.lanes.toml` already exists, `lanes init` on a terminal asks
  `Overwrite it with the default template? [y/N]` (default no; declining keeps the file and exits 0) and then
  `Keep a backup of the current file as .lanes.toml.bak? [Y/n]` (default yes; an older `.bak` is replaced). Prompts
  go to stderr. Without a terminal on stdin it refuses (exit 1) and never overwrites. `:edit project` never overwrites.

The project config **overlays** the global one — it does not replace it. You only write what differs:

| Section            | Merged                                                                 |
|--------------------|------------------------------------------------------------------------|
| `[settings]`       | per field — set only `preview = false` and `poll_interval` is kept     |
| `[theme]`          | per field, also within `[theme.light]`, `[theme.dark]` and `[theme.agent_view]` |
| `[keymap]`         | per key                                                                |
| `[provider.<n>]`   | by name — a provider defined in the project replaces the global one of that name as a whole |
| `[action.<n>]`     | by name, as a whole                                                    |
| `[[view]]`         | by `name` — same name replaces the global view, new names are appended |
| `[[marker]]`       | by `text` — same text replaces the global marker in place, new ones are appended |

To share views, actions and keys between some projects (not all), define them once as a [profile](#profiles) in the
global config and select it with `profiles = [...]` in each project.

Example project config:

```toml
# .lanes.toml
[provider.gitlab]
type = "gitlab"
project = "acme/payments"

[[view]]
name = "payments-review"
provider = "gitlab"
filter = '"needs-review" in labels && author != me'

[keymap]
r = "review"
```

Both files are watched. Saving either one — or pressing `ctrl-r` / `:reload` — applies the change **to the
running app**: views, keymap, actions, theme and poll interval update, lists refetch, and the footer reports
`config reloaded: 4 views, 3 actions`. If a file does not parse, the old configuration stays active and the
footer shows the error. The reload (trust check, reading the files, building the providers) runs in the background:
the keys keep working and the old configuration stays active until the new one is ready. Applying it cancels the
fetches still running for the old configuration and fetches every provider again at once.

## Profiles

A profile is a named set of views, actions and keymap entries, defined once in the **global** config and selected
by the projects that want it — instead of copying the same blocks into every `.lanes.toml`. A profile mirrors the
top-level shape under `[profile.<name>]`:

```toml
# ~/.config/lanes/config.toml
[[profile.go.view]]
name = "go"
provider = "forge"
filter = '"go" in labels'

[profile.go.action.fix-ci]
run = 'claude "/fix-ci {{.url}}"'

[profile.go.keymap]
f = "fix-ci"

[[profile.review.view]]
name = "review"
provider = "forge"
filter = '"needs-review" in labels && author != me'
```

A project selects profiles with a top-level `profiles` key in its `.lanes.toml` (before any `[table]`, as TOML
requires for top-level keys):

```toml
# .lanes.toml
profiles = ["review", "go"]
```

**Precedence:** global top-level items < each selected profile, in listed order (later wins) < the project's own
items. Every level merges the same way as the project overlay: views by `name`, actions by name as a whole, keymap
per key. The global top-level views, actions and keymap stay active in a project that selects profiles; a profile
only adds items or replaces them by name. A project without `profiles` loads exactly as before, and profiles nobody
selects are ignored.

- **Providers** are not part of a profile. A profile's view names a provider defined globally or in the project
  (typically `forge`, which the project may redefine); an unknown provider is an error as for any view.
- **Validation** is the same as for top-level items (reserved view names, `on_enter`, `when`, action chains, keymap
  targets); errors name the profile they come from (`…: profile "go": …`).
- **Unknown profile:** selecting a name the global config does not define fails to load with
  `.lanes.toml: unknown profile "x" (defined: go, review)`.
- **Global only:** `[profile.*]` in a `.lanes.toml` is an error (profiles are defined in the global config only),
  and so is `profiles` in the global config.
- **Reload:** editing either file reloads like any other change; on an error the old configuration stays active.

## Trusting a project

A `.lanes.toml` can run shell commands (`command` providers, actions), so `lanes` does not read a project it has
not been told to trust. Before it runs git, loads any config or starts a provider, it finds the project root (the
nearest directory with a `.git` directory or file, found without running git, so worktrees count; otherwise the
current directory) and checks it against its trust list. It asks in every new project, even one without a
`.lanes.toml`, since a later pull could add one. The trust hash covers every `.lanes.toml` under that root and any
in a parent directory up to your home directory, since `lanes` loads those too: editing one makes the project
untrusted again.

The TUI checks in the background behind a `checking whether <root> is trusted…` frame, since hashing every
`.lanes.toml` under a large root can take seconds (`ctrl-c` quits it). If the root is not trusted, the TUI,
`lanes init` and `lanes doctor` show a dialog:

```
Do you trust /home/you/src/payments?

Its .lanes.toml can run shell commands (command providers, actions).

Type yes to trust it: [NO]
[ ] always trust this project, even when its .lanes.toml changes

enter confirm · tab toggle always · esc quit
```

Only `yes` (any case, surrounding spaces ignored) followed by `enter` trusts the project; `lanes` then continues and
does not ask again there. Anything else — an empty answer, other text, `esc`, `ctrl-c` — is a no: `lanes` prints
`not trusted` to stderr, exits 1 and records nothing, whether the checkbox is checked or not.

- **Always trust.** `tab` toggles the checkbox (`[x]`); it starts unchecked and only counts together with `yes`.
  Checked, `lanes` trusts this project root whatever its `.lanes.toml` files say: changing, adding or removing one
  never asks again, and while `lanes` runs a changed `.lanes.toml` is reloaded and applied like any trusted change.
  Only the root shown in the dialog is covered; other projects are still asked the first time. Use it for projects
  you edit yourself, not for repositories where a `git pull` could bring in commands you have not read.

- **Changes ask again.** The trust covers the content of every `.lanes.toml` under the root (`.git` and
  `node_modules` are skipped). Changing one, or adding one where there was none, makes the project untrusted at the
  next start. While `lanes` runs, a reload of an untrusted change is not applied: the old config stays and the footer
  shows `.lanes.toml changed — restart lanes to trust it`. Edits you make through `lanes` itself — `:edit project` and
  `lanes init` in a trusted project — are trusted right away. In an always-trusted project none of this applies, and
  `:edit project` and `lanes init` keep it always-trusted.
- **No terminal, no trust.** Without a terminal on stdin an untrusted project is refused with
  `<root> is not trusted; run lanes in a terminal to trust it` and exit 1. There is deliberately no way to trust a
  project from a script — no subcommand, flag or environment variable; a person has to type `yes`.
- **Silent commands.** `lanes status` and the Claude Code hooks (`lanes hook`) never ask: in an untrusted project
  `status` prints nothing and the hooks do nothing, except in a session `lanes` started itself (its working
  directory, e.g. a git worktree, may be a root of its own).
- **Untrust.** `lanes --untrust` removes the current project root's entry, whether it is a hash or always, prints
  `<root> is no longer trusted` (or `<root> was not trusted` when there was no entry) and exits 0; the next start
  shows the dialog again. It runs before any git, config or provider, needs no terminal and never asks, so scripts
  may use it. With a corrupt trust list it reports the error on stderr, exits 1 and leaves the file alone.
- **The trust list** is `trust.json` in the state directory (see [Where things live](#where-things-live)), mode 0600,
  one entry per project root: the hash of the root's `.lanes.toml` files, or `"always"` for an always-trusted root.
  Use `lanes --untrust` to be asked again, or delete an entry or the whole file by hand. A corrupt file trusts
  nothing: `lanes` reports the error and leaves the file alone rather than overwriting it.
- **The walk cache** in `trust-walk/` in the state directory holds, per project root, the directories under it, their
  modification times and which of them hold a `.lanes.toml`, so the next start reads only the directories that
  changed instead of walking the whole tree. It holds no trust decision and no file contents: every `.lanes.toml` it
  finds is read and hashed again on each start. A missing, corrupt or outdated cache file just means a full walk, so
  it is safe to delete any of them, or the whole directory, at any time.

## Full reference

```toml
profiles = ["<name>", ...]  # project config only, before any [table]: select global profiles (see Profiles)

[settings]
poll_interval = "30s"   # how often lists refresh (minimum 5s)
poll_interval_blurred = "5m"  # how often lists refresh while the terminal is unfocused (never below poll_interval)
preview       = false   # open the preview pane at startup
no_notify     = false   # disable desktop notifications
no_update_check = false # never check GitHub for a newer lanes release (see Updates under Install)
claude_dir    = "~/.claude"   # show this Claude profile's usage in the footer (see Claude Code integration)
max_claude_sessions = 3 # live action sessions (waiting Claude ones excluded) before more starts queue; 0 = no limit (see Running on many items)

[provider.<name>]
type    = "gitlab"      # or "github", "github-issues", "linear", "auto" (github for a github.com origin, else gitlab), "command"
project = "group/project"   # gitlab/github/github-issues (owner/repo); default: detected from git origin
command = "..."             # command provider
team      = "ENG"             # linear: team key (required)
token_env = "LINEAR_API_KEY"  # linear: env var holding the API key

[[view]]
name     = "review"
provider = "<name>"
filter   = '<expression>'   # empty = everything
sort     = "-field,field"    # "-" = descending; empty = provider order
groups   = [{ name = "<name>", filter = '<expression>' }]   # first match wins; the rest stay ungrouped below
on_enter = "<action name>"   # start this action on items that newly enter the view (see Running on arrival)
on_enter_repeat = "24h"      # optional: fire again on re-entry once the last auto-start is older than this

[[marker]]
text   = "<emoji or short text>"   # shown after the name in the list row
labels = ["<label>"]               # any of them (case-insensitive) shows the marker
color  = "#RRGGBB"                 # optional; #RGB or #RRGGBB, empty = text color

[keymap]
<key> = "<action name>"     # single character, e.g. r

[action.<name>]
type    = "session"         # or "run"
run     = '<templated shell command>'
resume  = '<templated shell command>'   # gr: resume the newest finished Claude session; adds {{.claude_session}}
cwd     = '<templated dir>'
confirm = "none"            # or "single"
vars    = { key = "value" }
refresh = false
when    = '<expression>'    # only run on items it matches; empty = every item

[profile.<name>]             # global config only; see Profiles
# [[profile.<name>.view]], [profile.<name>.action.<n>], [profile.<name>.keymap]: same keys as [[view]], [action.<n>], [keymap]

[theme]
accent = "#A78BFA"          # also: ok, warn, bad, info, text, dim, selection

[theme.agent_view]
opus = "#FF4FE3"            # also: sonnet, haiku, text, dim, accent, track, ok, bad, selection
```

## Markers

A `[[marker]]` flags list rows by label: when an item carries any of the marker's `labels` (exact name, compared
case-insensitively, like `[theme.labels]`), its `text` shows right after the name in the row, before a ticket's
priority strip. Several matching markers show in config order, separated by a space. Markers are never truncated; the
title shrinks first. Items without a `labels` list get none.

```toml
[[marker]]
text   = "🤖"
labels = ["review::ai"]

[[marker]]
text   = "🧑"
labels = ["review::human", "review::needed"]
color  = "#F59E0B"          # optional
```

`text` and `labels` are required and `text` must be unique within a file; `color` must be `#RGB` or `#RRGGBB`. Anything
else is a config error naming the file and the entry. A project marker with the same `text` replaces the global one;
new ones are appended. Changes apply on reload. Markers are not part of [profiles](#profiles).

## Theme

`lanes` ships an adaptive light/dark palette. Override any of `accent`, `ok`, `warn`, `bad`, `info`, `text` (body), `dim`
(secondary text) and `selection` (selected row and tab background) with a hex color under `[theme]`. Changes apply live.

The selected row and the active tab are painted with a medium-strength violet `selection` background (dark `#404872`, light `#CFC4F0`).
Set `selection` (flat, or per mode under `[theme.light]` / `[theme.dark]`) to change it.

The footer draws errors in the `bad` color and warnings (a request that was not fully carried out, such as
`tmux unavailable` or a skipped item) in `warn`; other messages stay plain.

`mode` picks the palette: `system` (the default) follows the terminal background, detected at start and again each
time the terminal window regains focus, so switching the system or terminal between light and dark takes effect when
you come back to `lanes`; `light` and `dark` force that palette. Forcing a mode helps when detection is slow or wrong,
for example over SSH. A focus regain the terminal does not answer within 200 ms keeps the current palette.

Inside tmux, detection asks the outer terminal through tmux's passthrough, which needs `set -g allow-passthrough on`
in your tmux config, at start and on focus regain alike. Without it the answer never arrives: at start `lanes` falls
back to `COLORFGBG`, then to the dark palette, and on focus regain it keeps the palette; on a light terminal set that
option or use `mode = "light"`.

The flat `[theme]` keys apply to both modes. `[theme.light]` and `[theme.dark]` take the same color keys for one mode
only. Per key, the active mode's table wins over the flat `[theme]` value, which wins over the built-in default:

```toml
[theme]
mode = "system"        # system | light | dark
accent = "#A78BFA"     # both modes
[theme.light]
accent = "#6C3FD1"     # light mode only; wins over the flat accent
[theme.dark]
selection = "#2E2A4F"  # dark mode only
```

Global config and `.lanes.toml` merge per field, also within `[theme.light]` and `[theme.dark]`. An invalid `mode` is a
config error, like an invalid color.

`[theme.labels]` colors labels in the preview's `labels` line by name. Names match case-insensitively (`Bug` uses the
key `bug`); a label without an entry keeps the text color. `[theme.light.labels]` and `[theme.dark.labels]` work like
the other per-mode tables: per label, the active mode's entry wins over `[theme.labels]`, which wins over the text color.
Global config and `.lanes.toml` merge per label. Two names in one table that differ only in case are a config error:

```toml
[theme.labels]
bug = "#F87171"
needs-info = "#FBBF24"
[theme.light.labels]
bug = "#C53030"        # light mode only
```

### Agent view colors

The [agent view](claude.md#agent-view) has its own neon palette. `[theme.agent_view]` overrides any of its colors: `opus`,
`sonnet` and `haiku` (the model families), `text`, `dim` (labels, connectors), `accent` (header), `track` (the
empty part of bars and gauges), `ok`, `bad` and `selection` (the active hook state). `[theme.light.agent_view]`
and `[theme.dark.agent_view]` take the same keys for one mode; per key, the active mode's table wins over
`[theme.agent_view]`, which wins over the neon default. Global config and `.lanes.toml` merge per key:

```toml
[theme.agent_view]
sonnet = "#00F0FF"     # both modes
[theme.light.agent_view]
sonnet = "#00707F"     # light mode only
[theme.dark.agent_view]
track = "#1A1030"      # dark mode only
```

Colors must be `#RGB` or `#RRGGBB` (case-insensitive); an empty value keeps the default (a label color cannot be
empty). Anything else (`A78BFA`, `red`, `212`) is a config error: `lanes` refuses to start, keeps the previous config
on reload, and `lanes doctor` reports a failing `config` line.

## Where things live

| What                                   | Where (default)                                         |
|----------------------------------------|---------------------------------------------------------|
| Global config                          | `~/.config/lanes/config.toml`                           |
| Project config                         | `.lanes.toml` in the project (or a parent)              |
| Trusted projects                       | `~/.local/state/lanes/trust.json` (all projects)        |
| Trust check walk cache                 | `~/.local/state/lanes/trust-walk/` (one file per root)  |
| In-app views and filter edits          | `~/.local/state/lanes/<project>/views.json`             |
| Item history                           | `~/.local/state/lanes/<project>/history.jsonl`          |
| Item notes                             | `~/.local/state/lanes/<project>/notes.json`             |
| Snoozed items                          | `~/.local/state/lanes/<project>/snoozes.json`           |
| Session status files, tmux config      | `~/.local/state/lanes/<project>/`                       |
| Cached lists, last update check        | `~/.cache/lanes/<project>/`                             |

`<project>` is the git root's directory name plus a short hash of its path (for example `payments-3fa9c1`), so
two projects never share history, views or sessions. The `$XDG_*` variables are honoured.
