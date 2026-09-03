# The Tater Benchmark: Coeus against opencode, Hermes and OpenClaw

One task, four coding agents, two models. Every agent gets the same words, starts
from nothing, and is judged by the same checker. This file is the report. The
plan and its caveats are in `TEST.md`; the run folders are under
`~/work/bench/tater2/`.

**The task**, word for word from `~/work/bench/canonical/task.txt`: build a
tic-tac-toe game for the browser with a pure-function game module, a page with a
3 by 3 grid, a status line and a New game button, plus a test file for Node's
built-in test runner, run the tests, and stop the moment they pass.

## Opus 4.8: Coeus, OpenClaw, Hermes

Each harness reaches Opus 4.8 the way its own users do on a Claude subscription,
with no API key, and all three end up running the same program: `claude -p`,
which is Claude Code in one-shot mode. The difference is what the harness does
around it. Coeus runs it as a bare model with Claude Code's tools switched off
and does the work with its own tools. OpenClaw and Hermes hand it the whole job
and let Claude Code's own tools write the files and run the tests. opencode has
no subscription route and was left out of this phase.

All three runs were on 2026-09-03 between 13:12 and 13:35, from fresh folders
and fresh homes, on the same task text, with no cap of any kind.

| | Coeus | OpenClaw | Hermes |
|---|---|---|---|
| How it reaches Opus 4.8 | its own provider runs `claude -p` as a bare model; Coeus's tools do the work | its `claude-cli` mode runs Claude Code with Claude Code's own tools | its Claude Code skill: Hermes, on the local Qwen, hands the task to `claude -p` and waits |
| Finished green | **yes** | **yes** | **yes** |
| Wall clock | 2 min 55 s | 27 s | 1 min 57 s |
| Opus model calls | 9 | 7 | 7, plus 6 Qwen calls by Hermes itself |
| Tool calls | 25 by Coeus | 5 by Claude Code | 5 by Claude Code, plus Hermes's own terminal calls |
| Tokens in, total | 82,748 | 298,994 | 120,141 |
| of which read from cache, $0.50 per million | 18,800 | 244,444 | 85,356 |
| of which written to cache or fresh, $5 to $10 per million | 63,948 | 54,550 | 34,785 |
| Tokens out, $25 per million | 11,258 | 9,602 | 9,959 |
| **Cost at Anthropic list prices** | **$0.97** | **$0.91** | **$0.64** (plus free local Qwen) |
| Logic checks | 6/6 | 6/6 | 6/6 |
| Play-through checks | 4/4 | 4/4 | 4/4 |
| Tests written / passing | 6 / 6 | 6 / 6 | 6 / 6 |
| `game.js` size | 27 lines | 27 lines | 26 lines |
| Thinking setting | the program's default | the program's default | the program's default |

What each row means:

- **Coeus** drove the model itself, nine calls, and its own tools ran the writes
  and the test command. Its record kept the context small: the last call read
  under 3,000 tokens. The first call alone cost $0.22 because the model wrote all
  three files in one reply (6,753 tokens out). The cost is Claude Code's own
  per-call figure, summed.
- **OpenClaw** handed the task to Claude Code through its claude-cli mode and had
  the finished folder 27 seconds later. Its own report shows only the last turn,
  so the tokens come from the Claude Code session left behind: seven calls, four
  writes and one shell command, and a 44,000-token Claude Code prompt (its own
  system prompt plus OpenClaw's tools) read from cache each call.
- **Hermes** did what its Claude Code skill page says: Hermes itself ran on the
  local Qwen (six calls, free), checked that `claude` was there, ran `claude -p`
  with the task word for word and the flags the skill documents, and waited. The
  Claude Code side was seven calls, four writes and one shell command, with a
  smaller prompt than OpenClaw's because no extra tools were attached. Hermes's
  own prompt carried one extra sentence telling it to use Claude Code, which is
  recorded in `~/work/bench/opus2/hermes/prompt.txt`.
- All three produced a correct, playable game with the same six tests.

**Why tokens in is not the cost.** Anthropic has four prices: fresh input $5 per
million, writing the prompt into the cache $10 per million (Claude Code uses the
one-hour cache), reading it back $0.50 per million, and output $25 per million.
OpenClaw's 299,000 tokens were mostly cheap cache reads of the same prompt;
Coeus's 83,000 were mostly a fresh, growing conversation written to cache at the
dear price. Output is the biggest single line for all three.

**What this says about the harnesses.** On a strong model and a small task, all
three finish, and Claude Code's own loop is the fastest way through it. Coeus
is the only one of the three that is actually driving the model: it decides
every step, runs every tool through its own permission function and log, and
keeps a record that a person can read. The other two, in their subscription
modes, are wrappers around Claude Code; their harness logic is not what is being
measured on this path. That is the honest comparison this run can give. The
Qwen phase below, where all four harnesses drive the same local model directly,
is the one that compares the harnesses themselves.

## Qwen 3.8 (local, thinking off), two at a time

_Running now, one harness at a time on card A._

## What was pinned so the comparison is fair

| Thing | Opus 4.8 phase | Qwen 3.8 phase |
|---|---|---|
| Model | `claude-opus-4-8` on the user's Claude subscription, no API key, each harness through its own supported path (the first row of the Opus table says which) | Qwen 3.8 27B, TurboQuant llama-server, one daemon per card, byte-identical settings, thinking off at the daemon |
| Thinking setting | each program's own default; Coeus gains a `think` setting and a `/think` command for pinning it | off at the daemon for everyone |
| State | fresh home and fresh work folder per run, no memory, no past sessions, no instruction files, for every harness | same |
| Task | the same bytes, checked by hash | same |
| Cap | none of any kind; Coeus's own task budget raised so it cannot act as one | same |
| Counting | each harness's own report, and for OpenClaw the Claude Code session it left behind | the daemon log, `scripts/bench/countcalls.sh`, the same for all four |
| Judge | `scripts/bench/check-tater.mjs`: six logic checks, four play-through checks in a headless browser, tests written and passing | same |

## How to read the numbers

- **Model calls** is how many times the harness asked the model; **tool calls** is how many tools it ran. A harness that batches several tool calls into one reply spends fewer model calls.
- **Tokens in** is what the model read across the run, split into the part it could not take from cache and the part it could. **Tokens out** is what it wrote.
- **Cost** is at Anthropic list rates: Claude Code's own per-call figure where it reports one, otherwise computed from the tokens.
- **Model time** is the sum of the model's own call durations; **harness time** is everything else: tool runs, its own bookkeeping, process start-up.
- **Quality** is the checker's verdict, never the harness's own claim.
