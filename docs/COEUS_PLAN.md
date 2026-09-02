# COEUS

**Coeus is an open-source AI agent that runs on any Linux machine. You talk to it in a terminal or through the Signal messaging app. It works with any language model, large or small, running on your own machine or in the cloud. And it does not forget what it is doing.**

A few words in that summary deserve a plain explanation. Open-source means the code is public, and anyone may read it, use it, or change it. Linux is a free operating system that runs most servers and many small home computers. A terminal is the text window where you type commands to a computer. A language model is the program people usually mean when they say "the AI." It reads text and writes text back. An AI agent is a program that uses a language model to get work done for you, such as reading files, using websites, or sending messages.

This document is the design overview for the first version. Nothing is built yet. The next document will be the work plan for building it. This design grew out of a comparison of other agents, which is in `HARNESS_V2.md`. The research behind that comparison is in `docs/research/`.

The name comes from Greek mythology. Coeus was the Titan of intelligence.

---

## 1. The idea in one page

Every AI agent today is built the same way. At the center is a language model. The model works like a simple machine. Text goes in, and text comes out. One request sent to the model, together with the reply that comes back, is called a call. The model has no memory of earlier calls, and it has no way to act on the world by itself. The **harness** is the program wrapped around the model. It gives the model both of the things it lacks: a memory, and the ability to act. It acts by running tools on the model's behalf. A tool is one thing the harness knows how to do, such as reading a file, running a command, or opening a web page. When the model wants a tool run, it asks the harness, and that request is called a tool call. Everyone can use the same models, so the harness is what makes one agent different from another.

We studied the harnesses people run today: OpenClaw, Hermes, Prime, OpenCode, Atomic, ZeroClaw, and the two built by the model companies themselves, Codex and Claude Code. All of them keep long-term memory outside the conversation, in files or in a database. But every one of them uses the conversation transcript as the record of the task the agent is working on. The transcript is the full text of everything said so far by the user, the model, and the tools. On each turn, the model re-reads the whole transcript to work out where it is. A turn is one exchange: the user sends a message, and the agent works until it replies. When the transcript grows too long to fit, most of them summarize it and hope that nothing important was lost. Picture a video game that loads your saved game by replaying every button you ever pressed since you started playing. Re-reading the transcript is that replay.

Coeus is built on a different idea. It is not a new one.

**State.** In computer science, state is the small amount of information about the past that you need in order to act correctly next. A counter does not remember every time it was bumped up. It remembers only the current count, say seven. A database keeps a log of everything that happened. It also keeps a snapshot of what is true right now. Those two things are not the same. An operating system, such as Linux, can pause a program and resume it days later from one small record. Coeus keeps three things separate. The first is the history, which is what happened. The second is the state, which is what is true now. The third is the working context, which is what the model is looking at during this one turn. Picture a writer at work. The library holds everything ever written, and that is the history. The desk holds the few books open for today's chapter, and that is the state. The page in front of the writer is the working context. Nobody reads the whole library before writing the next sentence.

**The operations order.** The United States Army faces a larger version of the same problem. A headquarters cannot see every unit. Radios fail in the field. People rotate out in the middle of a mission. The Army's answer is a fixed document format that anyone can write and anyone can check. It is called the five-paragraph operations order, and it comes with rules for what to do when the plan breaks. We borrow its parts directly. The first part is the situation. The second is the mission, which includes what the user wants, called the intent, and a description of what "done" looks like. The third is the plan. The fourth is a list of conditions that should make the agent stop and report. Small changes to the order are delivered as short messages called fragmentary orders. A fragmentary order changes one part of the plan without restating the rest. When the work is over, there is an after-action review, which is a short look back at what happened and what to change next time.

**The result** is an agent that always knows what it is working on. It keeps working until the job is done. It stops and tells you when it is supposed to. And it spends its tokens, which are the small pieces of text that a model reads and writes, on the task instead of on re-reading its own past. Model providers, meaning the companies or programs that run models, charge by the token. Coeus works the same way on a small model running on your own machine and on a frontier model. A frontier model is one of the biggest and newest models, and it can hold a million tokens at once. Only the size of the working context changes. A big model is never shrunk to fit a small one.

```mermaid
flowchart LR
  H["History<br/>what happened"] -->|"folded into"| S["State<br/>what is true now"]
  S -->|"always included"| W["Working context<br/>what the model sees"]
  H -->|"fetched by id"| W
  W -->|"produces"| H
```

History is the log of every message, every tool call, and every result. It is append-only, which means new lines are added at the end and old lines are never changed or removed. Elsewhere in this document the history is also called the event log. The history is never placed in front of the model by default. State is the task record, which is one to three thousand tokens long and is shaped like an operations order. The working context is what the model actually sees on a turn. Another word for it is the prompt, which means everything the harness sends to the model in one call. It holds the harness rules, the persona, the task record, any pinned evidence, and the recent messages. The persona is a description of who the agent is and who the user is. Pinned evidence is a past result the model asked to keep in view, word for word. The size of the working context is chosen to fit the model.

---

## 2. What we learned from the others

We read the source code, not the marketing. The full comparison is in `HARNESS_V2.md`. This is the short version.

**What went wrong.** The two most popular harnesses are 2.9 million and 1.5 million lines of hand-written code. One of them merged 10,574 commits in a single month. A commit is one saved batch of changes to the code. Of those commits, 63 percent came from one person, at a rate of 223 commits a day. Nobody reviews code at that pace, and the result is software that breaks on every update. Neither of them puts a hard limit on how many times the model can call tools in one turn. So "it got stuck" is a built-in outcome rather than a bug. One of them keeps its screen and its reasoning in two different processes, meaning two separate running programs. The two are joined by a pipe, which is a channel that passes text from one program to the other. And that pipe breaks. The agent loop is the part of the code that calls the model, runs the tools it asks for, and repeats. The agent loops themselves are small, between one and seven thousand lines. The code wrapped around the loop is fifty to seventy-five times bigger, and that wrapper is where everything fails.

