#!/usr/bin/env bash
# Run every fuzz target in the repository for the duration given as the one
# argument, for example `scripts/fuzz.sh 5s` or `scripts/fuzz.sh 60s`.
#
# Go runs one fuzz target per invocation of `go test`, so this script finds the
# targets first and then runs them one at a time. `make test` calls it with a
# five-second smoke and `make fuzz` calls it with a minute each.
set -euo pipefail

if [ "$#" -ne 1 ]; then
	echo "usage: scripts/fuzz.sh <duration>, for example: scripts/fuzz.sh 5s" >&2
	exit 2
fi

duration="$1"
cd "$(dirname "$0")/.."

found=0

# `go test -list` prints one name per line plus a trailing "ok <package>" line
# for each package, so read the package and its target names together.
while read -r package; do
	targets="$(go test -list '^Fuzz' "$package" 2>/dev/null | grep '^Fuzz' || true)"
	if [ -z "$targets" ]; then
		continue
	fi
	while read -r target; do
		[ -z "$target" ] && continue
		found=$((found + 1))
		echo "fuzzing $target in $package for $duration"
		go test "$package" -run '^$' -fuzz "^${target}\$" -fuzztime "$duration"
	done <<<"$targets"
done < <(go list ./...)

if [ "$found" -eq 0 ]; then
	echo "no fuzz targets found yet; nothing to fuzz"
fi
