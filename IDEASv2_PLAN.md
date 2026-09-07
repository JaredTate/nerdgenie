# The plan for IDEAS_V2: build it test first, keep it simple, measure it with the new Tetris prompt

Written 7 September 2026. This is the build plan for what `IDEAS_V2.md` proposes and `PROMPT_TEMPLATE_GUIDE.md` explains. It waits for the owner's yes. Every step is built the way everything here is built: write the test, watch it fail, write the code, watch it pass, run the whole gate, and measure the step on a real run before the next step begins. Two rules hold throughout: the simplest thing that passes the test, and nothing that no step needs.

## 1. What we are building, in one paragraph

A person writes an ask under six headings: Goal, Where, Done when, Rules, Tasks, Details. The harness reads the headings and writes the job's record itself, so there are no planning rounds and the finish is the person's. Every task sees the goal, the rules, its own line, and only the Details sections it names; it reads any other section by name. The harness holds tests first with a line on any code write that no failing test covers, runs the bracketed checks at the end of every task, and refuses a finish while one fails. The project's three documents, `AGENTS.md`, `ARCHITECTURE.md` and `REPO_MAP.md`, reach the model as the rules in full and the rest by section on request, and the harness keeps them true. When there is no work order, the model plans in the same shape, because the job tool asks for it.

## 2. What is already built

Three pieces the plan leans on exist and are tested. A done line must rest on a test run newer than the last edit (7 September). The `expect` line's four rules, tests, exit code, contains, parses, are read by `judge` in `internal/loop/expect.go`. The done check already runs a command a done line names and looks for a file it names (`internal/loop/donecheck.go`). The orientation block already lists the folder and the ports (`internal/orientation`). The read tool already reads a result by its label. None of that is rebuilt; each step adds to it.

## 3. How every step is measured

The measure is a Tetris run on the show home, done the way the owner watches it: a fresh home, memory wiped, `/yolo` on, the TUI window on the screen, the Chrome window on the screen, no message sent to the model until the job ends. Same daemon, same model, same folder name.

| Run | Prompt | Harness | What it measures |
|---|---|---|---|
| R0 | `TETRIS_TEST_PROMPT.md` | today's, commit b4702dcf (this is run 15) | the baseline |
| RA | `TETRIS_PROMPT_V2.md` | today's, no code change | the prompt alone |
| RB | `TETRIS_PROMPT_V2.md` | after steps 1 to 3 | the lift and the slice |
| RC | `TETRIS_PROMPT_V2.md` | after steps 4 and 5 | tests first and the checks |
| RD | `TETRIS_PROMPT_V2.md` | after steps 6 and 7 | the three documents |
| RE | `TETRIS_PROMPT_V2.md` | after step 8 | the finished set |

What is counted, per run and per task, from the event log through `scripts/runreport` and the nightly table, with four new columns the steps add:

- Rounds, model minutes, tokens in and out, the cached share.
- Planning rounds: rounds before the job exists. Record-writing rounds: rounds that only wrote the record.
- Uncached tokens at each task start.
- Reads of a file that had not changed since it was last read.
- Done lines proved by the harness against lines judged by the model. (new)
- Tests-first lines written on a code write. (new)
- Reads of a document by section. (new)
- Guard events: nudges, cuts, rethinks, fresh windows, same-call refusals.
- The check: `scripts/nightly/checks/05-tetris.sh` green, and the owner's own play in the window.

The rule for going on to the next step: the job finished on its own, every done line was proved, the check is green, and no counted number is worse than the run before by more than a tenth. A step that fails the rule is reverted before the next one starts. Every step lands as two commits, the test and then the code, with the session trailer, and updates `ARCHITECTURE.md`, the check table in `NERDGENIE.md`, and `docs/PROGRESS.md` in the same change.

## 4. The steps

### Step 0: the new prompt, and the prompt alone (half a day)

- Copy `TETRIS_PROMPT_V2.md` to `scripts/nightly/asks/05-tetris.md`, keeping the old ask as `05-tetris-v1.md` for a while. The check script does not change: it looks for the game folder, a green `npm test`, and the page served.
- Run RA. No code changes. This tells us how much the shape alone buys on today's harness, which reads it as a plain ask.

Expected: fewer planning rounds, because the Tasks list is a plan; done lines closer to the person's; fewer rules lost, because they sit in one place.

### Step 1: `internal/workorder`, the parser (one day)

One small package that turns the ask's text into a work order and says whether it is one.

