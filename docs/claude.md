# Claude Code integration

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

## Resuming a Claude session

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

## Usage in the footer

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


## Agent view

`A` on an item opens a full-screen, animated view of its newest Claude session; on the sessions tab, of the
selected session (closing it returns to the tab). `esc`, `q` or `A` go back. It needs the [hooks](#claude-code-integration)
(`--settings {{.settings}}`): they record where Claude keeps the session's transcript. Without them, or before
Claude has started, the footer says `no Claude session for this item`.

The view is a dashboard across the whole terminal, with one layout at every size: it never hides or replaces
anything because the terminal is small. It shows, top to bottom:

- **header** — `AGENT VIEW · <title>`: the item's title, or the session's name when the view was opened from the
  sessions tab (or the title is unknown); under its rule a **legend** of the model family colors
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

Each of the three scrolls with a scrollbar (see [Quick start](../README.md#quick-start)): the diagram's sits in the last column of its viewport, plus
a horizontal one on its last row when the diagram is wider than the viewport (the viewport is one column or row
smaller while they show); the log's and the session column's sit inside their border, next to it. The bar of the
focused area is drawn in the accent.

**Look.** The view has its own neon palette: opus magenta, sonnet cyan, haiku yellow, a violet header, heavy box
borders with a double one around the lead, and bars that light up over a deep track. Light terminals get a
toned-down variant of the same hues. Change any of its colors under
[`[theme.agent_view]`](configuration.md#agent-view-colors); the app's `[theme]` colors don't apply here. Labels are dim and
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
