# The GPT-5.6 Sol Benchmark: all four harnesses, each driving the model itself

One clean round on 2026-09-03: Nerd Genie, opencode, Hermes and OpenClaw on
GPT-5.6 Sol at thinking medium, on the user's ChatGPT subscription with no API
key, on the same tic-tac-toe task as the Opus and Qwen phases. This is the
comparison the Opus phase could not give: every harness here drives the model
with its own loop and its own tools, because OpenAI lets its Codex login be
used by outside tools and Anthropic does not.

**The short version.** All four harnesses finished green on GPT-5.6 Sol, each
driving the model with its own loop on the subscription, no API key: opencode
in 76 seconds and 5 calls for about eight cents, OpenClaw's own loop in 88
seconds and 5 turns for about fourteen cents, Hermes in 2 min 2 s and 10
calls for about twenty-four cents, Nerd Genie in 2 min 28 s and 10 calls for about
twenty-eight cents, through a provider built this afternoon after its first
route proved unusable.
**Nerd Genie's first attempt could not be measured**, for a defect in Nerd Genie proved
below: the `codex` program it ran describes its own tools to the model inside
a developer message that no switch removes, so the model used those tools in
a read-only sandbox and Nerd Genie's own tools were never called. The fix was a
new provider, `provider = "codex"`, that talks to OpenAI's Codex backend
directly with the codex login, the way the other three do, built and gated
the same afternoon; round 2 above is the result: Nerd Genie green in ten calls,
2 min 28 s, about 28 cents.


## The task

The same text, byte for byte, from `~/work/bench/canonical/task.txt` (SHA-256
begins `c8ed58f2`, checked before each run): a tic-tac-toe game for the
browser in the current folder, `game.js` with three pure functions, an
`index.html` with a 3 by 3 grid, a status line and a New game button,
`tests/game.test.mjs` for Node's test runner with six named cases, run the
tests, make them pass, install nothing, stop when they pass.

## How each harness reaches GPT-5.6 Sol, and what went wrong for Nerd Genie

| | Nerd Genie | opencode | Hermes | OpenClaw |
|---|---|---|---|---|
| Login | the `codex` program, logged in to the subscription | opencode's own "ChatGPT Plus/Pro" login | the codex login, which Hermes's `openai-codex` provider reads | the codex login, through OpenClaw's `openai` provider |
| How the model is called | round 1: `codex exec` as a bare model (unusable, see below); round 2: its `codex` provider, straight to OpenAI's Codex backend with the codex login; Nerd Genie's tools do the work | its own loop, straight to OpenAI's Codex backend | its own loop, straight to OpenAI's Codex backend | **by default, the codex program** (OpenClaw's "codex" runtime, a wrapper like claude-cli); its own embedded loop only when `agentRuntime.id = "openclaw"` is pinned on the model in its config |
| Whose loop | Nerd Genie | opencode | Hermes | codex by default; OpenClaw when pinned |
| Thinking | `think = "medium"` in its config, sent as the request's reasoning effort | `--variant medium` | `--reasoning medium` | `--thinking medium` |
| Command | `coeus serve` on a fresh home, driven over its socket by `scripts/bench/drive.py` | `opencode run -m openai/gpt-5.6-sol --variant medium --format json "<task>"` | `hermes chat -q "<task>" --provider openai-codex -m gpt-5.6-sol --reasoning medium --yolo --in <work>` | `openclaw agent exec --message-file task.txt --model openai/gpt-5.6-sol --thinking medium --cwd <work> --timeout 0 --json` |

Each was proved with a one-word test call before the round. Then the kept
state folder of an OpenClaw run showed a `codex-home` with a codex rollout
file: on a ChatGPT subscription OpenClaw's default is to hand the whole turn
to the codex program (its docs: "selecting `openai/*` for an agent model now
means run this through Codex"), the same wrapper pattern as claude-cli on Opus.
Its docs also say an explicit `agentRuntime.id: "openclaw"` on the model
"keeps a Codex-eligible route on OpenClaw", so a further run pinned that and
kept its state: no rollout file, OpenClaw's own tools (`exec`, `apply_patch`,
`progress_card`), six assistant turns in its envelope. Both are in the table,
labelled. Hermes's session shows its own tool calls and seven API calls of its
own, so Hermes ran its own loop.


**Why Nerd Genie's row is not a result.** Nerd Genie's `codex` provider runs `codex exec`
once per model call with its shell tools switched off (`--disable shell_tool
--disable unified_exec`), Nerd Genie's system prompt in place, and a read-only
sandbox, expecting the program to behave as a bare model and to write tool
calls in Nerd Genie's text form. It does not. A local listener stood in for the
backend and recorded the exact request the program sends: the `tools` field is
empty, but the `input` carries a developer item of type `additional_tools`,
18,472 characters long, describing the program's own tools (its exec, its file
patcher, its planner, its agent-spawning tools). With every documented switch
off (`shell_tool`, `unified_exec`, `multi_agent`, `apps`, `web_search`,
`view_image`, `agents.enabled`, `approval_policy=never`) that item shrinks to
8,142 characters and does not go away. So the model always has the program's
tools in front of it, prefers them, tried to write the files with them inside
the read-only sandbox, was refused, looped inside the program for 146,933
tokens, and told Nerd Genie "the workspace is read-only, so file creation was
rejected." Nerd Genie's own `write` and `shell` tools were never asked for. The
run is recorded, but it measures the codex program, not Nerd Genie.

