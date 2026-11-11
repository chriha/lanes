# Adapter: GitHub (issues + pull requests, `gh` CLI)

Tracker **and** forge. `<n>` = issue/PR number, referenced as `#<n>`. Ticket id regex: `#?(\d+)`. Worktree name: `issue-<n>`.
Closing keyword: `Closes #<n>` in the PR description (GitHub closes the ticket when the PR reaches the default branch).
Reference form (non-final slice of a multi-PR plan, must not close the ticket): `Refs #<n>`.
Branch ↔ ticket: branch `<type>/<n>-<slug>`; the leading number after `<type>/` is the ticket.

## Tracker operations

| Operation | Command |
|---|---|
| read-ticket | `gh issue view <n> --json number,title,body,labels,comments,assignees,state` |
| list-tickets (mine, open) | `gh issue list --assignee @me --state open --json number,title,labels,createdAt` |
| list-triage | `gh issue list --label triage --state open --json number,title,labels,createdAt,author` |
| search-tickets | `gh issue list --state open\|all --search "<kw>" --json number,title,state,labels` |
| create-ticket | `gh issue create --title … --body-file <f> --label … --assignee @me` (omit `--assignee` for triage issues) |
| edit-description | read body to a scratchpad file → change only your sections → `gh issue edit <n> --body-file <f>` (re-read right before writing) |
| comment | `gh issue comment <n> --body …` |
| add-label / remove-label | `gh issue edit <n> --add-label … / --remove-label …` |
| create-label (once) | `gh label create <name> --color <hex>` |
| assign-me | `gh issue edit <n> --add-assignee @me` |
| set-priority | `gh issue edit <n> --add-label priority:<1-4>` (and `--remove-label` any other `priority:*`) |
| accept-triage | `gh issue edit <n> --remove-label triage --add-assignee @me` |
| close-not-planned | `gh issue close <n> --reason "not planned"` |
| close-duplicate | `comment` `Duplicate of #<m>` → `gh issue close <n> --reason "not planned"` |
| blockers-read | `gh api repos/{owner}/{repo}/issues/<n>/dependencies/blocked_by` |
| blockers-add | `gh api -X POST repos/{owner}/{repo}/issues/<n>/dependencies/blocked_by -F issue_id=<blocker numeric id>`; numeric id = `gh api repos/{owner}/{repo}/issues/<blocker> --jq .id` |
| related | no relation type: write `Related: #<m>` under `## Reference` |
| in-flight | an open issue with an open PR whose branch is `<type>/<n>-*` (`gh pr list --state open --json number,title,headRefName,body`); no label |
| collision-search | `gh pr list --state open --json number,title,headRefName` + 2–3 `search-tickets` on the `## Touches` names |
| ticket-of-pr | `gh pr view <pr> --json closingIssuesReferences`, then `#<n>`/`Closes #<n>`/`Refs #<n>` in the commit log, then the branch number |
| harness-issues | `gh issue list --label agent-harness --state all --search "<key>" --json number,title,state` |
| status-of-ticket | the issue's PR state: open PR → in review; merged → done (GitHub closes it) |

Priority: labels `priority:1` (urgent) … `priority:4` (low); none sorts last. Type labels: `bug`, `feature`, `improvement`.
Special labels: `on hold`, `ready-for-human` (not for agents) · `needs-info` · `planning` · `triage` · `agent-harness`.
Dedupe key for harvest issues: first line of the body `Key: <key>`.

## Forge operations

| Operation | Command |
|---|---|
| list-prs (open) | `gh pr list --state open --json number,title,headRefName,baseRefName,url,author,isDraft,labels` |
| pr-of-branch | `gh pr list --head <branch> --state all --json number,state,url,baseRefName` |
| view-pr | `gh pr view <n> --json title,body,headRefName,baseRefName,headRefOid,closingIssuesReferences,isDraft,state,url` |
| pr-diff | `gh pr diff <n>` |
| create-draft-pr | `gh pr create --draft --base <base> --title "<subject of the first non-stub commit>" --body-file <f>` |
| edit-pr-base | `gh pr edit <n> --base <base>` |
| pr-template | `.github/pull_request_template.md` (or `.github/PULL_REQUEST_TEMPLATE.md`) if present |
| comment-pr (general) | `gh pr comment <n> --body …` |
| comment-pr (inline) | `gh api repos/{owner}/{repo}/pulls/<n>/comments -f body='…' -f commit_id=<headRefOid> -f path=<file> -F line=<line> -f side=RIGHT` |
| review-threads | `gh api graphql` (`reviewThreads` with `isResolved`, `comments`) |
| reply-to-comment | `gh api repos/{owner}/{repo}/pulls/<n>/comments/<id>/replies -f body=…` |
| resolve-thread | `gh api graphql -f query='mutation{resolveReviewThread(input:{threadId:"<id>"}){thread{id}}}'` |
| merged / closed state | `gh pr list --head <branch> --state all --json state` → `MERGED` / `CLOSED` |
| pr-mine-open | `gh pr list --author @me --state open …` |

Rules: PRs are always **draft** when agents open them; nobody runs `gh pr ready`, `gh pr merge` or approves.
Pushes need the sandbox off.

## Allow-list fragment (for `.claude/settings.json`)
`Bash(gh issue list:*)`, `Bash(gh issue view:*)`, `Bash(gh pr list:*)`, `Bash(gh pr view:*)`, `Bash(gh pr diff:*)`, `Bash(gh label list:*)` (read-only); writers (`gh issue create|edit|comment`, `gh pr create|edit|comment`, `gh api`, `gh label create`) are granted per skill via `allowed-tools`.
