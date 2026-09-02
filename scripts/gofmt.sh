#!/usr/bin/env bash
# Check that every Go file in this repository is gofmt clean.
#
# This is the step `make check` used to hold inline, moved here unchanged so that
# it can be tested like the other gate scripts.
set -uo pipefail

cd "$(dirname "$0")/.."

unformatted=$(gofmt -l $(find . -name '*.go' -not -path '*/testdata/*'))
if [ -n "$unformatted" ]; then
	echo "these files are not gofmt clean; run gofmt -w on them:"
	echo "$unformatted"
	exit 1
fi