- **Tests first.** `TestReadsTheSixHeadings` (each heading, in any case, in any order). `TestAnAskWithoutGoalAndDoneWhenIsNotAWorkOrder`. `TestADoneLineKeepsItsCheck` (the four kinds, `tests pass`, `exit 0`, `shows ... at ...`, `exists`, and a bracket it does not know is left in the text and reported). `TestARuleStartingWithStopIsAStopLine`. `TestATaskNamesItsDetailsSections` (`(Details: Dragon, Tests required)`). `TestDetailsAreSplitByHeading`. A golden test on `TETRIS_PROMPT_V2.md`: fourteen tasks, seven done lines, eight rules and no stop lines, eleven sections. `FuzzParse`: never panics, and every heading it reports is in the text.
- **Code.** `Parse(text string) WorkOrder`. A `WorkOrder` holds `Name`, `Goal`, `Where`, `DoneWhen []DoneLine{Text, Check}`, `Rules`, `StopIf`, `Tasks []Task{Text, Details}`, `Sections []Section{Heading, Body}`, and `IsWorkOrder`. Nothing else. Headings are the six names, matched without regard to case. The section-cutting helper lives in `internal/markdown`, a package of one function, `Sections(text)`, with its own fuzz target, because steps 3, 6 and 7 use it too.
- **Size.** About two hundred lines and their tests.

### Step 2: the lift (one day)

When a task's ask is a work order, the harness makes the job before the model's first call.

