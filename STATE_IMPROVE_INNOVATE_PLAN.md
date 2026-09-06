# The plan: what to build next so Nerd Genie finishes hard jobs on Qwen 3.8, every time

Written 5 September 2026, after five Tetris runs on the local Qwen 3.8 27B, two research documents (`STATE_RESEARCH.md`, `LLM_RESEARCH.md`) and the five reports under `docs/research/2026-09-05-state-and-llm/`. Plain English. Every idea below was weighed against what actually broke on this machine, and against two rules: keep it simple (KISS), and build nothing until it is needed (YAGNI).

## The short version

1. The design is right. The record beside the log beside a fresh working context is what every other harness is now backing toward: the field measured summarization and found it loses 30 points, and the cache-hit-rate metric that Manus calls the one that matters is exactly what a fixed record buys. Run 5 tonight reused 40,000 of 41,000 tokens per round.
2. What failed tonight was not the design. It was three lies and two gaps: a click tool that said "worked" when nothing changed, a browser result that never showed the page's errors, a guard that stopped instead of rewinding, a follow-up message that started a blank task, and a job whose tasks never started because the task that made it kept working. All five are fixed, tested and committed today, and run 5 is running on them.
3. The next wins are about **signals and bounds**, not about cleverness. The model is capable. The harness has to tell it the truth about the world, keep its window small, notice when progress has stopped, and never let a claim of done stand without proof a person would accept.
4. Twelve ideas made the cut. Six go first. The rest wait for evidence. A longer list of good ideas was rejected on YAGNI grounds, and the reasons are written down so nobody rebuilds them by accident.
5. Nothing on this list needs a bigger model, a bigger window, a second model, or a rewrite. Each is a small change with a test that is red first.

## What was fixed today, so the baseline is clear

| Failure seen | What was wrong | What changed | Proof |
|---|---|---|---|
| Start button did nothing, tool said "what was expected happened" | The click judge matched a word of the expectation against the clicked button's own name | The clicked element is no evidence. A click that changed nothing says "nothing changed" | `worker/browser/test/actions.test.ts` |
| Script answered 404, nothing told the model | No browser result carried the page's console | Every snapshot carries `errors`: failed loads with status, script errors, first five and a count; printed as `page errors:` | `worker/browser/test/page-errors.test.ts`, `internal/tool/browserread` |
| Task stopped after four identical reads | The guard ended the task on the third refusal | A stall clears the conversation, writes the stall into the record as a failure, three times, then stops | `internal/loop/guard_test.go` |
| "the start game button wont work" started a blank task | Only four words picked a stopped task up | Any message after a stopped or failed task carries it on, as a correction; `/clear` sets it aside. A follow-up on finished work sees the files and where it stood | `cmd/nerdgenie/resuming_test.go`, `test/functional/resume_test.go` |
| Job of ten tasks showed 0 of 10 for an hour | The task that made the job kept working; the job's tasks could not start | The task ends the moment the job is made, with one done line naming the job; the driver takes task one | `internal/loop/handoff_test.go` |

Also in today: plan steps can be marked done (`step_done`), the probe rule (five throwaway scripts with no edit sends one line), the review runs before the wrap-up clock, tool markup is never saved as a lesson, and the message window drops its oldest half at once instead of one message per call.

## How the ideas were judged

Each idea got four questions. Would it have changed tonight's outcome, or the outcome of the sixty-round stall in run 4? Is there evidence outside our own runs? Can it be built as one small change with one test? What does it cost per round in tokens or time? An idea that failed the first two was set aside whatever its elegance. An idea that failed the third was cut down until it passed.

Impact is scored on three things, in this order: reliability (the task finishes and is right), state (the model always knows where it is), and speed (tokens and seconds per round).

## The twelve, ranked

### 1. A progress meter, and a ladder: nudge, rewind, stop

**What.** The harness already knows when real progress happens: a done line proven, a plan step marked, the tests line improving, a browser action whose expectation was met. Count rounds since the last one, and write it into the Situation as one line: "rounds since progress: 14". At fifteen, send one line in the harness's own words. At thirty, rewind: clear the conversation, keep the record, write one failure line with the cause. At forty-five, stop and ask the person.

**Why.** The run-4 stall was sixty rounds on two tests, every probe a character different, so the identical-call guard never saw it. Tonight's rewind only fires on identical calls. The evidence: models "become more likely to make mistakes when the context contains their errors from prior turns", not fixed by size (ICLR 2026); rolling back recovered 45 percent of failed runs against 16 for plain retries (Aug 2026); OpenHands ships a stuck detector by default.

