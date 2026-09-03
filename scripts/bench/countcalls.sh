#!/usr/bin/env bash
# Count the model calls and the tokens in one slice of a llama-server log.
#
# The daemons on this machine are started without --metrics, so /metrics answers
# "501 not supported" and must not be restarted to add it. The same numbers are
# in the daemon's own log: one "launch_slot_" line per call, a "prompt eval
# time = ... / N tokens" line with the prompt tokens it had to read, and an
# "eval time = ... / N tokens" line with the tokens it generated. The prompt
# tokens read are the ones the daemon could not take from its cache, so the
# average per call and the largest single prefill say how much of its context
# a harness makes the model read again on every call.
#
# Note the log's line count before the run, then afterwards:
#   countcalls.sh ~/llm/logs/19093.log <lines before>
#
# Every harness is counted with this same script.
set -u
LOG="$1"
BEFORE="${2:-0}"
SLICE=$(mktemp)
trap 'rm -f "$SLICE"' EXIT
tail -n "+$((BEFORE + 1))" "$LOG" > "$SLICE"

calls=$(grep -c 'launch_slot_' "$SLICE" || true)
prompt=$(grep -oP 'prompt eval time =\s+[\d.]+ ms /\s+\K\d+' "$SLICE" |
  awk '{ total += $1 } END { print total + 0 }')
generated=$(grep -oP '\|\s+eval time =\s+[\d.]+ ms /\s+\K\d+' "$SLICE" |
  awk '{ total += $1 } END { print total + 0 }')
context=$(grep -oP 'stop processing: n_tokens = \K\d+' "$SLICE" |
  awk 'BEGIN { most = 0 } { if ($1 > most) most = $1 } END { print most }')
largest=$(grep -oP 'prompt eval time =\s+[\d.]+ ms /\s+\K\d+' "$SLICE" |
  awk 'BEGIN { most = 0 } { if ($1 > most) most = $1 } END { print most }')
average=$(awk -v prompt="$prompt" -v calls="$calls" \
  'BEGIN { if (calls > 0) printf "%d", prompt / calls + 0.5; else print 0 }')

# The full prompt a call was sent is its end-of-call context ("stop processing:
# n_tokens") minus what that call generated. Summed over the calls this is the
# total input tokens, cached and uncached both, which is the number an API would
# bill as input if it re-sent the whole prompt every call with no caching. The
# generated tokens are the total output. The same reconstruction is used for
# every harness, since all four speak to this one daemon.
ntokens_sum=$(grep -oP 'stop processing: n_tokens = \K\d+' "$SLICE" |
  awk '{ total += $1 } END { print total + 0 }')
total_input=$(awk -v n="$ntokens_sum" -v g="$generated" 'BEGIN { print n - g }')

printf 'model calls: %s\nprompt tokens read: %s\nprompt tokens read per call on average: %s\nlargest single prefill: %s\ngenerated tokens: %s\ntotal input tokens: %s\ntotal output tokens: %s\nlargest context: %s\nlines now: %s\n' \
  "$calls" "$prompt" "$average" "$largest" "$generated" "$total_input" "$generated" "$context" "$(wc -l < "$LOG")"
