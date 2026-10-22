---
allowed-tools: Read, Grep, Glob, AskUserQuestion, Bash(ls:*), Bash(cat:*), Bash(grep:*), Bash(find:*), Bash(gh issue list:*), Bash(gh issue view:*), Bash(gh issue create:*), Bash(gh issue edit:*), Bash(gh issue comment:*), Bash(gh pr list:*), Bash(gh api:*), Bash(gh label list:*), Bash(date:*), mcp__codegraph__codegraph_explore
description: Report a bug, change request or feature and file it as a ready GitHub issue for /work-tickets — or pass an issue number to answer a needs-info ticket's open questions
---

Turn something the human reports — a **bug**, a **change request** or a **new feature** — into an
actionable GitHub issue that `/work-tickets` picks up. Ticket shape and labels:
`.claude/work-tickets/tickets.md`. Plan-first criteria and plan format: `.claude/work-tickets/implementer.md`
("Gate first", "Planning").

**This command writes no application code, runs no tests, and never touches git.** It only writes to GitHub issues.

Argument:
- **An issue number** (`/ticket 812`; `#812` also works) → **resume mode**, below.
- **Free text** (`/ticket the review view ignores the author filter`) → the report flow, Steps 1–4.
- **Omitted** → ask for it via `AskUserQuestion` (options: `Bug` / `Change request` / `Feature`; the human
  types the details via "Other" or a follow-up).

## Resume mode — answer a `needs-info` ticket

- `gh issue view <n> --json number,title,body,state,labels,comments` plus its blockers (`tickets.md`, "Writing").
  It must be open and labeled `needs-info`. If not, say what it is (no `needs-info` → nothing to answer,
  `/work-tickets` handles it; `planning` → the `## Plan` is approved by answering its questions in the
  description and removing the label by hand; closed → nothing to do) and stop without writing.
- Read the description and comments in full. Ask each `## Open questions` item via `AskUserQuestion` (as in
  Step 2), with follow-ups until the answer is concrete.
- Confirm understanding (Step 2b) before drafting.
- Draft the update: replace `## Open questions` with `## Decision` (an `Understanding:` bullet, then one
  bullet per answer, each naming the alternatives rejected and why); update the acceptance criteria and `## Touches` where the answers change them. Never edit `## Problem`
  (`tickets.md`, "Requirements are human-owned").
- Re-check the plan-first criteria and run the collision check (Step 1). If plan-first now applies, add a
  `## Plan` as in Step 3.
- Show the draft and ask once via `AskUserQuestion`: **"Update this ticket?"** — `Update (Recommended)` /
  `Adjust`.
- Then `gh issue edit <n> --body-file <file> --remove-label needs-info` (+ blocked-by dependencies for collision
  hits, `Related:` lines under `## Reference`), set `## Status` to `Ready for /work-tickets.`, and post a
  `Changed` comment (`tickets.md`). Report the issue number + url.

End with: **"Ready — `/work-tickets #nnn` will pick it up."**

Everything below is the report flow.

## Step 1 — Investigate

- Classify → label: `bug` (behaviour differs from intent), `improvement` (existing behaviour should
  differ), `feature` (new behaviour).
- **Duplicate check**: `gh issue list --state open --search "<keywords>" --json number,title,labels` with a few
  keyword queries; open similar ones. If one already covers it, show it and ask via `AskUserQuestion`:
  `Add to #mmm` (update that ticket's description — fold in the new report, `## Status` — plus a `Changed`
  comment, then stop) / `File separately` / `Drop`.
- Look at the code (CodeGraph first when `.codegraph/` exists, then grep/read) and the matching `docs/`
  (`docs/design.md`, `README.md`). For a bug, find the cause (file:line, what happens vs. what should). Don't
  run tests or the app.
- List every file the change would create or modify, **including** tests, docs and the README sections
  (config reference, key bindings) it affects. Check it against the plan-first criteria.
- **Collision check:** run the recipe in `tickets.md` ("In-flight work & the collision check") on the
  packages/types/dirs in that list (they become `## Touches`). On a hit, show it and ask via
  `AskUserQuestion`: `Add blocked-by #mmm` / `Relate only` / `Merge into #mmm` (update its description plus a
  `Changed` comment, as above, then stop).
- Brief the human (≤ 10 lines): what you understood, what the code does today (file:line), the likely
  fix/approach, the file estimate, just-solve vs. plan-first.

## Step 2 — Ask

Ask everything an implementer would otherwise guess — behaviour, config keys, naming, key bindings, edge cases,
scope, splitting — via `AskUserQuestion`: standalone question text, 2–4 options, recommended first with
`(Recommended)`, `preview` for code/UI/config shapes, follow-ups until the decision is concrete. No questions
is fine if the report is unambiguous. If the human wants to postpone a decision, the ticket gets
`needs-info` (Step 3).

## Step 2b — Confirm understanding

Before drafting, restate the problem in 2–3 lines, checked against the code/docs (file:line), and have the
human confirm it via `AskUserQuestion`: `Correct (Recommended)` / `Not quite`. On `Not quite`, take the
correction, re-investigate if needed, and restate. Skip only if the human's report was already this precise
and nothing was asked in Step 2.

## Step 3 — Draft

```
## Status
Ready for /work-tickets. (needs-info: Waiting on the human to answer `## Open questions`.)

## Problem
<1–5 sentences: what happens vs. what should, with file:line evidence; for features: what's missing and why>

## Decision
- Understanding: <the 2–3 line problem restatement the human confirmed in Step 2b>
- <one bullet per answer from Step 2, concrete; name the alternatives rejected and why>

## Acceptance criteria
- [ ] <testable criterion>

## Touches
<package, type or dir per line, from the file list in Step 1>

## Reference
Reported by the human (YYYY-MM-DD). Docs: <path>. Related: #nnn
```

- Title: short, imperative, what to implement (`Apply the author filter in the review view`).
- **Plan-first** → also draft a `## Plan` section (after `## Touches`; implementer's planning format, with the
  `<!-- work-tickets plan -->` marker, no `### Questions` — they were answered in Step 2). If the slices are independent, offer to file them as separate tickets.
- **Decision postponed** → an `## Open questions` section with the concrete options, after the `## Decision`
  (which keeps the `Understanding:` bullet and any decisions already made).

Show the human the title, body (and plan) and ask once via `AskUserQuestion`: **"File this ticket?"** —
`File (Recommended)` / `Adjust`. On `Adjust`, update the draft and ask again.

## Step 4 — File & report

- Write the body to a file in the scratchpad, then
  `gh issue create --title "<title>" --body-file <file> --assignee @me --label <type label>`
  (+ `--label needs-info` for a postponed decision; `Related:` lines under `## Reference` for related tickets and
  `Relate only` hits; for `Add blocked-by` hits, add the dependency afterwards — `tickets.md`, "Writing"). No priority label unless it's a
  bug the human rated (`priority:1`–`priority:4`). A missing label: create it first with `gh label create`.
- Plan-first → the plan is in the description, **without** the `planning` label — it's approved.
- Report: issue number + url, title, label, acceptance-criteria count, file estimate.

End with: **"Ready — `/work-tickets #nnn` will pick it up."** (needs-info: **"Filed — answer the question
and remove `needs-info`."**)
