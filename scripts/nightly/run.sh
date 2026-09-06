#!/usr/bin/env bash
# The nightly set: a few real asks run one after another against the local
# model on a home of their own, each followed by a check that says pass or
# fail without a person, and each measured by scripts/runreport. It is the
# number before and after for every change to the harness, and it grows by
# one ask every time a live task fails: that failure becomes the next check.
#
#   scripts/nightly/run.sh HOME_FOLDER [--with-tetris]
#
# HOME_FOLDER is an initialised Nerd Genie home (run "nerdgenie init" once).
# Never run this beside a live run: the local daemon has one slot, and a
# second task would sit behind the first. The serve is started on the home,
# yolo is turned on before the first ask, every ask gets a fresh work folder
# under the work folder beside the home, and the serve is stopped by its exact
# process id at the end. The results go to docs/nightly/<date>.md as one row
# per ask, and the last line of this script's output is the summary.
set -u
HOME_FOLDER="${1:?usage: run.sh HOME_FOLDER [--with-tetris]}"
WITH_TETRIS="${2:-}"
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
BINARY="$ROOT/bin/nerdgenie"
SOCKET="$HOME_FOLDER/run/agent.sock"
DATE="$(date +%F)"
OUT="$ROOT/docs/nightly/$DATE.md"
# The work folders live beside the home, under the work folder the home's
# config names as a sandbox root, never inside the home: the agent's home is
# outside every fence, and the first four runs put the work there, so the file
# tools refused every write and the model wrote through the shell instead.
WORKROOT="$(dirname "$HOME_FOLDER")/work/$DATE-$(date +%H%M)"
SCRATCH="$(mktemp -d)"
mkdir -p "$ROOT/docs/nightly" "$WORKROOT"

[ -x "$BINARY" ] || { echo "no binary at $BINARY; run make build first" >&2; exit 2; }
[ -f "$HOME_FOLDER/config.toml" ] || { echo "$HOME_FOLDER is not an initialised home; run nerdgenie init on it first" >&2; exit 2; }
# Every run serves from a fresh copy of the home, so that a job the last run
# left running, or a task it left put down, never haunts the next: the sixth
# run's game became a job, the runner stopped its serve under it, and the job
# would have come back on the next serve beside the warm-up asks.
# The fresh home sits beside the work folder, never under it: a home inside a
# sandbox root is refused by the serve, because the home must stay outside
# the fence.
TEMPLATE="$HOME_FOLDER"
HOME_FOLDER="$(dirname "$TEMPLATE")/homes/$DATE-$(date +%H%M)"
mkdir -p "$HOME_FOLDER"
for piece in config.toml persona skills vault.key; do
  [ -e "$TEMPLATE/$piece" ] && cp -r "$TEMPLATE/$piece" "$HOME_FOLDER/"
done
SOCKET="$HOME_FOLDER/run/agent.sock"
if curl -s --max-time 3 http://127.0.0.1:19091/health | grep -q '"ok"'; then :; else echo "the local daemon is not answering on 19091" >&2; exit 2; fi

NERDGENIE_HOME="$HOME_FOLDER" "$BINARY" serve > "$WORKROOT/serve.log" 2>&1 < /dev/null &
SERVE_PID=$!
stop_serve() {
  if [ "$(ps -o comm= -p "$SERVE_PID" 2>/dev/null)" = "nerdgenie" ]; then
    kill -INT "$SERVE_PID"
    wait "$SERVE_PID" 2>/dev/null
  fi
}
trap 'stop_serve; exit 143' TERM INT
for _ in $(seq 1 60); do
  [ -S "$SOCKET" ] && grep -q 'is listening' "$WORKROOT/serve.log" && break
  sleep 1
done
grep -q 'is listening' "$WORKROOT/serve.log" || { cat "$WORKROOT/serve.log" >&2; stop_serve; exit 1; }
NERDGENIE_HOME="$HOME_FOLDER" "$BINARY" run -timeout 20s "/yolo" | grep -q 'yolo is on' || { echo "yolo would not turn on" >&2; stop_serve; exit 1; }

{
  echo "# Nightly set, $DATE"
  echo
  echo "Local Qwen 3.8 through the daemon on 19091, binary $(cd "$ROOT" && git rev-parse --short HEAD 2>/dev/null || echo unknown), home $HOME_FOLDER."
  echo
  echo "| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks | cut off at cap |"
  echo "|---|---|---|---|---|---|---|---|---|---|"
} > "$OUT"

# newest_task is the highest task number the log holds, or zero.
newest_task() {
  python3 - "$HOME_FOLDER/nerdgenie.db" <<'PY'
import sqlite3, sys
try:
    c = sqlite3.connect("file:" + sys.argv[1] + "?mode=ro", uri=True)
    ids = [r[0] for r in c.execute("select distinct task_id from events where task_id glob '[0-9]*'")]
    print(max((int(i) for i in ids), default=0))
except Exception:
    print(0)
PY
}

# job_state is the first line of a job's newest checkpoint, such as "done 12
# of 12 tasks done", read off the log.
job_state() {
  python3 - "$HOME_FOLDER/nerdgenie.db" "$1" <<'PY'
import sqlite3, sys, json
try:
    c = sqlite3.connect("file:" + sys.argv[1] + "?mode=ro", uri=True)
    r = c.execute("select body from events where task_id=? and kind='checkpoint' order by sequence desc limit 1", ("j" + sys.argv[2],)).fetchone()
    head = json.loads(r[0])["text"].split("\n")[0] if r else "no record"
    print(" ".join(head.split()[3:]))
except Exception as e:
    print("unreadable")
PY
}

