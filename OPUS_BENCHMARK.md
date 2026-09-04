# The Opus 4.8 Benchmark: Nerd Genie, OpenClaw and Hermes on the same task

Nine clean runs on 2026-09-03: three each of Nerd Genie, OpenClaw and Hermes, on
Claude Opus 4.8, on the user's Claude subscription with no API key, on one
tic-tac-toe task. This file is the whole record: what was held equal, every
run, every model call, and what the numbers say. Everything was measured from
its primary source and can be added up by hand from the tables at the end.

**The short version.** All nine runs finished green: the same correct, playable
game with six passing tests every time. Nerd Genie, driving Opus with its own loop,
took three to five model calls and 34 to 42 seconds. Claude Code's loop,
launched by OpenClaw, took three calls and 25 to 28 seconds. Hermes's runs are
Claude Code's loop again, plus about ninety seconds of Hermes managing the job
on the local Qwen. Priced from a cold cache, Nerd Genie is the cheapest way to get
the job done with a harness in the path ($0.35 a run against OpenClaw's
$0.54); priced with a warm cache, Nerd Genie costs a few cents more ($0.30 against
$0.24), and the whole of that gap is one provider bug in Nerd Genie that is named
below. Only Nerd Genie's own loop touched Opus today; every other column is Claude
Code's loop with a wrapper, and that limit is explained before the numbers.


## The task

One text, byte for byte for every run, from `~/work/bench/canonical/task.txt`
(its SHA-256 begins `c8ed58f2`, checked before each run): build a tic-tac-toe
game for the browser, in the current folder, with `game.js` exporting three pure
functions, an `index.html` with a 3 by 3 grid of buttons, a status line and a
New game button, and `tests/game.test.mjs` for Node's built-in test runner
covering a win in a row, a column and a diagonal, a draw, an illegal move on a
taken cell and no move after a win. Run the tests, make them pass, install
nothing, and stop the moment they pass.

## How each harness reaches Opus 4.8

There is no API key on this machine. Every path to Opus 4.8 goes through the
`claude` program, which is Claude Code, logged in to the subscription. Run with
`-p` it does one job and exits. All three harnesses end up running that one
program; what differs is what they do around it.

| | Nerd Genie | OpenClaw | Hermes |
|---|---|---|---|
| What it does | runs `claude -p` with Claude Code's own tools switched off and Nerd Genie's system prompt in place of Claude Code's, so the program is a bare model; the model asks Nerd Genie for a tool in text, Nerd Genie runs it, writes its record, and calls again | its `claude-cli` mode sends the task to Claude Code once, with Claude Code's own tools on; Claude Code's loop writes the files and runs the tests and hands back the finished folder | its Claude Code skill: Hermes runs on the local Qwen, types `claude -p --model claude-opus-4-8 --effort medium --dangerously-skip-permissions` with the task into its terminal tool, and waits |
| Whose loop drives Opus | Nerd Genie | Claude Code | Claude Code |
| Whose tools run | Nerd Genie's (`write`, `shell`, `task`) | Claude Code's (`Write`, `Bash`) | Claude Code's (`Write`, `Bash`) |
| Command | `coeus serve` on a fresh home, driven over its socket by `scripts/bench/drive.py` | `openclaw agent exec --message-file task.txt --model anthropic/claude-opus-4-8 --cwd <work> --timeout 0 --json` | `hermes chat -q "<prompt>" --provider bench -m local-coder --yolo --in <work>` with a fresh `HERMES_HOME` |

This is each harness as its own users run it on a subscription, and it means
the comparison on this path is Nerd Genie's loop against Claude Code's loop with
OpenClaw or Hermes wrapped around it. The proof is in the tools that ran: the
sessions OpenClaw and Hermes left behind show Claude Code's own `Write` and
`Bash`, not OpenClaw's `write` and `exec` or Hermes's `write_file`.

Hermes's prompt carries one extra paragraph in front of the task telling it to
use Claude Code and which flags to pass, because its skill needs to be asked
for; it is recorded word for word in every `hermes-N/prompt.txt`. opencode has
no route to Opus on a subscription (its own docs say Anthropic prohibits it,
and the installed copy lists no Anthropic model), so it is not in this phase.