**What worked.** The next table lists what worked in each project and what we keep from it. The table uses some terms that come up again later, so here is what they mean. A cache is the model provider's saved copy of the part of a prompt it has already read. The provider can reuse that part for much less money, and a cache boundary is the point in the prompt where the reused part ends. A sandbox is a walled-off area of the computer where a program can run without touching anything outside it. An API is a set of commands that one program exposes so that other programs can talk to it. A hook is a place in a program where extra code can be attached to run at a set moment. Backoff means waiting a little longer before each retry. A tool round is one exchange in which the model asks for tools and gets the results back. A session is one conversation with the agent. A browser profile is the folder where Chrome keeps its logins, history, and settings. A reference tag is a short label the agent gives to each element on a web page so that it can point at that element later. Below the fold means the part of a page you cannot see without scrolling. Streaming means sending a reply piece by piece as it is written, and a delta is one such piece. A thin client is a screen that shows what a program is doing but holds none of its state. Redaction means blacking out secrets before text leaves the program.

| From | What we keep |
|---|---|
| **OpenClaw** | A pure agent loop with hooks around it. A message that arrives in the middle of a turn steers the next step instead of interrupting the current one. A repair layer for models that write their tool calls as plain text instead of structured data. Scheduled jobs with backoff after failures. Signal support through the signal-cli program, with pairing required for unknown senders. A cache boundary in the prompt. A delivery queue stored on disk. Launching a real Chrome web browser with its own user profile and reading the web page as a tree with short reference tags |
| **Hermes** | The user's own words are never summarized. Memory files with hard size limits. A breaker that stops crash loops. One lease per session, so two turns cannot run at once. A delivery ledger that admits when a message may be a duplicate. A masked prompt for entering passwords. One redaction function applied to everything that leaves the program |
| **Prime** | Notes with a history and a rollback that the next prompt actually reads. Scheduled jobs used as a heartbeat. Budgets with a checker at the end |
| **OpenCode** | One core with an API inside it, so that every screen is a thin client. Streaming replies as small deltas. One command table shared by every surface. Approvals shown inline, with the choices once, always, and reject. A permission engine built from rules, where the last matching rule wins and the default is to ask. Tool descriptions written as plain text. A bad tool call becomes an error the model can fix, never a crash |
| **ZeroClaw** | A cap of ten tool rounds followed by a forced final answer. A parser for messy tool calls. A job store where two processes cannot run the same job. An updater that keeps the old program and rolls back if the new one fails |
| **browser-use, Stagehand, agent-browser** | Marks on the web page elements that just appeared. Hints about how much of the page is below the fold. Aborting a batch of actions when the page changes underneath it. A finish step that must state whether it succeeded |
| **Codex and Claude Code** | The operating-system sandbox is the real security boundary. Escalation is a field on the tool call with a written reason. The model decides what to do, the harness decides what is allowed, and the two never share a layer |
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

A message from the user goes into a queue on disk. The queue is a waiting line of messages, saved to the hard drive so that none are lost. The router, which is the part of the harness that sorts messages, looks at each message and decides what it is. It might be a command, meaning one of the commands that start with a slash, such as `/status`, listed in section 12. It might be a trigger for a saved skill, which is a procedure the agent already knows. Or it might be a real task. A command or a skill runs its tools directly, without calling the model. A task goes through the agent loop.

First the agent orients, which means it works out where it stands. The harness places the task record in front of the model, and the model states where the work stands and what comes next. Then the harness calls the model with the full working context: the harness rules, the persona, the task record, any pinned evidence, and the recent messages.

The model may reply with a tool call. If it does, the guard checks the request before anything runs. The guard is a set of checks the harness runs on every tool call. It looks for repeated calls, badly formed calls, the cap on tool rounds, and the stop conditions. Next, the permission function, which is the harness's rulebook for what is allowed, decides whether to allow the call, ask the user, or deny it. Anything that cannot be undone is shown to the user first, as a preview of exactly what is about to happen.

The tools run inside the sandbox. Each tool has a time limit and a cap on how much text it may return, and it returns only what changed. The harness writes the results into the task record, and the agent loop goes back to orienting. When the model replies without asking for tools, the turn ends. When the whole task ends, the done-check runs, which confirms that every part of "done" is actually true. Then the after-action review runs. What was learned goes into memory and into skills.

One process owns everything: the queue, the agent loop, the permissions, the task record, the memory, the scheduled jobs, and one database file. The database is SQLite, which keeps everything in one ordinary file on disk. That one process starts two helper programs when it needs them. One is a browser worker, which runs a real Chrome web browser launched with its own user profile, never the user's daily Chrome profile. The other is a desktop worker, which controls the screen, the mouse, and the keyboard. They are separate processes so that a frozen browser can never take the agent down. There are two screens, the terminal and Signal. Neither screen holds any state, and both use the same list of commands.

### The turn, in ten rules

