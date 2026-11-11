# Implementer brief (/harness:work)

Your prompt gives `<id>`, `<repo>`, `<wt>`, `<branch>`, `<report>`, `<base>` (default branch, or the branch this ticket is
**stacked** on — then also `<base-wt>` and the base PR url), the PR url when continuing, `plan approved` when set, and the flags
`no-verify` / `allow-external-apis` when set. Follow `tickets.md` and `git-rules.md` next to this file; operations come from the
adapter (`harness-resolve adapters/<tracker>.md`); project facts from `<repo>/.claude/harness.md` (`harness-config …`) — its
sections (Commands, Plan-first triggers, Self-check, Test conventions, Never) **extend and override** the generic text below.

- **Read the ticket:** `read-ticket` (with comments) and `blockers-read` — the description (current state, incl. `## Plan` when
  plan-approved) and comments. An old-format ticket is folded into the description on your first write (`tickets.md`, "Fold on touch").

## Gate first — before any setup or code change

Explore read-only. Locate code with CodeGraph when `<repo>/.codegraph/` exists, otherwise grep/read; read files in `<wt>` if it
exists (continuing), otherwise `<base-wt>` when stacked, otherwise `<repo>/.user/worktrees/_main` (a detached checkout of
`origin/<default>`). Never edit those.

1. **Readiness:** if you can't write the failing test yet, you don't understand the ticket → **needs-info** (below).
2. **Just solve it** when all hold: root cause known or quick to find; 1–3 files in one module/layer; follows a pattern the
   codebase already has (an endpoint/provider like its siblings, a missing guard, a wrong cast, a copy fix); easy to revert (no
   schema/format/migration, no public API or contract change, no on-disk/data format change); one test proves it; one reasonable
   approach (a real toss-up → pick one and move).
3. **Plan first** when any hold: unknown cause (investigate first — if it then turns out small, just solve it); blast radius
   (a shared function with many callers, a change to a core interface, a cross-module change, event/message flow change); hard to
   undo (schema, data backfill, API contract, queue/payload shape, persisted formats, config formats); multiple real designs
   (new module vs. existing, sync vs. async); needs sequencing (several PRs, flags, deploy order, deprecation period); money,
   auth or data loss involved; **plus every trigger in harness.md § Plan-first triggers** → **planning** (below). No worktree, no branch.
4. **Plan approved** skips 2–3: implement the plan's next unshipped PR slice. If the plan turns out wrong in a way that needs a
   decision → **needs-info**.

## Worktree setup (only when implementing)

If `<wt>` doesn't exist (idempotent; continuing an existing branch is detected by itself):
```
harness-worktree add issue-<nnn> <base> --branch <branch>
```
It fetches `origin/<base>`, adds the worktree (new branch from `origin/<base>`, or the existing branch) and runs harness.md
`setup-worktree` when set.

- **Work only inside `<wt>`.** `cd <wt>` before every command; read and edit only paths under `<wt>` (plus `<report>`).
  CodeGraph indexes `<repo>` — use it to locate code, then read/edit the file in `<wt>`.
- Never change dependencies (package manager add/update/remove, lockfiles, go.mod by hand) — a dependency change is an open
  question. A lockfile that changed after `git merge origin/<base>` is fine; install/build per harness.md.
- Read `CLAUDE.md`, `.claude/rules/*` and docs for what you touch; activate matching project skills. Follow sibling-file conventions.

## Implementing

- If mid-work a just-solve ticket trips a plan-first trigger or grows past ~400 changed lines (excluding tests, lockfiles,
  `exclude_from_size`): discard **your own** uncommitted changes (`git restore <files>`, delete files you created). Not pushed yet →
  also `git -C <repo> worktree remove --force <wt>` and `git -C <repo> branch -D <branch>`. Then switch to planning.
- **One concern per PR.** A diff is either "no behaviour change" or "behaviour change + its test" — never both. Stay in scope: no
  "while I'm here" cleanups, no changed defaults, no loosened validation. Report anything else under `## Findings`.
- **Test first, and prove it can fail:** write the test, run it **red**, then fix, run it green. For bug fixes the test must fail
  with the fix reverted. No assertion-free tests, no asserting a fake's own return value, no faking the type under test.
  Follow harness.md § Test conventions.
