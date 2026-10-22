# lanes

A fast, live, vim-style terminal UI for the review queue you actually work through every day.

`lanes` shows **filtered lists of work items** (merge requests today, anything you can print as JSON tomorrow),
keeps them up to date in the background, and lets you run **actions on them with a single key** — on one item or
on a whole selection. Actions run in **tmux sessions you can jump into at any time**, so interactive tools like
[Claude Code](https://claude.com/claude-code) keep working exactly as in a normal terminal, while `lanes` shows
you what each session is doing (running, working, asking a question, waiting for you, finished, failed).

```
 lanes   review 4  mine 3  drafts 1                                      ◐ 1  ? 1   updated 12s ago
 ─────────────────────────────────────────────────────────────────────────────────────────────────
 ▸  !1948   feat: prune entries missing from the provider   alex   ✓ ci  ⚑ 7  ✔ approved
 ●  !1957   fix: clear the change detector by id   sam    ✗ ci  ⚑ 2  ◐ review: working
    !1961   chore: bump dependencies                                 lee    ✓ ci        ? review: asking
 ─────────────────────────────────────────────────────────────────────────────────────────────────
 2 marked                       j/k move · h/l view · space mark · enter jump · x close · ? help
```

> The sketch above is illustrative; run `lanes` for the real thing.

## Contents

- [Why](#why)
- [Features](#features)
- [Requirements](#requirements)
- [Install](#install)
- [Quick start](#quick-start)
- [Using lanes](#using-lanes)
  - [Views](#views)
  - [Filters](#filters)
  - [Navigation and selection](#navigation-and-selection)
  - [Searching](#searching)
  - [The command line](#the-command-line)
  - [Row badges](#row-badges)
  - [Preview pane and item history](#preview-pane-and-item-history)
- [Actions](#actions)
  - [Session actions](#session-actions)
  - [Run actions](#run-actions)
  - [Templates](#templates)
  - [Running on many items](#running-on-many-items)
  - [Sessions: jump, close, status](#sessions-jump-close-status)
  - [Claude Code integration](#claude-code-integration)
  - [Notifications](#notifications)
- [Providers](#providers)
  - [gitlab](#gitlab)
  - [command](#command)
- [Configuration](#configuration)
  - [Global and project config](#global-and-project-config)
  - [Full reference](#full-reference)
  - [Theme](#theme)
  - [Where things live](#where-things-live)
- [How it works](#how-it-works)
- [tmux and lanes](#tmux-and-lanes)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [License](#license)

## Why

Working through a queue of merge requests is mostly *switching*: open the MR list, apply the same filter again,
copy an ID, open a terminal, start a command, remember which terminal belongs to which MR, come back later.
With AI agents producing more MRs and more review comments, that overhead grows faster than the work itself.

`lanes` replaces the switching with **views** (saved filters that stay current) and **one-key actions**. You
look at a list, press a key, and the work starts in the background in a session you can open whenever you want.

## Features

- **Views** — any number of named, filtered lists, switched with `h`/`l`, `tab` or `1`–`9`.
- **Powerful filters** — boolean expressions over item fields, e.g.
  `"waiting-for-review" in labels && !draft && !approved_by_me && !open_thread_by_me`.
- **Live** — lists refresh in the background (default every 30 s) without losing your cursor or selection; the
  first paint comes from a disk cache so startup is instant.
- **Vim navigation** — `h j k l`, `gg`/`G`, `ctrl-d`/`ctrl-u`, `/` search, `:` command line.
- **One-key actions** — bind any shell command to a key; templated with the item's fields.
- **Multi-select** — mark items with `space`, run an action on all of them, with a confirmation.
- **Interactive sessions** — actions run in detached tmux sessions on a private tmux server; jump in, interact,
  detach. Your shell, your environment.
- **Live session status** — running / working / asking / waiting / finished / failed, per item, in the list.
  For Claude Code sessions the state comes from Claude's own hooks.
- **Confirmations** — destructive or bulk operations (closing running sessions, multi-item runs) ask first.
- **Headless actions** — `type = "run"` actions (e.g. mark MR as ready) run in the background; the result is
  shown in the footer.
- **In-app filter editing** — change a view's filter, or create/delete views, without leaving the UI.
- **Item history** — per-item log of which actions ran, when and how they ended.
- **Preview pane** — details of the selected item, its sessions and last run.
- **Notifications** — desktop notification when a session needs you or ends.
- **Global + project config** — one config for you, optional `.lanes.toml` per project that overrides only what
  it defines. Hot-reloaded, like tmux's `source-file`, but automatic.
- **Not just MRs** — the `command` provider turns any JSON-printing command into a list.
- **Themeable** — adaptive light/dark palette with hex overrides.

## Requirements

- macOS or Linux
- [tmux](https://github.com/tmux/tmux) ≥ 3.2 (sessions run in tmux; `lanes` never touches your own tmux server)
- For the built-in GitLab provider: the [`glab`](https://gitlab.com/gitlab-org/cli) CLI, authenticated
  (`glab auth login`). `lanes` reuses its credentials — it never asks for a token.
- Go ≥ 1.23 only if you build from source

## Install

```sh
go install github.com/chriha/lanes@latest
```

This puts a `lanes` binary in `$(go env GOPATH)/bin` (usually `~/go/bin`; make sure it is on your `PATH`).

From a clone:

```sh
git clone https://github.com/chriha/lanes.git
cd lanes
make build           # ./bin/lanes
make install         # into $GOPATH/bin
lanes --version
```

## Quick start

1. `cd` into a git repository whose `origin` is on GitLab.
2. Run `lanes`.

On first start `lanes` writes a default global config (see [Where things live](#where-things-live)) with three
views — `review`, `mine`, `drafts` — and the GitLab provider, which detects the project from your `origin` remote.
Press `?` at any time for the built-in help (scroll with `j`/`k`, close with `q`).

Then make it yours: press `:edit` (or edit the file in your editor — changes apply immediately), define the
filters for the lists you care about and bind keys to the commands you run all the time.

## Using lanes

### Views

A *view* is a name, a provider and a filter. All views of the same provider share one fetch, so more views cost
nothing extra. The tab bar shows each view with its current item count.

| Key              | Action                         |
|------------------|--------------------------------|
| `l`, `tab`, `→`  | next view                      |
| `h`, `shift-tab`, `←` | previous view             |
| `1`–`9`          | jump to the n-th view          |
| `:view <name>`   | switch by name                 |

Views come from three places: the global config, the project config, and views you create inside the app with
`:newview` (stored outside your config, see [The command line](#the-command-line)). A view whose filter was
edited inside the app shows a `*` in the tab bar.

### Filters

A view's `filter` is a boolean expression evaluated for every item, using
[expr-lang](https://expr-lang.org/docs/language-definition) syntax. Items for which it is true are shown.
An empty filter shows everything.

```toml
[[view]]
name = "review"
provider = "gitlab"
filter = '"waiting-for-review" in labels && !draft && !approved_by_me && !open_thread_by_me && author != me'
```

Useful building blocks:

| Syntax                         | Meaning                                    |
|--------------------------------|--------------------------------------------|
| `a == b`, `a != b`             | equality                                   |
| `&&`, `\|\|`, `!`, `( )`         | and, or, not, grouping                     |
| `"x" in labels`                | list membership                            |
| `title contains "fix"`         | substring (also `startsWith`, `endsWith`)  |
| `title matches "^feat"`        | regular expression                         |
| `len(reviewers) >= 2`          | list length                                |
| `open_threads > 0`             | numbers                                    |

**Fields** are the item's own (`id`, `title`, `url`, `author`, `draft`) plus whatever the provider adds. For
GitLab:

| Field               | Type        | Meaning                                                                    |
|---------------------|-------------|----------------------------------------------------------------------------|
| `me`                | string      | your GitLab username                                                       |
| `author`            | string      | MR author                                                                  |
| `draft`             | bool        | MR is a draft                                                              |
| `labels`            | list        | label titles                                                               |
| `reviewers`         | list        | reviewer usernames                                                         |
| `assignees`         | list        | assignee usernames                                                         |
| `approved`          | bool        | the MR has approvals                                                       |
| `approved_by`       | list        | usernames that approved                                                    |
| `approved_by_me`    | bool        | you approved                                                               |
| `open_threads`      | number      | unresolved, resolvable discussions                                         |
| `open_thread_by_me` | bool        | you started a discussion that is still unresolved                          |
| `ci`                | string      | latest pipeline status, lowercase (`success`, `failed`, `running`, …)      |
| `conflicts`         | bool        | MR has merge conflicts                                                     |
| `diverged`          | bool        | source branch is behind its target branch (needs a rebase)                 |
| `branch`, `target`  | string      | source and target branch                                                   |
| `ticket`            | string      | first `ABC-123` style id found in the title or branch, upper-cased         |
| `iid`               | string      | the MR number (used in the `id` as `!<iid>`)                               |

Examples:

```text
author == me && !draft && "to-be-deployed" not in labels      # my MRs still in flight
ci == "failed" && author == me                                 # my MRs with a red pipeline
!draft && len(reviewers) == 0                                  # nobody assigned yet
ticket startsWith "ABC-" && open_threads > 0                   # open threads on one ticket prefix
```

If a filter has a syntax error it is rejected when you edit it (in-app) or shown in the view (config); a
runtime error (e.g. comparing a missing field) shows as a message in the list instead of silently hiding items.

**Editing filters in the app.** Press `F` on a view to open its filter in the command line, change it and press
`enter`. The change is saved *outside* your config (see [Where things live](#where-things-live)), so your
hand-written TOML is never rewritten, and shows as `*` in the tab bar. `:filter reset` goes back to the
config's filter.

### Navigation and selection

| Key                 | Action                                             |
|---------------------|----------------------------------------------------|
| `j` / `k`, `↓` / `↑`| move down / up                                     |
| `gg` / `G`          | first / last item                                  |
| `ctrl-d` / `ctrl-u` | half a page down / up                              |
| `space`             | mark / unmark the item, then move down             |
| `V`                 | mark all items of the view, or unmark them if all are marked |
| `esc`               | clear marks and the search                         |
| `enter`             | jump into the item's newest session (opens the URL if it has none) |
| `o`                 | open the item's URL in the browser                 |
| `x`                 | close the item's sessions (asks if any is still running) |
| `i`                 | show the item's history                            |
| `p`                 | toggle the preview pane                            |
| `F`                 | edit the current view's filter                     |
| `R`                 | refresh data now                                   |
| `ctrl-r`            | reload the config now                              |
| `?`                 | help (scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; close with `q`) |
| `q`                 | quit — sessions keep running                       |

Any other key you bind in `[keymap]` runs its [action](#actions). Built-in keys cannot be rebound; if you try,
the help marks the binding as shadowed and the reload message tells you.

### Searching

`/` opens an incremental search over the current view: it matches id, title, author and ticket, case
insensitively, as you type. `enter` keeps the search active (shown in the footer), `esc` clears it. The search
narrows the view; it does not change its filter.

### The command line

`:` opens a vim-style command line.

| Command                   | Does                                                                  |
|---------------------------|-----------------------------------------------------------------------|
| `:view <name>`            | switch to a view                                                      |
| `:filter`                 | show the current view's filter                                        |
| `:filter <expr>`          | set the current view's filter (validated; saved outside the config)   |
| `:filter reset`           | drop the in-app override and use the config's filter again            |
| `:newview <name> <expr>`  | create a view on the current view's provider                          |
| `:delview <name>`         | delete a view created with `:newview`                                 |
| `:run <action>`           | run an action by name, even if it has no key                          |
| `:refresh`                | refetch now                                                           |
| `:reload`                 | re-read the config now                                                |
| `:config`                 | print the global and project config paths                             |
| `:edit`                   | open the global config in `$VISUAL` / `$EDITOR`                       |
| `:edit project`           | open the project config, creating `.lanes.toml` if missing            |
| `:quit`                   | quit                                                                  |

### Row badges

Each row ends with small status badges: the item's own (from the provider) followed by up to three of its
sessions.

| Badge          | Meaning                                                       |
|----------------|---------------------------------------------------------------|
| `draft`        | MR is a draft                                                 |
| `✓ ci` `✗ ci` `◌ ci` | pipeline passed / failed / running or pending           |
| `↓ behind`     | source branch is behind its target branch                     |
| `⚠ conflict`   | merge conflicts                                               |
| `⚑ 3`          | three unresolved discussions                                  |
| `✔ approved`   | has approvals                                                 |
| `▶ review`     | the `review` action is running in a session                   |
| `◐ review: working`  | a Claude session is working                             |
| `? review: asking`   | a Claude session asks a question / needs permission     |
| `● review: waiting`  | a Claude session finished its turn and waits for you    |
| `✓ review`     | the session's command exited with status 0                    |
| `✗ review (3)` | the session's command exited with status 3                    |

The top right of the screen summarizes live sessions (`◐ 2  ? 1  ● 1`) and when the list was last refreshed.

### Preview pane and item history

`p` toggles a pane under the list with the selected item's title, URL, author, the provider's fields (branch,
labels, reviewers, CI, …), its sessions and its last run. Set `preview = true` under `[settings]` to start with
it open.

`i` shows the item's **history**: every action started on it, and how it ended (`finished`, `failed (exit n)`,
`killed`). History is stored per project and survives restarts.

## Actions

An *action* is a named, templated shell command. Bind it to a key in `[keymap]`:

```toml
[keymap]
r = "review"

[action.review]
run = 'claude "/review {{.url}}"'
```

Pressing `r` runs `review` on the marked items, or on the item under the cursor if nothing is marked.

Every action accepts:

| Key       | Default   | Meaning                                                                              |
|-----------|-----------|--------------------------------------------------------------------------------------|
| `run`     | required  | shell command, templated                                                             |
| `type`    | `session` | `session` (tmux session) or `run` (headless)                                         |
| `cwd`     | current dir | working directory, templated                                                       |
| `confirm` | `none`    | `single` asks before running even on one item (multi-item runs always ask)          |
| `vars`    | none      | your own values, available as `{{.vars.name}}`                                       |
| `refresh` | `false`   | `run` type only: refetch the lists when the command finishes                         |

### Session actions

The command runs in a **detached tmux session** using **your `$SHELL`** (`$SHELL -ic '<command>'`, so your
aliases and PATH apply). The session appears as a badge on the item. Press `enter` on the item to jump in; you
get the full interactive terminal. If the command is interactive (for example `claude`), you can answer its
questions there.

If an action is already running for an item it is not started again (you are told so). If its previous session
has ended, it is replaced by a fresh one.

When the command exits the session stays around (tmux `remain-on-exit`), showing `✓` or `✗ (exit code)`, so you
can still read the output. Close it with `x`.

### Run actions

`type = "run"` actions run in the background with no session — for quick operations that need no interaction:

```toml
[action.ready]
type = "run"
refresh = true
run = 'glab mr update {{.iid}} --ready --label waiting-for-review --reviewer {{.vars.reviewers}} --assignee {{.me}}'
vars = { reviewers = "alice,bob" }
```

Marked items are processed in parallel. The footer shows the result: `✓ ready done on 2 item(s)` or
`✗ ready failed on 1/2 — !12: <last line of the command's output>`. With `refresh = true` the lists refetch
afterwards, so the MR moves out of your `drafts` view.

### Templates

`run`, `cwd` are [Go templates](https://pkg.go.dev/text/template). Available fields:

| Field                           | Value                                                           |
|---------------------------------|-----------------------------------------------------------------|
| `{{.id}}` `{{.title}}` `{{.url}}` `{{.author}}` | the item's own fields                           |
| `{{.iid}}` `{{.branch}}` `{{.ticket}}` `{{.me}}` … | provider fields (see [Filters](#filters))   |
| `{{.vars.name}}`                | the action's `vars`                                             |
| `{{.settings}}`                 | path to a Claude settings file that reports session status      |
| `{{.session}}` `{{.action}}`    | the session / action name                                       |

`{{.title | q}}` shell-quotes a value — use it for anything that may contain spaces or quotes. A missing field or
variable is an error shown in the footer; nothing runs.

### Running on many items

Mark items with `space` (or `V` for all), then press the action key. Because this is easy to do by accident,
multi-item runs always ask first:

```text
Run "review" on 3 items (!1948, !1957, !1961)?  y/enter yes · n/esc no
```

Each item gets its own session; items that already have the action running are skipped.

### Sessions: jump, close, status

- **Jump** — `enter` on an item attaches to its newest session. Inside tmux this opens a popup over `lanes`;
  outside tmux, `lanes` suspends and attaches. **Detach with `ctrl-b d`** to come back (the session keeps
  running). A hint is shown in the session's status bar.
- **Close** — `x` closes the sessions of the marked items, or of the item under the cursor. Running sessions are
  confirmed first (`Kill 2 running session(s)?`); finished ones are removed immediately.
- **Quit** — `q` leaves `lanes`; sessions keep running and reappear when you start it again.
- **Status** — shown as row badges and in the header summary, refreshed every second.

### Claude Code integration

Add `--settings {{.settings}}` to a `claude` command and `lanes` knows what the session is doing:

```toml
[action.review]
run = 'claude --settings {{.settings}} "/review {{.url}}"'
```

`{{.settings}}` points to a small settings file `lanes` generates; it registers [Claude Code
hooks](https://docs.claude.com/en/docs/claude-code/hooks) that call `lanes hook <event>`, which records the
state for the session:

| Claude event                         | State shown       |
|--------------------------------------|-------------------|
| prompt submitted / tool about to run | `◐ working`       |
| notification (permission, question)  | `? asking`        |
| turn finished                        | `● waiting`       |
| idle reminder                        | `● waiting`       |

The settings are merged with your own Claude settings (via `--settings`), they don't replace them. Commands
without the flag simply show `▶` while running.

### Notifications

`lanes` raises a desktop notification (macOS: `osascript`, Linux: `notify-send` if installed) when a session
starts asking, starts waiting, finishes or fails. Disable with `no_notify = true` under `[settings]`.

## Providers

A provider produces the items of a view. Define one in `[provider.<name>]` and reference it from a view.

### gitlab

```toml
[provider.gitlab]
type = "gitlab"
# project = "group/subgroup/project"   # default: detected from the git `origin` of the current directory
```

Lists the project's **open merge requests** via `glab api graphql` (so it uses your existing `glab` login). The
fields are listed under [Filters](#filters). Fetching is **two-pass**: a fast query (~2 s) paints the list
immediately; a second pass (~9 s on large projects) adds who started which discussion
(`open_thread_by_me`), because GitLab's discussion query is slow. Until it finishes, `open_thread_by_me` keeps
its value from the previous refresh, so the list does not flicker. Only the 100 most recently updated open MRs
(and their first 100 discussions) are considered.

### command

Turn anything into a list. The command runs through `$SHELL -c` and must print a JSON array:

```toml
[provider.todos]
type = "command"
command = "my-todo-exporter --json"

[[view]]
name = "todos"
provider = "todos"
filter = 'priority == "high"'
```

```json
[
  {
    "id": "T-12",
    "title": "Write the README",
    "url": "https://example.com/T-12",
    "author": "chris",
    "draft": false,
    "badges": [{ "text": "due today", "kind": "warn" }],
    "fields": { "priority": "high", "labels": ["docs"] }
  }
]
```

`id` must be unique within the provider (it identifies the item across refreshes and in session names). `kind`
of a badge is one of `ok`, `warn`, `bad`, `info`, `dim`. Everything under `fields` becomes available to filters
and templates.

## Configuration

### Global and project config

- **Global** config: `~/.config/lanes/config.toml` (honours `$XDG_CONFIG_HOME`). Created with defaults on first
  start. Put *your* views, actions and keys here.
- **Project** config: `.lanes.toml` in the project directory — `lanes` uses the nearest one in the working
  directory or any parent (the search stops at your home directory). Use it for project-specific settings such
  as the GitLab project, an extra view, or a project-specific action. Create it with `:edit project`.

The project config **overlays** the global one — it does not replace it. You only write what differs:

| Section            | Merged                                                                 |
|--------------------|------------------------------------------------------------------------|
| `[settings]`       | per field — set only `preview = false` and `poll_interval` is kept     |
| `[theme]`          | per field                                                              |
| `[keymap]`         | per key                                                                |
| `[provider.<n>]`   | by name — a provider defined in the project replaces the global one of that name as a whole |
| `[action.<n>]`     | by name, as a whole                                                    |
| `[[view]]`         | by `name` — same name replaces the global view, new names are appended |

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
footer shows the error.

### Full reference

```toml
[settings]
poll_interval = "30s"   # how often lists refresh (minimum 5s)
preview       = false   # open the preview pane at startup
no_notify     = false   # disable desktop notifications

[provider.<name>]
type    = "gitlab"      # or "command"
project = "group/project"   # gitlab; default: detected from git origin
command = "..."             # command provider

[[view]]
name     = "review"
provider = "<name>"
filter   = '<expression>'   # empty = everything

[keymap]
<key> = "<action name>"     # single character, e.g. r

[action.<name>]
type    = "session"         # or "run"
run     = '<templated shell command>'
cwd     = '<templated dir>'
confirm = "none"            # or "single"
vars    = { key = "value" }
refresh = false

[theme]
accent = "#A78BFA"          # also: ok, warn, bad, info
```

### Theme

`lanes` ships an adaptive light/dark palette. Override any of `accent`, `ok`, `warn`, `bad`, `info` with a hex
color under `[theme]`. Changes apply live.

### Where things live

| What                                   | Where (default)                                         |
|----------------------------------------|---------------------------------------------------------|
| Global config                          | `~/.config/lanes/config.toml`                           |
| Project config                         | `.lanes.toml` in the project (or a parent)              |
| In-app views and filter edits          | `~/.local/state/lanes/<project>/views.json`             |
| Item history                           | `~/.local/state/lanes/<project>/history.jsonl`          |
| Session status files, tmux config      | `~/.local/state/lanes/<project>/`                       |
| Cached lists                           | `~/.cache/lanes/<project>/`                             |

`<project>` is the git root's directory name plus a short hash of its path (for example `payments-3fa9c1`), so
two projects never share history, views or sessions. The `$XDG_*` variables are honoured.

## How it works

- **Data flow.** On start `lanes` paints the cached list, then fetches each provider in the background and
  re-fetches every `poll_interval`. New data is diffed into the list by item `id`, so the cursor and your marks
  stay put. Every view re-applies its filter on each refresh.
- **Filters** are compiled once per view and evaluated against each item's fields; a view with a broken filter
  shows its error instead of items.
- **Sessions** are tmux sessions on a dedicated tmux server named `lanes-<project>` (`tmux -L …`). The session
  name is `<action>~<item id>`, which is how `lanes` maps sessions back to items without any database — the
  list of sessions is simply asked from tmux once per second. This also means sessions survive quitting or
  crashing `lanes`.
- **Claude status** is written by hooks to a small file per session in the state directory; `lanes` reads those
  files together with the tmux pane state.
- **Config reload** compares the modification time of the global and project files every second.

## tmux and lanes

`lanes` is designed not to get in your way if you use tmux:

- It uses its **own tmux server** (a separate socket), never your default one. Your sessions, windows, key
  bindings and `~/.tmux.conf` are not touched; the private server uses a tiny config of its own.
- Inside tmux, jumping into a session opens a **popup** (`display-popup`) rather than switching your client.
  `TMUX` is unset for the nested attach, so tmux does not complain about nesting.
- You can always inspect sessions yourself:
  ```sh
  tmux -L lanes-<project> ls
  tmux -L lanes-<project> attach -t '=review~_1948'
  ```
  (In session names, characters outside `A-Za-z0-9_-` in the item id are replaced with `_`; `!1948` becomes
  `_1948`.)
- Detach with the private server's prefix, `ctrl-b d`.

## Troubleshooting

- **`glab: …` error in the list** — run `glab auth status`; `lanes` needs a logged-in `glab`. For a project that
  cannot be detected from the git remote, set `project` on the provider.
- **`tmux not found in PATH`** — install tmux (≥ 3.2).
- **A key does nothing** — it may be a built-in key; open `?` and look at the actions list, shadowed bindings
  are flagged.
- **An action reports `map has no entry for key "x"`** — the template uses `{{.vars.x}}` (or another field)
  that is not defined. Add it to the action's `vars`.
- **The list is empty** — the filter excludes everything, or its fields don't exist for that provider. Press
  `F` to look at the filter, or `:filter reset`.
- **Start fresh** — delete `~/.local/state/lanes` and `~/.cache/lanes` (and the config if you want the
  defaults back). Running sessions: `tmux -L lanes-<project> kill-server`.

## Development

```sh
make build        # ./bin/lanes
make test         # go vet + go test
```

The code is organised by concern under `internal/`: `config` (TOML, project overlay, in-app overrides),
`provider` (GitLab, command), `filter` (expressions), `session` (tmux, Claude hooks), `history`, `cache` and
`ui` (Bubble Tea). Design notes: [docs/design.md](docs/design.md).

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), [Bubbles](https://github.com/charmbracelet/bubbles),
[expr](https://github.com/expr-lang/expr) and [BurntSushi/toml](https://github.com/BurntSushi/toml).

## License

[MIT](LICENSE) © Christopher Hartmann
