# Ideas, second set: a standard ask, and a map the harness keeps

Written 7 September 2026, the morning the three-bit model on rosie ran the whole Tetris job to the end on its own. The harness now holds a small model on course. The next question is how to get more out of the same model, and this set answers it in two parts: shape the person's ask the way the record is shaped, and give the harness, not the model, the job of knowing where things are. A third part is the guide: the three documents any project carries so the harness can do that, whether the project is a code base, a book, a video pipeline or an advertising campaign.

Nothing here is built. The owner approves an idea before it is built, it is built test first, and it lands in `docs/PROGRESS.md` with its measured effect. Every idea has to hold on any task and on the local model, and each one has to pass the two rules: the simplest thing that works, and nothing until somebody needs it.

## The one idea behind all of them

The model's window is small and dear. The harness's disk is large and exact. Whatever can be computed should be computed by the harness and shown to the model as a slice, only when it matters, and never asked of the model. The harness already does this for the working folder's listing, the ports that are listening, the newest results, the state of the tests and the step line at the end of every prompt. The 6 September measurements say where the next slices are: 297 of 2,173 rounds did nothing but write the record; 121 rounds after rewinds and restarts were the model finding its bearings again; one task read the same file forty-eight times; 84 rounds repeated a command already run. Each of those is the model doing by hand what the harness already knows.

## Part 1: the standard ask

### What the record is shaped like

The record is an operations order, and its parts are fixed: the goal (the ask word for word, a name, a why, and the done lines), the rules (corrections and the stop lines), the work (the situation, the plan, the job's tasks, and the results), and the lessons (decisions and failures). An Army order has the same bones: situation, mission, execution, what must reach the commander. The shape is what lets a small model pick a task up cold and know where it stands.

### What the Tetris ask is shaped like

`TETRIS_TEST_PROMPT.md` is 735 lines, 2,898 words, thirty headings. The outcome is on line 8. The acceptance criteria are on line 678. Test-driven development is demanded in four places. There is no stop list at all. The model reads the whole of it at the start of every task of the job, most of it about the other tasks, and then spends rounds turning it into the record's parts. It does that well now (run fifteen's planning took five rounds), but it does it with the model's tokens, and every task start re-reads the parts of the ask that belong to other tasks.

### The template

Seven headings, plain words, in this order. A person writes them; the harness recognises them by name.

1. **Outcome.** One paragraph: what exists when this is done, for whom, and where (the folder, the site, the file).
2. **Done when.** Numbered lines, each one checkable. A line the harness can check itself carries the check in brackets, using the four rules the `expect` line already has: `[tests pass: npm test]`, `[exit 0: node build.js]`, `[shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`, `[parses: src/game.js]`.
3. **Stop if.** The things that must reach the person before the work goes on.
4. **Rules.** The standing constraints: tests first, the language, the ports, what not to touch, the house style.
5. **Steps.** The order of work, one line each. Optional; the model plans when it is absent.
6. **Read first.** The documents that carry the working context: the architecture page, the style guide, a spec.
7. **Details.** Everything else, as long as it needs to be, under its own headings.

The Tetris ask, rewritten, begins like this:

```
# Tater Tots Tetris

## Outcome
A complete, polished, playable Tetris-style web game in <WORK>/Tater Tots Tetrisv1,
with a dragon that locks the piece and a yeti that blows it sideways, three line-clear
effects, and every rule proven by automated tests before it is built.

## Done when
1. Every automated test passes. [tests pass: npm test]
2. The game loads and shows the board. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]
3. A full game can be played in the browser to game over, with a photograph of each state.
4. The dragon, the yeti and the three line-clear effects each have their own test file.

## Stop if
- A test cannot be made to pass after three different fixes.
- The browser cannot open the page.

## Rules
- Tests first: write the test, watch it fail, write the code, watch it pass.
- Plain JavaScript, no framework, no build step. Serve on any port but 8090.

## Steps
1. Scaffold and a smoke test. 2. The engine. 3. The dragon. 4. The yeti.
5. Line-clear effects. 6. Browser play test and visual check. 7. Polish.

## Details
### Core game ...
### Dragon ...
```

### What the harness does with it

This is the build. A free-form ask still works exactly as today; the template only adds.

- **The record is written by the harness at task start.** The outcome becomes the goal's name and why. Each done line becomes a numbered done line in the record, with its harness check attached, so the check runs itself and the line is proved without a round of argument. The stop lines become the stop list. The steps become the job's tasks, handed to the planning call as a draft it may keep or change. Rounds the model spends writing the record go to nought for a templated ask.
- **The details are sliced per task.** A task named "Dragon" gets the details section whose heading matches it in full, and every other section as a heading and a first line. A model that needs another section reads it by name. For the Tetris ask this cuts about 2,500 words from every task start after the first.
- **The rules ride in the per-task front, word for word,** in the place the job summary sits, so they are the same bytes for every task of the job and the daemon's cache keeps them.
- **The read-first list is opened by the harness** into the first window of the job, and the map (part 2) knows those documents by name after that.

