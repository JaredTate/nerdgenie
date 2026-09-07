#!/usr/bin/env bash
# One clean round of all four harnesses on GPT-5.6 Sol at thinking medium, on
# the user's ChatGPT/Codex subscription, no API key, each harness driving the
# model with its own loop: Nerd Genie through its "codex" provider, which borrows
# the codex program's login and drives the model itself, opencode
# through its ChatGPT login, Hermes and OpenClaw through the codex login, OpenClaw pinned to its own
# loop rather than its codex runtime. One
# at a time, every run from a brand-new empty work folder and a brand-new home,
# no cap of any kind. Nothing is killed by name.
#
#   gpt_runs.sh [round]      (round defaults to 1; folders are <harness>-<round>)
set -u
ROUND="${1:-1}"
REPO=$HOME/Code/nerdgenie
BASE=$HOME/work/bench/gpt
TASK=$HOME/work/bench/canonical/task.txt
MODEL=gpt-5.6-sol
cd "$REPO" || exit 1
mkdir -p "$BASE"
sha=$(sha256sum "$TASK" | cut -c1-8); [ "$sha" = c8ed58f2 ] || { echo "task sha mismatch $sha"; exit 1; }
cp "$TASK" "$BASE/task.txt"
[ -x bin/nerdgenie ] || go build -o bin/nerdgenie ./cmd/nerdgenie || exit 1
echo "nerdgenie $(git rev-parse --short HEAD); $(codex --version 2>&1 | head -1); opencode $($HOME/.opencode/bin/opencode --version 2>/dev/null | head -1); $(openclaw --version 2>/dev/null | head -1); $(hermes --version 2>/dev/null | head -1)" | tee "$BASE/versions.txt"

fresh_work() { # $1 = run folder
  rm -rf "$1"; mkdir -p "$1/work"; git init -q "$1/work"; cp "$TASK" "$1/work/task.txt"
  [ "$(ls -A "$1/work" | grep -v -E '^(\.git|task\.txt)$' | wc -l)" = 0 ] || { echo "work folder not empty"; exit 1; }
}

run_nerdgenie() {
  R="$BASE/nerdgenie-$1"; fresh_work "$R"; H="$R/home"
  NERDGENIE_HOME="$H" bin/nerdgenie init --yes >/dev/null 2>&1 || { echo "init failed"; return 1; }
  # The model is reached through Nerd Genie's "codex" provider: a fresh, uncommented
  # [[models]] block named "gpt" is appended, whatever the init template wrote
  # (its own codex example is commented out), and made the default.
  python3 - "$H/config.toml" "$R/work" "$MODEL" <<'PY'
import sys, re
cfg, work, model = sys.argv[1], sys.argv[2], sys.argv[3]; t = open(cfg).read()
t = re.sub(r'^default_model = .*$', 'default_model = "gpt"', t, count=1, flags=re.M)
t = re.sub(r'^fallback_chain = .*$', 'fallback_chain = []', t, count=1, flags=re.M)
t = re.sub(r'^sandbox_roots = .*$', 'sandbox_roots = ["%s"]' % work, t, count=1, flags=re.M)
t += '\n[[models]]\nname = "gpt"\nprovider = "codex"\nmodel_name = "%s"\ncontext_length = 400000\nthink = "medium"\n' % model
t += '\n[caps]\nrounds_per_task = 1000000\ntime_per_task = "1000h"\ntime_per_turn = "100h"\ntime_per_tool = "100h"\n'
open(cfg, 'w').write(t)
PY
  grep -q '^default_model = "gpt"$' "$H/config.toml" && grep -q '^name = "gpt"$' "$H/config.toml" && grep -q '^provider = "codex"$' "$H/config.toml" \
    && grep -q "^model_name = \"$MODEL\"$" "$H/config.toml" && grep -q '^think = "medium"$' "$H/config.toml" || { echo "config edit failed"; return 1; }
  NERDGENIE_HOME="$H" bin/nerdgenie doctor > "$R/doctor.txt" 2>&1 || { cat "$R/doctor.txt"; return 1; }
  s=$(date +%s)
  bash scripts/bench/run-nerdgenie.sh "$REPO/bin/nerdgenie" "$H" "$R/work/task.txt" 100000 > "$R/run.log" 2>&1 < /dev/null
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness nerdgenie --run "gpt$1" > "$R/check.json" 2> "$R/check.err"
}

