#!/bin/sh
# Fail if any commit in <base>..HEAD carries a Co-Authored-By line other than the one in .claude/harness.md.
# Usage: scripts/check-trailers.sh [base]   (default origin/main)
base=${1:-origin/main}
want=$(sed -n 's/^\(Co-Authored-By: .*\)$/\1/p' "$(dirname "$0")/../.claude/harness.md" | head -n 1)
[ -n "$want" ] || { echo "no Co-Authored-By line in .claude/harness.md" >&2; exit 2; }
bad=0
for c in $(git rev-list "$base..HEAD"); do
  git show -s --format=%B "$c" | grep -i '^co-authored-by:' | grep -qvxF "$want" || continue
  echo "$(git show -s --format='%h %s' "$c"): Co-Authored-By differs from '$want'" >&2
  bad=1
done
exit $bad