Why this is new: other agents take a prompt and a rules file. Here the ask has the same bones as the record that will carry the work, so the harness can lift it straight in, check the done lines itself, and show each task only its own part of the spec.

- Evidence: 297 record-writing rounds in a day of runs; the 2,898-word ask re-read at every task start; three of the last eight nightly runs closing "done" on a job that did not work, which harness-checked done lines address.
- Measure: record-writing rounds per task; uncached tokens at each task start; done lines with harness proof against lines only the model judged.
- Cost: two days.

### The ask that lives with the project

A small step past the template: keep the ask in the project as `ASK.md`, and give `nerdgenie check` one job, to run the done lines' checks against the folder as it is now. The ask becomes the project's acceptance test, and it works for a folder of ad copy as well as a game: "is this still done?" is one command. The nightly table's check column and `nerdgenie bench` read the same lines.

## Part 2: a map the harness keeps

### What the two documents are today

`REPO_MAP.md` is generated: a legend of one line per root folder and a tree of every tracked path, 1,812 lines and 60 kilobytes. `ARCHITECTURE.md` is written by hand: 1,909 lines and 431 kilobytes of prose on how the code is put together. Neither can go in a prompt. The front of the prompt is ten to fourteen thousand tokens already, and the architecture page alone would be a hundred thousand. So the question is not "show the model the map". It is "let the harness answer from the map and show only the slice", which is what the model already gets for the folder listing and the ports. And the model already has a `search` tool (one pattern, files and lines, capped at fifty rows) and a `read` tool (a file, a folder, a past result). The map makes those two answer better; it does not add a third.

### 2.1 The map is an index, not a file

At task start, and after every write and edit, the harness keeps a table of the work folder in the same SQLite file as the log and the memory: the path, the size, the kind, a one-line purpose, the names the file defines, and the task and round that last touched it. The purpose is the file's first comment or heading; when the harness itself wrote the file, the purpose is the plan step it was written under, so nothing has to be asked of the model. The names come from one regular expression per language, the way the test reader reads test output, not from a parser: `func`, `function`, `class`, `def`, a Markdown heading, a scene heading in a script. The map stays true because every change to a file goes through the harness.

Borrowed design: aider's repository map, which ranks a code base's names by how often the files in the conversation refer to them and puts the top slice in the prompt. Ours keeps the ranking out and the map out of the prompt: the slice is chosen by three plain facts, the files this task's ask section names, the files this task has touched, and the files the model's last three calls named.

- Evidence: one task read the same file forty-eight times; the folder listing and `ls -la` rounds at every task start; the model reading its own files back after a fresh window (2.6 results by id after a restart).
- Measure: repeated reads of an unchanged file per task; shell rounds whose command is `ls`, `find` or `grep`.
- Cost: three days, with 2.2.

### 2.2 The orientation shows the slice

The orientation block already says what is in the working folder and which ports are listening. It gains one section, "the files of this task", one line each: the path, the purpose, the names, and "last touched by t2 at r41". At a task start inside a job the same section says what the job has built so far, which is the thing the model now reads its own files to learn. The section changes only when a file changes, so the daemon's cache holds it across the rounds between. Nothing else in the prompt moves.

### 2.3 "Where is that" answered from the index

The `search` tool keeps its one field. Behind it, a name is looked up in the map's names table first and answered as `file:line` with the file's purpose beside it, and only a pattern that names nothing goes on to ripgrep. A question in words ("where is gravity timed") goes to a full-text index over the folder's text files, kept in the same FTS5 table the memory already runs, and comes back as the lines that match with their files' purposes. The answer is the same shape either way. This is cheap because the full-text engine is already in the binary and the memory index already proves it on every run.

- Evidence: the 84 repeated-command rounds are mostly the model searching for what it found earlier; a search whose answer carries no purpose line is followed by a read.
- Measure: searches followed by a read of the file that was just searched; search rounds per task.
- Cost: one day, after 2.1.

### 2.4 The architecture page as the project's situation, kept by the harness

`ARCHITECTURE.md` is too big to read and too important to skip, and today it is kept true by hand. Two moves make it the project's living situation report.

