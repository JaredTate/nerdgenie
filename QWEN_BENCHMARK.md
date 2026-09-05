# The Qwen 3.8 Benchmark: four harnesses driving the same local model

Nerd Genie, opencode, Hermes and OpenClaw on the same tic-tac-toe task, each driving
the local Qwen 3.8 27B with its own loop and its own tools, two at a time on
the machine's two graphics cards. This is the phase where nothing from Claude
Code or the codex program sits between a harness and the model: every harness
talks to the daemon over plain HTTP, so the loops are compared against each
other and nothing else. Runs on 2026-09-03; the plan and its caveats are in
`TEST.md`; the Opus and GPT phases are in `OPUS_BENCHMARK.md` and
`GPT_BENCHMARK.md`.

**The short version.** On a small local model, with every harness in the
setup it ships with and no cap on anyone, Nerd Genie finished the task green both
times, in 24 and 55 model calls, never carrying more than 34,500 tokens of
context. Hermes, opencode and OpenClaw all built a working game and then
looped on the draw case for hundreds of calls with one of their own tests
still red: 446 calls and 24 minutes for Hermes, 317 and 16 minutes for
opencode, and OpenClaw was stopped by hand at 623 calls and 38 minutes with
its prompt at 132,000 tokens. The mechanism is in the numbers: the three
transcript harnesses show the model everything it already tried and it
repeats it; Nerd Genie shows it a short record and refuses a repeated call.

Round 2, on the same rules, added the other half of the story: OpenClaw, stopped
at 623 calls in round 1, finished green in 82 seconds and 20 calls in round 2;
Hermes needed 504 calls and 81 minutes and still had a red test; opencode had
to be stopped after 1,587 calls and two and a half hours. Nerd Genie finished green
both times. Over eight runs, Nerd Genie is the only harness that was green every
time, and the only one whose prompt never left the range a small model reasons
well in.
opencode's round-2 run was stopped by hand after two and a half hours and
1,587 model calls with its prompt at 230,000 tokens and no sign of ending;
the user had stopped OpenClaw's equivalent run in round 1 the same way. It is
recorded as not finished, and its token totals are read from the daemon log
slice in its run folder.

## The task

The same text, byte for byte, from `~/work/bench/canonical/task.txt` (SHA-256
begins `c8ed58f2`, checked before each run): a tic-tac-toe game for the browser
in the current folder, `game.js` with three pure functions, an `index.html`
with a 3 by 3 grid, a status line and a New game button, `tests/game.test.mjs`
for Node's test runner with six named cases, run the tests, make them pass,
install nothing, stop when they pass.

## The model and the cards

One model file, Qwen 3.8 27B (`hauhau-Q4_K_P.gguf`), served by the TurboQuant
`llama-server` on two identical Radeon 7900 XTX cards with byte-identical
settings: 262,144-token context, temperature 0.7, speculative decoding on, one
request at a time per card. Card A answers on port 19091 and card B on 19093.
A run on either card is the same run.

**Thinking is off for everyone, and it was checked.** The daemon's chat
template is started with thinking disabled. A harness could turn it back on
only by sending `chat_template_kwargs: {enable_thinking: true}`; a request
with that field made the model think (173 tokens for a one-number answer,
with a reasoning field in the reply), and a request without it, carrying only
`reasoning_effort: low` the way the harnesses do, did not (5 tokens, no
reasoning field). Nerd Genie sends the field set to false; the other three send
nothing. So every run here is at thinking off, which is the floor; "low" and
"off" are the same thing on this daemon.

## What was held equal

- **The daemons' cache.** Round 1 began with both daemons restarted cold for
  Nerd Genie and Hermes; opencode and OpenClaw then ran on the same daemons without
  a restart, because a restart's memory spike had already made the kernel kill
  two other programs that afternoon. That is not identical treatment, and it is
  said here rather than hidden: the cache cannot change what the model says,
  since it only helps a prompt that begins with the previous conversation, and
  the daemon log shows opencode's first call reading its whole 7,732-token
  prompt from scratch; but a freshly started daemon has warm-up costs on its
  first calls that a running one does not, worth seconds, paid by the two that
  started cold. Round 2 runs everyone on the loaded daemons, the same for all,
  with each daemon's cache state recorded at every launch.