1. A message that arrives in the middle of a turn is treated as a fragmentary order. It changes only what it changes. It is like texting a driver "take the next exit" without re-sending the whole route. The harness writes the message into the task record as a correction, and the model re-plans from there. The message never interrupts a tool that is already running.
2. The model orients before it acts. Before any tool call, it writes one line stating where the work stands and what comes next. If the situation does not match the plan, the plan is fixed first.
3. There is a cap of twenty tool rounds per turn, on every model. When the cap is reached, the model gets one last call with tools turned off. In that call it is asked to say what it did and what is left. A long task also gets a budget of rounds, tokens, and minutes.
4. If the model makes the same call twice with the same arguments, the harness does not run it a second time. The arguments are the details the model fills in, such as which file to read. Instead, the harness tells the model to do something different or to answer. A third identical call ends the turn.
5. A badly formed tool call is repaired if the tool name is close to a real tool's name. Otherwise the model gets back the list of real tools. Text that merely looks like a tool call is read as one. The agent loop never crashes because of something the model wrote.
6. Every error message ends the same way, with three options. The model can answer the user, ask one question, or try different arguments.
7. Every tool has a time limit, its own process group, and a cap on its output. A process group means the tool and anything it starts can be stopped together. Anything over the output cap goes to a file, and the result tells the model where the file is.
8. A turn that has read a web page or a feed is marked as tainted. Tainted means the turn has taken in words that did not come from the user. Nothing that cannot be undone can happen in a tainted turn until the user speaks again. An unattended run that hits a question stops and reports.
9. Retries live outside the agent loop. When a call to the model fails, the harness tries three times, waiting a little longer before each try. Then it moves to the next model in the chain, which is the list of backup models set in the agent's settings.
10. A question ends the turn. The model asks in plain text, and the task is marked as waiting. The user's next message resumes it, even if that message comes days later.

---

## 4. State: three kinds, one format

### Three kinds of state

| Kind | What it holds | How often it changes | Where it sits |
|---|---|---|---|
| **Persona** | Who the agent is, who the user is, the standing rules, and durable memory | Rarely | At the start of every prompt, where it is cheapest |
| **Skill** | How to do one kind of thing: the steps, what to expect at each step, the permissions, and the known failures | When a website or a tool changes | Loaded only when the skill is used |
| **Task** | What is true right now about the thing being worked on | Every turn | In the task record, which is always in context |

The three kinds are kept apart because they change at different speeds. The persona almost never changes. A skill changes only when a website or a tool changes. The task changes every turn. A carpenter makes this easy to picture. The persona is who the carpenter is. A skill is a joint the carpenter has learned to cut and can cut again without thinking. The task is the one cabinet sitting on the workbench today. Only the cabinet changes from day to day. Keeping the three kinds apart also saves money. The model provider can reuse the parts of a prompt it has already read, and it charges much less for them. So the more of the prompt that stays the same from call to call, the less each turn costs.

### The task record, shaped like an operations order

An Army order has five paragraphs: situation, mission, execution, sustainment, and command and signal. The mission paragraph answers who, what, when, where, and why. It also carries the commander's intent, which is the purpose behind the mission. And it carries the desired end state, which is what things should look like when the mission is over. Those two let the unit act correctly even when the plan breaks. Before the operation, the commander also lists the facts that would change a decision. That list is called the critical information requirements. Our task record uses the same shape, with the user in the commander's place.

Here is one way to picture the task record. A pilot straps a small card to one knee during a flight. The card holds the mission, the current leg of the flight, and the rules for when to abort. That card is the task record. The full flight log stays on the ground, and that log is the history.

One term in the next table needs explaining first. A login wall is a web page that demands a sign-in before it shows anything.

| Section of the record | Paragraph of the order | Who writes it |
|---|---|---|
| Ask | Mission: who and what | The user, word for word. It is never edited |
| Intent and Done | Mission: why, and the end state | The model drafts it, and the user can correct it |
| Corrections | Fragmentary orders | The user, word for word. They are only ever added to |
| Stop and tell the user if | Critical information requirements | The model drafts the list, and the harness adds the budget and login walls |
| Situation | Situation | The harness, from the world as it is right now |
| Plan, Decisions, Failures | Execution | The model, through the `task` tool |
| Results, budget, and source | Sustainment, and command and signal | The harness |

Here is a complete example of a task record.

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

**What goes in.** The task record holds only what the world cannot answer on its own. Which files changed is answered by `git diff`, the command that lists the changes made to a project's files. What is on the web page is answered by the browser's snapshot of the page. Whether a job ran is answered by the list of scheduled jobs in the database. Those things are looked up, never remembered. The record holds what only the conversation knows: what was asked, why it was asked, what was corrected, what was decided and why, and what failed and why.

**Who writes what.** The harness writes everything it can verify, and it does so without spending any model tokens, because ordinary code does the writing. That covers the header line at the top of the record, the corrections, the situation, the results, and the status of each step based on how its tools turned out. It also covers a failure line whenever a tool errors or an expectation is not met. The model writes the parts that require judgment: the intent, the stop conditions, the plan, the decisions, and the failures it understands. The stop conditions work like a smoke detector. You decide what counts as an alarm before there is a fire, not during one. The ask, the intent, and the corrections are never edited by anyone.

**Size.** The task record stays between one and three thousand tokens. When it grows past that, the harness folds finished steps and old result lines. Folding means squeezing each of them down to a single line. Nothing is deleted, and every result stays readable by its id, which is a short label such as r7.

**Cache.** Model providers charge much less for the part of a prompt that is identical to the previous call, because they can reuse their work on it. That reused part is the cache. Coeus takes advantage of that by putting the parts of the task record that rarely change first. Those parts are the ask, the intent, the corrections, the stop conditions, and the decisions. The parts that change every turn come after: the situation, the plan status, and the results. The line between the two groups is called the cache line in the rest of this document.