First, the harness keeps an architecture card per part of the project: one paragraph per top folder. At the end of each task in a job, one call with the tools off asks the model, "what did this task add or change, in one paragraph, for the architecture page of the part it touched?" The paragraph goes under that part's heading in `ARCHITECTURE.md` and into the map. That is one short round per task, in the place the review call already sits, and it makes the page a record of what was built rather than a page somebody remembers to update.

Second, the per-task front shows only the paragraphs for the parts this task's files belong to, two hundred words instead of sixty thousand. A task on the dragon reads the engine's paragraph and the hazards' paragraph and nothing about rendering. A fresh window after a cut or a cap opens on the same paragraphs, which is the bearing the model re-derives today by reading.

- Evidence: 121 rounds, 820 thousand tokens and 33 minutes of re-orientation after rewinds and restarts on 6 September; every one of those began with the model reading files to learn what existed.
- Measure: uncached tokens and reads in the three rounds after each task start and fresh window.
- Cost: two days.

Why this is new: Claude Code reads `CLAUDE.md`, Codex reads `AGENTS.md`, and aider puts a map in the prompt. All three hand the model a document. Here the documents are the human-readable face of an index the harness keeps and answers from, and the model sees two hundred words chosen for the task in hand.

## Part 3: the guide, three documents every project carries

The harness can keep the books for any folder that follows three small conventions. They cost a person a few minutes and they are the same for a code base, a book, a video pipeline and a campaign.

1. **`AGENTS.md`, the standing order.** What this project is, in one paragraph. How to run it and how to test it, as commands. The rules that hold on every task. Where things are, as a table of contents pointing at the other two documents, never as the whole story: a giant rules file crowds out the task, which is the lesson OpenAI drew from its own harness. Under sixty lines. The harness reads it into the front of every task in that folder, the way the plan for the first release already intends.
2. **`ARCHITECTURE.md`, how it is put together.** One section per part, kept by whoever changes a part, which from 2.4 on is the harness as often as the person. Each section says what the part is for, what it holds, and what it depends on.
3. **`REPO_MAP.md`, where everything is.** Generated, never by hand: the map of 2.1 written out as a file, so a person and git can read it. The one convention it asks of files: the first line of each file says what the file is. A file without one gets its purpose from the step it was written under or is listed by name alone.

What the three look like away from code:

- **A book.** The parts are the chapters and the notes. The map lists each chapter file with its first line, its scenes as names, and the draft that last touched it. The architecture page holds the arc, the people, and the rules of the world, one section each. A done line reads `[contains: "the harbour" in ch12.md]` or a word count. The harness's slice for a task on chapter twelve is chapters eleven to thirteen and the people in them.
- **A video pipeline.** The parts are ingest, cut, grade and render. The map lists clips, scripts and presets with a purpose each and which stage wrote them. The architecture page says the stages, the formats between them and where renders land. Done lines read `[exit 0: render.sh]` and `[exists: out/final.mp4]`. A task on the grade is shown the cut's paragraph and the render's, and the presets it may use.
- **An advertising campaign.** The parts are the audiences, the channels and the assets. The map lists briefs, copy variants and images with a purpose line and the task that made them. The architecture page holds the funnel, the voice rules and the calendar. Done lines read `[contains: "call to action" in ads/variant-a.md]` and a length rule. A stop line reads "any claim about price". The harness's slice for a task on one channel is that channel's assets and the voice rules, and nothing about the others.

A `nerdgenie map` command that writes the three skeletons and the first map for any folder is one afternoon, but it waits for the second project that wants it. The guide can be written now, and it costs nothing.

## Further out, one line each

- The QA skill fills an app map the same way: pages, their titles and the elements it found, so a later task on the same site starts oriented.
- The map's "last touched" column is the seed of a job-scoped change list, which is what a review call should read instead of the transcript.
- The purpose line is the one convention that makes any folder mappable, so the write tool's description asks for it, and nothing enforces it.

## What not to do

- Do not put the map or the architecture page in the prompt. The slice is the whole point.
- Do not build a parser per language. One regular expression per language for names, and the first line for the purpose, is enough for a map; the syntax check is the net for correctness.
- Do not summarise the conversation into the architecture page. Compaction was measured worse than truncation; the card is written from the record and the task's files, not from the transcript.
- Do not make the template mandatory. A person types what they type; the seven headings are a way to be understood at once.

## Order and cost

1. The guide (part 3): a day of writing, no code, and it makes the rest concrete.
2. The standard ask (part 1): two days; the biggest gain and it needs no index.
3. The index and the orientation slice (2.1 and 2.2): three days.
4. Search from the index (2.3): one day.
5. The architecture card (2.4): two days.

Nine working days in all, each step measured by the nightly table before the next begins.