- **Blank start, every run.** Before the round, every earlier Qwen run folder
  was deleted. Every run gets a new, empty work folder holding only
  `task.txt` and an empty git repository, and a new home for the harness: a
  fresh `NERDGENIE_HOME`; fresh `XDG` folders and config for opencode; a fresh
  `HERMES_HOME` outside `~/.hermes`; a fresh `OPENCLAW_HOME` and config for
  OpenClaw. No memory, no past session, no instruction file.
- **One harness per card.** Round 1: Nerd Genie and opencode on card A, Hermes and
  OpenClaw on card B. Round 2 on card A alone, one after another, while the
  user tests Nerd Genie by hand on card B. A run never shares a card.
- **No cap of any kind.** Nerd Genie's own task budget raised out of reach; each
  harness keeps only the limits it ships with.
- **The same judge**, `scripts/bench/check-tater.mjs`, after the harness exited.
- **The same referee for tokens.** Every harness talks to the same daemon, and
  the daemon's own log records every call: the prompt tokens it had to read
  afresh, the tokens it generated, and the size of the context at the end of
  each call. `scripts/bench/countcalls.sh` reads that slice of the log for
  every harness alike, so the token numbers do not depend on any harness's
  bookkeeping. Each harness's own report is kept beside them as a
  cross-check.
- **The same clock**: launch to exit. Nerd Genie's driver leaves a short quiet
  period after the final reply, so its "task in to answer out" is also given.
- **No dollars.** The model runs on the user's own cards. Where a projection
  helps, tokens are what to compare.

## How to read the token columns

- **Calls** is how many times the harness asked the model.
- **Tokens in** is the full prompt of every call added up, cached or not: what
  an API would bill as input.
- **Read afresh** is the part of that the daemon could not take from its cache
  and had to process: the real work the card did. A harness that re-sends a
  long transcript pays it once in "read afresh" and thereafter in "cached".
- **Largest context** is the biggest prompt any single call carried, which is
  what decides whether a harness fits a small model at all.
- **Out** is what the model wrote.

## Results: round 1, everyone on shipped defaults

| Harness | Green (logic, plays, own tests) | Launch to exit | Model calls | Tokens in, all calls | Read afresh | Biggest context | Tokens out | Logic | Plays | Own tests passing |
|---|---|---|---|---|---|---|---|---|---|---|
| **Nerd Genie** | **yes** | 4 min 55 s (task in to answer out 4 min 15 s) | 24 | 253,214 | 82,577 | 15,858 | 5,821 | 6/6 | 4/4 | 6 of 6 |
| Hermes | game works, one own test red | 23 min 39 s | 446 | 29,474,314 | 65,320 | 105,566 | 41,708 | 6/6 | 4/4 | 5 of 6 |
| opencode | game works, one own test red | 16 min 20 s | 317 | 11,733,880 | 25,613 | 63,660 | 38,071 | 6/6 | 4/4 | 5 of 6 |
| OpenClaw | game works, one own test red; **stopped by the user** | 37 min 42 s when stopped | 623 | 44,246,207 | 51,264 | 131,948 | 80,686 | 6/6 | 4/4 | 5 of 6 |

"Green" is strict: the checker's six logic checks, its four play-through
checks, and every test the harness itself wrote passing. The runner's own
summary line calls a run "finished" on the first two alone, which is why the
three transcript harnesses say "game works" here and not "green."

## Results: round 2, everyone on shipped defaults, daemons already loaded

| Harness | Green | Launch to exit | Model calls | Tokens in, all calls | Read afresh | Biggest context | Tokens out | Logic | Plays | Own tests passing |
|---|---|---|---|---|---|---|---|---|---|---|
| **Nerd Genie** | **yes** | 12 min 30 s (task in to answer out 11 min 13 s) | 55 | 1,143,152 | 223,330 | 34,520 | 10,984 | 6/6 | 4/4 | 6 of 6 |
| opencode | game works, own tests 4 of 6; **stopped by the orchestrator** | 2 h 35 min when stopped | 1,587 | see note | see note | 230,324 | see note | 6/6 | 4/4 | 4 of 6 |
| Hermes | game works, one own test red | 1 h 21 min | 504 | see run folder | 660,135 | 214,099 | 66,026 | 6/6 | 4/4 | 5 of 6 |
| **OpenClaw** | **yes** | 1 min 22 s | 20 | 229,711 | 9,998 | 14,396 | 4,399 | 6/6 | 4/4 | 6 of 6 |

