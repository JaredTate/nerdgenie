# Ideas, third set: one work order for any job, and the scalpel's way of working brought into the harness

Written 7 September 2026. `docs/IDEAS_V2.md` stays as it is; this set builds on it and goes further. The goal is one sentence: a small local model, in a small window, makes accurate changes to a very large code base without breaking anything, runs the tests it should, and proves what it did. The same harness, the same work order and the same documents have to serve three kinds of work: a new standalone build (Tater Tots Tetris, an interactive solar system), a change inside an existing code base of the size of Home Recon (six thousand tracked files, about four hundred thousand lines of TypeScript, SQL and Java, four apps in one monorepo, a live production database), and, later, a Nerd Scalpel ticket, when Nerd Genie becomes the agent behind the scalpel's board and Opus orchestrates Nerd Genie sub-agents. Nothing here is built. The owner approves an idea before it is built, it is built test first, and it lands in `docs/PROGRESS.md` with its measured effect.

## 1. What Nerd Scalpel gets right

Nerd Scalpel puts every screen of the running app on one canvas. A person points at a pixel, says what is wrong, and a ticket goes to a coding agent in a terminal. The agent is Claude Code or Codex, with a large window; the reason a ticket lands well is not the agent, it is what the board gathers before the agent reads a word. Read from `~/Code/nerdscalpel/ARCHITECTURE.md` and the two prompt templates, the ticket carries eight things, every one of them computed and none of them asked of the agent:

1. **The starting point.** The fiber probe walks from the clicked pixel to the React component chain and the source `file:line` that renders it. The prompt says "start HERE, no need to grep".
2. **The evidence.** A recorder in every frame keeps the console errors, uncaught exceptions and failed requests of that screen, and they ride with the ticket.
3. **The blast radius.** A dependency graph of the repo, inverted, answers "what depends on this file" and "which tests cover it", with dist-consumed packages remapped back to their source, so the prompt can say "this is a shared contract: rebuild the package and run its tests".
4. **The data behind the screen.** The screen's own network traffic names the tables it reads; the generated database types give their columns, and the data-model document its sections.
5. **The current styles**, computed, with the CSS variables that produced them.
6. **Pointers to documents, never their contents.** `contextDocs` is a list of path and one-line hook: "ARCHITECTURE.md: system map, surfaces, roles, AI bridge". The agent opens what it needs.
7. **House rules, verbatim**, in the session primer: production is live, never push to main, the dashboard is also the phone app, rebuild the core package after editing it.
8. **A verify gate.** Done is not taken on faith: a configured command must exit zero in the ticket's own git worktree before the done callback is accepted, and at the repository root before a merge lands; a red run pastes its tail back into the terminal and the agent keeps the ticket.

Around those eight, three pieces of protocol: every ticket runs in its own worktree on its own branch and is merged back when green; the queue is an event log replayed on boot, so a restart requeues in-flight work at the front; and done is a callback the agent must make, with a twenty-five minute timeout so the queue never stalls.

That is the harness-keeps-the-books principle, applied to one edit at a time. Nerd Genie already has the other half: the record, the shelf of results read by id, a done check that needs proof, the same-call guard, the stall ladder, the tests rerun after every edit, and a browser on the person's screen. What it does not have is the scalpel's gatherers and the scalpel's gate. This set gives it both, in the harness's own shapes, and puts one work order over all three kinds of work.

## 2. The work order

An operations order has five paragraphs: situation, mission, execution, sustainment, command and signal. The record already has that shape. The work order is the record's shape written by a person before the work starts, with two paragraphs the second set left out: where the work is, and what must be green before anything counts as done. Eight headings, in this order.