- **Tests first**, in `internal/loop` with the fake model: `TestAWorkOrderMakesTheJobBeforeTheFirstCall` (the first request already carries the job summary with the done lines, the rules and fourteen tasks; the model made no `job` call). `TestAWorkOrdersRulesRideAsCorrections` (C1 to Cn in the person's words; tests first is C1 whether the ask wrote it or not). `TestAWorkOrdersStopLinesAreTheStopList`. `TestAPlainAskIsUnchanged` (the existing goldens hold byte for byte).
- **Code.** In the loop's first turn: `workorder.Parse(ask)`; when it is one, create the job through the same `contract.Job` the job tool uses, with the whole ask as the job's ask, the Goal's first sentence as the name, the rest of the Goal as the why, one task per Tasks line, the done lines with their brackets kept in the text, the stop lines, and the rules as corrections. Then start the first task. No new tool. No new option.
- **Size.** About a hundred lines in `internal/loop/workorder.go`.

### Step 3: the task's slice (one day)

A job task sees the Details sections its line names, in full, and the other sections by heading.

- **Tests first.** A builder golden in `internal/context`: a task naming `(Details: Dragon)` has the Dragon section in full under its task line and the other ten headings as one line each ending "read one with `read ask <heading>`". `TestATaskNamingNoSectionsSeesEveryHeadingByLine`. In `internal/tool/read`: `TestReadsASectionOfTheAskByHeading`, `TestAnUnknownHeadingListsTheHeadings`.
- **Code.** The per-task front, where the job summary sits, gains the task's sections, cut with `markdown.Sections` from the job's ask, which the record already holds whole. The read tool's label reader learns one label, `ask`, followed by a heading. Bounded by the read tool's existing caps.
- **Size.** About eighty lines.

### Step 4: the tests-first line (half a day)

- **Tests first**, in `internal/loop`: `TestACodeWriteAfterAGreenRunGetsTheTestsFirstLine` (the newest test result is all passing, the write is to `src/engine.js`: the result's first line reads "tests first: no failing test covers this change; write it first"). `TestACodeWriteAfterARedRunGetsNoLine`. `TestATestFileNeverGetsTheLine` (`test/dragon.test.js`, `engine_test.go`, `spec/`, `__tests__/`). `TestBeforeAnyTestRunThereIsNoLine` (the scaffold task writes freely). `TestAFileThatIsNotCodeGetsNoLine` (`package.json`, `index.html`, `styles.css`, `README.md`).
- **Code.** Where the write and edit results are decorated today with "tests after this change", one check on the path and the newest test state. Code is a file whose extension is one of a short list; a test file is one whose name or folder says test or spec.
- **Size.** About sixty lines.

### Step 5: the checks at the end of every task and at the finish (one day)

- **Tests first**, in `internal/loop`: `TestABracketedDoneLineIsRunByTheHarness` for each kind, with the fake shell and the fake browser: `tests pass` runs the command and reads the count, `exit 0` reads the code, `shows` opens the page and looks for the text, `exists` looks for the file. `TestAJobTasksEndRunsTheJobsChecks` (the job summary header reads "done lines proved: 3 of 7" after task nine). `TestTheFinishIsRefusedWhileACheckFails` (the send-back names the line and carries the output's tail). `TestAFailingCheckStopsTheTaskAfterThreeTries` (the person wrote the check, so the harness never lets the line stand on the model's word; after three the task stops and says which line).
- **Code.** `checkOneDoneLine` in `donecheck.go` reads the bracket through `workorder.Check` and runs it through the calls the done check already makes; `finishJobTask` runs the job's bracketed lines and writes the count into the job summary's header.
- **Size.** About a hundred and twenty lines.

### Step 6: the three documents in the window (a day and a half)

- **Tests first.** In `internal/orientation`: a golden with the two document lines for a folder holding `ARCHITECTURE.md` with nine sections and a `REPO_MAP.md` with forty-one files, and a golden without them (nothing added). In `internal/context`: a golden with `AGENTS.md` under the job summary, `TestAStandingOrderOverSixtyLinesIsCutAndSaysSo`, and the cache-shape test still holding (the block is inside the stable prefix). In `internal/tool/read`: `TestReadsASectionOfAMarkdownFileByHeading`, `TestASectionIsBoundedLikeAFile`, `FuzzSection`.
- **Code.** `orientation.documentLines(folder)`. `BuildInput.StandingOrder`, read once at task start from the Where folder, printed after the job summary. The read tool gains one optional field, `section`.
- **Size.** About a hundred and fifty lines.

### Step 7: the documents kept true (a day and a half)

- **Tests first.** In `internal/loop`: `TestTheReviewAsksWhichArchitectureSectionChanged` (with the fake model: after a task in a folder with `ARCHITECTURE.md`, the named section's body is the model's answer, dated; a folder without the file gets no fifth question; an answer naming no section changes nothing). `TestAMapWithTheGeneratedMarkIsRewrittenAfterAWrite` and `TestAMapWithoutTheMarkIsNeverTouched`. `TestAFinishedJobWritesAgentsMdOnceAndNeverOverwrites` (from the work order's Goal, the test command of the first done line, the Rules, and pointers to the other two documents). In `internal/repomap`: a golden on a fixture tree, with `node_modules`, `dist` and `.git` left out. In `cmd/nerdgenie`: `nerdgenie map <folder>` writes the three files when absent and only the map when the other two exist.
- **Code.** A fifth question in `review.go` and a writer that replaces one section through `internal/markdown`. `internal/repomap`, a tree generator of one file, marked `<!-- generated: nerdgenie map -->`, run by the loop after a write or edit under a folder whose map carries the mark. `AGENTS.md` written at the job's finish. This repository keeps its own generator in `scripts/repomap`; the two are not merged until a second reason appears.
- **Size.** About two hundred lines.

### Step 8: the job tool asks for the shape (half a day)

- **Tests first.** In `internal/tool/job`: `TestCreateRefusesATaskWithoutADoneLine` (the refusal names the task and says what a done line is). `TestATaskMayNameItsDetails`. In `internal/context`: the instructions golden with the "Jobs and tasks" paragraph saying to write a job as a work order: name, why, done lines, rules, tasks each with one done line. The forty-step fixture unchanged.
- **Code.** A task object gains `done` and `details`; the tool's description names the six parts; one paragraph of the instructions text changes.
- **Size.** About forty lines.

## 5. The order, the days, and what each run should show

| Step | Days | Run after it | What should move |
|---|---|---|---|
| 0 | 0.5 | RA | planning rounds down; done lines closer to the person's |
| 1 to 3 | 3 | RB | planning rounds to zero; task-start tokens down by the size of the unread sections; reads of unchanged files down |
| 4 to 5 | 1.5 | RC | tests-first lines appear and then stop appearing; done lines proved by the harness up; no finish on an unproved line |
| 6 to 7 | 3 | RD | reads by section replace reads of whole files; the three documents exist in the game folder at the end |
| 8 | 0.5 | RE | a plain ask plans in the shape; nothing else changes |

Eight and a half days. Each run is a night on the show home, so the whole plan is about two weeks with the measuring.

## 6. What we are not building

- No index of files, no full-text search of the folder, no import graph, no worktrees, no gate line. Those are `IDEAS_V3.md`, for large code bases, and they wait until this set is measured.
- No new tool. Sections are read through `read`. The ask is read through `read ask`.
- No parser per language. A test file is known by its name; code is known by its extension.
- No configuration option. A work order is recognised by its headings; a plain ask is unchanged.
- No summary of anything. Details are cut by heading and shown whole or not at all.

## 7. Risks, and what catches them

- **The model ignores the task's slice and reads the whole ask anyway.** RB's reads-by-section count shows it. The slice stays; the heading lines say how to read more.
- **The tests-first line fires on a write the model had to make** (a fixture, a helper). The short code-extension list and the "before any test run" rule keep it quiet on scaffolds; RC's count shows whether it fires more than once or twice a task.
- **A bracketed check cannot run on the machine** (the browser cannot open the page). After three tries the task stops and names the line, which is the one place this set lets a run wait for a person, and it is the harness's doing, not a stop line in the ask.
- **The fifth review question writes nonsense into the architecture page.** The section is replaced, dated, and the old body is in the log; the owner reads the page after RD.
- **The cache breaks.** The standing order sits inside the stable prefix and the task's slice sits in the per-task front; `cacheshape_test.go` holds the layout, and every run's cached share is on the table.
