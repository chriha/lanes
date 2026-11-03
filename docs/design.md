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
  on its first fetch after start or a config reload (in the background, so building it runs no `git`): `github` for a `github.com` origin, `gitlab` for any other host, so the global default config works in both kinds of
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
 ▌ !1234  ABC-901 Fix result import      alex   ✗ci  💬2   ◐ review: working
   !1229  ABC-888 Speed up search        sam    ✓ci        
 ─────────────────────────────────────────────────────────────────
 preview / history / sessions panel (DONE: `p`, `i`, `S`)
 : command line      / filter      ? help
```

- Vim keys: `h j k l`, `gg G`, `ctrl-d/u`, `space` mark, `V` mark all, `esc` clear.
- `/` fuzzy filter, `:` command line, `?` help, `enter` open session/jump in, `o` open URL, `x` close session
  (always confirms), `X` interrupt the session's command and keep its output (confirm; again: SIGKILL), `R` refresh, `i` item history, `n` jump to the next session asking/waiting (asking first, oldest
  first, cycles), `S` sessions screen (every session, also of items no longer listed; jump / close), `A` agent view
  (below), `q` quit.
- Live: background poll (default 30s) diffs into the list, preserving cursor + marks. First paint from disk
  cache (stale-while-revalidate). Terminal focus reporting (`tea.WithReportFocus`) slows the poll to
  `poll_interval_blurred` (default 5m) while blurred; focus-in fetches every provider older than `poll_interval`.
  lanes starts focused, so terminals without focus events poll as before.
- Fetch status: per provider (`fetchState` in `internal/ui/fetchstate.go`, changed only on the UI goroutine) —
  cached / fetching with spinner and elapsed / failed / updated, plus the last fetch duration, at the right of the
  header (only the current view's provider when it does not fit) and in place of an empty view's list while it
  fetches. The text is computed in `View` from the start times. The spinner is a fixed-width scanner (`[ ●   ]`, a
  dot sweeping back and forth, `scanFrames`) at one frame per 100 ms of the fetch; a `spinTickMsg` loop redraws it.
  `Update` starts that loop (`armSpin`) when a fetch runs and none does, and the loop ends at the first tick that
  finds every provider idle, so at most one runs and none while idle. The 1 s tick advances the elapsed text.
- Reload: trust check, `config.Load` and provider construction run in a `tea.Cmd`; the old config stays active
  until its `reloadedMsg` lands, which cancels the old config's in-flight fetches (their results are dropped by
  fetch generation) and fetches every provider at once.
- Jump into a session: `tmux display-popup` / `switch-client` when inside tmux, `attach` otherwise.
- **Agent view** (`A` on an item or on the sessions screen) — a full-screen animated diagram of a Claude
  session as a dense dashboard: a full-height session column as the lead's dashboard (context bar, effort meter,
  tool-call sparkline over 15 min, hook states on a `┆` timeline, counters incl. the token split, the lead's 5h share and agent counts,
  last 4 files touched, last text, advisor), lead box with the tools box right of it, layered agent tree, session log and a status line
  with one colored entry per model family plus totals (an ended session reports no running agents). Agent boxes
  show full name, model, effort meter, ctx bar, a usage row, task, live text and status (`▸ tool arg` while running). The box is
  7 rows (`boxH`). The usage row spreads the agent's tokens (`Agent.Tokens`: input + cache write + output, counted like the
  lead's), tool calls (`Agent.Calls`), 5h share and, while it runs processes, their CPU share (`cpu N%`, `State.cpu`) over the row
  (`fmtCount`; the units go when it doesn't fit, then the 5h share). The status line adds `cpu [N%]` for the whole tree
  once a process has a CPU share. The 5h
  share (`State.share5h`) is the account's 5h % (`session.Usage5h`: `session` of `usage-line.json`, unknown when missing,
  unreadable, without `session` or older than 15 min) x the actor's tokens / the tokens of the lead and all agents,
  rounded; 0 when they have none; left out when unknown. It assumes this session drives the account's usage. The lead's
  share uses its tokens as read, not the easing counter. `internal/ui/model.go` reads it on the 1 s tick and
  `agents.go` passes it to the open view as a `flowview.UsageEvent`; a frozen view keeps its last figure once it has one. The task (`session.Agent.Task`) is the subagent's `meta.json` description, else the first sentence of its
  first prompt (a leading `Read and follow …` line skipped), ≤ 40 chars; the live text (`Agent.Text`, drawn as
  `» text`) is the first line of its newest assistant text, ≤ 60 chars, dim once the agent is done or failed. One layout at every size (no compact
  or too-small mode): a pinned header (`AGENT VIEW · <item title>`, the session name as fallback; no live/ended
  state, the status line has it) with the family legend under its rule, session column, session log and status
  line, and between them a viewport onto the diagram (lead with its effort and the tools box right of it, agent tree). The diagram is
  drawn whole on its own canvas and the visible window is blitted in; `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G` and
  `h`/`l` move the window, clamped at the edges, kept across snapshots, shown as `lines a-b/n  cols a-b/n` above
  the log. `tab` / `shift+tab` cycle the focus session column → diagram → session log (the diagram on open); the
  focused area has its border / title in the accent and takes the scroll keys, so the others stay put. The session
  log is a closed square box of 4 lines (`time actor message`, the actor in its family color, failed calls in the
  bad color) holding the newest 200 lines: it follows the newest line until scrolled up, then holds its place
  while lines append and shows `n/total` in its top border; `G` follows again. Boxes are 28–30 cols (widened for long names, never narrower), as many per row as fit, every agent drawn
  (a depth wraps into as many lines as it needs); a viewport narrower than a box scrolls sideways. Every bar row (the ctx row of agent
  boxes, the lead's ctx row in the session column, the tools box's rows) reads `LABEL BAR XX%`: the label left, the % right-aligned
  in 4 cells, and the bar (`▬` for ctx, `■` for tools) stretched over every cell between them, one space on each side. One cell is one
  step, so the share per step is 100 / cells and differs per row; filled cells = share x cells rounded to nearest, at least 1 above 0. The view has its own neon palette
  (`flowview.NeonPalette`: magenta/cyan/yellow families, violet accent header, a deep track behind bars, heavy
  panel borders and a double lead border; a toned-down light variant), overridden per field by
  `[theme.agent_view]` (also `[theme.light.agent_view]` / `[theme.dark.agent_view]`, resolved like `[theme]` in
  `internal/ui/theme.go`). Packets stream continuously: parent → child along a running agent's edge (~1.2 s,
  jittered per edge, ≤ 3 in flight), a burst of 3 child → parent on finish/fail, none when ended. Lives in `internal/ui/flowview`: a character-grid canvas with
  run-length styled output, a pure `State` changed by events (`Apply`) and stepped by `dt` (`Step`), panel
  renderers and the tree layout; it has its own tick (~70 ms) that runs only while the view is open. Its data
  comes from `session.Tailer` (`internal/session/transcript.go`), which reads only the new bytes of the
  transcript the hooks recorded (`transcript_path` in the session's sidecar) and of `<session>/subagents/
  agent-<id>.{jsonl,meta.json}`; `internal/ui/agents.go` polls it in a `tea.Cmd` every 500 ms and hands the
  snapshot over as a message. Each snapshot carries the log events since the last poll (`Snapshot.Events`, each
  once): tool calls once their result is in (with a failure flag, at the call's time), the first line of each
  assistant text, and the spawns, finishes and failures it diffs against the last snapshot, for the lead and every
  agent, ordered by time; lines without a timestamp or a tool name give no event. The view adds the lead's hook
  state changes on `HookEvent`. Context % uses one model-id table (`session.ContextWindow`, 200k default, 1M for
  `[1m]`). Effort is the newest `effort` field per transcript (absent → the meter is omitted). An ended session is
  read once more and frozen.
  **Processes:** `session.Sampler` (`internal/session/procs.go`) runs `ps -A -o pid=,ppid=,time=,rss=,command=` and
  keeps the tree under the session's pane pid (`#{pane_pid}`); it parses both `time` formats (Linux `[dd-]hh:mm:ss`,
  macOS `mm:ss.ss`) and turns the CPU-time delta of two samples over the wall-time delta into a CPU % (`ps`'s
  `%cpu` is a lifetime average on Linux, a decaying one on macOS). A pid's first sample, or a reused pid, has no
  share; a vanished root, empty output or a `ps` error give an empty tree. `agents.go` samples every 3 s in a
  `tea.Cmd`, one sample in flight, only while the view is open and Claude runs, and hands it over as a
  `flowview.ProcsEvent`. `session.Attribute` maps each process to the agent whose running `Bash` call's command it
  holds as a whole word (the longest such command; its descendants go with it); the rest, and a command several
  agents run at once, go to the lead. It is best-effort: subagents share the claude process, and a command that
  forks away from its shell lands on the lead. The view draws the top 5 by CPU in the processes box right of the
  tools box, each agent's summed share in its usage row and the tree's total in the status line.

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

[theme.agent_view]   # the agent view's neon palette, per field; also under [theme.light] / [theme.dark]
opus = "#FF4FE3"
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
    session/    tmux backend, status, claude hooks, transcript tailer
    history/    per-item log
    notes/      per-item notes (<state>/notes.json)
    snooze/     snoozed items, until a time or a changed fingerprint (<state>/snoozes.json)
    quietterm/  mutes bubbletea's init-time colour query (TERM=dumb until main); termbg/ asks once, bounded
    ui/         bubbletea models: root, list, preview, cmdline, confirm, help; theme
      flowview/ agent view: canvas, state, animation, layered tree layout, panels
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
11. **Agent view — DONE.** `A` opens an animated, live diagram of a Claude session's agents (`internal/ui/flowview`),
    fed by an incremental transcript tailer (`internal/session/transcript.go`).

## Later

Attention key (jump to next session needing input), headless CLI (`lanes run`, `lanes ls --json`; `lanes status` for status bars exists), more
providers.
