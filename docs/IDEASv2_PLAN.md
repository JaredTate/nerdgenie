# The plan for IDEAS_V2: what changes, why, how much it gains, and how to build it

Written 7 September 2026, revised the same night with the estimate from run fifteen's own log, the sub-agent split and the goal below. It waits for the owner's yes.

## The goal (copy and paste this to start the work)

> Read `~/Code/nerdgenie/IDEASv2_PLAN.md`, `docs/IDEAS_V2.md`, `PROMPT_TEMPLATE_GUIDE.md` and `EX_PROMPT_2_TETRIS.md`, then execute the plan exactly as written. Test first on every step: write the failing test, watch it fail, write the code, watch it pass, and commit the test before the code with the session trailer. Build wave 0 yourself, then run wave 1 as three Fable 5.1 sub-agents on high, each in its own git worktree with the step assignments in the plan; when all three report, merge them onto main one at a time, resolve any conflict by hand, run the whole gate (gofmt, go vet, staticcheck, the style checker, the repo-map drift test, `go test ./...`, the integration tests of every touched package, the functional suite, `scripts/coverage.sh`) and `make build`. Then run wave 2 as one sub-agent, merge, gate, build. Update `ARCHITECTURE.md`, the check table in `NERDGENIE.md` and `docs/PROGRESS.md` in the same commits. Leave the fuzzers out while the local daemon is on the card. Never kill a process by name pattern; exact pids only, liveness by port. Do not touch the daemon's start line or sampling. When the build is green, archive the show home's database by run number, wipe its memory the way the start script does, empty the game folder, and start a fresh Tetris run on the show home with `EX_PROMPT_2_TETRIS.md`, `/yolo` on, the TUI window and the Chrome window on display :1, and do not send the model a single message until the job ends. Report the run against run fifteen on the nightly table's columns plus the four new ones, and say plainly which of the expected numbers moved and which did not.

## The logic, on one page

A small model can only do good work on what is in its window, and the harness decides what is in the window. So the harness has one job on every call: put three things in front of the model and nothing else. What the work is: the goal, the rules, this task. What is true now: the record. The specific knowledge the next step needs: the file being edited, the part of the spec, the part of the design. Everything else must be one round away by name, and out of the window.

Today the third thing is the problem. The model gets the specific knowledge it needs by searching for it itself: it reads whole files, reads them again after an edit or a fresh window, and lists and greps to find them. Every one of those is a round. Every whole-file read is up to sixteen kilobytes that pushes the record's own results out of the window, which causes more reads. Run fifteen, on the current harness, shows the size of it below.

The three documents are three answers the model can fetch in one round instead of several.

- "What are the rules here, how do I run and test this?" is `AGENTS.md`. It is short, so it is always in the window.
- "How is this part put together, what are its names, which file?" is one section of `ARCHITECTURE.md`, two hundred words.
- "Where is X?" is the map's legend in the orientation, and the search tool for the rest.

**Round by round, the dragon task.** Today: round one lists `src`; rounds two and three read `hazards.js` in two halves because it is over the read cap; round four reads `config.js`; round five reads `engine.js` for the piece interface; round six writes the test. With the documents: the orientation block already says "ARCHITECTURE.md: sections engine, hazards, effects, shell, tests; read one with `read ARCHITECTURE.md hazards`", so round one reads that section, which names the states, the `raise(kind, piece)` function, the config values, the file and the test file. Round two reads the eighty lines of `hazards.js` around `raise`. Round three writes the test. Three rounds instead of six, and the window holds two hundred words and one region instead of three whole files, so nothing is evicted and nothing is read twice. The section is a result with an id, so after a cut it comes back by name for nothing.

**How the model uses what it fetched.** The section names the contract between the parts, so the model edits with the right names in the right file, and does not break the part next door. When a task changes a part, the review writes the section at the task's end, so the next task reads the truth, not last week's. The documents feed the window, the window feeds the work, the work feeds the documents.