**How.** One counter on the run, reset by the four progress signals, and the rewind that exists. Polling a long command is exempt. Three constants.

**Test.** A scripted task that edits and runs red tests for thirty rounds gets the nudge at fifteen and the rewind at thirty, and its record holds the failure line.

**Cost.** One line in the Situation. **Risk.** A wrong signal counts as progress, so the four are kept strict. **Verdict.** Build first.

### 2. A smaller working window, measured

**What.** Hold the working context near 32,000 tokens. Today the message window keeps 200 messages, which is about 100 rounds and up to 65,000 tokens before the half-drop. Cut it to 100 messages, and measure the effect on rounds to green and on time per round.

**Why.** Open models fall from 79 percent at 8,000 tokens to 61 at 32,000 and 45 at 64,000 (LOCA-bench, Feb 2026); "context rot" was found in every one of 18 models (Chroma, 2025). Our own runs agree: run 4 stalled once the window passed 60,000 tokens, and run 5's hard rounds began after round 90. Smaller prompts also prefill faster on this card, at about 400 tokens a second.

**How.** One constant, `MaxMessagesKept`, from 200 to 100. Results that leave the window stay one `read r7` away, which is the design.

**Test.** The window test already holds the half-drop; the benchmark harness records rounds and minutes to green before and after.

**Cost.** One more cold prefill per fifty rounds, about 75 seconds each. **Risk.** The model reaches for `read r7` more often; that is the point. **Verdict.** Build first, then measure.

### 3. Check the world after every change: a syntax check on every write, and the real error line in every failure

**What.** After every `write` or `edit`, the harness runs the language's own checker (`node --check`, `gofmt -l`, `python -m py_compile`, `tsc --noEmit` when present) and puts the answer on the result line: "wrote main.js, 214 lines, parses". A file that does not parse is a failure line with the checker's first line as its cause. And when the tests go red, the failure's cause is the first assertion line of the first failing test, word for word, not "the change to engine.js before the run".

**Why.** SWE-agent measured a linter on every edit at three points on SWE-bench; Aider found disabling flexible edits gave "a 9X increase in editing errors"; Cursor drives tool reliability by counting errors per tool. Tonight the game did not boot because of one path, and the closest true signal the model had was a 404 nobody showed it. A parse check would not have caught that one, but it catches the most common small-model slip, a broken file, in the same round.

**How.** A table of four checkers keyed by file extension, run under the tool time limit, output cut to one line. The test-state reader already finds failing test names; keep the first assertion line beside them.

**Test.** A write of a file with a syntax error yields a result line saying so and a failure with the checker's line as cause; a red test run yields a failure whose cause is the assertion line.

**Cost.** Under a second per write. **Risk.** A checker missing on the machine; then the line says "no checker for .rs". **Verdict.** Build first.

### 4. Done means proof a person would accept

**What.** A task that changed a page the person will open (an `.html` file, or a server it started) cannot pass its done-check until the record holds at least one browser action whose expectation was met on that page. The `task` tool refuses to mark such a done line with a unit-test result. The last call before done, tools off, asks the model to read the done list as a tester would, and name the result behind each line.

**Why.** Tonight had 62 green unit tests and a game that did not start. Anthropic's long-running harness forbids editing the feature list and requires end-to-end checks; its named failure was "declaring victory prematurely". Strengthening the tests on SWE-bench dropped the top agent 17 points, which says how much weak proof over-credits.

**How.** One rule in the done-check: if `filesChanged` holds a served page, require a result from `browser_click` or `browser_act` with "what was expected happened". The refusal names the rule and the tool to use.

**Test.** A task that writes `index.html` and marks "the game is playable" with a shell result is sent back; the same task with a met browser action passes.

**Cost.** One browser round per task that ships a page. **Risk.** A flaky page makes the check flaky; cap the retries at two and then hand the window to the person. **Verdict.** Build first.

### 5. Start every task oriented: a snapshot of the world

**What.** Before the first model call of a task, the harness runs one compound command in the work folder and writes the answer into the Situation: the folder listing two levels deep, `git status --short` when there is a repository, which test runner the folder uses, and whether the last recorded test run was green. For a job's task, add the job's previous task's report in full.

**Why.** Meta-Harness (Mar 2026) found this snapshot alone beat the best hand-built harness on Terminal-Bench. On this machine, every job task starts with an empty Situation and spends its first rounds on `ls` and `cat package.json`. Task 2 tonight went looking for the game under `~/Code` for want of one line.

