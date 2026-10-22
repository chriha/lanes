# Implementer brief (/work-tickets)

Your prompt gives `#nnn`, `<repo>`, `<wt>`, `<branch>`, `<report>`, `<base>` (`main`, or the branch this
ticket is **stacked** on — then also `<base-wt>` and the base PR url), the PR url when continuing, and
`plan approved` when set. Follow `tickets.md` and `git-rules.md` next to this file.

- **Read the ticket:** `gh issue view nnn --json title,body,labels,comments` and the blockers
  (`tickets.md`, "Writing") — the description (current state, incl. `## Plan` when plan-approved) and
  comments. An old-format ticket is folded into the description on your first write (`tickets.md`, "Fold on touch").

## Gate first — before any setup or code change

Explore read-only. Locate code with CodeGraph when `<repo>/.codegraph/` exists, otherwise grep/read; read files in
`<wt>` if it exists (continuing), otherwise in `<base-wt>` when stacked, otherwise in
`<repo>/.user/worktrees/_main` (a detached checkout of `origin/main`). Never edit those.

1. **Readiness:** if you can't write the failing test yet, you don't understand the ticket → **needs-info**
   (below).
2. **Just solve it** when all hold: root cause known or quick to find; 1–3 files in one package; follows a
   pattern the codebase already has (a provider like its siblings, a missing guard, a copy fix); easy to
   revert (no config-format or on-disk cache/history format change, no change to a documented CLI flag, key
   binding or provider JSON contract); one test proves it; one reasonable approach (a real toss-up → pick one
   and move).
3. **Plan first** when any hold: unknown cause (investigate first — if it then turns out small, just solve
   it); blast radius (an exported function with many callers, a change to the `Provider` or `session`
   interfaces, a change in the Bubble Tea update/message flow); hard to undo (config schema, cache/history file
   format, the JSON contract of the `command` provider, session state kept in tmux); multiple real designs
   (new package vs. existing one, sync vs. background refresh); needs sequencing (several PRs, deprecation
   period); data loss involved (closing sessions, deleting files) → **planning** (below). No worktree, no branch.
4. **Plan approved** skips 2–3: implement the plan's next unshipped PR slice. If the plan turns out wrong in
   a way that needs a decision → **needs-info**.

## Worktree setup (only when implementing)

If `<wt>` doesn't exist (idempotent; continuing an existing branch is detected by itself):
```
<repo>/scripts/work-tickets/worktree.sh add issue-nnn <base> --branch <branch>
```
It fetches `origin/<base>` and adds the worktree (new branch from `origin/<base>`, or the existing branch). Go
needs no further setup; `bin/` is gitignored and built per worktree.

- **Work only inside `<wt>`.** `cd <wt>` before every command; read and edit only paths under `<wt>` (plus
  `<report>`).
- Never run `go get`, `go mod tidy` or edit `go.mod`/`go.sum` by hand — a dependency change is an open
  question. If `go.mod` changed after a `git merge origin/<base>`, that is fine; `go build` picks it up.
- Read `CLAUDE.md` if the repo has one and `docs/design.md` for what you touch. Follow sibling-file
  conventions.

## Implementing

- If mid-work a just-solve ticket trips a plan-first trigger or grows past ~400 changed lines (excluding
  tests and `go.sum`): discard **your own** uncommitted changes (`git restore <files>`, delete files you
  created). Not pushed yet → also `git -C <repo> worktree remove --force <wt>` and
  `git -C <repo> branch -D <branch>`. Then switch to planning.
- **One concern per PR.** A diff is either "no behaviour change" or "behaviour change + its test" — never
  both. Stay in scope: no "while I'm here" cleanups, no changed defaults, no loosened validation. Report
  anything else under `## Findings`.
- **Test first, and prove it can fail:** write the test, run it **red**, then fix, run it green. For bug
  fixes the test must fail with the fix reverted. No assertion-free tests, no asserting a fake's own return
  value, no faking the type under test. Use `t.TempDir()`, `t.Setenv`, table-driven cases; never touch the
  user's real tmux server, config dir or network.
- **Red check before push (behaviour changes):** commit, then `scripts/work-tickets/red-check.sh <wt> <base>`
  must exit 0 (red without the non-test diff, green with it; it restores the tree itself, no stash). Paste its
  output into `## Tests`. Exit 1 = test can't fail, 2 = not green with the fix, 3 = no test changes, 4 = setup
  error/dirty tree, 5 = red from a compile error, not a failing test: fix and rerun. "No behaviour change" PRs
  are exempt — say so in `## Tests`.
- **Checks** — same as CI, green before every commit: `make test` (`go vet ./...` + `go test ./...`), `gofmt -l .`
  prints nothing, and `go build ./...`. Fix new findings in the code — no `//nolint`, no skipped tests. There is
  no pre-commit hook: run `gofmt -l .` yourself and format only the files you changed (`gofmt -w <files>`).
- Self-check before committing: no invented functions or methods (verify they exist), no swallowed errors or
  `_ =` added to silence one, no helper duplicating an existing one (grep first), no goroutine without a way to
  stop it, no data race on state shared with the Bubble Tea model (go through messages), no shelling out with
  an unquoted/unvalidated string.
- **Commits:** Conventional Commits per `git-rules.md`, e.g. `fix: keep null placings out of the ranking`.
  Body: one or two lines on what and why, then `Refs #nnn` and
  `Co-Authored-By: Claude <noreply@anthropic.com>`. Stage by path only (never `git add -A`/`.`). One commit by
  default; each commit leaves tests green.