**Why that is a more effective harness.** A task's cost is its rounds, and a round's cost is what is new in front of the model. Fewer reads means fewer rounds. Smaller reads means the window keeps what matters, so the model stops re-reading. The contract in front of the model means fewer wrong edits, which means fewer test-fix rounds. And the same three fetches work on a game in an empty folder and on a four-hundred-thousand-line repository, because the harness needs nothing from the project except the three files.

## The baseline: run fifteen, on the current code

Only run fifteen counts as a baseline, because only it runs the harness as it is tonight. Counted from its event log at task nine of twelve, 387 rounds in:

| What the rounds went on | Rounds | Share |
|---|---|---|
| Planning: reading the whole ask and writing the job | 4 | 1 percent |
| Rounds that only wrote the record | 34 | 9 percent |
| Reads in the first three rounds of a task, the model finding its bearings | 36 | 9 percent |
| Reads of a file the task had already read | 70 | 18 percent |
| Shell rounds that only listed, found or printed files | 10 | 3 percent |
| Fresh windows at the message cap | 1 | |
| Uncached tokens at a task start | 1,000 to 1,300 on seven of nine tasks; 8,000 and 11,500 on two | |

Two things that table says. The task-start cost is already small, because the cache-shaped prompt of this morning fixed it; the slice of the ask saves attention there, not tokens. And the largest single cost is re-reading: seventy rounds in which the model read a file it had read before, most of them after an edit, after a fresh window, or to find an interface again.

## How much faster, from those numbers

Each line assumes what fraction of the baseline rounds the change removes, and says so. The run after each wave measures it.

| Change | Baseline rounds it works on | Assumed | Rounds saved per run of twelve tasks |
|---|---|---|---|
| The work order makes the job | 4 planning | all | 4 |
| The done lines and rules come from the work order, not the model | 34 record-only, about 45 over twelve tasks | a third to a half | 15 to 22 |
| Sections instead of files when finding bearings | 36 first-three-round reads, about 48 over twelve tasks | half | 24 |
| Sections and the task's slice instead of re-reading files | 70 repeat reads, about 95 over twelve tasks | a third (the re-reads after an edit stay) | 30 |
| The legend and the orientation instead of listing | 10 shells, about 13 over twelve tasks | half | 6 |
| **Total** | about 520 rounds over twelve tasks | | **80 to 86 rounds, about 15 percent** |

At run fifteen's pace of about twenty seconds a round that is about 27 minutes of a three-hour build. The lines are assumptions on real counts, not measurements; RB is the measurement, and the plan says how to compare.

Reliability is the other half, and it has no baseline number yet: run fifteen has not finished. From RB on, the table gains "done lines proved by the harness against lines judged by the model", and the count that matters is jobs that end "done" with a check red, which the harness makes impossible.

## A fresh Tetris run, task by task

On a fresh run the folder is empty and none of the three documents exist. This is where each thing helps, and where it does not.

- **Before task one.** The template does the work. The job with fourteen tasks exists before the first model call instead of after five planning rounds. The seven done lines are the person's, two of them checks the harness runs. The eight rules sit in the record's rules for every task.
- **Task one, the scaffold.** Nothing to fetch. At its end the review writes the first section of `ARCHITECTURE.md` (the test runner, `npm test`, `npm start` on 8091, the files) and the harness writes `AGENTS.md` from the work order. From here on both exist.
- **Tasks two to nine, the engine and the hazards.** Each task's end writes its section: engine (the file, the board and piece functions, the test file), hazards (the states, `raise`, the config values). When task six starts, it does not read `engine.js`, `hazards.js` and `config.js` whole to learn the interfaces; it reads the hazards section in one round, opens the eighty lines it needs, and writes the test. Every time a task hits the message cap and gets a fresh window, as task seven of run fifteen did at round 58, the model re-orients from the section list instead of reading its own files back.
- **Tasks ten and eleven, the browser shell and the effects on screen.** The expensive tasks today, and where the documents matter most: the shell has to know the engine's interface and the hazard states. Two sections, four hundred words, two rounds, instead of three files each over the read cap.
- **Tasks twelve to fourteen, play test, QA, regression.** The documents matter little. The checks matter: after every task the harness runs `npm test` and opens the page, and the finish cannot close while either fails.
- **Where it does not help.** `REPO_MAP.md` buys nothing on a fresh Tetris run; the orientation's folder listing already shows forty files. The map is for a repository the size of Home Recon. And the documents cannot make the model write better game code; they make it start every task, and every fresh window, already knowing how the parts fit, which is what it spends rounds finding out today.

