# lanes

A vim-style terminal UI (Go, Bubble Tea) that lists work items from providers (GitLab, or any command printing
JSON) and runs actions on them in tmux sessions. Read `README.md` for usage and `docs/design.md` for the design.

## Commands

```sh
make build    # ./bin/lanes
make test     # go vet ./... && go test ./...  (what CI runs)
gofmt -l .    # must print nothing
```

## Layout

Code is organised by concern under `internal/`: `config` (TOML, project overlay, in-app overrides), `provider`
(gitlab, command), `filter` (expr expressions), `session` (tmux, Claude hooks), `history`, `cache`, `item`, `ui`
(Bubble Tea), `app` (name constant). `scripts/work-tickets/` is tooling for the agent workflow below, not part
of the app.

## Conventions

- Dependencies: never run `go get` / `go mod tidy` or edit `go.mod` / `go.sum` by hand without being asked.
- Tests: table-driven, `t.TempDir()` / `t.Setenv` for state; never touch the user's real tmux server, config dir
  or network.
- The app is a Bubble Tea model: state shared with it changes through messages, not from goroutines.
- Renaming the app touches three places only (see `docs/design.md`).

## Git

- Branches: `<type>/<nnn>-<slug>` with `<type>` in `fix|feature|refactor|chore`, `<nnn>` the GitHub issue number.
- Commits: Conventional Commits (`fix: keep drafts out of the review view`), at most 72 chars.
- PRs: description contains `Closes #<nnn>`; agents open them as drafts and never merge or mark them ready.

## Agent workflow

Tickets are GitHub issues (`gh`). Commands in `.claude/commands/`: `/ticket` files or refines an issue,
`/work-tickets` works the open issues assigned to you in worktrees under `.user/` (gitignored) and opens draft
PRs, `/local-code-review` and `/work-on-comments` review and address feedback. Shapes, labels and briefs:
`.claude/work-tickets/`; review rules: `.claude/review-criteria.md`.
