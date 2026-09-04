#!/usr/bin/env bash
# Test the benchmark runner without a model and without a daemon.
#
#   bash scripts/bench/tater_run_test.sh
#
# There is nothing to install: this is plain bash with a handful of check
# functions. Every test writes only inside one throwaway folder under the
# system's temporary directory, which is deleted at the end, and the runner is
# pointed at it with TATER_ROOT, so nothing under ~/work is touched.
#
# What is checked:
#   1. The runner refuses a bad phase, harness, card, label or task, and refuses
#      a second run on a card a live run already holds.
#   2. A dry run of all four harnesses on both cards writes the fresh run
#      folder, the harness's own fresh config, and exactly the environment and
#      command the brief calls for, all compared against expected text.
#   3. tater_result.py turns each harness's raw output into the same flat
#      result.json, from made-up but true-to-shape files.
#   4. tater_table.mjs prints a markdown table over those results.
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
RUNNER="$HERE/tater_run.sh"
SCRATCH="${TMPDIR:-/tmp}/tater-run-test-$$"
ROOT="$SCRATCH/runs"
OPENCODE_BIN="${TATER_OPENCODE_BIN:-/home/jared/.opencode/bin/opencode}"

rm -rf "$SCRATCH"
mkdir -p "$ROOT"
trap 'rm -rf "$SCRATCH"' EXIT

passed=0
failed=0

pass() { passed=$((passed + 1)); printf '  ok    %s\n' "$1"; }

fail() {
  failed=$((failed + 1))
  printf '  FAIL  %s\n' "$1"
  shift
  for line in "$@"; do printf '        %s\n' "$line"; done
}

same() { # same NAME EXPECTED ACTUAL
  if [ "$2" = "$3" ]; then pass "$1"; else fail "$1" "expected: $2" "actual:   $3"; fi
}

same_file() { # same_file NAME EXPECTED_FILE ACTUAL_FILE
  if diff -u "$2" "$3" > "$SCRATCH/diff.txt" 2>&1; then
    pass "$1"
  else
    fail "$1" "the file is not the expected text:"
    sed 's/^/        /' "$SCRATCH/diff.txt"
  fi
}

has_line() { # has_line NAME FILE LINE
  if grep -Fxq -- "$3" "$2"; then pass "$1"; else fail "$1" "no line reading: $3" "in $2"; fi
}

starts_with() { # starts_with NAME HAYSTACK PREFIX
  case "$2" in "$3"*) pass "$1";; *) fail "$1" "does not start with: $3" "it is:              ${2:0:200}";; esac
}

ends_with() { # ends_with NAME HAYSTACK SUFFIX
  case "$2" in *"$3") pass "$1";; *) fail "$1" "does not end with: $3" "it is:             ${2: -200}";; esac
}

exists() { # exists NAME PATH
  if [ -e "$2" ]; then pass "$1"; else fail "$1" "there is nothing at $2"; fi
}

# ---- 1. what the runner refuses --------------------------------------------

echo "the runner refuses what it should"

refuses() { # refuses NAME EXPECTED_FRAGMENT ARGS...
  local name="$1" fragment="$2"
  shift 2
  local output status
  output="$(TATER_ROOT="$ROOT" "$RUNNER" "$@" 2>&1)"
  status=$?
  if [ "$status" -eq 0 ]; then
    fail "$name" "it ran instead of refusing"
    return
  fi
  case "$output" in
    *"$fragment"*) pass "$name";;
    *) fail "$name" "expected a message about: $fragment" "it said: $output";;
  esac
}

refuses "an unknown phase" "the phase must be" tuesday nerdgenie t --dry-run
refuses "the opus phase, which is driven by hand" "driven by hand" opus nerdgenie t --dry-run
refuses "an unknown harness" "the harness must be" qwen nosuch t --dry-run
refuses "an unknown card" "the card must be" qwen nerdgenie t c --dry-run
refuses "a label with a slash" "must be letters, digits" qwen nerdgenie a/b --dry-run
refuses "too few arguments" "usage: tater_run.sh" qwen nerdgenie
refuses "an unknown option" "unknown option" qwen nerdgenie t --go-faster

printf 'not the canonical task\n' > "$SCRATCH/wrong-task.txt"
wrong_output="$(TATER_ROOT="$ROOT" TATER_TASK="$SCRATCH/wrong-task.txt" "$RUNNER" qwen nerdgenie t --dry-run 2>&1)"
case "$wrong_output" in
  *"not the canonical task; refusing to run"*) pass "a task with the wrong checksum";;
  *) fail "a task with the wrong checksum" "it said: $wrong_output";;
