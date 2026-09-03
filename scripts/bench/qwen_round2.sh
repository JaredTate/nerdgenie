#!/usr/bin/env bash
# Qwen round 2: all four harnesses on the daemons already loaded (no restart,
# the same for everyone), two cards, one harness per card, opencode and Hermes
# in the setup the user's notes prescribe. Card A: Coeus then opencode. Card B:
# Hermes then OpenClaw, starting when the card is free. Each launch records the
# daemon's cache state first. Nothing is killed by name.
set -u
REPO=/home/jared/Code/coeus
RUNNER="$REPO/scripts/bench/tater_run_tuned.sh"
LABEL="${1:-2}"
ROOT=/home/jared/work/bench/tater2
cd "$REPO" || exit 1

card_busy() { # $1 = harness folder name of the run that may still hold the card
  local f="$ROOT/$1/harness.pid"
  [ -f "$f" ] && ps -p "$(cat "$f")" >/dev/null 2>&1
}
wait_free() { while card_busy "$1"; do sleep 10; done; }
snapshot() { curl -s -m 3 "http://127.0.0.1:$1/slots" > "$ROOT/slots-$2-before-$3-$LABEL.json"; }
run() { # $1 harness, $2 card letter, $3 port
  snapshot "$3" "$2" "$1"
  echo "== $(date '+%T') $1 on card $2 starts"
  bash "$RUNNER" qwen "$1" "$LABEL" "$2" > "/home/jared/work/bench/qwen-$1-$LABEL.log" 2>&1
  echo "== $(date '+%T') $1 done: $(grep -E 'finished|wall clock|model calls:' "$ROOT/qwen-$1-$LABEL/result.txt" 2>/dev/null | tr '\n' ' ')"
}

( run coeus a 19091; run opencode a 19091 ) &
CHAIN_A=$!
( wait_free qwen-openclaw-1; wait_free "qwen-hermes-$LABEL"; run hermes b 19093; run openclaw b 19093 ) &
CHAIN_B=$!
wait "$CHAIN_A"; wait "$CHAIN_B"
echo "== $(date '+%T') round $LABEL done"
