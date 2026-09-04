#!/usr/bin/env bash
# tater_run.sh <phase> <harness> <run-label> [card]
#
# Run one coding harness once on the canonical Tater task, from a completely
# fresh state, and measure it the same way as the other three.
#
#   phase    "qwen", the only phase this runner drives: every harness talks to
#            the llama-server daemon already running on this machine, card "a"
#            (port 19091) or card "b" (port 19093). Opus 4.8 runs are driven by
#            hand, so "opus" is refused here rather than half-supported.
#   harness  nerdgenie | opencode | hermes | openclaw
#   label    a short name for this run, such as "1" or "rerun".
#   card     "a" or "b". Default "a".
#
# Options:
#   --dry-run   do all the setup, print the exact environment and command that
#               would be launched, and stop before launching anything.
#
# Settings, all through the environment:
#   TATER_CAP      unset, which is the default, means no cap at all. Set it to a
#                  number of model calls to cap the run: the runner watches the
#                  daemon log and stops the harness at that many calls.
#   TATER_MINUTES  unset, the default, means no wall-clock cap at all. Set it to
#                  a number of minutes to stop the harness after that long.
#   TATER_ROOT     where run folders go. Default ~/work/bench/tater2.
#   TATER_TASK     the canonical task. Default ~/work/bench/canonical/task.txt.
#   TATER_NERDGENIE_BIN  the Coeus binary. Default <repo>/bin/nerdgenie, built if absent.
#
# Everything one run needs and everything it writes lives in one folder,
# ~/work/bench/tater2/<phase>-<harness>-<label>/, which is deleted and made
# again at the start of every run so no run can ever see another's work:
#
#   work/            empty, git-initialised, with the task copied in as task.txt
#   home/            the harness's whole home: its config, state, and cache
#   harness.pid      the process id this script launched
#   harness.out      everything the harness printed
#   launch.txt       the exact environment and command that was launched
#   countcalls.txt   what the daemon log says the model did
#   check.json       the checker's verdict on the work folder
#   result.json      every measurement in one flat object
#   result.txt       the same measurements in plain words
#
# Nothing is ever killed by name pattern, and no daemon is ever started or
# stopped here. The harness is stopped, when there is a cap to stop it at, by
# the exact process id recorded when it was launched, and only after
# "ps -o comm=" confirms the process still has the name it had then.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

# ---- arguments -------------------------------------------------------------

DRY_RUN=no
POSITIONAL=()
for argument in "$@"; do
  case "$argument" in
    --dry-run) DRY_RUN=yes;;
    -*) echo "tater_run.sh: unknown option $argument; the only option is --dry-run" >&2; exit 2;;
    *) POSITIONAL+=("$argument");;
  esac
done

if [ "${#POSITIONAL[@]}" -lt 3 ] || [ "${#POSITIONAL[@]}" -gt 4 ]; then
  echo "usage: tater_run.sh <phase> <harness> <run-label> [card] [--dry-run]" >&2
  echo "  phase: qwen; harness: nerdgenie, opencode, hermes or openclaw; card: a or b" >&2
  exit 2
fi

PHASE="${POSITIONAL[0]}"
HARNESS="${POSITIONAL[1]}"
LABEL="${POSITIONAL[2]}"
CARD="${POSITIONAL[3]:-a}"

case "$PHASE" in
  qwen) ;;
  opus) echo "tater_run.sh: the opus phase is driven by hand, not by this runner; the phase here is \"qwen\"" >&2; exit 2;;
  *) echo "tater_run.sh: the phase must be \"qwen\", not \"$PHASE\"" >&2; exit 2;;
esac

case "$HARNESS" in
  nerdgenie|opencode|hermes|openclaw) ;;
  *) echo "tater_run.sh: the harness must be nerdgenie, opencode, hermes or openclaw, not \"$HARNESS\"" >&2; exit 2;;
esac

case "$LABEL" in
  *[!A-Za-z0-9_.-]*|"") echo "tater_run.sh: the run label \"$LABEL\" must be letters, digits, dot, dash or underscore" >&2; exit 2;;
esac

