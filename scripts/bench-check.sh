#!/bin/sh
# Compare the median ns/op of a benchmark in two `go test -bench` outputs and fail when the head is more than
# LIMIT times the base (the startup gate of the ci workflow's bench job, see docs/design.md § Startup). Prints a
# Markdown table for the job summary. A base without a result (missing file, or the benchmark did not compile or
# run there) is reported, not failed; a head without a result fails.
#
# Usage: scripts/bench-check.sh <base.txt> <head.txt>
#   BENCH=BenchmarkTimeToFirstList   the benchmark to compare
#   LIMIT=2                          the largest allowed head/base ratio
set -eu

[ $# -eq 2 ] || {
	echo "usage: $0 <base.txt> <head.txt>" >&2
	exit 2
}
base=$1 head=$2
bench=${BENCH:-BenchmarkTimeToFirstList}
limit=${LIMIT:-2}

# median ns/op and run count of $bench in file $1 ("" when it has no result)
median() {
	[ -r "$1" ] || return 0
	awk -v b="$bench" '
		$0 ~ "^" b "(-[0-9]+)?[ \t]" { for (i = 2; i < NF; i++) if ($(i + 1) == "ns/op") print $i }
	' "$1" | sort -n | awk '
		{ v[NR] = $1 }
		END {
			if (NR == 0) exit
			m = NR % 2 ? v[(NR + 1) / 2] : (v[NR / 2] + v[NR / 2 + 1]) / 2
			printf "%.0f %d\n", m, NR
		}'
}

h=$(median "$head")
b=$(median "$base")

echo "### Startup benchmark: $bench"
echo
if [ -z "$h" ]; then
	echo "FAIL: the head has no $bench result"
	exit 1
fi
echo "| | median ns/op | runs |"
echo "|---|---|---|"
[ -z "$b" ] || echo "| base | ${b% *} | ${b#* } |"
echo "| head | ${h% *} | ${h#* } |"
echo
if [ -z "$b" ]; then
	echo "no comparison: the base has no $bench result"
	exit 0
fi
awk -v b="${b% *}" -v h="${h% *}" -v l="$limit" 'BEGIN {
	r = h / b
	if (r > l) { printf "FAIL: the head is %.2fx the base (limit %sx)\n", r, l; exit 1 }
	printf "ok: the head is %.2fx the base (limit %sx)\n", r, l
}'