**Checkpoints.** A checkpoint is a saved copy of the task record at one moment, like a saved game. Every change to the record saves a numbered checkpoint tied to the log. A task that is waiting on the user resumes from its last checkpoint. In the meantime, nothing is held in any context window, which is the most text a model can look at in one call. The command `/tasks 17 back 3` reloads an earlier checkpoint and lets the model try a different path, the way you would reload a saved game. Any failed task can be replayed from a checkpoint as a test after a fix.

### The done-check and the after-action review

A task cannot close until every line under "Done" has been answered true, with evidence. Those lines are a checklist, and the harness works through them the way a pilot does before landing. A pilot does not land because the flight feels finished. The pilot checks the landing gear, the flaps, and the clearance from the tower, one at a time. The finish was defined before the work started, and the harness will not let the model declare victory otherwise.

Then come the four questions of the Army's after-action review, answered in one line each. What was supposed to happen? What actually happened? Why was there a difference? What do we keep, and what do we change? It is the way a football coach reviews the game film with the team, and here the game film is the task's history. The answer to the last question is what goes into memory or into a skill. This replaces the vague "save what you learned" step that every other harness uses.

### Working context: one rule, sized to the model

Context is the model's working memory for a single call. Nothing carries over from one call to the next except what the harness puts back. Coeus has one rule for it. **The working context is never smaller than the task needs and never bigger than the model can hold.** The task record is the same size on every model. The window around it is the only thing that changes. On a small model running on your own machine, the window is a few thousand tokens. On a frontier model, it is most of a million. A big model is never held back to suit a small one.

Two facts about tokens decide the layout. First, the part of a prompt that is identical to the previous call costs about a tenth as much as new text, because the provider reuses its work. So the order of the prompt matters more than its size, and the parts that stay the same must come first. Second, adding text to the end of a prompt is cheap, while rewriting text is expensive. A rewrite changes what the provider has already read, so the cache is lost. Agents that summarize their transcript rewrite their entire history every time they compact it. Compacting means squeezing the transcript down to make room. Coeus only adds to its history, and the only thing it rewrites is a task record of one to three thousand tokens.

The prompt is built in layers. They are ordered from the part that changes least to the part that changes most. The cache point column marks three places in the prompt. Everything above each of those places can be reused from the cache.

| Layer | How often it changes | Cache point |
|---|---|---|
| Harness rules and persona | Rarely | A |
| Tools | When the software is updated | B |
| Record, stable part: ask, intent, corrections, stop conditions, decisions | Rarely during a task | C |
| Record, live part: situation, plan status, results | Every turn | |
| Pinned evidence, kept word for word | When something is pinned | |
| Recent messages, appended and never rewritten | Every turn | |
| Memory hint, three lines from search | Every turn | |

**Tiered folding.** When a message or a result leaves the recent window, it does not vanish, and it is not summarized. It drops one tier. First it sits in the window, word for word. Then it becomes a one-line entry in the task record. Then it lives only in the log, where `read r7` brings it back in full. It is like packing for a trip. Today's clothes are on the chair, and the chair is the recent window. This week's clothes are in the closet, and the closet is the task record. The rest are in the suitcase, and the suitcase is the log. Nothing gets thrown away. A large model rarely folds anything. A small model folds constantly and loses nothing.

**Cost on every turn.** The harness knows the token count of each layer and the cache hit rate, which is how much of the prompt the provider was able to reuse. It writes one line into the task record header, such as "this turn: 6.1k in, 5.2k of it cached, 0.4k out." The `/status` command totals the cost per task. The user can always see what a task cost and where the tokens went.

**The proof.** There is a test task with forty steps. It runs on every supported model size every time the software is built, and it checks the same three things every time. The ask and the corrections are identical at the end, down to the last character. The done-check passes at the end. And no result has become unreadable. "Works on any model" is a test, not a claim.

**Why not just use the million tokens.** A model's attention gets spread thin. A model reasons worse over a million tokens of noise than over thirty thousand tokens of signal. The leaders on the public tests for long tasks all use explicit plans and memory for exactly that reason. The task record is also what lets a task be paused for three days and resumed, possibly on a different model, with nothing held in any window. A million-token context window is a bigger window. It is not a save file.

---

## 5. What the model is told

The model works inside a harness. It cannot do its job well unless it understands what the harness does for it and what the harness expects from it. So the first thing in every prompt, before the persona and before the tools, is a short explanation of the harness written for the model. It is the same on every model, and it is under four hundred words. Here it is in full.

> **Where you are.** You are the reasoning engine inside Coeus, an assistant that runs on the user's computer. You do not remember earlier calls. The harness around you does. On every call it gives you, in this order: these rules, your persona, your tools, the record of the current task, any evidence that has been pinned, the most recent messages, and a short memory hint. Everything else that ever happened is stored on disk, and you can fetch any past result by its id.
>
> **The task record is the truth.** The record tells you what the user asked, why, what they corrected, what has been decided, what has failed, and where the work stands. Trust the record over your own recollection of the conversation. Your first line on every turn states where the work stands and what you will do next. If what you see does not match the plan, update the plan before you act.
>
> **Your part of the record.** Use the `task` tool to update the plan, add a fact with its source, record a decision with its reason, or record a failure with its cause and what not to do again. The harness fills in the rest. You cannot change the ask, the intent, or a correction, and you should not try.
>
> **When to stop.** Stop when any "stop and tell the user" condition is true, and say which one. Otherwise keep going until every line of "done" is true or the budget runs out. To ask the user something, ask in plain text and end your reply. The harness will resume you when the answer arrives.
>
> **Tools.** Call a tool only when you need it. Never make the same call twice with the same arguments. If a result was cut short, read the file the result names. Never type a password into anything. Use the login tool. Nothing irreversible will run without the user seeing a preview first.
>
> **How to write.** Use plain, short English that a high-school student could follow. Avoid jargon. When a technical term is needed, explain it simply. Match the length of your reply to the question. State facts, and say "not sure" when you are not sure. When work is done, report three things: what changed, what you checked, and what is left.

