# Ideas, second set: an ask written for the harness, and a project's own documents as its working context

Written 7 September 2026, the day the three-bit model on rosie ran the whole Tetris job to the end on its own. The harness now keeps a small model on course through a long job. Two inputs to it have never been shaped: the ask a person writes, and the documents a project already carries, its `ARCHITECTURE.md` and its `REPO_MAP.md`. This set shapes both, and it does it with the machinery the harness already has, the record, the shelf of results, the skill loader and the harness's own checks, rather than with new machinery. Nothing here is built. The owner approves an idea before it is built, it is built test first, and it lands in `docs/PROGRESS.md` with its measured effect.

## 1. How the harness reads, in six facts

Everything below follows from these six facts, all of them from `NERDGENIE.md` and the design.

1. **The record is the truth, and it has a fixed shape.** Goal: the ask word for word, a why, and the done lines. Rules: the corrections, in the person's words, and the stop list. Work: the situation, the plan, the results. Lessons: decisions with reasons, failures with causes. The model writes the why, the done list, the stop list and the plan; the harness writes everything ordinary code can verify.
2. **The prompt is built in layers, ordered by how often each changes.** The harness rules, the persona and the skill list come first and are cached across a whole run. Then the tools. Then, for a task inside a job, the job summary: the job's goal, rules and task list, the same bytes for every round of the task. Then the record's goal and rules. Everything under that is written anew each turn and is where the cost is.
3. **Everything a tool returns is a book on a shelf.** It gets one line in the record with an id such as r7, its full text goes to the log, and `read r7` brings it back. The window holds the newest results in full and nothing is ever rewritten or summarised.
4. **A skill rides in the prompt as a name and one line.** Its body loads only when it is used. That is how the harness carries two hundred procedures for the price of twenty lines.
5. **The harness checks what it can, so the model does not have to be trusted on it.** A done line that names a file or a command is checked by the harness. An `expect` line on a shell, write or edit call is checked by four rules: a test count, an exit code, a contained string, parses. Tests rerun after every edit once the model has run them. Rounds without progress climb a ladder, and a repeated call is refused.
6. **A job is a record whose plan is a task list.** Its ask is held whole in a checkpoint. Tasks run one at a time, each task's report goes into the job with an id such as j4.2, and the next task starts.

The rule this set follows: give the harness things it already has a shape for. An ask that arrives in the record's shape is lifted straight in. A document the harness can shelve is read by name, not pasted.

## 2. The ask as an operations order

### 2.1 The Tetris ask, read the way the harness reads it

`TETRIS_TEST_PROMPT.md` is 735 lines, 2,898 words and thirty headings. The outcome is on line 8. The finish, twenty-nine acceptance bullets, is on line 678. Test-driven development is demanded in four places. There is no stop list and there is no why. It is a good ask for a person; it is not in the record's shape, so the harness cannot lift any of it, and the model does the lifting: on run fifteen it read the whole ask and in five rounds wrote a job of twelve tasks, each task a line of its own words. That works. Three things about it are worse than they need to be.

- **The finish is the model's translation.** The person wrote twenty-nine bullets; the job's done list is whatever the model made of them, and the harness can check none of it, because none of the bullets names a command or a file.
- **The rules travel by luck.** "Tests first", "never a headless browser", "not port 8090" live in the body of the ask. They reach task nine only if the model carried them into the words of task nine, which on run fifteen it did by naming every task "TDD ...".
- **Every task reads the whole ask.** The ask rides in the job summary, about four thousand tokens, cached for the rounds of a task but read once at every task start. The part about the task in hand is a tenth of it, and the other nine tenths are in front of the model while it works.

### 2.2 The template: seven headings, each with a place in the record

A person writes seven headings, in this order, in plain words. Each one is a part of the record, so the harness knows what to do with it.

