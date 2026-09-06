# Nerd Genie: the research ideas not yet built, and eight bold ones

Written 6 September 2026 from a read-only pass over `NERDGENIE.md`, `docs/NERDGENIE_PLAN.md` (sections 3 to 7), `LLM_RESEARCH.md`, `STATE_RESEARCH.md`, `STATE_IMPROVE_INNOVATE_PLAN.md`, `docs/research/14-doom-loop-and-tokens.md`, `docs/research/10-metrics.md`, `docs/research/12-loop-comparison.md`, and the 5 and 6 September sections of `docs/PROGRESS.md`. Every "built" claim was checked against `ARCHITECTURE.md` and, where the prose there was stale, against the code in `internal/`. Where the code and the prose disagreed, the code won and it is said so.

One correction to the prose worth knowing before the arithmetic. `ARCHITECTURE.md`'s working-context section says the record's work and lessons ride "in the first message below the cache line", before the conversation. The builder (`internal/context/window.go`, `messagesFor`) actually orders the messages: what is known from `USER.md` and `MEMORY.md`, then the pins, then the conversation, then the record's done list, work and lessons, then the results that have left the table, then the memory hint, then the record's header. So the changing half of the record is already in the tail, a step mark costs only the tail, and that is why the measured steady-state uncached count is 1 to 1.5 thousand tokens a round. Two things that do sit before the conversation and rewrite it when they change are the what-is-known block and the pins.

The numbers every estimate below rests on, all from the documents:

| Measure | Value | Source |
|---|---|---|
| Prompt processing on the local daemon | about 500 tokens a second, so 1,000 uncached tokens is about 2 seconds | NERDGENIE.md §8; PROGRESS.md 00:40 on 6 Sept |
| Generation | about 70 tokens a second, so 100 written tokens is about 1.4 seconds | same |
| Uncached tokens a round, steady state, after the checkpoint spacing change | 0.8 to 1.5 thousand | PROGRESS.md, the fourteenth run; the task brief |
| Cache share in steady state | 94 to 98 percent | the task brief |
| Written tokens a round on long tasks | about 350 (run 5: 52,700 over 150 rounds; run 13: 165,200 over 471) | STATE_IMPROVE_INNOVATE_PLAN.md "The numbers"; docs/nightly/2026-09-06-j |
| Seconds a round | 16 to 19 before the spacing change, 5 to 6 after on small tasks | PLAN "The numbers"; PROGRESS.md fourteenth run |
| Rounds on a long task | 100 to 250; a whole game job 471 rounds for 8 of 11 tasks | PROGRESS.md run 5 table; nightly j |
| A rewind (conversation cleared) | a full re-read of about 50,000 tokens, about 100 seconds of prefill, plus the rounds spent finding bearings | the task brief; PROGRESS.md "re-read about six results by their ids" |
| A person's message that becomes a correction | rewrites the record's rules above cache boundary C, so everything below is read again | ARCHITECTURE.md working context; the task brief |
| The half-drop of the conversation at 200 messages | measured at about three minutes per drop under the old 8,192 spacing; not measured under 1,024 | PLAN "What the second check changed"; `MaxMessagesKept = 200` in `internal/loop/run.go` |
| Rounds that only wrote the record | 22 percent in run 5; 93 of 471 in run 13 | PLAN; nightly j |
| Rounds that only ran the tests | 18 percent in run 5; 43 of 203 in run 13 before the tests line moved onto the change's record line | PLAN; PROGRESS.md 07:35 |

---

## 1. Every idea in the documents, and where it stands

Status words: **built** (where), **partly** (what is missing), **rejected** (the reason the documents give), **never tried**, **dropped** (tried and taken out), **n/a** (does not apply to this design). "ARCH" is `ARCHITECTURE.md`. "PLAN" is `STATE_IMPROVE_INNOVATE_PLAN.md`. "LLM" is `LLM_RESEARCH.md`. "STATE" is `STATE_RESEARCH.md`. "DL" is `docs/research/14-doom-loop-and-tokens.md`. "LC" is `docs/research/12-loop-comparison.md`. "NG" is `NERDGENIE.md`, "DESIGN" is `docs/NERDGENIE_PLAN.md`.

### 1a. The design itself (NG and DESIGN §3 to §7)

| # | Idea | Source | Status | Where, or what is missing |
|---|---|---|---|---|
| D1 | History, state and working context kept as three separate things | NG §3; DESIGN §4 | built | `internal/log`, `internal/record`, `internal/context`; ARCH "The record", "The working context" |
| D2 | The record's fixed shape; harness writes the verifiable half, model the judgment half through the `task` tool | NG §4; DESIGN §4 | built | `record.Keeper.Apply` takes only the model's fields; `loop/situation.go` writes the rest |
| D3 | Ask and corrections never rewritten; decision needs a reason, failure a cause | NG §4 | built | record rules; forty-step fixture asserts byte for byte |
| D4 | Done needs a result id; the harness runs a command a done line names in backticks | DESIGN §4 done-check | built | `record/donecheck.go`, `loop/donecheck.go` |
| D5 | Checkpoints every change, `/tasks 17 back 3` | DESIGN §4 | built | one checkpoint per round (`SaveOncePerRound`); `record.Back` |
| D6 | Any failed task becomes a test (replay) | NG §10; DESIGN §4 | built | `internal/replay`, `nerdgenie replay --as-test` |
| D7 | After-action review, four questions, only the last answer kept | NG §4; DESIGN §4 | built | `loop/review.go`, its own three minutes |
| D8 | Stop list: two lines the harness checks itself, the rest by `stop_now` | NG §4 | built | ARCH "The tools"; `TestTheStopConditionFiresAtRoundThirty` |
| D9 | One rule sizes the window; nothing summarized, results shelved and readable by id | NG §7; DESIGN §4 | built | `context/window.go`; `read r7`; offset reads of a past result |
| D10 | Cache boundaries A, B, C; nothing above the record's work rewritten in a task | NG §7; DESIGN §4 | built | `context/builder.go`; `TestMostOfEveryPromptIsWhatTheLastOneAlreadySaid` reads 84 to 87 percent byte share |
| D11 | Cost line every turn from the provider's own numbers | DESIGN §4 | built | `openaistream.go` takes llama-server's `cache_n`; `context/cost.go` |
| D12 | Jobs: one task at a time, a fresh window per task, reports by id, the task that made a job ends at once | NG §4 | built | `loop/handoff.go`, `loop/jobs.go`; run 5's seven verification tasks at 4 to 15 s a round |
| D13 | Memory: two capped files, zero-token capture, three-line hint, everything searchable | NG §9 | built | `internal/memory`; the cap moves facts to dated notes rather than refusing |
| D14 | A task without a budget unless the user sets one; a forced final answer when a set budget ends | DESIGN §3 rule 3 | built | `loop/budget.go` |
| D15 | Per-turn tool cap of 8 for local models, 20 for cloud | DL §5.1 part 4 | rejected | DESIGN §3 rule 3 chose "no cap unless the user sets one"; the meter and the same-call guard bound the loop instead |
| D16 | The model orients before it acts; the first line says where the work stands | DESIGN §3 rule 2 | built | first line read by `loop/firstline.go`; the orient line written into the situation |
| D17 | Vision: the model's eyes on | PLAN idea 12; NG §8 | built 6 Sept | `--mmproj` on the daemon; `browser_screenshot`; newest two pictures kept; 799 tokens a screenshot |
| D18 | Checkpoint spacing 1,024 on the daemon | LLM idea 1; PROGRESS 06:30 | built 6 Sept | `--checkpoint-min-step 1024 --ctx-checkpoints 16`; uncached tokens fell from 3.6 to 5.0k to 0.8 to 1.0k a round |

