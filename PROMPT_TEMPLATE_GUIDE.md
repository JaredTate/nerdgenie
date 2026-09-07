# How to write a prompt for Nerd Genie

This guide is for anyone who writes an ask for Nerd Genie: a person at the keyboard, or a larger model writing work for a smaller one. It explains the shape an ask should have, why that shape makes the model do better work, and how the model gets more context when it needs it without being buried in it. `TETRIS_PROMPT_V2.md` is the worked example. `IDEASv2_PLAN.md` is how the harness learns to use the shape.

## 1. Why the shape of a prompt matters here

Start with three facts about how Nerd Genie works.

**The model remembers nothing between calls.** Every time the harness calls the model, the model reads a fresh prompt and has no memory of the last one. Everything it knows about the work is what the harness put in front of it this time.

**The harness builds that prompt from a record, not from the transcript.** The record is a short page: what was asked, why, what done looks like, the rules, the plan, the results so far, the lessons. The harness keeps it and rewrites only its small changing parts. Around the record it puts a window of recent results, in full, newest first. On a local model that window is small: about fifteen thousand tokens, roughly ten pages.

**The harness can check some things itself.** Whether a file exists. Whether a command exited zero. Whether the tests passed, and how many. Whether a page shows some text. Whether a file parses. What it checks, the model does not have to argue about.

Put those together. A prompt is not read once by a mind that remembers it. It is re-read, in pieces, on every call, hundreds of times over a long job, through a window of ten pages. So the question to ask of every line you write is: **who needs this line, when, and can the harness check it?**

Every ask has three kinds of lines in it:

- Lines the model must see on every call: the goal and the rules.
- Lines that say what done means, which the harness can often check itself.
- Lines that only one task needs: the details of one feature.

A free-form prompt mixes the three. The model has to sort them out on every task, by itself, in its small window, and it gets that partly wrong: it buries the goal, repeats a rule in one task and forgets it in the next, and reads the whole spec to find the one paragraph it needs. The template separates the three kinds of lines, so the harness can put each where it belongs: the goal and the rules in front of every task, the done lines into the finish it checks, and the details in front of only the task that needs them.

That is the whole logic. The rest of this guide is the shape, what the harness does with each part, and how to write each part well.

## 2. The shape: six headings

Write the ask under six headings, in this order.

```
# <Name>

## Goal
## Where
## Done when
## Rules
## Tasks
## Details
```

**Goal.** What we are building or changing, for whom, and why. Two to five sentences. This is the one part of the ask the model sees on every single call, so it has to carry the whole intent in a paragraph.

**Where.** New project: the folder. Existing project: the repository, the branch, and the files to start from. The harness opens the folder before the model's first call and looks there for the project's documents (section 4).

**Done when.** Numbered lines. Each one is something that must be true at the end. When the harness can check a line itself, say how, in brackets at the end of the line:

- `[tests pass: npm test]` runs the command and reads the test result.
- `[exit 0: node build.js]` runs the command and reads the exit code.
- `[shows: "Tater Tots Tetris" at http://127.0.0.1:8091]` opens the page in the browser and looks for the text.
- `[exists: dist/index.html]` looks for the file.

A line with no check is judged by the model, and the model must name the result that proves it. Make the first done line the test suite; the harness runs it at the end of every task.

**Rules.** What holds on every task, one line each, in your own words. A line that starts with "Stop" means stop and ask me: the harness ends the task the moment that line comes true. The first rule is always tests first, and the harness writes it there itself, whether or not you did: write the test, watch it fail, write the code, watch it pass, the whole suite green before a task ends.

**Tasks.** The order of work, one line each, each with one done line, in the order they depend on each other. Name the Details sections each task needs, like this: `(Details: Dragon, Tests required)`. A task is one sitting of work, thirty seconds to an hour. If you leave Tasks out, the model plans the job itself, in this same shape.

**Details.** Everything else, as long as it needs to be, under its own headings. Every feature, every list, every rule of the game. This is where the length goes. Say what you want, not how to build it. A framework is fine; say so in Rules if you have a preference.

## 3. What the harness does with each part, and why that helps the model

| Part | What the harness does | Why the model does better |
|---|---|---|
| Goal | Prints it at the top of every task. | When a plan breaks, the model decides from the intent instead of guessing. |
| Where | Opens the folder and the starting files, finds the three project documents. | The model starts oriented instead of spending rounds listing and reading. |
| Done when | Writes the finish into the record with the checks attached; runs the checks at the end of every task and at the finish. | The finish is yours, not the model's translation of it. The model cannot declare the job done early, and it knows exactly what to prove. |
| Rules | Puts them in front of every task, in your words, never rewritten; ends a task the moment a Stop line is true. | Nothing drifts at task nine. Tests first is always there. Dangerous ground stops the work instead of being crossed. |
| Tasks | Makes the job from them with no planning rounds; runs them one at a time; each task sees only its own line. | The model works on one thing with one done line, and reads nothing about the other tasks. |
| Details | Shelves each section by heading; shows a task the sections it names, in full, and the others by heading only. | The model reads the one page it needs instead of the whole spec, and can ask for any other section by name. |

Then every task runs through the same five moves, and the harness checks each one.

