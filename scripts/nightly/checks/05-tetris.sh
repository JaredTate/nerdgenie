#!/usr/bin/env bash
# Passes when the suite is green and the log holds a page answer naming a playing state.
set -u
WORK="$1"
LOG="$2"
# The ask lets the model choose its test runner, so the suite is whatever
# npm test runs, and green is its exit code.
if ! out="$(cd "$WORK" && npm test 2>&1)"; then echo "$out" | tail -8; exit 1; fi
python3 - "$LOG" <<'PY'
import sqlite3, json, sys
c = sqlite3.connect("file:" + sys.argv[1] + "?mode=ro", uri=True)
for (body,) in c.execute("select body from events where kind='tool result' order by sequence"):
    text = json.loads(body).get("text") or ""
    if "the page answered:" in text and ("PLAYING" in text or "playing" in text):
        print("the page answered a playing state"); sys.exit(0)
print("no page answer named a playing state"); sys.exit(1)
PY
