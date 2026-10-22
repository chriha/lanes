---
allowed-tools: Read, Grep, Glob, Agent, SendMessage, AskUserQuestion, Bash(ls:*), Bash(cat:*), Bash(grep:*), Bash(mkdir:*), Bash(rm:*), Bash(jq:*), Bash(git status:*), Bash(git diff:*), Bash(git log:*), Bash(git show:*), Bash(git fetch:*), Bash(git rev-parse:*), Bash(git worktree:*), Bash(git branch:*), Bash(git -C:*), Bash(git add:*), Bash(git commit:*), Bash(git push:*), Bash(git merge:*), Bash(git restore:*), Bash(git checkout:*), Bash(gh pr create:*), Bash(gh pr list:*), Bash(gh pr view:*), Bash(gh pr edit:*), Bash(gh issue list:*), Bash(gh issue view:*), Bash(gh issue create:*), Bash(gh issue edit:*), Bash(gh issue comment:*), Bash(gh label list:*), Bash(gh label create:*), Bash(gh api:*), Bash(make test:*), Bash(go test:*), Bash(go vet:*), Bash(go build:*), Bash(go run ./scripts/work-tickets/candidates:*), Bash(gofmt:*), Bash(scripts/work-tickets/sync.sh:*), Bash(*/scripts/work-tickets/sync.sh:*), Bash(scripts/work-tickets/worktree.sh:*), Bash(*/scripts/work-tickets/worktree.sh:*), Bash(scripts/work-tickets/red-check.sh:*), Bash(*/scripts/work-tickets/red-check.sh:*), mcp__codegraph__codegraph_explore
description: Work my open GitHub issues — a triager gates them, sub-agents fix each in its own worktree and open a draft PR (or plan it, if risky), and verified side findings land in triage
---

Work the GitHub issues **assigned to me** that are open in this repository. Ticket shape and labels:
`.claude/work-tickets/tickets.md`. One branch and one **draft** PR per ticket against `main` (or its chain
base). The PR's `Closes #nnn` closes the ticket on merge — nobody closes or reassigns a ticket.

**Definition of done:** a draft PR whose title is the first commit's subject, whose description is the
project's PR template (or the default in `implementer.md`) filled in concisely and contains `Closes #nnn`,
whose tests fail without the fix and pass with it, and whose code passes `make test` and `gofmt -l .`.
**The PR stays draft** — you mark it ready.

Paths: `<repo>` is the main checkout (absolute), `<wt>` is `<repo>/.user/worktrees/issue-<nnn>`, `<report>`
is `<repo>/.user/reports/issue-<nnn>.md`. `.user/` is gitignored.

## The orchestrator stays thin

**Context is the budget.** Everything the orchestrator reads stays in its context until the run ends, so it
**only routes**: it hands out ticket numbers, spawns sub-agents, and passes short status lines between them.

| Agent | Model | Does | Returns |
|-------|-------|------|---------|
| **triager** | haiku | Step 1: select, gate, needs-info, overlap, chains | the plan block |
| **implementer** | sonnet / opus (per triage row) | Step 2: gate, worktree, code, commits, push, PR, GitHub write-back — or a plan | one line |
| **verifier** | haiku | Step 3: verify one finding, file a triage issue if real | one line |

Briefs live in `<repo>/.claude/work-tickets/`: `triager.md`, `implementer.md`, `verifier.md`, plus the
shared `tickets.md` and `git-rules.md`.

Orchestrator rules:
- **Never** read tickets, comments, reports, diffs, test output, agent transcripts or output files
  yourself. Don't re-run checks a sub-agent already ran. If you need a fact, ask a sub-agent for it in one
  line.
- Details go in `<report>` or the issue, never in agent replies. Every prompt says: *"Your final reply must be
  only the RESULT block from your brief — no preamble, no recap."*
- **Don't read or paste the brief files.** A prompt holds only the concrete values the step lists and the
  line *"Read and follow `<repo>/.claude/work-tickets/<role>.md`"*, with absolute paths.
- All sub-agents use `subagent_type: "general-purpose"` and `run_in_background: true`; wait for
  notifications, don't poll.

Argument (optional):
- **Ticket numbers** (e.g. `/work-tickets 812 815`; `#812` also works) → exactly those, in parallel, skipping the
  assignee filter (the skip/blocked checks still apply). A ticket with an open PR **continues** on it.
- **Chains** (e.g. `/work-tickets 812>815 820`) → `>` works tickets **one after another**, each
  **stacked** on the previous one's branch (see *Chains*). Space-separated groups still run in parallel.
- **Omitted** → all eligible tickets, highest priority first, then oldest. Blocked ones are reported, not
  asked about.
- **`-nnn`** (repeatable, any mode; e.g. `/work-tickets -837`) → excludes that ticket. Alone: all
  eligible tickets except the excluded ones. With numbers or chains: removes them from the named set; an
  excluded chain ticket is dropped with everything after it (`812>837 -837` → chain `812`).
- **`--budget N`** (any mode, N ≥ 1) → stop **starting** implementers once N have returned `done` or `partial` (every result that opens a PR). Running
  implementers (and their verifiers) finish and are reported; none is killed. Combinable with numbers, chains,
  `-nnn` and `--dry-run`. Not passed to the triager.
