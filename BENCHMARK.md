# The Tater Benchmark: Coeus against opencode, Hermes and OpenClaw

One task, four coding agents, two models. Every agent gets the same words, starts
from nothing, and is judged by the same checker. This file is the report. The
plan and its caveats are in `TEST.md`; the run folders are under
`~/work/bench/tater2/`.

**The task**, word for word from `~/work/bench/canonical/task.txt`: build a
tic-tac-toe game for the browser with a pure-function game module, a page with a
3 by 3 grid, a status line and a New game button, plus a test file for Node's
built-in test runner, run the tests, and stop the moment they pass.

## Opus 4.8

Each harness reaches Opus 4.8 the way its own users do on a Claude subscription,
with no API key. Two of the four can; the other two are blocked, and the table
says by what. Both runs below started at 13:12 on 2026-09-03 from fresh folders
and fresh homes, at the same time, with no cap of any kind.

| | Coeus | OpenClaw | Hermes | opencode |
|---|---|---|---|---|
| How it reaches Opus 4.8 | its own provider runs `claude -p` as a bare model; Coeus's tools do the work | its `claude-cli` mode runs Claude Code with Claude Code's own tools; Claude Code does the work | its Anthropic route uses the Claude login; Anthropic answered **Out of credits** | its Claude Pro/Max login, listed in its docs with the note that Anthropic prohibits it; not yet logged in on this machine |
| Finished green | **yes** | **yes** | not run | not run |
| Wall clock | 2 min 55 s | 27 s | | |
| Model calls | 9 | 7 | | |
| Tool calls | 25 | 5 | | |
| Tokens in | 82,748 | 298,994 | | |
| of which read from cache | 18,800 | 244,444 | | |
| Tokens out | 11,258 | 9,602 | | |
| Cost at list rates | $0.97 | $0.70 | | |
| Logic checks | 6/6 | 6/6 | | |
| Play-through checks | 4/4 | 4/4 | | |
| Tests written / passing | 6 / 6 | 6 / 6 | | |
| `game.js` size | 27 lines | 27 lines | | |
| Effort setting | the program's default | the program's default | | |

How to read the two that ran:

- **Coeus** drove the model itself: nine calls, its tools ran the writes and the
  test command, and its record kept the context small (the last call read under
  3,000 tokens). About a quarter of what it sent came back from cache. The cost
  is Claude Code's own per-call figure, summed.
- **OpenClaw** handed the whole task to Claude Code and got the finished folder
  back 27 seconds later. Its own report shows only the last turn, so the token
  numbers come from the Claude Code session it left behind: seven model calls,
  five tool uses (four writes, one shell command), and nearly all of the input
  was Claude Code's own prompt read from cache. The cost is computed from those
  tokens at list rates ($5 in, $6.25 cache write, $0.50 cache read, $25 out per
  million). This measures Claude Code with OpenClaw around it, which is what
  OpenClaw's subscription mode is.
- Both produced a correct game with the same size of logic file and the same six
  tests.

## Qwen 3.8 (local, thinking off), two at a time

_Running now, one harness at a time on card A._

## What was pinned so the comparison is fair

| Thing | Opus 4.8 phase | Qwen 3.8 phase |
|---|---|---|
| Model | `claude-opus-4-8` through `claude -p` on the user's subscription, one call per model call, effort `medium`, program tools off, no MCP, no session saved, safe mode, scratch folder per call | Qwen 3.8 27B, TurboQuant llama-server, one daemon per card, byte-identical settings, thinking off at the daemon |
| Access path | one local bridge per harness (`scripts/bench/bridge/`), same flags for all four, Coeus included | each harness's own OpenAI-style HTTP to the daemon |
| Tool calls | one text form for all four, one parser | each harness's own |
| State | fresh home and fresh work folder per run, no memory, no past sessions, no instruction files | same |
| Task | same bytes, checked by hash | same |
| Cap | none, of any kind; Coeus's own task budget raised so it cannot act as one | same |
| Counting | the bridge log: uncached input, cache write, cache read, output, cost, latency per call | the daemon log: `countcalls.sh` |
| Judge | `scripts/bench/check-tater.mjs`: six logic checks, four play-through checks in a headless browser, tests written and passing | same |

## How to read the numbers

- **Model calls** is how many times the harness asked the model; **tool calls** is how many tools it ran. A harness that batches several tool calls into one reply spends fewer model calls.
- **Tokens in** is what the model read across the run, split into the part it could not take from cache and the part it could. **Tokens out** is what it wrote.
- **Cost** is the dollar figure Claude Code itself computed per call at list rates, summed. On the `claude -p` path only the system prompt caches, so this is close to full price for every harness alike.
- **Model time** is the sum of the model's own call durations; **harness time** is everything else: tool runs, its own bookkeeping, process start-up.
- **Quality** is the checker's verdict, never the harness's own claim.
