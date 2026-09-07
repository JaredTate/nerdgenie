# How to write a prompt for Nerd Genie

This guide is for anyone who writes an ask for Nerd Genie: a person at the keyboard, or a larger model writing work for a smaller one. It explains the shape an ask should have, why that shape makes the model do better work, and how the model gets more context when it needs it without being buried in it. The worked examples are the `EX_PROMPT_*` files in the root, numbered from easiest to hardest, each with its difficulty and expected time on the local model in its first comment:

| Number | Example | Difficulty | Expected time | What makes it hard |
|---|---|---|---|---|
| 1 | Tic Tac Toe | 1 of 10 | fifteen to forty minutes | almost nothing; two players at one keyboard, and the polish is judged by eye |
| 2 | Tic Tac Toe vs Computer | 3 of 10 | forty minutes to two hours | an unbeatable minimax opponent proved by a test that plays every opening |
| 3 | Tetris | 5 of 10 | one and a half to four hours | two hazards and three effects on a state machine |
| 4 | Solar System | 5 of 10 | one and a half to four hours | orbital maths and WebGL glow |
| 5 | Interactive Earth | 6 of 10 | two to five hours | textures, shaders, labels, lighting across a terminator |
| 6 | Tower Defense | 8 of 10 | four to ten hours | pathfinding, an economy, and balance only play can prove |
| 7 | Flight Simulator | 9 of 10 | five to twelve hours | flight physics, a large world, and a performance pass |

The times are wide on purpose: they depend on the GPU and the model. Tetris finished inside two hours on one 7900 XTX with the four-bit Qwen; a three-bit model on a smaller card takes longer, a faster card less. Each example's time is corrected after it has run once.

A new example takes the next number when it is harder than the last, or every number above it moves up by one. `docs/IDEASv2_PLAN.md` is how the harness learns to use the shape.

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

**Rules.** The help you would give a colleague on day one, one line each, in your own words: how to run it, how to test it, what to use, where things go, what good looks like, what to do when something will not pass. A rule makes a choice once so the model never has to make it, and never makes it wrong on task nine. A rule is never a threat and never a reason to stop; the harness's own guards look after a stuck model, and anything that needs your yes lives on the ask-me-first list in the config, where the harness shows you a preview. The first rule is always tests first, and the harness writes it there itself, whether or not you did: write the test, watch it fail, write the code, watch it pass, the whole suite green before a task ends.

**Tasks.** The order of work, one line each, each with one done line, in the order they depend on each other. Name the Details sections each task needs, like this: `(Details: Dragon, Tests required)`. A task is one sitting of work, thirty seconds to an hour. If you leave Tasks out, the model plans the job itself, in this same shape.

**Details.** Everything else, as long as it needs to be, under its own headings. Every feature, every list, every rule of the game. This is where the length goes. Say what you want, not how to build it. A framework is fine; say so in Rules if you have a preference.

## 3. What the harness does with each part, and why that helps the model

| Part | What the harness does | Why the model does better |
|---|---|---|
| Goal | Prints it at the top of every task. | When a plan breaks, the model decides from the intent instead of guessing. |
| Where | Opens the folder and the starting files, finds the three project documents. | The model starts oriented instead of spending rounds listing and reading. |
| Done when | Writes the finish into the record with the checks attached; runs the checks at the end of every task and at the finish. | The finish is yours, not the model's translation of it. The model cannot declare the job done early, and it knows exactly what to prove. |
| Rules | Puts them in front of every task, in your words, never rewritten. | The model never guesses how to run, test or serve, or which tool to use, and never re-decides on task nine what a rule decided on task one. Tests first is always there. |
| Tasks | Makes the job from them with no planning rounds; runs them one at a time; each task sees only its own line. | The model works on one thing with one done line, and reads nothing about the other tasks. |
| Details | Shelves each section by heading; shows a task the sections it names in full, up to three of at most 150 words each, and the others by heading only; a longer named section is read by heading. | The model reads the one page it needs instead of the whole spec, and can ask for any other section by name. |

Then every task runs through the same five moves, and the harness checks each one.

1. **Orient.** The model sees the goal, the rules, its own task line, its Details sections, the folder as it is now, and the last task's report.
2. **Test.** The model writes the failing test. The harness runs it and puts the result on the write's own line. Code written before a failing test exists gets one line saying so.
3. **Code.** The model writes or edits. After every change the harness checks the file parses and runs the tests again, and puts both on the change's own line.
4. **Prove.** The model marks the done line with the green run. The harness refuses a mark that points at a run older than the last edit.
5. **Report.** The task's report goes into the job and to you. The next task starts.

Five moves, the same on every task of every project, each with a check the harness makes. That is what makes the work repeatable: the shape does not depend on the model remembering how to work, because the harness holds the shape.

## 4. On-demand context: the three project documents

A project we keep carries three documents. A new project gets them from the job that builds it. An existing project already has them.

- **`NERDGENIE.md`**: the rules of the project and how to run it and test it. Under sixty lines.
- **`ARCHITECTURE.md`**: how the project is put together, one section per part.
- **`REPO_MAP.md`**: where everything is. Generated, never written by hand.

The model cannot read these whole. A real architecture page is a hundred thousand tokens on a large code base, and a map is thousands of lines. Nor should it: a task on the dragon needs the hazards section, not the rendering section. So the harness handles the three like this.

