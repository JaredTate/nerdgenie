# The plan for IDEAS_V2: what changes, why, and how it improves the harness

Written 7 September 2026, rewritten the same evening to say what, why and how in plain words. It waits for the owner's yes. Every step is a failing test, then the code, then a measured Tetris run before the next step. The rule for every step: the simplest thing that passes the test.

## The problems we are fixing, with the numbers

These come from the 6 September logs (three runs, 32 tasks, 2,173 rounds) and from runs twelve to fifteen today.

1. **Re-orientation.** After every task start, every cut and every fresh window, the model re-learns the project by reading files: 121 rounds, 33 minutes, 820 thousand tokens in one day. After a restart it reads 2.6 results back by id before it does anything.
2. **Planning and bookkeeping.** 297 rounds did nothing but write the record. The planning task of every Tetris run reads the whole 2,898-word ask and writes a job in its own words.
3. **Rules by luck.** The ask's rules reach task nine only if the model carried them into its own task names. A project's house rules (Home Recon's `CLAUDE.md`) sit in a file nothing points at, so the model never sees them.
4. **Finding where things are.** 84 rounds repeated a command already run, most of them `ls`, `find` and `grep`. One task read the same file forty-eight times.
5. **Premature done.** Three of the last eight nightly runs closed done on a job that did not work.

Six steps. Each names the problem it fixes, what changes, how it improves things, and where we see it in the numbers.

## Step 1: the work order becomes the record (fixes 2 and 3)

**What changes.** A new package `internal/workorder` reads the six headings of an ask. When an ask has them, the harness writes the job's record itself before the first model call: name and why from Goal, the done lines with their checks from Done when, the rules into the record's rules, the tasks from Tasks. A plain ask is untouched.

**How it improves.** No planning rounds. The finish is the person's seven lines, not the model's translation of twenty-nine bullets. The rules sit in the record's rules section, which rides in front of every task, cached, so they cannot be lost at task nine.

**Tests first.** The parser: each heading; a plain ask is not a work order; a done line keeps its check; a task names its Details sections; a golden on `TETRIS_PROMPT_V2.md`; a fuzz target. The lift: the first model request already carries the job with fourteen tasks and seven done lines and no `job` call was made; the existing goldens for a plain ask hold byte for byte.

**We see it in.** Planning rounds, from five to zero. Record-writing rounds per task.

**Size.** About three hundred lines. Two days.

## Step 2: each task sees only its own part of the ask (fixes 1 and 2)

**What changes.** The per-task front shows the task's line and the Details sections its line names, in full. The other sections show as one heading each, with "read one with `read ask <heading>`". The read tool learns the label `ask` followed by a heading. The ask stays whole in the job's checkpoint.

**How it improves.** The dragon task reads six hundred words about the dragon instead of three thousand about everything. Less to read at every task start, and the model's attention is on the part it is building.

**Tests first.** A context golden for a task naming `(Details: Dragon)`; a task naming nothing sees every heading by line; `read ask dragon` returns the section; an unknown heading lists the headings.

**We see it in.** Uncached tokens at each task start. Reads of the ask by section.

**Size.** About eighty lines. One day.

## Step 3: `AGENTS.md` in front of every task (fixes 3)

**What changes.** At task start the harness looks in the Where folder for `AGENTS.md`. When it is there, its text rides under the job summary on every task in that folder, capped at sixty lines with a line saying so when it is longer. The ask's own rules come after it and win.

**How it improves.** The house rules of a project are always in front of the model, in the project's words, for zero rounds. On Home Recon that is "never push main, rebuild core after editing, test:node before push". A rule the model always sees is not broken by accident. This is exactly what the scalpel does with its house rules, and it is part of why scalpel tickets land well.

**Tests first.** A context golden with the block under the job summary; a folder without the file adds nothing; the sixty-line cut; the cache-shape test still holds, because the block is inside the stable prefix.

**We see it in.** Rounds spent reading rule files. Rules broken and corrected by the owner, which the nightly notes record.

**Size.** About forty lines. Half a day.

## Step 4: `ARCHITECTURE.md` read by section, and `REPO_MAP.md`'s legend (fixes 1 and 4)

**What changes.** Two lines in the orientation block, the block that already lists the folder and the ports at every task start and fresh window:

- "ARCHITECTURE.md: sections engine, hazards, effects, shell, tests. Read one with `read ARCHITECTURE.md hazards`."
- The map's roots legend, when `REPO_MAP.md` has one: one line per top folder with what it is for, fifteen lines at most.

And the read tool takes a heading after a Markdown file's path and returns that section only.

**How it improves.** When the model needs to know how a part is put together, it reads two hundred words in one round instead of opening files for three to five rounds. When it needs the lay of the land on a large repository, it already has it: the four apps and the packages of Home Recon are fifteen lines in the orientation, not five rounds of `ls`. Nobody reads the 6,192-line map; the search tool finds files by name as it does today.

**Tests first.** An orientation golden with both lines for a folder with the two files, and a golden without them; the read tool reads a section, is bounded like a file, and has a fuzz target on the heading match; the section cutter lives in a tiny `internal/markdown` package shared with step 2.

**We see it in.** Reads in the three rounds after a task start or a fresh window, and how many of them are reads by section.

**Size.** About a hundred and twenty lines. One day.

## Step 5: the documents are written by the job (fixes 1 for the next job)

**What changes.** The review at the end of every task already asks four questions with the tools off. When the folder has `ARCHITECTURE.md`, it asks a fifth: which section did this task change, and what should it say now? The answer replaces that section, dated. When the folder has no `ARCHITECTURE.md`, the first task's review starts one. When a job finishes in a folder with no `AGENTS.md`, the harness writes one from the work order: what this is from Goal, the test command from the first done line, the Rules, and a pointer to the architecture page. It never overwrites a file that exists.

**How it improves.** The Tetris folder ends the night with an architecture page whose hazards section says what task five built, and an `AGENTS.md` that says `npm test`. The next task of the same job, and the next job on the same folder, start from a page instead of from reading files, which is problem 1 again.

**Tests first.** After a task in a folder with the page, the named section holds the answer, dated; a folder without the page gets one; an answer naming no section changes nothing; `AGENTS.md` is written once and never overwritten.

**We see it in.** The two files in the game folder at the end of a run, and the reads in the three rounds after a task start in the run after.

**Size.** About a hundred lines. One day.

## Step 6: proof at every task's end (fixes 5)

**What changes.** Two things. A write or edit to a code file, when no test has failed since the last green run, gets one line on its own result: "tests first: no failing test covers this change; write it first." And the bracketed checks of the Done when lines run at the end of every task and at the finish, through the done check's existing command runner and the browser: `tests pass`, `exit 0`, `shows`, `exists`. The job summary carries "done lines proved: 3 of 7". The finish is refused while a check fails; after three failures the check's output goes into the record as a failure and the model goes on by another route.

**How it improves.** The finish cannot be talked past, because the person's checks run and the model does not judge them. Tests first is held by the harness, not by the model's memory of a rule. A job that runs all night is measured after every task, not judged at the end.

**Tests first.** The tests-first line appears after a green run and not after a red one, never on a test file, never on a file that is not code, never before the first test run. Each check kind with the fake shell and the fake browser. The finish refused with the line named. The count in the summary.

**We see it in.** Done lines proved by the harness against lines judged by the model. Tests-first lines per task, which should appear early and then stop. Jobs closed done that the nightly check then failed, which should go to zero.

**Size.** About a hundred and eighty lines. One and a half days.

## The order and the runs

| After | Run | What should move |
|---|---|---|
| nothing (tonight) | RA: `TETRIS_PROMPT_V2.md` on today's harness | planning rounds; done lines closer to the person's |
| steps 1 and 2 | RB | planning rounds to zero; task-start tokens down; reads of unchanged files down |
| steps 3 and 4 | RC | reads after a task start and a fresh window down; reads by section appear |
| step 5 | RD | the two files exist in the game folder; the run after starts from them |
| step 6 | RE | done lines proved by the harness up; no finish on an unproved line |

Seven days of building, and one Tetris run on the show home after each pair of steps, the way the owner watches them: a fresh home, memory wiped, `/yolo` on, both windows on the screen, no message to the model until the job ends. Run fifteen is the baseline. The rule for going on: the job finished on its own, every done line proved, the nightly check green, and nothing on the table worse than the run before by more than a tenth. A step that fails the rule is reverted before the next.

## What we are not building

No index of files, no full-text search of the folder, no import graph, no worktrees, no gate line: those are `IDEAS_V3.md`, for large code bases, and they wait until this set is measured. No new tool: sections go through `read`. No parser per language. No configuration option. No summary of anything.