| Heading | What goes under it | What it becomes in the record | What the harness does with it |
|---|---|---|---|
| **Outcome** | One paragraph: what exists when this is done, for whom, why | The job's name and why | Printed in the job summary of every task; the why is what the model uses when the plan breaks |
| **Where** | The terrain: the folder or repository, the branch, the app and screen, the starting file and line when known, the persona to sign in as | The situation's first lines, fixed for the job | Opens the worktree, the screen and the starting file before the first model call; builds the edit pack (section 3) from it |
| **Done when** | Numbered lines, each checkable; a check in brackets where the harness can run it; one line marked as the gate | The job's done list | Runs the bracketed lines at the end of every task and at the finish; a task cannot finish while the gate is red |
| **Stop if** | What must reach the person before the work goes on | The stop list | Adds its own two lines and watches the rest |
| **Rules** | The standing constraints, in the person's words | Corrections C1, C2, ... of the job | Ride in the cached front of every task, never rewritten; the first rule, tests first, is the harness's own and is there whether or not the person wrote it |
| **Steps** | The order of work, one line each; each may name the Details sections it needs and the files it touches | The job's task list | Makes the job with no planning rounds; the model may split or add a task, never remove a finished one |
| **Read first** | Pointers: a path and a one-line hook saying when to open it | Shelved books with ids, listed by hook | Never inlined; opened by name when the hook fits the task |
| **Details** | Everything else, under its own headings | Shelved by heading | A task's front carries the sections its step names in full, the rest by heading and first line |

The checks in brackets are the `expect` line's four rules and two more: `[tests pass: <command>]`, `[exit 0: <command>]`, `[shows: "<text>" at <url>]`, `[parses: <file>]`, `[exists: <path>]`, and `[gate: <command>]`. A gate is a check that must be green before any task of the job may finish, which is the scalpel's verify gate in a done line. A job on Home Recon has one gate line, `[gate: corepack pnpm test:node]`, because the repository's own rules say that command must be green before any push and it takes ten seconds.

What is new against the second set is Where, the gate, hooks on every pointer, and files on every step. Those four are what a change inside a large code base needs and a new build does not miss.

**Tests first is in every work order, whether the person writes it or not.** The harness adds two lines of its own to every stop list; it adds one rule of its own to every Rules section, and it is the first: "Tests first: write the test, watch it fail, write the code, watch it pass; the whole suite green at every milestone." A person may add rules under it and may not remove it. Three things hold the rule without a round of the model's time. A step that changes code is two halves, the test and then the code, and the harness writes a step that names files as those two halves. A done line on a code change rests on a test run made after the last edit, which the done check already requires since 7 September. And a write or edit to a file that is not a test, when no test has failed since the last green run, gets one line on its own result: "tests first: no failing test covers this change; write it first". The test reader already knows the state of the suite, so the line costs nothing, and a model that writes code before its test reads why on the same result.

### 2.1 A new build: Tater Tots Tetris

The second set holds the full rewrite. In the work order it changes in two lines: Where reads "a new folder, `<WORK>/Tater Tots Tetrisv1`, empty; the game served on any port but 8090; the Chrome window on the screen", and Done when gains `0. [gate: npm test]`. An interactive solar system has the same shape with different Details: the bodies and their orbits, the controls, what a test can prove without a screen (positions at a time, scale, collisions) and what only the screen can (the look at five sizes).

### 2.2 A change inside Home Recon

This is the case the set exists for. The person wants the Overview page of the inspector dashboard to show a card the inspector has asked for.