case "$CARD" in
  a) PORT=19091;;
  b) PORT=19093;;
  *) echo "tater_run.sh: the card must be \"a\" or \"b\", not \"$CARD\"" >&2; exit 2;;
esac

# ---- where everything lives ------------------------------------------------

TATER_ROOT="${TATER_ROOT:-$HOME/work/bench/tater2}"
TATER_TASK="${TATER_TASK:-$HOME/work/bench/canonical/task.txt}"
NERDGENIE_BIN="${TATER_NERDGENIE_BIN:-$REPO/bin/nerdgenie}"
OPENCODE_BIN="${TATER_OPENCODE_BIN:-/home/jared/.opencode/bin/opencode}"

RUN="$TATER_ROOT/$PHASE-$HARNESS-$LABEL"
WORK="$RUN/work"
HOMEDIR="$RUN/home"

MODEL="local-coder"
CONTEXT=262144
DAEMON_LOG="$HOME/llm/logs/$PORT.log"
BASE_URL="http://127.0.0.1:$PORT/v1"

CAP="${TATER_CAP:-}"
MINUTES="${TATER_MINUTES:-}"
case "$CAP" in
  ""|*[0-9]) ;;
  *) echo "tater_run.sh: TATER_CAP must be a whole number of model calls, not \"$CAP\"" >&2; exit 2;;
esac
case "$MINUTES" in
  ""|*[0-9]) ;;
  *) echo "tater_run.sh: TATER_MINUTES must be a whole number of minutes, not \"$MINUTES\"" >&2; exit 2;;
esac

# ---- refusals, before anything is written ----------------------------------

# The task must be byte-identical for every harness.
if [ ! -f "$TATER_TASK" ]; then
  echo "tater_run.sh: there is no task at $TATER_TASK; put the canonical task there or set TATER_TASK" >&2
  exit 2
fi
task_sha="$(sha256sum "$TATER_TASK" | cut -c1-8)"
if [ "$task_sha" != "c8ed58f2" ]; then
  echo "tater_run.sh: the task at $TATER_TASK has checksum $task_sha, not c8ed58f2, so it is not the canonical task; refusing to run" >&2
  exit 2
fi

# This runner never starts or stops a daemon. If the one it needs is not
# answering, it says exactly what to run and stops.
if [ "$DRY_RUN" = "no" ]; then
  health="$(curl -s -m 5 "http://127.0.0.1:$PORT/health" 2>/dev/null || true)"
  if [ "$health" != '{"status":"ok"}' ]; then
    vulkan=$([ "$CARD" = "a" ] && echo Vulkan1 || echo Vulkan2)
    echo "tater_run.sh: the llama-server daemon on port $PORT is not answering /health with {\"status\":\"ok\"}; it said: ${health:-nothing}" >&2
    echo "This runner never starts a daemon. Ask the orchestrator to run:" >&2
    echo "  setsid ~/llm/igo.sh $vulkan $PORT 262144 mtp > ~/llm/logs/$PORT.log 2>&1 < /dev/null &" >&2
    exit 2
  fi
  if [ ! -f "$DAEMON_LOG" ]; then
    echo "tater_run.sh: the daemon on port $PORT is up but its log $DAEMON_LOG is missing, and the measurements are read from it; refusing to run" >&2
    exit 2
  fi
fi

