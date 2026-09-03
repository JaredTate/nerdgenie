# The Tater Benchmark — what we tested, how, and what it means

This document explains, in plain words, the test we ran to see how Coeus stacks
up against three other coding agents. It is written so anyone can read it, follow
it, and judge whether the comparison is fair. Where a number here could mislead,
the document says so out loud.

## The question

Coeus is built around one idea: instead of re-reading its whole conversation
every turn, it keeps a short written record shaped like an Army operations order
and works from that. The claim is that this keeps the model's context small and
steady, so the agent does not spiral, and the cost per finished task stays
bounded.

The Tater benchmark is meant to test that claim honestly, against the agents a
person might otherwise use:

- **opencode** — the harness we liked best before Coeus. No task record; it
  re-feeds the growing conversation each turn.
- **Hermes** — a harness with loop guards and a turn limit, also no operations-
  order record.
- **OpenClaw** — a harness with a code-mode for local models.
- **Coeus** — ours.

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
- **A fresh, empty start.** Every run gets a new empty folder and, for Coeus, a
  brand-new home with an empty record and empty memory. Nothing carries over
  between runs, so no agent can read another's earlier work.
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

To show the same harness on a strong model versus a weak one, Coeus was also run
once on Opus 4.8 through `claude -p` on the user's subscription (no API key, no
graphics card, a fresh isolated home). This measures how many fewer rounds a
strong model needs for the identical task. The other three harnesses are wired to
the local endpoint and cannot reach Opus headless without an API key, so for them
the Opus column is the priced projection only, said plainly — not a real run.

## How a run is driven, per harness

- **Coeus:** `coeus serve` on a fresh home, its socket driven with the task, then
  stopped by the exact process id recorded at launch.
- **opencode:** its own `run` command, task read from the file, pointed at the
  local daemon.
- **Hermes:** `hermes chat -q` with the task, reasoning low, pointed at the local
  daemon (it stops at its own built-in 250-turn limit).
- **OpenClaw:** its headless `agent exec` in the local-model code-mode its docs
  prescribe.

## What the results are for

The results table (in `docs/TATER_BENCHMARK.md`) shows, per harness: did it
finish, how long it took, how many calls, its processed and sent tokens, its
with-caching cost projection, and its quality scores. Read it with the two
caveats above in mind: use the with-caching cost, and remember a single run does
not capture a harness's variance — opencode, for one, finished the same task in
98 seconds on one run and never finished on another, which is itself a finding.

## Fairness limits, stated openly

- **A safety timeout was added by the orchestrator, not requested by the user.**
  It was applied so a non-converging run could not tie up the machine forever. A
  run stopped by it is recorded as "did not finish," not as a failure at that
  exact minute. Hermes instead stopped at its own 250-turn limit; that is the
  harness's own setting, not the timeout.
- **One run is not a reliable average.** Where a harness was run more than once,
  both runs are reported and the spread is treated as the point.
- **Only Coeus was run on Opus for real.** The rest are projections.

## Where the pieces live

- The task: `~/work/bench/canonical/task.txt`
- The checker and counting scripts: `scripts/bench/`
- The full results, tables, screenshots, and verdict: `docs/TATER_BENCHMARK.md`
- The raw per-run logs: `~/work/bench/tater/`
