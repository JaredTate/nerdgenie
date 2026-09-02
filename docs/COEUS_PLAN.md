# COEUS

**Coeus is an open-source agent that runs on any Linux machine. You talk to it in a terminal or on Signal. It works with any model, large or small, local or cloud. And it does not forget what it is doing.**

This is the design document for the first version. Nothing is built yet. The comparison of other agents that this design grew out of is in `HARNESS_V2.md`, and the research behind that comparison is in `docs/research/`.

Coeus is named for the Titan of intellect, the axis the heavens turn on.

---

## 1. The idea in one page

Every AI agent today is built the same way. A language model is a function: text goes in and text comes out. The model has no memory and no hands. The **harness** is the program wrapped around the model that gives it both. The same models are available to everyone, so the harness is where the difference lies.

We studied the harnesses people run today: OpenClaw, Hermes, Prime, OpenCode, Atomic, ZeroClaw, and the two from the model labs, Codex and Claude Code. All of them keep long-term memory outside the conversation, in files or a database. But every one of them uses the conversation transcript as the record of the task itself. On each turn, the model re-reads what happened in order to work out where it is. When the transcript gets too long, most of them summarize it and hope that nothing important was lost. That is like a video game loading your save by replaying every button you ever pressed.

Coeus is built on a different idea, and it is an old one.

**State.** In computer science, state is the small amount of information about the past that you need in order to act correctly next. A counter does not remember every increment. It remembers the number seven. A database keeps a log of everything that happened, and it also keeps a snapshot of what is true right now, and those two things are not the same. An operating system can pause a program and resume it days later from one small record. Coeus keeps three things separate: what happened, which we call history; what is true now, which we call state; and what the model is looking at during this turn, which we call the working context. Think of a library, a desk, and the page in front of you. Nobody reads the whole library to write the next sentence.

**The operations order.** The United States Army faces a larger version of the same problem. A headquarters cannot see every unit, radios fail, and people rotate out in the middle of a mission. The Army's answer is a fixed document format that anyone can write and anyone can check, called the five-paragraph operations order, together with a rule for what to do when the plan breaks. We borrow its parts directly. The situation. The mission, including the user's intent and a description of what "done" looks like. The plan. A list of things that should make the agent stop and report. Small changes delivered as fragmentary orders. And an after-action review when the work is over.

**The result** is an agent that always knows what it is working on, keeps working until the job is done, tells you when it should, and spends its tokens on the task instead of on re-reading its own past. It works the same way on a small local model and on a million-token frontier model. Only the size of the working context changes, and a big model is never shrunk to fit a small one.

```mermaid
flowchart LR
  H["History<br/>what happened"] -->|"folded into"| S["State<br/>what is true now"]
  S -->|"always included"| W["Working context<br/>what the model sees"]
  H -->|"fetched by id"| W
  W -->|"produces"| H
```

History is the append-only log of every message, tool call, and result. It is never in the context by default. State is the task record, one to three thousand tokens, shaped like an operations order. The working context is what the model actually sees on a turn: the harness rules, the persona, the task record, any pinned evidence, and the recent messages, sized to the model.

---

## 2. What we learned from the others

We read the source code, not the marketing. The full comparison is in `HARNESS_V2.md`. This is the short version.

**What went wrong.** The two most popular harnesses are 2.9 million and 1.5 million lines of hand-written code. One of them merged 10,574 commits in a single month, and 63 percent of those came from one person at a rate of 223 commits a day. Nobody reviews code at that pace, and the result is software that breaks on every update. Neither of them puts a hard limit on how many times the model can call tools in one turn, so "it got stuck" is a built-in outcome rather than a bug. One of them keeps its screen and its brain in two different processes joined by a pipe, and the pipe breaks. The agent loops themselves are small, between one and seven thousand lines. The code wrapped around them is fifty to seventy-five times bigger, and that wrapper is where everything fails.

**What worked, and what we keep from each:**