The harness enforces what it can, so the model does not have to be trusted on those points. After every turn, the harness checks that the ask and the intent have not changed by so much as a character. Every fact line must name a source, and every failure line must name a cause, or the `task` tool rejects the update. Every step marked done must have a result behind it. A task cannot close until the done-check has been written. If any check fails, the turn does not close, and the model receives one line naming the rule.

---

## 6. Built to grow: the four extension points

The first version has exactly two screens, the terminal and Signal, and a small set of tools. But other people will want Telegram, Slack, iMessage, or a web page, and every user will want to add tools and skills of their own. So the core is built around four fixed shapes, and the things that fit those shapes can multiply. Those four shapes are the extension points, which are the places where new pieces plug in. Adding a Telegram connection later is a new file that fits the channel shape. It does not touch the agent loop, the task record, the permissions, or the memory.

```mermaid
flowchart LR
  subgraph Core["The core, fixed"]
    L["Loop"] --- R["Record"] --- P["Permissions"] --- M["Memory"]
  end
  C["Channels<br/>terminal, Signal,<br/>later Telegram, Slack"] --> Core
  T["Tools<br/>18 built in,<br/>plus your own"] --> Core
  S["Skills<br/>a folder each"] --> Core
  MP["Model providers<br/>Anthropic, OpenAI-style,<br/>later others"] --> Core
```

**Channels.** A channel is anything a user can talk through. Every channel does the same six things. It receives a message. It sends a message. It sends a file. It shows a preview and collects an approve or a reject. It shows a masked prompt for a secret if it can, meaning a place to type a password that shows asterisks instead of letters. And it reports whether it is healthy. The terminal and Signal are the first two channels. Telegram, Slack, or iMessage would each be one new file that does those six things. Everything a channel receives goes into the same queue. Everything it sends comes from the same event stream, which is the live feed of everything the agent loop does. So the loop never knows which channel it is talking to.

**Tools.** A tool has five parts. It has a name. It has a plain-English description under forty words that says when to use it and when not to. It has a list of input fields with their types, meaning what kind of value each field takes. It has a permission class, which is explained in section 7. And it has a function that runs it and returns text. That is the whole contract. The eighteen built-in tools follow it. A user's own tool is a small program in a `tools/` folder that follows it too. The harness reads the folder at startup, adds those tools to the list the model sees, and puts them through the same permission function as everything else. MCP, which stands for Model Context Protocol, is a standard the industry shares for connecting tools to models. It fits the same shape, so a later version can load MCP servers, which are programs that offer tools in that standard, without changing the core.

**Skills.** A skill is a folder with four things in it: a description file, the recorded steps or a script, a dry-run test, and a changelog. Section 8 describes each of them. Because a skill is just a folder, people can share skills by sharing folders.

**Model providers.** A provider is the piece of the harness that sends the prompt to a model and brings the reply back. It streams the reply back piece by piece as it is written, and it reports how many tokens it used. Two providers cover nearly every model in existence. One is the Anthropic API. The other is the OpenAI-compatible API with a base address, meaning the web address of whatever server is answering. LM Studio, Ollama, and llama.cpp, which are programs that run models on your own machine, all present themselves that way. So does every cloud gateway, which is a service that forwards requests to models in the cloud. A third provider would be one new file. The model's name and the fallback chain are set in the configuration, which is the agent's settings file. The fallback chain is the list of backup models to try when the first one fails.

**What stays fixed.** The agent loop, the task record format, the permission function, the memory layers, and the event log have no extension points. They are the core, and keeping them closed is what keeps the agent simple.

---

## 7. Tools

There are eighteen tools built in, and all of them are shown to every model. In the class column, R means the tool reads, and W means it writes. X means it runs a program, and N means it uses the network. I means it does something that cannot be undone. Every description is plain text under forty words, and it says when not to use the tool.

A few rows use terms that need a word of explanation. A regular expression is a pattern for matching text, such as "any line with a phone number in it." A cron expression is a short code that says when a job should run, such as every weekday at nine in the morning. Sudo is the Linux command that runs something with administrator powers. A job id is a label for a command that is still running. The model can poll the job, which means check on it. It can tail the job, which means watch the output as it comes. Or it can kill the job, which means stop it.

| Tool | What it does | Class |
|---|---|---|
| `read` | Reads a file, a directory listing, or a past result by its id | R |
| `write` | Creates a file or overwrites an existing one | W |
| `edit` | Replaces one exact span of text in a file | W |
| `search` | Finds files by name pattern, or finds lines by regular expression | R |
| `shell` | Runs a command. After ten seconds it returns a job id that can be polled, tailed, or killed. An `escalate` field with a written reason requests sudo | X |
| `web` | Searches the web, or fetches a public web page as text. Anything that needs a login or a click belongs to the browser tools | N |
| `memory` | Searches, gets, or saves a memory | R and W |
| `task` | Updates the plan, adds a fact with its source, records a decision or a failure with its reason, or pins a result | W |
| `skill` | Views, runs, or saves a skill | R, X, and W |
| `schedule` | Creates one scheduled job that runs at a time, on an interval, or on a cron expression | I |
| `browser_open`, `browser_read`, `browser_click`, `browser_type`, `browser_act` | The web browser tools, described in section 9 | N and X |
| `browser_login`, `browser_handoff` | The web browser tools that involve credentials or the user, described in section 9 | N and I |
| `computer` | The desktop tool, described in section 10 | X and I |

