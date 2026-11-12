# lanes

A fast, live, vim-style terminal UI for the review queue you actually work through every day.

`lanes` shows **filtered lists of work items** (merge requests today, anything you can print as JSON tomorrow),
keeps them up to date in the background, and lets you run **actions on them with a single key** — on one item or
on a whole selection. Actions run in **tmux sessions you can jump into at any time**, so interactive tools like
[Claude Code](https://claude.com/claude-code) keep working exactly as in a normal terminal, while `lanes` shows
you what each session is doing (running, working, asking a question, waiting for you, finished, failed).

```
 lanes   review 4  mine 3  drafts 1                        ◐ 1  ? 1   gitlab updated 12s ago · 1.4s
 ─────────────────────────────────────────────────────────────────────────────────────────────────
   !1948   feat: prune entries missing from the provider   alex   ✓ ci  ⚑ 7  ✔ approved
 ▌ !1957   fix: clear the change detector by id   sam    ✗ ci  ⚑ 2  ◐ review: working
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
  - [Updates](#updates)
- [Quick start](#quick-start)
- [Using lanes](#using-lanes)
  - [Views](#views)
  - [Filters](#filters)
  - [Sorting](#sorting)
  - [Groups](#groups)
  - [Navigation and selection](#navigation-and-selection)
  - [Searching](#searching)
  - [The command line](#the-command-line)
  - [Row badges](#row-badges)
  - [Preview pane and item history](#preview-pane-and-item-history)
- [Actions](#actions)
  - [Session actions](#session-actions)
  - [Shell sessions](#shell-sessions)
  - [Run actions](#run-actions)
  - [Templates](#templates)
  - [Running on many items](#running-on-many-items)
  - [Restricting actions (`when`)](#restricting-actions-when)
  - [Chaining actions (`on_success`, `on_failure`)](#chaining-actions-on_success-on_failure)
  - [Running on arrival (`on_enter`)](#running-on-arrival-on_enter)
  - [Sessions: jump, close, status](#sessions-jump-close-status)
  - [Claude Code integration](#claude-code-integration)
  - [Agent view](#agent-view)
  - [Notifications](#notifications)
- [Providers](#providers)
  - [gitlab](#gitlab)
  - [github](#github)
  - [github-issues](#github-issues)
  - [linear](#linear)
  - [auto](#auto)
  - [command](#command)
- [Configuration](#configuration)
  - [Global and project config](#global-and-project-config)
  - [Trusting a project](#trusting-a-project)
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
- **Auto-run on arrival** — a view can start an action by itself on each item that newly enters it (`on_enter`).
- **Interactive sessions** — actions run in detached tmux sessions on a private tmux server; jump in, interact,
  detach. Your shell, your environment.
- **Live session status** — running / working / asking / waiting / finished / failed, per item, in the list.
  For Claude Code sessions the state comes from Claude's own hooks.
- **Confirmations** — destructive or bulk operations (closing sessions, multi-item runs) ask first.
- **Headless actions** — `type = "run"` actions (e.g. mark MR as ready) run in the background; the result is
  shown in the footer.
- **In-app filter editing** — change a view's filter, or create/delete views, without leaving the UI.
- **Item history** — per-item log of which actions ran, when and how they ended.
- **Notes** — your own note per item (`N`), shown in every view, usable in filters and templates.
- **Snooze** — put an item aside (`z`) until a time or until it changes; it leaves every view until then.
- **Agent view** — `A` shows a Claude session as a live, scrollable neon dashboard: which agents run, who spawned
  whom, their model, effort, context use and current tool.
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
  (`glab auth login`); for the GitHub providers: the [`gh`](https://cli.github.com) CLI, authenticated
  (`gh auth login`). `lanes` reuses their credentials — it never asks for a token. For the Linear provider: a
  Linear personal API key in an environment variable.
- Go ≥ 1.23 only if you build from source

## Install

With Homebrew (macOS and Linux; also installs `tmux`):

```sh
brew install chriha/lanes/lanes
```

With Go:

```sh
go install github.com/chriha/lanes@latest
```

This puts a `lanes` binary in `$(go env GOPATH)/bin` (usually `~/go/bin`; make sure it is on your `PATH`).

From a release: download the archive for your platform (darwin or linux, amd64 or arm64) from the
[releases page](https://github.com/chriha/lanes/releases), check it against `checksums.txt`, and put the `lanes`
binary on your `PATH`:

```sh
tar -xzf lanes_<version>_<os>_<arch>.tar.gz
sha256sum -c --ignore-missing checksums.txt   # on macOS: shasum -a 256 -c --ignore-missing checksums.txt
install lanes ~/.local/bin/
lanes --version                                # prints the release tag
```

From a clone:

```sh
git clone https://github.com/chriha/lanes.git
cd lanes
make build           # ./bin/lanes
make install         # into $GOPATH/bin
lanes --version
```

### Updates

At most once a day, `lanes` asks GitHub in the background whether a newer release exists. If so, the footer shows
`lanes vX.Y.Z available` with the command that fits how you installed it (`brew upgrade lanes`, `go install
github.com/chriha/lanes@latest`, or the releases page). `lanes version` prints the same hint from the last check,
without asking the network; it needs a trusted project, since the result is cached per project. A failed check stays
silent and is retried at the next start. `lanes` never replaces its own binary. Turn the check off with
`no_update_check = true` under `[settings]`. Builds without a release version (`dev`) never check.

## Quick start

1. `cd` into a git repository whose `origin` is on GitHub or GitLab.
2. Run `lanes`. The first time in a project it asks whether you trust it; type `yes` (see
   [Trusting a project](#trusting-a-project)).

On first start `lanes` writes a default global config (see [Where things live](#where-things-live)) with three
views — `review`, `mine`, `drafts` — and one `auto` provider named `forge`, which picks GitHub for a `github.com`
`origin` and GitLab otherwise, and detects the repository from that remote.
Press `?` at any time for the built-in help (scroll with `j`/`k`, close with `q`).

Then make it yours: press `:edit` (or edit the file in your editor — changes apply immediately), define the
filters for the lists you care about and bind keys to the commands you run all the time.

Besides the TUI, `lanes` has these commands:

| Command          | Does                                                                                      |
|------------------|-------------------------------------------------------------------------------------------|
| `lanes init`     | write a commented `.lanes.toml` to the current directory; asks before overwriting one, with a backup (see [Global and project config](#global-and-project-config)); asks for trust first, like the TUI |
| `lanes doctor`   | check tmux, the forge CLIs, the config and the state dirs (see [Troubleshooting](#troubleshooting)); asks for trust first, like the TUI |
| `lanes status`   | print the current project's sessions that need input (see [tmux and lanes](#tmux-and-lanes)); prints nothing in an untrusted project and never asks |
| `lanes --version`| print the version; never asks for trust                                                   |
| `lanes --untrust`| forget the trust of the current project, so the next start asks again (see [Trusting a project](#trusting-a-project)); never asks, works without a terminal |

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

To group tabs, set `divider = true` on a view: a dim `¦` is drawn before its tab. A divider is not a view, so
`h`/`l`, `1`–`9` and `:view` skip it and the numbering is unchanged.

```toml
[[view]]
name = "mine"
provider = "gitlab"
divider = true
```

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
| `hasNone(labels, "needs-info", "triage", "planning")` | none of these are in the list (also `hasAny`, `hasAll`) |
| `title contains "fix"`         | substring (also `startsWith`, `endsWith`)  |
| `title matches "^feat"`        | regular expression                         |
| `len(reviewers) >= 2`          | list length                                |
| `open_threads > 0`             | numbers                                    |

**Fields** are the item's own (`id`, `title`, `url`, `author`, `draft`), your [note](#preview-pane-and-item-history)
(`note`, empty when there is none, e.g. `note != ""`), whether you [snoozed](#snoozing) it (`snoozed`), whether it changed since you last saw it
(`unread`, see [Row badges](#row-badges)) plus whatever the provider adds. The `fields` section at the
bottom of the help (`?`) lists every provider type's keys, and for each configured `command` provider the keys of its
last fetched items. For GitLab (GitHub has the same fields, with `number` instead of `iid`):

| Field               | Type        | Meaning                                                                    |
|---------------------|-------------|----------------------------------------------------------------------------|
| `kind`              | string      | `pr` for every MR/PR                                                       |
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

`hasAny`, `hasNone` and `hasAll` take a list field and any number of values; a missing field counts as an empty
list. `["a", "b"] in labels` is rejected: use `hasAny(labels, "a", "b")` or `hasNone(...)`.

If a filter has a syntax error it is rejected when you edit it (in-app) or shown in the view (config); a
runtime error (e.g. comparing a missing field) shows as a message in the list instead of silently hiding items.

**Editing filters in the app.** Press `F` on a view to open its filter in the command line, change it and press
`enter`. The change is saved *outside* your config (see [Where things live](#where-things-live)), so your
hand-written TOML is never rewritten, and shows as `*` in the tab bar. `:filter reset` goes back to the
config's filter. The one exception is `:filter save`: it writes the edit into the config file that defines the
view (`.lanes.toml` when the project defines it, else the global config), changing only that view's `filter`
line, and drops the `*`. A filter written across several lines can't be saved this way; edit the file by hand.

### Sorting

A view lists items in provider order unless it sets `sort`: a comma-separated list of field names (the same
fields as in filters), each optionally prefixed with `-` for descending. Later keys break ties of earlier ones;
the sort is stable, so items that compare equal keep provider order. Numbers, bools (`false` first) and times
compare by value, everything else as text. Items missing a field sort last for that key, in either direction.
A `priority` of 0 (none) counts as missing too, so `sort = "priority,title"` lists urgent, high, medium, low,
then unprioritised items by title.

```toml
[[view]]
name = "mine"
provider = "gitlab"
filter = 'author == me'
sort = "-open_threads,title"
```

An unknown field name is not an error (fields come from the provider); an empty key (`"a,,b"`) is.

### Groups

A view can split its items into named groups, each with a filter (the same expressions as view filters). An item
goes into the first group it matches; items matching no group are listed below the groups without a header.

```toml
[[view]]
name = "queue"
provider = "gitlab"
groups = [
  { name = "mine",   filter = 'author == me' },
  { name = "urgent", filter = '"urgent" in labels' },
]
```

Each non-empty group shows under a `── <name> (<count>) ──` header, in config order; empty groups are hidden. The
view's `sort` applies inside each group and in the ungrouped tail. The cursor skips headers, so navigation, marks,
actions, search and the tab count work as in a flat view. A group without a name, or with a filter that does not
compile, is a config error. Groups are set in the config only (global or project), not with `:filter`/`:newview`.

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
| `y`                 | copy the item's ID to the clipboard (all marked IDs, space-separated, if any are marked) |
| `x`                 | close the item's sessions (asks first)             |
| `X`                 | interrupt the item's running sessions and keep their output (asks first; again: `SIGKILL`) |
| `t`                 | open a shell for the item and jump into it (see [Shell sessions](#shell-sessions)) |
| `gr`                | resume the item's newest finished Claude session (see [Resuming a Claude session](#resuming-a-claude-session)) |
| `i`                 | show the item's history                            |
| `d`                 | item details: every field untruncated, description, pipeline, threads and comments (scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; `J`/`K` move between threads, `n`/`N` jump to the next/previous open thread, `enter`/`space` fold the one under the cursor, `za`/`zA` fold/unfold all, `o` opens it in the browser; close with `q`/`esc`) |
| `N`                 | edit the item's note in `$VISUAL` / `$EDITOR` (save it empty to delete it) |
| `m`                 | mark the item unread (all marked items, if any are marked); when all of them are unread: mark them read (see [Row badges](#row-badges)) |
| `z`                 | [snooze](#snoozing) the item (all marked items, if any are marked); on a snoozed item: unsnooze it |
| `n`                 | jump into the next session that is asking or waiting (any view) |
| `S`                 | sessions screen: every session, also of items no longer listed |
| `A`                 | [agent view](#agent-view) of the item's newest Claude session |
| `p`                 | toggle the preview pane                            |
| `F`                 | edit the current view's filter                     |
| `R`                 | refresh data now                                   |
| `ctrl-r`            | reload the config now                              |
| `?`                 | help: keys, actions, commands, config paths and the fields per provider (scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; close with `q`) |
| `q`                 | quit after confirming — sessions keep running      |

Any other key you bind in `[keymap]` runs its [action](#actions). Built-in keys cannot be rebound; if you try,
the help marks the binding as shadowed and the reload message tells you. `X` became a built-in key with
interrupt; an older `[keymap]` binding of `X` is now shadowed.

### Searching

Ticket rows end with a priority strip after the assignees: `▰▰▰` urgent (red), `▰▰▱` high (orange), `▰▱▱` medium
(yellow), `▱▱▱` low (dim); none for priority `0`.

`/` opens an incremental search over the current view: it matches id, title, author, ticket and (for tickets) assignees, case
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
| `:filter save`            | write the in-app edit into the config file that defines the view      |
| `:newview <name> <expr>`  | create a view on the current view's provider                          |
| `:delview <name>`         | delete a view created with `:newview`                                 |
| `:note <text>`            | set the selected item's note to one line of text                      |
| `:note`                   | delete the selected item's note                                       |
| `:snooze [when]`          | snooze the selected / marked items until `when` (empty: until they change) |
| `:snoozed`                | show / hide snoozed items                                             |
| `:run <action>`           | run an action by name, even if it has no key                          |
| `:auto [on\|off]`         | pause / resume the views' [`on_enter`](#running-on-arrival-on_enter) actions; bare: show their state |
| `:resume`                 | resume the selected item's newest finished Claude session (same as `gr`) |
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
sessions. A blue `✎` after the name (the author, or the assignees on tickets), or after the title when there is no name, is not a badge: it marks an
item you left a [note](#preview-pane-and-item-history) on.

A **bold title** marks an unread item: it is new, or its title, draft state, badges or provider fields changed since
you last saw it. The cursor resting on it for a second marks it read; `m` toggles it by hand. Filters see `unread`
(`true` / `false`). On the first run every listed item counts as read. Seen items are stored per project
(`seen.json`, a corrupt file moves to `seen.json.bak` like notes).

| Badge          | Meaning                                                       |
|----------------|---------------------------------------------------------------|
| `draft`        | MR is a draft                                                 |
| `✓ ci` `✗ ci` `◌ ci` | pipeline passed / failed / running or pending           |
| `↓ behind`     | source branch is behind its target branch                     |
| `⚠ conflict`   | merge conflicts                                               |
| `⚑ 3`          | three unresolved discussions                                  |
| `✔ approved`   | has approvals                                                 |
| `▶ review`     | the `review` action is running in a session                   |
| `queued review` | an [`on_enter`](#running-on-arrival-on_enter) start of `review` waits for a free session |
| `◐ review: working`  | a Claude session is working                             |
| `? review: asking`   | a Claude session asks a question / needs permission     |
| `● review: waiting`  | a Claude session finished its turn and waits for you    |
| `✓ review`     | the session's command exited with status 0                    |
| `✗ review (3)` | the session's command exited with status 3                    |

The top right of the screen summarizes live sessions (`◐ 2  ? 1  ● 1`) and when the list was last refreshed.

### Preview pane and item history

`p` toggles a pane under the list with the selected item's title, URL, author, your note, the provider's fields
(the identifier — `iid`, `number` or `key` — then branch, labels, reviewers, CI, …), its sessions and its last run.
Tickets also get `blocked_by`, `blocking` and `related` lines after labels and assignees, listing the open ids
followed by the closed ones dimmed with a `✓` (e.g. `blocked_by  #7, #9 ✓`); empty relations are skipped. Set `preview = true` under `[settings]` to start with
it open.

The **last run** line shows the item's newest history event as `<action> · <duration> · <tokens> tok · <outcome>`,
e.g. `work · 14m · 2.1M tok · ✓` or `fix · 42s · ✗ 1`. The duration runs from the session's creation (or the run
command's start) to its end: `42s`, `14m`, `1h05m`. Tokens are those of a Claude session, read from its transcript
when it ends, and count input, cache writes and output, not cache reads (they mostly repeat earlier context):
`850`, `12k`, `2.1M`. The outcome is `✓` (finished), `✗ <exit>` (failed), or `killed`, `interrupted`, `started`.
Parts the event lacks — a started action has no duration, a run or a non-Claude session no tokens, older history
lines neither — are left out. The history stores every token count separately.

`d` opens the item's **details** on a full screen, in stacked sections: a header with the id, title, URL, author
and the pipeline (`ci`) and draft state, then **Fields** with your note and every remaining field untruncated, the
CI jobs (`ci_jobs`) one per line, so a failed job stays readable when the preview has to cut it short. The values
line up in one column after the longest key, and a long value wraps under its value column. For GitLab MRs, GitHub
PRs and issues, Linear and a `command` provider with a [`detail`](#command) command, pressing `d` also
fetches in the background, once per item version, the **Description**, the **Pipeline** (every job grouped by stage),
the review **Threads** and the **Comments** (newest last); the fields show at once and the sections fill in.
Scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; `q` or `esc` returns to the list. Open threads start expanded,
resolved ones collapsed to one line (`✓ resolved · file:line · author · n replies`): `J`/`K` move the thread cursor
(`›`), `n`/`N` jump to the next/previous open thread and unfold it (they stop at the last/first one),
`enter`/`space` fold or unfold the thread under it, `za` folds all, `zA` unfolds all. `o` opens the thread under
the cursor in the browser (GitLab and GitHub threads); a thread without a URL (`command` provider), or no thread,
opens the item's URL. Folding is kept while lanes runs, also when the details are fetched again.

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

### Snoozing

`z` puts the selected item (or all marked items) aside: it opens a prompt (`:snooze`) that takes a duration
(`30m`, `2h`, `3d`), a weekday (`mon`, `friday`: its next occurrence, at midnight) or a date (`2026-11-03`).
Empty input snoozes **until the item changes**: its title, draft state, badges or provider fields differ on a later
fetch (your note does not count). Invalid input or a time in the past shows an error and snoozes nothing.

A snoozed item drops out of every view and its tab count; the footer shows how many items of the current view are
snoozed. `:snoozed` shows them again, dimmed and marked `z`, and `z` on one of them unsnoozes it. Filters and
action templates see `snoozed` (`true` / `false`), which is useful with `:snoozed` on. Snoozes are stored per
project (`snoozes.json`, a corrupt file moves to `snoozes.json.bak` like notes) and removed once they wake.

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
| `when`    | none      | [filter expression](#filters); the action runs only on items it matches (see [below](#restricting-actions-when)) |
| `jump`    | `false`   | `session` type only: on a single item, attach to the session once it has started (see [below](#session-actions)) |
| `on_success` | none   | action to start on the same item when this one exits 0 (see [below](#chaining-actions-on_success-on_failure)) |
| `on_failure` | none   | action to start on the same item when this one exits non-zero                        |

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

With `jump = true`, running the action on a single item also takes you into its session, exactly like `enter`:
lanes attaches once tmux has started it, or attaches to the running one instead of saying it is already running
(a finished one is restarted first). On several marked items it starts them all and jumps into none. `jump` on a
`type = "run"` action is a config error.

When the command exits the session stays around (tmux `remain-on-exit`), showing `✓` or `✗ (exit code)`, so you
can still read the output. Stop a running command with `X` (the session stays with its output and shows
`✗ (143)`), close it with `x` (it asks first).

### Shell sessions

`t` opens a plain shell for the item under the cursor: your `$SHELL`, interactive, in the directory you started
`lanes` in, as the session `shell~<item id>`. It works with no config and jumps right in. Marks are ignored. If the
item already has a live shell, `t` jumps into it; if the shell has ended (`exit`), it shows as `✓ shell` (or
`✗ shell (n)`) and the next `t` restarts it. Close it with `x` and remove finished ones with `:prune`, like any
other session. `shell` is a reserved action name: a config action called `shell` fails to load.

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

`run`, `resume`, `cwd` are [Go templates](https://pkg.go.dev/text/template). Available fields:

| Field                           | Value                                                           |
|---------------------------------|-----------------------------------------------------------------|
| `{{.id}}` `{{.title}}` `{{.url}}` `{{.author}}` | the item's own fields                           |
| `{{.iid}}` `{{.branch}}` `{{.ticket}}` `{{.me}}` … | provider fields (see [Filters](#filters))   |
| `{{.note}}`                     | your note on the item (empty when there is none)                |
| `{{.snoozed}}`                  | whether the item is [snoozed](#snoozing)                        |
| `{{.vars.name}}`                | the action's `vars`                                             |
| `{{.settings}}`                 | path to a Claude settings file that reports session status      |
| `{{.session}}` `{{.action}}`    | the session / action name                                       |
| `{{.claude_session}}`           | the Claude session id to resume; only in `resume` (an error in `run` or `cwd`) |

`run` is shell code, and fields like `title` or `branch` are written by whoever opened the item, so `lanes`
escapes every value a `run` (or `resume`) template inserts, for the quoting it lands in:

| Template                       | `title` = `fix $(rm -rf ~)` becomes |
|--------------------------------|-------------------------------------|
| `echo {{.title}}`              | `echo 'fix $(rm -rf ~)'`            |
| `claude "/review {{.title}}"`  | `claude "/review fix \$(rm -rf ~)"` |

Outside quotes a value is single-quoted; inside `"…"` its `\`, `$`, `` ` `` and `"` are backslash-escaped.
So write placeholders bare or inside double quotes, never inside single quotes: `'{{.title}}'` is an error
(drop the quotes, or use `"…"`). The same goes for any spot where `lanes` cannot tell the quoting for sure,
such as inside `` `…` `` (use `$(…)`), `${…}`, a here-document or a `#` comment: an error, never a guess.
`lanes doctor` reports such templates, naming the action.

`{{.x | raw}}` inserts a value unchanged — only for values you control, such as a var holding several flags
(`vars = { flags = "--draft --fill" }`, `run = 'glab mr create {{.vars.flags | raw}}'`). `{{.x | q}}` is
no longer needed: it gives the same output as `{{.x}}`. `cwd` is a directory, not shell code, so its values
go in unchanged.

A missing field or variable is an error shown in the footer; nothing runs. The message names the missing field and the item's
identifier field (`iid` on GitLab, `number` on GitHub, `key` on Linear, otherwise `id`); `p` shows the item's fields:

```text
action work-tickets: no field "iid"; this item's identifier is "key" (see fields with p)
```

### Running on many items

Mark items with `space` (or `V` for all), then press the action key. Because this is easy to do by accident,
multi-item runs always ask first:

```text
Run "review" on 3 items (!1948, !1957, !1961)?  y/enter yes · n/esc no
```

Each item gets its own session; items that already have the action running are skipped.

**Session limit.** `max_claude_sessions` under `[settings]` (default `3`, `0` = no limit) caps how many live
sessions lanes' actions run before starting more asks first. Every live session lanes started counts, idle,
waiting or a [shell](#shell-sessions) included, whatever its command; finished (dead) sessions and
[run actions](#run-actions) do not, and restarting a finished session counts as a new one. When a start (an
action key, `gr` / `:resume`) would take the count above the limit, lanes asks; on a multi-item run this is the
one prompt it shows:

```text
Limit of 3 reached (2 live). Run "review" on 3 items (!1948, !1957, !1961) anyway?  y/enter yes · n/esc no
```

Yes starts the whole batch, no starts nothing. [Chained](#chaining-actions-on_success-on_failure) follow-ups
start without asking. [`on_enter`](#running-on-arrival-on_enter) starts never ask: over the limit they wait in a
queue instead. A negative value fails the config load.

### Restricting actions (`when`)

Every action can run on every item unless you restrict it. `when` takes a [filter expression](#filters),
evaluated per item against the same fields; the action only runs on items it matches:

```toml
[action.review]
run  = 'claude --settings {{.settings}} "/local-code-review {{.url}}"'
when = 'kind == "pr"'

[action.work]
run  = 'claude --settings {{.settings}} "/harness:work {{.number}}"'
when = 'kind == "ticket"'
```

`kind` is `pr` for items from `gitlab` and `github` and `ticket` for items from `github-issues` and `linear` (a `command`
provider sets it in its fields, if at all).

- A run on marked items starts the action on the matching ones and reports `<action>: skipped N item(s) (when)`.
  Skipped items stay marked, so you can see what did not run. If no item matches, nothing runs.
- The multi-item confirm prompt counts only the matching items.
- `:run <action>` honours `when` like a key does. Built-in keys (`o`, `x`, `enter`, …) work on every item.
- The help screen (`?`) lists only the actions that match the item under the cursor.
- An invalid expression fails config load, naming the action. An expression that fails on one item (for example
  a type mismatch) counts as no match; the error is shown in the footer.

### Chaining actions (`on_success`, `on_failure`)

`on_success` and `on_failure` name another action that lanes starts on the **same item** when this one ends:
exit 0 starts `on_success`, any other exit `on_failure`.

```toml
[action.work]
run        = 'claude --settings {{.settings}} -p "/harness:work {{.number}}"'
on_success = "review"
on_failure = "fix"
```

- A session action chains when lanes sees its session's command exit. An interactive `claude` session chains
  when you quit Claude, not when it stops to ask; `claude -p` and scripts chain when they finish. A session that
  ends while lanes is not running is never seen ending, so it starts nothing.
- A run action chains per item: in a multi-item run each item starts the follow-up for its own outcome. After a
  `refresh = true` run the follow-up starts right away; it does not wait for the refetch.
- Sessions closed with `x` or interrupted with `X` start nothing.
- The follow-up goes through the normal start path, without asking and without jumping in: its `when` is honoured,
  and an item that is busy, already running it or no longer listed is skipped with a notice. Your marks stay.
- The footer says `work finished on #5 → starting review` (or `work failed on #5 → …`); the item history records
  the follow-up's `started` as usual.
- Config load fails on an action name that does not exist, on a follow-up with `confirm = "single"` (lanes never
  asks out of the blue) and on a chain that loops back (`work → review → work`).

### Running on arrival (`on_enter`)

`on_enter` on a view names an action that lanes starts by itself on every item that **newly enters** the view, for
example a Claude review for each MR that shows up in `review`:

```toml
[[view]]
name     = "review"
provider = "gitlab"
filter   = '"waiting-for-review" in labels && !draft && !approved_by_me'
on_enter = "review"

[action.review]
run  = 'claude --settings {{.settings}} "/local-code-review {{.url}}"'
when = '"auto-review" in labels'   # optional gate: only items with this label are started
```

- **Only new arrivals.** The first fetch after lanes starts (and after a config reload) only records what the view
  lists; nothing already there fires. An item fires the first time a later fetch shows it in the view.
- **Once per item.** Each start writes an `auto` event to the item history before it runs, so the action never
  fires on that item again, not after a restart either. An item that leaves and re-joins the view does not fire
  again unless the view sets `on_enter_repeat`.
- **Normal start path, unattended.** The action's `when` is honoured, so a label in `when` works as an opt-in gate
  (it applies to the action key too); an item that does not match counts as seen and does not fire later. Items
  that are busy or already run the action are skipped. Auto-starts never ask, whatever `confirm` says, never jump
  into the session, whatever `jump` says, and leave your marks alone.
- **Session limit.** A start that would go over [`max_claude_sessions`](#running-on-many-items) waits in a queue
  (first in, first out) instead of asking. The row shows `queued <action>` and the footer `queued N`. The queue
  starts the next entry when a session ends or a fetch lands and there is room again, checking `when` once more;
  an entry whose item left the view is dropped. The queue lives in memory only: on quit it is lost, and the
  items stay handled; run the action by hand.
- **Failures are not retried.** An auto-start that fails, or whose command exits non-zero, is recorded in the item
  history like any run; later fetches do not start it again. Run it by hand to retry.
- **Footer.** Each auto-start is reported, e.g. `auto-started review on !1961` or `auto-started review on 3 items`;
  for run actions and failed starts the usual notice is prefixed with `auto:`.
- **Only full fetches count.** A fetch that fails or returns a partial list never makes items look as if they left
  or arrived.
- **Pause.** `:auto off` pauses every view's `on_enter`: nothing starts and the queue stops draining (its entries
  stay). Items that arrive meanwhile count as seen and do not fire after `:auto on`, which resumes the queue.
  `:auto` alone shows the state and the queue length. The switch is not saved: lanes always starts with auto-runs on.

**Re-entry (`on_enter_repeat`).** To handle items that come back to a view (an MR that went back to draft and is
ready for review again), set a cooldown as a Go duration:

```toml
[[view]]
name            = "review"
provider        = "gitlab"
on_enter        = "review"
on_enter_repeat = "24h"
```

An item then fires again when lanes saw it leave the view and later come back, both while running, **and** its
last `auto` start of this action is older than the cooldown. Re-entries inside the cooldown do not fire, so an item
that flaps in and out starts at most one session per cooldown; pick hours, not seconds. An item that left while
lanes was closed counts as already there at startup. Unlike a plain refetch, a re-entry after the cooldown also
starts an item whose earlier auto-run failed.

Config load fails when `on_enter` names an action that does not exist, and when `on_enter_repeat` is not a valid
positive duration or is set without `on_enter`.

### Sessions: jump, close, status

- **Jump** — `enter` on an item attaches to its newest session. Inside tmux this opens a popup over `lanes`;
  outside tmux, `lanes` suspends and attaches. **Detach with `ctrl-b d`** to come back (the session keeps
  running). A hint is shown in the session's status bar.
- **Next needing you** — `n` jumps into the next live session that is asking or waiting, across all views:
  asking before waiting, oldest first. Pressing `n` again goes to the next one and wraps around; with none,
  the footer says `nothing needs you`.
- **Close** — `x` closes the sessions of the marked items, or of the item under the cursor. It always asks
  first (`Close 3 session(s) (1 running)?`, the running part only when one is still running): a finished session
  still holds its output.
- **Interrupt** — `X` stops the command of the running sessions of the marked items, or of the item under the
  cursor, after a confirmation (`Interrupt 2 running session(s)?`); finished sessions are skipped, and with none
  running the footer says `no running sessions here`. The command gets `SIGTERM`; the session stays, shows
  `✗ <action> (143)` and keeps its output, so you can still read it with `enter` and close it with `x`. If a
  command ignores `SIGTERM`, `X` on it again sends `SIGKILL`. The item history records `interrupted`.
- **Sessions screen** — `S` (or `:sessions`) lists every session on the `lanes` tmux server, newest first:
  item ID and title (or the item key when no provider returns the item any more), status badge and age.
  Sessions whose item is in no view — filtered out, or merged/closed — are marked `(not in any view)`. `enter`
  jumps in, `A` opens its [agent view](#agent-view), `X` interrupts, `x` closes (after confirming), `j`/`k`,
  `gg`/`G` move, `q` / `esc` / `S` go back.
- **Resume** — `gr` (or `:resume`) starts the item's newest finished Claude session again, see
  [Resuming a Claude session](#resuming-a-claude-session).
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

#### Resuming a Claude session

The hooks also note the Claude session id. When the `lanes` session ends (Claude finished or failed, or you
closed or interrupted it), the item history keeps that id with the end event. Give the action a `resume`
template to continue that conversation later:

```toml
[action.review]
run    = 'claude --settings {{.settings}} "/review {{.url}}"'
resume = 'claude --settings {{.settings}} --resume {{.claude_session}}'
```

`gr` (or `:resume`) on an item takes its newest history event that has a Claude session id and whose action
has `resume`, renders `resume` like `run` (with the action's `cwd`, which Claude needs to find the session),
plus `{{.claude_session}}`, and starts it in that action's session, restarting a finished one. Marks are
ignored and kept. If the action's session is still running, the footer says `work is still running — enter
jumps` and nothing starts; with nothing to resume, it says so. `gr` wins over a `[keymap]` binding of `r`.
After `/clear` inside Claude, `gr` resumes the conversation that followed it.

#### Usage in the footer

With `claude_dir` under `[settings]`, the left of the footer shows that Claude profile's usage, e.g.
`claude usage: 5h 23% · wk 30% · budget +4% · reset 15:00` (a notice, the mark count or an active search take its
place while shown). lanes reads it from `<claude_dir>/usage-line.json`, which a Claude
Code mod writes whenever Claude measures usage:

```json
{ "text": "5h 23% · wk 30% · budget +4% · reset 15:00", "session": 23, "at": 1761561600 }
```

lanes shows `text` as is, so it matches your Claude statusline. The file only changes while a Claude
session of that profile runs; after 10 minutes the text shows its age. Each profile has its own
dir (e.g. `~/.claude` and `~/.claude-personal` for `CLAUDE_CONFIG_DIR=~/.claude-personal claude`), and a
project's `.lanes.toml` can point at a different one than the global config. Unset, or no file: nothing is shown.
The [agent view](#agent-view) splits `session` (the account's 5h usage in %) over the lead and its agents.

### Agent view

`A` on an item opens a full-screen, animated view of its newest Claude session; on the sessions screen, of the
selected session. `esc`, `q` or `A` go back. It needs the [hooks](#claude-code-integration)
(`--settings {{.settings}}`): they record where Claude keeps the session's transcript. Without them, or before
Claude has started, the footer says `no Claude session for this item`.

The view is a dashboard across the whole terminal, with one layout at every size: it never hides or replaces
anything because the terminal is small. It shows, top to bottom:

- **header** — `AGENT VIEW · <title>`: the item's title, or the session's name when the view was opened from the
  sessions screen (or the title is unknown); under its rule a **legend** of the model family colors
  (`■ opus  ■ sonnet  ■ haiku`), always drawn.

- **session** (left column, full height) — the lead's dashboard:
  - the item's name and how long the session has run, the lead's `ctx` bar, stretched over the row between the label and its % (one cell is one step, so the % per step depends on the width), and its `effort` meter;
  - `activity` — tool calls of the lead and its agents per minute over the last 15 minutes, as a sparkline
    (`▁▂▃▄▅▆▇█`, one cell per minute);
  - the hook state on a dashed `┆` timeline (`◆ working`, `◇ asking`, `◇ waiting`; the active one highlighted);
  - counters for `turns`, `tools` (tool calls), `tokens` (input + cache write + output), split into `in`, `out`
    and `cache` (read + write), the lead's `5h` share (see **5h share** below; left out when unknown),
    and `agents` running `◓`, done `✓` and failed `✗`;
  - `files` — the last 4 files that Edit, Write or NotebookEdit touched, newest first;
  - `last:` — the lead's newest text, typed out;
  - `advisor` — the advisor model and its `calls`, when the session uses one.
- **lead** — the main Claude, in a double border: its model and `effort` meter with its level (left out when
  unknown), current tool and argument, state and elapsed time.
- **tools** — the lead's three most used tools, with their share of all its calls, each as a `■` bar stretched over the row between the name and the %, which sits at the right edge (one cell is one step). The box sits left of the lead
  on its top row, `3` columns apart and joined to nothing; the agent tree starts directly below the lead. It
  scrolls with the diagram, which is wide enough for both: on a narrow terminal reach it with `h`/`l`.
- **processes** — right of the lead on its top row, `3` columns apart, the 5 processes of the session using the most CPU: the agent running
  each (in its family color), its CPU share, memory and command (see **Processes** below). Left out until the
  first sample, and once the session ended.
- **agents** — the subagents as a layered tree, one row per spawn depth, each connected to the agent that
  spawned it (or to the lead). Each box has the agent's full name in its top border, then its model and `effort`
  meter, a `ctx` bar stretched over the row, with its % at the right edge, a usage row (`41.2k tok  37 calls  5h 3%  cpu 180%`:
  its tokens, tool calls, 5h share and the CPU share of the processes it runs, left out while it runs none, spread
  over the row), its task and a live text row; the bottom border shows the status: a spinner with elapsed time and
  `▸ <tool> <argument>` while running, `✓ done` with its duration, `✗` when it failed. The border takes the
  model family's color; finished agents are dimmed.
  The **task** is the subagent's description, else the first sentence of its first prompt (a leading
  `Read and follow …` line skipped), at most 40 characters. The **live text** `» …` is the first line of the
  agent's newest text, at most 60 characters, cut to the box; it changes while the agent runs, and a done or
  failed agent keeps its last text and its usage figures, dimmed. Tokens are input + cache write + output, like
  the lead's; where the row is too narrow for the units, it shows only the numbers (`412.3k  12.3k  5h 7%`), and where that doesn't fit either, the 5h share goes. Every agent is drawn: as many boxes as fit go side by side,
  and a depth that doesn't fit one line wraps into as many lines as it needs.
- **session log** — a closed box of 4 lines, newest at the bottom, of everything the lead and the agents do:
  their tool calls (`Read file.go`, `Bash go test ./…`; failed calls in red), the first line of each of their
  texts, spawns, finishes and failures, and the lead's hook state changes (`working`, `asking`, `waiting`). Each
  line is `time actor message`, the actor in its model family's color; the name column fits names of up to 20
  characters. It keeps the newest 200 lines; an ended session keeps its log.
- **status line** — one entry per model family in its color, e.g. `opus [2 running]`, `sonnet [1 failed]`,
  `haiku [done]`; then `cpu [n%]` (the CPU share of the whole process tree, left out until there is one),
  `agents [running/total]`, `tools [n]`, `tokens [n]` and `● live` or `■ ended`.

**Focus.** `tab` and `shift+tab` move the focus between the session column, the diagram and the session log (and
around); the diagram has it when the view opens. The focused area is highlighted in the accent color (the
diagram's name shows above the log) and takes the scroll keys; the others stay where they are.

**Scrolling.** The diagram (lead, tools box and agent tree) scrolls as one block when it is higher or wider than the
space between the session column and the log: `j`/`k` (or the arrows) a line, `ctrl-d`/`ctrl-u` half a page,
`g`/`G` to the top/bottom, `h`/`l` (or the arrows) a column sideways when it is wider than the terminal. Scrolling
stops at the edges and keeps its place as the session updates. The header, legend, session column, log and status
line stay put; above the log, `lines 1-6/45` and `cols 1-29/36` show where you are (nothing when it all fits and
the diagram has no focus). The session log scrolls back with the same keys: it follows the newest line until you
scroll up; then it stays put while new lines come in, its top border shows the newest line shown and the total
(`12/87`), and `G` (or scrolling back to the bottom) follows the newest line again. The session column scrolls
when it is taller than the terminal.

**Look.** The view has its own neon palette: opus magenta, sonnet cyan, haiku yellow, a violet header, heavy box
borders with a double one around the lead, and bars that light up over a deep track. Light terminals get a
toned-down variant of the same hues. Change any of its colors under
[`[theme.agent_view]`](#agent-view-colors); the app's `[theme]` colors don't apply here. Labels are dim and
values bright; numbers have thousands separators.

**Effort** is the newest `effort` in the lead's or the agent's transcript, shown as a 4-cell meter: `low ▮▯▯▯`,
`medium ▮▮▯▯`, `high ▮▮▮▯`, `xhigh` / `max ▮▮▮▮`. When a transcript has none, the meter is left out.

**Packets.** Connectors show where work happens. While an agent runs, packets in its parent's color stream down
its connector, about one every 1.2 seconds (each connector at its own pace). When it finishes or fails, a burst
of 3 packets runs back up, then the connector goes quiet and dim.

**Where the data comes from.** The hooks note where Claude writes the session's
transcript (`<session>.jsonl` under Claude's config dir). While the view is open, `lanes` reads what was added to it every
half second, together with the subagents next to it: `<session>/subagents/agent-<id>.jsonl` (each agent's
transcript) and `agent-<id>.meta.json` (type, name, task, parent, model, finished). Only new bytes are read;
unknown lines and a half-written last line are skipped. Nothing is read or animated while the view is closed.
An agent's name is the meta's `name`, else the brief its prompt says to follow (`Read and follow
…/briefs/implementer.md` → `implementer`), else its agent type.

**Context %.** An agent's `ctx` is the input + cache read + cache write tokens of its newest reply, divided by the
model's context window: 1,000,000 for model ids ending in `[1m]`, otherwise 200,000 (more tokens than that mean a
1M window).

**5h share.** With [`claude_dir`](#usage-in-the-footer) set, each agent box and the session column show the
actor's share of the account's 5h usage window: the `session` % of `usage-line.json` times the actor's tokens over
the tokens of the lead and all agents of this session, rounded to a whole %; 0 while the session has no tokens.
It assumes this session drives the usage: other Claude sessions of the account are not split out. The share is
left out (tokens and calls stay) when the file is missing or unreadable, has no `session` value, or was written
more than 15 minutes ago. An ended session keeps the share it ended with.

**Processes.** While the view is open and Claude runs, `lanes` samples the session's process tree (the pane's
process and everything under it: Claude, the commands its Bash calls run, MCP servers) with `ps` every 3 seconds,
and nothing while the view is closed. The CPU share is the CPU time a process used between two samples over the
time between them, in % of one core (above 100% for several threads); `ps`'s own `%cpu` is not used, as it is a
lifetime average on Linux and a decaying one on macOS. A process's first sample, or a pid reused in between,
shows `n/a`. Both `ps` time formats are read: Linux's `[dd-]hh:mm:ss` and macOS's `mm:ss.ss`. Which agent runs
a process is a best guess: a process whose command holds the command of an agent's running Bash call goes to
that agent, with everything it starts; the rest, and a command several agents run at once, go to the lead.
Subagents run inside the one Claude process, so their own CPU can't be split, and a command that forks away
from its shell may land on the lead.

**Ended sessions.** When Claude exits or the session finishes, the view reads the transcripts once more and
freezes on that final state; the status line says `■ ended` instead of `● live`. Nothing moves, no packets run, and no
agent counts as running: agents that hadn't finished show as `stopped` in the status line.

**Size.** The dashboard uses the whole terminal: the wider it is, the more agent boxes (28–30 columns each,
wider when a name needs it) fit side by side, e.g. 5 at 200 columns. Boxes never get narrower than 28 columns: on
a terminal narrower than about 52 columns (the session column plus one box) the diagram scrolls sideways instead.
Rows of the session column that don't fit its height are left out.

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

### github-issues

```toml
[provider.issues]
type = "github-issues"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Lists the repository's **open issues** via `gh api graphql` (pull requests are not included), the 100 most
recently updated, each with its first 20 blockers, blocked issues and sub-issues. The ID is `#<number>`; `author` and `url` are set as usual
(the list shows the assignees instead of the author, then a priority strip), and a `blocked` badge marks issues with an open blocker.

Ticket fields:

| Field        | Type     | Meaning                                                              |
|--------------|----------|----------------------------------------------------------------------|
| `kind`       | string   | `ticket`                                                             |
| `number`     | number   | issue number                                                         |
| `status`     | string   | always `open` (only open issues are listed; filter on labels)        |
| `assignees`  | []string | assignee logins                                                      |
| `labels`     | []string | label names                                                          |
| `blocked`    | bool     | an issue it is blocked by (GitHub issue dependencies) is still open  |
| `blocked_by` | []string | the open blockers, e.g. `["#7"]`                                     |
| `blocking`   | []string | the open issues it blocks                                            |
| `related`    | []string | its open parent and sub-issues (GitHub has no "related" relation)    |
| `closed_links` | []string | closed blockers, blocked and related issues as `<relation>:<id>`, e.g. `["blocked_by:#9"]` |
| `priority`   | number   | from `priority:1`..`priority:4` labels (lowest wins), `0` if none    |
| `me`         | string   | your login                                                           |

A view of your unblocked issues that are not being planned, with a key that hands the selected one to Claude:

```toml
[[view]]
name     = "tickets"
provider = "issues"
filter   = 'me in assignees && !blocked && !("planning" in labels)'

[keymap]
w = "work"

[action.work]
run  = 'claude --settings {{.settings}} "/harness:work {{.number}}"'
when = 'kind == "ticket"'
```

### linear

```toml
[provider.linear]
type = "linear"
team = "ENG"                    # required: the team key (the prefix of its issue identifiers)
# token_env = "LINEAR_API_KEY"  # env var holding your Linear API key (default LINEAR_API_KEY)
```

Lists the team's **open issues** (state not completed or canceled) via the Linear GraphQL API, fetched
100 per request until all pages are read. The API key is read from the environment on every fetch and never written anywhere;
`lanes doctor` reports whether the variable is set. The ID is the identifier (`ENG-12`), `author` is the
creator (the list shows the assignee instead, then a priority strip), and badges show `blocked` and the workflow state.

Items carry the same [ticket fields](#github-issues) as `github-issues`, so views and actions work with either
tracker. The differences:

| Field        | Type     | Meaning on Linear                                                    |
|--------------|----------|----------------------------------------------------------------------|
| `key`        | string   | identifier, e.g. `ENG-12` (instead of `number`)                      |
| `status`     | string   | workflow state name, lower-cased, e.g. `todo`, `in progress`         |
| `assignees`  | []string | the assignee's display name, or empty                                |
| `blocked`    | bool     | an issue that blocks it is not completed or canceled                 |
| `blocked_by` | []string | those blocking issues, e.g. `["ENG-7"]`                              |
| `blocking`   | []string | the open issues it blocks                                            |
| `related`    | []string | open issues linked with a `related` relation, in either direction    |
| `closed_links` | []string | completed or canceled linked issues as `<relation>:<id>`, e.g. `["blocking:ENG-8"]` |
| `priority`   | number   | `0` none, `1` urgent, `2` high, `3` medium, `4` low                  |
| `me`         | string   | your display name                                                    |

For a `work` action, pass `{{.key}}` instead of `{{.number}}`.

### auto

```toml
[provider.forge]
type = "auto"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Picks `github` when the `origin` remote of the current directory is on `github.com` (https or ssh URL) and
`gitlab` for any other host, on its first fetch each time `lanes` starts or reloads its config — so one global config works in
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

For the [details](#preview-pane-and-item-history) screen (`d`), add an optional `detail` command. It runs through
`$SHELL -c` when you press `d`, never with the list refresh, gets the item's `id` in `$LANES_ITEM_ID` and must print
one JSON object; every key is optional:

```toml
[provider.todos]
type = "command"
command = "my-todo-exporter --json"
detail = 'my-todo-exporter --json --show "$LANES_ITEM_ID"'
```

```json
{
  "description": "Explain install, config and keys.",
  "jobs": [{ "name": "lint", "stage": "test", "status": "success", "duration": 42 }],
  "threads": [
    {
      "file": "README.md",
      "line": 12,
      "resolved": false,
      "comments": [{ "author": "sam", "body": "Mention brew?", "created": "2026-11-01T10:00:00Z" }]
    }
  ],
  "comments": [{ "author": "chris", "body": "Draft is up.", "created": "2026-11-02T09:30:00Z" }]
}
```

`jobs` fill the Pipeline section, grouped by `stage`; `status` is one of `success`, `failed`, `warning`, `running`,
`pending`, `skipped`, `canceled`, `manual`, and `duration` is in seconds. A thread's first comment opens it, the
rest are replies; `created` is an RFC 3339 time. Without `detail` the screen shows the item's fields only; a
failing command or invalid JSON shows an error line under them.

## Configuration

### Global and project config

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

### Trusting a project

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

### Full reference

```toml
[settings]
poll_interval = "30s"   # how often lists refresh (minimum 5s)
poll_interval_blurred = "5m"  # how often lists refresh while the terminal is unfocused (never below poll_interval)
preview       = false   # open the preview pane at startup
no_notify     = false   # disable desktop notifications
no_update_check = false # never check GitHub for a newer lanes release (see Updates under Install)
claude_dir    = "~/.claude"   # show this Claude profile's usage in the footer (see Claude Code integration)
max_claude_sessions = 3 # live action sessions before starting more asks first; 0 = no limit (see Running on many items)

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

[theme]
accent = "#A78BFA"          # also: ok, warn, bad, info, text, dim, selection

[theme.agent_view]
opus = "#FF4FE3"            # also: sonnet, haiku, text, dim, accent, track, ok, bad, selection
```

### Theme

`lanes` ships an adaptive light/dark palette. Override any of `accent`, `ok`, `warn`, `bad`, `info`, `text` (body), `dim`
(secondary text) and `selection` (selected row and tab background) with a hex color under `[theme]`. Changes apply live.

The selected row and the active tab are painted with a medium-strength violet `selection` background (dark `#404872`, light `#CFC4F0`).
Set `selection` (flat, or per mode under `[theme.light]` / `[theme.dark]`) to change it.

The footer draws errors in the `bad` color and warnings (a request that was not fully carried out, such as
`tmux unavailable` or a skipped item) in `warn`; other messages stay plain.

`mode` picks the palette: `system` (the default) follows the terminal background as detected at start, `light` and
`dark` force that palette. Forcing a mode helps when detection is slow or wrong, for example over SSH.

Inside tmux, detection asks the outer terminal through tmux's passthrough, which needs `set -g allow-passthrough on`
in your tmux config. Without it the answer never arrives and `lanes` falls back to `COLORFGBG`, then to the dark
palette; on a light terminal set that option or use `mode = "light"`.

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

#### Agent view colors

The [agent view](#agent-view) has its own neon palette. `[theme.agent_view]` overrides any of its colors: `opus`,
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

### Where things live

| What                                   | Where (default)                                         |
|----------------------------------------|---------------------------------------------------------|
| Global config                          | `~/.config/lanes/config.toml`                           |
| Project config                         | `.lanes.toml` in the project (or a parent)              |
| Trusted projects                       | `~/.local/state/lanes/trust.json` (all projects)        |
| In-app views and filter edits          | `~/.local/state/lanes/<project>/views.json`             |
| Item history                           | `~/.local/state/lanes/<project>/history.jsonl`          |
| Item notes                             | `~/.local/state/lanes/<project>/notes.json`             |
| Snoozed items                          | `~/.local/state/lanes/<project>/snoozes.json`           |
| Session status files, tmux config      | `~/.local/state/lanes/<project>/`                       |
| Cached lists, last update check        | `~/.cache/lanes/<project>/`                             |

`<project>` is the git root's directory name plus a short hash of its path (for example `payments-3fa9c1`), so
two projects never share history, views or sessions. The `$XDG_*` variables are honoured.

## How it works

- **Data flow.** On start `lanes` paints the cached list, then fetches each provider in the background and
  re-fetches every `poll_interval`. While the terminal is unfocused it re-fetches only every `poll_interval_blurred`
  (default 5m, so desktop notifications lag by up to that much), and when it regains focus every provider older than
  `poll_interval` is fetched at once. This needs a terminal that reports focus; inside tmux add `set -g focus-events on`
  to `~/.tmux.conf`. Without focus reports lanes polls at `poll_interval` throughout. A failing provider is retried after `poll_interval`, doubling per consecutive failure up to 5 minutes (`R` / `:refresh` retry at once). New data is diffed into the list by item `id`, so the cursor and your marks
  stay put. Every view re-applies its filter on each refresh.
- **Fetch status.** The right of the header shows each provider's status, updated every second:
  `gitlab cached` (only the cached list so far), `gitlab [  ●  ] fetching 4s · last 12.3s` (a fetch running for 4s,
  with a dot sweeping back and forth in its track ten times a second; the previous one took 12.3s), `gitlab updated 12s ago · 1.4s` (the last fetch took 1.4s) or `gitlab ✗ failed · 2s` in
  the error colour. A slow provider shows up there without a profiler. When the terminal is too narrow for every
  provider, only the current view's one is shown. An empty view whose provider is still fetching lists the same
  status lines in place of the items.
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
`gh auth status` for `github` and `github-issues` (an `auto` provider counts as the forge its `origin` resolves to here; without an
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
- **An action reports `no field "x"; this item's identifier is …`** — the template uses a field the item does
  not have; check the name against `p`. **`map has no entry for key "x"`** means `{{.vars.x}}` is not defined:
  add it to the action's `vars`.
- **The list is empty** — the filter excludes everything, or its fields don't exist for that provider. Press
  `F` to look at the filter, or `:filter reset`.
- **Start fresh** — delete `~/.local/state/lanes` and `~/.cache/lanes` (and the config if you want the
  defaults back). Running sessions: `tmux -L lanes-<project> kill-server`.

## Development

```sh
make build        # ./bin/lanes
make test         # go vet + go test
make version      # tag and push the next release
```

### Releasing

`make version` tags the next release from the [Conventional Commits](https://www.conventionalcommits.org) since
the last `v*` tag: a `feat` bumps the minor version, anything else the patch; the first release is `v1.0.0`. It
checks that you are on `main` with a clean tree, in sync with `origin/main`, that the tag doesn't exist yet and that
the `ci` workflow succeeded on HEAD (needs `gh`). It then shows the version and the commits and asks before it
creates and pushes an annotated tag. The pushed tag runs the release workflow, which tests, builds the binaries with
GoReleaser and publishes the GitHub release with a grouped changelog.

| Variable     | Effect                                                                                    |
|--------------|-------------------------------------------------------------------------------------------|
| `DRY_RUN=1`  | show the version and commits, create no tag                                               |
| `MAJOR=1`    | allow a major bump (a `type!:` commit or a `BREAKING CHANGE:` footer); refused otherwise, as v2+ needs a `/v2` module path for `go install …@latest` to see it |
| `V=vX.Y.Z`   | tag this version instead; must be greater than the last release                           |
| `SKIP_CI=1`  | skip the CI check                                                                         |

Example: `make version DRY_RUN=1`.

The code is organised by concern under `internal/`: `config` (TOML, project overlay, in-app overrides),
`provider` (GitLab, GitHub, command), `filter` (expressions), `session` (tmux, Claude hooks), `history`, `cache` and
`ui` (Bubble Tea). Design notes: [docs/design.md](docs/design.md).

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss), [Bubbles](https://github.com/charmbracelet/bubbles),
[expr](https://github.com/expr-lang/expr) and [BurntSushi/toml](https://github.com/BurntSushi/toml).

## License

[MIT](LICENSE) © Christopher Hartmann