## What was held equal

- **Blank start, every run.** Before the first run every earlier run folder was
  deleted, together with the Claude Code session and memory folders those runs
  had left under `~/.claude/projects/`. Every run got a new, empty work folder
  with a path used by no run before it, holding only `task.txt` and an empty
  git repository, and a new home for the harness: `COEUS_HOME`, a temporary
  OpenClaw state folder (which `agent exec` makes and deletes itself), a fresh
  `HERMES_HOME` outside `~/.hermes`. No memory file, no past session, no skill
  learned earlier, no instruction file in or above the work folder.
- **One at a time.** Nerd Genie, then OpenClaw, then Hermes, three rounds, never
  two at once, so no run slowed another.
- **Effort medium** wherever the knob exists: Nerd Genie through its `think`
  setting (`--effort medium` on every call), Hermes through the flag it types.
  OpenClaw's claude-cli path has no effort knob; its Claude Code ran at the
  program's default and its sessions show 10 to 20 thinking tokens per run, so
  it is already at the floor.
- **No cap of any kind.** Nerd Genie's own task budget (a hundred rounds and an hour
  by default) was raised out of reach so it could not act as one.
- **The same judge.** `scripts/bench/check-tater.mjs` on the work folder after
  the harness had exited: six logic checks, four play-through checks in a
  headless browser, and the harness's own tests counted and run.
- **The same clock.** Time is from the task going in to the final answer
  coming out: for Nerd Genie, the driver's log (the message sent, the reply
  received); for OpenClaw and Hermes, launch to exit. Nerd Genie's raw launch-to-exit
  is longer because the driver waits a fixed quiet period before leaving, and
  that is not the harness's time.
- **The same prices.** Anthropic's list prices per million tokens: fresh input
  $5, one-hour cache write $10 (the cache Claude Code uses), cache read $0.50,
  output $25. OpenClaw and Hermes rows are computed from the per-call usage in
  the Claude Code session file; Nerd Genie rows use Claude Code's own bill per call,
  which Nerd Genie's provider records.

## One thing that cannot be held equal, so it is shown both ways

Anthropic keeps a prompt in its cache for an hour after a call. A run whose
first call finds its prompt already there, left by an earlier run of the same
program within the hour, reads it at fifty cents a million instead of writing it
at ten dollars. The cache carries no information the model did not already
receive, so it cannot help a harness with the task; it only changes the price
of the first call. Runs 2 and 3 of every harness started warm from run 1, and
run 1 of OpenClaw and Hermes started warm from earlier runs that day. So every
run is priced twice: **as run**, and **if cold**, which charges the first
call's cache reads at the write price and is exactly what the same tokens would
have cost from a cold cache.
## Results: every run

| Harness | Run | Green | Task in to answer out | Opus calls | Tool calls | Tokens in | Cache write | Cache read | Out | Thinking | Cost as run | Cost if cold | Logic | Plays | Tests | game.js |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| coeus | 1 | yes | 34 s | 3 | 6 | 23,296 | 18,540 | 4,756 | 2,794 | not counted | $0.26 | $0.30 | 6/6 | 4/4 | 6/6 | 34 |
| coeus | 2 | yes | 42 s | 5 | 10 | 42,437 | 29,331 | 13,106 | 3,290 | not counted | $0.38 | $0.43 | 6/6 | 4/4 | 6/6 | 28 |
| coeus | 3 | yes | 35 s | 3 | 8 | 24,191 | 19,429 | 4,762 | 2,867 | not counted | $0.27 | $0.31 | 6/6 | 4/4 | 6/6 | 28 |
| openclaw | 1 | yes | 25 s | 3 | 4 | 130,618 | 12,801 | 117,811 | 1,983 | 16 | $0.24 | $0.54 | 6/6 | 4/4 | 6/6 | 27 |
| openclaw | 2 | yes | 28 s | 3 | 5 | 130,806 | 12,896 | 117,904 | 2,013 | 12 | $0.24 | $0.54 | 6/6 | 4/4 | 6/6 | 26 |
| openclaw | 3 | yes | 26 s | 3 | 5 | 130,894 | 12,935 | 117,953 | 2,030 | 10 | $0.24 | $0.54 | 6/6 | 4/4 | 6/6 | 26 |
| hermes | 1 | yes | 2 min 1 s | 3 (+9 Qwen) | 5 | 54,503 | 9,131 | 45,366 | 2,202 | 10 | $0.17 | $0.26 | 6/6 | 4/4 | 6/6 | 27 |
| hermes | 2 | yes | 1 min 56 s | 3 (+8 Qwen) | 4 | 53,990 | 8,882 | 45,102 | 2,047 | 12 | $0.16 | $0.26 | 6/6 | 4/4 | 6/6 | 26 |
| hermes | 3 | yes | 2 min 6 s | 2 (+13 Qwen) | 4 | 35,279 | 8,762 | 26,513 | 1,879 | 12 | $0.15 | $0.24 | 6/6 | 4/4 | 6/6 | 27 |

