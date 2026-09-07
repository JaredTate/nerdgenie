# One work order for any coding project, and how the harness runs it

Written 7 September 2026, rewritten the same day to be simpler. This is a proposal, not a description of what is built. The owner approves it before it is built, it is built test first, and it lands in `docs/PROGRESS.md` with its measured effect.

The goal is repeatable success with a small local model: the same shape of ask, the same shape of work, the same proof at the end, on a new project or an existing one. Large code bases get their own set in `IDEAS_V3.md`; this page is the base that set stands on.

## 1. Three words

- **A task** is one sitting of work with one done line.
- **A job** is a list of tasks. The harness runs them one at a time.
- **The record** is the page the harness keeps for every task and every job: what was asked, what done looks like, the rules, the plan, the results, the lessons. The model reads it on every call. It is the truth.

The work order is the record, written by you before the work starts. That is the whole idea. Everything the harness does with it follows from the fact that it already knows the shape.

## 2. The work order

Six headings, in this order. Write them in plain words.

| Heading | What you write | What the harness does with it |
|---|---|---|
| **Goal** | What we are building or changing, for whom, and why. Two to five sentences. | Becomes the job's name and its why. Printed at the top of every task, so the model always knows what the work is for. |
| **Where** | New project: the folder. Existing project: the repository, the branch, and the files to start from. | Opens the folder and the starting files before the model's first call. Finds the project's documents there (section 5). |
| **Done when** | Numbered lines. Each one is something that must be true at the end. Put a check in brackets when the harness can run it. | Becomes the job's done list. Runs the bracketed checks at the end of every task and at the finish. The model cannot declare the job done; the list does. |
| **Rules** | What holds on every task. A line that starts with "Stop" means stop and ask me. | Every rule rides in front of every task, in your words, never rewritten. Stop lines end the task the moment they come true. |
| **Tasks** | The order of work, one line each, one done line each. Optional: the model plans when it is absent. | Becomes the job's task list with no planning rounds. The model may split a task or add one, never remove a finished one. |
| **Details** | Everything else, as long as you like, under its own headings. | Each task sees the sections it names in full and the other sections by heading. The model reads any section by name. |

Four checks the harness can run, written in brackets at the end of a done line:

- `[tests pass: npm test]`
- `[exit 0: node build.js]`
- `[shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`
- `[exists: dist/index.html]`

A done line with no check is judged by the model, and the model must name the result that proves it, as today.

Three rules of thumb. Keep the Goal short and put the length in Details. Make the first done line the test suite, because the harness runs it at the end of every task. Give each task one done line and name the Details section it needs.

**Tests first is always the first rule.** The harness puts it there itself, the way it adds two lines of its own to every stop list: "Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green before the task ends." You may add rules under it. You may not remove it.

## 3. Tater Tots Tetris as a work order

The ask we test with today is 735 lines and thirty headings. The goal is on line 8, the finish is twenty-nine bullets on line 678, and tests first is said four times. Here is the same ask as a work order. Nothing is lost; the long parts move under Details.

```
# Tater Tots Tetris

## Goal
A complete, polished, playable Tetris-style web game called Tater Tots Tetris, for
players in a browser. Normal Tetris play, plus a dragon that locks the falling piece
after a warning, a yeti that blows the piece sideways from the left or the right, and
three line-clear effects: freeze and disintegrate, bomb, fire. Every rule of the game
is proved by a test before it is built, and the finished game is played and looked at
in Chrome before it is called done.

## Where
A new, empty folder: <WORK>/Tater Tots Tetrisv1. Serve it on any port but 8090.

## Done when
1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The game loads and shows the board. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]
3. A whole game has been played in the Chrome window to game over, with a photograph
   of each state: play, dragon warning, dragon attack, yeti from each side, the three
   clears, pause, game over, restart.
4. The layout has been looked at at five window sizes with nothing overlapping and no
   console errors.
5. Restart resets everything and no hazard timer survives it.
6. The developer controls used for testing are hidden in the finished game.

## Rules
- Plain HTML, CSS and JavaScript. No framework. No build step.
- The browser is the Chrome window on the screen, never a headless one from a script.
- Hazard values live in one config file: probabilities, warning times, forced-drop speed.
- The random source can be injected, so any hazard can be forced in a test.
- Stop if a test cannot be made to pass after three different fixes.
- Stop if the Chrome window cannot open the page.

## Tasks
1. Scaffold: package.json, a test runner, index.html, a smoke test. (Details: Core game)
2. The engine: board, pieces, movement, rotation, collision, locking, lines, scoring,
   levels, speed, game over, restart, pause, next piece, high scores. (Details: Core game)
3. The hazard state machine and the config file. (Details: Hazard architecture)
4. The dragon. (Details: Dragon)
5. The yeti, both sides. (Details: Yeti)
6. The three line-clear effects. (Details: Line clear effects)
7. Hazard safety: no stale timers, no lost piece, clean restart. (Details: Hazard safety)
8. The browser shell: HUD, keyboard, developer controls. (Details: Core game)
9. Play it in Chrome and fix what the play shows. (Details: Browser play testing)
10. Look at it at five sizes and polish. (Details: Visual QA, Visual polish)
11. The whole suite once more, a last look, hide the controls.

## Details
### Core game
(the original's list, word for word)
### Dragon
### Yeti
### Line clear effects
### Hazard architecture
### Hazard safety
### Browser play testing
### Visual QA
### Visual polish
```

That is the same 2,898 words. The finish is six lines, two of them run by the harness. The rules are six lines that reach every task. Each task says which part of the details it needs.

