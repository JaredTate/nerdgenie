#!/usr/bin/env bash
# Nine clean runs on Opus 4.8: three each of Nerd Genie, OpenClaw and Hermes, one at
# a time, every run from a brand-new empty work folder and a brand-new home,
# effort medium wherever the knob exists, no cap of any kind. Old data is purged
# first. Nothing is killed by name.
set -u
REPO=$HOME/Code/nerdgenie
BASE=$HOME/work/bench/opus3
TASK=$HOME/work/bench/canonical/task.txt
cd "$REPO" || exit 1

echo "== $(date '+%T') purge old data"
rm -rf $HOME/work/bench/opus2 "$BASE"
rm -rf $HOME/.claude/projects/-home-jared-work-bench-opus2-* $HOME/.claude/projects/-home-jared-work-bench-opus3-*
mkdir -p "$BASE"
sha=$(sha256sum "$TASK" | cut -c1-8); [ "$sha" = c8ed58f2 ] || { echo "task sha mismatch $sha"; exit 1; }
cp "$TASK" "$BASE/task.txt"
curl -s -m 3 http://127.0.0.1:19091/health | grep -q '"ok"' || { echo "daemon A is down; Hermes needs it"; exit 1; }
go build -o bin/nerdgenie ./cmd/nerdgenie || exit 1
echo "nerdgenie built from $(git rev-parse --short HEAD); claude $(claude --version | cut -d' ' -f1); openclaw $(openclaw --version 2>/dev/null | head -1); hermes $(hermes --version 2>/dev/null | head -1)" | tee "$BASE/versions.txt"

fresh_work() { # $1 = run folder
  rm -rf "$1"; mkdir -p "$1/work"; git init -q "$1/work"; cp "$TASK" "$1/work/task.txt"
  [ "$(ls -A "$1/work" | grep -v -E '^(\.git|task\.txt)$' | wc -l)" = 0 ] || { echo "work folder not empty"; exit 1; }
}

run_nerdgenie() { # $1 = run number
  R="$BASE/nerdgenie-$1"; fresh_work "$R"; H="$R/home"
  NERDGENIE_HOME="$H" bin/nerdgenie init --yes >/dev/null 2>&1 || { echo "init failed"; return 1; }
  python3 - "$H/config.toml" "$R/work" <<'PY'
import sys, re
cfg, work = sys.argv[1], sys.argv[2]; t = open(cfg).read()
t = re.sub(r'^default_model = .*$', 'default_model = "claude"', t, count=1, flags=re.M)
t = re.sub(r'^fallback_chain = .*$', 'fallback_chain = []', t, count=1, flags=re.M)
t = re.sub(r'^sandbox_roots = .*$', 'sandbox_roots = ["%s"]' % work, t, count=1, flags=re.M)
parts = t.split('[[models]]')
for i, part in enumerate(parts):
    if 'program = "claude"' in part:
        parts[i] = re.sub(r'^think = .*$', 'think = "medium"', part, count=1, flags=re.M)
t = '[[models]]'.join(parts)
t += '\n[caps]\nrounds_per_task = 1000000\ntime_per_task = "1000h"\ntime_per_turn = "100h"\ntime_per_tool = "100h"\n'
open(cfg, 'w').write(t)
PY
  grep -q 'think = "medium"' "$H/config.toml" || { echo "think line missing"; return 1; }
  NERDGENIE_HOME="$H" bin/nerdgenie doctor > "$R/doctor.txt" 2>&1 || { cat "$R/doctor.txt"; return 1; }
  s=$(date +%s)
  bash scripts/bench/run-nerdgenie.sh "$REPO/bin/nerdgenie" "$H" "$R/work/task.txt" 100000 > "$R/run.log" 2>&1 < /dev/null
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness nerdgenie --run "$1" > "$R/check.json" 2> "$R/check.err"
}

run_openclaw() {
  R="$BASE/openclaw-$1"; fresh_work "$R"
  s=$(date +%s)
  ( cd "$R/work" && openclaw agent exec --message-file "$R/work/task.txt" --model anthropic/claude-opus-4-8 --cwd "$R/work" --timeout 0 --json > "$R/out.json" 2> "$R/err.log" < /dev/null )
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness openclaw --run "$1" > "$R/check.json" 2> "$R/check.err"
}

run_hermes() {
  R="$BASE/hermes-$1"; fresh_work "$R"; H="$R/home"; mkdir -p "$H"
  cat > "$H/config.yaml" <<'YAML'
model:
  default: local-coder
  provider: custom
  base_url: http://127.0.0.1:19091/v1
  context_length: 262144
custom_providers:
  - name: bench
    base_url: http://127.0.0.1:19091/v1
    api_key: local
    models:
      local-coder:
        context_length: 262144
terminal:
  timeout: 1800
telemetry:
  shared_metrics:
    enabled: false
YAML
  { printf 'Use Claude Code for all of the work: run claude in print mode (claude -p) with --model claude-opus-4-8, --effort medium and --dangerously-skip-permissions, in this folder, hand it the task below word for word, wait for it to finish, and then stop. Do not write any code yourself.\n\nThe task:\n\n'; cat "$TASK"; } > "$R/prompt.txt"
  lines_before=$(wc -l < ~/llm/logs/19091.log)
  s=$(date +%s)
  ( cd "$R/work" && ANTHROPIC_MODEL=claude-opus-4-8 HERMES_HOME="$H" hermes chat -q "$(cat "$R/prompt.txt")" --provider bench -m local-coder --yolo --in "$R/work" > "$R/out.log" 2>&1 < /dev/null )
  echo "exit $? launch_to_exit $(( $(date +%s) - s ))" > "$R/wall.txt"
  bash scripts/bench/countcalls.sh ~/llm/logs/19091.log "$lines_before" > "$R/qwen-calls.txt"
  timeout 200 node scripts/bench/check-tater.mjs "$R/work" --harness hermes --run "$1" > "$R/check.json" 2> "$R/check.err"
}

for round in 1 2 3; do
  for h in nerdgenie openclaw hermes; do
    echo "== $(date '+%T') round $round: $h"
    "run_$h" "$round"
    echo "== $(date '+%T') $h $round: $(cat "$BASE/$h-$round/wall.txt")"
  done
done
echo "== $(date '+%T') all nine done"