esac

# A run folder that names this test's own process id, with this process's own
# command name, stands in for a run that is still going on that card.
mkdir -p "$ROOT/qwen-hermes-busy"
printf '19091\n' > "$ROOT/qwen-hermes-busy/port"
printf '%s\n' "$$" > "$ROOT/qwen-hermes-busy/harness.pid"
ps -o comm= -p "$$" > "$ROOT/qwen-hermes-busy/harness.comm"
refuses "a second run on a card that is taken" "is still running on port 19091" qwen hermes second a --dry-run
rm -rf "$ROOT/qwen-hermes-busy"

# ---- 2. the dry runs -------------------------------------------------------

TASK="${TATER_TASK:-$HOME/work/bench/canonical/task.txt}"
if [ ! -f "$TASK" ]; then
  echo "There is no canonical task at $TASK, so the dry runs cannot be checked." >&2
  exit 1
fi
if [ ! -x "$REPO/bin/nerdgenie" ]; then
  echo "There is no Nerd Genie binary at $REPO/bin/nerdgenie; run \"make build\" first." >&2
  exit 1
fi

model="local-coder"
context=262144

# The runner writes the launch details as a block of named lines and then one
# very long command line, so the block is compared as a whole and the command
# line on its own.
head_of() { sed -n '/^command:/q;p' "$1"; }
command_of() { grep -m1 '^command:' "$1"; }

for card in a b; do
  [ "$card" = a ] && port=19091 || port=19093
  url="http://127.0.0.1:$port/v1"
  for harness in nerdgenie opencode hermes openclaw; do
    echo
    echo "a dry run of $harness on card $card"
    run="$ROOT/qwen-$harness-$card"

    if ! TATER_ROOT="$ROOT" "$RUNNER" qwen "$harness" "$card" "$card" --dry-run \
        > "$SCRATCH/dry.out" 2>&1; then
      fail "$harness card $card dry run" "the runner exited nonzero:"
      sed 's/^/        /' "$SCRATCH/dry.out"
      continue
    fi
    pass "$harness card $card dry run finished"

    # The fresh folder.
    exists "$harness card $card work folder" "$run/work"
    exists "$harness card $card git repository in the work folder" "$run/work/.git"
    exists "$harness card $card home folder" "$run/home"
    same "$harness card $card task copied in byte for byte" \
      "$(sha256sum "$TASK" | cut -d' ' -f1)" "$(sha256sum "$run/work/task.txt" | cut -d' ' -f1)"
    same "$harness card $card work folder holds only the task and git" \
      "task.txt" "$(ls -A "$run/work" | grep -v '^.git$' | tr '\n' ' ' | sed 's/ $//')"
    has_line "$harness card $card said nothing was launched" "$SCRATCH/dry.out" \
      "dry run: nothing was launched"
    has_line "$harness card $card named the health check it would make" "$SCRATCH/dry.out" \
      "would check first: curl -s http://127.0.0.1:$port/health"

    # The launch block, against expected text.
    budget=no
    [ "$harness" = "nerdgenie" ] && budget=yes
    {
      echo "phase: qwen"
      echo "harness: $harness"
      echo "label: $card"
      echo "card: $card"
      echo "port: $port"
      echo "model: $model"
      echo "base url: $url"
      echo "context length: $context"
      echo "work folder: $run/work"
      echo "home folder: $run/home"
      echo "launched from: $run/work"
      echo "call cap: none"
      echo "wall-clock cap: none"
      echo "nerdgenie budget raised: $budget"
      echo "environment:"
      case "$harness" in
        nerdgenie) echo "  NERDGENIE_HOME=$run/home";;
        opencode)
          echo "  XDG_DATA_HOME=$run/home/data"
          echo "  XDG_CONFIG_HOME=$run/home/config"
          echo "  XDG_CACHE_HOME=$run/home/cache"
          echo "  XDG_STATE_HOME=$run/home/state";;
        hermes) echo "  HERMES_HOME=$run/home/hermes";;
        openclaw) echo "  OPENCLAW_HOME=$run/home/openclaw";;
      esac
    } > "$SCRATCH/expected-head.txt"
    head_of "$run/launch.txt" > "$SCRATCH/actual-head.txt"
    same_file "$harness card $card launch details" "$SCRATCH/expected-head.txt" "$SCRATCH/actual-head.txt"

    # The command itself.
    line="$(command_of "$run/launch.txt")"
    case "$harness" in
      nerdgenie)
        same "$harness card $card command" \
          "command: bash $HERE/run-nerdgenie.sh $REPO/bin/nerdgenie $run/home $run/work/task.txt" "$line";;
      opencode)
        starts_with "$harness card $card command" "$line" \
          "command: $OPENCODE_BIN run -m bench/$model --format json "
        ends_with "$harness card $card command ends with the task" "$line" \
          "index.html opens in a browser and plays.'";;
      hermes)
        starts_with "$harness card $card command" "$line" "command: hermes chat -q "
        ends_with "$harness card $card command" "$line" \
          " --provider bench -m $model --yolo --in $run/work";;
      openclaw)
        same "$harness card $card command" \
          "command: openclaw agent exec --config $run/home/openclaw/.openclaw/openclaw.json --message-file $run/work/task.txt --model bench/$model --local-model-lean --cwd $run/work --timeout 0 --json" \
          "$line";;
    esac

    # The harness's own fresh config, against expected text.
    case "$harness" in
      nerdgenie)
        # The lines above the model block come from "nerdgenie init", which guesses
        # at what is installed, so only the three settings this benchmark
        # decides and the whole block it writes are compared.
        config="$run/home/config.toml"
        has_line "nerdgenie card $card names bench as its model" "$config" 'default_model = "bench"'
        has_line "nerdgenie card $card has no fallback" "$config" "fallback_chain = []"
        has_line "nerdgenie card $card fences on the work folder" "$config" "sandbox_roots = [\"$run/work\"]"
        cat > "$SCRATCH/expected-config" <<EXPECTED
