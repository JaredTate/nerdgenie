# The Tater Benchmark — what we tested, how, and what it means

This document explains, in plain words, the test we ran to see how Nerd Genie stacks
up against three other coding agents. It is written so anyone can read it, follow
it, and judge whether the comparison is fair. Where a number here could mislead,
the document says so out loud.

## The question

Nerd Genie is built around one idea: instead of re-reading its whole conversation
every turn, it keeps a short written record shaped like an Army operations order
and works from that. The claim is that this keeps the model's context small and
steady, so the agent does not spiral, and the cost per finished task stays
bounded.

The Tater benchmark is meant to test that claim honestly, against the agents a
person might otherwise use:

- **opencode** — the harness we liked best before Nerd Genie. No task record; it
  re-feeds the growing conversation each turn.
- **Hermes** — a harness with loop guards and a turn limit, also no operations-
  order record.
- **OpenClaw** — a harness with a code-mode for local models.
- **Nerd Genie** — ours.

## The task

Every agent is given the exact same job, word for word, from a single file
(`~/work/bench/canonical/task.txt`, checksum begins `c8ed58f2`): build a
tic-tac-toe game that runs in a web browser, with a small pure-function game
module, a page with a 3×3 grid, a status line, and a New game button, plus a test
file for Node's built-in test runner. The task text begins by telling the model
to be quick, do only what is asked, and stop as soon as the tests pass. The same
line goes to all four; none gets a different prompt.

Tic-tac-toe was chosen on purpose: small enough to finish in a couple of minutes,
but with real edge cases (a draw, an illegal move, no move after a win) that a
weak model can get stuck on.

## What is held identical, and what is measured

A harness comparison is only fair if the inputs and the judge are the same and
only the harness differs. So four things are held constant, and the rest is the
result we read off:

**Held identical for every run**

- **The model.** The local Qwen 3.8 27B, served by the TurboQuant llama-server.
- **The card settings.** Both graphics cards (two identical Radeon 7900 XTX) run
  the byte-identical daemon: 262,144-token context, temperature 0.7, speculative
  decoding on, and **thinking turned off at the daemon** (`--reasoning-budget 0`,
  `enable_thinking: false`). Thinking-off is enforced below the harness, where no
  harness can change it — confirmed by zero `<think>` blocks in either card's
  log. When two harnesses run at once, each is alone on its own card.
- **A fresh, empty start, for every harness.** Every run gets a new empty work
  folder and a brand-new home folder for the harness: a fresh `COEUS_HOME` for
  Nerd Genie, fresh `XDG_*` folders and config for opencode, a fresh `HERMES_HOME`
  outside `~/.hermes` for Hermes, and a fresh `OPENCLAW_HOME` and config for
  OpenClaw. No memory files, no past sessions, no skills learned earlier, no
  instruction files in or above the work folder. The first Qwen runs did not do
  this for the three other harnesses (Hermes, for one, ran with the user's own
  memory, skills, past sessions and persona), so those numbers are indicative
  only and every harness is rerun.
- **The thinking setting.** On Opus every harness runs at effort `medium`. On
  Qwen thinking is off at the daemon for everyone (above). Nerd Genie has a `/think`
  command and a `think` setting per model for this.
- **The judge.** Our own checker (`scripts/bench/check-tater.mjs`) decides pass
  or fail — never the agent's own tests. It checks the game logic six ways (a win
  in a row, a column, a diagonal, a draw, an illegal move on a taken cell, and no
  move after a win), then loads the page in a headless browser and actually plays
  it (nine cells present, a real winning sequence is announced, New game resets,
  a taken cell does nothing).

**Measured as the result**

- **Speed:** wall-clock time from launch to exit, and the number of model calls.
- **Quality:** the six logic checks, the four play-through checks, how many tests
  the harness wrote, how many of its own tests pass, and the size of its code.
- **Tokens:** see the honest note below — this one needs care.

The steps each harness takes are **not** forced. That is the whole point: we are
measuring how efficiently each harness drives the same model, so the calls,
tokens, and time are the output, not something scripted.

## The honest note about tokens and cost

There are two different "input token" numbers, and mixing them up flatters or
slanders a harness. This document keeps them separate:

- **Processed tokens** — what the model actually computed each call, the part it
  could *not* take from its cache. This is the real work the graphics card did.
- **Sent tokens** — the full prompt handed over each call, cache hits included.
  This is the API-billing lens, and for a harness that re-reads a long history it
  is mostly cache hits.

An early draft of the comparison quoted the **sent** total for opencode (tens of
millions of tokens) and turned it into a dollar figure as if all of it were paid
at full price. That is misleading, because prompt caching is exactly what makes
those re-reads cheap. The fair cost figure is the **with-caching** column: the
uncached part at full price, the cached part at the cache-read rate. Any
API-cost projection in the results uses real Anthropic Opus 4.8 rates (input
$5.00, output $25.00, cache read $0.50 per million tokens, as of 2026-09-03) and
is labelled a projection, because the runs themselves were on the free local
model.

## The Opus 4.8 comparison

