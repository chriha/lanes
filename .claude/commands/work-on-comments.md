---
allowed-tools: Bash(gh pr list:*), Bash(gh pr view:*), Bash(gh pr diff:*), Bash(gh api:*), Bash(gh issue list:*), Bash(gh issue create:*), Bash(gh issue comment:*), Bash(git branch:*), Bash(git log:*), Bash(git diff:*), Bash(git add:*), Bash(git commit:*), Bash(make test:*), Bash(go test:*), Bash(go vet:*), Bash(gofmt:*), AskUserQuestion, Agent
description: Fetch open PR review comments and work through them systematically
---

Look up the current branch's open pull request on GitHub and fetch all review comments.

**Step 1 — Fetch & categorize**

Run these in parallel:
- Get the current branch: `git branch --show-current`
- Get the branch diff: `git diff main...HEAD`

Then fetch the open PR's review threads with these sequential steps:
- PR number for the branch: `gh pr view --json number,url,title,isDraft --jq .` (it resolves the current branch). No open PR → say so and stop.
- Unresolved review threads (resolution state is only available through GraphQL):
  ```
  gh api graphql -F owner='{owner}' -F repo='{repo}' -F number=<n> -f query='
    query($owner: String!, $repo: String!, $number: Int!) {
      repository(owner: $owner, name: $repo) {
        pullRequest(number: $number) {
          reviewThreads(first: 100) {
            nodes {
              id isResolved isOutdated path line
              comments(first: 20) { nodes { databaseId author { login __typename } body url createdAt } }
            }
          }
        }
      }
    }'
  ```
  `{owner}`/`{repo}` are filled in by `gh`. More than 100 threads → page with `after`.
- Top-level review comments left as a conversation comment (no thread state): `gh pr view <n> --json comments,reviews`.
  Treat a human comment that asks for a change as open; ignore approvals and pure acknowledgements.

Filter to only **unresolved** threads with human-authored notes (`author.__typename == "User"`, not the PR author
replying to themselves). Ignore bot/system notes and already-resolved threads. Keep outdated threads (`isOutdated`) but
say so — the code they point at may have changed.

If there are no open comments, say so and stop here.

**Verify first.** Before categorizing, give every comment a verdict by reading the code it references (and its
callers where the claim depends on them). Run one sub-agent per comment (in parallel), model `sonnet`; use `opus`
when the claim is about design, concurrency, security or data loss. Each returns a verdict with file:line evidence:
- ✅ **Valid** — the claim holds
- ◐ **Partly valid** — holds in part; say which part
- ❌ **Invalid** — the code contradicts the claim
- ❓ **Unverifiable** — cannot be confirmed from the code

Invalid and Unverifiable comments never trigger a code change: categorize them as 💬 Clarify with a drafted reply
(cite the evidence, or state what is needed to verify).

Group every other comment into one of three categories:
- 🟢 **Quick fix** — trivial change, low risk (naming, formatting, simple logic) → fixer model `sonnet`
- 🔴 **Complex** — requires design thinking or significant refactoring, or a Partly valid / disputed claim → fixer model `opus`
- 💬 **Clarify** — ambiguous intent, or best resolved with a reply

Then scan the branch diff for the *same class of issues* the reviewers flagged but missed elsewhere. Add those as additional items, noting they were self-identified.

**Step 2 — Overview + quick-fix plan**

Present:
1. Short summary of the PR's overall review status
2. Categorized comment list (file + line reference, verdict with evidence, and chosen model for each)
3. For each quick fix: one sentence on the approach
4. For each Invalid/Unverifiable comment: the drafted reply

Then ask: "Shall I work on all the quick fixes?"

**Step 3 — Quick fixes (after approval)**

Work through each quick fix, each in a `sonnet` sub-agent. One focused commit per fix with a clear Conventional
Commit message (`.claude/work-tickets/git-rules.md`), `make test` and `gofmt -l .` clean before each commit.
Report when done. Nothing is pushed and no thread is resolved or replied to without the human saying so.

**Step 4 — Complex tasks, one at a time**

For each complex task:
- Run in an `opus` sub-agent (or on the session model if it is already opus)
- Read all relevant code before forming an opinion
- Present 1–3 concrete approaches with trade-offs
- Give a clear recommendation and reasoning

Wait for a choice. Implement it, summarize, then wait for confirmation before moving to the next. Repeat until all complex tasks are done.

**Step 5 — Clarifications, one at a time**

For each remaining comment:
- Quote the reviewer's comment and its verdict
- Provide context: what the code does, relevant types/functions with line numbers
- Show the drafted reply (Invalid/Unverifiable) and ask how to respond or resolve. To post a reply:
  `gh api repos/{owner}/{repo}/pulls/<n>/comments/<databaseId of the thread's first comment>/replies -f body='…'`;
  to resolve the thread: the `resolveReviewThread(input: {threadId: "<id>"})` GraphQL mutation. Only on request.

Continue one by one until all are addressed.

**Step 6 — Harvest into the harness**

Run the harness harvest (`.claude/work-tickets/tickets.md`, "Harness harvest") over every comment resolved in this session, including the self-identified ones. Skip if none were resolved.
