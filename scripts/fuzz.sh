#!/bin/sh
# Fuzzes every Fuzz target in the module for the given time each. With a shard
# index and count, it fuzzes only every count-th target, so CI can spread the
# targets over parallel jobs: scripts/fuzz.sh 5s 2 8 runs shard 2 of 0..7.
set -eu
time=${1:?usage: fuzz.sh <fuzztime> [shard shards]}
shard=${2:-0}
shards=${3:-1}
grep -rEo --include='*_test.go' '^func Fuzz[A-Za-z0-9_]*\(' . | sed -E 's|^\./(.*)/[^/]*:func (Fuzz[A-Za-z0-9_]*)\($|\1 \2|' | sort |
	awk -v s="$shard" -v n="$shards" '(NR - 1) % n == s' > "${TMPDIR:-/tmp}/fuzz-targets.$$"
status=0
while read -r dir target; do
	echo "::group::$dir $target"
	# A short -fuzztime can end on "context deadline exceeded" while a worker
	# minimizes a new input; only a reported failing input fails.
	if ! out=$(go test -run '^$' -fuzz "^${target}\$" -fuzztime "$time" "./$dir" 2>&1); then
		echo "$out"
		if echo "$out" | grep -q "Failing input" || ! echo "$out" | grep -q "context deadline exceeded"; then
			status=1
		else
			echo "::notice::$target ended on the fuzzing deadline without a failing input"
		fi
	fi
	echo "::endgroup::"
done < "${TMPDIR:-/tmp}/fuzz-targets.$$"
rm -f "${TMPDIR:-/tmp}/fuzz-targets.$$"
exit "$status"
