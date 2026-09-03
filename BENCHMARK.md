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
with no API key, and all three end up running the same program, `claude -p`,
which is Claude Code in one-shot mode. The difference is what the harness does
around it:

- **Coeus** runs `claude -p` with Claude Code's tools switched off and its own
  system prompt in place of Claude Code's. Claude Code is then a bare model. The
  model's reply asks Coeus for tools in text; Coeus runs them, updates its
  record, and calls again. Coeus's loop is what is measured.
- **OpenClaw** (claude-cli mode) sends the task to Claude Code once, with Claude
  Code's own tools on. Claude Code's loop writes the files and runs the tests
  and hands back the finished folder. OpenClaw's log shows exactly one turn.
- **Hermes** (its Claude Code skill) runs on the local Qwen, types the command
  `claude -p ... --model claude-opus-4-8 --effort medium
  --dangerously-skip-permissions` into its terminal tool, and waits. Claude
  Code's loop does the work, as with OpenClaw.

The proof is in the Claude Code session each run left behind: the tools that ran
were Claude Code's own `Write` and `Bash`, not OpenClaw's `write` and `exec`
or Hermes's `write_file`. So on this path OpenClaw and Hermes measure Claude
Code's loop with a wrapper around it, and Coeus measures Coeus's loop. opencode
has no subscription route and was left out of this phase.

### The equal runs: effort medium everywhere

Effort is pinned to `medium` on every path that has the knob (Coeus through its
new `think` setting, Hermes through the flag it types). OpenClaw's claude-cli
path has no effort knob; its Claude Code ran at the program's default and did
no thinking (10 to 20 thinking tokens in the whole run), so it is already at
the floor. Every Claude Code session was counted by unique model call. Fresh
folders and homes, the same task text, no cap. 2026-09-03, 13:12 to 13:53.

| | Coeus | OpenClaw, run 1 | OpenClaw, run 2 | Hermes |
|---|---|---|---|---|
| Whose loop drove Opus | Coeus | Claude Code | Claude Code | Claude Code (Hermes on Qwen typed the command) |
| Finished green | **yes** | **yes** | **yes** | **yes** |
| Time from task in to answer out | **34 s** | 27 s | 25 s | 110 s, of which Hermes's own five Qwen calls are most |
| Opus model calls | 3 | 3 | 3 | 3 |
| Tool calls | 9 | 5 | 4 | 4 |
| Tokens in, total | 24,803 | 130,846 | 130,482 | 54,024 |
| of which read from cache, $0.50 per million | 0 | 117,916 | 117,745 | 45,120 |
| of which written to cache, $10 per million | 24,803 | 12,924 | 12,731 | 8,898 |
| Tokens out, $25 per million | 2,886 | 2,062 | 1,898 | 2,047 |
| of which thinking | about 0 | 19 | 10 | 19 |
| **Cost at Anthropic list prices** | **$0.33** | **$0.24** | **$0.23** | **$0.16** |
| Logic checks | 6/6 | 6/6 | 6/6 | 6/6 |
| Play-through checks | 4/4 | 4/4 | 4/4 | 4/4 |
| Tests written / passing | 6 / 6 | 6 / 6 | 6 / 6 | 6 / 6 |
| `game.js` size | 28 lines | 27 lines | 27 lines | 27 lines |

Time is measured the same way for all: from the moment the task is handed over
to the moment the final answer comes back. For Coeus that is the driver's log
(first message sent 13:51:34, final reply 13:52:08); the driver then waits a
fixed quiet period before it leaves, which is why the raw wall clock says 75 s
and is not the harness's time. For OpenClaw and Hermes it is launch to exit.

Cost arithmetic, so every column can be checked: Coeus is 24,803 × $10 +
2,886 × $25, per million, = $0.25 + $0.07 = $0.32 (Claude Code's own per-call
figures sum to $0.33; the difference is a tiny Haiku side call the program
makes on its own). OpenClaw run 1 is 12,924 × $10 + 117,916 × $0.50 + 2,062 ×
$25 = $0.13 + $0.06 + $0.05 = $0.24. The same four prices for everyone.

**Reading it.** At equal effort, on this task, all three loops do it in three
model calls, and Coeus is within ten seconds of Claude Code's own loop while
driving the model itself, with its record, done list, and permission function
in the path. It costs about a third more, and the whole of that difference is
one thing: **Coeus's prompt is never read from cache** (0 tokens, against
118,000 for OpenClaw). Every call pays the $10 write price for the persona,
rules, tool list, and record. The cause is in the provider: each call runs in a
brand-new scratch folder, and Claude Code puts the folder in its system prompt,
so no two calls share a prefix. A fixed folder per model plus the program's
`--exclude-dynamic-system-prompt-sections` flag would let the unchanging part
be read back at a twentieth of the price, which on these numbers puts Coeus at
about $0.24, level with OpenClaw. The other difference is nine tool calls
against five: Coeus's model writes the record (why, done list, plan) through
the `task` tool, which is the point of Coeus and costs a few hundred tokens.

### The first runs: the program's default effort, and why they misled

The first Coeus run, before the `think` setting existed, took 2 min 43 s, nine
model calls, 82,748 tokens in, 11,258 out, $0.97. OpenClaw and Hermes were
unchanged (their Claude Code does not think at the default). The difference
was thinking: on Coeus's prompt the model chose to think about 5,000 tokens
before its first reply, and kept thinking on every call; on Claude Code's own
prompt it thought 20 tokens. Same program, same default setting, different
prompt, different behaviour. That is why effort must be pinned, and the equal
runs above are the ones to read.

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
