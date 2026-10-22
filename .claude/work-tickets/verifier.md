# Verifier brief (/work-tickets)

Your prompt gives the finding's short title, the source `#nnn`, `<repo>`, `<wt>`, `<report>` (full text
under `## Findings`) and the titles of findings already in flight. Follow `tickets.md` next to this file.

- **Read-only**: verify against `<repo>/.user/worktrees/_main` (a detached checkout of `origin/main`) — `cd`
  there for every command. If the finding only exists in the ticket branch's changes, look in `<wt>` too.
  Don't modify code, commit or touch git state. Running a focused test (`go test -run … ./pkg`) to confirm a bug
  is fine.
- **Duplicate check**: `gh issue list --state all --search "<keywords>" --json number,title,state` with a few
  keyword queries (closed ones included); compare with the in-flight titles too.
- **Verify** against the code: real, reproducible on `main`, not already handled or intended? Code-style /
  lint / `go vet` nits are **invalid**.
- **Verdict:**
  - **invalid** or **duplicate #mmm** → create nothing.
  - **confirmed** → `gh issue create --label triage --label bug|improvement --title … --body-file …` — unassigned,
    no priority. Short imperative title; description per `tickets.md` (`## Status` — one line, e.g. "In triage,
    found while working #nnn"; `## Problem` with file:line evidence, what happens vs. what should;
    `## Acceptance criteria`; `## Reference` → `Found while working #nnn`). Keep it small.
- **Persist the verdict** — `gh issue comment <source nnn> --body …` with the body being
  `<!-- finding-verdict -->` on the first line, then the RESULT line below verbatim (measures finding precision
  later).
- **Result** — your final reply must be only this line:
  `RESULT F-<source nnn>: confirmed|duplicate|invalid · <created #id + url | duplicate of #mmm | none> · <reason, ≤ 15 words>`