```
# Overview: next inspection card

## Outcome
The inspector's Overview page shows one card, "Next inspection", with the date, time,
address and client of the soonest scheduled inspection, or "nothing scheduled" when
there is none. It is for the inspector opening the app in the morning. It must look
right on a phone first, because the dashboard is also the iPhone and Android app.

## Where
- Repository ~/Code/homerecon, branch develop, in a worktree of its own.
- App: dashboard (apps/dashboard, dev server 5273). Screen: overview (/overview).
- Persona: Inspector demo (owner). Data: the local Docker Supabase stack, never cloud.
- Start at apps/dashboard/src/pages/Overview.tsx, the cards grid.
- The inspections query lives in packages/data; the date helpers in packages/core
  (dist-consumed: rebuild after editing).

## Done when
0. [gate: corepack pnpm test:node]
1. A test in apps/dashboard proves the card renders the soonest future inspection and the
   empty state. [tests pass: corepack pnpm --filter @homerecon-ai/dashboard test -- Overview]
2. The card is on the screen at 390 wide with nothing overlapping, and at 1440.
   [shows: "Next inspection" at http://localhost:5273/overview]
3. No console error and no failed request on the Overview screen after the change.
4. Typecheck is green. [exit 0: corepack pnpm typecheck]
5. The change is on a branch named nerdgenie/overview-next-card, merged to develop only
   when lines 0 to 4 are green at the repository root.

## Stop if
- The card needs a new table or a migration.
- Any file under supabase/ or packages/data/src/database.types.ts would change.
- The gate is red before the first edit.

## Rules
- Tests first: write the test, watch it fail, write the code, watch it pass. (the harness's
  own line, present in every work order)
- Cloud Supabase is production; never point a test or a query at it.
- Native-safe: no browser-only APIs, reach outward only through appOrigin(), inputs >= 16px.
- Never push to main. Never db reset anything but the local stack.
- Words banned on any pricing surface: bid, quote, proposal, scope of work.

## Steps
1. Read the Overview page and the inspections query; write the failing test.
   (files: apps/dashboard/src/pages/Overview.tsx, packages/data/src/inspections.ts)
2. The card component and its empty state. (files: apps/dashboard/src/components/NextInspectionCard.tsx)
3. Wire it into the grid; typecheck; the dashboard tests.
4. Look at it in Chrome at 390, then 1440, signed in as the inspector demo; fix what shows.
5. The gate at the root; report.

## Read first
- CLAUDE.md: the house rules, pricing truth, cloud safety, the testing contract.
- ARCHITECTURE.md, section Product Surfaces: how the dashboard is put together.
- DATA_MODEL.md, section inspections: the table and its columns.
- TESTING.md, section Where does my new test go.

## Details
### The card
(what it shows, the order of fields, the empty state, the tap target)
### Style
(match the existing cards on the page; use the design tokens, no new colours)
```

Read that as the harness reads it. Where gives it a worktree to open, a screen to load, a persona to sign in as, a file to open at a line, and two packages to watch. Done when gives it a gate to run before any task ends, two commands it can run itself, one screen check it can make itself, and one line the model must prove with a result. Steps name their files, so the edit pack for each task is built before the model's first call. Read first is four pointers with hooks, so the model opens the one it needs and the 154-kilobyte architecture page never enters the window. The rules are the repository's own hard rules, in the person's words, in front of every task.

### 2.3 What the harness builds for the work order

- **The lift** (from the second set): the eight headings recognised by name, the record written by the harness, the details shelved by heading.
- **Where opens the terrain.** A repository in Where gets a git worktree on a branch named for the job before the first call (section 4.3). A screen in Where is opened in the Chrome window and signed in with the persona before the first call, the way the scalpel's board does with `loginSelectors` and `postLoginGate`, and its outline and its errors are the first result. A starting file is opened at its line as the second.
- **The gate.** A `[gate: ...]` line is run at the end of every task and its tail pasted back on red, the way the scalpel's verify gate does; the task cannot finish until it is green. It is also run once at the job's start, so a job that begins red stops on its own stop line instead of blaming its first edit on itself.
- **Hooks on pointers.** Read first is shelved as books with ids, and the results list shows each as its hook: "b2 DATA_MODEL.md, section inspections: the table and its columns; read b2". A pointer whose hook names a section shelves only that section.
- **Files on steps.** A step's files seed the edit pack of the task it becomes.