**How.** One shell call at task start, bounded at 60 lines, written by the harness under "Situation:". The recent-work line already carries the folder for a person's follow-up; this does the same for a job's tasks.

**Test.** A task started in a folder with a `package.json` and a `test` folder has both named in its first request.

**Cost.** About 200 tokens, once per task. **Risk.** None new. **Verdict.** Build first.

### 6. A git commit at every green

**What.** When a test run comes back green, the harness commits the work folder with a message naming the checkpoint ("green at c17.39, 62 tests"). If the folder has no repository and the setting allows, it makes one. The rewind ladder of idea 1 can then put the files back to the last green commit, not only the conversation.

**Why.** "Coherence Collapse" (Mar 2026): 60 to 69 percent of code-agent failures reach the right code and then destroy it, and a checkpoint after each edit recovered every case. Anthropic's harness commits after every green run. Tonight's 62-green state would have been a named commit the follow-up could point at.

**How.** One shell call by the harness after a green tests line, behind one setting, off by default for folders that are not already repositories.

**Test.** A scripted green run leaves a commit whose message names the checkpoint; a rewind restores the file.

**Cost.** One command per green run. **Risk.** Commits in a folder the person did not expect; the setting and the message make it visible. **Verdict.** Build second.

### 7. "Unchanged since r3": do not redo what has not changed

**What.** For the pure tools only, `read`, `search`, `browser_read`, `web`, the harness keeps a fingerprint of each result. A repeat of the same call whose answer fingerprints the same is answered in one line: "unchanged since r3; read r3 to see it again". After any write or edit, the read runs.

**Why.** Build systems have decided staleness by fingerprint since Make. Tonight's four reads of one file were four two-thousand-token answers with nothing new in them; four "unchanged" lines cost forty tokens and are a stuck signal the meter of idea 1 counts.

**How.** A hash of the result text on the result line, checked on the same call signature. Shell and every browser action are never short-circuited, because they depend on the world.

**Test.** Two identical reads of an unchanged file: the second is the one-line answer; an edit in between makes the third read run.

**Cost.** None. **Risk.** A model that needed the text again pays one `read r3` round. **Verdict.** Build second.

### 8. The last line the model reads: where it is and what comes next

**What.** The harness writes one line at the very end of every prompt, below everything else: "Step 3 of 9: build the UI. Last result r41: tests all 62 passing. Next: open the page and click Start." The step comes from the plan's first unmarked step, the result from the newest line, the next from the model's own "where the work stands" line.

**Why.** Models read the start and the end of a context best and the middle worst ("Lost in the Middle", 2023); Manus rewrites a to-do list at the end of the context on every step and calls it the cheapest reliability gain they found. Working memory holds about four chunks (Cowan, 2001); these are the four.

**How.** Twenty tokens, in the tail, below the cache boundary, so the cache is untouched.

**Test.** The working-context test asserts the line is last and names the first unmarked step.

**Cost.** Twenty tokens per round. **Risk.** The model parrots the line; the forty-step fixture on Qwen decides. **Verdict.** Build second.

### 9. Assert the cache in the fixture, and count failures per tool

**What.** The forty-step fixture asserts that from round three on, at least 80 percent of input tokens were cache reads, on the local model. `/status` gains a line of tool failures per tool for the running task, and the review is told the worst one.

**Why.** Manus: the hit rate is "the single most important metric for a production-stage AI agent". A June 2026 post traced months of full re-reads on llama.cpp to one changing header. The measurement exists (the cost line reads `cache_n` from the daemon); a test is what keeps it from drifting. Cursor counts errors per tool and drives each to "two or often three nines".

**How.** One assertion in the live fixture; one map on the run.

**Test.** The assertion is the test.

**Cost.** None per round. **Risk.** None. **Verdict.** Build second.

### 10. Proofs a person can read: `[v]` and `[o]`

**What.** In every report and `/tasks N`, a done line the harness checked itself (a test count, a browser expectation met, a file that parses) is marked `[v]`; a line the model judged is marked `[o]`. The person can challenge an `[o]` line by its result id and the harness re-runs that one check, never the task.

**Why.** A Bitcoin light client checks a payment from a handful of hashes, not the chain. Tonight the person could not tell from "done" what existed on disk. The evidence behind idea 4 applies: model judges spot false success barely better than a coin.

**How.** One field on a done line, set by the harness when the result behind it is one it computed.

**Test.** A done line proved by a tests line prints `[v]`; one proved by a reply prints `[o]`.