What the table should show against run fifteen: the planning rounds gone, fewer reads in the first three rounds of each task, fewer repeat reads, and every run ending with the finish proved.

## The five problems, in run fifteen's terms

1. **Re-orientation and re-reading.** 36 reads in the first three rounds of tasks and 70 reads of files already read, 27 percent of all rounds.
2. **Planning and bookkeeping.** 4 planning rounds and 34 rounds that only wrote the record, 10 percent.
3. **Rules by luck.** The ask's rules reach task nine only because the model named every task "TDD ..."; a project's house rules sit in a file nothing points at.
4. **Finding where things are.** 10 shell rounds that only listed, found or printed files, and the same on any larger repository many times over.
5. **The finish is the model's word.** The harness checks a done line that names a file or a command; the rest the model judges. No run so far has had a check the person wrote.

## The steps

Six steps, built in three waves. Wave 0 is one small package the others share, built by the orchestrator before any sub-agent starts. Wave 1 is three sub-agents in parallel, each in its own worktree, on steps that touch different files. Wave 2 is one sub-agent on the step that needs wave 1 merged. Every step: the failing test committed first, then the code, then the documents, with the session trailer on every commit.

### Wave 0, the orchestrator: `internal/markdown` (half a day)

**What.** One package, one job: cut a Markdown text into its sections by heading. `Sections(text) []Section{Level, Heading, Body}` and `Section(text, heading) (Section, bool)` with a match that ignores case and surrounding spaces. A `doc.go`, a fuzz target `FuzzSections` that never panics and whose headings all appear in the text, table tests on nested headings, code fences that contain `#` lines, and an empty text.

**Why first.** Steps 2, 4 and 5 all cut sections; one package built and merged before the fork means no two agents write it.

### Wave 1, agent A: steps 1 and 2, the work order and the task's slice (three days)

**Step 1, the work order becomes the record.** New package `internal/workorder`: `Parse(text) WorkOrder` with `Name`, `Goal`, `Where`, `DoneWhen []DoneLine{Text, Check}`, `Rules`, `Tasks []Task{Text, Details}`, `Sections`, `IsWorkOrder`. The six headings matched without regard to case; a check is the trailing `[kind: argument]` of a done line, kinds `tests pass`, `exit 0`, `shows`, `exists`, an unknown kind left in the text and reported; a task's `(Details: A, B)` read into `Details`. Then in `internal/loop`, a new file `workorder.go`: on a task's first turn, when the ask parses as a work order and the task is not already a job's, create the job through the same `contract.Job` the job tool uses, the whole ask as the job's ask, the Goal's first sentence as the name, the rest of the Goal as the why, one task per Tasks line, the done lines with their brackets kept in their text, the Rules as the record's rules with tests first as the first whether or not the ask wrote it, and start the first task. No new tool, no option, and a plain ask is untouched.

- Tests first: in `internal/workorder`, each heading, a plain ask is not a work order, a done line keeps its check, a task names its details, a golden on `EX_PROMPT_2_TETRIS.md` (fourteen tasks, seven done lines, eight rules, eleven sections), `FuzzParse`. In `internal/loop`, `TestAWorkOrderMakesTheJobBeforeTheFirstCall` (the first request carries the job summary with the tasks, the done lines and the rules, and no `job` call was made), `TestAWorkOrdersRulesRideAsTheRecordsRulesWithTestsFirstFirst`, `TestAPlainAskIsUnchanged` (the existing goldens hold byte for byte).
- Fixes problems 2 and 3. Shows in: planning rounds, five to zero; record-writing rounds per task.