- Evidence: the scalpel's tickets land well on a large repository because they carry these things; the Tetris runs finish without them because a new folder has no terrain; run twelve's rules travelled only in the model's own task names.
- Measure: tasks on Home Recon that end with the gate green and the change accepted by the owner; rounds and tokens per task; reads that hit a file the pack already carried.
- Cost: three days beyond the second set's lift.

## 3. The edit pack: working context for one edit in four hundred thousand lines

A person who changes one card on one page of Home Recon does not read Home Recon. They open the page's file, look at the card next to it, glance at the query it calls and the type it returns, check who else calls what they are about to change, find the test file, and keep the house rules in mind. That is a few hundred lines out of four hundred thousand. The scalpel gathers most of that for a ticket from the running app and the dependency graph. The harness can gather it for a task from three things it has or can cheaply keep: the map's index (the second set, 3.3), an import graph, and the page in the Chrome window.

**What the pack holds**, in order, capped at about three thousand tokens:

1. **The target**: the starting file, forty lines either side of the starting line, with line numbers, and the file's own names (functions, components, exports) as one line.
2. **The contracts it uses**: for each import of the target file, the exported names and their signatures only, never the bodies. One regular expression per language finds `export function`, `export const`, `export type`, `export interface`, `func Name(`, `def name(`; the second set's names index already keeps them.
3. **Who depends on it**: the files that import the target, with the line on which they use each name the task is likely to change, and, when the target is in a dist-consumed package, the line "shared package: rebuild it after editing, and its dependents' tests run".
4. **The tests that pin it**: co-located test files (`Overview.test.tsx`, `Overview.*.test.tsx`), and the tests of the dependents, as a list and a command.
5. **The page**: when Where names a screen, the outline of the screen from `browser_read`, the errors the page has raised, and, for the element the step names, the component chain and source `file:line` resolved by the same fiber walk the scalpel's probe does, run through the browser worker.
6. **The part's paragraph**: the section of `ARCHITECTURE.md` for the part the target belongs to, and nothing else of that page.
7. **The rules that name these files**: any Rules line or standing-order line that mentions the package, the folder or the screen. "packages/core is dist-consumed" appears in the pack of a task that touches `packages/core` and in no other.

**Where the pack comes from.** The map's index gives 1, 2 and 7. An import graph gives 3 and 4: one regular expression per language for import lines, inverted and cached, rebuilt for the files a write or edit touched, and, where the repository already carries a graph tool as Home Recon does, that tool's output read instead. The browser worker gives 5. The architecture page's headings give 6.

**Where the pack rides.** It is a result with an id, `p1`, shelved like any other, and pinned for the task, so it never leaves the window while the task runs. When the task edits the target, the pack is rebuilt for the new file and the new pins replace the old, which is a change at the end of the window and cheap. When the model reads beyond it, it reads by name: a file the pack lists, a section the hook names, a test the list names.

**A new build has a pack too.** On task one of Tetris the pack is the empty folder and the Details section the step names. By task four it is `engine.js` with its exports, `hazards.js` with the names that call them, the two test files, and the hazards section of the architecture page the review wrote at the end of task three.

Why this is the heart of the set: a small model's window is the whole reason it fails on a large code base. Given a starting point, the contracts around it, the callers and the tests, in three thousand tokens, the edit is the same size as an edit in a new folder. The scalpel proved the shape on a large model; the pack is the same shape for a small one.

- Evidence: one task read the same file forty-eight times, and 2.6 reads by id follow every restart; the scalpel's prompts say "start HERE, no need to grep" for a reason.
- Measure: reads per task of files the pack already carried; searches per task; edits that landed on the wrong file; tasks on Home Recon finished with the gate green.
- Cost: five days, on top of the map's index.

## 4. Not breaking anything

Accuracy is the pack; not breaking things is what runs after every edit. Six pieces, four of them extensions of what the harness does now.

### 4.1 The ripple list