| From | What we keep |
|---|---|
| **OpenClaw** | A pure loop with hooks around it. A message that arrives in the middle of a turn steers the next step instead of interrupting the current one. A repair layer for models that write their tool calls as plain text. Scheduled jobs with backoff. Signal through the signal-cli daemon, with pairing required for unknown senders. A cache boundary in the prompt. A delivery queue on disk. Launching a real Chrome with its own profile and reading the page as a tree with short reference tags |
| **Hermes** | Your own words are never summarized. Memory files with hard size limits. A breaker that stops crash loops. One lease per session. A delivery ledger that admits when a message may be a duplicate. A masked prompt for entering passwords. One redaction function applied to everything that leaves the program |
| **Prime** | Notes with a history and a rollback that the next prompt actually reads. Scheduled jobs used as a heartbeat. Budgets with a checker at the end |
| **OpenCode** | One core with an API inside it, so that every screen is a thin client. Streaming as small deltas. One command table shared by every surface. Approvals shown inline, with the choices once, always, and reject. A permission engine built from rules, where the last matching rule wins and the default is to ask. Tool descriptions written as plain text. A bad tool call becomes an error the model can fix, never a crash |
| **ZeroClaw** | A cap of ten tool rounds followed by a forced final answer. A parser for messy tool calls. A job store where two processes cannot run the same job. An updater that keeps the old binary and rolls back |
| **browser-use, Stagehand, agent-browser** | Marks on the page elements that just appeared. Hints about how much page is below the fold. Aborting a batch of actions when the page changes underneath it. A finish step that must state whether it succeeded |
| **Codex and Claude Code** | The kernel sandbox is the real security boundary. Escalation is a field on the tool call with a written reason. The model decides what to do, the harness decides what is allowed, and the two never share a layer |
| **Systems design** | Event sourcing, where the log is the truth and the snapshot is the compact image of it. Process control blocks, which let a program pause and resume from a small record. Paging, which keeps what was touched recently and fetches the rest on demand. Save files, which are checkpoints you can reload |
| **The U.S. Army** | The five-paragraph order, the commander's intent, fragmentary orders, critical information requirements, and the after-action review |

---

## 3. How the agent works

```mermaid
flowchart TD
  U["User"] --> Q["Queue"]
  Q --> R{"Route"}
  R -->|"command or skill"| X["Run tools"]
  R -->|"task"| O["Orient"]
  O --> M["Model call"]
  M --> K{"Tool calls?"}
  K -->|"yes"| G["Guard"]
  G --> P["Permission"]
  P --> X
  X --> S["Update record"]
  S --> O
  K -->|"no"| F["Reply"]
  F --> A["Done-check and review"]
  A --> ME["Memory and skills"]
```

A message from the user goes into a queue on disk. The router decides whether it is a command, a trigger for a saved skill, or a real task. A command or a skill runs its tools directly. A task goes through the loop. First the agent orients, which means it reads the record and states where the work stands and what comes next. Then it calls the model with the harness rules, the persona, the record, any pinned evidence, and the recent messages. If the model asks for tools, the guard checks the request for repeats, malformed calls, the round cap, and the stop conditions. The permission function then decides to allow, ask, or deny, and anything irreversible is shown to the user as a preview first. The tools run inside the sandbox, with timeouts and output caps, and they return only what changed. The harness updates the record with the results, and the loop goes back to orient. When the model replies without tool calls, the turn ends. When the task ends, the done-check runs, then the after-action review, and what was learned goes into memory and skills.

One process owns everything: the queue, the loop, the permissions, the record, the memory, the scheduled jobs, and one SQLite file. The process starts two helpers when it needs them. One is a browser worker that runs a real Chrome with its own profile. The other is a desktop worker. They are separate processes so that a hung browser can never take the agent down. There are two thin screens, the terminal and Signal. Neither one owns any state, and both use the same command table.

### The turn, in ten rules

