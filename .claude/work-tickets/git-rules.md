# Git rules (/work-tickets)

- Never touch the main checkout's branch or working tree — all code work happens in worktrees.
- Branch: `<type>/<nnn>-<slug>` (`fix/57-short-desc`), lowercase only, `<nnn>-<slug>` (everything after
  `<type>/`) ≤ 60 chars, cut from `origin/<base>` — `main`, or the branch it's stacked on when the prompt gives one.
  `<type>` ∈ `fix|feature|refactor|chore`.
- Commits: Conventional Commits, `<type>: <what>` with `<type>` ∈ `feat|fix|docs|style|refactor|perf|test|chore|ci`,
  optional single-word scope, ≤ 72 chars in total.
- Push only that branch, only to `origin`. Never push to `main` or to the base branch, never force-push,
  never rewrite history (`rebase`, `reset --hard`, `commit --amend`, `stash`). Integrate a newer base with
  `git merge origin/<base>`.
- Pushes need the sandbox off.
- PRs are always **draft** and stay draft — the human marks them ready. Nobody runs `gh pr ready`,
  `gh pr merge`, or approves.