## Results: the average of the three runs

| Harness | Green | Task in to answer out | Opus calls | Tool calls | Tokens in | Cache read | Out | Cost as run | Cost if cold |
|---|---|---|---|---|---|---|---|---|---|
| coeus | 3 of 3 | 37 s | 3.7 | 8.0 | 29,975 | 7,541 | 2,984 | $0.30 | $0.35 |
| openclaw | 3 of 3 | 26 s | 3.0 | 4.7 | 130,773 | 117,889 | 2,009 | $0.24 | $0.54 |
| hermes | 3 of 3 | 2 min 1 s | 2.7 | 4.3 | 47,924 | 38,994 | 2,043 | $0.16 | $0.26 |

## What the numbers say, simply

1. **Everyone finished, and the work is the same.** Nine runs, nine green
   verdicts from the same checker, a `game.js` of 26 to 34 lines, six tests
   each. Quality does not separate them on this task.
2. **Nerd Genie keeps pace with Claude Code's own loop.** Three calls in two of its
   runs, five in one; 34, 42 and 35 seconds against Claude Code's 25, 28 and
   26. The eight to sixteen extra seconds are Nerd Genie's own bookkeeping: writing
   the done list into the record before the first file, checking the done list
   at the end, and one pause for permission (below).
3. **Nerd Genie paused twice for a yes.** In runs 1 and 3 Nerd Genie's permission
   function stopped before `cd "$(pwd)" && node --test tests/game.test.mjs`
   because the command "builds part of itself at run time" (the `$(pwd)`),
   showed a preview, and waited. The benchmark driver said yes at once; a
   person at the keyboard would have had to. Claude Code, OpenClaw and Hermes
   never ask. This is Nerd Genie doing what it was built to do, and it is also a
   rule worth a second look: `$(pwd)` cannot hurt anyone.
4. **Run 2 of Nerd Genie wandered.** The model wrote a `game.js.placeholder`,
   deleted it with a shell command, and then updated the record line by line
   in four separate `task` calls. Same green result, five calls and ten tool
   calls instead of three and six, $0.38 instead of $0.26. That is the spread
   one model shows on one prompt, and it is why there are three runs.
