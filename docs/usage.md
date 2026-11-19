# Using lanes

## Views

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
edited inside the app shows a `*` in the tab bar. The names `queue` and `sessions` are reserved for the built-in
[queue view](actions.md#running-on-many-items) and [sessions view](actions.md#sessions-jump-close-status): a view with either name fails
the config load.

To group tabs, set `divider = true` on a view: a dim `¦` is drawn before its tab. A divider is not a view, so
`h`/`l`, `1`–`9` and `:view` skip it and the numbering is unchanged. The built-in tabs come last, the
[queue tab](actions.md#running-on-many-items) and then the [sessions tab](actions.md#sessions-jump-close-status), set apart from the
views as one group by a divider before the queue tab; there is no setting for that. `1`–`9` address the configured
views only; `Q` and `S` reach the built-in tabs.

```toml
[[view]]
name = "mine"
provider = "gitlab"
divider = true
```

## Filters

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

**Debugging a filter:** `lanes filter test '<expr>'` runs an expression against the cached items of one view's
provider (the first view, or `--view <name>`) and prints `match` or `no match` for each item. The view's own filter is
not applied, so every cached item shows up. For a non-match it names the failed top-level operand: for `&&` the
operands that are false, for `||` all of them, for `!` its operand. Deeper nesting is not split further. It reads only
the cache (start `lanes` once to fill it), never fetches, and exits 2 when the expression does not compile.

```
$ lanes filter test '"waiting-for-review" in labels && !draft' --view review
match  !12 Add retry to the uploader
no match  !15 WIP: new parser
  failed: !draft
1 of 2 items match
```

**Fields** are the item's own (`id`, `title`, `url`, `author`, `draft`), your [note](#preview-pane-and-item-history)
(`note`, empty when there is none, e.g. `note != ""`), whether you [snoozed](#snoozing) it (`snoozed`), whether it changed since you last saw it
(`unread`, see [Row badges](#row-badges)), where it sits in a stack of MRs/PRs (`base`, `stack_pos`, `stack_size`,
`stacked_by`, see [Stacks](#stacks)) plus whatever the provider adds. The `fields` section at the
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
`enter`. The change is saved *outside* your config (see [Where things live](configuration.md#where-things-live)), so your
hand-written TOML is never rewritten, and shows as `*` in the tab bar. `:filter reset` goes back to the
config's filter. The one exception is `:filter save`: it writes the edit into the config file that defines the
view (`.lanes.toml` when the project defines it, else the global config), changing only that view's `filter`
line, and drops the `*`. A filter written across several lines can't be saved this way; edit the file by hand.

### Stacks

lanes relates the items of one provider's list by their branches: an item whose `target` is another item's `branch` is
stacked on it (both set and non-empty; an item whose `branch` equals its own `target`, such as a fork PR from its
`main` into `main`, is nobody's base). This works for GitLab MRs, GitHub PRs and `command` items that set `branch` and
`target`; it needs no extra API calls. Only open items are listed, so once a base is merged, the items on it lose
their `base`. The whole provider list counts, so a base hidden by the view's filter or snoozed still makes its items
stacked. Branches match by name only: two items from different forks with the same branch name can pair up. A provider
field of the same name wins.

| Field        | Type   | Meaning                                                                        |
|--------------|--------|--------------------------------------------------------------------------------|
| `base`       | string | the id of the item it is stacked on, `""` when none                            |
| `stack_pos`  | number | 1 for the bottom of a stack, 2 for an item on it, …; `0` when in no stack      |
| `stack_size` | number | how many items the stack has; `0` when in no stack                             |
| `stacked_by` | list   | the ids stacked directly on it                                                 |

Every item has these fields (empty or `0` outside a stack, also on tickets), so filters, sorts and groups can use
them, e.g. `base == ""` for items that can merge now or `sort = "stack_pos"`. They don't make an item unread.

## Sorting

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

## Groups

A view can split its items into named groups, each with a filter (the same expressions as view filters). An item
goes into the first group it matches; items matching no group are listed below the groups without a header.

```toml
[[view]]
name = "triage"
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

## Navigation and selection

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
| `Q`                 | switch to the [queue view](actions.md#running-on-many-items) (the tab before the sessions tab) |
| `x`                 | close the item's sessions (asks first); in the queue view: drop the entry from the queue; in the sessions view: close the marked sessions or the one under the cursor |
| `r`                 | in the queue view: start the entry (all marked entries, if any are marked) now, past the [session limit](actions.md#running-on-many-items); elsewhere `r` is free for an action key |
| `X`                 | interrupt the item's running sessions and keep their output (asks first; again: `SIGKILL`); in the sessions view: of the marked sessions or the one under the cursor |
| `t`                 | open a shell for the item and jump into it (see [Shell sessions](actions.md#shell-sessions)) |
| `gr`                | resume the item's newest finished Claude session (see [Resuming a Claude session](claude.md#resuming-a-claude-session)) |
| `i`                 | show the item's history                            |
| `d`                 | item details: every field untruncated, description, pipeline, threads and comments (scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; `J`/`K` move between threads, `n`/`N` jump to the next/previous open thread, `enter`/`space` fold the one under the cursor, `za`/`zA` fold/unfold all, `o` opens it in the browser; close with `q`/`esc`) |
| `N`                 | edit the item's note in `$VISUAL` / `$EDITOR` (save it empty to delete it) |
| `m`                 | mark the item unread (all marked items, if any are marked); when all of them are unread: mark them read (see [Row badges](#row-badges)) |
| `z`                 | [snooze](#snoozing) the item (all marked items, if any are marked); on a snoozed item: unsnooze it |
| `n`                 | jump into the next session that is asking or waiting (any view) |
| `S`                 | switch to the [sessions view](actions.md#sessions-jump-close-status) (the last tab): every session, also of items no longer listed |
| `A`                 | [agent view](claude.md#agent-view) of the item's newest Claude session |
| `p`                 | toggle the preview pane                            |
| `F`                 | edit the current view's filter                     |
| `R`                 | refresh data now                                   |
| `ctrl-r`            | reload the config now                              |
| `?`                 | help: keys, actions, commands, config paths and the fields per provider (scroll with `j`/`k`, `ctrl-d`/`ctrl-u`, `g`/`G`; close with `q`) |
| `q`                 | quit after confirming — sessions keep running      |

Any other key you bind in `[keymap]` runs its [action](actions.md#actions). Built-in keys cannot be rebound; if you try,
the help marks the binding as shadowed and the reload message tells you. `X` became a built-in key with
interrupt, and `Q` for the queue view; an older `[keymap]` binding of either is now shadowed.

### Jumping in after a tmux upgrade

Sessions run on lanes' own tmux server (`tmux -L lanes-<project>`), started by the tmux that was installed back
then. After a tmux upgrade the new `tmux` client may fail to attach to that older server (`open terminal failed:
not a terminal`): the session flashes up and you are back in lanes. So before every jump (`enter`, `n`, the
sessions view, an action with `jump = true`) lanes compares the installed `tmux -V` with the version of the
running server. On a mismatch it asks first:

```
lanes tmux server runs 3.5a, installed tmux is 3.8: restart it, closing 2 session(s): review~_1948, shell~_1951?
```

`y` / `enter` kills the server and with it **every** session of the project (running commands stop, their output
is gone; history records them as `killed`), then re-polls; jump in again afterwards, now on a server of the new
version. Any other key cancels and changes nothing. If the version check itself fails, lanes warns in the footer
and attaches anyway; an attach that fails shows its error in the footer, a normal detach does not.

To restart the server by hand instead (same effect, e.g. while lanes is not running):
`tmux -L lanes-<project> kill-server`.

## Searching

Ticket rows end with a priority strip after the assignees: `▰▰▰` urgent (red), `▰▰▱` high (orange), `▰▱▱` medium
(yellow), `▱▱▱` low (dim); none for priority `0`.

`/` opens an incremental search over the current view: it matches id, title, author, ticket and (for tickets) assignees, case
insensitively, as you type. `enter` keeps the search active (shown in the footer), `esc` clears it. The search
narrows the view; it does not change its filter.

## The command line

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
| `:auto [on\|off]`         | pause / resume the views' [`on_enter`](actions.md#running-on-arrival-on_enter) actions; bare: show their state |
| `:resume`                 | resume the selected item's newest finished Claude session (same as `gr`) |
| `:sessions`               | switch to the sessions view (same as `S`)                             |
| `:prune`                  | kill every finished session; running ones are never touched           |
| `:prune failed`           | kill only finished sessions that exited non-zero                      |
| `:refresh`                | refetch now                                                           |
| `:reload`                 | re-read the config now                                                |
| `:config`                 | print the global and project config paths                             |
| `:edit`                   | open the global config in `$VISUAL` / `$EDITOR`                       |
| `:edit project`           | open the project config, creating `.lanes.toml` if missing            |
| `:quit`                   | quit                                                                  |

## Row badges

Each row ends with small status badges: the item's own (from the provider) followed by up to three of its
sessions. A blue `✎` after the name (the author, or the assignees on tickets), or after the title when there is no name, is not a badge: it marks an
item you left a [note](#preview-pane-and-item-history) on.

A **bold title** marks an unread item: it is new, or its title, draft state, badges or provider fields changed since
you last saw it. The cursor resting on it for a second marks it read; `m` toggles it by hand. Filters see `unread`
(`true` / `false`). On the first run every listed item counts as read. Seen items are stored per project
(`seen.json`, a corrupt file moves to `seen.json.bak` like notes).

A **dimmed title** followed by `↑ !12` marks an MR/PR [stacked](#stacks) on `!12`, which is not merged yet; drafts
are dimmed too.

| Badge          | Meaning                                                       |
|----------------|---------------------------------------------------------------|
| `draft`        | MR is a draft                                                 |
| `✓ ci` `✗ ci` `◌ ci` | pipeline passed / failed / running or pending           |
| `↓ behind`     | source branch is behind its target branch                     |
| `⚠ conflict`   | merge conflicts                                               |
| `⚑ 3`          | three unresolved discussions                                  |
| `✔ approved`   | has approvals                                                 |
| `▶ review`     | the `review` action is running in a session                   |
| `queued review` | a start of `review` waits in the [queue](actions.md#running-on-many-items) for a free session |
| `◐ review: working`  | a Claude session is working                             |
| `? review: asking`   | a Claude session asks a question / needs permission     |
| `● review: waiting`  | a Claude session finished its turn and waits for you    |
| `◐ review: background` | a Claude session finished its turn while a background job still runs (see below) |
| `✓ review`     | the session's command exited with status 0                    |
| `✗ review (3)` | the session's command exited with status 3                    |

`background` is its own state, not one that needs you: it is not counted in the top-right summary's `?` / `●`, not
offered by `n`, not announced by a desktop notification and not in `lanes status`. lanes spots a launch from the
`run_in_background` Bash option or a background subagent and clears it on the next prompt or a finished subagent.
Limit: `background` can show a little too long when a job ends without Claude reacting to it yet; a Claude version
that renames that option falls back to `waiting`.

The top right of the screen summarizes live sessions (`◐ 2  ? 1  ● 1`) and when the list was last refreshed.

## Preview pane and item history

`p` toggles a pane under the list with the selected item's title, URL, author, your note, the provider's fields
(the identifier — `iid`, `number` or `key` — then branch, labels, reviewers, CI, …), its sessions and its last run.
An MR/PR in a [stack](#stacks) gets its `base`, `stack_pos`, `stack_size` and `stacked_by` lines after the target
branch; items in no stack get none.
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
PRs and issues, Linear and a `command` provider with a [`detail`](providers.md#command) command, pressing `d` also
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

## Snoozing

`z` puts the selected item (or all marked items) aside: it opens a prompt (`:snooze`) that takes a duration
(`30m`, `2h`, `3d`), a weekday (`mon`, `friday`: its next occurrence, at midnight) or a date (`2026-11-03`).
Empty input snoozes **until the item changes**: its title, draft state, badges or provider fields differ on a later
fetch (your note does not count). Invalid input or a time in the past shows an error and snoozes nothing.

A snoozed item drops out of every view and its tab count; the footer shows how many items of the current view are
snoozed. `:snoozed` shows them again, dimmed and marked `z`, and `z` on one of them unsnoozes it. Filters and
action templates see `snoozed` (`true` / `false`), which is useful with `:snoozed` on. Snoozes are stored per
project (`snoozes.json`, a corrupt file moves to `snoozes.json.bak` like notes) and removed once they wake.