**Step 2, each task sees only its own part of the ask.** The per-task front, where the job summary sits in `internal/context`, gains the task's Details sections in full and the other sections as one heading each ending "read one with `read ask <heading>`", cut with `markdown.Section` from the job's ask. The read tool's label reader in `internal/tool/read` learns the label `ask` followed by a heading, bounded by the tool's existing caps.

- Tests first: a builder golden for a task naming `(Details: Dragon)`; `TestATaskNamingNoSectionsSeesEveryHeadingByLine`; `TestReadsASectionOfTheAskByHeading`; `TestAnUnknownHeadingListsTheHeadings`; the cache-shape test still holds.
- Fixes problems 1 and 2. Shows in: uncached tokens at each task start; reads of the ask by section.

### Wave 1, agent B: steps 3 and 4, the documents in the window (a day and a half)

**Step 3, `AGENTS.md` in front of every task.** `BuildInput` in `internal/context` gains `StandingOrder string`, read once at task start by the loop from the Where folder, printed after the job summary and before the record's goal and rules, capped at sixty lines with a line saying it was cut. The ask's own rules come after it and win.

- Tests first: a builder golden with the block under the job summary; a folder without the file adds nothing; `TestAStandingOrderOverSixtyLinesIsCutAndSaysSo`; the cache-shape test still holds because the block is inside the stable prefix.
- Fixes problem 3. Shows in: rounds reading rule files; rules broken and corrected by the owner.

**Step 4, `ARCHITECTURE.md` by section and the map's legend.** `internal/orientation` gains `documentLines(folder)`: for `ARCHITECTURE.md`, one line naming its sections and how to read one; for `REPO_MAP.md` with a Roots legend, the legend's lines, at most fifteen. The read tool gains one optional field, `section`: a Markdown file followed by a heading returns that section only, through `markdown.Section`, bounded like a file; an unknown heading lists the headings.

- Tests first: an orientation golden with both lines for a folder holding the two files and a golden without them; `TestReadsASectionOfAMarkdownFileByHeading`; `TestASectionIsBoundedLikeAFile`; `FuzzSectionRequest`.
- Fixes problems 1 and 4. Shows in: reads in the three rounds after a task start or a fresh window, and how many are by section.

### Wave 1, agent C: step 6, proof at every task's end (a day and a half)

**The tests-first line.** Where write and edit results are decorated in `internal/loop` today with "tests after this change", one check: a write or edit to a code file (by a short extension list), when the newest test state the harness holds is all passing, gets as its result's first line "tests first: no failing test covers this change; write it first". A test file (name or folder says test or spec), a file that is not code, a write after a red run, and a write before any test run in the task get no line.

**The checks at the end of every task and at the finish.** `checkOneDoneLine` in `donecheck.go` reads a bracketed check and runs it through the done check's existing command runner and the browser: `tests pass` runs the command and reads the count through the test reader, `exit 0` reads the code, `shows` opens the page and looks for the text, `exists` looks for the file. `finishJobTask` runs the job's bracketed lines and writes "done lines proved: 3 of 7" into the job summary's header. The finish is refused while a check fails, with the line named and the output's tail; after three failures of one check the tail goes into the record as a failure with its cause and the model goes on by another route; the finish stays refused until the check passes.

- Tests first: `TestACodeWriteAfterAGreenRunGetsTheTestsFirstLine`, `TestACodeWriteAfterARedRunGetsNoLine`, `TestATestFileNeverGetsTheLine`, `TestAFileThatIsNotCodeGetsNoLine`, `TestBeforeAnyTestRunThereIsNoLine`; one test per check kind with the fake shell and the fake browser; `TestAJobTasksEndRunsTheJobsChecks`; `TestTheFinishIsRefusedWhileACheckFails`; `TestAFailingCheckBecomesAFailureAfterThreeTries`.
- Fixes problem 5. Shows in: done lines proved by the harness against lines judged by the model; tests-first lines per task, which should appear early and then stop; jobs closed done that the nightly check then failed, which should go to zero.