- **Review (once, before the push):** with every commit in and tests green, spawn one fresh
  sonnet sub-agent (a new agent, not a fork). Its prompt carries the ticket's acceptance criteria, the full
  `git -C <wt> diff origin/<base>...HEAD` and the commit log as author intent, and tells it: read-only (no
  edits, no commits), apply `.claude/review-criteria.md` (rules compliance + test quality, signal
  threshold, false-positive list), validate each candidate with a separate sonnet sub-agent per its
  "Validation", and reply with only validated findings, one per line as
  `- <file>:<line> — <problem; quote the rule if any> — <fix>`, or `none`.
  - **Bug review, in parallel:** in the same message spawn a second fresh sonnet sub-agent that runs
    `/code-review <level> <branch>` in `<wt>` (read-only, no `--fix`/`--comment`) and replies with its findings in
    the same line format, or `none`. Always pass `<level>` explicitly. Level from changed lines =
    insertions + deletions of `git -C <wt> diff --shortstat origin/<base>...HEAD -- . ':(exclude,glob)**/*_test.go' ':!go.sum'`
    (tests and `go.sum` excluded): **≤ 200 → `low`, > 200 → `medium`**. Never `high` or above.
  - Merge both reviewers' findings into one list and drop duplicates (same file:line or same problem) before fixing.
  - Fix each finding that is in scope, in one extra commit (rules above), then re-run `make test` and
    `gofmt -l .`. An out-of-scope finding goes under `## Findings`.
  - No second review round. If the fixes themselves need review, or the re-run is not green and not
    quickly fixable, list the remaining findings under `## Open` and set the status to `partial`.
  - Record the outcome in the report's `## Review`: each finding → fixed | out of scope | open, or `none`.
    Also record the `/code-review` level used, the changed-line count, and each review sub-agent's token use.
- **Push & open the draft PR** once every commit is in and tests are green (also for partial work):
  - `git push -u origin <branch>` (never force).
  - No PR yet → `gh pr create --draft --base <base> --title "<subject of the first commit, verbatim>" --body-file
    <file>`. Existing PR → just push.
  - **Description** — use `.github/pull_request_template.md` when the repo has one, filled in, nothing added
    beyond it. Otherwise (stacked: first line `> Stacked on <base PR url> — merge that first.`):
    - `## What` — 1–3 concise sentences.
    - `## Acceptance criteria` — each criterion, one line each, with how it's met.
    - `## Tests` — the commands run and the result (for behaviour changes, the red-check line).
    - `Closes #nnn` (a stacked PR whose base is not `main` also gets it — GitHub closes the issue when the
      PR reaches the default branch).
    - End with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.

## Planning — no code changes, commits, push or PR

Write this block into the description as `## Plan` (editing rules in `tickets.md`; marker inside it), set
`## Status` (planned — awaiting approval: answer the questions, then remove the `planning` label), post a
`Changed` comment (`tickets.md`), then `gh issue edit nnn --add-label planning`:
```
## Plan
<!-- work-tickets plan -->
**Approach:** <2–5 sentences: the design and why over the alternatives>
**Trigger:** <which plan-first criteria hit>
### PRs
1. <slice> — files: `path`, … — no behaviour change | behaviour change — **Done when:** <test/condition>
2. …   (each ≤ ~400 lines, one concern; note rollout order if it matters)
### Risks
- <config/cache/history format changes, contract changes, performance, rollback>
### Questions
1. **<question>** — Options: **A)** … **B)** … — _recommended: A, because …_
```

## Needs info

No code changes (discard your own, as above). `gh issue edit nnn --add-label needs-info` and write what's
missing **concretely** into the description's `## Open questions` — questions with options where possible
(which provider, expected output, which key, …) — plus `## Status` (needs info). Post a `Changed`
comment. Don't guess. A problem, decision or acceptance criterion that looks wrong lands here too — never
edit those sections (`tickets.md`, "Requirements are human-owned").

## Report & GitHub write-back

Write `<report>` (overwrite):
```
## Status
done | partial | blocked | planned | needs-info
## Commits
<short sha + subject per line, or "none">
## Touched
<each file changed + one-line why>
## Tests
<commands run + result; for bug fixes, the red run before the fix; gofmt>
## Review
<reviewer findings → fixed | out of scope | open, or "none"; code-review level, changed lines, review sub-agent tokens>
## Open
<what's left + why, or "none">
## Findings
<problems outside this ticket's scope, one per line: `- **<short title>** — file:line — what you saw vs.
 what you expected`. "none" if nothing. Don't fix them.>
```

- **done** → never close the issue (the PR's `Closes #nnn` does it on merge). Update `## Status` (PR url,
  awaiting review) and mark the shipped slice in `## Plan` with the PR url. If the ticket asks for a decision
  record, write it as `## Outcome` in the description: **Decision** (what the PR does), **Consequences**
  (trade-offs, follow-ups), **Rules touched** (`CLAUDE.md` / `.claude/rules/*` changes, or "none"), **PR**
  (url). Then post a `Changed` comment.
- **partial / blocked** → update `## Status` with what's done, what's open and why, and the draft PR url;
  then a `Changed` comment.

**Result** — your final reply must be only this line:
`RESULT #nnn: <done|partial|blocked|planned|needs-info> · PR <url|none> · commits <n> · findings: <short title>; <short title>; … (or "none")`
