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
  - [Sorting](#sorting)
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
  - [github](#github)
  - [auto](#auto)
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
- **Notes** — your own note per item (`N`), shown in every view, usable in filters and templates.
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
  (`glab auth login`); for the GitHub provider: the [`gh`](https://cli.github.com) CLI, authenticated
  (`gh auth login`). `lanes` reuses their credentials — it never asks for a token.
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

1. `cd` into a git repository whose `origin` is on GitHub or GitLab.
2. Run `lanes`.

On first start `lanes` writes a default global config (see [Where things live](#where-things-live)) with three
views — `review`, `mine`, `drafts` — and one `auto` provider named `forge`, which picks GitHub for a `github.com`
`origin` and GitLab otherwise, and detects the repository from that remote.
Press `?` at any time for the built-in help (scroll with `j`/`k`, close with `q`).

Then make it yours: press `:edit` (or edit the file in your editor — changes apply immediately), define the
filters for the lists you care about and bind keys to the commands you run all the time.

## Using lanes

### Views

A *view* is a name, a provider and a filter. All views of the same provider share one fetch, so more views cost
nothing extra. The tab bar shows each view with its current item count. A second, coloured number counts the items
that need attention: those with a live session that is asking or waiting for input. It is yellow and hidden when
zero.

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

**Fields** are the item's own (`id`, `title`, `url`, `author`, `draft`), your [note](#preview-pane-and-item-history)
(`note`, empty when there is none, e.g. `note != ""`) plus whatever the provider adds. For GitLab (GitHub has the same fields, with `number` instead of `iid`):

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
| `number`            | number      | GitHub only, instead of `iid`: the PR number (used in the `id` as `#<number>`) |

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

### Sorting

A view lists items in provider order unless it sets `sort`: a comma-separated list of field names (the same
fields as in filters), each optionally prefixed with `-` for descending. Later keys break ties of earlier ones;
the sort is stable, so items that compare equal keep provider order. Numbers, bools (`false` first) and times
compare by value, everything else as text. Items missing a field sort last for that key, in either direction.

```toml
[[view]]
name = "mine"
provider = "gitlab"
filter = 'author == me'
sort = "-open_threads,title"
```

An unknown field name is not an error (fields come from the provider); an empty key (`"a,,b"`) is.

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
| `N`                 | edit the item's note in `$VISUAL` / `$EDITOR` (save it empty to delete it) |
| `n`                 | jump into the next session that is asking or waiting (any view) |
| `S`                 | sessions screen: every session, also of items no longer listed |
| `p`                 | toggle the preview pane                            |
| `F`                 | edit the current view's filter                     |
| `R`                 | refresh data now                                   |
| `ctrl-r`            | reload the config now                              |
| `?`                 | help (scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; close with `q`) |
| `q`                 | quit after confirming — sessions keep running      |

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
| `:note <text>`            | set the selected item's note to one line of text                      |
| `:note`                   | delete the selected item's note                                       |
| `:run <action>`           | run an action by name, even if it has no key                          |
| `:sessions`               | open the sessions screen (same as `S`)                                |
| `:prune`                  | kill every finished session; running ones are never touched           |
| `:prune failed`           | kill only finished sessions that exited non-zero                      |
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
| `✎`            | you left a [note](#preview-pane-and-item-history) on the item |
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

`p` toggles a pane under the list with the selected item's title, URL, author, your note, the provider's fields
(branch, labels, reviewers, CI, …), its sessions and its last run. Set `preview = true` under `[settings]` to start with
it open.

`i` shows the item's **history**: every action started on it, and how it ended (`finished`, `failed (exit n)`,
`killed`). History is stored per project and survives restarts.

`N` opens the item's **note** in `$VISUAL` / `$EDITOR` (as `:edit` does): keep your own context there, e.g.
"waiting on backend". Saving stores it, saving an empty file deletes it, quitting the editor with an error (vim's
`:cq`) leaves it unchanged; `:note <text>` sets a one-line note without the editor. A note belongs to the item, not
to a view: every view that lists the item shows `✎` on its row, and the preview shows the full text. Filters and
action templates see it as `note` (`note != ""`, `{{.note}}`); a provider field called `note` takes precedence
(the preview then labels yours `my note`).
Notes are stored per project and kept when the item leaves the list. A corrupt notes file is reported in the
footer and left alone until you save a note, which moves it to `notes.json.bak`.

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

As soon as you press the key, each item shows a `⋯ <action>` badge and the footer says
`starting <action> on <ids>…`. Until tmux has started the session, the item is **busy**: any action on it is
skipped (`!12 busy (review)`). In a multi-item run the other items still run and the result adds `, N busy`. The
lock ends once the session has started, so a long-running session does not block other actions on the item.

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

While the command runs, the item shows a `⋯ <action>` badge and is busy: any action on it is skipped, as for
session actions. After a successful `refresh = true` run the item stays busy, with its row muted, until every
provider has finished a fetch that started after the run (a fetch already under way gets a follow-up). A failed
run unlocks the item right away.

### Templates

`run`, `cwd` are [Go templates](https://pkg.go.dev/text/template). Available fields:

| Field                           | Value                                                           |
|---------------------------------|-----------------------------------------------------------------|
| `{{.id}}` `{{.title}}` `{{.url}}` `{{.author}}` | the item's own fields                           |
| `{{.iid}}` `{{.branch}}` `{{.ticket}}` `{{.me}}` … | provider fields (see [Filters](#filters))   |
| `{{.note}}`                     | your note on the item (empty when there is none)                |
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
- **Next needing you** — `n` jumps into the next live session that is asking or waiting, across all views:
  asking before waiting, oldest first. Pressing `n` again goes to the next one and wraps around; with none,
  the footer says `nothing needs you`.
- **Close** — `x` closes the sessions of the marked items, or of the item under the cursor. Running sessions are
  confirmed first (`Kill 2 running session(s)?`); finished ones are removed immediately.
- **Sessions screen** — `S` (or `:sessions`) lists every session on the `lanes` tmux server, newest first:
  item ID and title (or the item key when no provider returns the item any more), status badge and age.
  Sessions whose item is in no view — filtered out, or merged/closed — are marked `(not in any view)`. `enter`
  jumps in, `x` closes (running ones are confirmed first), `j`/`k`, `gg`/`G` move, `q` / `esc` / `S` go back.
- **Quit** — `q` asks first (`y`/`enter` quits, any other key cancels); `ctrl-c` and `:quit` leave at once. Sessions keep running and reappear when you start it again.
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
| Claude exited (`SessionEnd`)         | `▶` running       |

The settings are merged with your own Claude settings (via `--settings`), they don't replace them. Commands
without the flag simply show `▶` while running.

#### Usage in the footer

With `claude_dir` under `[settings]`, the left of the footer shows that Claude profile's usage, e.g.
`claude usage: 5h 23% · wk 30% · budget +4% · reset 15:00` (a notice, the mark count or an active search take its
place while shown). lanes reads it from `<claude_dir>/usage-line.json`, which a Claude
Code mod writes whenever Claude measures usage:

```json
{ "text": "5h 23% · wk 30% · budget +4% · reset 15:00", "at": 1761561600 }
```

lanes shows `text` as is, so it matches your Claude statusline. The file only changes while a Claude
session of that profile runs; after 10 minutes the text shows its age. Each profile has its own
dir (e.g. `~/.claude` and `~/.claude-personal` for `CLAUDE_CONFIG_DIR=~/.claude-personal claude`), and a
project's `.lanes.toml` can point at a different one than the global config. Unset, or no file: nothing is shown.

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

### github

```toml
[provider.github]
type = "github"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Lists the repository's **open pull requests**, drafts included (`draft = true`), via `gh api graphql` (so it uses
your existing `gh` login), in one query. Items get the same fields and badges as GitLab (see [Filters](#filters)),
with `number` instead of `iid`: `approved` is an approving review decision or any approving latest review,
`open_threads` counts unresolved review threads, `ci` is the head commit's check rollup (`success`, `failed`,
`running`), `conflicts` is a conflicting PR and `diverged` a branch behind its base. Only the 100 most recently
updated open PRs (and their first 100 review threads) are considered. Use it for GitHub Enterprise hosts, which
`auto` treats as GitLab.

### auto

```toml
[provider.forge]
type = "auto"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Picks `github` when the `origin` remote of the current directory is on `github.com` (https or ssh URL) and
`gitlab` for any other host, each time `lanes` starts or reloads its config — so one global config works in
GitHub and GitLab repositories alike. Without an `origin` remote it fails with an error asking you to set the
type explicitly. This is what the default config uses.

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
claude_dir    = "~/.claude"   # show this Claude profile's usage in the footer (see Claude Code integration)

[provider.<name>]
type    = "gitlab"      # or "github", "auto" (github for a github.com origin, else gitlab), "command"
project = "group/project"   # gitlab/github (owner/repo); default: detected from git origin
command = "..."             # command provider

[[view]]
name     = "review"
provider = "<name>"
filter   = '<expression>'   # empty = everything
sort     = "-field,field"    # "-" = descending; empty = provider order

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
| Item notes                             | `~/.local/state/lanes/<project>/notes.json`             |
| Session status files, tmux config      | `~/.local/state/lanes/<project>/`                       |
| Cached lists                           | `~/.cache/lanes/<project>/`                             |

`<project>` is the git root's directory name plus a short hash of its path (for example `payments-3fa9c1`), so
two projects never share history, views or sessions. The `$XDG_*` variables are honoured.

## How it works

- **Data flow.** On start `lanes` paints the cached list, then fetches each provider in the background and
  re-fetches every `poll_interval`; a failing provider is retried after `poll_interval`, doubling per consecutive failure up to 5 minutes (`R` / `:refresh` retry at once). New data is diffed into the list by item `id`, so the cursor and your marks
  stay put. Every view re-applies its filter on each refresh.
- **Filters** are compiled once per view and evaluated against each item's fields; a view with a broken filter
  shows its error instead of items.
- **Sessions** are tmux sessions on a dedicated tmux server named `lanes-<project>` (`tmux -L …`). The session
  name is `<action>~<item id>`, which is how `lanes` maps sessions back to items without any database — the
  list of sessions is simply asked from tmux once per second. This also means sessions survive quitting or
  crashing `lanes`.
- **Claude status** is written by hooks to a small file per session in the state directory; `lanes` reads those
  files together with the tmux pane state. The state is ignored once only a shell is left in the pane (Claude
  quit or crashed), so the session counts as running again.
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
- `lanes status` prints the current project's sessions that need input, as `? <asking> ● <waiting>` (zero parts
  left out, nothing at all when none do). It never prints errors, so it fits a status bar or a shell prompt:
  ```tmux
  # ~/.tmux.conf
  set -g status-right '#(cd #{pane_current_path} && lanes status) %H:%M'
  ```

## Troubleshooting

Run `lanes doctor` first. Without starting the TUI or touching any tmux server, it checks `tmux` (in `PATH`,
≥ 3.2), `glab` and `glab auth status` (only when a `gitlab` provider is configured) and likewise `gh` and
`gh auth status` for `github` (an `auto` provider counts as the forge its `origin` resolves to here; without an
`origin` it warns), that the config loads, that
every view filter compiles and every action template parses, and that the state and cache directories are
writable. It prints one `ok` / `warn` / `fail` line per check, naming the view or action, and exits 1 on any fail.

- **`glab: …` / `gh: …` error in the list** — run `glab auth status` / `gh auth status`; `lanes` needs a
  logged-in CLI. For a project that cannot be detected from the git remote, set `project` on the provider.
- **`auto: cannot detect the forge …`** — the directory has no `origin` remote; set `type = "github"` or
  `type = "gitlab"` on the provider (in a `.lanes.toml` for that project).
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
`provider` (GitLab, GitHub, command), `filter` (expressions), `session` (tmux, Claude hooks), `history`, `cache` and
`ui` (Bubble Tea). Design notes: [docs/design.md](docs/design.md).

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), [Bubbles](https://github.com/charmbracelet/bubbles),
[expr](https://github.com/expr-lang/expr) and [BurntSushi/toml](https://github.com/BurntSushi/toml).

## License

[MIT](LICENSE) © Christopher Hartmann
