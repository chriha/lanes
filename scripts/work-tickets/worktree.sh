#!/usr/bin/env bash
#
# The single worktree tool for /work-tickets (worktrees live under <repo>/.user/worktrees).
# Usage:  worktree.sh add <issue-nnn> [<base>] [--branch <name>]   create (or reuse) the ticket worktree
#         worktree.sh add _main                                     create/refresh the detached origin/main reference worktree
#         worktree.sh clean                                         remove worktrees whose PR is merged/closed or whose branch is deleted on origin
# `add` prints the worktree path. `clean` prints `issue-nnn merged|closed|deleted` per removal.
# A new branch needs --branch; a continuing one (local or on origin, named <type>/nnn-*) is found by itself.
#
set -euo pipefail

repo="$(dirname "$(git -C "$(dirname "$0")" rev-parse --path-format=absolute --git-common-dir)")"
root="$repo/.user/worktrees"

add() {
    local name="${1:?usage: worktree.sh add <issue-nnn> [<base>] [--branch <name>]}" base=main branch=""
    shift
    while [ $# -gt 0 ]; do
        case "$1" in
            --branch) branch="${2:?--branch needs a name}"; shift 2 ;;
            *) base="$1"; shift ;;
        esac
    done
    name="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')"
    local wt="$root/$name" number="${name#issue-}"

    mkdir -p "$root" "$repo/.user/reports"
    git -C "$repo" fetch -q origin "$base" || echo "! fetch failed, using the last known origin/$base" >&2

    if [ "$name" = _main ]; then
        if [ -d "$wt" ]; then
            git -C "$wt" checkout -q --detach origin/main
        else
            git -C "$repo" worktree add -q --detach "$wt" origin/main
        fi
    elif [ ! -d "$wt" ]; then
        if [ -z "$branch" ]; then
            branch="$(git -C "$repo" for-each-ref --format='%(refname:short)' "refs/heads/*/$number-*" | head -n1)"
        fi
        if [ -z "$branch" ]; then
            branch="$(git -C "$repo" ls-remote --heads origin "*/$number-*" | head -n1 | sed 's#.*refs/heads/##')"
        fi
        [ -n "$branch" ] || { echo "✗ no branch for $name; pass --branch <name>" >&2; exit 1; }
        if git -C "$repo" show-ref --verify --quiet "refs/heads/$branch"; then
            git -C "$repo" worktree add -q "$wt" "$branch"
        elif git -C "$repo" ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1; then
            git -C "$repo" fetch -q origin "$branch"
            git -C "$repo" worktree add -q -b "$branch" "$wt" "origin/$branch"
        else
            git -C "$repo" worktree add -q -b "$branch" "$wt" "origin/$base"
        fi
    fi

    echo "$wt"
}

clean() {
    local wt name branch state pr
    for wt in "$root"/issue-*/; do
        [ -d "$wt" ] || continue
        wt="${wt%/}"
        name="$(basename "$wt")"
        branch="$(git -C "$wt" branch --show-current)"
        [ -n "$branch" ] || continue

        pr="$(gh pr list --head "$branch" --state all --json state 2>/dev/null | jq -r '.[0].state // "none"' 2>/dev/null || echo unknown)"
        state=""
        case "$pr" in
            MERGED) state=merged ;;
            CLOSED) state=closed ;;
        esac
        # deleted = it was pushed with -u (upstream is its own origin branch) but origin no longer has it
        if [ -z "$state" ] && [ "$(git -C "$repo" config --get "branch.$branch.merge")" = "refs/heads/$branch" ] \
            && ! git -C "$repo" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
            state=deleted
        fi
        [ -n "$state" ] || continue

        [ -z "$(git -C "$wt" status --porcelain)" ] || continue
        if git -C "$repo" show-ref --verify --quiet "refs/remotes/origin/$branch" \
            && [ "$(git -C "$wt" rev-list --count "origin/$branch..HEAD")" != 0 ]; then
            continue
        fi

        git -C "$repo" worktree remove --force "$wt"
        git -C "$repo" branch -D "$branch" >/dev/null
        rm -f "$repo/.user/reports/$name.md"
        echo "$name $state"
    done
}

case "${1:-}" in
    add) shift; add "$@" ;;
    clean) clean ;;
    *) echo "usage: worktree.sh add <issue-nnn> [<base>] [--branch <name>] | clean" >&2; exit 2 ;;
esac