# The one model this benchmark run talks to.
[[models]]
name = "bench"
provider = "openai"
base_address = "$url"
model_name = "$model"
context_length = $context

# The benchmark puts no cap on any harness, so Nerd Genie's own task budget is
# raised out of the way. Every other cap is left at its default.
[caps]
rounds_per_task = 1000000
time_per_task = "240h"
EXPECTED
        sed -n '/^# The one model/,$p' "$config" > "$SCRATCH/actual-config"
        same_file "nerdgenie card $card model block and raised budget" \
          "$SCRATCH/expected-config" "$SCRATCH/actual-config"
        ;;
      opencode)
        cat > "$SCRATCH/expected-config" <<EXPECTED
{
  "\$schema": "https://opencode.ai/config.json",
  "provider": {
    "bench": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "the benchmark endpoint",
      "options": {
        "baseURL": "$url",
        "apiKey": "local"
      },
      "models": {
        "$model": {
          "name": "$model",
          "limit": {
            "context": $context,
            "output": 32000
          }
        }
      }
    }
  },
  "model": "bench/$model",
  "share": "disabled",
  "autoupdate": false,
  "permission": {
    "edit": "allow",
    "bash": "allow",
    "webfetch": "allow"
  }
}
EXPECTED
        same_file "opencode card $card config" \
          "$SCRATCH/expected-config" "$run/home/config/opencode/opencode.json"
        ;;
      hermes)
        cat > "$SCRATCH/expected-config" <<EXPECTED
# Written by tater_run.sh for one benchmark run. Hermes's own defaults are
# the harness, so nothing below changes its sampling, memory or turn limit.
model:
  default: $model
  provider: custom
  base_url: $url
  context_length: $context
custom_providers:
  - name: bench
    base_url: $url
    api_key: local
    models:
      $model:
        context_length: $context
telemetry:
  shared_metrics:
    enabled: false
EXPECTED
        same_file "hermes card $card config" "$SCRATCH/expected-config" "$run/home/hermes/config.yaml"
        ;;
      openclaw)
        cat > "$SCRATCH/expected-config" <<EXPECTED
{
  "models": {
    "providers": {
      "bench": {
        "baseUrl": "$url",
        "apiKey": "local",
        "api": "openai-completions",
        "timeoutSeconds": 900,
        "models": [
          {
            "id": "$model",
            "name": "$model",
            "reasoning": false,
            "input": [
              "text"
            ],
            "cost": {
              "input": 0,
              "output": 0,
              "cacheRead": 0,
              "cacheWrite": 0
            },
            "contextWindow": $context,
            "maxTokens": 32000
          }
        ]
      }
    }
  }
}
EXPECTED
        same_file "openclaw card $card config" \
          "$SCRATCH/expected-config" "$run/home/openclaw/.openclaw/openclaw.json"
        ;;
    esac
  done
done