1. A message that arrives in the middle of a turn is treated as a fragmentary order. It changes only what it changes, the way you might text a driver "take the next exit" without re-sending the whole route. The harness writes it into the record as a correction, and the model re-plans from there. It never interrupts a tool that is already running.
2. The model orients before it acts. Before any tool call, it writes one line stating where the work stands and what comes next. If the situation does not match the plan, the plan is fixed first.
3. There is a cap of twenty tool rounds per turn on every model. When the cap is reached, the model gets one last call with tools turned off and is asked to say what it did and what is left. A long task also gets a budget of rounds, tokens, and minutes.
4. If the model makes the same call twice with the same arguments, the harness does not run it a second time. It tells the model to do something different or to answer. A third identical call ends the turn.
5. A malformed tool call is fixed if the name is close to a real one. Otherwise the model gets back the list of real tools. Text that looks like a tool call is parsed as one. The loop never crashes on the model's output.
6. Every error message ends the same way, with three options: answer the user, ask one question, or try different arguments.
7. Every tool has a timeout, its own process group, and a cap on its output. Anything over the cap goes to a file, and the result tells the model where the file is.
8. A turn that has read a web page or a feed is marked as tainted. Nothing irreversible can happen in a tainted turn until the user speaks again. An unattended run that hits a question stops and reports.
9. Retries live outside the loop. The harness tries three times with backoff, then moves to the next model in the chain.
10. A question ends the turn. The model asks in plain text, the task is marked as waiting, and the user's next message resumes it, even if that message comes days later.

---

## 4. State: three kinds, one format

### Three kinds of state

| Kind | What it holds | How often it changes | Where it sits |
|---|---|---|---|
| **Persona** | Who the agent is, who the user is, the standing rules, and durable memory | Rarely | In the cached prefix of every prompt |
| **Skill** | How to do one kind of thing: the steps, what to expect at each one, the permissions, and the known failures | When a website or a tool changes | Loaded only when the skill is used |
| **Task** | What is true right now about the thing being worked on | Every turn | In the task record, which is always in context |

Think of a carpenter, the way that carpenter cuts a joint, and the cabinet on the bench today. Only the task changes from turn to turn. Keeping the three separate keeps the prompt cache warm and keeps the record small.

### The task record, shaped like an operations order

An Army order has five paragraphs: situation, mission, execution, sustainment, and command and signal. The mission paragraph answers who, what, when, where, and why. It also carries the commander's intent and the desired end state, so that when the plan breaks, the unit can still act correctly. Before the operation, the commander also lists the facts that would change a decision, called the critical information requirements. Our record uses the same shape, with the user in the commander's place. It is the pilot's kneeboard: one card that holds the mission, the current leg, and the abort rules, while the full flight log stays on the ground.

| Section of the record | Paragraph of the order | Who writes it |
|---|---|---|
| Ask | Mission: who and what | The user, word for word, and it is never edited |
| Intent and Done | Mission: why, and the end state | The model drafts it, and the user can correct it |
| Corrections | Fragmentary orders | The user, word for word, and they are only ever added to |
| Stop and tell the user if | Critical information requirements | The model drafts the list, and the harness adds the budget and login walls |
| Situation | Situation | The harness, from the world as it is right now |
| Plan, Decisions, Failures | Execution | The model, through the `task` tool |
| Results, budget, and source | Sustainment, and command and signal | The harness |

Here is a complete example:

```
# t17  status: executing  from: signal  budget: 6 of 20 rounds, 41k tokens, 9 min
## Ask (word for word, never edited)
"Write a tweet for our product anniversary and post it from the company account."
## Intent (why, and what done looks like)
Why: a timely, accurate post in the company's voice.
Done: a tweet under 280 characters, previewed and approved, live on the site.
## Corrections (word for word, only added to)
- 17:05 "don't mention pricing"
## Stop and tell the user if
- the post would mention pricing, a person, or a date I cannot source
- the site shows a login wall, a captcha, or a bot check
- the task passes 15 rounds or 30 minutes
## Situation (what the world says now; checked again each time, never assumed)
- tab t1: the compose page, logged in as the company, box empty
- facts on hand: r3 (memory/product.md), r4 (the about page)
## Plan
1. [done] gather facts -> r3, r4
2. [doing] draft under 280 characters, no pricing
3. [ ] preview to the user and wait
4. [ ] post with the skill post-update, then verify it is live
## Decisions (with the reason, so they are not argued again)
- D1 Lead with the date, not the features. Reason: the ask says anniversary.
## Failures (so they are not repeated)
- F1 Draft 1 was 312 characters. Cause: three facts. Do not put three facts in one post.
## Results (one line each; read any of them with `read r7`)
- r3 read memory/product.md: 2,100 characters
- r4 web fetch of the about page: 1,800 characters
- r5 browser_open of the compose page: ok, tab t1
```

