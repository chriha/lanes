# lanes — design notes

Working name: **lanes** (each *view* is a lane of work items). Generic on purpose: no GitLab/MR/horse
coupling in the core, so it works in any project.

**Renaming:** the name lives in exactly three places — `go.mod` module path (and imports), `internal/app/app.go`
(`const Name`), and the `Makefile` (`NAME :=`). Config dir, tmux socket, state dir and the binary all derive
from `Name`. Change those three and `grep -ri lanes` for docs.

## Goal

A fast, live, vim-style TUI that shows filtered lists of items (MRs first) and runs actions on them in
interactive tmux sessions with one keypress — single or multi-select.

## Non-goals

- Creating worktrees/branches itself. Commands that need a worktree (e.g. Claude commands such as
  `work-tickets`) do that; lanes only sets `cwd` if an action says so.
- Macros. Attention-key, headless/scriptable CLI: later.
- Windows support.

## Stack

Go, Bubble Tea (`bubbletea`), Lip Gloss (`lipgloss`), Bubbles (`bubbles`), `BurntSushi/toml`,
`expr-lang/expr` for filter expressions. tmux is a hard dependency. GitLab via `glab api graphql`,
GitHub via `gh api graphql` (both reuse existing auth), Linear via its GraphQL API over `net/http` (API key from
an env var).

## Concepts

- **Provider** — produces items. Built-in `gitlab` and `github` (GitHub PRs mapped onto the GitLab MR fields,
  so filters and badges carry over); `github-issues` (a repo's open issues as tickets: `kind = "ticket"`,
  `assignees`, `labels`, `blocked`/`blocked_by` from issue dependencies, `status`, `priority`); `linear` (a team's
  open Linear issues with the same ticket fields, `key` instead of `number`); `auto`, resolved
  when the provider is built (start, config reload): `github` for a `github.com` origin, `gitlab` for any other host, so the global default config works in both kinds of
  repo; or `command` provider: any shell command printing a JSON array of items. Items are generic: `{id, title, url, fields{...}}`.
- **View** — name + provider + filter expression + columns + optional keymap overrides. Switch views with
  `tab`/`shift-tab`, `1-9`, or `:view <name>`.
- **Filter** — `expr` expression evaluated client-side over item fields, e.g.
  `"waiting-for-review" in labels && !draft && !approved_by_me && !open_thread_by_me`.