echo
echo "a cap and a wall-clock cap show up in the launch details when they are set"
TATER_ROOT="$ROOT" TATER_CAP=40 TATER_MINUTES=12 "$RUNNER" qwen openclaw capped a --dry-run \
  > "$SCRATCH/capped.out" 2>&1
has_line "the call cap is named" "$SCRATCH/capped.out" "call cap: 40"
has_line "the wall-clock cap is named" "$SCRATCH/capped.out" "wall-clock cap: 12 minutes"

echo
echo "nothing was written outside the test's own folder"
if [ -e "$HOME/work/bench/tater2/qwen-nerdgenie-a" ]; then
  fail "the default benchmark folder was left alone" "the test wrote into ~/work/bench/tater2"
else
  pass "the default benchmark folder was left alone"
fi

# ---- 3. one result from each harness's own kind of output -------------------

echo
echo "the measurements are read from the daemon log and each harness's own files"

CHECKER_REPORT='{"harness":"x","run":"1","correctness":{"passed":6,"of":6},
  "plays":{"passed":4},
  "thoroughness":{"testsWritten":8,"testsPassing":8,"testsTotal":8,"gameJsLines":74,
  "quality":{"showsTurn":true,"announcesDraw":true,"announcesWin":true,
  "hasNewGame":true,"oneAccent":true,"purelyFunctional":false}}}'

COUNTCALLS='model calls: 14
prompt tokens read: 20000
prompt tokens read per call on average: 1429
largest single prefill: 9000
generated tokens: 5000
total input tokens: 120000
total output tokens: 5000
largest context: 31000
lines now: 900'

make_run() { # make_run FOLDER HARNESS
  local folder="$1" harness="$2"
  mkdir -p "$folder/work" "$folder/home"
  printf '%s\n' "$CHECKER_REPORT" > "$folder/check.json"
  printf '%s\n' "$COUNTCALLS" > "$folder/countcalls.txt"
  python3 - "$folder/facts.json" "$harness" <<'PYTHON'
import json
import sys
path, harness = sys.argv[1:3]
json.dump({
    "phase": "qwen", "harness": harness, "label": "t", "card": "a",
    "port": 19091, "model": "local-coder", "base_url": "u", "context_length": 262144,
    "cap": "", "minutes_cap": "", "budget_raised": harness == "nerdgenie",
    "started": 100, "ended": 400, "wall_seconds": 300, "exit_status": 0,
    "finished_by": "exited", "run_folder": path, "daemon_log": "log",
    "daemon_log_lines_before": 0,
}, open(path, "w"))
PYTHON
}

field() { # field FOLDER NAME
  python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]])' \
    "$1/result.json" "$2"
}

# The daemon's numbers are read the same way for every harness.
run="$SCRATCH/result-nerdgenie"
make_run "$run" nerdgenie
printf '12:00:00 << status tokensIn=120000 | tokensOut=5000 | cost=0\n12:00:05 == done in 5.0 min: replies=3 previews=7 asks=2 errors=0 toolLines=11 recordLines=4 continues=1\n' \
  > "$run/home/drive.log"
python3 "$HERE/tater_result.py" "$run"
same "model calls, from the daemon log" "14" "$(field "$run" calls)"
same "input tokens, cached and uncached both" "120000" "$(field "$run" tokens_in)"
same "the cached part" "100000" "$(field "$run" cache_read)"
same "the part read afresh" "20000" "$(field "$run" prompt_tokens_read)"
same "output tokens" "5000" "$(field "$run" tokens_out)"
same "no dollar figure for a local model" "None" "$(field "$run" cost_usd)"
same "nerdgenie driver asks" "2" "$(field "$run" own_asks)"
same "nerdgenie driver continues" "1" "$(field "$run" own_continues)"
same "nerdgenie's own input tokens" "120000" "$(field "$run" own_tokensIn)"
same "nerdgenie's budget was raised" "True" "$(field "$run" budget_raised)"
same "nerdgenie finished the task" "True" "$(field "$run" finished)"
same "nerdgenie logic checks" "6" "$(field "$run" logic_passed)"
same "nerdgenie play-throughs" "4" "$(field "$run" plays_passed)"
exists "nerdgenie plain-words result" "$run/result.txt"