# wait_for_job waits until the job is no longer running, polling the log, or
# until the ask's timeout is spent counting from when the ask was sent.
wait_for_job() {
  job="$1"; started="$2"; limit="$3"
  seconds="$(echo "$limit" | sed -n 's/^\([0-9]*\)h$/\1*3600/p; s/^\([0-9]*\)m$/\1*60/p; s/^\([0-9]*\)s$/\1/p' | bc)"
  [ -n "$seconds" ] || seconds=3600
  while [ "$(( $(date +%s) - started ))" -lt "$seconds" ]; do
    case "$(job_state "$job")" in running*) sleep 30 ;; *) return 0 ;; esac
  done
  echo "the job $job was still running when the ask's time ran out" >> "$work/run.log"
}

passed=0; total=0
run_one() {
  name="$1"; ask="$2"; check="$3"; timeout="$4"
  work="$WORKROOT/$name"; mkdir -p "$work"
  if [ -d "$HERE/fixtures/$name" ]; then cp -r "$HERE/fixtures/$name/." "$work/"; fi
  text="$(sed "s|<WORK>|$work|g" "$ask")"
  # A task the last run left put down would be picked up by the next message,
  # so it is set aside first: the ask is a new task, never a steer.
  NERDGENIE_HOME="$HOME_FOLDER" "$BINARY" run -timeout 20s "/clear" > /dev/null 2>&1
  first="$(( $(newest_task) + 1 ))"
  started="$(date +%s)"
  NERDGENIE_HOME="$HOME_FOLDER" "$BINARY" run -wait -timeout "$timeout" "$text" > "$work/run.log" 2>&1
  # An ask that became a job goes on after its own task ends: wait for the
  # job to stop running, within the ask's timeout, before measuring.
  job="$(grep -oE 'job [0-9]+' "$work/run.log" | head -1 | awk '{print $2}')"
  if [ -n "$job" ]; then wait_for_job "$job" "$started" "$timeout"; fi
  cp "$HOME_FOLDER/nerdgenie.db" "$SCRATCH/" 2>/dev/null; cp "$HOME_FOLDER/nerdgenie.db-wal" "$SCRATCH/" 2>/dev/null; cp "$HOME_FOLDER/nerdgenie.db-shm" "$SCRATCH/" 2>/dev/null
  report="$(cd "$ROOT" && go run ./scripts/runreport --log "$SCRATCH/nerdgenie.db" --from "$first" 2>&1)"
  ended="$(echo "$report" | sed -n 's/^task [0-9 to]*: \([a-z]*\) after.*/\1/p')"
  if [ -n "$job" ]; then ended="job $job $(job_state "$job")"; fi
  rounds="$(echo "$report" | sed -n 's/^task [0-9 to]*: [a-z]* after \([0-9]*\) rounds.*/\1/p')"
  minutes="$(echo "$report" | sed -n 's/.* in \([0-9]*\) minutes.*/\1/p')"
  perround="$(echo "$report" | sed -n 's/.*(\([0-9]*\) s a round).*/\1/p')"
  cache="$(echo "$report" | sed -n 's/.*cached (\([0-9]*\)%).*/\1/p')"
  out="$(echo "$report" | sed -n 's/.*, \([0-9.]*k\) out.*/\1/p')"
  recordonly="$(echo "$report" | sed -n 's/^rounds that only wrote the record: \([0-9]*\);.*/\1/p')"
  firstline="$(echo "$report" | sed -n 's/.*marks made from the first line: \([0-9]*\).*/\1/p')"
  cutoff="$(echo "$report" | sed -n 's/.*rounds cut off at the output cap: \([0-9]*\).*/\1/p')"
  if "$check" "$work" "$SCRATCH/nerdgenie.db" > "$work/check.log" 2>&1; then result="pass"; passed=$((passed+1)); else result="FAIL: $(tail -1 "$work/check.log")"; fi
  total=$((total+1))
  echo "| $name | ${ended:-?} | $result | ${rounds:-?} | ${minutes:-?} | ${perround:-?} | ${cache:-?}% | ${out:-?} | ${recordonly:-?} | ${firstline:-?} | ${cutoff:-?} |" >> "$OUT"
  echo "$name: ${ended:-?}, $result, ${rounds:-?} rounds in ${minutes:-?} min"
}

run_one 01-hello "$HERE/asks/01-hello.md" "$HERE/checks/01-hello.sh" 10m
run_one 02-node-tests "$HERE/asks/02-node-tests.md" "$HERE/checks/02-node-tests.sh" 15m
run_one 03-fix-the-test "$HERE/asks/03-fix-the-test.md" "$HERE/checks/03-fix-the-test.sh" 15m
run_one 04-page "$HERE/asks/04-page.md" "$HERE/checks/04-page.sh" 20m
if [ "$WITH_TETRIS" = "--with-tetris" ] && [ -f "$HERE/asks/05-tetris.md" ]; then
  run_one 05-tetris "$HERE/asks/05-tetris.md" "$HERE/checks/05-tetris.sh" 3h
fi

stop_serve
rm -rf "$SCRATCH"
echo >> "$OUT"; echo "$passed of $total checks passed." >> "$OUT"
echo "$passed of $total checks passed; the table is in $OUT"
