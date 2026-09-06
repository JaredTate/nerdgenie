#!/usr/bin/env bash
# Passes when add.js exists, the suite has at least three tests, and node --test is green.
set -u
WORK="$1"
[ -f "$WORK/add.js" ] || { echo "no add.js"; exit 1; }
[ -f "$WORK/test/add.test.js" ] || { echo "no test/add.test.js"; exit 1; }
out="$(cd "$WORK" && node --test test/*.test.js 2>&1)"
echo "$out" | grep -Eq 'fail 0' || { echo "$out" | tail -8; exit 1; }
passed="$(echo "$out" | grep -Eo 'pass [0-9]+' | head -1 | awk '{print $2}')"
[ "${passed:-0}" -ge 3 ] || { echo "only $passed tests"; exit 1; }
echo "$passed tests green"