When an edit changes an exported name's signature, the harness lists every caller from the graph as a checklist on the edit's own result: "callers of `formatInspectionDate`: 3, in `apps/dashboard/src/pages/Overview.tsx:212`, `apps/report-viewer/src/lib/dates.ts:40`, `packages/core/src/index.ts:9`". Each caller is cleared by the typecheck or by an edit. The list is the harness's, so the model cannot forget a caller, and the typecheck is the proof that it did not.

### 4.2 Three checks after every edit, in order, on the edit's own result, after the tests-first line

The harness already runs the language's own checker after every write and edit and, once the model has run the tests, runs them again. On a large repository the second of those is too big to run every time, so the order becomes: parse the file; typecheck the package the file belongs to (`tsc -p apps/dashboard`, `go build ./internal/loop`, ten to thirty seconds); run the co-located tests from the pack; and, when the file is in a shared package, rebuild it and run the dependents' tests. Each answer lands on the edit's result, as the expect line's answers do now, so the model never spends a round finding out. The gate runs at the task's end, not after every edit.

### 4.3 A worktree per job, and the gate at the root before the merge

A job whose Where names a repository runs in `git worktree add ../nerdgenie-<job> -b nerdgenie/<name>` with the main checkout's `node_modules` linked in, exactly as the scalpel gives every ticket its own worktree and branch. The main checkout stays clean, which matters on Home Recon, where a dirty tree blocks the auto-deploy and where `develop` deploys on push. The job's last task merges the branch only when the gate and the bracketed done lines are green at the repository root, which is the scalpel's Gate B. A merge is never pushed by the harness; pushing is on the ask-me-first list unless the person takes it off. This is also what lets Opus run several Nerd Genie sub-agents at once later: each in its own worktree, none touching another's files, merged one at a time.

### 4.4 The gate is the person's test contract, not the model's choice

Home Recon's rules say which command must be green before a push and why, and they pin their own documents with contract tests. The gate line in the work order is that command, written once by the person. The model does not choose which tests to run; the pack names the tests near its change, and the gate names the tests that guard the repository. Both run without a round.

### 4.5 The visual check is on the screen, in the order the project says

Home Recon's rule is a visible Chrome the owner can watch, mobile 390 first, then desktop 1440, with the console and the network watched. The work order's `[shows: ...]` line and the Rules line carry that; the harness's browser is already the window on the screen, `browser_resize` already exists, and the page's errors already come back on every read. What is added is that the check runs as a done line the harness marks itself, on both widths, with the errors list empty as part of the proof.

### 4.6 The stop lines are the fence

The scalpel's house rules say what never to touch. The work order says it in Stop if, which the harness watches through the `task` tool, and the write and edit tools can carry the same fence as a plain rule: a Stop if line that names a path ("any file under supabase/") becomes a refusal on write and edit to that path, with the stop line quoted, before the model has done it. That is the cheapest safety in the set and it is one comparison in the tool.

- Evidence: the scalpel's verify gate exists because agents marked broken code done; Home Recon's rules list two traps `test:node` catches that keep biting; three of eight nightly runs closed done on a job that did not work.
- Measure: edits reverted by the owner; gate failures per task and at which check they were caught; callers missed.
- Cost: four days.

## 5. Screens as terrain

For work on an app, the scalpel's `screens.json` is a map of the user interface: 159 screens on Home Recon, each with an id, a path, a title, a group and the screens it leads to. The harness should read the same file when a repository has one. Where names a screen by id; the harness opens it, signed in as the persona Where names, at the width the project's rule says first. The map slice for a task on a screen is the screen's file, its components, and the screens it leads to. A `[shows: ...]` line can name a screen id instead of a URL. And the source locator, the fiber walk that turns a clicked element into a component chain and a `file:line`, runs through the browser worker on the element the step names, so a task that says "the cards grid on overview" starts at a line, not at a grep.

This is what makes a UI change in a large app the same work as a UI change in a new game: open the screen, find the element, open its source, change it, look. The harness does the first three before the model's first call.