run_opencode() {
  R="$BASE/opencode-$1"; fresh_work "$R"; H="$R/home"
  mkdir -p "$H/data/opencode" "$H/config/opencode" "$H/cache" "$H/state"
  # The ChatGPT login lives in the user's opencode credential file; the fresh
  # data folder gets a copy of that one file and nothing else.
  cp $HOME/.local/share/opencode/auth.json "$H/data/opencode/auth.json"
  cat > "$H/config/opencode/opencode.json" <<JSON
{
  "\$schema": "https://opencode.ai/config.json",
  "model": "openai/$MODEL",
  "share": "disabled",
  "autoupdate": false,
  "permission": { "edit": "allow", "bash": "allow", "webfetch": "allow" }
}
JSON
  s=$(date +%s)
  ( cd "$R/work" && XDG_DATA_HOME="$H/data" XDG_CONFIG_HOME="$H/config" XDG_CACHE_HOME="$H/cache" XDG_STATE_HOME="$H/state" $HOME/.opencode/bin/opencode run -m "openai/$MODEL" --variant medium --format json "$(cat "$R/work/task.txt")" > "$R/out.jsonl" 2> "$R/err.log" < /dev/null )
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness opencode --run "gpt$1" > "$R/check.json" 2> "$R/check.err"
}

run_hermes() {
  R="$BASE/hermes-$1"; fresh_work "$R"; H="$R/home"; mkdir -p "$H"
  cat > "$H/config.yaml" <<YAML
model:
  default: $MODEL
  provider: openai-codex
terminal:
  timeout: 1800
telemetry:
  shared_metrics:
    enabled: false
YAML
  # Hermes only looks in its own home for the codex login, so the fresh home
  # gets a copy of that one credential from the user's Hermes file, the way the
  # opencode run copies its login file. Nothing else is copied.
  python3 - "$H/auth.json" <<'PY'
import json, sys
src = json.load(open('$HOME/.hermes/auth.json'))
out = {"version": src.get("version", 1), "providers": {"openai-codex": src["providers"]["openai-codex"]},
       "credential_pool": {"openai-codex": src["credential_pool"]["openai-codex"]}, "active_provider": "openai-codex"}
json.dump(out, open(sys.argv[1], 'w'))
PY
  chmod 600 "$H/auth.json"
  s=$(date +%s)
  ( cd "$R/work" && HERMES_HOME="$H" hermes chat -q "$(cat "$R/work/task.txt")" --provider openai-codex -m "$MODEL" --reasoning medium --yolo --in "$R/work" > "$R/out.log" 2>&1 < /dev/null )
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness hermes --run "gpt$1" > "$R/check.json" 2> "$R/check.err"
}

run_openclaw() {
  R="$BASE/openclaw-$1"; fresh_work "$R"; mkdir -p "$R/state"
  # On a ChatGPT subscription OpenClaw's default is to hand the whole turn to
  # the codex program (its "codex" runtime), which measures codex, not
  # OpenClaw. Pinning agentRuntime.id = "openclaw" on the model keeps the turn
  # in OpenClaw's own loop, per its docs (providers/openai.md, "Implicit agent
  # runtime"). The pinned config is the user's own config plus that one line;
  # the kept state folder proves which runtime ran (a codex rollout file means
  # the codex runtime).
  python3 - "$R/openclaw.json" "$MODEL" <<'PY'
import json, sys
cfg = json.load(open('$HOME/.openclaw/openclaw.json'))
models = cfg.setdefault('agents', {}).setdefault('defaults', {}).setdefault('models', {})
entry = models.get('openai/' + sys.argv[2]) or {}
entry['agentRuntime'] = {'id': 'openclaw'}
models['openai/' + sys.argv[2]] = entry
json.dump(cfg, open(sys.argv[1], 'w'), indent=2)
PY
  chmod 600 "$R/openclaw.json"
  s=$(date +%s)
  ( cd "$R/work" && openclaw agent exec --config "$R/openclaw.json" --state-dir "$R/state" --message-file "$R/work/task.txt" --model "openai/$MODEL" --thinking medium --cwd "$R/work" --timeout 0 --json > "$R/out.json" 2> "$R/err.log" < /dev/null )
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  [ "$(find "$R/state" -name 'rollout-*.jsonl' | wc -l)" = 0 ] || echo "WARNING: openclaw ran through the codex program, not its own loop" >> "$R/wall.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness openclaw --run "gpt$1" > "$R/check.json" 2> "$R/check.err"
}

for h in nerdgenie opencode hermes openclaw; do
  echo "== $(date '+%T') round $ROUND: $h"
  "run_$h" "$ROUND"
  echo "== $(date '+%T') $h $ROUND: $(cat "$BASE/$h-$ROUND/wall.txt")"
done
echo "== $(date '+%T') round $ROUND done"