Tool results are built by the harness. They are never pulled out of the model's own text, so a model cannot invent a result. Asking the user a question is not a tool. The model simply asks in plain text, and the turn ends in a waiting state.

---

## 8. Skills

A skill is a saved procedure for one kind of job. It works like a recipe card. The first time you cook a dish, you have to think about every step. After that, you follow the card and only stop to think when something looks wrong. The card is the skill, and the cook is the agent.

On disk, a skill is a folder that contains four things. The first is a file named `SKILL.md`. It holds the skill's name, a one-line description, the words that trigger it, and its permissions. The permissions cover the browser profile it may use, the websites it may visit, its daily limit, and which of its steps cannot be undone. The second is a file of recorded steps, or a script, which holds the procedure itself. The third is a dry-run test, which runs the procedure up to the first step that cannot be undone and then stops. The fourth is a changelog, which records every change to the skill along with a way to undo that change.

A skill is born in one of three ways. The user demonstrates the task once while the harness records it. The user points the agent at documentation, and the agent writes the skill from it. Or the agent finishes a task and offers to save the procedure it just used. Only the names and one-line descriptions of skills sit in the prompt. The body of a skill loads when the skill is used. When a skill's trigger words match a message, the router runs the skill directly, without calling the model. The model is only called if a step fails.

---

## 9. The web browser, used like a human

The web browser is the centerpiece of the design. The agent gets a real Chrome web browser, launched with its own user profile folder, and never the user's daily Chrome profile. The user logs in to each website once by hand, or the agent fills in the username and password from the vault. The vault is the agent's encrypted store of secrets, and section 11 describes it. After that first login, the saved cookies are the login. Cookies are the small pieces of data a website stores in the browser so that it remembers you are signed in. From then on, the agent uses the site the way the user would.

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

Every browser action works the same way. The model asks for an action and states what it expects to happen. For example, it asks for a click on the compose button and expects that a text box will appear. The browser worker then goes through the following steps.

The browser worker finds the element by its reference tag. That tag is the short label the agent gave the element in its last snapshot of the page. If the tag has gone stale because the page changed, the worker finds the element again by its role and its name. Role and name mean what kind of element it is and what it is called, such as a button labeled "Compose." If that fails, it looks for the element by its visible text. It scrolls the element into view and waits until it is visible, enabled, and no longer moving. It performs the action with human pacing, meaning at the speed a person would. It waits for the page to settle. Settled means either that a move to a new page has finished or that nothing on the page has changed for three hundred milliseconds, with a limit of three seconds. It takes a new snapshot of the page and compares it with the old one. It notes any change of address, any new elements, any dialog box, any new tab, and any download.

If the expectation was met, the browser worker returns the difference and a fresh snapshot. If a click produced no visible change, it retries once by clicking at the element's spot on the screen, and then it reports. If the expectation was not met, it reports what it expected and what it saw instead. If at any point it hits a login wall, a two-factor prompt, or a captcha, it hands off to the user. A two-factor prompt asks for a second code after the password. A captcha is a puzzle meant to prove you are human. To hand off, the worker brings the browser window to the front and sends the user a message. Then it waits for the user to say "done."

**What the model sees.** The model sees a compact tree of the web page with short reference tags, a few hundred tokens in all. A tree here means an outline of the page's parts, nested the way they appear on the page. Every link, button, and text field appears with its name and its tag. Elements that just appeared are marked as new. A count shows how much of the page is below the fold, which means below the part you can see without scrolling. A screenshot with numbered marks on the clickable elements is available with one call.

**Act and assert.** Every action carries an expected result, and the browser worker checks that result before the next step. To assert means to check that what you expected is actually true. A good mechanic tightens a bolt and then tries to turn it by hand to make sure it is tight. That hand check is the assert. This is how the programs that test websites automatically get reliable, and it is why the agent will not click the wrong thing three times in a row.

**Like a human.** The agent uses a real Chrome web browser so that its fingerprint matches a real browser. A fingerprint is the set of details a website can see about the browser that is visiting it. The browser runs with its window visible on the machine's own display, on the user's own network connection. It moves the mouse along a curve, holds a click for a human length of time, and types one key at a time with small variations in speed. It scrolls in steps and pauses between actions. Each website gets a daily budget of actions. There are no proxy servers, which are middlemen that hide where a connection comes from. There is no invisible headless mode for logged-in accounts, meaning the browser is never run without a visible window. There is no copying of cookies from one browser to another, and there is no captcha solving. All of this is about keeping the user's accounts safe.

**Everything a human can do.** The browser worker handles tabs and popup windows, frames inside pages, uploads and downloads, dialog boxes, and PDF files read as text. It also handles keyboard shortcuts, drag and drop, hovering, and the back and forward buttons.

**Skills in the browser.** The user records a procedure once. The harness logs each step's intent, the element it used, and what was expected to happen. Replaying the skill needs no model at all. When a website changes and a step fails, the model finds the element that matches the recorded intent and proposes a one-line patch, which the user approves.

**The engine.** The browser worker is built on Playwright, a browser automation library from Microsoft, meaning a set of ready-made code for driving a browser. There is one long-lived Playwright program per browser profile, attached to the Chrome web browser that the agent launched. The runner-up choice is Vercel's agent-browser, which is a single program written in the Rust programming language.

---

## 10. The desktop, command-line tools, and memory

