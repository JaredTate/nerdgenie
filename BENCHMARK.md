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

**Correction, 14:05.** An earlier version of this table overcounted OpenClaw and
Hermes by about two and a half times: Claude Code writes one line per piece of a
reply into its session file, and a reply with four tool calls was counted as
four model calls. Every Claude Code session was recounted by unique model call.
The numbers below are the corrected ones, and OpenClaw was run a second time to
confirm them.

Each harness reaches Opus 4.8 the way its own users do on a Claude subscription,
with no API key, and all three end up running the same program, `claude -p`,
which is Claude Code in one-shot mode. The difference is what the harness does
around it:

- **Coeus** runs `claude -p` with Claude Code's tools switched off and its own
  system prompt in place of Claude Code's. Claude Code is then a bare model. The
  model's reply asks Coeus for tools in text; Coeus runs them, updates its
  record, and calls again. Coeus's loop is what is being measured.
- **OpenClaw** (claude-cli mode) sends the task to Claude Code once, with Claude
  Code's own tools on. Claude Code's loop writes the files and runs the tests
  and hands back the finished folder. OpenClaw's log shows exactly one turn.
- **Hermes** (its Claude Code skill) runs on the local Qwen, types the command
  `claude -p ... --model claude-opus-4-8 --dangerously-skip-permissions` into
  its terminal tool, and waits. Same as OpenClaw: Claude Code's loop does the
  work.

The proof is in the Claude Code sessions each run left behind: the tools that
ran were Claude Code's own `Write` and `Bash`, not OpenClaw's `write` and
`exec` or Hermes's `write_file`. So on this path OpenClaw and Hermes measure
Claude Code's loop with a wrapper around it, and Coeus measures Coeus's loop.
opencode has no subscription route and was left out of this phase.

All runs on 2026-09-03, fresh folders and homes, the same task text, no cap.

| | Coeus | OpenClaw, run 1 | OpenClaw, run 2 | Hermes |
|---|---|---|---|---|
| Whose loop drove Opus | Coeus | Claude Code | Claude Code | Claude Code (Hermes on Qwen typed the command) |
| Finished green | **yes** | **yes** | **yes** | **yes** |
| Wall clock | 2 min 55 s | 27 s | 25 s | 1 min 57 s |
| Opus model calls | 9 | 3 | 3 | 3 (plus 6 free Qwen calls by Hermes) |
| Tool calls | 25 | 5 | 4 | 5 |
| Tokens in, total | 82,748 | 130,846 | 130,482 | 54,273 |
| of which read from cache, $0.50 per million | 18,800 | 117,916 | 117,745 | 45,240 |
| of which written to cache, $10 per million | 63,948 | 12,924 | 12,731 | 9,027 |
| Tokens out, $25 per million | 11,258 | 2,062 | 1,898 | 2,135 |
| **Cost at Anthropic list prices** | **$0.97** | **$0.24** | **$0.23** | **$0.17** |
| Logic checks | 6/6 | 6/6 | 6/6 | 6/6 |
| Play-through checks | 4/4 | 4/4 | 4/4 | 4/4 |
| Tests written / passing | 6 / 6 | 6 / 6 | 6 / 6 | 6 / 6 |
| `game.js` size | 27 lines | 27 lines | 27 lines | 26 lines |
| Thinking setting | the program's default | the program's default | the program's default | the program's default |

Cost arithmetic, so the columns can be checked: OpenClaw run 1 is
12,924 × $10 + 117,916 × $0.50 + 2,062 × $25, all per million, = $0.13 +
$0.06 + $0.05 = $0.24. Coeus is Claude Code's own per-call figure, summed; its
first call alone was $0.22 (4,916 tokens written to cache, 6,753 out).

### What this says about Coeus, honestly

On this task, with this model, Claude Code's loop did the job in three calls
and about a quarter dollar. Coeus's loop took nine calls and about a dollar,
four times the cost and five times the time, for the same green result. Three
reasons, all visible in the numbers, and all fixable:

1. **Coeus's prompt is almost never cached.** Five of its nine calls read zero
   tokens from cache, and the other four only 4,700. Every call is written to
   the cache at $10 per million and then thrown away. The cause is in how the
   provider runs the program: each call runs in a brand-new scratch folder, and
   Claude Code adds the working folder to its system prompt, so no two calls
   share a prefix. A fixed scratch folder per model, plus the program's
   `--exclude-dynamic-system-prompt-sections` flag, would let the persona,
   rules and tool list be read from cache at a twentieth of the price.
2. **Coeus writes five times the output.** 11,258 tokens out against about
   2,000. The model rewrites the record through the `task` tool (the why, the
   done list, the plan), and the done-check and the after-action review each
   cost a call and a reply. Output is the dearest token there is.
3. **Coeus makes three times the calls.** Calls six to nine happened after the
   tests were already green: the done-check, a refused record change, and the
   review. Claude Code stopped when the tests passed.

None of this touches what Coeus is for. Claude Code's loop keeps no record a
person can read, has no done list with proof, no stop list, no corrections
kept word for word, and grows its prompt every call. On a forty-round task on a
small model that is where it falls over, and that is what the Qwen phase below
is for. But on a three-call task on a strong model, the record is overhead, and
these numbers say how much.

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
