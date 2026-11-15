# Providers

A provider produces the items of a view. Define one in `[provider.<name>]` and reference it from a view.

## gitlab

```toml
[provider.gitlab]
type = "gitlab"
# project = "group/subgroup/project"   # default: detected from the git `origin` of the current directory
```

Lists the project's **open merge requests** via `glab api graphql` (so it uses your existing `glab` login). The
fields are listed under [Filters](usage.md#filters). Fetching is **two-pass**: a fast query (~2 s) paints the list
immediately; a second pass (~9 s on large projects) adds who started which discussion
(`open_thread_by_me`), because GitLab's discussion query is slow. Until it finishes, `open_thread_by_me` keeps
its value from the previous refresh, so the list does not flicker. Only the 100 most recently updated open MRs
(and their first 100 discussions) are considered.

## github

```toml
[provider.github]
type = "github"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Lists the repository's **open pull requests**, drafts included (`draft = true`), via `gh api graphql` (so it uses
your existing `gh` login), in one query. Items get the same fields and badges as GitLab (see [Filters](usage.md#filters)),
with `number` instead of `iid`: `approved` is an approving review decision or any approving latest review,
`open_threads` counts unresolved review threads, `ci` is the head commit's check rollup (`success`, `failed`,
`running`), `conflicts` is a conflicting PR and `diverged` a branch behind its base. Only the 100 most recently
updated open PRs (and their first 100 review threads) are considered. Use it for GitHub Enterprise hosts, which
`auto` treats as GitLab.

## github-issues

```toml
[provider.issues]
type = "github-issues"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Lists the repository's **open issues** via `gh api graphql` (pull requests are not included), the 100 most
recently updated, each with its first 20 blockers, blocked issues and sub-issues. The ID is `#<number>`; `author` and `url` are set as usual
(the list shows the assignees instead of the author, then a priority strip), and a `blocked` badge marks issues with an open blocker.

Ticket fields:

| Field        | Type     | Meaning                                                              |
|--------------|----------|----------------------------------------------------------------------|
| `kind`       | string   | `ticket`                                                             |
| `number`     | number   | issue number                                                         |
| `status`     | string   | always `open` (only open issues are listed; filter on labels)        |
| `assignees`  | []string | assignee logins                                                      |
| `labels`     | []string | label names                                                          |
| `blocked`    | bool     | an issue it is blocked by (GitHub issue dependencies) is still open  |
| `blocked_by` | []string | the open blockers, e.g. `["#7"]`                                     |
| `blocking`   | []string | the open issues it blocks                                            |
| `related`    | []string | its open parent and sub-issues (GitHub has no "related" relation)    |
| `closed_links` | []string | closed blockers, blocked and related issues as `<relation>:<id>`, e.g. `["blocked_by:#9"]` |
| `priority`   | number   | from `priority:1`..`priority:4` labels (lowest wins), `0` if none    |
| `me`         | string   | your login                                                           |

A view of your unblocked issues that are not being planned, with a key that hands the selected one to Claude:

```toml
[[view]]
name     = "tickets"
provider = "issues"
filter   = 'me in assignees && !blocked && !("planning" in labels)'

[keymap]
w = "work"

[action.work]
run  = 'claude --settings {{.settings}} "/harness:work {{.number}}"'
when = 'kind == "ticket"'
```

## linear

```toml
[provider.linear]
type = "linear"
team = "ENG"                    # required: the team key (the prefix of its issue identifiers)
# token_env = "LINEAR_API_KEY"  # env var holding your Linear API key (default LINEAR_API_KEY)
```

Lists the team's **open issues** (state not completed or canceled) via the Linear GraphQL API, fetched
100 per request until all pages are read. The API key is read from the environment on every fetch and never written anywhere;
`lanes doctor` reports whether the variable is set. The ID is the identifier (`ENG-12`), `author` is the
creator (the list shows the assignee instead, then a priority strip), and badges show `blocked` and the workflow state.

Items carry the same [ticket fields](#github-issues) as `github-issues`, so views and actions work with either
tracker. The differences:

| Field        | Type     | Meaning on Linear                                                    |
|--------------|----------|----------------------------------------------------------------------|
| `key`        | string   | identifier, e.g. `ENG-12` (instead of `number`)                      |
| `status`     | string   | workflow state name, lower-cased, e.g. `todo`, `in progress`         |
| `assignees`  | []string | the assignee's display name, or empty                                |
| `blocked`    | bool     | an issue that blocks it is not completed or canceled                 |
| `blocked_by` | []string | those blocking issues, e.g. `["ENG-7"]`                              |
| `blocking`   | []string | the open issues it blocks                                            |
| `related`    | []string | open issues linked with a `related` relation, in either direction    |
| `closed_links` | []string | completed or canceled linked issues as `<relation>:<id>`, e.g. `["blocking:ENG-8"]` |
| `priority`   | number   | `0` none, `1` urgent, `2` high, `3` medium, `4` low                  |
| `me`         | string   | your display name                                                    |

For a `work` action, pass `{{.key}}` instead of `{{.number}}`.

## auto

```toml
[provider.forge]
type = "auto"
# project = "owner/repo"   # default: detected from the git `origin` of the current directory
```

Picks `github` when the `origin` remote of the current directory is on `github.com` (https or ssh URL) and
`gitlab` for any other host, on its first fetch each time `lanes` starts or reloads its config — so one global config works in
GitHub and GitLab repositories alike. Without an `origin` remote it fails with an error asking you to set the
type explicitly. This is what the default config uses.

## command

Turn anything into a list. The command runs through `$SHELL -c` and must print a JSON array:

```toml
[provider.todos]
type = "command"
command = "my-todo-exporter --json"

[[view]]
name = "todos"
provider = "todos"
filter = 'priority == "high"'
```

```json
[
  {
    "id": "T-12",
    "title": "Write the README",
    "url": "https://example.com/T-12",
    "author": "chris",
    "draft": false,
    "badges": [{ "text": "due today", "kind": "warn" }],
    "fields": { "priority": "high", "labels": ["docs"] }
  }
]
```

`id` must be unique within the provider (it identifies the item across refreshes and in session names). `kind`
of a badge is one of `ok`, `warn`, `bad`, `info`, `dim`. Everything under `fields` becomes available to filters
and templates.

For the [details](usage.md#preview-pane-and-item-history) screen (`d`), add an optional `detail` command. It runs through
`$SHELL -c` when you press `d`, never with the list refresh, gets the item's `id` in `$LANES_ITEM_ID` and must print
one JSON object; every key is optional:

```toml
[provider.todos]
type = "command"
command = "my-todo-exporter --json"
detail = 'my-todo-exporter --json --show "$LANES_ITEM_ID"'
```

```json
{
  "description": "Explain install, config and keys.",
  "jobs": [{ "name": "lint", "stage": "test", "status": "success", "duration": 42 }],
  "threads": [
    {
      "file": "README.md",
      "line": 12,
      "resolved": false,
      "comments": [{ "author": "sam", "body": "Mention brew?", "created": "2026-11-01T10:00:00Z" }]
    }
  ],
  "comments": [{ "author": "chris", "body": "Draft is up.", "created": "2026-11-02T09:30:00Z" }]
}
```

`jobs` fill the Pipeline section, grouped by `stage`; `status` is one of `success`, `failed`, `warning`, `running`,
`pending`, `skipped`, `canceled`, `manual`, and `duration` is in seconds. A thread's first comment opens it, the
rest are replies; `created` is an RFC 3339 time. Without `detail` the screen shows the item's fields only; a
failing command or invalid JSON shows an error line under them.