# Two runs at a time are fine, but never two on the same card, because the two
# would share one daemon and one slice of its log. Every run folder records the
# port it uses and the process id it launched, so a live run holds its card.
mkdir -p "$TATER_ROOT"
for other in "$TATER_ROOT"/*/; do
  [ -d "$other" ] || continue
  [ "${other%/}" = "$RUN" ] && continue
  [ -f "$other/port" ] && [ -f "$other/harness.pid" ] && [ -f "$other/harness.comm" ] || continue
  [ "$(cat "$other/port")" = "$PORT" ] || continue
  other_pid="$(cat "$other/harness.pid")"
  other_comm="$(cat "$other/harness.comm")"
  [ -n "$other_pid" ] || continue
  if [ "$(ps -o comm= -p "$other_pid" 2>/dev/null || true)" = "$other_comm" ]; then
    echo "tater_run.sh: ${other%/} is still running on port $PORT as process $other_pid; wait for it or use the other card" >&2
    exit 2
  fi
done

# ---- a completely fresh folder ---------------------------------------------

case "$RUN" in
  "$TATER_ROOT"/*) ;;
  *) echo "tater_run.sh: refusing to delete $RUN, which is not inside $TATER_ROOT" >&2; exit 2;;
esac
rm -rf "$RUN"
mkdir -p "$WORK" "$HOMEDIR"
git init -q "$WORK"
cp "$TATER_TASK" "$WORK/task.txt"
printf '%s\n' "$PORT" > "$RUN/port"
printf '%s\n' "$PHASE" > "$RUN/phase"
printf '%s\n' "$CARD" > "$RUN/card"

# ---- the Coeus binary ------------------------------------------------------

if [ "$HARNESS" = "nerdgenie" ] && [ ! -x "$NERDGENIE_BIN" ]; then
  echo "building the Coeus binary, which is missing from $NERDGENIE_BIN"
  if ! (cd "$REPO" && timeout 900 make build); then
    echo "tater_run.sh: \"make build\" failed, so there is no Coeus binary to run" >&2
    exit 2
  fi
fi

# ---- the harness's own fresh home ------------------------------------------

# Every harness is given a home of its own under this run's folder, so it starts
# with no memory, no sessions, no cache, and no model but the one this run
# points it at. What each harness needs was found by reading its code.
LAUNCH_ENV=()
LAUNCH_CMD=()
BUDGET_RAISED=no

case "$HARNESS" in

  nerdgenie)
    # Coeus keeps everything under NERDGENIE_HOME: config, record, memory, logs.
    if ! NERDGENIE_HOME="$HOMEDIR" "$NERDGENIE_BIN" init --yes > "$RUN/init.out" 2>&1; then
      echo "tater_run.sh: \"nerdgenie init --yes\" failed; its output is in $RUN/init.out" >&2
      exit 2
    fi
    # Coeus ships its own task budget, a hundred tool rounds and an hour, which
    # would cap Coeus and nothing else. The benchmark has no cap, so the budget
    # is raised to a number no run reaches, and the raise is recorded.
    python3 - "$HOMEDIR/config.toml" "$WORK" "$BASE_URL" "$MODEL" "$CONTEXT" <<'PYTHON'
import sys

config_path, work, base_url, model, context = sys.argv[1:6]
text = open(config_path).read()

# Keep the settings the template wrote above the model blocks, replacing the
# three lines this benchmark decides, and drop every model block the template
# guessed at in favour of the one this run points at.
head = text.split("[[models]]")[0].rstrip().splitlines()
# The template puts a comment above its first model block; drop it, since the
# block it introduces is being replaced.
while head and (head[-1].startswith("#") or not head[-1].strip()):
    head.pop()
lines = []
for line in head:
    if line.startswith("default_model "):
        line = 'default_model = "bench"'
    elif line.startswith("fallback_chain "):
        line = "fallback_chain = []"
    elif line.startswith("sandbox_roots "):
        line = 'sandbox_roots = ["%s"]' % work
    lines.append(line)

open(config_path, "w").write("\n".join(lines) + """

# The one model this benchmark run talks to.
[[models]]
name = "bench"
provider = "openai"
base_address = "%s"
model_name = "%s"
context_length = %s

# The benchmark puts no cap on any harness, so Coeus's own task budget is
# raised out of the way. Every other cap is left at its default.
[caps]
rounds_per_task = 1000000
time_per_task = "240h"
""" % (base_url, model, context))
PYTHON
    BUDGET_RAISED=yes
    if ! NERDGENIE_HOME="$HOMEDIR" "$NERDGENIE_BIN" doctor > "$RUN/doctor.out" 2>&1; then
      echo "tater_run.sh: \"nerdgenie doctor\" is unhappy with the config this run wrote; see $RUN/doctor.out" >&2
      exit 2
    fi
    LAUNCH_ENV=("NERDGENIE_HOME=$HOMEDIR")
    LAUNCH_CMD=(bash "$HERE/run-nerdgenie.sh" "$NERDGENIE_BIN" "$HOMEDIR" "$WORK/task.txt")
    if [ -n "$MINUTES" ]; then
      LAUNCH_CMD+=("$MINUTES")
    fi
    ;;

  opencode)
    # opencode reads the four XDG folders and nothing else, and its global
    # config is $XDG_CONFIG_HOME/opencode/opencode.json. OPENCODE_CONFIG is
    # never set, because that file is merged on top of the user's global config
    # instead of replacing it.
    mkdir -p "$HOMEDIR/data" "$HOMEDIR/config/opencode" "$HOMEDIR/cache" "$HOMEDIR/state"
    # The user's own opencode setup (LOCAL_SUBAGENTS.md section 3b): the whole
    # file as the user keeps it, with only the provider address, the model name
    # and the output limit the notes decree; loop guard allowed, four tools off,
    # small_model pinned local, compaction reserved, run with --auto.
    python3 - "$HOMEDIR/config/opencode/opencode.json" "$BASE_URL" "$MODEL" "$CONTEXT" <<'PYTHON'
import json
import sys

config_path, base_url, model, context = sys.argv[1:5]
config = json.load(open("/home/jared/.config/opencode/opencode.json"))
config["provider"] = {
    "bench": {
        "npm": "@ai-sdk/openai-compatible",
        "name": "the benchmark endpoint",
        "options": {"baseURL": base_url, "apiKey": "local"},
        "models": {model: {"name": model, "limit": {"context": int(context), "output": 131072}}},
    },
}
config["model"] = "bench/%s" % model
config["small_model"] = "bench/%s" % model
config["share"] = "disabled"
config["autoupdate"] = False
json.dump(config, open(config_path, "w"), indent=2)
open(config_path, "a").write("\n")
PYTHON
    LAUNCH_ENV=(
      "XDG_DATA_HOME=$HOMEDIR/data"
      "XDG_CONFIG_HOME=$HOMEDIR/config"
      "XDG_CACHE_HOME=$HOMEDIR/cache"
      "XDG_STATE_HOME=$HOMEDIR/state"
    )
    LAUNCH_CMD=("$OPENCODE_BIN" run --auto -m "bench/$MODEL" --format json "$(cat "$WORK/task.txt")")
    ;;

  hermes)
    # Hermes keeps its config, its state database and its memory under
    # HERMES_HOME, which must be somewhere other than ~/.hermes or the run
    # would read the user's own sessions and memory.
    mkdir -p "$HOMEDIR/hermes"
    # The user's documented Hermes setup for the local Qwen (LOCAL_SUBAGENTS.md
    # section 3): the sampler and thinking-off repeated per request, reasoning
    # low, the loop guardrails, the 250-turn limit the notes prescribe, approvals
    # off, and the tools restricted to file and terminal on the command line.
    python3 - "$HOMEDIR/hermes/config.yaml" "$BASE_URL" "$MODEL" "$CONTEXT" <<'PYTHON'
import sys

config_path, base_url, model, context = sys.argv[1:5]
open(config_path, "w").write(
    "# Written by tater_run_tuned.sh: the user's own Hermes setup for the local Qwen.\n"
    "model:\n"
    "  default: %s\n"
    "  provider: custom\n"
    "  base_url: %s\n"
    "  context_length: %s\n"
    "custom_providers:\n"
    "  - name: bench\n"
    "    base_url: %s\n"
    "    api_key: local\n"
    "    models: { %s: { context_length: %s } }\n"
    "    extra_body: { chat_template_kwargs: { enable_thinking: false }, temperature: 0.7, top_p: 0.8, top_k: 20, presence_penalty: 1.5 }\n"
    "agent:\n"
    "  reasoning_effort: low\n"
    "  reasoning_overrides: { %s: low }\n"
    "  max_turns: 250\n"
    "approvals:\n"
    "  mode: off\n"
    "tool_loop_guardrails:\n"
    "  warnings_enabled: true\n"
    "  hard_stop_enabled: true\n"
    "  warn_after: { exact_failure: 2, same_tool_failure: 3, idempotent_no_progress: 2 }\n"
    "  hard_stop_after: { exact_failure: 3, same_tool_failure: 5, idempotent_no_progress: 3 }\n"
    "display:\n"
    "  show_reasoning: false\n"
    "compression: { enabled: true, threshold: 0.5, target_ratio: 0.2 }\n"
    "telemetry:\n"
    "  shared_metrics:\n"
    "    enabled: false\n"
    % (model, base_url, context, base_url, model, context, model)
)
PYTHON
    LAUNCH_ENV=("HERMES_HOME=$HOMEDIR/hermes")
    LAUNCH_CMD=(hermes chat -q "$(cat "$WORK/task.txt")" --provider bench -m "$MODEL" --reasoning low --yolo --max-turns 250 -t file,terminal --in "$WORK")
    ;;

  openclaw)
    # OpenClaw reads $OPENCLAW_HOME/.openclaw/openclaw.json, and --config pins
    # the run to exactly that file. --state-dir is never passed, so the run's
    # state stays under this run's home and is thrown away with it.
    mkdir -p "$HOMEDIR/openclaw/.openclaw"
    python3 - "$HOMEDIR/openclaw/.openclaw/openclaw.json" "$BASE_URL" "$MODEL" "$CONTEXT" <<'PYTHON'
import json
import sys

config_path, base_url, model, context = sys.argv[1:5]
json.dump({
    "models": {
        "providers": {
            "bench": {
                "baseUrl": base_url,
                "apiKey": "local",
                "api": "openai-completions",
                "timeoutSeconds": 900,
                "models": [{
                    "id": model,
                    "name": model,
                    "reasoning": False,
                    "input": ["text"],
                    "cost": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
                    "contextWindow": int(context),
                    "maxTokens": 32000,
                }],
            },
        },
    },
}, open(config_path, "w"), indent=2)
open(config_path, "a").write("\n")
PYTHON
    LAUNCH_ENV=("OPENCLAW_HOME=$HOMEDIR/openclaw")
    LAUNCH_CMD=(openclaw agent exec
      --config "$HOMEDIR/openclaw/.openclaw/openclaw.json"
      --message-file "$WORK/task.txt"
      --model "bench/$MODEL"
      --local-model-lean
      --cwd "$WORK"
      --timeout 0
      --json)
    ;;
esac

# ---- what would be launched ------------------------------------------------

{
  echo "phase: $PHASE"
  echo "harness: $HARNESS"
  echo "label: $LABEL"
  echo "card: $CARD"
  echo "port: $PORT"
  echo "model: $MODEL"
  echo "base url: $BASE_URL"
  echo "context length: $CONTEXT"
  echo "work folder: $WORK"
  echo "home folder: $HOMEDIR"
  echo "launched from: $WORK"
  echo "call cap: ${CAP:-none}"
  if [ -n "$MINUTES" ]; then echo "wall-clock cap: $MINUTES minutes"; else echo "wall-clock cap: none"; fi
  echo "nerdgenie budget raised: $BUDGET_RAISED"
  echo "environment:"
  for pair in "${LAUNCH_ENV[@]}"; do echo "  $pair"; done
  printf 'command:'; printf ' %q' "${LAUNCH_CMD[@]}"; printf '\n'
} > "$RUN/launch.txt"

if [ "$DRY_RUN" = "yes" ]; then
  cat "$RUN/launch.txt"
  echo "would check first: curl -s http://127.0.0.1:$PORT/health"
  echo "dry run: nothing was launched"
  exit 0
fi

# ---- launch ----------------------------------------------------------------

LINES_BEFORE="$(wc -l < "$DAEMON_LOG")"

# Every harness is started in the run's own work folder, which is the folder the
# task tells it to build in, so none of them gets a different starting point.
# PWD is set as well as the folder itself, because `env -C` changes the folder
# without touching that variable and opencode reads the variable for its
# project folder (it wrote into the runner's folder twice before this line).
STARTED="$(date +%s)"
if [ -n "$MINUTES" ]; then
  env -C "$WORK" PWD="$WORK" "${LAUNCH_ENV[@]}" timeout --kill-after=30 "${MINUTES}m" "${LAUNCH_CMD[@]}" \
    > "$RUN/harness.out" 2>&1 < /dev/null &
else
  env -C "$WORK" PWD="$WORK" "${LAUNCH_ENV[@]}" "${LAUNCH_CMD[@]}" > "$RUN/harness.out" 2>&1 < /dev/null &
fi
HARNESS_PID=$!
HARNESS_COMM="$(ps -o comm= -p "$HARNESS_PID" 2>/dev/null || true)"
printf '%s\n' "$HARNESS_PID" > "$RUN/harness.pid"
printf '%s\n' "$HARNESS_COMM" > "$RUN/harness.comm"
echo "==== $PHASE-$HARNESS-$LABEL launched as process $HARNESS_PID ($HARNESS_COMM) ===="

# ---- the cap, when there is one --------------------------------------------

# There is no cap unless TATER_CAP says so. When it does, the daemon does the
# counting in its log, one "launch_slot_" line per call, so this watcher reads
# that slice every ten seconds and stops the harness by the exact process id it
# launched, after checking the process still has the name it had at launch.
WATCHER_PID=""
if [ -n "$CAP" ]; then
  (
    # Bounded at ten thousand looks, which is a bit over a day of watching.
    for _ in $(seq 1 10000); do
      [ -f "$RUN/harness.done" ] && exit 0
      sleep 10
      [ -f "$RUN/harness.done" ] && exit 0
      calls="$(tail -n "+$((LINES_BEFORE + 1))" "$DAEMON_LOG" | grep -c 'launch_slot_' || true)"
      if [ "$calls" -ge "$CAP" ]; then
        if [ "$(ps -o comm= -p "$HARNESS_PID" 2>/dev/null || true)" = "$HARNESS_COMM" ]; then
          echo "the cap of $CAP model calls was reached at $calls; stopping process $HARNESS_PID" >> "$RUN/cap.log"
          printf 'stopped at the cap\n' > "$RUN/capped"
          kill -TERM "$HARNESS_PID" 2>/dev/null || true
        fi
        exit 0
      fi
    done
  ) &
  WATCHER_PID=$!
fi

wait "$HARNESS_PID"
STATUS=$?
ENDED="$(date +%s)"
: > "$RUN/harness.done"
if [ -n "$WATCHER_PID" ]; then
  wait "$WATCHER_PID" 2>/dev/null || true
fi

# ---- measure ---------------------------------------------------------------

bash "$HERE/countcalls.sh" "$DAEMON_LOG" "$LINES_BEFORE" > "$RUN/countcalls.txt" 2>&1 || true

# The checker runs only after the harness has exited, so nothing it does can be
# mistaken for the harness's own work.
timeout 300 node "$HERE/check-tater.mjs" "$WORK" \
  --harness "$HARNESS" --run "$PHASE-$LABEL" --shot "$RUN/screenshot.png" \
  > "$RUN/check.json" 2> "$RUN/check.err" || true

FINISHED_BY=exited
[ -f "$RUN/capped" ] && FINISHED_BY="stopped at the cap"
if [ -n "$MINUTES" ] && { [ "$STATUS" = "124" ] || [ "$STATUS" = "137" ]; }; then
  FINISHED_BY="stopped at the wall-clock cap"
fi

python3 - "$RUN/facts.json" <<PYTHON
import json
import sys

json.dump({
    "phase": "$PHASE",
    "harness": "$HARNESS",
    "label": "$LABEL",
    "card": "$CARD",
    "port": $PORT,
    "model": "$MODEL",
    "base_url": "$BASE_URL",
    "context_length": $CONTEXT,
    "cap": "${CAP:-}",
    "minutes_cap": "${MINUTES:-}",
    "budget_raised": "$BUDGET_RAISED" == "yes",
    "started": $STARTED,
    "ended": $ENDED,
    "wall_seconds": $ENDED - $STARTED,
    "exit_status": $STATUS,
    "finished_by": "$FINISHED_BY",
    "run_folder": "$RUN",
    "daemon_log": "$DAEMON_LOG",
    "daemon_log_lines_before": $LINES_BEFORE,
}, open(sys.argv[1], "w"), indent=2)
PYTHON

python3 "$HERE/tater_result.py" "$RUN"

echo
cat "$RUN/result.txt"