- **Red check before push (behaviour changes):** commit, then `harness-red-check <wt> <base>` must exit 0 (red without the non-test
  diff, green with it; it restores the tree itself, no stash). Paste its output into `## Tests`, plus one line `red-check: exit <n>` per run (`harness-retro` counts these). Exit 1 = test can't fail, 2 = not
  green with the fix, 3 = no test changes, 4 = setup error/dirty tree, 5 = red from a compile/setup error, not a failing test: fix and
  rerun. "No behaviour change" PRs are exempt — say so in `## Tests`.
  - **New identifiers** (tests call functions/types/fields the diff adds, so the red run cannot compile): first commit only the
    stubs — the new identifiers with stubbed behaviour (zero values / "not implemented" errors), compiling, no tests — then the tests
    and the real code in later commits. Run `harness-red-check <wt> <base> <stub-commit sha>`: the red run keeps the stubs and drops
    only what came after them. Name the stub commit in `## Tests`. A stub that already makes the tests pass exits 1.
- **Checks** — same as CI, green before every commit: harness.md § Commands `test`, `static`, `format`, `build` (empty = skip).
  **Fix budget: 3 attempts per failing check** (a fix + re-run is one attempt); then stop, set `partial` (class `checks-red`) and
  list the rest under `## Open`.
  Fix new findings in the code — no suppression comments, no skipped tests, no baseline/exclusion entries, no widened types to
  silence a tool. A baseline entry your fix resolved → delete it. Only format the files you changed.
- **Self-check** before committing: no invented functions or methods (verify they exist), no swallowed errors or catch-alls added
  to silence one, no helper duplicating an existing one (grep first), no unquoted/unvalidated shell or SQL strings,
  plus harness.md § Self-check.
- **Commits:** per `git-rules.md`. Body: one or two lines on what and why, then `Refs <id>` and the Co-Authored-By trailer.
  One commit by default; each commit leaves tests green.
- **Review (once, before the push):** with every commit in and checks green, spawn one fresh sonnet sub-agent (a new agent, not a
  fork). Its prompt carries the ticket's acceptance criteria, the full `git -C <wt> diff origin/<base>...HEAD` and the commit log
  as author intent, and tells it: read-only (no edits, no commits), apply `harness-resolve briefs/review-criteria.md` (rules
  compliance + test quality, signal threshold, false-positive list), validate each candidate with a separate sonnet sub-agent per its
  "Validation", and reply with only validated findings, one per line as `- <file>:<line> — <problem; quote the rule if any> — <fix>`,
  or `none`.
  - **Bug review, in parallel:** in the same message spawn a second fresh sonnet sub-agent that runs `/code-review <level> <branch>`
    in `<wt>` (read-only, no `--fix`/`--comment`) and replies in the same line format, or `none`. Always pass `<level>`. Level from
    changed lines = insertions + deletions of `git -C <wt> diff --shortstat origin/<base>...HEAD -- . <tests, lockfiles, generated,
    exclude_from_size as ':(exclude,glob)…'>`: **≤ 200 → `low`, > 200 → `medium`**. Never `high` or above.
  - Merge both reviewers' findings, drop duplicates (same file:line or problem) before fixing.
  - Fix each in-scope finding in one extra commit, then re-run the checks. An out-of-scope finding goes under `## Findings`.
  - No second review round. If the fixes need review, or the re-run is still red after the 3-attempt budget, list the remaining
    findings under `## Open` and set the status to `partial`.
  - Record in the report's `## Review`: one line per finding, `- [<source>] <file>:<line> — fixed | out of scope | open`, where
    `<source>` is `rules`, `test-quality` (both from the review-criteria reviewer), `code-review` or `intent` (a finding that the
    diff misses the ticket's acceptance criteria); `none` when there are no findings. Then the `/code-review` level, changed-line
    count and each review sub-agent's token use. `harness-retro` counts these tags.
- **Verify before the PR (unless `no-verify` or harness.md has no `## Verify` section):** after checks are green and before pushing,
  follow `harness-resolve skills/verify/SKILL.md`'s procedure for this ticket on `<wt>` (pass `--allow-external-apis` only when your
  prompt says `allow-external-apis`; never ask the user — unattended: no fix loop, no tickets). Failed / aborted / skipped steps
  **never block the PR**: still push and open it; put the result (passed / failed / aborted / skipped + one line each) into the PR
  description and the ticket `## Status`. Only `passed` with **0 skipped** steps → add the `verified` label to the PR once it exists
  (create the label if missing).
