#!/usr/bin/env bash
# Run Coeus once with no person at the keyboard: start "coeus serve" on a
# benchmark home folder, drive its socket with the task, then stop it.
#
#   run-coeus.sh BINARY HOME_FOLDER TASK_FILE [TIMEOUT_MINUTES]
#
# With no TIMEOUT_MINUTES, or with zero, the driver waits as long as the program
# takes: the benchmark puts no wall-clock cap on any harness.
#
# The serve's own output goes to HOME_FOLDER/serve.log and the driver's log of
# every envelope to HOME_FOLDER/drive.log. The serve is stopped by the exact
# process id recorded at launch, checked by name first, never by pattern.
set -u
BINARY="$1"
HOME_FOLDER="$2"
TASK="$3"
MINUTES="${4:-0}"
HERE="$(cd "$(dirname "$0")" && pwd)"
SOCKET="$HOME_FOLDER/run/coeus.sock"

COEUS_HOME="$HOME_FOLDER" "$BINARY" serve > "$HOME_FOLDER/serve.log" 2>&1 < /dev/null &
SERVE_PID=$!

# The serve is stopped by the exact process id recorded above, checked by name
# first, whether the driver finishes or this script is told to stop.
stop_serve() {
  if [ "$(ps -o comm= -p "$SERVE_PID" 2>/dev/null)" = "coeus" ]; then
    kill -INT "$SERVE_PID"
    wait "$SERVE_PID"
  fi
}
trap 'stop_serve; exit 143' TERM INT

for _ in $(seq 1 60); do
  [ -S "$SOCKET" ] && grep -q 'is listening' "$HOME_FOLDER/serve.log" && break
  sleep 1
done
if ! grep -q 'is listening' "$HOME_FOLDER/serve.log"; then
  echo "coeus serve did not come up within a minute; its log says:" >&2
  cat "$HOME_FOLDER/serve.log" >&2
  kill "$SERVE_PID" 2>/dev/null
  exit 1
fi

python3 "$HERE/drive.py" "$SOCKET" "$TASK" "$HOME_FOLDER/drive.log" "$MINUTES"
status=$?

stop_serve
tail -n 40 "$HOME_FOLDER/drive.log"
exit $status