### 1b. LLM_RESEARCH.md, twelve ideas and their parts

| # | Idea | Source | Status | Where, or what is missing |
|---|---|---|---|---|
| L1a | Freeze the front; changing parts at the very end; recent messages append-only | LLM idea 1 | built | `context/window.go` order; done list moved to the tail 5 Sept (`donelisttail_test.go`) |
| L1b | One recitation line at the end: "step 3 of 5, next: confirm the compose box" | LLM idea 1; STATE 14; PLAN idea 7 | never tried | the tail still ends with the results list, the hint and the header (cost and budget); `recit` has no hit in ARCH or `internal/` |
| L1c | Fail a `make live` run on more than one full re-read per task | LLM idea 1 | never tried | the fixture asserts byte share, not the live daemon's `cache_n` |
| L2a | Read the server's `cache_n`/`prompt_n` into the cost line | LLM idea 2 | built | `openaistream.go`, `openaitimings_test.go` |
| L2b | The fixture asserts at least 80 percent cache reads from round three | LLM idea 2; PLAN idea 9 | partly | `TestMostOfEveryPromptIsWhatTheLastOneAlreadySaid` fails under 80 percent byte share at rounds 10, 20, 30; nothing asserts the live daemon's number in `make live` |
| L2c | Tool definitions byte-identical and sorted; budget line only when a budget is set | LLM idea 2 | built | boundary B; the budget line rides in the header only when set |
| L3a | Never believe a tool: a click with no visible change fails; page errors on every browser result | LLM idea 3; STATE 4 | built | `worker/browser` diff with `newText`, second look at 700 ms, `page errors:` on every snapshot |
| L3b | A computed difference on every world-changing result: "0 files changed", "tests 62/62 to 62/62" | LLM idea 3 | partly | tests after a change ride on the write's line and the record's line; syntax check on every write; shell results carry no before/after difference and edits carry no "files changed" count |
| L3c | A done line whose only proof is a tool's own claim stays unproven | LLM idea 3; STATE 6 | never tried | the done-check takes any result id the record reached as proof (`TestAResultThatLeftTheListStillProvesTheLineItWasPinnedTo`); no proof classes |
| L4a | Progress meter; nudge at N, rewind at 2N, stop at 3N; polls exempt | LLM idea 4; PLAN idea 2 | built | `loop/progress.go`: 10, 20, then 20 more; six progress signals including "a look whose answer the task has not seen" |
| L4b | The nudge worded differently each time (Manus "structured variation") | LLM idea 4; LLM "What Manus says" | never tried | two fixed lines, `TheStallLine` and `TheStallLineAfterAFailure`; the second names the failure's cause |
| L4c | Rewind drops only the looping results from the table and keeps the rest | LLM idea 4 | partly | `guard.go rewindIfDue` clears the whole conversation and leaves `TheRewindLine`; the record stands; three rewinds allowed |
| L4d | A git commit after every green test run to go back to after a thrash | LLM idea 4; PLAN idea 10; STATE 11 | never tried | `git` appears in `internal/loop` nowhere; PLAN ranks it "build second", opt-in |
| L5a | The `task` tool refuses to delete or reword a done line | LLM idea 5 | never tried | the done list can be rewritten by a later `task` call; only the ask and corrections are locked. A pin rewrites the whole list (PROGRESS, sittings 6 to 9) |
| L5b | A line about user-visible behaviour needs a browser action with a difference, never a unit-test id | LLM idea 5; STATE 12; PLAN idea 4 first half | never tried | `loop/donecheck.go` has no page rule; PLAN said "build first" but only the `ask` field half landed |
| L5c | One last call, tools off, reviewing the done list as a tester and a user would | LLM idea 5 | never tried | the review runs after the done-check and asks the four questions, not "would you accept this" |
| L6 | Working context held at about 32,000 tokens; daemon at 65,536 | LLM idea 6 | rejected as a build | PLAN "A smaller window as a build": a measured three-minute penalty per drop, and the hand-off gives small windows free; the daemon runs at 131,072 now so the projector fits (PROGRESS 09:30) |
| L7a | Fixed caps, head and tail, a pointer to the rest | LLM idea 7 | built | one read at most 16 kB or 400 lines with offsets; search capped at fifty rows with "narrow the pattern or the path" (`internal/tool/search/search.go:204`); overflow to a file |
| L7b | A failure's cause is the real error line, word for word | LLM idea 7; PLAN idea 3 second half | never tried | PLAN: "The assertion line as a failure's cause is not built yet"; red runs after an edit write "the change to X before the run" as the cause. The Jest header names are now read into the summary line (PROGRESS 14:15), which is the raw material |
| L7c | Failures per tool and per model in `/status` | LLM idea 7; PLAN idea 9 | partly | `scripts/runreport` counts refusals by tool for the report; `/status` does not carry them |
| L8a | The model writes a gist of at most 25 words as a result goes to the shelf | LLM idea 8 | never tried | result lines are the tool's own first line cut to seventy characters; no gist field on the `task` tool |
| L8b | Round and clock time stamped on every result line | LLM idea 8 | never tried | lines carry a label and a summary only |
| L8c | `read` by line range; `search` inside a result by id; `read r7 digest` | LLM idea 8 | partly | a past result reads from an offset like a file (PROGRESS, sitting 3); `search` takes files only; no digest |
| L9a | Thinking off by default, on with a small budget at three moments | LLM idea 9 | partly, and the automatic half rejected | `/think` sets one of six levels by hand for the model in use (ARCH `think.go`); PLAN: "One experiment on the done-check, after idea 6 can measure it" |
| L9b | No echoes: forbid re-quoting files, prefer small edits | LLM idea 9 | built in effect | run 5 had 0 whole-file rewrites of 19; the cut-off rule says a long file goes in parts of at most 300 lines |
| L9c | Prompt-lookup drafter beside MTP; read the acceptance rate | LLM idea 9 | never tried | daemon-side; nothing in the repository or `docs/nightly` records an acceptance rate |
| L10a | Reason in plain text first, constrain only the call | LLM idea 10 | built | the first line, then the calls; the repair layer reads every shape |
| L10b | A grammar on the tool-call span | LLM idea 10 | rejected | PLAN: the repair layer covers it; "constraint tax" research; measure malformed calls first |
| L10c | Fuzzy `edit` with a high threshold, plus a syntax check | LLM idea 10 | partly | the syntax check after every write and edit is built (`loop/syntax.go`); `edit` still needs the exact span |
| L10d | Mask tools by phase, keep all definitions cached | LLM idea 10 | rejected | PLAN: "Eighteen tools worked; the model picked well. Build it when idea 6 shows a selection error rate" |
| L11a | A snapshot of the world at task start | LLM idea 11; PLAN idea 5 | built | `internal/orientation`: folder entries, listening ports, newest two results on a pick-up; placed before the ask so the ask stays last |
| L11b | Re-run the job's last passing check so a task never builds on a broken base | LLM idea 11 | never tried | orientation says what is on disk and listening, not whether the last test run is still green |
| L11c | Copy the job's failures and decisions into the new task's record | LLM idea 11 | rejected | PLAN: the job summary carries every report; the hint carries the lessons; revisit if a second task repeats the first's mistake |
| L11d | Worker calls with no record access, their text a result id | LLM idea 11 | rejected | PLAN: multi-agent or planner-and-worker splits, fifteen times the tokens, hurt on easy tasks |
| L12a | Every memory fact points at the raw log event | LLM idea 12; STATE 8 | partly | captured facts carry the event number in their id (`Capture`), review facts carry the task; search returns past messages raw (`msg:` ids); no pointer on a review's fact to the reply it came from |
| L12b | A traps list, by tool and site, dropped when proven wrong | LLM idea 12; STATE 8 | rejected for now | PLAN: "cheap and unmeasured; the review already saves lessons. Later, with idea 6" |
| L12c | A nightly twenty-task set on every model; track the keep rate | LLM idea 12; PLAN idea 6 | partly | `scripts/nightly`: four asks with a check each and the Tetris build as the fifth, local model only; no keep rate |