- Cost: two days, with the browser worker carrying the probe.

## 6. The three documents at Home Recon's scale

The second set says how `AGENTS.md`, `REPO_MAP.md` and `ARCHITECTURE.md` become working context: one line in the prompt, a slice per task, the rest by name. At Home Recon's scale three things change.

- **The standing order is already too big to paste.** Home Recon's `CLAUDE.md` is a hundred and fifty lines of hard rules, and the scalpel does not paste it; it points at it with a hook. The harness does the same: the standing order rides as pointers with hooks, and only the Rules lines the work order quotes ride in full. The rules that can be tests already are tests there (`tests/docs/*_contract.test.mjs`), and those run in the gate, which is the right place for a rule with teeth.
- **The map is an index or it is nothing.** The generated map is 6,192 lines. Nobody reads it, and the model must not. The index of the second set, with names and purposes and last-touched, is what the pack and the search tool read; the file stays for people and for git, and the harness regenerates it the way the repository's own script does.
- **The architecture page is read by section and written by the review.** 2,263 lines, twenty-six sections. The pack carries one section. The review at each task's end writes the paragraph for the section the task changed, so the page stops drifting between waves, which its own last section admits it does.

## 7. When Nerd Genie is the scalpel's agent, and Opus orchestrates

Not yet, but the shape should be settled now so nothing built this month has to be undone. The scalpel speaks to its agent through a prompt with placeholders and a done callback. Every placeholder has a heading in the work order:

| Scalpel | Work order |
|---|---|
| `{message}`, `{intent}`, `{severity}` | Outcome |
| `{route}`, `{selector}`, `{component}`, `{source}`, the persona | Where |
| `{evidence}`, `{styles}`, `{data}`, `{impact}` | The edit pack, items 5, 3 and 4 |
| `{contextDocs}` | Read first, with the same hooks |
| `{houseRules}` | Rules |
| `verify.command` | The gate line |
| `{doneInstruction}` | A done line the harness itself calls when every other line is proved |
| the ticket's worktree and branch | Where's worktree, 4.3 |

So `agent: { cli: "nerdgenie" }` is an adapter that renders a ticket as a work order and reads the job's report as the done callback. A ticket with one done line is a task; a ticket with more is a job, split by the harness, and the board sees one ticket. A typed ticket's output validation is an `[exists: ...]` and a `[parses: ...]` line.

Opus as orchestrator writes work orders and reads records, never transcripts. That is the split Cognition warns about, and the record is why it is safe here: a sub-agent's record holds the ask word for word, its decisions with reasons, its failures with causes and its results by id, so the orchestrator reads a page and can fetch any result by name. Each sub-agent runs a job in its own worktree, which is a scalpel slot with the harness inside it. The orchestrator's own work order names the sub-jobs as Steps.

## 8. What to measure first

A bench of ten small, real changes on Home Recon, each written as a work order, each with `[gate: corepack pnpm test:node]`, run on the local model on the local Docker stack in a worktree, with the owner judging the diff. The numbers: tasks finished with the gate green; diffs accepted; rounds and tokens per task; reads of files the pack carried; callers missed; edits on the wrong file. The same ten run through Claude Code from the scalpel's board give the comparison. One of the ten joins the nightly set beside the Tetris job, so the large-code-base case is measured every night, not once.

## 9. Order and cost

1. The work order's four additions, Where, the gate, hooks, files on steps (section 2): three days, after the second set's lift.
2. The three checks after every edit and the ripple list (4.1, 4.2): three days.
3. The worktree per job and the merge gate (4.3): one day.
4. The stop line as a fence on write and edit (4.6): half a day.
5. The edit pack (section 3), on the map's index: five days.
6. Screens as terrain and the source locator through the browser worker (section 5): two days.
7. The Home Recon bench (section 8): one day to write, then nightly.

About sixteen working days, and the bench runs from step two onward, so every step after it is measured on the case that matters.