### The rules for the record

**What goes in.** The record holds only what the world cannot answer on its own. Which files changed is answered by `git diff`. What is on the page is answered by the snapshot. Whether a job ran is answered by the jobs table. Those things are looked up, never remembered. The record holds what only the conversation knows: what was asked, why it was asked, what was corrected, what was decided and why, and what failed and why.

**Who writes what.** The harness writes everything it can verify, and it does so with zero model tokens: the header, the corrections, the situation, the results, the status of each step based on tool outcomes, and a failure line whenever a tool errors or an expectation is not met. The model writes the parts that require judgment: the intent, the stop conditions, the plan, the decisions, and the failures it understands. The stop conditions work like a smoke detector. You decide what counts as an alarm before the kitchen is on fire. The ask, the intent, and the corrections are never edited by anyone.

**Size.** The record stays between one and three thousand tokens. When it grows past that, the harness folds finished steps and old result lines into single lines. Nothing is deleted, and every result stays readable by its id.

**Cache.** The parts of the record that rarely change sit above the cache line: the ask, the intent, the corrections, the stop conditions, and the decisions. The parts that change every turn sit below it: the situation, the plan status, and the results.

**Checkpoints.** Every change to the record saves a numbered checkpoint tied to the log. A task that is waiting on the user resumes from its last checkpoint, with nothing held in any context window in the meantime. The command `/tasks 17 back 3` reloads an earlier checkpoint and lets the model try a different path, the way you would reload a saved game. Any failed task can be replayed from a checkpoint as a test after a fix.

### The done-check and the after-action review

A task cannot close until every line under "Done" has been answered true, with evidence. A plane does not land because the pilot feels done. The gear, the flaps, and the clearance are each checked in turn. The finish was defined before the work started, and the harness will not let the model declare victory otherwise.

Then come the four questions of the Army's after-action review, answered in one line each, the way a coach runs the film after a game. What was supposed to happen? What actually happened? Why was there a difference? What do we keep, and what do we change? The answer to the last question is what goes into memory or into a skill. This replaces the vague "save what you learned" step that every other harness uses.

### Working context: one rule, sized to the model

Context is the model's working memory for a single call. Nothing carries over from one call to the next except what the harness puts back. Coeus has one rule for it. **The working context is never smaller than the task needs and never bigger than the model can hold.** The record is the same size on every model. The window around it is the only thing that changes, from a few thousand tokens on a small local model to most of a million on a frontier model. A big model is never held back to suit a small one.

Two facts about tokens decide the layout. First, a cached prefix costs about a tenth of a fresh one, so the order of the prompt matters more than its size, and stable things must come first. Second, appending is cheap while rewriting is expensive, because a rewrite breaks the cache. Agents that summarize their transcript rewrite their entire history every time they compact it. Coeus only appends to history, and the only thing it rewrites is a record of one to three thousand tokens.

The prompt is built in layers, ordered from the part that changes least to the part that changes most:

| Layer | How often it changes | Cache point |
|---|---|---|
| Harness rules and persona | Weekly | A |
| Tools | When the software is updated | B |
| Record, stable part: ask, intent, corrections, stop conditions, decisions | Rarely during a task | C |
| Record, live part: situation, plan status, results | Every turn | |
| Pinned evidence, kept word for word | When something is pinned | |
| Recent messages, appended and never rewritten | Every turn | |
| Memory hint, three lines from search | Every turn | |