**The desktop.** There is one `computer` tool, and it runs through the desktop worker. It can launch or focus an application, take a screenshot with numbered marks on the clickable elements, click, type, press keys, drag, and use the clipboard. It uses the same human pacing and the same act-and-assert loop as the browser. The user grants the agent access to an application once per session, meaning once per conversation. Actions inside that application that cannot be undone still get a preview. The desktop is the last resort. If the browser can do the job, the browser does it.

**Any command-line tool.** A command-line tool is a program you run by typing its name in the terminal. The `shell` tool plus a skill covers this. When the user hands the agent a new command-line tool, the agent reads the tool's help text and documentation. It writes a skill containing the commands it will actually use, with one example of each. It runs a smoke test, which is a quick check that the basic commands work, and it saves the skill.

**Memory.** The persona holds two files with hard size limits. `MEMORY.md` holds facts about the world, and `USER.md` holds facts about the user. There is also a folder of markdown files, which are plain text files with simple formatting, for anything larger. All of it is indexed by full-text search, which is search that looks at every word in every file, along with every past message. The harness records what it can verify without spending any model tokens. That covers files changed, commands run, websites visited, and any user message that starts with "no," "actually," "always," "never," or "don't." That kind of message is kept word for word as a correction. The after-action review writes the rest. Every fact has a source and a date. Nothing is ever deleted. A new fact replaces the old one, and the old one stays searchable. A three-line memory hint rides below the cache line on every turn, and the model can search for more.

---

## 11. Safety, the vault, and reliability

**Five safety rules.** First, one permission function checks every tool call. It is built from rules, and each rule names a tool, a pattern that says which files or commands it covers, and an action. The last matching rule wins, and the default is to ask the user. Second, every shell command and every file-writing tool runs inside a sandbox built on bwrap and Landlock. Those are two Linux tools for walling a program off from the rest of the machine. The sandbox works like a workbench with a raised edge. The bench is the sandbox, and the floor is the rest of the machine. Whatever rolls away stays on the bench instead of falling to the floor. The vault, the browser profile, and the user's SSH keys are always outside the sandbox. SSH keys are the keys used to log in to other machines. If the sandbox is missing from the machine, the shell tool is turned off. Third, a tainted turn cannot do anything that cannot be undone. Fourth, nothing that cannot be undone runs without a preview, the same way you read a text message back before you hit send. Fifth, secrets are handled as references and never as values. That means the model sees a name that points at a secret, never the secret itself. One redaction pass, which blacks out secrets, runs on everything that leaves the program. And everything is logged.

**The vault.** The vault is an encrypted file. Its key can be read only by the agent's own user account on the machine. Secrets are entered only in the terminal, through a masked prompt that shows asterisks, and never over Signal. The model never sees a secret. It points at the login fields on the page, and the `browser_login` tool types the username, the password, and the two-factor code itself. Sudo has its own path. A shell call with the `escalate` field and a written reason produces a preview. When the user approves it, the harness runs the command outside the sandbox with the sudo password from the vault. The database, the vault, and the browser profile are backed up every night in encrypted form.

**Reliability.** The agent runs under systemd, the Linux service manager, which starts programs and restarts them when they die. It has a watchdog line, which means systemd restarts the agent if the agent stops checking in. It uses exit codes, the numbers a program reports when it quits, to mean either "restart me" or "bad configuration, stop." There are caps everywhere: twenty tool rounds per turn, fifteen minutes per turn, seven minutes per tool, and one hundred queued messages. The event log is the ledger, meaning the one record that decides what really happened. A reply is logged before it is sent. After a crash, the agent replays the log to rebuild its state. A resent message says that it may be a duplicate. A breaker stops crash loops while keeping the agent serving. A crash loop is when a program keeps crashing and restarting in a circle, and the breaker is what breaks the circle. A readiness check, which confirms that the agent is actually able to work, runs before anything else relies on it. Updates keep the previous program and switch a link to the new one. The link is a pointer on disk that says which copy of the program is the live one. If the new copy is not ready within sixty seconds, the link is switched back.

| Self-fixing tier | Who does it | When |
|---|---|---|
| Restart a dead process | systemd | Automatically |
| Repair state and replay the log | The agent, at startup | Automatically |
| Roll back a bad update | A supervisor script | Automatically |
| Replay a failed task as a test after a fix | The user, with one command | On demand |
| Propose a repair from its own logs | The model | Only after asking |
| Edit its own code | The model, in a branch, with tests | Never unattended |

---

## 12. Interfaces

There is one command table, meaning one list of commands that works the same everywhere. The terminal prints the result, and Signal sends it as a message.

| Command | What it does |
|---|---|
| `/new` and `/sessions` | Starts a fresh session, or lists sessions and switches between them |
| `/tasks` | Shows what is running, waiting, or done. `/tasks 17` shows one record, and `/tasks 17 back 3` rewinds it three checkpoints |
| `/model` | Shows or sets the model |
| `/status` | Shows the model, the token cost, the jobs, the pending approvals, and the health |
| `/stop`, `/pause`, and `/resume` | Stops this turn, or pauses and resumes all scheduled work |
| `/cron` | Lists every scheduled job with its name, its schedule in plain words such as "every weekday at 7 in the morning," what it does in one line, when it last ran, and when it runs next. `/cron 3` shows one job in full, `/cron run 3` runs it now, and `/cron off 3` disables it |
| `/approve 3` and `/deny 3` | Answers a preview or a question |
| `/screen` | Sends a screenshot of the browser or the desktop right now |
| `/memory`, `/skills`, and `/vault` | Show and manage each. The vault works only in the terminal |
| `/undo` and `/help` | Reverts the last turn's file changes, and shows this list |

