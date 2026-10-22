# Ticket shape (/work-tickets, /ticket)

Tickets are **GitHub issues of this repository** (`gh issue …`, `gh api`), referenced as `#nnn`.
`/work-tickets` works the open ones **assigned to me**. A pull request closes its ticket through
`Closes #nnn` in its description — **no agent ever closes a ticket or changes its assignee.**

Labels that steer `/work-tickets`:

| Label | Means | Who removes it |
|-------|-------|----------------|
| `on hold`, `ready-for-human` | not for agents | human |
| `needs-info` | `## Open questions` in the description | `/ticket nnn`, after answering |
| `planning` | a `## Plan` awaits approval | human — removing it approves the plan |

Type labels: `bug`, `feature`, `improvement`. Priority labels (optional): `priority:1` (urgent) … `priority:4` (low);
no label sorts last. Other labels: `triage` (found by an agent, not yet accepted), `agent-harness`.

Create a missing label once with `gh label create <name> --color <hex>`.

**Body (description)** — the full, **current** state of the ticket, rewritten in place (GitHub keeps the edit
history). Sections, in this order, omitting the empty optional ones:
- `## Status` — one or two lines: where the ticket stands, the next step, who it waits on, the PR url.
- `## Problem` — what happens vs. what should, with file:line evidence; for features: what's missing and why.
- `## Decision` — optional; the agreed answers, concrete. Opens with an `Understanding:` bullet (the
  problem restated in 2–3 lines and confirmed by the human); each decision bullet names the alternatives
  rejected and why.
- `## Open questions` — optional; questions with options. When answered, the human or `/ticket` folds the
  answer into `## Decision` and removes the question; an autonomous agent marks it `→ answered (see comment)`.
- `## Acceptance criteria` — checklist, each item testable.
- `## Touches` — the packages, types and dirs it changes, one per line, e.g. `internal/session`,
  `provider.Provider`.
- `## Plan` — optional; the implementer's planning format (`implementer.md`) with the
  `<!-- work-tickets plan -->` marker inside it. Each shipped slice is marked in place with its PR url.
- `## Outcome` — optional; the decision record.
- `## Reference` — optional; docs, `Related: #nnn`.

A ticket without a clear problem and testable acceptance criteria is not ready.

**Requirements are human-owned.** On an existing ticket, `## Problem`, `## Decision` and `## Acceptance criteria`
change only through an interactive command whose draft the human approves (`/ticket`). Autonomous agents
(triager, implementer) never edit them — they write only `## Status`, `## Open questions`, `## Touches`,
`## Plan`, `## Outcome`. If a requirement looks wrong, adjust the solution or go **needs-info** with an open
question; never fit the requirement to the solution. Creating a new ticket writes all sections.

**Every writer updates `## Status`**: `/ticket` on filing, the triager on needs-info, the implementer on
planned/partial/blocked/done.

**Editing the description:** GitHub has no partial edit. Read the current body
(`gh issue view nnn --json body --jq .body > <file>` in the scratchpad), change **only your own sections** in
the file, write it back with `gh issue edit nnn --body-file <file>`. Re-read right before writing; keep every
other character intact. Only the interactive commands may rewrite the human-owned sections.

**Plan-approved** = the description has `## Plan` (or, on an unfolded old ticket, a comment starting with
`<!-- work-tickets plan -->`) and the ticket has **no** `planning` label. Implement its **next unshipped PR
slice** only.

**`Changed` comment** — every description change is followed by one comment
(`gh issue comment nnn --body …`):
`**Changed (YYYY-MM-DD) · <sections touched>:** <1–3 lines: what and why>`. It is a pointer, not a change log;
the content lives in the description.

**Fold on touch** — the first writer to touch an old-format ticket (state in comments, an old plan comment,
`## Update (…)` comments) folds that into the living description, then posts one `Changed` comment. Old
comments stay. An autonomous agent folds only into its own sections; what belongs in a human-owned section
stays in its comment (still read as context).

**Writing:** labels via `gh issue edit nnn --add-label … / --remove-label …`. New issues:
`gh issue create --title … --body-file … --label … --assignee @me`, body starting with a `## Status` section.
**Blockers** are GitHub issue dependencies:
- read: `gh api repos/{owner}/{repo}/issues/<nnn>/dependencies/blocked_by`
- add: `gh api -X POST repos/{owner}/{repo}/issues/<nnn>/dependencies/blocked_by -F issue_id=<blocker's numeric id>`
  (the numeric id is `gh api repos/{owner}/{repo}/issues/<blocker> --jq .id`, not the issue number).

There is no relation type for "related": write `Related: #mmm` under `## Reference`.

**Every PR needs a ticket:** its description contains `Closes #nnn`, its branch is `<type>/<nnn>-<slug>`.

## In-flight work & the collision check

**In-flight** = an open issue that has an open pull request, i.e. a branch `<type>/<nnn>-*` in
`gh pr list --state open --json number,title,headRefName,body`. There is no `in-flight` label.

Direction of a migration never goes into code or docs while it is in flight. The PR slice that lands the new
path on `main` adds a **freeze test** on the legacy side (a Go test that fails when the legacy package gains
new importers or exports); the test goes when the legacy side goes.

**Collision check** — bounded; never iterate over all open tickets:
1. `gh pr list --state open --json number,title,headRefName` — the in-flight tickets.
2. 2–3 `gh issue list --state open --search "<touched name>" --json number,title,labels` on the ticket's
   `## Touches` entries (no `## Touches` → keywords from its title/problem). This also catches tickets with an
   approved plan and no PR yet.
3. Open a ticket in full (`gh issue view`, incl. its `## Plan`) only when its title or a query hit suggests it
   touches the same package/type/dir. Ignore the ticket itself and tickets already linked to it (`Related:`,
   blocks / blocked by) — that overlap was decided.

A hit is a ticket that is in-flight or plan-approved and touches the same code. What happens on a hit is up
to the caller (`/ticket`, the `/work-tickets` triager).

## Harness harvest (/work-on-comments, /local-code-review --fix)

Turns review corrections into guardrails. Input: the comments resolved / findings applied in this run.

1. **Classify** each item and print a table: `link (or file:line) -> class -> one-line reason`.
   - **mechanizable** — could be a Go test, a `go vet` / linter rule or a CI check.
   - **instructable** — a missing or unclear line in `CLAUDE.md` (or `.claude/rules/*`) or a work-tickets brief.
   - **judgment** — nothing to change; creates nothing.
2. **Group** mechanizable/instructable items into one pattern each. No patterns -> stop. Give each pattern a stable
   lowercase key `<guardrail-type>:<area>` (e.g. `mechanizable:session-layering`); reuse the key of an existing
   `agent-harness` issue when the pattern matches it.
3. **Dedupe:** `gh issue list --label agent-harness --state all --search "<key>" --json number,title,state`
   (triage ones included); only if that finds nothing, retry with `<pattern keywords>`. A duplicate gets a
   `gh issue comment` with the new evidence (PR + comment link), not a new issue.
4. **Confirm once:** one `AskUserQuestion` listing every new issue and every duplicate comment to post. File
   nothing without approval; drop what the human rejects.
5. **File** each approved new pattern: `gh issue create --label agent-harness --label triage --title … --body-file …`,
   unassigned, no priority. Description: `Key: <key>`, `## Status` (triage, filed from review), `## Problem`
   (the comment(s) as evidence, file:line), the proposed guardrail (which rule file / test / brief), and
   `## Acceptance criteria`.
