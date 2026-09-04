# The Tater Benchmark: Nerd Genie against opencode, Hermes and OpenClaw

One task, four coding agents, two models. Every agent gets the same words, starts
from nothing, and is judged by the same checker. This file is the report. The
plan and its caveats are in `TEST.md`; the run folders are under
`~/work/bench/tater2/`.

**The task**, word for word from `~/work/bench/canonical/task.txt`: build a
tic-tac-toe game for the browser with a pure-function game module, a page with a
3 by 3 grid, a status line and a New game button, plus a test file for Node's
built-in test runner, run the tests, and stop the moment they pass.

## Opus 4.8

The Opus 4.8 comparison lives in `OPUS_BENCHMARK.md` at the root: nine clean
runs, three per harness, from blank folders and blank homes, one at a time,
effort medium wherever the knob exists, no cap. Every earlier Opus number was
withdrawn when those runs were made.

## Qwen 3.8 (local, thinking off)

The Qwen comparison lives in `QWEN_BENCHMARK.md` at the root: two rounds,
all four harnesses on shipped defaults, no cap, blank homes, one harness per
card, the daemon log as the token referee. Nerd Genie was green in both rounds;
OpenClaw once; Hermes and opencode never fully.

## What was pinned so the comparison is fair

| Thing | Opus 4.8 phase | Qwen 3.8 phase |
|---|---|---|
| Model | `claude-opus-4-8` on the user's Claude subscription, no API key, each harness through its own supported path (the first row of the Opus table says which) | Qwen 3.8 27B, TurboQuant llama-server, one daemon per card, byte-identical settings, thinking off at the daemon |
| Thinking setting | each program's own default; Nerd Genie gains a `think` setting and a `/think` command for pinning it | off at the daemon for everyone |
| State | fresh home and fresh work folder per run, no memory, no past sessions, no instruction files, for every harness | same |
| Task | the same bytes, checked by hash | same |
| Cap | none of any kind; Nerd Genie's own task budget raised so it cannot act as one | same |
| Counting | each harness's own report, and for OpenClaw the Claude Code session it left behind | the daemon log, `scripts/bench/countcalls.sh`, the same for all four |
| Judge | `scripts/bench/check-tater.mjs`: six logic checks, four play-through checks in a headless browser, tests written and passing | same |

## How to read the numbers

- **Model calls** is how many times the harness asked the model; **tool calls** is how many tools it ran. A harness that batches several tool calls into one reply spends fewer model calls.
- **Tokens in** is what the model read across the run, split into the part it could not take from cache and the part it could. **Tokens out** is what it wrote.
- **Cost** is at Anthropic list rates: Claude Code's own per-call figure where it reports one, otherwise computed from the tokens.
- **Model time** is the sum of the model's own call durations; **harness time** is everything else: tool runs, its own bookkeeping, process start-up.
- **Quality** is the checker's verdict, never the harness's own claim.
