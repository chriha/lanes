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

## Documentation

- [Using lanes](docs/usage.md) — views, filters, sorting, groups, navigation, search, the command line, badges, preview, snoozing
- [Actions](docs/actions.md) — session, shell and run actions, templates, multi-select, `when`, chaining, `on_enter`, notifications
- [Claude Code](docs/claude.md) — session status from Claude's hooks, resuming, usage in the footer, the agent view
- [Providers](docs/providers.md) — gitlab, github, github-issues, linear, auto and the `command` provider
- [Configuration](docs/configuration.md) — global and project config, profiles, trusting a project, the full reference, theme, where things live
- [How it works](docs/how-it-works.md) — the architecture, and how lanes uses tmux
- [Troubleshooting](docs/troubleshooting.md) — `lanes doctor` and common problems
- [Development](docs/development.md) — building, testing and releasing
- [Design](docs/design.md) — the design notes behind lanes

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
   [Trusting a project](docs/configuration.md#trusting-a-project)).

On first start `lanes` writes a default global config (see [Where things live](docs/configuration.md#where-things-live)) with three
views — `review`, `mine`, `drafts` — and one `auto` provider named `forge`, which picks GitHub for a `github.com`
`origin` and GitLab otherwise, and detects the repository from that remote.
Press `?` at any time for the built-in help (scroll with `j`/`k`, close with `q`).

**Scrollbars.** Every scrollable area shows a one-column scrollbar on its right edge while its content overflows: the
list, queue and sessions views, the help and detail screens, and in the agent view the diagram, the session log and
the session column. The thumb (`█`) is as tall as the visible part of the content (at least one cell) and moves with
the scroll position over the track (`░`); nothing is drawn, and the full width stays, while everything fits. The
list and the screens give up their last column to the bar, only while it shows. The text positions (`1–20 of 80`,
the log's `12/87`, the diagram's `lines`/`cols`) stay.

Then make it yours: press `:edit` (or edit the file in your editor — changes apply immediately), define the
filters for the lists you care about and bind keys to the commands you run all the time.

Besides the TUI, `lanes` has these commands:

| Command          | Does                                                                                      |
|------------------|-------------------------------------------------------------------------------------------|
| `lanes init`     | write a commented `.lanes.toml` to the current directory; asks before overwriting one, with a backup (see [Global and project config](docs/configuration.md#global-and-project-config)); asks for trust first, like the TUI |
| `lanes doctor`   | check tmux, the forge CLIs, `claude` and the lanes binary (for hooks), the trust state, the global and project config, and the state dirs (see [Troubleshooting](docs/troubleshooting.md#troubleshooting)); asks for trust first, like the TUI |
| `lanes filter test '<expr>' [--view <name>]` | run a filter expression against the cached items of a view (the first one, or `--view`), print `match` or `no match` per item and, for a non-match, the failed top-level `&&` / `\|\|` / `!` operand (see [Filters](docs/usage.md#filters)); never fetches; asks for trust first, like `doctor`; exits 2 when the expression does not compile |
| `lanes status`   | print the current project's sessions that need input (see [tmux and lanes](docs/how-it-works.md#tmux-and-lanes)); prints nothing in an untrusted project and never asks |
| `lanes --version`| print the version; never asks for trust                                                   |
| `lanes --untrust`| forget the trust of the current project, so the next start asks again (see [Trusting a project](docs/configuration.md#trusting-a-project)); never asks, works without a terminal |

## License

[MIT](LICENSE) © Christopher Hartmann