1. **Orient.** The model sees the goal, the rules, its own task line, its Details sections, the folder as it is now, and the last task's report.
2. **Test.** The model writes the failing test. The harness runs it and puts the result on the write's own line. Code written before a failing test exists gets one line saying so.
3. **Code.** The model writes or edits. After every change the harness checks the file parses and runs the tests again, and puts both on the change's own line.
4. **Prove.** The model marks the done line with the green run. The harness refuses a mark that points at a run older than the last edit.
5. **Report.** The task's report goes into the job and to you. The next task starts.

Five moves, the same on every task of every project, each with a check the harness makes. That is what makes the work repeatable: the shape does not depend on the model remembering how to work, because the harness holds the shape.

## 4. On-demand context: the three project documents

A project we keep carries three documents. A new project gets them from the job that builds it. An existing project already has them.

- **`AGENTS.md`**: the rules of the project and how to run it and test it. Under sixty lines.
- **`ARCHITECTURE.md`**: how the project is put together, one section per part.
- **`REPO_MAP.md`**: where everything is. Generated, never written by hand.

The model cannot read these whole. A real architecture page is a hundred thousand tokens on a large code base, and a map is thousands of lines. Nor should it: a task on the dragon needs the hazards section, not the rendering section. So the harness handles the three like this.

- **The rules ride in full.** `AGENTS.md` is short and it is the rules, so it sits under the job summary in front of every task.
- **The other two ride as one line each.** In the block that already tells the model what is in the folder and which ports are open, two more lines: "ARCHITECTURE.md has 9 sections: engine, hazards, effects, shell, tests, ...; read one with `read ARCHITECTURE.md hazards`" and "REPO_MAP.md has 41 files in 6 folders; read a folder with `read REPO_MAP.md src`".
- **The model reads a section by name when it would help.** The read tool takes a Markdown file and a heading and returns that section only. Two hundred words, one round, exactly the part that matters. The search tool still finds any file or line by pattern.
- **The documents stay true without a person.** The harness regenerates the map after every write. At the end of every task it asks the model one question with the tools off: which section of the architecture page did this task change, and what should it say now? The answer goes under that heading. A new project gets `AGENTS.md` from the work order's Rules, Where and test command, and an architecture page built from the task reports.

The rule in one line: the rules ride in full, and the rest is read by name when it would help. The model can reach any of the three at any moment, and none of them can crowd the task out of the window.

## 5. How to write each part well

**A good goal** says what, for whom, and why, in the words you would use to a colleague. "A complete, playable Tetris game for players in a browser, with two hazards and three clear effects, every rule proved by a test." Not "You are an expert game developer."

**A good done line** is a fact the harness or the model can check: "every test passes", "the page shows the board", "a whole game was played to game over and each state photographed". Not "the game is fun" on its own; put what fun means under Details and make the done line about what was seen.

**A good rule** is one line, imperative, in your words, and it holds on every task: "Port 8090 is taken; use 8091." "Never start a browser from a script." Put a rule once. If you find yourself writing the same rule under three features, it belongs here.

**A good stop line** names a state you want to hear about before the work goes on: "Stop if a test cannot be made to pass after three different fixes." "Stop if any file under supabase/ would change."

**A good task** is one sitting of work with one done line, and names its Details: "6. The dragon: probability, warning, lock, forced drop, cleanup. Done when the dragon tests pass. (Details: Dragon, Tests required)". Order tasks by what they depend on. Put the tests-only tasks (a safety suite) before the screen work, and the play test and visual QA last, because they need everything else.

**Good details** are complete. Every feature, every list, every state, every value that has a name. Say what you want and let the model choose how. Group them under headings the tasks can name.

## 6. Mistakes the old Tetris ask made, so you can avoid them

`TETRIS_TEST_PROMPT.md` is a good ask for a person and a hard one for the harness, for five reasons that are easy to fix.

- The goal is on line 8 and the finish is on line 678. The model met the finish only after reading everything, on every task.
- Tests first is said in four places. The rule belongs in one place that rides in front of every task.
- The finish is twenty-nine bullets with no check on any of them, so the harness could run none and the model translated them into its own done list.
- There is no stop list, so nothing could stop the work early.
- Whole sections lecture the process: "write test, run test, implement, run test, fix, refactor, run all tests". The harness holds that process (section 3); the ask does not need to teach it.

`TETRIS_PROMPT_V2.md` says everything the old ask says, under the six headings, with seven done lines and fourteen tasks.

## 7. A checklist before you submit

- [ ] The goal is two to five sentences and carries the whole intent.
- [ ] Where names the folder, and for an existing project the files to start from.
- [ ] The first done line is the test suite, with `[tests pass: ...]`.
- [ ] Every done line is a fact that can be checked, by the harness or by the model against a result.
- [ ] Each rule appears once. Anything dangerous has a Stop line.
- [ ] Each task has one done line and names its Details sections.
- [ ] Details hold every feature, complete, and nothing about process.
- [ ] Nothing in the ask tells the model how to be an agent. The harness does that.

## 8. If you do not use the shape

A plain ask still works. The model writes the record itself: the why, the done list, the stop list, the plan, and for a large ask a job with tasks, in the same six-heading shape, because that is the shape the job tool asks for. What you lose is the checks: the finish is the model's translation, the rules are wherever the model carried them, and every task reads the whole ask. For anything longer than a page, the shape is worth the five minutes.