- **Action** — config-defined, templated (`{{.id}} {{.branch}} {{.ticket}} {{.url}} {{.field.x}}`):
  - `session`: command run in a detached tmux session (user's `$SHELL -ic`), optional `cwd`.
  - `api`: provider mutation without a session (e.g. gitlab `ready`, `sync`).
  - `confirm`: `none | single | always`. Multi-select always confirms.
  - `when`: optional filter expression evaluated per item (compiled at config load); items that don't match are
    skipped with a notice, and the help screen lists only actions matching the cursor item. Items carry a `kind`
    field (`pr` from gitlab/github, `ticket` from github-issues/linear) to scope actions by item type.
- **Session** — tmux session on dedicated socket (`tmux -L <Name>`), one per (action, item). Never touches the
  user's own tmux server. `remain-on-exit` keeps exit status.
- **Status** — session: `running | exited(0) | failed(n)`. Claude sessions additionally:
  `working | waiting-input | question | done`, reported by hooks injected via `--settings` that write
  `state/<session>.json`.
- **Item history** — per-item log of actions run, when, outcome (JSON-lines in state dir), viewable in a
  detail panel.
- **Badges** — extra per-row status icons from provider fields: CI failed/running/passed, threads open,
  approvals, draft, conflicts.

## UI

```
 lanes   [review] mine  drafts                         ⟳ 12s ago
 ─────────────────────────────────────────────────────────────────
 ▸ ● !1234  ABC-901 Fix result import      alex   ✗ci  💬2   ◐ review: working
   ○ !1229  ABC-888 Speed up search        sam    ✓ci        
 ─────────────────────────────────────────────────────────────────
 preview / history / sessions panel (DONE: `p`, `i`, `S`)
 : command line      / filter      ? help
```

- Vim keys: `h j k l`, `gg G`, `ctrl-d/u`, `space` mark, `V` mark all, `esc` clear.
- `/` fuzzy filter, `:` command line, `?` help, `enter` open session/jump in, `o` open URL, `x` kill session
  (confirm), `R` refresh, `i` item history, `n` jump to the next session asking/waiting (asking first, oldest
  first, cycles), `S` sessions screen (every session, also of items no longer listed; jump / close), `q` quit.
- Live: background poll (default 30s) diffs into the list, preserving cursor + marks. First paint from disk
  cache (stale-while-revalidate).
- Jump into a session: `tmux display-popup` / `switch-client` when inside tmux, `attach` otherwise.

## Config (`~/.config/lanes/config.toml`, hot reloaded)

```toml
[settings]
poll_interval = "30s"

[provider.forge]
type = "auto"   # github for a github.com origin, else gitlab; or type = "gitlab" / "github"

[[view]]
name = "review"
provider = "forge"
filter = '"waiting-for-review" in labels && !draft && !approved_by_me && !open_thread_by_me && author != me'

[[view]]
name = "mine"
provider = "forge"
filter = 'author == me && "waiting-for-review" in labels && !approved && !("to-be-deployed" in labels)'

[keymap]
r = "review"
w = "work-on-comments"
s = "sync"

[action.review]
type = "session"
run = 'claude "/local-code-review {{.url}}"'
confirm = "none"
```

## Layout

```
.user/lanes/
  PLAN.md  Makefile  go.mod  main.go
  internal/
    app/        Name const, paths (config/state/cache dirs)
    config/     TOML load + validation + hot reload
    item/       Item type, fields
    provider/   Provider iface; gitlab, github + github-issues (graphql), linear (http), auto (forge from origin), command
    filter/     expr env + helpers (has_label, approved_by_me, …)
    session/    tmux backend, status, claude hooks
    history/    per-item log
    notes/      per-item notes (<state>/notes.json)
    snooze/     snoozed items, until a time or a changed fingerprint (<state>/snoozes.json)
    ui/         bubbletea models: root, list, preview, cmdline, confirm, help; theme
```

## Phases

1. **Core — DONE.** Skeleton, config (+hot reload), gitlab provider, filter engine, live list view, vim nav,
   marks, `/` filter, cache. GitLab fetch is two-pass: quick (~2s, no thread authorship; `Item.Pending` fields
   inherited from previous snapshot) then full (~9s, discussions), because GraphQL discussions are slow.
2. **Sessions — DONE.** tmux backend on its own socket (`tmux -L lanes`, config in state dir with
   remain-on-exit), run/jump (popup inside tmux, attach outside)/kill+confirm, status badges, Claude hooks
   (`lanes hook <event>` via `claude --settings {{.settings}}`: working / asking / waiting).
3. **Actions & config — DONE.** `[keymap]` + `[action.*]` (`session` and headless `run` types, `vars`,
   `refresh`), multi-select confirm, item history (`i`), in-TUI filter editing (`F`, `:filter`, `:newview`,
   `:delview` — stored in `<state>/views.json`, config untouched; `:filter save` writes an edit back into the
   defining config's `filter` line), `:edit` / `:config` / `:run`, live config reload.
4. **Polish — DONE.** `[theme]` color overrides, preview pane (`p`), CI/thread/approval badges, header session
   summary, desktop notifications (claude asking/waiting, session finished/failed; `no_notify` to disable).
5. **Extract — DONE.** Own repo, per-project config (`.lanes.toml` overlays the global config) and per-project
   state/tmux socket, MIT license, README, CI.
6. **Attention key — DONE.** `n` jumps to the next session that is asking or waiting.
7. **Notes — DONE.** Per-item notes (`N` in `$EDITOR`, `:note`), stored in `<state>/notes.json` keyed by item ID.
   The UI copies `note` into an item's fields when it builds the views (never into the provider data or its
   cache), so filters and templates see it; rows get a blue `✎` after the author, the preview shows the text.
8. **Claude usage — DONE.** `[settings] claude_dir` shows that profile's usage at the left of the footer
   (`claude usage: …`), read from `<claude_dir>/usage-line.json` (written by a Claude Code mod; text shown
   verbatim, with its age after 10 minutes).
9. **Scoped actions and tickets — DONE.** `[action.<name>] when = '<expr>'` restricts an action to the items it
   matches (key, `:run`, help screen); gitlab/github items get `kind = "pr"`, and the `github-issues` provider
   lists a repo's open issues as `kind = "ticket"` items (ticket field set shared with the Linear provider).
10. **Linear — DONE.** A `linear` provider lists a team's open issues with the same ticket fields, via the Linear
    GraphQL API over `net/http` (API key from `token_env`, default `LINEAR_API_KEY`; `lanes doctor` checks it is set).

## Later

Attention key (jump to next session needing input), headless CLI (`lanes run`, `lanes ls --json`; `lanes status` for status bars exists), more
providers.