| Heading | What goes under it | What it becomes | What the harness does with it |
|---|---|---|---|
| **Outcome** | One paragraph: what exists when this is done, for whom, where | The job's name and its why | Prints it in the job summary of every task; the why is what the model uses when a plan breaks |
| **Done when** | Numbered lines, each one checkable; a line the harness can check carries the check in brackets | The job's done list | Checks the bracketed lines itself at the end of every task and at the finish; the model cannot declare the job done |
| **Stop if** | The things that must reach the person before work goes on | The job's stop list | Adds its own two lines (budget, login page), watches the rest through the `task` tool |
| **Rules** | The standing constraints for this job: tests first, the language, the ports, what not to touch | Corrections C1, C2, ... in the job record, in the person's words | Rides in the cached front of every task of the job; never rewritten |
| **Steps** | The order of work, one line each, optional | The job's task list | Makes the job with these tasks and no planning rounds; the model may split or add a task later, never remove a finished one |
| **Read first** | Files that carry the working context: the architecture page, a style guide, a spec | Shelved results with ids, opened in the first window; a short one pinned for the job | Never has to be found by the model |
| **Details** | Everything else, as long as it needs to be, under its own headings | Shelved by heading, read with `read ask dragon` | Shows a task the sections its step names in full and the rest by heading and first line |

Why these seven and not more: they are the record's own parts, in the order an operations order gives them, situation and mission first, execution next, and what must be reported at once. A heading that has no part in the record would be text the harness can only paste.

The checks in brackets are the four rules the `expect` line already has, plus one: `[tests pass: npm test]`, `[exit 0: node build.js]`, `[shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`, `[parses: src/game.js]`, `[exists: dist/index.html]`. A line without a check is judged by the model against a result it must name, as today.

### 2.3 The Tetris ask, rewritten

Everything the original says is kept; what moves is where it sits.

```
# Tater Tots Tetris

## Outcome
A complete, polished, playable Tetris-style web game in <WORK>/Tater Tots Tetrisv1,
branded Tater Tots Tetris, with normal Tetris play, two hazards (a dragon that locks
the piece after a warning, a yeti that blows the piece sideways from the left or the
right) and three line-clear effects (freeze and disintegrate, bomb, fire). It is for
players in a browser, and every rule of it is proved by a test before it is built.

## Done when
1. Every automated test passes and none is skipped. [tests pass: npm test]
2. The game loads and shows the board. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]
3. A whole game has been played in the Chrome window to game over, with a photograph
   of each state: play, dragon warning, dragon attack, yeti from each side, each of the
   three clears, pause, game over, restart.
4. The layout has been looked at in Chrome at five window sizes with no overlap, no
   clipping and no console errors.
5. The dragon, the yeti and the three effects each have a test file. [exists: test/dragon.test.js]
6. Restart resets everything, and no hazard timer survives it. [tests pass: npm test -- restart]
7. Every developer control used for testing is hidden in the finished game.

## Stop if
- A test cannot be made to pass after three different fixes.
- The Chrome window cannot open the page.
- The engine needs a framework or a build step to work.

## Rules
- Tests first: write the test, watch it fail, write the code, watch it pass; run the
  whole suite at every milestone and go on only when all of it is green.
- Plain HTML, CSS and JavaScript. No framework. No build step.
- Serve on any port but 8090. The browser is the Chrome window on the screen, never
  a headless one from a script.
- Hazard values live in one config file: probabilities, warning times, forced-drop speed.
- The random source is injectable, so any hazard can be forced in a test.

## Steps
1. Scaffold: package.json, a test runner, index.html, a smoke test. (Details: Core game)
2. The engine: board, pieces, movement, rotation, collision, locking, lines, scoring,
   levels, speed, game over, restart, pause, preview, high scores. (Details: Core game)
3. The hazard state machine and the config file. (Details: Hazard architecture)
4. The dragon. (Details: Dragon)
5. The yeti, both sides. (Details: Yeti)
6. The three line-clear effects. (Details: Line clear effects)
7. Hazard safety: no stale timers, no piece lost, restart clean. (Details: Hazard safety)
8. The browser shell, the HUD, keyboard, the developer controls. (Details: Core game)
9. Play it in Chrome and fix what the play shows. (Details: Browser play testing)
10. Visual QA at five sizes and polish. (Details: Visual QA, Visual polish)
11. Final regression: the whole suite, then a last visual pass, then hide the controls.

## Read first
- none; the folder is empty

## Details
### Core game
(the original's core requirements, word for word)
### Dragon
(the original's dragon section)
### Yeti
...
### Line clear effects
...
### Hazard architecture
...
### Hazard safety
...
### Browser play testing
...
### Visual QA
...
### Visual polish
...
```

