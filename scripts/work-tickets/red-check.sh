#!/usr/bin/env bash
# Usage: red-check.sh <worktree> <base>
# Proves the changed Go tests fail without the non-test diff and pass with it.
# Exit: 0 red then green, 1 never red, 2 not green with the fix, 3 no test changes, 4 setup error, 5 red without a test failure.
# RED_CHECK_TEST_CMD overrides the test command (the package list is appended).
set -u

wt=${1:-}
base=${2:-}
if [ -z "$wt" ] || [ -z "$base" ] || ! cd "$wt" 2>/dev/null; then
    echo "red-check: usage: red-check.sh <worktree> <base>"
    exit 4
fi

range="origin/$base...HEAD"
tests=$(git diff --name-only --diff-filter=ACMR "$range" -- ':(glob)**/*_test.go') || { echo "red-check: cannot diff $range"; exit 4; }
if [ -z "$tests" ]; then
    echo "red-check: no test changes (exit 3)"
    exit 3
fi
# test files -> their packages
packages=$(printf '%s\n' "$tests" | xargs -n1 dirname | sort -u | sed 's#^#./#')

patch=$(mktemp)
out=$(mktemp)
status_before=$(git status --porcelain)
git diff --binary "$range" -- . ':(exclude,glob)**/*_test.go' > "$patch"
if [ ! -s "$patch" ]; then
    rm -f "$patch" "$out"
    echo "red-check: no non-test diff, tests cannot be red (exit 1)"
    exit 1
fi

reversed=0
cleanup() {
    if [ "$reversed" = 1 ]; then
        git apply --binary "$patch" && reversed=0
    fi
    rm -f "$patch" "$out"
    if [ "$(git status --porcelain)" != "$status_before" ]; then
        echo "red-check: WARNING working tree differs from before the run"
    fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

if [ -n "$status_before" ]; then
    echo "red-check: working tree not clean, commit first (exit 4)"
    exit 4
fi

read -r -a cmd <<< "${RED_CHECK_TEST_CMD:-go test -count=1}"

git apply -R --binary "$patch" || { echo "red-check: cannot reverse-apply the non-test diff (exit 4)"; exit 4; }
reversed=1
# shellcheck disable=SC2086
"${cmd[@]}" $packages 2>&1 | tee "$out"
red=${PIPESTATUS[0]}
# A real red has a failing test; a build or setup failure is not one.
grep -q -e '^--- FAIL' -e '^    --- FAIL' "$out"
has_failure=$?
grep -q -e '\[build failed\]' -e '\[setup failed\]' "$out"
build_failed=$?

git apply --binary "$patch" || { echo "red-check: cannot re-apply the non-test diff (exit 4)"; exit 4; }
reversed=0

if [ "$red" -eq 0 ]; then
    echo "red-check: tests passed without the fix, they cannot fail (exit 1)"
    exit 1
fi

if [ "$has_failure" -ne 0 ] || [ "$build_failed" -eq 0 ]; then
    echo "red-check: tests did not compile or errored without a failing test, not a valid red (exit 5)"
    exit 5
fi

# shellcheck disable=SC2086
"${cmd[@]}" $packages
green=$?
if [ "$green" -ne 0 ]; then
    echo "red-check: tests not green with the fix (exit 2)"
    exit 2
fi

echo "red-check: red without the fix, green with it (exit 0)"
exit 0