- **The rules ride in full.** `NERDGENIE.md` is short and it is the rules, so it sits under the job summary in front of every task.
- **The other two ride as one line each.** In the block that already tells the model what is in the folder and which ports are open, two more lines: "ARCHITECTURE.md has 9 sections: engine, hazards, effects, shell, tests, ...; read one with `read ARCHITECTURE.md hazards`" and "REPO_MAP.md has 41 files in 6 folders; read a folder with `read REPO_MAP.md src`".
- **The model reads a section by name when it would help.** The read tool takes a Markdown file and a heading and returns that section only. Two hundred words, one round, exactly the part that matters. The search tool still finds any file or line by pattern.
- **The documents stay true without a person, from the first task on.** At the end of every task of a job the harness leaves all three in the project folder: it asks the model one question with the tools off, which section of the architecture page did this task change and what should it say now, and writes the answer under that heading; it writes `NERDGENIE.md` once from the job's name, why, rules and test command; and it regenerates `REPO_MAP.md`, a legend of the top folders and a tree of the files. The project folder is the one Where names, and a job whose ask named none learns it from the folder its first task's files share.

The rule in one line: the rules ride in full, and the rest is read by name when it would help. The model can reach any of the three at any moment, and none of them can crowd the task out of the window.

**Why this matters, in one example.** Take the dragon task. Without the documents the model finds out how the hazard code works by reading it: it lists `src`, reads `hazards.js` in two halves because the file is over the read cap, reads `config.js`, reads `engine.js` for the piece interface, and only then writes the first test. Five rounds, three whole files in the window, and the first thing the window drops when it fills is one of those files, which the model then reads again. With the documents, the orientation block has already told it that the architecture page has a hazards section. It reads that section in one round: the states, the `raise(kind, piece)` function, the config values, the file and the test file. It reads the eighty lines around `raise`. It writes the test. Three rounds instead of six, two hundred words and one region in the window instead of three files, and the section comes back by its result id after any cut. The model edits with the right names in the right file, and when it changes the part, the review writes the section so the next task reads the truth. That is the whole reason for the three documents: the model fetches exactly the knowledge the next step needs, in one round, and keeps it.

## 5. How to write each part well

**A good goal** says what, for whom, and why, in the words you would use to a colleague. "A complete, playable Tetris game for players in a browser, with two hazards and three clear effects, every rule proved by a test." Not "You are an expert game developer."

**A good done line** is a fact the harness or the model can check: "every test passes", "the page shows the board", "a whole game was played to game over and each state photographed". Not "the game is fun" on its own; put what fun means under Details and make the done line about what was seen.

**A good rule** helps. It is one line, in your words, and it holds on every task: "Serve on port 8091; 8090 is in use." "The Chrome window on the screen is how you see your work." "Keep every hazard value in one config file, so balancing is one edit." "When a test will not pass, write the failure and its cause and take another route." Each of those makes a choice or shows a way through. Put a rule once; if you are writing the same rule under three features, it belongs here.

**A rule that would stop the work is not a rule.** "Stop if a test cannot pass" turns a night's run into a wait for you, and the harness already handles a stuck model with its own guards. If there is something the model must never do without your yes, such as deleting many files, spending money or touching production, put it on the ask-me-first list in the config: the harness stops there, shows you a preview, and waits for your answer, and the ask stays about the work.

**A good task** is one sitting of work with one done line, and names its Details: "6. The dragon: probability, warning, lock, forced drop, cleanup. Done when the dragon tests pass. (Details: Dragon, Tests required)". Order tasks by what they depend on. Put the tests-only tasks (a safety suite) before the screen work, and the play test and visual QA last, because they need everything else.

**Good details** are complete. Every feature, every list, every state, every value that has a name. Say what you want and let the model choose how. Group them under headings the tasks can name.

## 6. Mistakes the old Tetris ask made, so you can avoid them

`TETRIS_TEST_PROMPT.md` is a good ask for a person and a hard one for the harness, for five reasons that are easy to fix.

- The goal is on line 8 and the finish is on line 678. The model met the finish only after reading everything, on every task.
- Tests first is said in four places. The rule belongs in one place that rides in front of every task.
- The finish is twenty-nine bullets with no check on any of them, so the harness could run none and the model translated them into its own done list.
- Its rules are scattered through the features, so the model carried them from task to task by luck.
- Whole sections lecture the process: "write test, run test, implement, run test, fix, refactor, run all tests". The harness holds that process (section 3); the ask does not need to teach it.

`EX_PROMPT_3_TETRIS.md` says everything the old ask says, under the six headings, with seven done lines, fourteen tasks, and eight rules that each make a choice or show a way through.

## 7. A checklist before you submit

- [ ] The goal is two to five sentences and carries the whole intent.
- [ ] Where names the folder, and for an existing project the files to start from.
- [ ] The first done line is the test suite, with `[tests pass: ...]`.
- [ ] Every done line is a fact that can be checked, by the harness or by the model against a result.
- [ ] Each rule appears once and helps: it makes a choice or shows a way through. None of them tells the model to stop.
- [ ] Each task has one done line and names its Details sections.
- [ ] Details hold every feature, complete, and nothing about process.
- [ ] Nothing in the ask tells the model how to be an agent. The harness does that.

## 8. If you do not use the shape

A plain ask still works. The model writes the record itself: the why, the done list, the stop list, the plan, and for a large ask a job with tasks, in the same six-heading shape, because that is the shape the job tool asks for. What you lose is the checks: the finish is the model's translation, the rules are wherever the model carried them, and every task reads the whole ask. For anything longer than a page, the shape is worth the five minutes.