### 1c. STATE_RESEARCH.md, fifteen ideas

| # | Idea | Source | Status | Where, or what is missing |
|---|---|---|---|---|
| S1 | A follow-up is never a blank page: a successor task inherits ask, corrections, stop list, lessons, open lines | STATE 1 | partly | any message after a stopped or failed task carries the same task on, written in as a correction (`resuming.go`); the recent-work block answers "where are we" on an empty record; the size cap trims result lines instead of closing the task. No successor record; a task that fills its record keeps going with the oldest lines gone |
| S2 | The checkpoint records the last event it reflects; verify in the background | STATE 2 | partly | every checkpoint is itself an event in the log, so it has a sequence number and replay reads events after it; the memory indexer reads "from where the last run stopped, in spans of sequence numbers". No explicit last-event field on the checkpoint and no background verification |
| S3 | A hash per record section; a checkpoint chained to its parent | STATE 3 | rejected | PLAN: "A state-root hash, a pure reducer, sequence-numbered writes. Elegant; they fix nothing that failed. Later." |
| S4a | Re-check every situation line on resume and rewrite it | STATE 4 | partly | on a pick-up the browser fact is taken back from the log's last browser result, not from the browser; orientation lists the folder and ports fresh; the tests fact and command fact come from memory of the run |
| S4b | Compute the difference after every world change | STATE 4 | partly | see L3b |
| S5 | An `expect` field on every tool, checked by plain rules | STATE 5 | partly | `browser_act` and the desktop tool take an expectation and the worker judges it; `shell`, `write` and `edit` have none (no `expect` in `internal/tool/shell`, `edit`, `write`) |
| S6 | `[v]` and `[o]` on every done line; challenge an `[o]` by id and re-run one check | STATE 6; PLAN idea 11 | never tried | no hit in ARCH; PLAN ranks it "build third" |
| S7a | A supervisor tree with a restart limit at each level | STATE 7 | partly | provider retries three; a crash breaker; a job pauses after three failed tasks; three rewinds; a cut-off reply sent back twice; "steps 1 to 4 keep their ids and later steps are re-planned" holds only when the rewritten plan's text is unchanged |
| S7b | After a step fails twice, the simpler task is to split it | STATE 7 | never tried | the stuck-test line names three ways out but never proposes a split |
| S8a | The last task's report first in the hint | STATE 8 | built differently | the recent-work block rides above C on a turn whose record is empty; the hint itself stays a search |
| S8b | Raw text behind every fact | STATE 8 | partly | see L12a |
| S9 | One pure reducer applies every change | STATE 9 | rejected | see S3 |
| S10 | "Unchanged since r3": fingerprint pure reads | STATE 10; PLAN idea 8 | dropped 5 Sept | the fixture re-reads on purpose; the guard tells a stall from a poll by result fingerprints, which a one-line answer defeats; the saving was one result's text once |
| S11a | Checkpoints as a tree; a rewind is a branch | STATE 11 | partly | a rewound record never reuses a label the abandoned path used, and abandoned checkpoints stay in the log; no branch shown on the panel |
| S11b | The files get a checkpoint: a git commit at every green | STATE 11; PLAN idea 10 | never tried | see L4d |
| S12a | The done check is DO-CONFIRM at the pause before "done" | STATE 12 | built | the done-check with proof ids |
| S12b | The review asks whether a stop line that never fired should be dropped | STATE 12 | never tried | no such question in `loop/review.go` |
| S12c | Proof classes: user-visible lines need an end-to-end result | STATE 12 | never tried | see L5b |
| S13 | Commander's intent: "If the plan breaks: try one different approach, then ask" | STATE 13 | rejected for now | PLAN: "cheap and unmeasured; later, with idea 6" |
| S14 | Seven chunks; the four that matter last; a final harness line naming the step and the next step | STATE 14; PLAN idea 7 | never tried | see L1b; the caps of five done lines and ten plan steps are the chunk limits |
| S15 | Number every state; refuse a write built on a stale checkpoint | STATE 15 | rejected for now | see S3; a mid-turn correction lands after the current tool call, so the case it guards is narrow |

### 1d. STATE_IMPROVE_INNOVATE_PLAN.md, the twelve and the follow-ups

