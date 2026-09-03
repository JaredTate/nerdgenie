#!/usr/bin/env bash
# Restart both Qwen daemons cold, then run one harness on card A and another on
# card B at the same time through the clean-state runner, so nothing from an
# earlier run is in either daemon's cache. Daemons are stopped only by the exact
# process id recorded when they were started, checked by command name.
#
#   qwen_pair.sh <harness-on-A> <harness-on-B> <label>
#
# RESTART=no skips the cold restart. Loading a daemon spikes memory by about
# twenty gigabytes for half a minute, and on 2026-09-03 that spike made the
# kernel kill other programs, so a restart only happens when the machine has
# thirty gigabytes free and the previous daemon has finished settling. Without a
# restart the daemon's cache holds one conversation, the last one it served, so
# a harness whose prompt does not begin with that conversation gets nothing from
# it; the /slots snapshot taken at launch records exactly what was cached.
set -u
A_HARNESS="$1"; B_HARNESS="$2"; LABEL="$3"; RESTART="${RESTART:-yes}"
REPO=/home/jared/Code/coeus
PIDS=/home/jared/work/bench/daemons
mkdir -p "$PIDS"
cd "$REPO" || exit 1

stop_daemon() { # $1 = port
  local pidfile="$PIDS/$1.pid" pid
  if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile")
    if [ "$(ps -o comm= -p "$pid" 2>/dev/null)" = "llama-server" ]; then
      kill -TERM "$pid"
      for _ in $(seq 1 60); do ps -p "$pid" >/dev/null 2>&1 || break; sleep 1; done
      ps -p "$pid" >/dev/null 2>&1 && { echo "daemon on $1 (pid $pid) did not stop"; exit 1; }
    fi
  fi
  # Anything else listening there is not ours to touch.
  if ss -ltn 2>/dev/null | grep -q ":$1 "; then echo "port $1 is still held by a process this script did not start; refusing"; exit 1; fi
}

available_gb() { awk '/MemAvailable/ {printf "%d", $2/1048576}' /proc/meminfo; }
shared_gb() { awk '/^Shmem:/ {printf "%d", $2/1048576}' /proc/meminfo; }

start_daemon() { # $1 = port, $2 = Vulkan device
  local log=~/llm/logs/$1.log
  for _ in $(seq 1 60); do [ "$(available_gb)" -ge 30 ] && break; sleep 5; done
  [ "$(available_gb)" -ge 30 ] || { echo "only $(available_gb) GB available; a daemon load needs thirty, refusing"; exit 1; }
  cp -n "$log" "$log.before-$(date +%H%M%S)" 2>/dev/null
  setsid ~/llm/igo.sh "$2" "$1" 262144 mtp > "$log" 2>&1 < /dev/null &
  echo $! > "$PIDS/$1.pid"
  for _ in $(seq 1 90); do curl -s -m 3 "http://127.0.0.1:$1/health" | grep -q '"ok"' && break; sleep 5; done
  curl -s -m 3 "http://127.0.0.1:$1/health" | grep -q '"ok"' || { echo "daemon on $1 did not come up"; tail -5 "$log"; exit 1; }
  # The loader's shared memory lingers past the health check; wait for it to settle.
  for _ in $(seq 1 24); do [ "$(shared_gb)" -lt 1 ] && break; sleep 5; done
}

# The card-A daemon started earlier today is recorded here so it can be stopped by pid.
[ -f "$PIDS/19091.pid" ] || { p=$(ss -ltnp 2>/dev/null | grep ":19091 " | grep -o 'pid=[0-9]*' | head -1 | cut -d= -f2); [ -n "$p" ] && echo "$p" > "$PIDS/19091.pid"; }

if [ "$RESTART" = yes ]; then
  echo "== $(date '+%T') cold restart of both daemons, one at a time"
  stop_daemon 19091; stop_daemon 19093
  start_daemon 19091 Vulkan1; start_daemon 19093 Vulkan2
else
  echo "== $(date '+%T') no restart; the daemons keep the last conversation each served"
fi
for p in 19091 19093; do curl -s -m 3 "http://127.0.0.1:$p/health" | grep -q '"ok"' || { echo "daemon on $p is not healthy"; exit 1; }; done
echo "== $(date '+%T') both healthy: A pid $(cat $PIDS/19091.pid), B pid $(cat $PIDS/19093.pid)"; free -g | sed -n 2p
mkdir -p ~/work/bench/tater2
curl -s -m 3 http://127.0.0.1:19091/slots > ~/work/bench/tater2/slots-a-before-$LABEL.json
curl -s -m 3 http://127.0.0.1:19093/slots > ~/work/bench/tater2/slots-b-before-$LABEL.json

echo "== $(date '+%T') pair: $A_HARNESS on A, $B_HARNESS on B"
bash scripts/bench/tater_run.sh qwen "$A_HARNESS" "$LABEL" a > "/home/jared/work/bench/qwen-$A_HARNESS-$LABEL.log" 2>&1 &
PA=$!
bash scripts/bench/tater_run.sh qwen "$B_HARNESS" "$LABEL" b > "/home/jared/work/bench/qwen-$B_HARNESS-$LABEL.log" 2>&1 &
PB=$!
wait "$PA"; echo "== $(date '+%T') $A_HARNESS done: $(grep -E 'finished|wall clock' ~/work/bench/tater2/qwen-$A_HARNESS-$LABEL/result.txt | tr '\n' ' ')"
wait "$PB"; echo "== $(date '+%T') $B_HARNESS done: $(grep -E 'finished|wall clock' ~/work/bench/tater2/qwen-$B_HARNESS-$LABEL/result.txt | tr '\n' ' ')"
echo "== $(date '+%T') pair done"
