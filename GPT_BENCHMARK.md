# The GPT-5.6 Sol Benchmark: all four harnesses, each driving the model itself

One clean round on 2026-09-03: Coeus, opencode, Hermes and OpenClaw on
GPT-5.6 Sol at thinking medium, on the user's ChatGPT subscription with no API
key, on the same tic-tac-toe task as the Opus and Qwen phases. This is the
comparison the Opus phase could not give: every harness here drives the model
with its own loop and its own tools, because OpenAI lets its Codex login be
used by outside tools and Anthropic does not.

**The short version.** opencode, Hermes and OpenClaw all finished green on
GPT-5.6 Sol, each driving the model with its own loop on the subscription:
opencode in 44 seconds and 5 model calls for about nine cents, Hermes in 1 min
39 s and 7 calls for about twenty cents, OpenClaw in 66 to 71 seconds three
times over (its own report does not count its tokens, so its cost is unknown).
**Coeus could not be measured on this path**, and the reason is a defect in
Coeus, proved below: the `codex` program it runs describes its own tools to
the model inside a developer message that no switch removes, the model used
those tools inside a read-only sandbox, and Coeus's own tools were never
called. The fix, a provider that talks to OpenAI's Codex backend directly the
way the other three do, is being built; its row goes in when it lands.


## The task

The same text, byte for byte, from `~/work/bench/canonical/task.txt` (SHA-256
begins `c8ed58f2`, checked before each run): a tic-tac-toe game for the
browser in the current folder, `game.js` with three pure functions, an
`index.html` with a 3 by 3 grid, a status line and a New game button,
`tests/game.test.mjs` for Node's test runner with six named cases, run the
tests, make them pass, install nothing, stop when they pass.

## How each harness reaches GPT-5.6 Sol, and what went wrong for Coeus

| | Coeus | opencode | Hermes | OpenClaw |
|---|---|---|---|---|
| Login | the `codex` program, logged in to the subscription | opencode's own "ChatGPT Plus/Pro" login | the codex login, which Hermes's `openai-codex` provider reads | the codex login, through OpenClaw's `openai` provider |
| How the model is called | `codex exec` once per model call, its own tools switched off, Coeus's system prompt in place, tool calls in text; Coeus's tools do the work | its own loop, straight to OpenAI's Codex backend | its own loop, straight to OpenAI's Codex backend | its own embedded loop, straight to OpenAI's Codex backend |
| Whose loop | Coeus | opencode | Hermes | OpenClaw |
| Thinking | `think = "medium"` in its config, passed as codex's reasoning effort | `--variant medium` | `--reasoning medium` | `--thinking medium` |
| Command | `coeus serve` on a fresh home, driven over its socket by `scripts/bench/drive.py` | `opencode run -m openai/gpt-5.6-sol --variant medium --format json "<task>"` | `hermes chat -q "<task>" --provider openai-codex -m gpt-5.6-sol --reasoning medium --yolo --in <work>` | `openclaw agent exec --message-file task.txt --model openai/gpt-5.6-sol --thinking medium --cwd <work> --timeout 0 --json` |

Each was proved with a one-word test call before the round, and the run logs
confirm the route: Hermes and OpenClaw both have a second mode that hands a
whole turn to the codex program, the wrapper pattern of the Opus phase, and
neither used it here (OpenClaw's log shows its embedded runtime, Hermes's
session shows its own tool calls).


**Why Coeus's row is not a result.** Coeus's `codex` provider runs `codex exec`
once per model call with its shell tools switched off (`--disable shell_tool
--disable unified_exec`), Coeus's system prompt in place, and a read-only
sandbox, expecting the program to behave as a bare model and to write tool
calls in Coeus's text form. It does not. A local listener stood in for the
backend and recorded the exact request the program sends: the `tools` field is
empty, but the `input` carries a developer item of type `additional_tools`,
18,472 characters long, describing the program's own tools (its exec, its file
patcher, its planner, its agent-spawning tools). With every documented switch
off (`shell_tool`, `unified_exec`, `multi_agent`, `apps`, `web_search`,
`view_image`, `agents.enabled`, `approval_policy=never`) that item shrinks to
8,142 characters and does not go away. So the model always has the program's
tools in front of it, prefers them, tried to write the files with them inside
the read-only sandbox, was refused, looped inside the program for 146,933
tokens, and told Coeus "the workspace is read-only, so file creation was
rejected." Coeus's own `write` and `shell` tools were never asked for. The
run is recorded, but it measures the codex program, not Coeus.

The fix is not a flag. It is a provider that speaks to OpenAI's Codex backend
directly with the codex login, the route Hermes, OpenClaw and opencode use, so
that Coeus's loop and tools drive the model. That provider is being built and
this file gets Coeus's row when it is in.

## What was held equal

- **Blank start.** A new, empty work folder with a path no run had used, only
  `task.txt` and an empty git repository; a new home for the harness
  (`COEUS_HOME`; fresh `XDG` folders for opencode, holding only a copy of its
  login file; a fresh `HERMES_HOME` outside `~/.hermes`; OpenClaw's own
  throw-away state folder). No memory, no past session, no instruction file.
- **One at a time.** Coeus, opencode, Hermes, OpenClaw, in that order.
- **Thinking medium** on every one, by the flag or setting each harness has for it.
- **No cap of any kind.** Coeus's own task budget raised out of reach.
- **The same judge**, `scripts/bench/check-tater.mjs`, after the harness exited.
- **The same clock**: task in to final answer out. For Coeus that is its
  driver's log; for the others, launch to exit.