| # | Idea | Status | Where, or what is missing |
|---|---|---|---|
| P1 | Tests run themselves after a write; a step mark on the first line | built | `loop/autotest.go`, `loop/firstline.go`; the tests line leads the change's record line since 07:35 on 6 Sept |
| P2 | Progress meter and ladder | built | `loop/progress.go` |
| P3 | Syntax check on every write; the real error line as a failure's cause | half built | checker yes (`loop/syntax.go`); assertion line no |
| P4 | Page proof in the done-check; the `ask` field | half built | `ask` yes (`browser_read`, local pages only); the done-check rule for a changed page no |
| P5 | Start oriented | built | `internal/orientation` |
| P6 | The nightly set with the four numbers | built at four plus one | `scripts/nightly`, `scripts/runreport`, `docs/nightly/` |
| P7 | Put what matters last, the cost line first | never tried | the tail order in `messagesFor` ends: record body, results list, hint, header |
| P8 | "Unchanged since r3" | dropped | see S10 |
| P9 | Assert the cache in the fixture; count failures per tool | partly | byte-share test yes; live `cache_n` assertion no; refusals per tool in the run report, not `/status` |
| P10 | A git commit at every green, opt-in | never tried | "build second" |
| P11 | `[v]` and `[o]` | never tried | "build third" |
| P12 | Eyes on | built 6 Sept | see D17 |
| F1 | A command whose background child holds the output: "finished; p49 holds the pipe" | never tried | PLAN follow-ups: "needs the process id across the sandbox contract, so it waits for a quiet moment" |
| F2 | Whether 300 lines is the right part for a cut-off file | to measure | PROGRESS 05:20 |
| F3 | What the play-test does with eyes; the model's judgment of a picture "is the weak link" | to measure | PROGRESS 09:25, 11:39 |
| F4 | The twenty-five-second click that did not reproduce outside the run | open | PROGRESS 08:45 |
| R1 | Summarizing or compacting | rejected | TRACE: 30 points lost against truncation |
| R2 | Capping the results list to shrink the tail | rejected, then done anyway for another reason | PLAN said under two seconds; the list went from a hundred lines to forty once the daemon's timings showed the tail was four seconds a round |
| R3 | A bigger window | rejected | worse reasoning past 32,000 |
| R4 | Multi-agent, planner and worker | rejected | fifteen times the tokens; one loop owns the record |
| R5 | Tool masking by phase; a grammar; thinking at moments; hashes and reducers; intent line, traps, scored memory; fuzzy plan-step matching; job tasks inheriting failures | rejected, most "later, with idea 6" | PLAN "What was rejected" |

### 1e. docs/research/14-doom-loop-and-tokens.md, the recommendation in §5

| # | Idea | Status | Where, or what is missing |
|---|---|---|---|
| G1 | Identical-call detector keyed on canonical arguments, across steps, third call refused | built and extended | `loop/guard.go`; the fingerprint leaves out intent, expectation, why, goal and reason (PROGRESS: eleven desktop launches each worded differently) |
| G2 | Stop after three identical tool outputs in a row | built in a different form | `IdenticalCallWindow` compares result fingerprints to tell a stall from a poll |
| G3 | Malformed-call catcher: name repair, catalogue back, "that was data, not a call" | built | `internal/repair`; ARCH "Tool-call repair" |
| G4 | Text tool-call repair: fences, `<tool_call>`, Harmony markers; two tries then accept the text | built | same; a cut-off reply is sent back twice and heard the third time (`loop/cutoff.go`) |
| G5 | Per-turn tool cap 8 local, 20 cloud, with a two-line "answer in text" message | rejected | see D15 |
| G6 | Every guard message ends with the same three options | built | DESIGN §3 rule 6; and one exception on purpose, the open-plan refusal, because "answer the user" closed a task with no game file |
| G7 | Idle watchdog; retry empty output twice | built | a hung tool killed after seven minutes; provider retries; the watchdog restarts the service |
| G8 | Fixed prefix under 2,000 tokens; 6 to 8 core tools; a `tool_search` bridge for the rest | rejected | all eighteen tools are shown to every model; the instruction text is held under five hundred words instead |
| G9 | Tool result cap 4,000 characters on local | built at a different size | 16 kB or 400 lines a read |
| G10 | Compaction at 50 percent; small model for summaries | rejected | R1 |
| G11 | Cache layout: stable system, boundary, volatile, last two messages | built | boundaries A, B, C |
| G12 | The 74-word output-style snippet | built | the "How to write" paragraph of the instruction text |
| G13 | Twelve shared commands, one registry for terminal and Signal | built | `internal/command`; ARCH "The commands" |
| G14 | OpenCode's `invalid` tool, "continue on unknown finish reason" | built in effect | the repair layer returns the error as a result; the loop reads the finish before the parse |

### 1f. docs/research/12-loop-comparison.md, §4 the loop and §5 learning

| # | Idea | Status | Where, or what is missing |
|---|---|---|---|
| C1 | Accept and ack; a writer claim so a superseded run cannot commit | built | queue on disk; a job store where two processes cannot claim one job |
| C2 | Steer at tool boundaries, never interrupt by default | built | DESIGN §3 rule 1; `loop/midturn.go` |
| C3 | Prompt in three tiers with caps and cache points | built | A, B, C |
| C4 | Re-read context files from disk every step | built | `SOUL.md`, `USER.md`, `MEMORY.md` read on every build |
| C5 | Repair text tool calls; validate arguments; error as a result | built | G3, G4 |
| C6 | Admit the batch; parallel and sequential segments | n/a | the local model never batches (0 of 150 in run 5); calls run in order |
| C7 | One permission function, last match wins, default ask | built, default allow | ARCH "The permission function"; the agent runs on its own by default and asks only for the ask-me-first list |
| C8 | Bound every tool: timeout, process group, output to a file | built | DESIGN §3 rule 7 |
| C9 | Taint the turn when a result came from the network; tighten the gate | never tried | no `taint` in ARCH; rule 8's data marker covers the injection half but nothing tightens permissions after a network read |
| C10 | `shouldStopAfterTurn` as one serialized checkpoint | built in spirit | `loop/endings.go`, the done-check and review |
| C11 | Compact at a threshold with a silent memory flush first | rejected | R1; no compaction, so no flush |
| C12 | Delegate without blocking, depth 1 | rejected | R4 |
| C13 | End the turn with a finalizer that may fork a review | built inline | the review runs in the loop, not as a fork |
| A1 | Two memory files with hard caps | built, improved | the cap moves the oldest facts to a dated note instead of refusing |
| A2 | Background review fork every ten iterations | not adopted | the after-action review runs once per task, inline, only for tasks worth reviewing |
| A3 | Harness-state notes with history and rollback, auto-refine gated by a review model | never tried | memory has supersede and `/memory forget`; no prompt-notes layer, no LLM-gated refine |
| A4 | Searchable session history with FTS5 | built | past messages indexed as `msg:` ids in `memory_search` |
| A5 | Agent-authored skills with an index in the prompt; `/learn` | built | the skill list rides in the persona block; the review offers a skill |
| A6 | Silent memory flush before compaction | n/a | no compaction |
| K1 | Skip vector memory, dreaming, pre-reply recall hooks, curator, graphs | followed | none built; the three-line hint is the only per-turn recall and it is empty more often than not |

