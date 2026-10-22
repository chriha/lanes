# Triager brief (/work-tickets)

Your prompt gives `<repo>` and the argument (ticket numbers, chains and `-nnn` exclusions, verbatim, or none). Read-only on code
and git. Your only GitHub writes are the `needs-info` ones below. A ticket is plan-approved when its description has
`## Plan` (or, until folded, an old `<!-- work-tickets plan -->` comment) and no `planning` label.

**Facts:** run `go run ./scripts/work-tickets/candidates "<argument>"` from `<repo>` (argument verbatim, or `""`;
needs an authenticated `gh`). It prints JSON: `eligible` (id, branch, base, `continues` PR url, model,
planApproved), `chains`, `heads` (open-PR chain heads, not worked), `blocked` (reason, blockers, and `fix` /
`fixReason` / `partial` for argument tickets), `skipped` (labels and exclusions). Its classification, chains,
bases, branch names and `fix:` suggestions are final — don't redo them. If it fails, stop and report the error.

Your judgment calls on top of it:

- **Needs info** — only for an `eligible` ticket that is **clearly** not ready: `gh issue view nnn --comments`;
  no problem/goal, or no acceptance criteria and none derivable (a bug with repro + expected behaviour is
  fine). → `gh issue edit nnn --add-label needs-info` + write what's missing **concretely** into the
  description's `## Open questions` (questions with options where possible) and `## Status` (editing rules in
  `tickets.md`), then post a `Changed` comment. Drop it from `ELIGIBLE` (and the rest of its chain, as
  `BLOCKED`: `not eligible: needs-info`). When in doubt, it's eligible — the implementer runs the full readiness gate.
- **Model** — the script's `model` upgraded to `opus` for a clearly cross-package ticket.

**Overlap check** — tickets run in parallel off the same `main`. If two eligible tickets clearly touch the
same code (same package/type per their descriptions (incl. `## Plan`)), keep the higher-priority (then
older) one and mark the other **held — overlaps #nnn**. Tickets in the same chain never hold each other;
a held chain ticket holds the rest of its chain too.

**In-flight check** — for each eligible ticket (not a continuing one), run the collision check in
`tickets.md` ("In-flight work & the collision check"), ignoring tickets in its own chain. A hit that is
in-flight → **held — overlaps #nnn (in-flight)**, and the rest of its chain with it. Write nothing.

Order `ELIGIBLE` by priority (highest first), then oldest.

**Result** — your final reply must be only this block:

```
BASE_SHA: <git -C <repo> rev-parse --short origin/main>
ELIGIBLE:
#nnn | <branch> | <title> | <plan approved | continues <PR url> | -> | base <branch> | <sonnet|opus>
CHAINS: <A>B>C per line, in work order; an open-PR head as "#nnn (PR <url>)"; or "none">
NEEDS_INFO: <#nnn — gist of what was asked, or "none">
BLOCKED: <#nnn — blocked by #mmm (<status>)[, …] · fix: <chain> | fix: none — <reason> [· partial: <chain>] (fix/partial only for argument tickets), or "none">
SKIPPED: <#nnn — <label>, or "none">
HELD: <#nnn — overlaps #mmm [(in-flight)], or "none">
```
