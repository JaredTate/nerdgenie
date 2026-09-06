#!/usr/bin/env bash
# Passes when the suite is green and the test file is byte for byte what the fixture holds.
set -u
WORK="$1"
HERE="$(cd "$(dirname "$0")/.." && pwd)"
cmp -s "$HERE/fixtures/03-fix-the-test/test/words.test.js" "$WORK/test/words.test.js" || { echo "the test file was changed"; exit 1; }
out="$(cd "$WORK" && node --test test/*.test.js 2>&1)"
echo "$out" | grep -Eq 'fail 0' || { echo "$out" | tail -8; exit 1; }
echo "the fix passes the untouched tests"