- **`--dry-run`** → Steps 0–1 only (the triager's needs-info writes still happen): print the plan and stop.
  With `--budget N`, mark the first N `ELIGIBLE` tickets (in order; a chain counts all its tickets) as
  *within budget* and the rest as *over budget*.

Git rules for everyone: `.claude/work-tickets/git-rules.md`.

### Chains

A chain `A>B>C` works `B` only after `A` is done, and cuts `B`'s branch from `A`'s instead of
`origin/main`; `B`'s PR targets `A`'s branch, so each PR shows only its own changes. A ticket with an open
PR can **head** a chain: it isn't worked, only the base. Once a base PR is merged, Step 0 moves the stacked
PR onto `main`.

- A chain holds **one implementer slot** at a time; the next ticket starts only after the previous one
  returns `done` (or it's the open-PR head).
- A chain is not started when the remaining budget is 0. Each `done`/`partial` chain ticket counts against the budget;
  once it is spent, the chain's remaining tickets are not started.
- If a chain ticket ends anything but `done`, **the chain stops**: the rest is reported as *chain stopped
  at #nnn*.

## Step 0 — Sync (orchestrator, shell only)

Run `scripts/work-tickets/sync.sh <repo>` once and keep its lines for the report. It fetches/prunes, removes
worktrees (plus branch and report) of merged/closed PRs or branches deleted on origin (`issue-nnn merged|closed|deleted`),
unstacks PRs whose base merged (`UNSTACKED: <url>`; a conflict, missing worktree or closed base prints
`UNSTACK_CONFLICT: <url> …`), and refreshes `.user/worktrees/_main`.

## Step 1 — Triage (one triager)

Spawn one **triager** (`model: "haiku"`) with `<repo>` and the argument verbatim (if any). Wait for it.
Keep `BASE_SHA`. Print Step 0's lists plus the triager's block, lightly formatted. With `--dry-run`, stop.

### Blocked argument tickets — ask instead of dropping

Only when the argument named tickets. For every `BLOCKED` line with a `fix:` or `partial:` chain, ask with
`AskUserQuestion` (one question per ticket, up to 4 per call; header `#nnn`): *"#nnn is blocked by
<blockers with status>. Work the blockers first?"* Options, only those that apply:
- **"Blockers, then #nnn"** (first, `(Recommended)`) — description: the `fix:` chain.
- **"Blockers only"** — the `fix:` chain without the ticket (or the `partial:` chain when there's no `fix:`).
- **"Skip #nnn"** — leave it blocked.

If any answer adds work, build the new argument (each blocked ticket replaced by its chosen chain; merge
chains that share tickets so each appears once, blockers first) and spawn a **fresh triager** with it. Its
block replaces the first; print it. Don't ask again — a ticket still `BLOCKED` goes to the report.

## Step 2 — Implement (one implementer per ticket)

Run **up to 3 implementers in parallel**, in `ELIGIBLE` order, each with the row's model. When one
finishes, handle its findings (Step 3) and start the next eligible ticket. A chain takes one slot (see
*Chains*).

**Budget** (`--budget N`): keep a count of PR-opening results (`done` + `partial`). Slots in flight may
exceed what the budget still needs, so start a ticket only while that count + implementers in flight < N;
otherwise hold it and start it later if a running implementer ends `blocked`/`planned`/`needs-info` and budget
remains. Once the count = N, start nothing new; let running implementers finish. Tickets never started land in the report as **Not started (budget)**.

Implementer prompt: `#nnn`, `<repo>`, `<wt>`, `<branch>`, `<report>`, `<base>`, the row's flag
(`plan approved` / continues `<PR url>`). Stacked: also the base ticket's `<base-wt>` and PR url.

## Step 3 — Verify findings (one verifier per finding)

For each finding title in a RESULT line, skip it if it obviously matches one already sent this run;
otherwise spawn a **verifier** (`model: "haiku"`). Verifiers are read-only on code, run in parallel with
everything and don't count against the implementer limit — don't wait for them.

Verifier prompt: the finding's short title, the source `#nnn`, `<repo>`, `<wt>`, `<report>`, and the
titles already in flight.

Each verifier persists its verdict as a `<!-- finding-verdict -->` comment on the source ticket (its brief
says how), so finding precision can be measured later.

New triage issues are **not** worked in this run.

## Step 4 — Report

Wait for every implementer and verifier. Build the report **only from RESULT lines, Step 0's lists and the
triage block** — don't open tickets or files. One line each:

- **Cleaned up:** Step 0's cleaned-up tickets.
- **Draft PRs:** implementer `done`, with PR url.
- **Partial / blocked / held:** implementer `partial`/`blocked`, triage `BLOCKED`/`HELD` (with the user's
  choice, if asked), with the draft PR url where there is one.
- **Stacked:** each chain as `A → B → C` with PR urls, *chain stopped at #nnn* where it stopped. Merge
  stacked PRs **in chain order**. Plus Step 0's `UNSTACK_CONFLICT` lines (need a manual merge of `main`).
- **Planned:** implementer `planned` → answer the `## Plan`'s questions on the issue, then remove the `planning`
  label to approve it.
- **Needs info:** triage `NEEDS_INFO` plus implementer `needs-info` → `/ticket nnn` answers the open questions and removes `needs-info`.
- **Skipped:** triage `SKIPPED`.
- **Not started (budget):** with `--budget N`, the eligible tickets (and chain remainders) never started.
- **New triage issues:** verifier `confirmed` (number + url), plus a one-line duplicate/invalid tally.
- Worktrees still around: one `git worktree list`.

End with: **"Draft PRs are open (base `<base-sha>`). Check CI, review, and mark them ready yourself. The
next `/work-tickets` run cleans up worktrees for merged/closed PRs."**
