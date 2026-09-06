# The plan: what to build next so Nerd Genie finishes hard jobs on Qwen 3.8, every time

Written 5 September 2026, after five Tetris runs on the local Qwen 3.8 27B, two research documents (`STATE_RESEARCH.md`, `LLM_RESEARCH.md`) and the five reports under `docs/research/2026-09-05-state-and-llm/`. Plain English. Every idea was weighed against what actually broke on this machine, then checked a second time against the numbers of run 5, and two ideas moved because of that. The rules: keep it simple (KISS), and build nothing until it is needed (YAGNI).

## The short version

1. The design is right. The record beside the log beside a fresh working context is what the field is now backing toward: summarization was measured to lose 30 points, and the cache-hit-rate metric that Manus calls the one that matters is exactly what a fixed record buys. Run 5 reused 62,600 of 69,000 tokens per round at round 134.
2. What failed tonight was not the design. It was three lies and two gaps: a click tool that said "worked" when nothing changed, a browser result that never showed the page's errors, a guard that stopped instead of rewinding, a follow-up message that started a blank task, and a job whose tasks never started because the task that made it kept working. All five are fixed, tested and committed today.
3. The measured truth about speed: a round costs about 16 seconds, all of it the model, and the model never once put two tool calls in one reply in 150 rounds. So **40 percent of run 5's rounds did no work on the world**: 22 percent wrote only to the record, 18 percent only ran the tests. Folding those into working rounds is the biggest speed win there is, and it needs no new model and no bigger window.
4. The reliability wins are about **signals and bounds**: tell the model the truth after every action, count progress and rewind before stopping, and never let "done" stand without proof a person would accept.
5. Twelve ideas made the cut. Six go first. Eleven were rejected on purpose, with reasons, so nobody rebuilds them by accident.

## What the second check changed

Two claims in the first draft did not survive the numbers.