That is the same 2,898 words, in an order the harness can lift. The finish is the person's, in seven lines, five of them checked by the harness. The rules are five lines that reach every task. Each step names the details it needs.

### 2.4 What the harness builds for it

Four things, each small, and a free-form ask still works exactly as today.

1. **The lift.** The seven headings are recognised by name. Outcome becomes the job's name and why, Done when its done list with the checks attached, Stop if its stop list, Rules its corrections, and Steps its task list, so the job exists before the first model call and the planning rounds go to nought. The model may still split a task or add one, as it can today.
2. **The finish checked by the harness.** At the end of every task of the job the bracketed done lines are run, and the job summary carries the count: "done lines proved: 3 of 7". At the finish, every bracketed line must pass and every other line must name a result, as the done check requires today. Three of the last eight nightly runs closed done on a job that did not work; a finish the harness runs cannot be talked past.
3. **Details shelved by heading.** The ask stays whole in the job's checkpoint, never rewritten. Each Details section is shelved like a result, under its heading, and `read ask dragon` brings it back. A task's front carries the sections its step names in full and every other section as its heading and first line. On the Tetris job that is about three hundred words of ask per task instead of three thousand.
4. **Read first opened by the harness.** Each file named is shelved with an id and opened into the job's first window; a file under sixty lines is pinned for the whole job, so a style guide or an engine's interface is in front of every task without being found.

- Evidence: the Tetris ask read whole at twelve task starts; 297 record-writing rounds in the 6 September logs; three of eight nightly runs closing done on a job that did not work; run twelve's rules reaching task nine only through the model's own task names.
- Measure: tokens at each task start; record-writing rounds per task; done lines proved by the harness against lines judged by the model; jobs closed done that the nightly check then failed.
- Cost: three days.

### 2.5 What this gives a long-running project

The person defines the finish once and the harness checks it after every task, so a job that runs all night is measured all night, not judged at the end. The rules cannot be lost at task nine, because they ride in the cached front of every task, in the person's words. Each task works from its own slice of the ask, and reads the rest by name when it wants it. And because the ask, the rules and the finish live in the job record, a job put down on Tuesday is picked up on Friday, or on another model, with all three intact, which the record already promises and the template makes worth having.

## 3. The project's documents as working context

### 3.1 The rule: shelve, do not paste

`REPO_MAP.md` in this repository is 1,812 lines and 60 kilobytes. `ARCHITECTURE.md` is 1,909 lines and 431 kilobytes. Claude Code pastes `CLAUDE.md` into every prompt, Codex pastes `AGENTS.md`, aider pastes a ranked map of the code. On a small model every one of those is the task's own room given to a document. The harness already has a better rule for anything large, the library rule: one line in the record, the full text on the shelf, read by its call number when wanted. A project's documents are three more books on that shelf, with fixed call numbers, and the model sees one line for each and the slice its task names.

So the three documents enter the prompt in three ways, none of them whole.

| Document | Its line in every prompt | What loads when | Who keeps it true |
|---|---|---|---|
| `AGENTS.md`, the standing order | "project: Tater Tots Tetris, a browser Tetris in plain JS" | Its body, under sixty lines, under the job summary for any task in that folder, loaded like a skill's body | The person; the harness appends what it learns |
| `REPO_MAP.md`, where everything is | "map: 14 files in 3 folders, 2 changed this task; read map" | A slice in the orientation: the files of this task; the whole map by `read map` | The harness, regenerated after every write and edit |
| `ARCHITECTURE.md`, how it is put together | "arch: engine, hazards, effects, shell, tests; read arch engine" | The sections a step names in full; a section by `read arch engine`; the contents line always | The person, and the harness at each task's end |

### 3.2 `AGENTS.md`: the standing order, loaded like a skill

The standing order is what is true of the project on every task: what it is in one paragraph, how to run it and how to test it as commands, the rules that hold on every task, and where things are as a table of contents pointing at the other two documents. Under sixty lines, because a giant rules file crowds out the task, which is what OpenAI found in its own harness. The name is the one the industry already uses, so a project that has one for Codex has one for Nerd Genie.