### 1g. docs/research/10-metrics.md and the 5 to 6 September PROGRESS entries

| # | Idea | Status | Where, or what is missing |
|---|---|---|---|
| M1 | Every number produced by a command shown in the document | built for the harness | `scripts/runreport` reads a task's numbers off the log; `docs/nightly` tables |
| M2 | Prime's design: one tool, a persistent REPL, work kept in variables not the transcript | never tried | eighteen named tools; LC §2 calls the REPL "lossless for the artifacts"; DL notes Prime has no permission gate, which is the reason not to |
| M3 | Tool surface held small: 18 against OpenClaw 57, Hermes 83, OpenCode 12 | built | NG §13 |
| M4 | A poll that waits up to twenty seconds | built 6 Sept | `internal/tool/shell/poll_test.go` |
| M5 | The record's page-level count of hidden-yet-drawn nodes with the one rule that fixes them | built 6 Sept | `internal/tool/browserread`; the per-element mark alone did not move the model, the count with the fix named did |
| M6 | The browser must run on the screen: enforced by the write and edit tools, not only the rules | built 6 Sept | a rule the model can read "is not enough on its own; the tools have to hold it" |

---

## 2. The never-tried and partly-built ideas: evidence for, evidence against, what to measure

Only the ideas that could still be built are listed. The order follows section 1.