- **Push & open the draft PR** once every commit is in and checks are green (also for partial work):
  - `git push -u origin <branch>` (never force).
  - No PR yet → `create-draft-pr` with `--base <base>`, title = the subject of the commit that delivers the feature or fix, **verbatim** (when the branch starts with a stub commit, that is the first non-stub commit — never the `chore: stub …` subject, which would end up on the default branch after a squash merge), description file below.
    Existing PR → just push.
  - **Description** — the repo's `pr-template` when there is one, filled in, nothing added beyond it. Otherwise (stacked: first line
    `> Stacked on <base PR url> — merge that first.`): `## What` (1–3 concise sentences) · `## Acceptance criteria` (one line each,
    with how it's met) · `## Tests` (commands + result; the red-check line for behaviour changes) · the adapter's closing keyword
    (a stacked PR whose base is not the default branch also gets it). **Multi-slice ticket** (`## Plan` has several PRs): only the
    last slice's PR carries the closing keyword; earlier slices carry the adapter's reference form instead (GitHub: `Refs #<n>`) · end with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.

## Planning — no code changes, commits, push or PR

Write this block into the description as `## Plan` (editing rules in `tickets.md`; marker inside it), set `## Status` (planned —
awaiting approval: answer the questions, then remove the `planning` label), post a `Changed` comment, then `add-label planning`:
```
## Plan
<!-- work-tickets plan -->
**Approach:** <2–5 sentences: the design and why over the alternatives>
**Trigger:** <which plan-first criteria hit>
### PRs
1. <slice> — files: `path`, … — no behaviour change | behaviour change — **Done when:** <test/condition>
2. …   (each ≤ ~400 lines, one concern; note rollout order if it matters)
### Risks
- <schema/format/contract changes, performance, rollback, deploy order>
### Questions
1. **<question>** — Options: **A)** … **B)** … — _recommended: A, because …_
```

## Needs info

No code changes (discard your own, as above). `add-label needs-info` and write what's missing **concretely** into `## Open questions`
— questions with options where possible — plus `## Status` (needs info). Post a `Changed` comment. Don't guess. A problem, decision
or acceptance criterion that looks wrong lands here too — never edit those sections (`tickets.md`, "Requirements are human-owned").

## Report & tracker write-back

Write `<report>` (overwrite):
```
## Status
done | partial | blocked | planned | needs-info — for partial/blocked add `class: <class>` (below)
## Commits
<short sha + subject per line, or "none">
## Touched
<each file changed + one-line why>
## Tests
<commands run + result; for bug fixes, the red run before the fix; static/format results>
## Review
<`- [rules|test-quality|code-review|intent] file:line — fixed | out of scope | open` per finding, or "none"; code-review level, changed lines, review sub-agent tokens>
## Verify
<passed|failed|aborted|skipped + one line each, or "not run">
## Open
<what's left + why, or "none">
## Findings
<problems outside this ticket's scope, one per line: `- **<short title>** — file:line — what you saw vs. what you expected`. "none" if nothing. Don't fix them.>
```

- **done** → never close the ticket (the last slice's PR closing keyword does it on merge). Update `## Status` (PR url, awaiting review) and mark
  the shipped slice in `## Plan` with the PR url. If the ticket asks for a decision record, write `## Outcome`: **Decision**
  (what the PR does), **Consequences** (trade-offs, follow-ups), **Rules touched** (`CLAUDE.md` / `.claude/rules/*` / harness changes, or
  "none"), **PR** (url). Then a `Changed` comment.
- **partial / blocked** → update `## Status` with what's done, what's open and why, and the draft PR url; then a `Changed` comment.

**Failure class** — every `partial`/`blocked` names exactly one, in the report `## Status` and the RESULT line:
- `env/setup` — worktree/setup/install/build or tool environment broke (`harness-red-check` exit 4; exit 5 when the compile/setup error is not from your code).
- `checks-red` — checks or tests still red after the 3-attempt budget (`harness-red-check` exit 1, 2 or 5 from your own code).
- `spec-unclear` — ticket/plan can't be satisfied as written (`harness-red-check` exit 3 when no test is possible).
- `tool-denied` — a needed tool/permission/hook (e.g. `harness-guard`) refused.
- `budget` — you ran out of turns/context before finishing.

**Result** — your final reply must be only this line:
`RESULT <id>: <done|partial|blocked|planned|needs-info> · PR <url|none> · commits <n> · findings: <short title>; <short title>; … (or "none")`
For `partial`/`blocked` insert `· class: <class>` right after the status.