Three things the harness does with it:

- **Loads it the way it loads a skill.** One line rides in every prompt; the body loads under the job summary for any task whose folder holds the file, byte-stable for the whole task, so the daemon's cache keeps it.
- **Runs its test command at every task start.** Anthropic's note on long-running agents has every session run a smoke script before touching code, because the failure modes of long work are premature done, undocumented progress, untested features and starting from scratch. Here the standing order's test line is that script: the harness runs it as the task's first result, the test reader knows the count from round one, and a task that begins on a red suite begins knowing it.
- **Keeps what it learns about this project here, not in the global memory.** The review at a task's end saves one lesson today, into `MEMORY.md`, which is one file for the whole machine. On 7 September run twelve read "task t7 browser shell complete at Tater Tots Tetrisv1" from that file, went looking for a build that had been archived, and stopped. A lesson about a project belongs to the project: the review's keep-or-change answer goes under `## Learned` in the folder's `AGENTS.md`, dated, capped at twenty lines, and a new folder starts with none. The global memory keeps facts about the machine and the person, which is what it was for.

- Evidence: run twelve's stop; the rules of a job carried only in the model's task names; the count of test rounds at task starts.
- Measure: tasks that begin with a test run of their own; lessons written to the project against lessons written to the global memory; stops caused by stale memory.
- Cost: two days.

### 3.3 `REPO_MAP.md`: a map the harness keeps, searched before it is read

Today the map is a tree of paths with a legend for the roots, generated from git. The model has a `search` tool (one pattern, files and lines, fifty rows) and a `read` tool (a file, a folder, a result by id), and it uses them well; what it lacks is the one thing a person has that it does not, a sense of what each file is for and which files belong together. That sense is the map, and the harness can keep it because every change to a file goes through the harness.

The map becomes a small table in the same SQLite file as the log and the memory: the path, the size, a one-line purpose, the names the file defines, and the task and round that last touched it. The purpose is the file's first comment or heading; for a file the harness wrote, it is the plan step it was written under, so the model is asked for nothing. The names come from one regular expression per language, the way the test reader reads test output, not from a parser. `REPO_MAP.md` is the table written out for people and for git, one line per file, regenerated after every write and edit.

Four things the model gets from it:

- **A slice in the orientation.** The orientation block already says what is in the working folder and which ports are listening. It gains "the files of this task", one line each: path, purpose, names, last touched by t4 at r41. At a task start inside a job the same section says what the job has built so far, which is the thing the model now reads its own files to learn.
- **`read map`.** The whole map by its call number, when the model wants the whole picture, and it leaves the window like any other result.
- **`search` answers from the map first.** A name goes to the names table and comes back as `file:line` with the file's purpose beside it; only a pattern that names nothing goes on to ripgrep. A question in words goes to a full-text index of the folder's text files kept in the FTS5 table the memory already runs, and comes back the same shape. The model asks "where is that" as it does today and gets an answer that says what the file is for.
- **The change list for free.** The last-touched column is what a review should read instead of the transcript: every file the job changed, by task and round.

Borrowed design: aider's repository map, which ranks the code's names by how often the files in the conversation refer to them and puts the top slice in the prompt. Ours keeps the ranking out and the map out of the prompt; the slice is chosen by three plain facts, the files the task's step names, the files the task touched, and the files named in the model's last three calls.

- Evidence: one task read the same file forty-eight times; 84 rounds repeated a command, most of them searches for something found before; 2.6 reads by id after every restart and 1.6 after every rewind, which is the model rebuilding this picture by hand.
- Measure: repeated reads of an unchanged file per task; shell rounds whose command is `ls`, `find` or `grep`; reads in the three rounds after a task start or a fresh window.
- Cost: three days.

### 3.4 `ARCHITECTURE.md`: read by section, written by the harness at each task's end

The architecture page is the one document a person cannot do without on a code base and the one no model can read whole. It becomes a shelved book with a table of contents: `read arch` prints the section names with their sizes, `read arch engine` prints one section, and `search` reports which section mentions a word. Every prompt carries the contents line. A task's front carries in full the sections its step names, by the same rule as the ask's details: the dragon task reads the hazards section and the engine's interface, and nothing about rendering.