The same four harnesses are also run on Opus 4.8, and this time all four are
real runs, not projections. There are no API keys on this machine and none are
wanted, so Opus is reached the way a person on a Claude subscription reaches it:
through the `claude` program, one `claude -p` call per model call. Nerd Genie has a
provider that does exactly this. The other three harnesses only speak the
OpenAI-style HTTP API, so a small local program called the bridge
(`scripts/bench/bridge/`) accepts their HTTP request and makes one `claude -p`
call with the identical command line. Every harness, Nerd Genie included, goes
through the bridge for the main run, so the model, its settings, the way tools
are described, and the way tokens are counted are the same for all four. One
extra Nerd Genie run uses its own provider directly, as a cross-check that the bridge
changes nothing.

What is pinned on every call, and verified with real calls before the runs:

- The model is `claude-opus-4-8`, named on the command line, never a default.
- Effort is `medium` for every harness (`--effort medium`).
- The program's own tools are off, no MCP servers, no session saved, safe mode
  so the user's own Claude Code settings are ignored, and each call runs in a
  scratch folder holding nothing but that call's system prompt, so no CLAUDE.md,
  memory, or project settings can reach the prompt.
- Tool calls are written in one text form (`<tool_call>{...}</tool_call>`),
  because `claude -p` has no tool interface; the bridge and Nerd Genie use the same
  form and the same parser.
- The program sometimes makes a tiny Haiku side call of its own (about a tenth of
  a cent). It is logged separately and left out of the Opus token columns.

One honest limit of this path: only the system prompt gets cached between
calls. The growing conversation is one block of text and never matches the
cache, so every harness pays full price for what it re-sends. That is equal for
all four, and it is harsher on a harness that re-sends a long transcript than a
real API with caching would be. Cache reads and writes are logged per call so
the difference can be seen.

## How a run is driven, per harness

One script, `scripts/bench/tater_run.sh <phase> <harness> <label> [card]`,
drives every run in either phase (`opus` or `qwen`) from a fresh state, records
the exact command and environment, and never kills anything by name. In the Opus
phase all four harnesses run at the same time, each on its own bridge port with
its own log. In the Qwen phase two run at a time, one per graphics card, and the
daemon is restarted before every run so its prompt cache is cold.

- **Nerd Genie:** `coeus serve` on a fresh home, its socket driven with the task by
  `scripts/bench/drive.py`, stopped by the exact process id recorded at launch.
- **opencode:** `opencode run` in the work folder, JSON output kept.
- **Hermes:** `hermes chat -q` with the task, its own default limits.
- **OpenClaw:** its headless `agent exec` in its normal tool mode with the lean
  flag, JSON envelope kept. Code mode is not used: on Qwen it looped forever
  writing shell commands into a tool that only takes JavaScript.

Tokens are counted the same way for all four: on Opus from the bridge log (a
line per call with uncached input, cache writes, cache reads, output, cost and
latency, which also separates model time from harness time); on Qwen from the
daemon's own log with `scripts/bench/countcalls.sh`. Each harness's own usage
report is kept beside those numbers as a cross-check. Quality comes from
`scripts/bench/check-tater.mjs` after the harness has exited.

## What the results are for

The results table (in `docs/TATER_BENCHMARK.md`) shows, per harness: did it
finish, how long it took, how many calls, its processed and sent tokens, its
with-caching cost projection, and its quality scores. Read it with the two
caveats above in mind: use the with-caching cost, and remember a single run does
not capture a harness's variance — opencode, for one, finished the same task in
98 seconds on one run and never finished on another, which is itself a finding.

## Fairness limits, stated openly

- **There is no cap of any kind.** The first runs had a thirty-minute cap that
  the orchestrator added and the user never asked for; it is gone, and so is
  any cap on model calls. The user's rule is that no harness, and Nerd Genie least
  of all, is capped on any model. Each harness keeps whatever limits it ships
  with, because those are part of the harness; Nerd Genie's own task budget is off
  unless a user sets one, and the benchmark config sets none, so it cannot act
  as a cap either, and the result file says so.
- **One run is not a reliable average.** Where a harness was run more than once,
  both runs are reported and the spread is treated as the point.
- **All four run on Opus for real,** through the same bridge and the same flags.

## Where the results live

- `OPUS_BENCHMARK.md`: nine runs on Opus 4.8, three per harness, through each
  harness's own subscription path (Nerd Genie's loop against Claude Code's).
- `GPT_BENCHMARK.md`: GPT-5.6 Sol on the ChatGPT subscription, all four
  harnesses driving the model with their own loops.
- `QWEN_BENCHMARK.md`: the local Qwen 3.8 on the two cards, all four harnesses
  on shipped defaults, the loop-against-loop comparison.
- `BENCHMARK.md`: the index and the older first-day notes.

## Where the pieces live

- The task: `~/work/bench/canonical/task.txt`
- The checker and counting scripts: `scripts/bench/`
- The full results, tables, screenshots, and verdict: `docs/TATER_BENCHMARK.md`
- The raw per-run logs: `~/work/bench/tater/` for the first Qwen runs, and
  `~/work/bench/tater2/<phase>-<harness>-<label>/` for every run from here on
- The bridge that carries the three other harnesses to Opus: `scripts/bench/bridge/`