- **The re-read tail is not the speed lever.** I expected the part of the prompt the daemon re-reads every round to grow with the record. It does not: it holds at about 1,200 tokens in steady state, three seconds of prefill. The rest of what looks like re-reading is new text that has to be read once anyway (the model's last reply and the new tool results). Capping the results list would save under two seconds a round. Dropped.
- **A smaller message window has a cost I had not counted.** After the window drops its oldest half (round 100 in run 5), the daemon re-reads five to seven thousand extra tokens a round for about twenty rounds while its checkpoints land again: roughly three minutes per drop. Halving the window doubles the drops. The quality evidence for a smaller window is real but general; the speed cost is measured and ours. So the window is now a measurement, not a build, and the job hand-off delivers small windows anyway: ten job tasks each start fresh.

And one thing the numbers made bigger than I had it: output tokens were 52,700 of run 5's first 150 rounds, which at 55 tokens a second is 16 of the 41 minutes. Writing less is a first-order lever, and every idea that writes more was re-checked against it.

## What was fixed today, so the baseline is clear

| Failure seen | What was wrong | What changed | Proof |
|---|---|---|---|
| Start button did nothing, tool said "what was expected happened" | The click judge matched a word of the expectation against the clicked button's own name | The clicked element is no evidence. A click that changed nothing says "nothing changed" | `worker/browser/test/actions.test.ts` |
| Script answered 404, nothing told the model | No browser result carried the page's console | Every snapshot carries `errors`: failed loads with status, script errors, first five and a count; printed as `page errors:` | `worker/browser/test/page-errors.test.ts`, `internal/tool/browserread` |
| Task stopped after four identical reads | The guard ended the task on the third refusal | A stall clears the conversation, writes the stall into the record as a failure, three times, then stops | `internal/loop/guard_test.go` |
| "the start game button wont work" started a blank task | Only four words picked a stopped task up | Any message after a stopped or failed task carries it on, as a correction; `/clear` sets it aside. A follow-up on finished work sees the files and where it stood | `cmd/nerdgenie/resuming_test.go`, `test/functional/resume_test.go` |
| Job of ten tasks showed 0 of 10 for an hour | The task that made the job kept working; the job's tasks could not start | The task ends the moment the job is made, with one done line naming the job; the driver takes task one | `internal/loop/handoff_test.go` |

Also in today: plan steps can be marked done (`step_done`), the probe rule (five throwaway scripts with no edit sends one line; it fired in run 5 and the model wrote the failure itself), the review runs before the wrap-up clock, tool markup is never saved as a lesson, and the message window drops its oldest half at once.

**Found and fixed later in run 5, after this plan's first draft:**

| Failure seen | What was wrong | What changed | Proof |
|---|---|---|---|
| Task 1 failed at its 223rd result with "the record would pass its size" | Result lines were never dropped, so a harness write of the situation hit the 3,000-token cap and failed the task | The results list gives first: oldest lines leave, down to a hundred and never below ten; every one stays readable by label from the log | `internal/record/size_test.go` |
| The Start button hangs the page; the tool said "open the page again"; the model did, twice | The hung-page error blamed the page, not its script | Both the scan deadline and the method deadline say the page's own script is the likely cause, an endless loop that starts on this action, and to fix the script | `worker/browser/test/hung-page.test.ts` |
| Eleven desktop launches in a row, never refused | Each call's intent sentence was worded differently, so the guard saw no repeat | The fingerprint leaves out intent, expectation, why, goal and reason | `internal/loop/guard_test.go` |
| `pkill -f` on its own server, exit 143 | The shell tool ran it | Refused with the reason and the fix: find the id by port, kill that id | `internal/tool/shell/shell_test.go` |
| Sixty rounds on two tests in run 4; thirty on a hung page in run 5, each round a character different | No meter for progress | Ideas 1 and 2 below are built: the tests run themselves after every change, and rounds without progress climb the ladder | `internal/loop/autotest_test.go`, `progress_test.go` |

And one the job phase proved: when task 1 ended, the driver took the job at once, and tasks t1 through t7 verified and reported in under thirty minutes with fresh windows of ten to forty thousand tokens and rounds of three to five seconds. The hand-off is the right shape.

## The numbers the ranking rests on

Run 5, first 150 rounds, 41 minutes, all on the local model:

| Measure | Value | What it means |
|---|---|---|
| Seconds per round | 16 | All model time; every tool answered in under 0.1 s |
| Replies with more than one tool call | 0 of 150 | The model never batches, whatever the instructions say |
| Rounds that only wrote the record | 33 (22%) | 13 plan rewrites, 14 step marks, the rest why, done list, failures, pins |
| Rounds that only ran the tests | 27 (18%) | Every test run was its own round |
| Repeated identical reads | 2 of 23 | Re-reading is not the waste it was in run 4 |
| Whole-file rewrites | 0 of 19 | The model edits rather than rewriting; good |
| Output tokens | 52,700 | 16 minutes of the 41 |
| Prompt re-read per round, steady state | about 1,200 tokens | 3 seconds; not the lever |
| Cache reuse at round 134 | 62.6k of 69.0k | The prefix cache works |

## How the ideas were judged

Four questions each. Would it have changed tonight's outcome, or run 4's sixty-round stall, or run 5's 41 minutes? Is there evidence outside our own runs? Can it be built as one small change with one test? What does it cost per round in tokens or seconds? Then a fifth, added on the second pass: play the change forward through run 5's log and say what it would have done, round by round.

## The twelve, ranked

### 1. Fold the wasted rounds: tests run themselves, and a step mark rides on the reply

**What.** Two changes, one idea. First, after any `write` or `edit` of a source or test file, the harness itself runs the project's test command, the one it learned from the model's last test run, and puts the tests line on the write's result: "wrote engine.js, 240 lines; tests: 2 failing of 62: ...". Second, the model marks a plan step done in its first line, the one the harness already reads for "where the work stands": "Step 3 done, r41. Next: the renderer." The harness marks the step; no tool call, no round. Plan rewrites stay tool calls.

**Why.** The model never sends two calls in one reply, so every record write and every test run is a whole round of 16 seconds. In run 5 that was 60 of 150 rounds. Played forward: the 27 test-only rounds fold into the 46 write and edit rounds that preceded them; the 14 step-mark rounds vanish; the run's first 41 minutes become about 28. That is the whole gap to the harnesses that finish in 30.

**How.** The test-state reader already parses the runner's output; the harness remembers the last test command the model ran and runs it under a ten-second cap after a source change, updating the tests line and the result line only, never writing a failure line for a run the model did not ask for. The first-line parser gains one pattern, "step N done, rM", checked against the record the way `step_done` is. Both are told to the model in one sentence each of the instruction text.

**Test.** A scripted write of a source file after one test run yields a result line carrying the fresh tests line; a reply whose first line says "step 2 done, r5" marks step 2 with r5, and one naming a result the record never wrote marks nothing and says so.

**Cost.** The test suite's own run time, capped. **Risk.** A slow suite; the cap and a "tests skipped, suite too slow" line cover it. A model that writes "step 3 done" without a result gets the same refusal the tool gives. **Verdict.** Built, the first half: the tests run themselves after every write or edit once the model has run them once, and the instruction sentence says so. The step mark on the first line is not built yet.

### 2. A progress meter, and a ladder: nudge, rewind, stop

**What.** The harness counts rounds since real progress: a done line proven, a plan step marked, the tests line improving, a browser action whose expectation was met, a new file written that is not a probe. It writes the count into the Situation as one line. At ten rounds without, one line in the harness's own words. At twenty, rewind: clear the conversation, keep the record, write one failure line with the cause. At thirty, stop and ask the person.

**Why.** Run 4's stall was sixty rounds on two tests, every probe a character different, so the identical-call guard never saw it; tonight's rewind fires only on identical calls. Played forward through run 4: improvement last seen at round 30, the nudge at 40, the rewind at 50, ten rounds before the model found its own way out; through run 5: never fires, because the tests line kept improving. Evidence: models "become more likely to make mistakes when the context contains their errors" and size does not fix it (ICLR 2026); rollback recovered 45 percent of failed runs against 16 for retries (Aug 2026); OpenHands ships a stuck detector on by default.

**How.** One counter on the run, reset by the five signals, and the rewind that exists. A polled long command is exempt. Three constants.

**Test.** A scripted task that edits and runs red tests for twenty-five rounds gets the nudge at ten and the rewind at twenty, and its record holds the failure line; a task whose tests improve every fifth round never sees either.

**Cost.** One line in the Situation. **Risk.** A wrong signal counts as progress, so the signals are kept strict; a rewind mid-change loses the model's thread, which the half-drop at round 100 tonight showed it survives. **Verdict.** Built. A sixth signal was needed the moment the tests ran: a read of something not read before counts, or a research task that reads a different file every round would be stopped at forty.

### 3. Check the world after every change: a syntax check on every write, and the real error line in every failure

**What.** After every `write` or `edit`, the harness runs the language's own checker (`node --check`, `gofmt -l`, `python -m py_compile`, `tsc --noEmit` when present) and says so on the result line: "parses". A file that does not parse is a failure line whose cause is the checker's first line. When the tests go red, the failure's cause is the first assertion line of the first failing test, word for word, not "the change to engine.js before the run".

**Why.** SWE-agent measured a linter on every edit at three points on SWE-bench; Aider found inflexible edits gave "a 9X increase in editing errors". Played forward: tonight's path bug would not have been caught by a parser, but the four failure lines run 5 holds all read "after changing X", which tells a model that has been rewound nothing; the assertion line would have. Idea 1 makes this cheaper, because the tests already run after the write.

**How.** A table of four checkers keyed by file extension, under the tool time limit, output cut to one line. The test-state reader keeps the first assertion line beside the failing names.

**Test.** A write with a syntax error yields a result line saying so and a failure with the checker's line as cause; a red run yields a failure whose cause is the assertion.

**Cost.** Under a second per write. **Risk.** A checker missing on the machine; then the line says "no checker for .rs". **Verdict.** Built, the checker half: `node --check`, `py_compile` and `gofmt -e` run after every write or edit and their line rides on the result, and a file that does not parse is not tested. The assertion line as a failure's cause is not built yet.

### 4. Done means proof a person would accept, and the model may ask its own page

**What.** Two halves. A task that changed a page the person will open (an `.html` file, or a server it started) cannot pass its done-check until the record holds a browser action on that page after which something changed, and a page read with no page errors. And a new field on `browser_read`, `ask`, takes one expression the page evaluates, allowed only on `localhost`, `127.0.0.1` and `file://` pages: "window.game.state" comes back as one line. On any other page the field is refused, because the browser holds the person's logins and the design says never scripts.

**Why.** Tonight had 62 green unit tests and a game that did not start. The honest click judge now reports "nothing changed" for that click, but a canvas game that starts correctly also shows the judge nothing new, only elements gone, so "expectation met" cannot be the gate; "something changed and no errors" can. And in task 2 tonight the model, once pointed at the folder, drove headless Chrome through the shell to read `window.__app.state` and fixed the bug in six minutes. It reached for the right signal; the tool did not offer it. Anthropic's long-running harness requires end-to-end checks and names "declaring victory prematurely" as the failure.

**How.** One rule in the done-check, keyed off `filesChanged`; one optional field on the read tool, refused by address outside the local set.

**Test.** A task that writes `index.html` and marks "the game is playable" with a shell result is sent back; with a browser action that changed the page and an errorless read it passes; `ask` on a `https://` page is refused naming the rule.

**Cost.** One browser round per task that ships a page. **Risk.** Evaluating on a local page the model itself wrote is the model reading its own work; the address rule keeps it there. **Verdict.** Build first.

### 5. Start every task oriented: a snapshot of the world

**What.** Before the first model call of a task, the harness runs one compound command in the work folder and writes the answer into the Situation: the folder listing two levels deep, `git status --short` when there is a repository, the test runner the folder uses, and whether the last recorded test run was green. For a job's task, the job summary already carries every earlier task's report; the snapshot adds what is on disk now.

**Why.** Meta-Harness (Mar 2026) found this snapshot alone beat the best hand-built harness on Terminal-Bench. Played forward through run 6, where the hand-off gives ten job tasks: each would otherwise spend its first two or three rounds on `ls` and `cat package.json`, thirty rounds across the job, eight minutes. Task 2 tonight went looking for the game under `~/Code` for want of one line.

**How.** One shell call at task start, bounded at 60 lines, written by the harness under "Situation:".

**Test.** A task started in a folder with a `package.json` and a `test` folder has both named in its first request.

**Cost.** About 200 tokens, once per task. **Risk.** None new. **Verdict.** Build first.

### 6. The twenty-task set, run nightly, with the four numbers

**What.** Twenty real tasks with a pass or fail check each, from tiny (rename a function) to the Tetris build, run nightly on the local model by a scheduled job. For each: rounds, minutes, output tokens, cache hit rate, and the outcome, written to `docs/PROGRESS.md`. Every failed live task becomes the twenty-first.

**Why.** Anthropic's research team found about twenty test queries enough to see the biggest problems. Two ideas in this plan's first draft were wrong until run 5's log was counted; nothing above can be called an improvement without a number before and after, and the window size question is settled only this way.

**How.** The functional harness and the scripted model exist; this is a folder of asks and checks, one cron job, and the counting script that produced the table above.

**Test.** It is the test.

**Cost.** One card for an hour a night. **Risk.** The set drifts toward what passes; every live failure goes in. **Verdict.** Built, the measuring half: `scripts/runreport` prints one task's numbers off the log, and `docs/PROGRESS.md` carries run 5 task by task. The nightly set of asks is not built yet.

### 7. Put what matters last, and the cost line first

**What.** The last thing the model reads before it writes is today the cost line ("this turn: 69k tokens in..."). Reorder the tail so it ends with the plan's first unmarked step, the newest result line, and the model's own "where the work stands"; the cost line and the header move to the top of the tail.

**Why.** Models read the end of a context best ("Lost in the Middle", 2023); working memory holds about four chunks (Cowan, 2001); Manus rewrites its to-do list at the end of the context on every step. The four chunks that matter already sit in the tail; only their order is wrong, and the least useful line holds the best seat.

**How.** A reorder in the working-context builder; no new tokens.

**Test.** The context test asserts the last lines are the step, the result, and the standing line.

**Cost.** None. **Risk.** The model parrots the last line; the forty-step fixture on Qwen decides. **Verdict.** Build second.

### 8. "Unchanged since r3": do not answer the same read twice

**What.** For the pure tools only, `read`, `search`, `browser_read`, `web`, the harness keeps a fingerprint of each result. A repeat of the same call whose answer fingerprints the same is answered in one line: "unchanged since r3; read r3 to see it again". After any write or edit, the read runs.

**Why.** Build tools have decided staleness by fingerprint since Make. Run 4's four reads of one file were four two-thousand-token answers with nothing new; run 5 repeated a read twice in 150 rounds, so this is a stall signal more than a saving now, and idea 2 counts it.

**How.** A hash of the result text on the result line, checked on the same call signature. Shell and every browser action are never short-circuited.

**Test.** Two identical reads of an unchanged file: the second is the one-line answer; an edit in between makes the third read run.

**Cost.** None. **Risk.** A model that needed the text again pays one `read r3` round. **Verdict.** Tried and dropped on 5 September: the forty-step fixture re-reads a page twice on purpose and expects the text both times, and the repeat guard tells a stall from a poll by comparing result fingerprints, which a one-line answer defeats. The guard already runs a repeat twice and refuses the third, so the saving was one result's text, once. YAGNI.

### 9. Assert the cache in the fixture, and count failures per tool

**What.** The forty-step fixture asserts that from round three on, at least 80 percent of input tokens were cache reads on the local model. `/status` gains a line of tool failures per tool for the running task, and the review is told the worst one.

**Why.** Manus: the hit rate is "the single most important metric for a production-stage AI agent". A June 2026 post traced months of full re-reads on llama.cpp to one changing header. The measurement exists; a test keeps it from drifting. Cursor counts errors per tool and drives each to "two or often three nines".

**How.** One assertion in the live fixture; one map on the run.

**Cost.** None per round. **Risk.** None. **Verdict.** Build second.

### 10. A git commit at every green, opt-in

**What.** When a test run comes back green, the harness commits the work folder with a message naming the checkpoint. Behind one setting, off by default for folders that are not already repositories. The rewind of idea 2 can then put the files back to the last green commit as well as the conversation.

**Why.** "Coherence Collapse" (Mar 2026): 60 to 69 percent of code-agent failures reach the right code and then destroy it, and a checkpoint after each edit recovered every case. Played forward: run 4's stall was on red tests with new test files, so a restore to green would have removed the tests it was trying to pass; the win is for the thrash-after-green case, which our runs have not shown yet. Hence opt-in and second.

**How.** One shell call after a green tests line, behind one setting.

**Test.** A scripted green run leaves a commit whose message names the checkpoint; a rewind restores the file.

**Cost.** One command per green run. **Risk.** Commits in a folder the person did not expect; the setting and the message make it visible. **Verdict.** Build second.

### 11. Proofs a person can read: `[v]` and `[o]`

**What.** In every report and `/tasks N`, a done line the harness checked itself (a test count, a browser change, a file that parses) is marked `[v]`; a line the model judged is marked `[o]`. The person can challenge an `[o]` line by its result id and the harness re-runs that one check.

**Why.** A Bitcoin light client checks a payment from a handful of hashes, not the chain. Tonight the person could not tell from "done" what existed on disk. Model judges spot false success barely better than a coin.

**How.** One field on a done line, set by the harness when the result behind it is one it computed.

**Cost.** Two characters a line. **Risk.** None. **Verdict.** Build third.

### 12. Turn the model's eyes on

**What.** The daemon reports vision off because the GGUF is loaded without its projector file. With the projector on the start line and an image path through the provider, the model reads screenshots, and the visual QA phase of a game build becomes real.

**Why.** Qwen 3.8 sees; our daemon does not load the half that does. A screenshot is the one proof a canvas game can give that no page outline can; idea 4's `ask` field covers the state, not the look.

**How.** The projector file and one flag on the daemon start line, which is the owner's call; and an image field on the tool result and the request, sent as the OpenAI image content part.

**Cost.** A few thousand tokens per screenshot; more video memory. **Risk.** Card memory; the daemon has been OOM-killed once this week. **Verdict.** Decide with the owner, then build.

## The order

**First, this week:** ideas 1 to 6. Together they fold the wasted rounds, close the stall, tell the truth after every change, refuse a false done, start every task oriented, and measure all of it.

**Second:** ideas 7 to 10.

**Third:** idea 11, then 12 with the owner.

**After every step:** run the Tetris ask on the local model and write rounds, minutes, output tokens and cache hit rate into `docs/PROGRESS.md`. The number that matters is rounds to a playable game with a browser change and no page errors, not rounds to green tests.

## Follow-ups found in run 5, not yet built

- **A command whose own process has ended but whose background child holds the output.** Twice tonight the model started a server with an ampersand and no redirect, the shell tool waited on a pipe the server held open, and three rounds went to polling and killing. The tool should say "the command finished; something it started still holds its output and is kept as p49". It needs the process id across the sandbox contract, so it waits for a quiet moment.
- **The `ask` field on `browser_read` for local pages.** Built: one expression, answered as JSON on a page served from this machine or a file, refused anywhere else, with the browser skill telling the model to expose its app's state on `window` and ask.
- **The step mark on the first line.** Idea 1's second half. Held back on purpose until the nightly set can measure whether a small model adopts it; the same model ignored the batching instruction for 150 rounds.
- **A cut-off task ends stopped, not failed.** Built after the 20:33 restart abandoned ninety rounds of a play-test task's record: a shutdown or a turn limit now puts a job's task down with its record, and the next message picks it up.
- **A cancelled call never falls through the model chain.** Built; every stop used to write three lines of fallback noise.
- **Vision.** The projector file this machine points at is a broken link into a removed Ollama store. It needs a download of the projector for this model and one flag on the daemon's start line, which is the owner's call, before the harness's image path is worth building.

## What was rejected, and why

- **Summarizing or compacting the conversation.** Measured to lose 30 points against plain truncation (TRACE, Aug 2026). The half-drop plus `read r7` is truncation with a way back.
- **A smaller window as a build.** Real quality evidence, but a measured three-minute penalty per drop on this daemon, and the hand-off gives small windows for free. Measure it with idea 6; do not guess.
- **Capping the results list to shrink the tail.** The tail is 1,200 tokens; the saving is under two seconds a round. Not worth a rule.
- **A bigger window.** The evidence says the model reasons worse past 32,000 tokens.
- **Multi-agent or planner-and-worker splits.** Fifteen times the tokens (Anthropic); hurt on easy tasks; models under 10B could not run them. One loop owns the record.
- **Tool masking by phase.** Eighteen tools worked; the model picked well. Build it when idea 6 shows a selection error rate.
- **A grammar on the tool-call span.** The repair layer reads every shape a model writes; the "constraint tax" research says whole-reply grammars make open models stop calling tools. Measure malformed calls first.
- **Thinking mode at chosen moments.** The daemon bakes thinking off and the benchmark was won that way. One experiment on the done-check, after idea 6 can measure it.
- **A state-root hash, a pure reducer, sequence-numbered writes.** Elegant; they fix nothing that failed. Later.
- **A commander's-intent line, a traps list, a scored memory.** Cheap and unmeasured; the review already saves lessons. Later, with idea 6.
- **Fuzzy plan-step matching.** Marks survive a rewrite when the text is the same; a rewrite that changes the words is a new plan.
- **Job tasks inheriting the job's failures.** The job summary carries every task's report; the memory hint carries the lessons. Revisit if a second task repeats the first's mistake.

## The one-page summary

- The record is right. Keep it.
- A round is 16 seconds of model; 40 percent of them did no work. Fold them.
- Tell the model the truth after every action.
- Count progress, not calls, and rewind before you stop.
- Done needs proof a person would accept, and the model may ask its own page.
- Start every task with a snapshot of the world.
- Put what matters last.
- Measure everything nightly, or it did not happen.