| Idea | Evidence for, from the documents | Evidence against or risk | What one nightly run would have to show |
|---|---|---|---|
| L1b, S14, P7: the tail ends with the step and the next step | "Lost in the Middle" (LLM §3); Cowan's four chunks; Manus rewrites its to-do list at the end on every step; the fresh run of 6 Sept wrote no plan when a folder listing was the last thing read and wrote one in every task once the ask was last (ARCH line 212, `freshwindow.go`), which is the same effect measured once already | the model may parrot the last line; the tail is also where the cost line sits and the design wants it visible; forty new tokens a round | record-only rounds and plan rewrites per task against the thirteenth run's 93 of 471; whether the first-line mark adoption rises from 41 |
| L1c, L2b: assert the live daemon's `cache_n` share in `make live` | Manus: the hit rate is the one metric; a June 2026 post traced months of full re-reads to one header; the fourteenth run measured 0.8 to 1.0k uncached, so the number exists | the assertion needs the daemon, so it cannot run in `make test`; drift shows up only nightly | the nightly table gains an "uncached a round" column and a threshold; today it carries the cache share only |
| L3b, S4b: a computed difference on shell, write and edit results | "false success" was 45 to 76 percent of failures (Advani 2026); the click lie cost four re-reads; tests after a change already ride on the line and folded 16 test-only rounds | more bytes per result; a difference on a shell command is hard to define beyond exit code and output change | the count of probe rounds (five-throwaway-script rule firings) and of rounds between a red run and its failure line |
| L3c, L5b, S6, S12c, P4, P11: proof classes, `[v]`/`[o]`, a page rule in the done-check | run 1's 62 green tests and unplayable game; the tenth run closed "done" with no game file; the fourteenth's job of one task finished with nothing built; the light-client argument (STATE 6) | end-to-end checks are slow and flaky; a canvas game changes nothing the outline sees, so "something changed and no errors" is the gate, not "expectation met" | rows in the nightly table that ended "done" while the check failed (three of the last eight runs); `[o]` lines per task |
| L4b: the nudge worded differently | Manus: fixed formats put a small model in a rut; the unattended Tetris run wrote the same failure seven times when the same stall line was read every round | the fix that landed (naming the failure's cause) may already be enough; more wordings are more tests | failures refused as already held per task, after the current fix, against the seven of that run |
| L4c: rewind by cutting the looping stretch, not the whole conversation | rollback recovered 45 percent against 16 for retries (Dubey 2026); each clear today costs about 50,000 tokens re-read and rounds of re-orientation; the half-drop showed the model survives losing old messages | the loop's cause may sit before the cut; a cut list must reach the replay reader | uncached tokens on the first round after each rewind; rounds from a rewind to the next progress signal |
| L4d, S11b, P10: a git commit at every green | Coherence Collapse: 60 to 69 percent of failures destroy a correct patch; a checkpoint after each edit recovered every case; Anthropic's harness commits at green | run 4's stall was pre-green, so a restore would have removed the tests it was writing; commits in a folder the person did not expect | the count of tasks whose test count fell after a green run (thrash after green); whether any rewind could have used a restore |
| L5a: lock the done list once written | Anthropic's feature list "may not be edited"; a pin rewrites the whole list today and once could not pin because the list had changed | the tenth run wrote a done list that was refused and needed rewriting; a lock needs a way to correct a wrong line | done-list rewrites per task in the log |
| L5c: a tools-off tester call before "done" | Krafton's final checklist; Claude Code's stop hook; model judges are weak but the harness-computed items are put in front of it | one round per task; a model that judges its own work leniently | rows that ended done and failed the check, before and after |
| L7b, P3: the assertion line as a failure's cause | SWE-agent: informative feedback moves results; PLAN: run 5's four failure lines all read "after changing X", which tells a rewound model nothing; the Jest header names are now read | a long assertion line against the 120-character lesson cap | rounds from a red run to the fix, and whether a rewound task repeats the same failure |
| L7c, P9: failures per tool in `/status` | Cursor drove per-tool errors to two or three nines; the tenth run had 23 of 62 task-tool calls refused | the number exists in the report already | none needed; a status line |
| L8a, L8b, L8c: gists, time stamps, search and slice by id | "Context Length Alone Hurts": recite the evidence into a short space; the play-test read its 30,000-character script twenty times to find one function | a model-written gist costs 30 output tokens and may be wrong; a harness-written one is free but shallow; the tail grows | repeated reads of the same file per task (`runreport` counts identical reads); result-line length against tail tokens |
| L9a: thinking at three moments | "thinking mitigates self-conditioning" (ICLR 2026) against "overthinking acts less"; a 1,500-token think is 27 seconds | the daemon bakes thinking off and the benchmark was won that way; the switch lives in the chat template so it must sit below the cache line | one experiment: the done-check call at a low think level, rounds and false dones before and after |
| L9c: prompt-lookup drafting on the daemon | LLM §3: MTP gives 1.9 times on this model; a model-free drafter guesses from words already in the prompt, which is what an edit's old-text span and a test name are | acceptance falls as temperature rises; card memory; daemon-side, so the owner's call | the daemon's generation tokens a second on the nightly Tetris, and the acceptance rate in its log |
| L10c: fuzzy `edit` | Aider: "a 9X increase in editing errors" without flexible patching; Qwen2.5-32B malformed 148 of 225 diff edits | the wrong place edited; the syntax check catches breakage only | edit refusals ("span not found") per task in the log |
| L11b: re-run the last green check at task start | Meta-Harness: a snapshot beat hand-built harnesses; a job's next task otherwise learns the base is red on its first write | the suite's own run time at every task start | tasks whose first test run was red |
| L12a: a pointer from every fact to its raw event | "Verbatim Chunks Beat Extracted Artifacts": 16 to 22 points | the review's facts come from a reply, not a tool result, so the pointer is to a message | none; a field |
| L12c: the twenty-task set, the keep rate | Anthropic's team: twenty queries show the biggest problems; the set has four and drifted once (the runner put the folder inside the home) | the set drifts toward what passes; every live failure should go in | the count of asks; tasks that failed live and became asks |
| S1: a successor task with inherited stable parts | Temporal's continue-as-new; a task that fills its record now silently loses its oldest results | carry too much and it is a transcript by another name | how often the results list hits its floor of ten kept lines |
| S2: the checkpoint names its last event; background verification | ARIES, Raft, assumeutxo | the checkpoint already is an event; verification fixes nothing that failed | none unless a replay ever rebuilds a different record |
| S4a: re-check the situation on resume | CRIU's honest page: connections, timers and other programs do not survive a freeze; the fourth sitting lost the browser line | a tab check needs the browser worker up; the tests fact needs a run | tasks picked up whose first browser call failed on a page the situation named |
| S5: `expect` on shell, write, edit | Hearsay-II; the click lie was caught on the fourth re-read and would be caught on the first with an expectation; the desktop tool already asks for one | vague expectations from a small model; ten tokens a call | the share of calls carrying one and the share of those that missed |
| S7b: split a step after two failures | Armstrong: "try to perform a simpler task"; the stuck-test line names three ways out | a split is a plan rewrite, which today costs a round | rounds on one failing test after the stuck line |
| S12b: the review asks about a stop line that never fired | Gawande: killer items only | stop lines are five at most; the review already has four questions | stop lines per task that never fired across a week |
| F1: a background child holding the output | three rounds of polling and killing, twice in one night | needs the process id across the sandbox contract | rounds spent polling a command that had already exited |
| C9: taint the turn after a network read | OpenClaw's turn taint; the data marker covers reading but nothing tightens the gate | the ask-me-first list already gates the dangerous calls | none needed; a security review item |
| A3: a prompt-notes layer with history and rollback | Prime's `harness_state.json`, auto-refine gated by review | Claude Code's warning that bloated instruction files get ignored; the 2026 study that a free-form scratchpad "never helps" | none; skip unless the persona files grow past their caps |
| M2: a REPL as the one tool | Prime's "lossless for the artifacts"; Aider's whole-file reliability for weak models | no permission gate is possible under it; the design's eighteen tools are what the permission function reads | none; a different agent |

---

## 3. Eight big and bold ideas

Each keeps one loop and one record, changes one place, can be measured by the nightly set, and is something a 27B model can follow because the harness does the work. The gains are estimates from the numbers in the table at the top; the measure column is how to find out whether they are true. Three of them come from thinking like somebody else: a film editor (idea 1), a pilot (idea 2), and a database engineer (ideas 5 and 6).

### Idea 1. Rewind by cutting, not by wiping: the film editor's cut

**What it is.** When the meter fires at twenty rounds without progress, the harness today clears the whole conversation and leaves one line, and the model then reads its way back in: about 50,000 tokens re-read and several rounds of `read r37`, `read r42`. A film editor with a scene that does not work cuts the bad stretch and keeps the reel on either side. The rewind should cut only the messages since the last progress signal, keep everything before them byte for byte, and append the rewind line and one failure line.

**Mechanism.** `loop/progress.go` already knows the round of the last progress signal and `loop/window.go` holds the messages as a list. `rewindIfDue` in `guard.go` becomes: find the first message of the round after the last progress, delete from there to the end, append `TheRewindLine` and the failure the model was asked for, and write a `cut` event into the log naming the two message numbers so the replay reader can skip the same stretch. The prefix before the cut is unchanged, so the daemon's checkpoint before it still matches and nothing above the cut is re-processed. The same-call guard's third clearing still ends the task; the cut is what the first two clearings do.

**Expected gain.** A rewind costs about 50,000 tokens at 2 seconds a thousand, about 100 seconds, plus the re-orientation rounds, which the orientation block now halves but which PROGRESS measured at about six reads of eight to sixteen thousand tokens each on the old layout, another two to four minutes. A cut keeps the prefix and costs the rewind line and the record's tail, about 1,500 tokens, three seconds. Task 10 of run 5 had five rewinds, task 11 two, the twelfth run's engine task one that stopped the task. At two rewinds a long task, the saving is four to eight minutes on a 100-minute task, and the model keeps the thread of the rounds that were working.

**Risk.** The cause of the loop may sit before the cut, in a plan step the model wrote thirty rounds ago; the failure line and the ladder's stop at forty cover that, and the record stands either way. A cut inside a tool call and its result pair would be refused on the wire, so the cut lands on a round boundary, which the checkpoint per round already marks.

**Measure in one nightly run.** Add two columns to the nightly table from the log: uncached tokens on the first round after each rewind, and rounds from each rewind to the next progress signal. Run the Tetris ask; compare with the thirteenth run's numbers.

**Cost.** One to two days, most of it the replay reader and its test.

### Idea 2. The landing checklist: the harness runs the killer items before "done", labels every line `[v]` or `[o]`, and asks one tools-off question

**What it is.** A pilot does not land because the co-pilot says the gear is down; the checklist is read aloud and each item is confirmed against an instrument. Today the done-check confirms that every done line names a result id, and any id the record reached counts. Three runs in the last day closed "done" on work that did not exist: no game file, a job of one task, sixty-two green tests and a game that did not start. The checklist makes the harness confirm what it can and say plainly what it could not.

**Mechanism.** Five items the harness checks itself when the model claims done, in `loop/donecheck.go`: every source file changed in the task parses (the syntax checker already ran); the last test run was green and came after the last edit (the test-state reader holds both); a task that changed an `.html` file or started a server has a browser action on that page after which something changed and a `browser_read` with no page errors (PLAN idea 4's unbuilt half); no plan step is open; no stop line has been reported. Then each done line is printed with `[v]` when the result behind it is one the harness computed (a test count, a parse, a page difference, a file that exists) and `[o]` when only the model judged it. Then one call with tools off, the checklist in front of it: "As the person who asked, would you accept this? Answer yes, or name the line and what is missing." A "no" sends the model back to work with that line in the record as a failure; a "yes" ends the task and the report carries the labels.

**Expected gain.** Reliability, not tokens: one extra round per task, eighteen seconds, against whole jobs lost. The tenth run spent 83 minutes and 224 rounds on a job that finished with nothing built; the fourteenth spent two minutes for the same result. The nightly set's Tetris check failed on a "done" job in three of the last eight runs. If the checklist catches two of three, that is two lost nights in eight recovered, about three hours of daemon time a week.

**Risk.** A page that changes nothing the outline sees, such as a canvas game, still fails the page item; the rule is "something changed and no errors", as PLAN idea 4 argues, not "expectation met". The tester call is a model judging its own work; the `[v]` items are what stop it from being the only judge.

**Measure in one nightly run.** The nightly table already has "ended" and "check"; add the count of `[o]` lines per task and the count of tasks the tester call sent back. A run where a task closes done with `[v]` on every line and the check fails is the bug to chase.

**Cost.** Two to three days: the page rule, the labels, the tester call, and one test each.

### Idea 3. Write-ahead intent: an `expect` line on every world-changing call, checked by the harness for free

**What it is.** A database writes what it is about to do before it does it, so recovery can tell what should have happened. The browser and desktop tools already take an expectation and judge it; `shell`, `write` and `edit` do not, so a red test run after an edit becomes a failure whose cause is "the change to engine.js before the run" and a probe script's output is read by the model alone. Every call that changes the world should carry one optional line saying what it should show, and the harness should say whether it did.

**Mechanism.** One optional string field, `expect`, on `shell`, `write` and `edit`. The harness checks it with four plain rules, in order: a test count in the form "tests 62 to 63" or "all passing" against the test-state reader; "exit 0" or "exit N" against the exit code; "contains X" against the output; "parses" against the syntax check. A miss writes "expected X, got Y" as a failure with that cause, which is the assertion line PLAN idea 3 wanted; a hit counts as progress for the meter and is written on the result line as "as expected". An empty field checks nothing. The tool descriptions gain seven words; the instruction text gains one sentence.

**Expected gain.** About ten written tokens a call, 0.15 seconds. Against it: the probe rule fired in run 5 after five throwaway scripts, the play-test spent rounds reading test output to learn what changed, and every red run after an edit costs the model a round to read the runner's output and write a failure. If one round in twelve on a coding task is the model reading a result to learn what an expectation would have told it, that is eight to twenty rounds on a 100 to 250 round task, two to six minutes, and every failure line in the record becomes one a rewound model can act on.

**Risk.** A small model writes vague expectations; anything the four rules cannot read is ignored and says so once. The desktop tool's history shows the trap: it asked for an expectation and then called a method that never checked it (ARCH line 1436), so the test must prove the check ran.

**Measure in one nightly run.** From the log: the share of world-changing calls carrying an expectation, the share of those that missed, and the rounds from a red run to a failure line with a real cause. `runreport` gains three counts.

**Cost.** One to two days.

### Idea 4. The tail ends with the next step: one harness line, and the cost line moves up

**What it is.** PLAN idea 7 and STATE 14, never built. The last thing the model reads before it writes is the record's header: the budget left and what the last call cost, the two least useful lines it holds. Working memory holds about four chunks and a model reads the end of its context best. The tail should end with the four things that matter: the open done lines, the plan's first unmarked step, the newest result, and one harness-written line naming the step and the next one.

**Mechanism.** In `messagesFor`, move the header message to the front of the tail, right after the conversation, and append one new message: "Step 3 of 5: post it. Last: r41 tests: all 128 passing. Next: step 4, confirm the compose box. Open done lines: 2." The harness has every piece already; the line is about forty tokens and changes every round, which is fine, because it sits after everything else that changes. No other layout change.

**Expected gain.** The measured effect of order on this model is already on record: with the orientation block after the ask the model wrote no plan in any task but the first; with the ask last it wrote one in every task (`freshwindow.go`). Rounds that only wrote the record were 93 of 471 in the thirteenth run, 13 plan rewrites among 33 record-only rounds in run 5, and the "where to next" offers inside a job needed a rule of their own. If the line cuts record-only and orientation rounds by a quarter, that is 20 to 25 rounds on a 471-round job, six to eight minutes, for forty tokens a round, about 0.1 seconds.

**Risk.** The model parrots the line as its first line, which costs nothing but reads badly; the forty-step fixture on the local model shows it in one run.

**Measure in one nightly run.** Record-only rounds and first-line marks, both already columns in the nightly table, against the thirteenth run's 93 and 41.

**Cost.** Half a day to one day.

### Idea 5. A fresh window at the cap instead of the half-drop: continue-as-new inside one task

**What it is.** When the conversation reaches 200 messages the oldest half leaves at once, and PLAN measured what follows under the old spacing: five to seven thousand extra uncached tokens a round for about twenty rounds while the daemon's checkpoints land again, about three minutes a drop. Temporal does not trim a long history; it closes the run and starts a new one with the state passed in. The record is that state. At the cap the task should open a fresh window the way a pick-up does: the orientation block, the newest two results in full, the record, and the ask last.

**Mechanism.** `window.go` already halves the list; replace the halving with a call to `openTheWindow` with the pick-up flag, which exists in `freshwindow.go`, and write a `window` event to the log so the replay reader and the panel know. Everything the dropped messages held is in the record's results list and readable by id. No new prose for the model; the pick-up path already proves it can carry on this way (tasks t9 and t10 ran eighteen and twenty minutes on fresh windows with no steer).

**Expected gain.** A half-drop leaves about 100 messages, twenty to thirty thousand tokens, that the daemon must read again from the drop point, forty to sixty seconds, plus the measured settling. A fresh window is the front plus about six thousand tokens, twelve seconds, and rounds after it are faster because the prompt is smaller: run 5's fresh-window tasks ran at four to fifteen seconds a round against nineteen to twenty-six on the long ones. A 250-round task with 350 written tokens and two results a round reaches 200 messages roughly twice. Two drops at three minutes against two fresh windows at under a minute is four to five minutes a long task, and every round after each is cheaper by whatever the smaller prompt saves, which LOCA-bench says is also better reasoning.

**Risk.** The model loses the thread of the last few exchanges; keeping the newest two results in full and the orient line in the situation is what the pick-up path does today. If the model then re-reads six results by id, the saving is gone; that is the number to watch.

**Measure in one nightly run.** Uncached tokens a round for the twenty rounds after each cap event, rounds from the event to the next progress signal, and reads of past results by id in those rounds. Under the new 1,024 spacing the half-drop's cost should be re-measured first; if it is now under a minute, this idea waits.

**Cost.** One day.

### Idea 6. Corrections ride in the tail until the next cold window: steering that keeps the cache warm

**What it is.** A correction from the person goes into the record's rules, which sit above cache boundary C, so every byte below C is read again on the next call: the whole conversation, twenty to fifty thousand tokens, forty to a hundred seconds. It also arrives as a message at the end of the conversation, where the model reads it best anyway. A database engineer would never rewrite the header page for a row that can be appended. The rules are only ever added to, so the addition can live at the tail for the rest of the sitting and be folded into the front at the next fresh window, when the cache is cold anyway.

**Mechanism.** `splitRecord` in `context/recordsplit.go` cuts the goal and rules for the stable block. Give the keeper a mark of which corrections were held when the window was opened; those print in the stable block, and any added since print in the record body at the tail under the same `Corrections:` label with the same `C4` numbering. A fresh window, a rewind, a pick-up, or the next task folds them all into the front. The record on disk is unchanged; only the printer's cut moves. The instruction text needs no words: the model reads the same labels in both places.

**Expected gain.** One full re-read of the conversation avoided per correction: at thirty thousand tokens, sixty seconds. The steered runs had one to three corrections each (the play-test's two steers, the fresh build's one message with two facts). One to three minutes per steered task, and the answer to the person's steer comes back sooner, which matters more than the minute when a person is waiting at the keyboard.

**Risk.** A correction at the tail for a sitting of a hundred rounds sits after the results list rather than under the ask; the design's principle is "the request in the words of the person who gave it, first". The ask stays first; only the new correction waits. If the model misses a tail correction that it would have honoured at the front, the fixture's correction at round twelve is the test that shows it.

**Measure in one nightly run.** Uncached tokens on the round after each correction, from the log, on the fixture's forty-step task run live. The forty-step fixture has a correction at round twelve and asserts it survives; that test already exists and must still pass.

**Cost.** Half a day to one day.

### Idea 7. The librarian's notes page: harness-written gists on every shelved result

**What it is.** The design's own picture is a notes page where every book gets one line: its call number, its title, and what it told you. Today the line says the call number and the title: "r25 read main.js, 30,000 characters". The play-test task read its whole script twenty times, fifteen seconds of prompt processing each, to find one function each time. LLM idea 8 asked the model for a gist at thirty tokens a shelving; the harness can write a better one for nothing.

**Mechanism.** When a result's line is written, the harness derives it by tool. A file read: the path, the line range, and the top-level names in it, taken by a regular expression per language (functions, classes, exports), the same table the syntax checker keys on. A test run: the counts and the first failing test's name, which the test-state reader already holds. A shell command: the exit code and the first and last lines. A browser read: the title, the page-error count, and the answer to `ask` if there was one. A search: the row count and the first path. The result-line cap rises from seventy characters to a hundred and twenty for reads only. The `read` tool gains a hint on a whole-file read of a file the task has read before: "r25 lists its names; read from an offset for the part you need."

**Expected gain.** Each avoided re-read is a round (five to eighteen seconds) and eight to sixteen thousand prompt tokens (sixteen to thirty-two seconds). The play-test's twenty re-reads were about ten minutes. Against it, forty result lines at fifty more characters each is about seven hundred tokens more in the tail, 1.4 seconds a round, which on a 250-round task is six minutes. So the gain is net only on tasks that re-read, which are the coding tasks; the lines could be long for reads and short for everything else to hold the tail near its size. If the re-reads halve, the play-test task saves four minutes net.

**Risk.** The regular expressions name the wrong things in an unusual file; the line is a hint and `read r25` stays the truth. The tail growth is measurable and the cap can move.

**Measure in one nightly run.** `runreport` already counts repeated identical reads; add reads of a file already read in the task (any offset) and the mean result-line length. The Tetris run's play-test task is where it shows.

**Cost.** One to two days.

### Idea 8. Spend the slow side: a line-anchored edit that matches loosely, and a prompt-lookup drafter on the daemon

**What it is.** After the spacing change a round is about two seconds of prefill and about five seconds of writing, so the written tokens are now most of the round, and half of what the model writes on a coding task is text copied from the prompt: the old span of an edit, a test name, a result id, a path. Two things cut that. The `edit` tool can take a line range instead of the old span, so the model writes only the new text. And llama.cpp's prompt-lookup drafter proposes tokens from words already in the prompt, which is exactly copied text, and the big model only confirms them.

**Mechanism.** Harness side: `edit` gains `from_line` and `to_line` beside `old`; with lines given, `old` may be empty or a few words of the first line as a check, matched with whitespace folded; the syntax checker and the tests-after-a-change already run behind every edit, and a mismatch on the check words refuses the edit naming the lines' real first words. The `read` tool already prints line numbers with offsets, so the model has them. Daemon side, the owner's call: the prompt-lookup drafter flag on the start line beside MTP, and the daemon's log read for the acceptance rate on one nightly run.

**Expected gain.** An edit of a twelve-line span writes about 150 tokens of old text and 150 of new; line anchors drop the old half, 75 tokens saved per edit, about one second; run 5 had 19 edits in 150 rounds, run 13 far more. A drafter that doubles the speed of copied spans, as MTP did at 1.9 times overall on this model, takes five seconds of writing a round toward three and a half if half the output is copied text; on a 471-round job that is about twelve minutes, or ten percent of the run.

**Risk.** Line numbers drift after an earlier edit in the same file; the check words and the syntax check are the two nets, and the `read` after an edit shows fresh numbers. Aider found Qwen-class models malformed most diff edits, so the line form must be optional beside the exact-span form. Drafting acceptance falls with sampling temperature and the projector already took the daemon to a 131k context to fit; the card has been out of memory once this week.

**Measure in one nightly run.** Written tokens a round and the daemon's generation tokens a second in the nightly table, edit refusals per task from the log, and the drafter's acceptance rate from the daemon's log.

**Cost.** One day for the edit form; half a day to try the flag and read the numbers.

---

### Where the eight sit against the rejected list

None of the eight reintroduces a rejected idea. Idea 1 keeps the rewind and changes only how much it cuts. Idea 2 is PLAN 4's unbuilt half, PLAN 11 and LLM 5 in one checklist. Idea 3 extends a field two tools already have. Idea 4 is PLAN 7 as written. Idea 5 reuses the pick-up path and is conditional on re-measuring the half-drop under the new spacing. Idea 6 moves the printer's cut and nothing on disk. Idea 7 does LLM 8's job in the harness at zero output tokens. Idea 8 keeps every tool and adds a form and a daemon flag. The three ideas that would come next if these measure well are the git commit at green (L4d), the assertion line as a failure's cause (L7b, which idea 3 delivers for calls with an expectation), and re-running the last green check at a job task's start (L11b).
