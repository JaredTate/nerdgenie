#!/usr/bin/env bash
# Measure line coverage for every package under internal/ and scripts/, print a
# table, and fail when a package with statements is under its threshold.
#
# The default threshold is ninety percent. internal/tui is seventy, because the
# terminal screen is tested by a person looking at it, which docs/WORK_PLAN.md
# Part 1 says under "Definition of done". cmd/coeus is measured and printed but
# never gated: it is the wiring the orchestrator owns, and a subcommand table is
# proved by the functional suite rather than by unit tests.
set -euo pipefail

cd "$(dirname "$0")/.."

default_threshold=90
tui_threshold=70

printf '%-56s %8s %8s  %s\n' "PACKAGE" "COVERED" "NEEDED" "RESULT"

failed=0

# `go test -cover` prints one line per package. A package whose files contain no
# executable statements prints "[no statements]" instead of a percentage, and
# passes: contract is mostly interfaces and struct types.
while read -r line; do
	package="$(echo "$line" | awk '{ print $2 }')"

	if echo "$line" | grep -q '^FAIL'; then
		printf '%-56s %8s %8s  %s\n' "$package" "-" "-" "TESTS FAILED"
		failed=1
		continue
	fi

	case "$package" in
	*/internal/tui) threshold="$tui_threshold" ;;
	*) threshold="$default_threshold" ;;
	esac

	if echo "$line" | grep -q 'no statements'; then
		printf '%-56s %8s %8s  %s\n' "$package" "none" "-" "no statements"
		continue
	fi

	if echo "$line" | grep -q 'no test files'; then
		printf '%-56s %8s %8s  %s\n' "$package" "none" "$threshold%" "NO TEST FILES"
		failed=1
		continue
	fi

	covered="$(echo "$line" | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p')"
	if [ -z "$covered" ]; then
		continue
	fi

	verdict="ok"
	case "$package" in
	*/cmd/coeus)
		verdict="not gated"
		;;
	*)
		if awk "BEGIN { exit !($covered < $threshold) }"; then
			verdict="UNDER THRESHOLD"
			failed=1
		fi
		;;
	esac

	printf '%-56s %7s%% %7s%%  %s\n' "$package" "$covered" "$threshold" "$verdict"
done < <(go test -tags integration -cover ./internal/... ./scripts/... ./cmd/... 2>&1 | grep -E '^(ok|FAIL|---|\?)' || true)

if [ "$failed" -ne 0 ]; then
	echo
	echo "coverage is below the threshold; add tests for the packages marked above"
	exit 1
fi