**Cost.** Two characters a line. **Risk.** None. **Verdict.** Build third.

### 11. The twenty-task set, run nightly, with a keep rate

**What.** Twenty real tasks with a pass or fail check each, from tiny (rename a function) to the Tetris build, run nightly on the local model by a scheduled job, with rounds, minutes, cache hit rate and the outcome written to `docs/PROGRESS.md`. Every failed live task becomes the twenty-first.

**Why.** Anthropic's research team found about twenty test queries enough to see the biggest problems. Nothing above can be called an improvement without a number before and after, and tonight's five fixes were found by watching one run by hand.

**How.** The functional harness and the scripted model exist; this is a folder of asks and checks and one cron job.

**Test.** It is the test.

**Cost.** One card for an hour a night. **Risk.** The set drifts toward what passes; every live failure goes in. **Verdict.** Build third, and before anything in the "later" list.

### 12. Turn the model's eyes on

**What.** The daemon reports vision off because the GGUF is loaded without its projector file. With the projector on the start line and an image path through the provider, the model reads screenshots, and the visual QA phase of a game build becomes real.

**Why.** Qwen 3.8 sees; our daemon does not load the half that does. Tonight the model tried to launch Chrome to look for itself and was refused. A screenshot is the one proof a canvas game can give that no page outline can.

**How.** Two pieces: the projector file and one flag on the daemon start line, which is the owner's call, not the harness's; and an image field on the tool result and the request, sent as the OpenAI image content part.

**Test.** A screenshot result reaches the model as an image; the fake model records it.

**Cost.** A few thousand tokens per screenshot; more video memory for the projector. **Risk.** Card memory; the daemon has been OOM-killed once this week. **Verdict.** Decide with the owner, then build.

## The order

**First, this week:** ideas 1, 2, 3, 4, 5. Each is a day or less with its test, and together they close the stall, the window, the lie, the false done, and the blank start.

**Second:** ideas 6, 7, 8, 9. Rewinds that restore files, no repeated reads, the recitation line, and the cache assertion.

**Third:** ideas 10 and 11, then 12 with the owner.

**After every step:** run the Tetris ask on the local model and write rounds, minutes and cache hit rate into `docs/PROGRESS.md`. The number that matters is rounds to a playable game with a met browser expectation, not rounds to green tests.

## What was rejected, and why

These came up in the research and were set aside on purpose. The reasons are here so the question is not reopened without new evidence.

- **Summarizing or compacting the conversation.** Measured to lose 30 points against plain truncation (TRACE, Aug 2026). The half-drop plus `read r7` is truncation with a way back.
- **A bigger window.** The daemon allows 262,144 tokens; the evidence says the model reasons worse past 32,000. Idea 2 goes the other way.
- **Multi-agent or planner-and-worker splits.** They cost fifteen times the tokens (Anthropic) and hurt on easy tasks; models under 10B could not run them at all. One loop owns the record. A narrow stateless worker for one well-defined step is allowed later, if idea 11's numbers ask for it.
- **Tool masking by phase.** Eighteen tools worked tonight; Qwen picked the right ones. Build it when the twenty-task set shows a selection error rate.
- **A grammar on the tool-call span.** The repair layer already reads every shape a model writes; the "constraint tax" research says whole-reply grammars make open models stop calling tools. Measure malformed calls first.
- **Thinking mode at chosen moments.** The daemon bakes thinking off, and the benchmark was won that way. Worth one experiment on the done-check alone, after idea 11 exists to measure it.
- **A state-root hash, a pure reducer, sequence-numbered writes.** Elegant, and they would make replay and diffs one-line checks. They fix nothing that failed. Later.
- **A commander's-intent line, a traps list, a scored memory.** Cheap, and unmeasured. The review already saves lessons; the recent-work line already carries the last task. Later, with idea 11.
- **Fuzzy plan-step matching.** Marks already survive a rewrite when the text is the same. A rewrite that changes the words is a new plan.
- **Job tasks inheriting the job's failures.** Jobs hold reports, not failures; the review writes lessons to memory, and the memory hint carries them. Revisit if a job's second task repeats its first task's mistake.

## The one-page summary for the whiteboard

- The record is right. Keep it.
- Tell the model the truth after every action.
- Keep the window small, and measure it.
- Count progress, not calls, and rewind before you stop.
- Done needs proof a person would accept.
- Start every task with a snapshot of the world.
- Commit at every green.
- Never answer the same read twice.
- Put where-we-are at the very end.
- Measure everything nightly, or it did not happen.
