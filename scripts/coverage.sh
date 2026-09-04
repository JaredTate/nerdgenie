#!/usr/bin/env bash
# Measure line coverage for every package under internal/, scripts/, and cmd/,
# print a table, and fail when a package is under its threshold, has no test
# file, or is missing from the report altogether.
#
# The default threshold is ninety percent. internal/tui is seventy, because the
# terminal screen is tested by a person looking at it, which docs/WORK_PLAN.md
# Part 1 says under "Definition of done". cmd/nerdgenie is measured and printed but
# never gated: it is the wiring the orchestrator owns, and a subcommand table is
# proved by the functional suite rather than by unit tests.
#
# The loop is driven by `go list` rather than by the test output, because `go
# test -cover` prints an untested package as a tab-leading line that is easy to
# filter away by accident. A package that go list names and the report does not
# mention is a failure, not a silence.
set -euo pipefail

cd "$(dirname "$0")/.."

default_threshold=90
tui_threshold=70
roots=(./internal/... ./scripts/... ./cmd/...)

# Nothing here starts a real Chrome any more, because the browser and desktop
# packages are measured without the integration tag below. Should that ever
# change, ask for the browser with no window, so that running the gate cannot put
# a window on the screen of whoever is running it: internal/browser reads
# NERDGENIE_HEADLESS_TESTS and `make test-browser` sets it.

# The packages that must be measured. Every one of them gets a row below, and a
# package with no row fails the gate.
packages="$(go list "${roots[@]}")"

# Run every test once with coverage. A failing test still leaves a report worth
# reading, so the failure is remembered and reported per package rather than
# ending the script here.
# The browser package is measured without the integration tag, because its
# integration tests start a real Chrome, which belongs to "make test-browser"
# and never to a coverage run; the desktop's live tests gate themselves.
tests_failed=0
measured="$(go list "${roots[@]}" | grep -v '/internal/browser$')"
if ! report="$(go test -tags integration -cover $measured 2>&1)"; then
	tests_failed=1
fi
windowed_packages="$(go list "${roots[@]}" | grep -E '/internal/browser$' || true)"
if [ -n "$windowed_packages" ]; then
	if ! windowed="$(go test -cover $windowed_packages 2>&1)"; then
		tests_failed=1
	fi
	report="$(printf '%s\n%s\n' "$report" "$windowed")"
fi

printf '%-56s %8s %8s  %s\n' "PACKAGE" "COVERED" "NEEDED" "RESULT"

failed=0

for package in $packages; do
	case "$package" in
	*/internal/tui | */internal/tui/*) threshold="$tui_threshold" ;;
	*) threshold="$default_threshold" ;;
	esac

	# Find the one report line that names this package as a whole field, so that
	# internal/tui does not pick up the line for internal/tui/screen.
	line="$(printf '%s\n' "$report" | awk -v want="$package" '{ for (field = 1; field <= NF; field++) if ($field == want) { print; exit } }')"

	if [ -z "$line" ]; then
		printf '%-56s %8s %8s  %s\n' "$package" "-" "$threshold%" "NO RESULT"
		failed=1
		continue
	fi

	case "$line" in
	FAIL*)
		printf '%-56s %8s %8s  %s\n' "$package" "-" "-" "TESTS FAILED"
		failed=1
		continue
		;;
	esac

	# A package with no test file at all has no verdict word in front of its name,
	# so the first field on its line is the package itself. Under -cover that line
	# also carries "coverage: 0.0%", which reads like a badly tested package rather
	# than an untested one, so it is caught here first and named for what it is.
	verdict_word="$(printf '%s\n' "$line" | awk '{ print $1 }')"
	if [ "$verdict_word" = "$package" ]; then
		printf '%-56s %8s %8s  %s\n' "$package" "none" "$threshold%" "NO TEST FILES"
		failed=1
		continue
	fi

	# A package whose files hold no executable statements passes: internal/contract
	# is mostly interfaces and struct types.
	case "$line" in
	*"no statements"*)
		printf '%-56s %8s %8s  %s\n' "$package" "none" "-" "no statements"
		continue
		;;
	*"no test files"*)
		printf '%-56s %8s %8s  %s\n' "$package" "none" "$threshold%" "NO TEST FILES"
		failed=1
		continue
		;;
	esac

	covered="$(printf '%s\n' "$line" | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p')"
	if [ -z "$covered" ]; then
		printf '%-56s %8s %8s  %s\n' "$package" "-" "$threshold%" "NO COVERAGE READ"
		failed=1
		continue
	fi

	verdict="ok"
	case "$package" in
	*/cmd/nerdgenie)
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
done

if [ "$tests_failed" -ne 0 ]; then
	echo
	echo "the tests did not all pass, so the coverage numbers above are not the whole story:"
	printf '%s\n' "$report" | grep -E '^(FAIL|---)' || true
	failed=1
fi

if [ "$failed" -ne 0 ]; then
	echo
	echo "coverage is below the threshold; add tests for the packages marked above"
	exit 1
fi
