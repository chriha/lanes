#!/usr/bin/env bash
#
# /work-tickets Step 0: fetch/prune, clean merged or deleted worktrees, unstack PRs whose base merged,
# refresh the _main reference worktree. Safe to run standalone and repeatedly.
# Usage: sync.sh <repo>
# Output: `issue-nnn merged|closed|deleted` (cleaned), `UNSTACKED: <url>`, `UNSTACK_CONFLICT: <url> [(reason)]`.
#
set -euo pipefail

repo="${1:?usage: sync.sh <repo>}"
tool="$repo/scripts/work-tickets/worktree.sh"
cd "$repo"

mkdir -p .user/worktrees .user/reports
git fetch -q origin --prune

"$tool" clean

# Source branch -> worktree path
worktree_of() {
    git worktree list --porcelain | awk -v b="refs/heads/$1" '/^worktree /{p=$2} $0=="branch " b{print p}'
}

gh pr list --author @me --state open --json number,headRefName,baseRefName,url 2>/dev/null \
    | jq -r '.[] | select(.baseRefName != "main") | "\(.number) \(.headRefName) \(.baseRefName) \(.url)"' \
    | { while read -r number source target url; do
        base_state="$(gh pr list --head "$target" --state all --json state 2>/dev/null | jq -r '.[0].state // "none"')"
        case "$base_state" in
            MERGED)
                wt="$(worktree_of "$source")"
                if [ -z "$wt" ]; then
                    echo "UNSTACK_CONFLICT: $url (no worktree)"
                elif git -C "$wt" merge origin/main --no-edit >/dev/null 2>&1; then
                    git -C "$wt" push -q && gh pr edit "$number" --base main >/dev/null && echo "UNSTACKED: $url"
                else
                    git -C "$wt" merge --abort
                    echo "UNSTACK_CONFLICT: $url"
                fi
                ;;
            CLOSED) echo "UNSTACK_CONFLICT: $url (base closed)" ;;
        esac
    done; } || true

"$tool" add _main >/dev/null