## 4. What the harness does with it, the same every time

**At the start.** The harness reads the six headings and writes the job's record itself: name and why from Goal, the done list with its checks from Done when, the rules and the stop lines from Rules, the task list from Tasks. There are no planning rounds. It opens the folder from Where. Then it starts task one.

**For every task, the same five moves.** This is the process, and it is the same on task one of a game and task nine of a change to a large application.

1. **Orient.** The model sees the goal, the rules, the task's own line with its done line, the Details section the task names, the folder as it is now, and the last task's report. Nothing about the other tasks beyond their names.
2. **Test.** The model writes the test for the task's done line. The harness runs it and puts the red result on the write's own line. A code file written before a failing test exists gets one line on its result: "tests first: no failing test covers this change; write it first."
3. **Code.** The model writes or edits the code. After every change the harness checks the file parses and runs the tests again, and puts both answers on the change's own line. The model never spends a round finding out.
4. **Prove.** The model marks the task's done line with the green test run. The harness refuses a mark that points at a run older than the last edit, and runs the job's bracketed checks.
5. **Report.** The task's report goes into the job and to you. The next task starts.

**At the end.** Every bracketed check must pass and every other done line must point at a result. Then the job is done, and the report says what changed, what was checked, and what is left.

**When there is no work order.** A person types what they type. Then the model writes the job in the same six-heading shape, because the `job` tool asks for exactly those fields: a name and a why, done lines, rules, tasks each with one done line and the files it touches. So the model outlines jobs and tasks the way the template does, and every task runs through the same five moves. The template is not only what a person writes; it is the shape the model is given to think in.

## 5. The three project documents, and when the model reads them

Every project we keep has three documents. A new project gets them from the job that builds it. An existing project already has them.

- **`AGENTS.md`**: the rules of the project and how to run and test it. Under sixty lines. This is the industry's name for the file, so a project that has one for another tool has one for Nerd Genie.
- **`ARCHITECTURE.md`**: how the project is put together, one section per part.
- **`REPO_MAP.md`**: where everything is. Generated, never written by hand.

**What the model sees.** The harness looks for the three in the Where folder at every task start. Then:

- `AGENTS.md` rides in full under the job summary, in front of every task, because it is the rules and it is short. Rules in the work order come after it and win.
- The other two ride as one line each in the orientation block, the block that already shows the folder and the ports: "ARCHITECTURE.md: 9 sections: engine, hazards, effects, shell, tests, ...; read one with `read ARCHITECTURE.md hazards`" and "REPO_MAP.md: 41 files in 6 folders; read a folder's part with `read REPO_MAP.md src`".
- The `read` tool learns one thing: a Markdown file followed by a heading returns that section and nothing else. That is how the model reads a part of the architecture page, or one folder of the map, whenever it would help, without ever reading the whole.
- The `search` tool already finds a file by name and a line by pattern, and it stays as it is.

The rule is short: the rules ride in full, the rest is read by name when it would help. The model can reference any of the three at any time, and none of them can crowd the task out of the window.

**What keeps them true.** The harness regenerates `REPO_MAP.md` after every write in the folder. At the end of every task the review already asks four questions with the tools off; it asks a fifth when the folder has an architecture page: "which section does this task change, and what should it say now?" The answer goes under that heading, dated. On a new project the harness starts the page from the task reports and starts `AGENTS.md` from the work order's Rules, Where and test command. So every job leaves the three documents behind, and the next job on the same folder begins oriented.

**A walk-through: task four, the dragon.** In front of the model: the goal, the seven rules, "Task 4: the dragon. Done when: the dragon tests pass. Details: Dragon.", the Dragon section in full, `AGENTS.md` as the job wrote it at task one, the folder with `src/engine.js`, `src/hazards.js`, `src/config.js` and three test files, the report of task three saying the hazard state machine and config are in and green, and the orientation's two lines for the architecture page and the map. The model reads `read ARCHITECTURE.md hazards` because task three wrote that section, writes `test/dragon.test.js`, watches it fail on the write's own line, writes the dragon into `src/hazards.js`, watches the suite go green on the edit's own line, marks the done line with that run, and reports. Five moves, no wasted round, and the next task starts with a page that says what the dragon is.

## 6. Why this makes success repeatable

- The finish is written by the person and checked by the harness, so a job cannot be talked done.
- The rules ride in front of every task in the person's words, so nothing is lost at task nine.
- Every task proves itself with a test, and the proof must be newer than the last edit.
- Every task sees its own slice: its done line, its details, its files, the last report. Not the whole ask.
- Every job leaves three documents behind, so the next job starts where this one ended.

## 7. What to build, in order

1. **The lift.** Six headings read into the job's record, the Details shelved by heading, the task's slice in the per-task front. Two days.
2. **The five moves held by the harness.** The tests-first line on a code write with no failing test; the checks run at the end of every task. The fresh-proof done line is built already. One day.
3. **The three documents in the window.** `AGENTS.md` in full under the job summary; one line each for the other two in the orientation; `read <file> <heading>`. One day.
4. **The documents kept true.** The map regenerated after every write; the fifth review question writing the architecture section; a new project's three documents started by the job. Two days.
5. **The `job` tool asking for the six-heading shape** when there is no work order, so the model plans the same way. One day.

Seven days. The measure is the nightly table: rounds and tokens per task, tasks that closed on a fresh test run, jobs closed done that the nightly check then failed, and reads of the three documents by section against reads of whole files.
