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
  echo "| ask | ended | check | rounds | minutes | s/round | cache | out tokens |"
  echo "|---|---|---|---|---|---|---|---|"
} > "$OUT"

passed=0; total=0
run_one() {
  name="$1"; ask="$2"; check="$3"; timeout="$4"
  work="$WORKROOT/$name"; mkdir -p "$work"
  if [ -d "$HERE/fixtures/$name" ]; then cp -r "$HERE/fixtures/$name/." "$work/"; fi
  text="$(sed "s|<WORK>|$work|g" "$ask")"
  NERDGENIE_HOME="$HOME_FOLDER" "$BINARY" run -wait -timeout "$timeout" "$text" > "$work/run.log" 2>&1
  cp "$HOME_FOLDER/nerdgenie.db" "$SCRATCH/" 2>/dev/null; cp "$HOME_FOLDER/nerdgenie.db-wal" "$SCRATCH/" 2>/dev/null; cp "$HOME_FOLDER/nerdgenie.db-shm" "$SCRATCH/" 2>/dev/null
  report="$(cd "$ROOT" && go run ./scripts/runreport --log "$SCRATCH/nerdgenie.db" 2>&1)"
  ended="$(echo "$report" | sed -n 's/^task [0-9]*: \([a-z]*\) after.*/\1/p')"
  rounds="$(echo "$report" | sed -n 's/^task [0-9]*: [a-z]* after \([0-9]*\) rounds.*/\1/p')"
  minutes="$(echo "$report" | sed -n 's/.* in \([0-9]*\) minutes.*/\1/p')"
  perround="$(echo "$report" | sed -n 's/.*(\([0-9]*\) s a round).*/\1/p')"
  cache="$(echo "$report" | sed -n 's/.*cached (\([0-9]*\)%).*/\1/p')"
  out="$(echo "$report" | sed -n 's/.*, \([0-9.]*k\) out.*/\1/p')"
  if "$check" "$work" "$SCRATCH/nerdgenie.db" > "$work/check.log" 2>&1; then result="pass"; passed=$((passed+1)); else result="FAIL: $(tail -1 "$work/check.log")"; fi
  total=$((total+1))
  echo "| $name | ${ended:-?} | $result | ${rounds:-?} | ${minutes:-?} | ${perround:-?} | ${cache:-?}% | ${out:-?} |" >> "$OUT"
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