run="$SCRATCH/result-opencode"
make_run "$run" opencode
cat > "$run/harness.out" <<'OUT'
{"type":"step_start","part":{}}
{"type":"step_finish","part":{"type":"step-finish","tokens":{"input":500,"output":200,"reasoning":10,"cache":{"read":900,"write":0}},"cost":0.01}}
{"type":"step_finish","part":{"type":"step-finish","tokens":{"input":40,"output":15,"cache":{"read":1400,"write":0}},"cost":0.002}}
not json at all
OUT
python3 "$HERE/tater_result.py" "$run"
same "opencode's own steps" "2" "$(field "$run" own_steps)"
same "opencode's own input tokens" "540" "$(field "$run" own_input_tokens)"
same "opencode's own cache reads" "2300" "$(field "$run" own_cache_read_tokens)"
same "opencode's own dollars" "0.012" "$(field "$run" own_cost_usd)"

run="$SCRATCH/result-hermes"
make_run "$run" hermes
mkdir -p "$run/home/hermes"
python3 - "$run/home/hermes/state.db" "$run/work" <<'PYTHON'
import sqlite3
import sys
database, work = sys.argv[1:3]
connection = sqlite3.connect(database)
connection.execute(
    "CREATE TABLE sessions (id TEXT, cwd TEXT, started_at REAL, api_call_count INT,"
    " input_tokens INT, output_tokens INT, cache_read_tokens INT, cache_write_tokens INT)"
)
connection.execute("INSERT INTO sessions VALUES ('older', ?, 1.0, 1, 1, 1, 1, 1)", (work,))
connection.execute("INSERT INTO sessions VALUES ('mine', ?, 2.0, 14, 120000, 5000, 100000, 0)", (work,))
connection.execute("INSERT INTO sessions VALUES ('elsewhere', '/somewhere/else', 3.0, 99, 9, 9, 9, 9)")
connection.commit()
connection.close()
PYTHON
python3 "$HERE/tater_result.py" "$run"
same "hermes's own call count, from its newest session row" "14" "$(field "$run" own_api_call_count)"
same "hermes's own cache reads" "100000" "$(field "$run" own_cache_read_tokens)"

run="$SCRATCH/result-openclaw"
make_run "$run" openclaw
cat > "$run/harness.out" <<'OUT'
[agent/embedded] tool-search: cataloged 32 tools
[provider-transport-fetch] start provider=bench model=local-coder
{
  "ok": true,
  "status": "ok",
  "assistantTurns": 6,
  "usage": {"inputTokens": 30000, "outputTokens": 900, "cacheWrite": 41882},
  "toolSummary": {"bash": 4, "edit": 3}
}
all done
OUT
python3 "$HERE/tater_result.py" "$run"
same "openclaw's own status" "ok" "$(field "$run" own_status)"
same "openclaw's own assistant turns" "6" "$(field "$run" own_assistant_turns)"
same "openclaw's own input tokens" "30000" "$(field "$run" own_usage_inputTokens)"
same "openclaw's own cache writes, a key only its route reports" "41882" \
  "$(field "$run" own_usage_cacheWrite)"
same "openclaw's own tool summary" '{"bash": 4, "edit": 3}' "$(field "$run" own_tool_summary)"

# A run the checker could not judge is not counted as finished.
run="$SCRATCH/result-unjudged"
make_run "$run" openclaw
printf 'the checker fell over\n' > "$run/check.err"
printf '' > "$run/check.json"
python3 "$HERE/tater_result.py" "$run"
same "a run with no checker verdict did not finish" "False" "$(field "$run" finished)"
same "and the reason is kept" "the checker fell over" "$(field "$run" checker_note)"

# ---- 4. the table ----------------------------------------------------------

echo
echo "the table gathers every finished run"
TABLE_ROOT="$SCRATCH/table"
mkdir -p "$TABLE_ROOT"
for made in nerdgenie opencode hermes openclaw; do
  mkdir -p "$TABLE_ROOT/qwen-$made-t"
  cp "$SCRATCH/result-$made/result.json" "$TABLE_ROOT/qwen-$made-t/result.json"
done
node "$HERE/tater_table.mjs" "$TABLE_ROOT" > "$SCRATCH/table.md" 2>&1
has_line "there is a qwen table" "$SCRATCH/table.md" "## qwen phase"
has_line "the columns are the ones asked for" "$SCRATCH/table.md" \
  "| harness | finished | wall | calls | tokens in | cached | out | cost | model time | harness time | logic 6 | plays 4 | tests written | tests passing |"
has_line "nerdgenie's row" "$SCRATCH/table.md" \
  "| nerdgenie (t) | yes | 5m 00s | 14 | 120,000 | 100,000 | 5,000 | — | — | — | 6 | 4 | 8 | 8 |"

# ---- how it went -----------------------------------------------------------

echo
echo "$passed checks passed, $failed failed"
[ "$failed" -eq 0 ]