Agent C's step reads the bracketed check, which agent A also parses. To keep the two apart, agent C writes the check reader in `internal/workorder/check.go` (a file agent A does not touch) with its own tests, and agent A's `Parse` calls it after the merge.

### Merge, gate, build, then wave 2

Merge A, B, C onto main one at a time, conflicts by hand, the whole gate, `make build`. Then wave 2.

### Wave 2, one agent: step 5, the documents are written by the job (one day)

**What.** The review in `internal/loop/review.go` asks a fifth question when the Where folder holds `ARCHITECTURE.md`: which section did this task change, and what should it say now? The answer replaces that section through `markdown`, dated, and a section that does not exist yet is appended; a folder with no page gets one from the first task's answer. When a job finishes in a folder with no `AGENTS.md`, the harness writes one from the work order: what this is from Goal, the test command from the first done line, the Rules, and a pointer to the architecture page. Neither file is ever overwritten by the harness when the person has written it.

- Tests first: `TestTheReviewAsksWhichArchitectureSectionChanged` (the named section holds the answer, dated); `TestAFolderWithoutAnArchitecturePageGetsOne`; `TestAnAnswerNamingNoSectionChangesNothing`; `TestAFinishedJobWritesAgentsMdOnceAndNeverOverwrites`.
- Fixes problem 1 for the next task and the next job. Shows in: the two files in the game folder at the end of a run, and the reads in the three rounds after a task start in the run after.

## The runs

| Run | Prompt | Harness | What it measures |
|---|---|---|---|
| R0 | `TETRIS_TEST_PROMPT.md` | commit b4702dcf, this is run fifteen | the baseline |
| RA | `EX_PROMPT_2_TETRIS.md` | the same, no code change | the prompt alone |
| RB | `EX_PROMPT_2_TETRIS.md` | after wave 1 merged | the lift, the slice, the documents in the window, the checks |
| RC | `EX_PROMPT_2_TETRIS.md` | after wave 2 merged | the documents written by the job |

Every run the way the owner watches them: a fresh home, the database archived by run number, memory wiped, the game folder empty, `/yolo` on, the TUI window and the Chrome window on display :1, and no message to the model until the job ends. What is counted, from the event log through `scripts/runreport` and the nightly table, with four new columns: rounds, model minutes, tokens in and out, the cached share; planning rounds and record-writing rounds; uncached tokens at each task start; reads of a file unchanged since it was last read; done lines proved by the harness against lines judged by the model (new); tests-first lines per task (new); reads of a document or the ask by section (new); guard events: nudges, cuts, rethinks, fresh windows, same-call refusals; the check script green; the owner's own play in the window.

The rule for going on from RB to wave 2, and for calling the plan done after RC: the job finished on its own, every done line was proved, the check is green, and nothing on the table is worse than run fifteen by more than a tenth. A wave that fails the rule is reverted before the next.

## What we are not building

No index of files, no full-text search of the folder, no import graph, no worktrees for the model's own work, no gate line: those are `IDEAS_V3.md`, for large code bases, and they wait until this set is measured. No new tool: sections go through `read`. No parser per language. No configuration option. No summary of anything.

## Definition of done for the whole plan

- Wave 0, wave 1 and wave 2 merged on main, every commit a test-then-code pair with the session trailer.
- The whole gate green and `make build` done after each merge; the fuzzers left for a time the daemon is not on the card.
- `ARCHITECTURE.md` (the loop, the context, the orientation, the read tool, the review), the check table in `NERDGENIE.md` (one row per step, each naming its test) and `docs/PROGRESS.md` updated.
- `scripts/nightly/asks/05-tetris.md` replaced by `EX_PROMPT_2_TETRIS.md`, the old ask kept as `05-tetris-v1.md`.
- RB and RC run and reported against run fifteen, with a plain sentence per expected number saying whether it moved.
