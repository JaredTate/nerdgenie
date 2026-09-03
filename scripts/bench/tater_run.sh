#!/usr/bin/env bash
# tater_run.sh HARNESS RUN [CARD]
#
# CARD is "a" (port 19091, Vulkan1) or "b" (port 19093, Vulkan2); default "a".
# Both cards run the byte-identical daemon (same model file, 262144 context, same
# sampling), so a run on either is apples-to-apples. Never run two harnesses on
# the same card at once.
#
# Runs one harness once on a fresh, empty, git-initialised folder against the
# byte-identical canonical task, then measures it the same way as every other
# harness: speed and cost from the card-A daemon log slice (countcalls.sh), and
# quality from check-tater.mjs. Everything is bounded; the harness is launched
# with setsid under a hard 30-minute cap, and nothing is killed by name.
#
# It writes:
#   results/<harness>-<run>.out   the harness's own output
#   results/<harness>-<run>.txt   the measured result (speed + quality JSON)
#   docs/bench/<harness>-<run>.png a screenshot of the finished game
set -u

HARNESS="$1"
RUN="$2"
CARD="${3:-a}"

case "$CARD" in
  a) LOG=/home/jared/llm/logs/19091.log; PORT=19091; PROVIDER=turbo-a; OC_CONFIG=/home/jared/.config/opencode/opencode.json; OCLAW_MODEL=vllm/local-coder;;
  b) LOG=/home/jared/llm/logs/19093.log; PORT=19093; PROVIDER=turbo-b; OC_CONFIG=/home/jared/llm/bench/oc-19093.json; OCLAW_MODEL=vllm-b/local-coder;;
  *) echo "unknown card: $CARD (use a or b)"; exit 2;;
esac

REPO=/home/jared/Code/coeus/.claude/worktrees/agent-a58b1fa4b2964028f
BIN="$REPO/bin/coeus"
BENCH=/home/jared/work/bench/tater
TASK=/home/jared/work/bench/canonical/task.txt
WORK="$BENCH/$HARNESS-$RUN"
HOMEDIR="$BENCH/$HARNESS-$RUN-home"
RESULT="$BENCH/results/$HARNESS-$RUN"
SHOT="$REPO/docs/bench/$HARNESS-$RUN.png"

mkdir -p "$BENCH/results" "$REPO/docs/bench"

# The task must be byte-identical for every harness.
sha=$(sha256sum "$TASK" | cut -c1-8)
if [ "$sha" != "c8ed58f2" ]; then
  echo "TASK SHA MISMATCH ($sha); refusing to run" | tee "$RESULT.txt"
  exit 2
fi

# Fresh state.
rm -rf "$WORK" "$HOMEDIR"
mkdir -p "$WORK"
git init -q "$WORK"
cp "$TASK" "$WORK/task.txt"

# Build the harness command.
case "$HARNESS" in
  coeus)
    COEUS_HOME="$HOMEDIR" "$BIN" init --yes >/dev/null 2>&1
    python3 - "$HOMEDIR/config.toml" "$WORK" <<'PY'
import sys, re
cfg, work = sys.argv[1], sys.argv[2]
t = open(cfg).read()
t = re.sub(r'sandbox_roots = .*', 'sandbox_roots = ["%s"]' % work, t, count=1)
# Local-only, like every other harness here: no silent fall back to a cloud
# subscription if the local model stumbles.
t = re.sub(r'fallback_chain = .*', 'fallback_chain = []', t, count=1)
open(cfg, 'w').write(t)
PY
    CMD=(bash "$REPO/scripts/bench/run-coeus.sh" "$BIN" "$HOMEDIR" "$WORK/task.txt" 28)
    ;;
  opencode)
    CMD=(bash -c 'cd "$1" && OPENCODE_CONFIG="$2" /home/jared/.opencode/bin/opencode run "$(cat task.txt)"' _ "$WORK" "$OC_CONFIG")
    ;;
  hermes)
    CMD=(hermes chat -q "$(cat "$WORK/task.txt")" --provider "$PROVIDER" -m local-coder --reasoning low --yolo --max-turns 250 --in "$WORK")
    ;;
  openclaw)
    # OpenClaw's default tool mode sends a GBNF grammar for structured tool
    # calls that this llama-server build rejects ("failed to parse grammar"), so
    # it is run in its documented local-model mode: code mode (the model writes
    # code instead of grammar-constrained tool JSON) with the lean tool surface.
    CMD=(openclaw agent exec --message-file "$WORK/task.txt" --model "$OCLAW_MODEL" --code-mode code --local-model-lean --cwd "$WORK" --timeout 1740)
    ;;
  *)
    echo "unknown harness: $HARNESS" | tee "$RESULT.txt"; exit 2;;
esac

# Measure launch -> exit, with a hard 30-minute cap.
lines_before=$(wc -l < "$LOG")
started=$(date +%s)
setsid --wait timeout --kill-after=30 30m "${CMD[@]}" > "$RESULT.out" 2>&1
status=$?
ended=$(date +%s)

{
  echo "harness: $HARNESS  run: $RUN  card: $CARD (port $PORT)  daemon log: $LOG"
  printf 'command:'; printf ' %q' "${CMD[@]}"; printf '\n'
  echo "exit status: $status  (124 = 30m SIGTERM cap, 137 = SIGKILL after cap)"
  echo "wall seconds: $((ended - started))"
  echo "started: $(date -d @"$started" '+%F %T')  ended: $(date -d @"$ended" '+%F %T')"
  bash "$REPO/scripts/bench/countcalls.sh" "$LOG" "$lines_before"
  echo "checker:"
  timeout 150 node "$REPO/scripts/bench/check-tater.mjs" "$WORK" --harness "$HARNESS" --run "$RUN" --shot "$SHOT"
} > "$RESULT.txt" 2>&1

echo "==== $HARNESS-$RUN done (exit $status, $((ended - started))s) ===="
cat "$RESULT.txt"
