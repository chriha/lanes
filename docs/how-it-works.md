# How it works

- **Data flow.** On start `lanes` paints the cached list, then fetches each provider in the background and
  re-fetches every `poll_interval`. While the terminal is unfocused it re-fetches only every `poll_interval_blurred`
  (default 5m, so desktop notifications lag by up to that much), and when it regains focus every provider older than
  `poll_interval` is fetched at once. Blurred, the tmux session poll, the usage reads and the config check slow to every 10s (the session preview stops), and focus polls them at once. This needs a terminal that reports focus; inside tmux add `set -g focus-events on`
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
  list of sessions is simply asked from tmux every three seconds (every second while the sessions tab or the agent view is open). This also means sessions survive quitting or
  crashing `lanes`.
- **Claude status** is written by hooks to a small file per session in the state directory; `lanes` reads those
  files together with the tmux pane state. The state is ignored once only a shell is left in the pane (Claude
  quit or crashed), so the session counts as running again. When Claude ends its turn while a background job (Bash
  `run_in_background`, a background subagent) is still running, the `Stop` hook writes `background` instead of
  `waiting`: a `PreToolUse` hook records the launch in the state file, and the next prompt or `SubagentStop`
  clears it. `background` is not counted as needing you. It can show a little too long when a job ends before Claude
  acts on it.
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
