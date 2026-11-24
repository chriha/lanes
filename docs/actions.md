# Actions

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
| `when`    | none      | [filter expression](usage.md#filters); the action runs only on items it matches (see [below](#restricting-actions-when)) |
| `jump`    | `false`   | `session` type only: on a single item, attach to the session once it has started (see [below](#session-actions)) |
| `on_success` | none   | action to start on the same item when this one exits 0 (see [below](#chaining-actions-on_success-on_failure)) |
| `on_failure` | none   | action to start on the same item when this one exits non-zero                        |

## Session actions

The command runs in a **detached tmux session** using **your `$SHELL`** (`$SHELL -ic '<command>'`, so your
aliases and PATH apply). The session appears as a badge on the item. Press `enter` on the item to jump in; you
get the full interactive terminal. If the command is interactive (for example `claude`), you can answer its
questions there.

As soon as you press the key, each item shows a `⋯ <action>` badge and the footer says
`starting <action> on <ids>…`. Until tmux has started the session, the item is **busy**: any action on it is
skipped (`!12 busy (review)`). In a multi-item run the other items still run and the result adds `, N busy`. The
lock ends once the session has started, so a long-running session does not block other actions on the item.

If an action is already running for an item it is not started again (you are told so). If its previous session
has ended, it is replaced by a fresh one. lanes checks again when tmux reports the session as existing although
its last look at the sessions did not show it: a finished session is still restarted, a running one is left alone
(and jumped to, see `jump`).

With `jump = true`, running the action on a single item also takes you into its session, exactly like `enter`:
lanes attaches once tmux has started it, or attaches to the running one instead of saying it is already running
(a finished one is restarted first). On several marked items it starts them all and jumps into none. `jump` on a
`type = "run"` action is a config error.

When the command exits the session stays around (tmux `remain-on-exit`), showing `✓` or `✗ (exit code)`, so you
can still read the output. Stop a running command with `X` (the session stays with its output and shows
`✗ (143)`), close it with `x` (it asks first).

## Shell sessions

`t` opens a plain shell for the item under the cursor: your `$SHELL`, interactive, in the directory you started
`lanes` in, as the session `shell~<item id>`. It works with no config and jumps right in. Marks are ignored. If the
item already has a live shell, `t` jumps into it; if the shell has ended (`exit`), it shows as `✓ shell` (or
`✗ shell (n)`) and the next `t` restarts it. Close it with `x` and remove finished ones with `:prune`, like any
other session. `shell` is a reserved action name: a config action called `shell` fails to load.

## Run actions

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

## Templates

`run`, `resume`, `cwd` are [Go templates](https://pkg.go.dev/text/template). Available fields:

| Field                           | Value                                                           |
|---------------------------------|-----------------------------------------------------------------|
| `{{.id}}` `{{.title}}` `{{.url}}` `{{.author}}` | the item's own fields                           |
| `{{.iid}}` `{{.branch}}` `{{.ticket}}` `{{.me}}` … | provider fields (see [Filters](usage.md#filters))   |
| `{{.note}}`                     | your note on the item (empty when there is none)                |
| `{{.snoozed}}`                  | whether the item is [snoozed](usage.md#snoozing)                        |
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

## Running on many items

Mark items with `space` (or `V` for all), then press the action key. Because this is easy to do by accident,
multi-item runs always ask first:

```text
Run "review" on 3 items (!1948, !1957, !1961)?  y/enter yes · n/esc no
```

Each item gets its own session; items that already have the action running are skipped.

**Session limit.** `max_claude_sessions` under `[settings]` (default `3`, `0` = no limit) caps how many live
sessions lanes' actions run at once; starts beyond it wait in a queue. Every live session lanes started counts, a
[shell](#shell-sessions) included, whatever its command; a Claude session that is `waiting` (finished its turn,
idle at the prompt), finished (dead) sessions and [run actions](#run-actions) do not. A session that is `background` (turn ended, a background job still runs) counts, since work is
running. Resuming a waiting session
makes it `working` and it counts again, so the count can end above the limit; nothing is stopped. Restarting a
finished session counts as a new one.

When a start (an action key, `:run`, `gr` / `:resume`) would take the count above the limit, lanes does not ask:
the items that fit start, the rest wait in the queue in item order. Their rows show `queued <action>` and the
footer `queued N`. The multi-item prompt (and `confirm = "single"`) still asks first, as without a limit.

- **Draining.** The queue starts its next entry, first in first out together with
  [`on_enter`](#running-on-arrival-on_enter) starts, when a session ends or goes `waiting`, or a fetch lands, and
  there is room again. Queued starts you made by hand drain under `:auto off` too. They start unattended: no jump
  into the session, your marks left alone, and the footer says `started queued review on !1961`.
- **Checked again.** Before it starts, a queued action checks its `when` once more; an entry that no longer
  matches, or whose item is busy or already runs the action, is dropped with a notice. An entry you queued stays
  when its item leaves the view: lanes starts it on the item as last listed.
- **Once per item.** An item waits in the queue at most once; starting it again says `already queued` and does
  nothing.
- **Queue view.** `Q` opens the [queue view](#running-on-arrival-on_enter): `r` starts the entry under the cursor
  (or the marked ones) now, past the limit; `x` drops it without starting anything. In every other view `x` drops
  the queued starts of the marked or cursor items too: with only queued starts it drops them at once, with
  sessions as well one prompt (`Close 2 session(s) and drop 1 from the queue?`) covers both.
- **Memory only.** The queue is lost on quit; start the action again after a restart.

`t` never waits: a shell starts at once, whatever the count. [Chained](#chaining-actions-on_success-on_failure)
follow-ups and [run actions](#run-actions) never queue. A negative value fails the config load.

## Restricting actions (`when`)

Every action can run on every item unless you restrict it. `when` takes a [filter expression](usage.md#filters),
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

## Chaining actions (`on_success`, `on_failure`)

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

## Running on arrival (`on_enter`)

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
  starts the next entry when a session ends or goes `waiting`, or a fetch lands, and there is room again, checking `when` once more;
  an entry whose item left the view is dropped. The queue lives in memory only: on quit it is lost, and the
  items stay handled; run the action by hand.
- **Queue view.** The tab before the sessions tab, `queue`, lists the queue: one row per entry, oldest first, `on_enter` starts and
  [starts you queued by hand](#running-on-many-items) alike, each the normal item row with `queued <action>`.
  The built-in tabs (`queue`, then `sessions`) are set apart from the views as a group by one `¦`, none between them. `Q` jumps to it from anywhere (`1`-`9` address the configured views only). The tab is muted, and shows a count only while entries wait; it is always there, and an empty queue shows an empty-state line.
  Navigation, marks, search, `o`, `y`, `enter` and `d` work as in other views. `r` starts the entry under the
  cursor (or the marked ones) now, past the limit, without asking or jumping, and takes it off the queue; an
  entry whose item is busy, already runs the action or no longer matches its `when` is reported and dropped. In
  this view `r` wins over an action bound to `r`. `x` drops the entry under the cursor (or the marked ones) from
  the queue, without asking and without touching any session; a dropped `on_enter` item stays handled and is not
  queued again. (In the other views `x` also drops queued starts, see above.) Action keys and `:run` do nothing there. The list follows the queue live. It is built in: not in
  the config, and `:newview` / `:delview` do not touch it.
- **Failures are not retried.** An auto-start that fails, or whose command exits non-zero, is recorded in the item
  history like any run; later fetches do not start it again. Run it by hand to retry.
- **Footer.** Each auto-start is reported, e.g. `auto-started review on !1961` or `auto-started review on 3 items`;
  for run actions and failed starts the usual notice is prefixed with `auto:`.
- **Only full fetches count.** A fetch that fails or returns a partial list never makes items look as if they left
  or arrived.
- **Pause.** `:auto off` pauses every view's `on_enter`: nothing starts and its queued entries stay put; starts
  you queued by hand keep draining. Items that arrive meanwhile count as seen and do not fire after `:auto on`,
  which resumes the `on_enter` entries.
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

## Sessions: jump, close, status

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
- **Sessions view** — the last tab, `sessions`, after the [queue tab](#running-on-many-items), lists every
  session on the `lanes` tmux server, newest first: item ID and title (or the item key when no provider returns
  the item any more), status badge and age. Sessions whose item is in no view — filtered out, or merged/closed —
  are marked `(not in any view)`. `S` (or `:sessions`) switches to it from anywhere; `h`/`l` reach it like any
  tab. Its label is `sessions <n>`, n counting every session, plus the yellow count of sessions asking or
  waiting; both update with the session refresh. With no sessions it shows an empty-state line and stays
  reachable. It is built in: not configurable, and `:newview` / `:delview` do not touch it.
  - `enter` jumps in, `A` opens the session's [agent view](claude.md#agent-view) (closing it returns here), `j`/`k`,
    `gg`/`G`, `ctrl-d`/`ctrl-u` move.
  - `space` marks a session (marks follow the session, survive the refresh and drop when it ends); `x` closes
    and `X` interrupts the marked sessions, or the one under the cursor, with the same confirmations as above.
  - `/` filters the rows by label; `esc` clears the marks and the search. The cursor stays on its session
    across refreshes.
  - `p` toggles a preview of the session: name, action, state, age, working directory and its last output
    lines, plus the item's title and URL when a view lists the item; orphan sessions get one too.
  - Item keys (`o`, `y`, `N`, `t`, …) and configured action keys do nothing here.
  - `q` quits (after confirming) as on any tab; there is no separate screen to go back from.
- **Resume** — `gr` (or `:resume`) starts the item's newest finished Claude session again, see
  [Resuming a Claude session](claude.md#resuming-a-claude-session).
- **Quit** — `q` asks first (`y`/`enter` quits, any other key cancels); `ctrl-c` and `:quit` leave at once. Sessions keep running and reappear when you start it again.
- **Status** — shown as row badges and in the header summary, refreshed every second.


## Notifications

`lanes` raises a desktop notification (macOS: `osascript`, Linux: `notify-send` if installed) when a session
starts asking, starts waiting, finishes or fails. Disable with `no_notify = true` under `[settings]`.