- **The same prices**, OpenAI's list for GPT-5.6 Sol on 2026-09-03: input $4,
  cached input $0.40, output $20, per million tokens. Every harness reports its
  own token counts, and the source for each is named in the notes.

## Where the token numbers come from, and one caution

Each harness counts its own tokens, from the usage the API returned to it:
Coeus from each `codex exec` result, opencode from its per-step records,
Hermes from its session database, OpenClaw from its run envelope. There is no
outside referee on this path the way the daemon log is on Qwen, so the counts
are only as honest as each harness's bookkeeping. OpenClaw's envelope reports a
run total and no per-call rows; Hermes reports a run total too. Coeus's cached
count comes from its record's per-turn line, which rounds to the hundred.
Reasoning tokens are inside the output count wherever the harness reports
them, and shown apart where it does.
## Results: every run

| Harness | Run | Green | Task in to answer out | Model calls | Tool calls | Tokens in | of which cached | Tokens out | of which reasoning | Cost | Logic | Plays | Tests | game.js |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| coeus | 1 | **NO, see below** | 1 min 25 s | 1 (codex looped inside it) | 0 of Coeus's | 146,933 | 84,480 | 2,171 | not counted | $0.30 | 0/6 | 0/4 | 0/0 | 0 |
| opencode | 1 | yes | 44 s | 5 | 5 | 35,620 | 23,936 | 1,721 | 101 | $0.09 | 6/6 | 4/4 | 6/6 | 29 |
| hermes | 1 | NO, never started | 0 s | 0 | 0 | 0 | 0 | 0 | 0 | $0.00 | 0/6 | 0/4 | 0/0 | 0 |
| openclaw | 1 | yes | 1 min 11 s | not reported | 3 | last turn only | last turn only | last turn only | not reported | not reported | 6/6 | 4/4 | 6/6 | 27 |
| hermes | 2 | yes | 1 min 39 s | 7 | 13 | 156,659 | 131,072 | 2,052 | 407 | $0.20 | 6/6 | 4/4 | 6/6 | 28 |
| openclaw | 2 | yes | 1 min 6 s | not reported | 3 | last turn only | last turn only | last turn only | not reported | not reported | 6/6 | 4/4 | 6/6 | 25 |
| openclaw | 3 | yes | 1 min 10 s | not reported | 3 | last turn only | last turn only | last turn only | not reported | not reported | 6/6 | 4/4 | 6/6 | 25 |


## What the numbers say

1. **Three loops, three green runs.** opencode, Hermes and OpenClaw each drove
   GPT-5.6 Sol with their own loop and tools and produced a correct, playable
   game with six passing tests. Same judge as the Opus and Qwen phases.
2. **opencode was fastest and cheapest.** 44 seconds, five model calls, 35,620
   tokens in of which 23,936 were cached, 1,721 out, about $0.09 at list
   prices. Its prompt is small (about 6,000 tokens of its own) and it wrote
   the files without ceremony.
3. **Hermes spent the most tokens.** 156,659 in over seven calls, 131,072 of
   them cached: Hermes carries a large system prompt (its skills index, its
   memory guidance, its bundled instructions) and re-sends it every call.
   Cached input is a tenth of the price, so the bill was still only about
   $0.20. Thirteen tool calls, 1 min 39 s.
4. **OpenClaw finished in about a minute every time, but does not count its
   own tokens on this path.** Its JSON envelope and its kept state store hold
   only the last turn (about 18,000 tokens in, 22 out), even with its model
   transport diagnostics switched on, so its tokens and cost are marked "not
   reported" rather than guessed. Three runs, 71, 66 and 70 seconds, three
   green verdicts.
5. **Coeus is missing for a reason that is Coeus's fault**, explained above,
   with the fix under way.
6. **Two runs failed for reasons that were the benchmark's fault, not the
   harness's**, and both are recorded: Hermes's first run never started
   because the fresh home had no codex login (fixed by copying the login file
   into the fresh home, exactly as opencode's run does); Coeus's run is the
   provider defect.

## What this comparison can say

On GPT-5.6 Sol the three other harnesses are measured driving the model
themselves, which is what the Opus phase could not do. When Coeus's Codex
backend provider lands, this becomes the first four-way, loop-against-loop
comparison on a frontier cloud model with no API key, and the round is rerun
in full so all four rows come from the same hour.

## Appendix: notes per run

- coeus 1: exit 0, launch to exit 1 min 45 s; cache split from the record's per-turn line, rounded to the hundred
- opencode 1: exit 0, launch to exit 44 s; per call from its step_finish lines
- hermes 1: exit 0, launch to exit 0 s; no sessions row for the work folder
- openclaw 1: exit 0, launch to exit 1 min 11 s; a run total from its JSON envelope; status ok
- hermes 2: exit 0, launch to exit 1 min 39 s; a run total from its sessions row; Hermes counts fresh input and cached input apart
- openclaw 2: exit 0, launch to exit 1 min 6 s; a run total from its JSON envelope; status ok
- openclaw 3: exit 0, launch to exit 1 min 10 s; a run total from its JSON envelope; status ok

## How to reproduce

- `scripts/bench/gpt_runs.sh [round]` runs the four harnesses one at a time
  from blank folders (Hermes's fresh home needs the codex login copied in, as
  the second Hermes run did; the script will carry that fix).
- `scripts/bench/gpt_report.py --price 4 0.40 20` prints the tables from the
  run folders under `~/work/bench/gpt/`.
- `scripts/bench/check-tater.mjs` is the judge.
- Versions on the day: coeus 5bdf7cd; codex-cli 0.147.0; opencode 1.18.18; OpenClaw 2026.8.2 (0965053); Hermes Agent v0.20.1 (2026.8.13).