## What the numbers say

1. **Everyone builds a working game; only Nerd Genie stops when it is done.** All
   four produced a board the checker can play and win on. The difference is
   what happens at the draw case, which the small model gets wrong for
   everyone: Nerd Genie fixed it and stopped, in 24 calls the first time and 55
   the second; the other three fixed the game but never got their own draw
   test green, and kept trying for hundreds of calls.
2. **Context is the whole story.** Nerd Genie's biggest prompt was 15,858 tokens in
   run 1 and 34,520 in run 2. Hermes reached 105,566, opencode 63,660, OpenClaw
   131,948. A 27-billion-parameter model at four bits reasons badly over a
   hundred thousand tokens of its own failed attempts, and imitates what it
   sees; Nerd Genie never shows it the attempts, only one line per failure with
   its cause, the plan with check marks, and the last few results.
3. **Repetition is refused, not detected.** Hermes's loop was the same two
   commands alternating twenty times; its guards count failures, and those
   commands succeeded. opencode's guard needs three byte-identical calls and
   its probes varied by a character. Nerd Genie refuses the second identical call
   outright and tells the model so.
4. **The card's real work.** "Read afresh" is what the daemon had to process:
   Nerd Genie 82,577 and 223,330 tokens; Hermes 65,320; opencode 25,613; OpenClaw
   51,264. The transcript harnesses re-send tens of millions of tokens but the
   daemon reads almost all of it from its cache; their cost on a local card is
   time, not tokens, and the time was 16 to 38 minutes against 5 and 12.
5. **Nerd Genie asked for a yes.** Its permission function stopped six times in
   run 1 and once in run 2, every time on a long `node -e` command it could
   not read to the end, and the benchmark driver answered yes at once. A
   person would have had to. The other three never ask. That is Nerd Genie doing
   what it was built to do, and it is a rule worth loosening for commands
   that only read.
6. **Runs vary, and what holds across them is the point.** Nerd Genie went from
   24 calls to 55 between two runs of the same setup. Hermes went from 446
   calls and 24 minutes to 504 and 81 minutes, with its prompt at 214,000
   tokens, red test both times. opencode went from 317 calls and 16 minutes
   to a run stopped after 1,587 calls and two and a half hours at 230,000
   tokens, two red tests. OpenClaw went from a run stopped at 623 calls and
   38 minutes with its prompt at 132,000 tokens to a clean green in 20 calls
   and 82 seconds, its prompt never past 14,400 tokens. That last pair is the
   whole lesson in one harness: when the small model happens not to hit the
   draw-case wall, any harness finishes fast; when it does, a transcript
   harness has no way out and a record harness does. Over eight runs Nerd Genie
   was green both times, OpenClaw once, Hermes and opencode never fully.

## Set aside: the user's tuned setups

Two runs were made in the setup the user's own notes prescribe for this
model (opencode with its loop guard allowed, four tools off, output limit
131,072; Hermes with two tools only, reasoning low, loop guardrails, a
250-turn limit, and the sampler repeated per request). Both were green and
fast: opencode in 24 calls and 98 seconds, Hermes in 9 calls and 49 seconds.
They are not in the comparison, because the comparison is shipped defaults
for everyone; they are recorded under `~/work/bench/tater2-tuned/` and they
say what tuning buys a transcript harness on a small model. Nerd Genie was never
tuned.

## How to reproduce

- `scripts/bench/tater_run2.sh qwen <harness> <label> <card>` makes one run
  from a blank folder and a blank home; `scripts/bench/qwen_pair.sh` runs two
  at once on the two cards with memory guards; `scripts/bench/tater_table.mjs`
  prints the table from the run folders under `~/work/bench/tater2/`.
- `scripts/bench/countcalls.sh` is the token referee; `scripts/bench/check-tater.mjs` is the judge.
- The daemon: `~/llm/igo.sh Vulkan1 19091 262144 mtp` on card A and
  `Vulkan2 19093` on card B, exactly as `~/Desktop/RUN_AGENTS.md` prescribes;
  the model file's header reads `Qwen3.8-27B-Uncensored-HauhauCS-Aggressive`, Q4_K Medium.