**Signal.** The agent supervises signal-cli, an external program that connects to Signal. It is linked to the user's Signal account as a secondary device, the same way the Signal desktop app is linked to a phone. Unknown senders receive a pairing code. Photos and files the user sends are saved, and the model can read them. The agent sends one message per reply. A preview arrives as the actual post or command. A handoff arrives with a screenshot. The user replies with `approve`, `done`, a code, or `abort`.

**Terminal.** The terminal is a thin client of the running agent. A thin client is a screen that shows what the agent is doing but holds none of its state. It shows its first screen at once, and it shows the reply as it arrives, word by word. It shows approvals inline, and it shows screenshots inline where the terminal supports it. It uses a masked prompt for the vault. It owns no state.

---

## 13. Building it

**Language.** The agent itself is written in Go, a programming language. We measured a Go daemon, meaning a program that runs in the background all the time, built with every library it needs. A library is a piece of ready-made code that a program depends on. The daemon started in eight milliseconds and used nineteen megabytes of memory while idle. It ships as one static binary, which is a single file that runs with nothing else installed. It builds in ten seconds. The compile-and-fix loop usually takes one round. That loop is the cycle of building the program, reading the errors, and fixing them. The browser worker is written in TypeScript, because Playwright is built for TypeScript. Two API shapes cover every model: the Anthropic API, and the OpenAI-compatible API with a base address. The second one covers LM Studio, Ollama, llama.cpp, and every cloud gateway.

**Size.** The whole thing is about twelve to sixteen thousand lines of code, which is under one percent of OpenClaw.

**Order of work.** The terminal comes first, then Signal, then the browser, then skills and memory, then scheduled jobs and the desktop. The next document is the work plan. It will use test-driven development, which means the tests that define "done" are written before the code. The tests come at four levels. Unit tests check one small piece at a time. Integration tests check that the pieces work together. Functional tests check the whole program from the outside, the way a user would. Fuzz tests throw random and broken input at the program to find crashes. The work will be done in waves of five sub-agents at a time, with a more capable orchestrator directing them. A sub-agent is an AI coding agent given one piece of the work. The orchestrator is a stronger model that hands out the pieces and checks them.

| Stage | What it adds | What you get |
|---|---|---|
| v0 | The agent process, the loop, the guard, five tools, one model provider, the terminal, the event log, and the record | A terminal agent |
| v1 | Signal with pairing, permissions, preview first, the sandbox with escalation, and the persona files | A phone assistant that does not get stuck and does not lose messages |
| v2 | The browser worker, the profile, the vault, login, handoff, act and assert, and human pacing | An agent that uses Chrome the way you do |
| v3 | Skills from demonstrations and documentation, memory, and the after-action review | An agent that learns and remembers |
| v4 | Scheduled jobs, the desktop worker, visual QA, the updater with rollback, and replay as test | An agent that is always on and fixes itself |

**Left out on purpose.** We are leaving several things out on purpose. There will be no dozens of chat channels and model providers, and no web interface. There will be no plugins that run inside the process, meaning add-ons loaded into the agent's own program. There will be no Python kernel, which is a built-in engine for running Python code. There will be no API shortcuts that bypass the browser, no cookie copying, no captcha solving, and no cloud browsers. There will be no reviewer that must always write something, and no model that updates its own code. Vector memory, which is memory searched by meaning rather than by exact words, is left out of the first version.

**Code that can be studied while building.** These projects are on disk or were copied down during the research. Each has a study file in `docs/research/` with citations down to the line of code.

| Project | Where | What to look at |
|---|---|---|
| OpenClaw | `~/Code/openclaw` | The agent loop in `packages/agent-core`, the tool-call repair package, the Signal extension, the cron service, the prompt cache boundary, the browser extension |
| Hermes | `~/Code/hermes-agent` | The context compressor, the memory tool, the gateway reliability files, the sudo prompt in the CLI, the Signal platform |
| Prime | `~/Code/prime-agent` | The cron jobs file, the goals file, the harness state and refinement code, the provider types |
| OpenCode | `~/Code/opencode` | The permission engine, the tool registry and its plain-text descriptions, the command registry in the terminal client, the session loop |
| ZeroClaw | `~/Code/zeroclaw` | The Signal channel, the cron store and scheduler, the tool-call parser crate, the updater, the Landlock wrapper, the skill creator |
| Moltis, Codex, Atomic, browser-use | The research scratch folder, cloned read-only | Moltis's browser manager, Codex's sandbox and tool specifications, browser-use's page representation and action set |

---

## Sources

The operations order and its five paragraphs are described in [FM 5-0, The Operations Process](https://armypubs.army.mil/epubs/DR_pubs/DR_a/ARN35403-FM_5-0-000-WEB-1.pdf). Fragmentary orders are covered in [Operation, Warning, and Fragmentary Orders](https://www.globalsecurity.org/military/library/policy/army/accp/in0541/ch1.htm). Commander's intent is explained in the [Marine Corps Gazette](https://www.mca-marines.org/gazette/commanders-intent-defined/), and critical information requirements are explained at [The Fivecoat Consulting Group](https://www.thefivecoatconsultinggroup.com/tfcg/ccir). Boyd's decision loop, and why orientation is its center, is covered in [Chet Richards, Boyd's OODA Loop](https://ooda.de/media/chet_richards_-_boyds_ooda_loop.pdf). The four questions of the after-action review are described by [Nick Milton](http://www.nickmilton.com/2009/10/after-action-review-4-questions.html). The harness comparison and its seventeen source studies are in `HARNESS_V2.md` and `docs/research/`.