5. **Tokens in is not the cost.** OpenClaw's runs send 131,000 tokens and cost
   $0.24 because 118,000 of them are the same 42,000-token Claude Code prompt
   (its system prompt plus OpenClaw's tool definitions) read back three times
   at fifty cents a million. Nerd Genie's runs send 23,000 to 42,000 tokens and cost
   $0.26 to $0.38 because most of them are written to the cache at ten dollars
   a million and never read back. A written token costs twenty read tokens.
6. **Cold, Nerd Genie is the cheapest harness in the path.** Charging every run's
   first call at the write price, as a first run of the day pays: Nerd Genie $0.35,
   Hermes $0.26 (plain Claude Code with a small prompt), OpenClaw $0.54
   (Claude Code carrying OpenClaw's 33 tools). Warm, which is what a person
   running several tasks in an hour pays: Nerd Genie $0.30, OpenClaw $0.24, Hermes
   $0.16. Both are true; the cache favours whoever sends the same big prompt
   again, and Nerd Genie's provider does not let it do that yet.
7. **Nerd Genie writes about a thousand more output tokens a run** (2,984 against
   about 2,000). That is the record: the why, the done list, the plan, and the
   lines that pin each done item to a result, written through the `task` tool.
   At $25 a million it is two and a half cents a run. It is the price of a
   record a person can read, and it is small.
8. **Effort.** Nerd Genie and Hermes ran at effort medium. OpenClaw's path has no
   knob and its Claude Code showed 10 to 16 thinking tokens a run, so it was
   already at the floor. Nerd Genie's counters do not separate thinking from other
   output; an earlier run at the program's default effort produced 6,753
   output tokens on its first call against 2,277 at medium, which is what led
   to the `think` setting and to pinning it.

## What Nerd Genie should fix, in order

1. **Read its own prefix from cache on every call.** On the `claude -p` path
   the provider writes the whole system prompt, the changing record included,
   into one file, so the cached prefix breaks the moment the record changes,
   which is every call. Runs show reads only on the first call of a run, when
   the record is still empty and matches the run before. Keeping the changing
   tail out of the system prompt file (it belongs with the conversation on
   standard input, where the Anthropic path already puts it) would let the
   persona, rules and tool list, about 4,900 tokens, be read at fifty cents
   instead of written at ten dollars on every call after the first. On these
   runs that is about nine cents a run: Nerd Genie warm would be about $0.21, under
   OpenClaw's $0.24.
2. **Do not ask about `$(pwd)`.** A command that only reads its own working
   folder is not a command that "builds part of itself." Two of three runs
   waited on a person for it.
3. **Let the model pin several done lines in one `task` call.** Run 2 spent
   four calls' worth of tool traffic on lines it could have written at once.

## What this comparison can and cannot say

It can say how Nerd Genie's loop compares with Claude Code's loop on the same task,
model and judge, three times over, and it can say what each wrapper adds on
top: OpenClaw adds a 42,000-token tool catalogue to every call, Hermes adds a
second brain on a local card and ninety seconds. It cannot say how OpenClaw's
own loop or Hermes's own loop would do on Opus, because on a subscription
neither of them drives the model; they launch Claude Code and wait. Nor can it
say anything about opencode on Opus. Each of those three drives Opus with its
own loop only through an Anthropic API key, which this machine does not have.
The four-way comparison of the loops themselves is the Qwen phase in
`BENCHMARK.md`, where every harness talks to the same local model directly.

## Appendix: every model call

| Run | Call | Fresh | Cache write | Cache read | Out | Thinking | Tokens in | Cost | Tools |
|---|---|---|---|---|---|---|---|---|---|
| coeus 1 | 1 | 0 | 160 | 4,756 | 2,277 |  | 4,916 | $0.061 | Nerd Genie's own tools |
| coeus 1 | 2 | 0 | 8,989 | 0 | 296 |  | 8,989 | $0.097 | Nerd Genie's own tools |
| coeus 1 | 3 | 0 | 9,391 | 0 | 221 |  | 9,391 | $0.099 | Nerd Genie's own tools |
| coeus 2 | 1 | 0 | 154 | 4,762 | 671 |  | 4,916 | $0.021 | Nerd Genie's own tools |
| coeus 2 | 2 | 0 | 7,076 | 0 | 1,778 |  | 7,076 | $0.115 | Nerd Genie's own tools |
| coeus 2 | 3 | 0 | 5,165 | 4,194 | 102 |  | 9,359 | $0.056 | Nerd Genie's own tools |
| coeus 2 | 4 | 0 | 5,595 | 4,150 | 523 |  | 9,745 | $0.071 | Nerd Genie's own tools |
| coeus 2 | 5 | 0 | 11,341 | 0 | 216 |  | 11,341 | $0.119 | Nerd Genie's own tools |
| coeus 3 | 1 | 0 | 154 | 4,762 | 2,223 |  | 4,916 | $0.059 | Nerd Genie's own tools |
| coeus 3 | 2 | 0 | 9,070 | 0 | 479 |  | 9,070 | $0.103 | Nerd Genie's own tools |
| coeus 3 | 3 | 0 | 10,205 | 0 | 165 |  | 10,205 | $0.106 | Nerd Genie's own tools |
| openclaw 1 | 1 | 2 | 10,421 | 31,632 | 1,811 | 16 | 42,055 | $0.165 | Write, Write, Write |
| openclaw 1 | 2 | 2 | 2,073 | 42,053 | 120 | 0 | 44,128 | $0.045 | Bash |
| openclaw 1 | 3 | 2 | 307 | 44,126 | 52 | 0 | 44,435 | $0.026 | none |
| openclaw 2 | 1 | 2 | 10,419 | 31,632 | 1,833 | 12 | 42,053 | $0.166 | Write, Write, Write, Write |
| openclaw 2 | 2 | 2 | 2,170 | 42,051 | 120 | 0 | 44,223 | $0.046 | Bash |
| openclaw 2 | 3 | 2 | 307 | 44,221 | 60 | 0 | 44,530 | $0.027 | none |
| openclaw 3 | 1 | 2 | 10,429 | 31,632 | 1,862 | 10 | 42,063 | $0.167 | Write, Write, Write, Write |
| openclaw 3 | 2 | 2 | 2,199 | 42,061 | 120 | 0 | 44,262 | $0.046 | Bash |
| openclaw 3 | 3 | 2 | 307 | 44,260 | 48 | 0 | 44,569 | $0.026 | none |
| hermes 1 | 1 | 2 | 6,455 | 10,029 | 2,036 | 10 | 16,486 | $0.120 | Write, Write, Write, Write |
| hermes 1 | 2 | 2 | 2,369 | 16,484 | 119 | 0 | 18,855 | $0.035 | Bash |
| hermes 1 | 3 | 2 | 307 | 18,853 | 47 | 0 | 19,162 | $0.014 | none |
| hermes 2 | 1 | 2 | 6,440 | 10,029 | 1,876 | 12 | 16,471 | $0.116 | Write, Write, Write |
| hermes 2 | 2 | 2 | 2,135 | 16,469 | 119 | 0 | 18,606 | $0.033 | Bash |
| hermes 2 | 3 | 2 | 307 | 18,604 | 52 | 0 | 18,913 | $0.014 | none |
| hermes 3 | 1 | 2 | 6,455 | 10,029 | 1,865 | 12 | 16,486 | $0.116 | Write, Write, Write, Bash |
| hermes 3 | 2 | 2 | 2,307 | 16,484 | 14 | 0 | 18,793 | $0.032 | none |

## Appendix: notes per run

- coeus 1: exit 0, launch to exit 50 s, Claude Code's own bills sum to $0.27; driver answered 0 questions and 1 previews
- coeus 2: exit 0, launch to exit 1 min 10 s, Claude Code's own bills sum to $0.39
- coeus 3: exit 0, launch to exit 1 min 20 s, Claude Code's own bills sum to $0.28; driver answered 0 questions and 1 previews
- openclaw 1: exit 0, launch to exit 25 s, Claude Code's own time 22.3 s; OpenClaw turns: 1
- openclaw 2: exit 0, launch to exit 28 s, Claude Code's own time 25.0 s; OpenClaw turns: 1
- openclaw 3: exit 0, launch to exit 26 s, Claude Code's own time 22.2 s; OpenClaw turns: 1
- hermes 1: exit 0, launch to exit 2 min 1 s; claude -p typed by Hermes
- hermes 2: exit 0, launch to exit 1 min 56 s; claude -p typed by Hermes
- hermes 3: exit 0, launch to exit 2 min 6 s; claude -p typed by Hermes

## How to reproduce

- `scripts/bench/opus_runs.sh` purges the old data and makes the nine runs,
  one at a time, exactly as above. It needs the card-A Qwen daemon up for
  Hermes's own brain, and the `claude`, `openclaw` and `hermes` programs logged
  in on the machine.
- `scripts/bench/opus_report.py` prints every table in this file from the run
  folders under `~/work/bench/opus3/` and the Claude Code session files under
  `~/.claude/projects/`.
- `scripts/bench/check-tater.mjs` is the judge.
- Versions on the day: coeus built from 0e85125; claude 2.1.259; openclaw OpenClaw 2026.8.2 (0965053); hermes Hermes Agent v0.20.1 (2026.8.13).