The page is kept true by the harness, which is the part nobody else does. The review at the end of every job task already asks four questions with the tools off; where the folder has an architecture page it asks a fifth: "which section does this task change, and what is its paragraph now?" The answer goes under that heading, dated, and the section's old paragraph stays in the log. A page written that way is a record of what was built, task by task, rather than a page somebody remembers to update, and the next task reads two hundred words chosen for it instead of sixty thousand. A fresh window after a cut or a cap opens on the same sections, which is the bearing the model re-derives by reading today.

- Evidence: 121 rounds, 820 thousand tokens and 33 minutes of re-orientation after rewinds and restarts on 6 September, every one of them beginning with reads; the architecture page of this repository is kept by hand and drifts between waves.
- Measure: uncached tokens and reads in the three rounds after each task start and fresh window; sections of the page older than the last task that touched their files.
- Cost: two days.

## 4. The guide: the three documents for any project

The harness can keep the books for any folder that carries the three documents, and they are the same three whether the folder holds code, a book, a video pipeline or a campaign. A person writes the first two in twenty minutes; `nerdgenie map` writes the third.

**`AGENTS.md`, the standing order.** Five headings, under sixty lines: What this is (one paragraph), Run (the commands), Test (the commands, which the harness runs at every task start), Rules (what holds on every task), Where things are (a table of contents pointing at the other two). The harness adds Learned.

**`ARCHITECTURE.md`, how it is put together.** One section per part. Each section says what the part is for, what it holds, what it depends on and what depends on it. Two hundred words a section is plenty; the harness keeps them current.

**`REPO_MAP.md`, where everything is.** Generated, never by hand. The one thing it asks of a file is that its first line says what the file is. A file without one is listed by its name, or by the step that wrote it.

What they look like away from code:

- **A book.** The parts are the chapters and the notes. The map lists each chapter file with its first line, its scenes as names, and the draft that last touched it. The architecture page holds the arc, the people and the rules of the world, one section each. Done lines read `[contains: "the harbour" in ch12.md]` and a word count on `wc -w`. The slice for a task on chapter twelve is chapters eleven to thirteen and the people in them.
- **A video pipeline.** The parts are ingest, cut, grade and render. The map lists clips, scripts and presets with a purpose each and the stage that wrote them. The architecture page says the stages, the formats between them and where renders land. The standing order's test line is a render of a ten-second sample. Done lines read `[exit 0: render.sh]` and `[exists: out/final.mp4]`. The slice for a task on the grade is the cut's section, the render's section and the presets it may use.
- **An advertising campaign.** The parts are the audiences, the channels and the assets. The map lists briefs, copy variants and images with a purpose line and the task that made them. The architecture page holds the funnel, the voice rules and the calendar. Done lines read `[contains: "call to action" in ads/variant-a.md]` and a length rule; a stop line reads "any claim about price". The slice for a task on one channel is that channel's assets and the voice rules, and nothing about the others.

The point of the four examples is that the harness needs to know nothing about books, film or advertising. It needs a first line on every file, a section per part, a test command, and an ask with seven headings.

## 5. Two commands, and what not to do

- `nerdgenie map <folder>` writes `REPO_MAP.md` for any folder, and the skeletons of the other two documents when they are absent.
- `nerdgenie check <folder>` runs the bracketed done lines of the folder's `ASK.md`, which is the ask kept beside the work, and prints which pass. The ask becomes the project's acceptance test, for a folder of ad copy as well as a game, and the nightly table's check column and `nerdgenie bench` read the same lines.

Not to do: paste any of the three documents whole into a prompt; build a parser per language, when one regular expression and a first line make a map; rewrite the ask, which is the person's words and is held whole in the checkpoint; make the template mandatory, since a person types what they type and the seven headings are a way to be understood at once.

## 6. Order, cost and measure

1. The guide (section 4): a day of writing, no code, and it makes the rest concrete.
2. The ask (section 2): three days. The biggest gain, and it needs no index.
3. The standing order (3.2): two days.
4. The map (3.3): three days.
5. The architecture page (3.4): two days.

Eleven working days, each step measured by the nightly table before the next begins.