**Tiered folding.** When a message or a result leaves the recent window, it does not vanish and it is not summarized. It drops one tier. It goes from being in the window word for word, to a one-line entry in the record, to the log, where `read r7` brings it back in full. Today's clothes are on the chair, this week's are in the closet, the rest are in the suitcase, and nothing was thrown away. A large model rarely folds anything. A small model folds constantly and loses nothing.

**Cost on every turn.** The harness knows the token count of each layer and the cache hit rate. It writes one line into the record header, such as "this turn: 6.1k in, 5.2k of it cached, 0.4k out." The `/status` command totals the cost per task. The user can always see what a task cost and where the tokens went.

**The proof.** The forty-step test task runs on every supported model size when the software is built, and it checks the same three things every time: the ask and the corrections are byte-for-byte identical at the end, the done-check passes, and no result has become unreadable. "Works on any model" is a test, not a claim.

**Why not just use the million tokens.** Attention dilutes. A model reasons worse over a million tokens of noise than over thirty thousand tokens of signal, and the leaders on long-task benchmarks all use explicit plans and memory for exactly that reason. The record is also what lets a task be paused for three days and resumed, possibly on a different model, with nothing held in any window. A million-token context is a bigger window. It is not a save file.

---

## 5. What the model is told

The model has to understand the harness it is working inside. The following text goes in the cached prefix of every prompt. It is under two hundred words, and it is the same on every model.

> **How this works.** You are Coeus, an assistant. You work inside a harness that keeps a record of the task. The record is the truth about the task. The conversation is not. Before you act, read the record and write one line that says where we are and what comes next. If the situation does not match the plan, fix the plan first.
>
> **The record.** Never edit the ask, the intent, or a correction. Change only what has changed. When you write a decision, include the reason. When you write a failure, include the cause and what not to do again. Add a fact only when you can name its source. Keep every line short.
>
> **Stopping.** If any "stop and tell the user" condition is true, stop and say which one. Otherwise, do not stop until every line of "done" is true or the budget is spent. A question ends your turn, and you will be resumed with the answer.
>
> **Tools.** Use a tool only when you need it. Never repeat a call with the same arguments. If a result is cut short, read the file it names. Never type a password. Use the login tool instead. Nothing irreversible happens without a preview.
>
> **Writing.** Write plain, short English that a high-school student could follow. Avoid jargon. When a technical term is needed, explain it simply. Match the length of your answer to the question. State facts, and say "not sure" when you are not sure. When work is done, report three things: what changed, what you checked, and what is left.

The harness checks what it can. The ask and the intent must be unchanged byte for byte. Every fact line must have a source. Every failure line must have a cause. Every step marked done must have a result behind it. The done-check must have been written before the task closed. If any check fails, the turn does not close, and the model gets one line back that names the rule it broke.

---

## 6. Tools

There are eighteen tools, and all of them are shown to every model. In the class column, R means read, W means write, X means execute, N means network, and I means irreversible. Every description is plain text under forty words, and it says when not to use the tool.

| Tool | What it does | Class |
|---|---|---|
| `read` | Reads a file, a directory, or a past result by its id | R |
| `write` | Creates or overwrites a file | W |
| `edit` | Replaces one exact span of text in a file | W |
| `search` | Finds files by pattern or lines by regular expression | R |
| `shell` | Runs a command. After ten seconds it returns a job id that can be polled, tailed, or killed. An `escalate` field with a reason requests sudo | X |
| `web` | Searches the web, or fetches a public page as text. Anything interactive belongs to the browser | N |
| `memory` | Searches, gets, or saves a memory | R and W |
| `task` | Updates the plan, adds a fact with its source, records a decision or a failure with its reason, or pins a result | W |
| `skill` | Views, runs, or saves a skill | R, X, and W |
| `schedule` | Creates one job that runs at a time, on an interval, or on a cron expression | I |
| `browser_open`, `browser_read`, `browser_click`, `browser_type`, `browser_act` | The browser tools, described in section 7 | N and X |
| `browser_login`, `browser_handoff` | The browser tools that involve credentials or the user, described in section 7 | N and I |
| `computer` | The desktop tool, described in section 8 | X and I |

