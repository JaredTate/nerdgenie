#!/usr/bin/env bash
# Passes when index.html exists and the log holds a browser_read that asked the page and was answered 1.
set -u
WORK="$1"
LOG="$2"
[ -f "$WORK/index.html" ] || { echo "no index.html"; exit 1; }
python3 - "$LOG" <<'PY'
import sqlite3, json, sys
c = sqlite3.connect("file:" + sys.argv[1] + "?mode=ro", uri=True)
asked = False
for (body,) in c.execute("select body from events where kind='tool call' order by sequence"):
    call = json.loads(body)
    if call.get("Name") == "browser_read" and (call.get("Input") or {}).get("ask"):
        asked = True
answered = False
for (body,) in c.execute("select body from events where kind='tool result' order by sequence"):
    text = json.loads(body).get("text") or ""
    if "the page answered: 1" in text:
        answered = True
if not asked:
    print("the page was never asked"); sys.exit(1)
if not answered:
    print("the page never answered 1"); sys.exit(1)
print("the page was asked and answered 1")
PY