The fix is not a flag. It is a provider that speaks to OpenAI's Codex backend
directly with the codex login, the route Hermes, OpenClaw and opencode use, so
that Nerd Genie's loop and tools drive the model. That provider is being built and
this file gets Nerd Genie's row when it is in.

## What was held equal

- **Blank start.** A new, empty work folder with a path no run had used, only
  `task.txt` and an empty git repository; a new home for the harness
  (`COEUS_HOME`; fresh `XDG` folders for opencode, holding only a copy of its
  login file; a fresh `HERMES_HOME` outside `~/.hermes`; OpenClaw's own
  throw-away state folder). No memory, no past session, no instruction file.
- **One at a time.** Nerd Genie, opencode, Hermes, OpenClaw, in that order.
- **Thinking medium** on every one, by the flag or setting each harness has for it.
- **No cap of any kind.** Nerd Genie's own task budget raised out of reach.
- **The same judge**, `scripts/bench/check-tater.mjs`, after the harness exited.
- **The same clock**: task in to final answer out. For Nerd Genie that is its
  driver's log; for the others, launch to exit.
- **The same prices**, OpenAI's list for GPT-5.6 Sol on 2026-09-03: input $4,
  cached input $0.40, output $20, per million tokens. Every harness reports its
  own token counts, and the source for each is named in the notes.

## Where the token numbers come from, and one caution

Each harness counts its own tokens, from the usage the API returned to it:
Nerd Genie from each `codex exec` result, opencode from its per-step records,
Hermes from its session database, OpenClaw from its run envelope. There is no
outside referee on this path the way the daemon log is on Qwen, so the counts
are only as honest as each harness's bookkeeping. OpenClaw's envelope reports a
run total and no per-call rows; Hermes reports a run total too. Nerd Genie's cached
count comes from its record's per-turn line, which rounds to the hundred.
Reasoning tokens are inside the output count wherever the harness reports
them, and shown apart where it does.
## Results: the round that counts (round 2, all four driving the model themselves)

Run at 15:15 to 15:23 on 2026-09-03, one harness at a time from blank folders
and blank homes, Nerd Genie through its new `codex` provider, OpenClaw pinned to its
own loop, thinking medium everywhere.

| Harness | Run | Green | Task in to answer out | Model calls | Tool calls | Tokens in | of which cached | Tokens out | of which reasoning | Cost | Logic | Plays | Tests | game.js |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| coeus | 2 | yes | 2 min 28 s | 10 | 21 | 69,085 | 16,000 | 3,167 | not counted | $0.28 | 6/6 | 4/4 | 6/6 | 28 |
| opencode | 2 | yes | 1 min 16 s | 5 | 5 | 35,633 | 27,520 | 1,710 | 84 | $0.08 | 6/6 | 4/4 | 6/6 | 32 |
| hermes | 2 | yes | 2 min 2 s | 10 | 20 | 233,087 | 206,080 | 2,327 | 385 | $0.24 | 6/6 | 4/4 | 6/6 | 26 |
| openclaw, its own loop | 2 | yes | 1 min 28 s | 5 | 5 | 88,313 | 69,120 | 1,805 | 106 | $0.14 | 6/6 | 4/4 | 6/6 | 28 |

Cost is OpenAI's list price for GPT-5.6 Sol on 2026-09-03: input $4, cached
input $0.40, output $20, per million tokens, applied to each harness's own
token counts. Nerd Genie's cached count comes from its record's per-turn line,
rounded to the hundred; Nerd Genie's provider does not count reasoning tokens
apart from output.