Tool results are built by the harness. They are never parsed out of the model's own text. Asking the user a question is not a tool. The model simply asks in plain text, and the turn ends in a waiting state.

---

## 7. The browser, like a human

The browser is the centerpiece. The agent gets a real Chrome, launched with its own profile folder, and never the user's daily profile. The user logs in once by hand, or the agent fills in a vault entry. After that, the cookies are the login.

```mermaid
flowchart TD
  A["Click with an expectation"] --> R["Find the element"]
  R --> P["Wait until it is ready"]
  P --> W["Act with human pacing"]
  W --> S["Wait for the page to settle"]
  S --> D["Snapshot and diff"]
  D --> C{"Expectation met?"}
  C -->|"yes"| O["Return the diff"]
  C -->|"no"| E["Report what was seen"]
  S --> H{"Login wall or captcha?"}
  H -->|"yes"| F["Hand off to the user"]
```

Every browser action works the same way. The model asks for an action and states what it expects to happen, for example a click on the compose button with the expectation that a text box appears. The worker finds the element by its reference tag, and if the tag has gone stale because the page changed, it finds the element again by its role and name, and then by its text. It scrolls the element into view and waits until it is visible, enabled, and stable. It performs the action with human pacing. It waits for the page to settle, which means either that navigation has finished or that nothing has changed for three hundred milliseconds, with a cap of three seconds. It takes a new snapshot and compares it with the old one, noting the new address, any new elements, any dialog, any new tab, and any download. If the expectation was met, it returns the difference and a fresh snapshot. If a click produced no visible change, it retries once by coordinates and then reports. If the expectation was not met, it reports what it expected and what it saw. If at any point it hits a login wall, a two-factor prompt, or a captcha, it hands off to the user by bringing the window to the front and sending a message, and it waits for the user to say "done."

**What the model sees.** The model sees a compact tree of the page with short reference tags, a few hundred tokens in all. Every link, button, and field appears with its name and its tag. Elements that just appeared are marked. A count shows how much of the page is below the fold. A screenshot with numbered marks is available with one call.

**Act and assert.** Every action carries an expected result, and the worker checks that result before the next step. A mechanic tightens the bolt and then tries to turn it. This is how test frameworks get reliable, and it is why the agent will not click the wrong thing three times in a row.

**Like a human.** The agent uses a real Chrome so its fingerprint matches a real browser. It runs headed on the machine's own display, on the user's own network. It moves the mouse along a curve, presses for a human length of time, types one key at a time with small variations, scrolls in steps, and pauses between actions. Each site gets a daily budget of actions. There are no proxies, no headless mode for logged-in accounts, no copying of cookies, and no captcha solving. All of this is about keeping the user's accounts safe.

**Everything a human can do.** Tabs and popups, frames, uploads and downloads, dialogs, PDFs read as text, keyboard shortcuts, drag and drop, hover, and back and forward.

**Skills in the browser.** The user records a procedure once. The harness logs each step's intent, the element it used, and what was expected. Replaying the skill needs no model at all. When a site changes and a step fails, the model finds the element that matches the recorded intent and proposes a one-line patch, which the user approves.

**Engine.** One long-lived Playwright worker per profile, attached to the Chrome that the agent launched. The runner-up is Vercel's agent-browser, which is a single Rust binary.

---

## 8. Desktop, command-line tools, skills, and memory

**Desktop.** There is one `computer` tool, and it runs through a worker. It can launch or focus an app, take a screenshot with numbered marks, click, type, press keys, drag, and use the clipboard. It uses the same pacing and the same act-and-assert loop as the browser. The user grants an app for the session once, and irreversible actions inside that app still get a preview. The desktop is the last resort. If the browser can do the job, the browser does it.

**Any command-line tool.** The `shell` tool plus a skill covers this. When the user hands the agent a new tool, the agent reads the help text and the documentation, writes a skill containing the commands it will actually use with one example each, runs a smoke test, and saves the skill.

