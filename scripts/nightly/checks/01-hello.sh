#!/usr/bin/env bash
# Passes when hello.py exists and prints a date in ISO form.
set -u
WORK="$1"
[ -f "$WORK/hello.py" ] || { echo "no hello.py"; exit 1; }
out="$(cd "$WORK" && python3 hello.py 2>&1 | tail -1)"
echo "$out" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' || { echo "hello.py printed: $out"; exit 1; }
echo "hello.py prints $out"