**Reading it.** All four finished green with their own loop and their own
tools, the same correct game, the same six tests. opencode was the fastest and
cheapest (five calls, 76 seconds, about eight cents). OpenClaw's own loop took
five turns and about fourteen cents. Nerd Genie took ten calls: it writes its record
first (the why, the done list, the stop list, the plan) before touching a file,
reads the folder, writes the three files, runs the tests, and checks the done
list, and on this run it also read its browser skill; the record costs calls
on a three-file task and pays for itself on a forty-round one. Hermes took ten
calls and twenty tool calls and re-sent a large prompt each time (206,000 of
its 233,000 tokens were cache reads). Time is task in to answer out for Nerd Genie
and launch to exit for the others.

## Results: round 1, kept for what it taught

| Harness | Run | Green | Task in to answer out | Model calls | Tool calls | Tokens in | of which cached | Tokens out | of which reasoning | Cost | Logic | Plays | Tests | game.js |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| coeus | 1 | NO | 1 min 25 s | 1 | 0 | 146,933 | 0 | 2,171 | not counted | $0.63 | 0/6 | 0/4 | 0/0 | 0 |
| opencode | 1 | yes | 44 s | 5 | 5 | 35,620 | 23,936 | 1,721 | 101 | $0.09 | 6/6 | 4/4 | 6/6 | 29 |
| hermes | 1 | NO | 0 s | 0 | 0 | 0 | 0 | 0 | 0 | $0.00 | 0/6 | 0/4 | 0/0 | 0 |
| openclaw | 1 | yes | 1 min 11 s | None | 3 | 18,370 | 17,920 | 22 | not counted | $0.01 | 6/6 | 4/4 | 6/6 | 27 |
| coeus | 2 | yes | 2 min 28 s | 10 | 21 | 69,085 | 16,000 | 3,167 | not counted | $0.28 | 6/6 | 4/4 | 6/6 | 28 |
| opencode | 2 | yes | 1 min 16 s | 5 | 5 | 35,633 | 27,520 | 1,710 | 84 | $0.08 | 6/6 | 4/4 | 6/6 | 32 |
| hermes | 2 | yes | 2 min 2 s | 10 | 20 | 233,087 | 206,080 | 2,327 | 385 | $0.24 | 6/6 | 4/4 | 6/6 | 26 |
| openclaw | 2 | yes | 1 min 28 s | 5 | 5 | 88,313 | 69,120 | 1,805 | 106 | $0.14 | 6/6 | 4/4 | 6/6 | 28 |
| openclaw | 3 | yes | 1 min 10 s | None | 3 | 18,316 | 17,920 | 22 | not counted | $0.01 | 6/6 | 4/4 | 6/6 | 25 |
| openclaw | own-1 | yes | 1 min 33 s | 6 | 5 | 105,945 | 86,144 | 2,022 | 200 | $0.15 | 6/6 | 4/4 | 6/6 | 30 |


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
4. **OpenClaw has two modes here, and the table shows both.** By default it
   handed the task to the codex program: 66 to 71 seconds, green every time,
   and the one run whose state was kept shows the codex program's own bill:
   4 calls, 69,576 tokens in of which 50,816 cached, 1,561 out, about $0.13,
   with codex's `exec` doing the work. That measures codex with OpenClaw
   around it. Pinned to its own loop, OpenClaw took 1 min 33 s, six turns,
   105,945 tokens in of which 86,144 cached, 2,022 out (200 reasoning), about
   $0.15 at list prices (OpenClaw's own figure says $0.20), with its own
   `exec` and `apply_patch` tools. That is OpenClaw's harness, and it is the
   row to compare with the others.
5. **Nerd Genie is missing for a reason that is Nerd Genie's fault**, explained above,
   with the fix under way.
6. **Two runs failed for reasons that were the benchmark's fault, not the
   harness's**, and both are recorded: Hermes's first run never started
   because the fresh home had no codex login (fixed by copying the login file
   into the fresh home, exactly as opencode's run does); Nerd Genie's run is the
   provider defect.

## What this comparison can say

On GPT-5.6 Sol the three other harnesses are measured driving the model
themselves (OpenClaw only in its pinned own-loop run), which is what the Opus
phase could not do. When Nerd Genie's Codex
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
  from blank folders, copies the codex login into Hermes's fresh home, and pins
  OpenClaw to its own loop with a kept state folder that proves it.
- `scripts/bench/gpt_report.py --price 4 0.40 20` prints the tables from the
  run folders under `~/work/bench/gpt/`.
- `scripts/bench/check-tater.mjs` is the judge.
- Versions on the day: coeus 5bdf7cd; codex-cli 0.147.0; opencode 1.18.18; OpenClaw 2026.8.2 (0965053); Hermes Agent v0.20.1 (2026.8.13).