**Skills.** A skill is a recipe card. The first time you cook the dish, you think hard. After that, you follow the card and only think when something looks wrong. On disk, a skill is a folder. It contains a file named `SKILL.md` with the name, a one-line description, the triggers, and the permissions, which cover the profile, the allowed domains, the daily limit, and which steps are irreversible. It contains the recorded steps or the script. It contains a dry-run test that stops at the irreversible step. And it contains a changelog with rollback. A skill is born in one of three ways: the user demonstrates the task once, the user points the agent at documentation, or the agent finishes a task and offers to save it. Only the names and one-line descriptions sit in the prompt. The body loads when the skill is used.

**Memory.** The persona holds two files with hard size limits, `MEMORY.md` and `USER.md`, plus a folder of markdown files. All of it is indexed by full-text search, along with every past message. The harness records what it can verify with zero model tokens: files changed, commands run, sites visited, and any user message that starts with "no," "actually," "always," "never," or "don't," which is kept word for word as a correction. The after-action review writes the rest. Every fact has a source and a date. Nothing is deleted. A new fact supersedes the old one, and the old one stays searchable. A three-line memory hint rides below the cache line, and the model can search for more.

---

## 9. Safety, the vault, and reliability

**Five safety rules.** First, one permission function checks every tool call. It is built from rules that name a tool, a pattern, and an action, the last matching rule wins, and the default is to ask. Second, every shell command and every file-writing tool runs inside a sandbox built on bwrap and Landlock. The vault, the browser profile, and the user's SSH keys are always outside the sandbox. If the sandbox is missing, the shell tool is turned off. Third, a tainted turn cannot do anything irreversible. Fourth, nothing irreversible runs without a preview, the way you read a text message back before you hit send. The sandbox itself is a workbench with a lip, so that whatever rolls off stays on the bench. Fifth, secrets are references and never values, one redaction pass runs on everything that leaves the program, and everything is logged.

**The vault.** The vault is an encrypted file with a key that only the agent's user can read. Secrets are entered only in the terminal, through a masked prompt, and never over Signal. The model never sees a secret. It points at the login fields, and the `browser_login` tool types the username, the password, and the two-factor code. Sudo has its own path. A shell call with the `escalate` field and a reason produces a preview. When the user approves it, the harness runs the command outside the sandbox with the sudo password from the vault. The database, the vault, and the browser profile are backed up every night in encrypted form.

**Reliability.** The agent runs under systemd with a watchdog line, and it uses exit codes that mean "restart me" or "bad configuration, stop." There are caps everywhere: twenty tool rounds, fifteen minutes per turn, seven minutes per tool, and one hundred queued messages. The event log is the ledger. A reply is logged before it is sent, a crash replays the log, a resent message says that it may be a duplicate, and a breaker stops crash loops while keeping the agent serving. A readiness check runs before anything trusts the process. Updates keep the previous binary, switch a symlink, and switch it back if the new binary is not ready within sixty seconds.

| Self-fixing tier | Who does it | When |
|---|---|---|
| Restart a dead process | systemd | Automatically |
| Repair state and replay the log | The agent, at boot | Automatically |
| Roll back a bad update | A supervisor script | Automatically |
| Replay a failed task as a test after a fix | The user, with one command | On demand |
| Propose a repair from its own logs | The model | Only after asking |
| Edit its own code | The model, in a branch, with tests | Never unattended |

---

## 10. Interfaces

There is one command table. The terminal prints the result, and Signal sends it.

| Command | What it does |
|---|---|
| `/new` and `/sessions` | Starts a fresh session, or lists sessions and switches between them |
| `/tasks` | Shows what is running, waiting, or done. `/tasks 17` shows one record, and `/tasks 17 back 3` rewinds it three checkpoints |
| `/model` | Shows or sets the model |
| `/status` | Shows the model, the token cost, the jobs, the pending approvals, and the health |
| `/stop`, `/pause`, and `/resume` | Stops this turn, or pauses and resumes all scheduled work |
| `/jobs` | Lists, runs, or disables scheduled jobs |
| `/approve 3` and `/deny 3` | Answers a preview or a question |
| `/screen` | Sends a screenshot of the browser or the desktop right now |
| `/memory`, `/skills`, and `/vault` | Show and manage each. The vault works only in the terminal |
| `/undo` and `/help` | Reverts the last turn's file changes, and shows this list |

**Signal.** The agent supervises signal-cli as an external daemon, linked as a secondary device to the user's phone. Unknown senders receive a pairing code. Photos and files the user sends are saved and can be read by the model. The agent sends one message per reply. A preview arrives as the actual post or command. A handoff arrives with a screenshot. The user replies with `approve`, `done`, a code, or `abort`.

**Terminal.** The terminal is a thin client of the running daemon. It draws its first frame at once, streams the reply as it arrives, shows approvals inline, shows screenshots inline where the terminal supports it, and uses a masked prompt for the vault. It owns no state.

---

## 11. Building it

**Language.** The daemon is written in Go. We measured it at eight milliseconds to start and nineteen megabytes of idle memory with every library it needs, in one static binary, with a ten-second build, and with a compile-and-fix loop that usually takes one round. The browser worker is written in TypeScript, because Playwright is built for TypeScript. Two API shapes cover every model: the Anthropic API and the OpenAI-compatible API with a base URL, which covers LM Studio, Ollama, llama.cpp, and every cloud gateway. A fallback chain of models is set in the configuration.

**Size.** The whole thing is about twelve to sixteen thousand lines, which is under one percent of OpenClaw.

**Tests first.** Every layer has the test that defines "done" before any code exists. A fake model emits identical, malformed, and text-shaped tool calls. A forty-step task must end with its ask and corrections byte-for-byte identical and with every result readable by id. A suspended task must resume from its checkpoint. A sandboxed command must fail to read the SSH keys. Twenty memory questions must be answered from the store. Recorded page fixtures must exercise the browser loop. A fake signal-cli must handle pairing, chunking, and reconnecting. A process killed in the middle of a turn must lose no reply. A bad update must roll back within sixty seconds. And any logged failure must be replayable as a test.

| Stage | What it adds | What you get |
|---|---|---|
| v0 | The daemon, the loop, the guard, five tools, one model provider, the terminal, the event log, and the record | A terminal agent |
| v1 | Signal with pairing, permissions, preview first, the sandbox with escalation, and the persona files | A phone assistant that does not get stuck and does not lose messages |
| v2 | The browser worker, the profile, the vault, login, handoff, act and assert, and human pacing | An agent that uses Chrome the way you do |
| v3 | Skills from demonstrations and documentation, memory, and the after-action review | An agent that learns and remembers |
| v4 | Scheduled jobs, the desktop worker, visual QA, the updater with rollback, and replay as test | An agent that is always on and fixes itself |

**Left out on purpose.** Dozens of chat channels and model providers, a web interface, plugins that run inside the process, a Python kernel, API shortcuts that bypass the browser, cookie copying, captcha solving, cloud browsers, vector memory in the first version, a reviewer that must always write something, and a model that updates itself.

---

## Sources

The operations order and its five paragraphs are described in [FM 5-0, The Operations Process](https://armypubs.army.mil/epubs/DR_pubs/DR_a/ARN35403-FM_5-0-000-WEB-1.pdf). Fragmentary orders are covered in [Operation, Warning, and Fragmentary Orders](https://www.globalsecurity.org/military/library/policy/army/accp/in0541/ch1.htm). Commander's intent is explained in the [Marine Corps Gazette](https://www.mca-marines.org/gazette/commanders-intent-defined/), and critical information requirements are explained at [The Fivecoat Consulting Group](https://www.thefivecoatconsultinggroup.com/tfcg/ccir). Boyd's loop, and why orientation is its center, is covered in [Chet Richards, Boyd's OODA Loop](https://ooda.de/media/chet_richards_-_boyds_ooda_loop.pdf). The four questions of the after-action review are described by [Nick Milton](http://www.nickmilton.com/2009/10/after-action-review-4-questions.html). The harness comparison and its seventeen source studies are in `HARNESS_V2.md` and `docs/research/`.
