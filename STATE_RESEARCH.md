# How computers keep state, and what Nerd Genie should borrow

Written 2026-09-05 for the Nerd Genie team, from the three reports in `docs/research/2026-09-05-state-and-llm/`. It is in plain English, and every technical word is explained the first time it appears. It does not repeat what Nerd Genie already builds; each idea says what is there and what is missing.

## The short version

1. State is the small amount of what happened that a program must keep so it can act correctly next. Not everything that happened. Only what matters now.
2. Every other AI agent keeps its state as the whole conversation, and the model re-reads all of it on every call. That is a video game that loads a saved game by replaying every button you ever pressed.
3. When the conversation no longer fits in the model's window, the most text it can read at once, the agent writes a summary and throws the rest away. The saved game is now a guess about which button presses mattered.
4. Nerd Genie already has the right shape: a small task record of what is true now, beside a log that only grows and is never edited, with a fresh working context built for each call.
5. Tonight showed that the shape is not enough. The record must pass to the next task instead of starting blank, be checked against the world instead of trusting a tool, and carry proof a person can read. Computer science has solved all three, many times, since 1945.

Three things went wrong tonight. A browser tool said a click worked when it had not (ideas 4 and 5). The task stopped, and the user's next message opened a new task with an empty record, so the model did not know it had just built a Tetris game with 62 passing tests (ideas 1, 2, 7, and 8). And the user could not tell from the report what was on disk (idea 6).

## The ideas, ranked by impact

A few words used throughout. The **harness** is the program wrapped around the model: it decides what the model reads, and it runs the **tools**, such as reading a file or clicking a button, that the model asks for. A **log** is a list that only grows; an **event** is one line in it. A **checkpoint** is a saved copy of the whole state at one moment, and **replay** means feeding the log back through the same program to rebuild that state. A **token** is a piece of text about the size of a short word.

### 1. A follow-up is never a blank page

**What it is.** Temporal is a workflow engine, a program that runs long jobs and survives crashes. It will not let one run's history grow forever: when it gets too long, the run closes and a new run starts at once under the same id, with "the latest relevant state" passed in as input. Nothing is summarized; the state is handed over.

**Where it comes from.** Distributed systems: Temporal's Continue-As-New and Durable Functions' "eternal orchestrations".

**What Nerd Genie would do.** A message that arrives after a task has stopped, failed, or filled its record opens a successor task, never a fresh one. The successor inherits the ask, the corrections, the stop list, the lessons, the open done lines, the situation, and one line: "continues task 17". The size cap gets the same mechanism: close task 17 with a report, open task 18 in the same job. Empty the message queue into the new record first, or a message is lost.

**The risk.** Carry too much and the successor is a transcript by another name; carry only the stable parts and the open items. A simple rule tells a follow-up from a new subject, and one line tells the model which it got.

### 2. The save point knows where it is in the log

**What it is.** Replaying a whole log to rebuild state is slow: the save-game problem. Every system with a log keeps a checkpoint and replays only the events after it. What makes that safe is that the checkpoint records the position of the last event it contains; Raft's snapshot carries "the last included index", and everything before it can be dropped.

**Where it comes from.** Databases (ARIES, 1992; SQLite's write-ahead log), event sourcing (Fowler's Memory Image), consensus systems (Raft, section 7; PBFT's stable checkpoints).

**What Nerd Genie would do.** Add one header field to every checkpoint: the id of the last event it reflects. Recovery becomes "load the newest checkpoint, replay the events after that id". Bitcoin's assumeutxo and Ethereum's checkpoint sync add a second step: trust the checkpoint, answer the user at once, and verify it against the log in the background by comparing fingerprints (idea 3). A mismatch stops the task and tells the user.

**The risk.** Replay is a read of the log, never a re-run of a tool; Fowler warns that replaying events into outside systems means "things will go wrong".

### 3. One short fingerprint that commits to everything

**What it is.** A hash is a fingerprint: a short string computed from some data, such that changing one letter of the data changes the whole fingerprint. Ethereum keeps its whole world state in a tree of hashes, and the one hash at the root "can be used as a secure identity for the entire system state". Git names every file and saved version by the hash of its content, and each version points at its parent.

**Where it comes from.** Blockchains (Ethereum yellow paper, section 4.1), version control (Git internals), cryptography (Merkle's 1987 hash tree).

**What Nerd Genie would do.** Give each record section a hash. A checkpoint becomes the section hashes, the parent checkpoint's hash, and the last event id from idea 2, hashed together, with the short form in the header. That buys a one-line check that a replay rebuilt the same record, a free diff between checkpoints ("since c17.39: plan changed, one failure added"), tamper evidence, and content addressing for results, so the same call with the same arguments returns the old result id.

**The risk.** The canonical form, one fixed way of writing the record out byte for byte, must be settled first. A hash per section is enough; do not build a tree store.

### 4. Measure the world; never trust a report of it

**What it is.** CRIU, a Linux tool that freezes a running program to disk and restores it later, keeps an honest page about what can change after a restore: connections, timers, and other programs may not survive. The 2026 agent research says the same from the other side: "false success", where a tool or a model says done when the world says otherwise, was 45 to 76 percent of failures, and model judges could not spot it.

**Where it comes from.** Process checkpointing (CRIU), database recovery (ARIES), agent reliability research (Advani 2026; Mansoor, Phadke, and Rana 2026).

**What Nerd Genie would do.** Two rules. On any resume, before the first model call, the harness re-checks every situation line it can (tab present, file exists, tests still pass) and rewrites the situation; a tab that is gone becomes a failure line with a cause. After every tool call that changes the world, the harness computes the difference itself: page changed or not, files changed, test count before and after. A click with no visible change returns a failure and a screenshot, never "ok".

**The risk.** Some effects are invisible, such as a network side effect. The browser worker already takes the snapshot, so the cost is small. Tonight's lying browser tool is exactly this failure.

### 5. The blackboard: every tool call says what it expects

**What it is.** A blackboard system, from the Hearsay-II speech program of the 1970s, puts independent specialists around one shared board. Before a specialist does any work, it tells a separate controller what it would add and at what cost. The controller is not an expert; it only compares promises with results.

**Where it comes from.** Artificial intelligence (Nii 1986; Corkill 1991; Forgy's Rete, 1982, which re-checks rules only against what changed).

**What Nerd Genie would do.** Idea 4 measures what happened; this idea says what the measurement should have shown. Add an optional `expect` field to every tool, one line: "the piece moved left", "exit code 0", "tests 62 to 63". The harness checks it with plain rules and, on a miss, writes "expected X, got Y" as a failure line, for free. Tonight the lie was caught on the fourth re-read of a file; with an expectation it is caught on the first click.

**The risk.** About ten tokens per call. A small model may write vague expectations, so an empty field is allowed and checks nothing.

### 6. Hand the person proofs, not a transcript

**What it is.** A Bitcoin light client is a phone that keeps only block headers, 80 bytes each, and checks a payment by asking for "the Merkle branch", a handful of hashes linking it to a header it holds. Rollups, systems that do work off the main chain and post results back, come in two kinds: optimistic ones accept a result unless someone challenges it, then re-run only the disputed step; validity ones attach a proof checked at once.

**Where it comes from.** Blockchains (Bitcoin whitepaper, section 8; EIP-1186; ethereum.org on rollups).

**What Nerd Genie would do.** The user is a light client. Every report line carries its proof id: "posted, 236 characters (r14); under 280, checked by the harness (r6)". Label each done line `[v]` when the harness checked it and `[o]` when the model judged it. The user challenges an `[o]` line by id, and the harness re-runs the one check behind it, never the task. Tonight the user would have read "game at ~/tetris, 62 of 62 tests [v] r38, playable in a browser [o] r40: click reported ok, page unchanged" and known what existed.

**The risk.** Browser results cannot be re-run, only re-shown. Keep ids at the ends of lines, where they read least like noise.

### 7. The harness is a supervisor, and every level has a restart limit

**What it is.** In Erlang, a language built for telephone switches, a worker does the job and a supervisor watches it. A worker that meets an error it cannot handle crashes, and the supervisor restarts it: "let it crash". Supervisors form a tree, each with a restart limit, "at most N restarts in T seconds", past which its own parent decides. Armstrong's rule: "If you cannot perform the task, then try to perform a simpler task."

**Where it comes from.** Fault-tolerant systems (Armstrong's 2003 thesis; the Erlang/OTP supervisor).

**What Nerd Genie would do.** Nerd Genie has the pieces: three provider retries, a crash breaker, a job that pauses after three failed tasks. Make them one tree with four levels, tool call, turn, task, and job, each with one limit and one test. When plan step 5 fails, steps 1 to 4 keep their result ids and only later steps are re-planned. After a step fails twice, the simpler task is to split it. A stalled task is a crashed worker: the supervisor restarts it with the failure line in the record, not a blank page.

**The risk.** A model is not deterministic (the same input does not always give the same output), so a restart is not a replay: the restarted worker must read the failure and its cause or it repeats the crash.

### 8. Memory across tasks: raw text, the last task first, and traps

**What it is.** CoALA, the standard map of agent memory, names four kinds: working (this call), episodic (what happened), semantic (facts), and procedural (how to do things). A controlled 2025 study found that raw conversation text beat model-extracted facts by 16 to 22 points, because of "lossy distillation, not structure per se". Generative Agents scored memories by recency, relevance, and importance. ReasoningBank stored lessons from failures.

**Where it comes from.** Agent memory research (Sumers 2023; An 2025; Zhang 2025; Park 2023; Ouyang 2025).

**What Nerd Genie would do.** Nerd Genie already captures facts from the log for free and puts a three-line memory hint on every call. Three changes. Every fact in MEMORY.md keeps a pointer to its raw log event, and search returns the raw text. The report of the most recent finished or stopped task is always the first hint line, so even a truly new task sees "task 17 stopped ten minutes ago: Tetris at ~/tetris, 62 of 62 tests, browser click unreliable". And the review may save a trap, such as tonight's lying click, found later by tool and site and dropped when a later task marks it wrong.

**The risk.** A 2026 study found a free-form scratchpad in every prompt "never helps" on long tasks. Keep memory structured, small, and measured. Bad traps poison future tasks; keep them few and dated.

### 9. One pure function applies every change

**What it is.** Redux, a state library for web apps, allows one way to change state: emit an action, and let a reducer turn the old state plus the action into the new state. A reducer is a pure function, one with no side effects: same inputs, same output, nothing else touched. So actions "can be logged, serialized, stored, and later replayed", and if the reducer changes, every past action is re-evaluated.

**Where it comes from.** Web state management (Redux), streaming (Kafka's log compaction), rollups (zkSync's state diffs), workflow engines (Temporal).

**What Nerd Genie would do.** The record already has stable keys: r7, D1, F1, C1, plan step 3, done line 2. Store every `task` tool call in the log as a pair, slot and new value, and define the record as "the last value of every slot", computed by one function `apply(record, event)` that reads no clock and no file. Then the record can never drift from the log, a withdrawn step is a tombstone (a marker meaning "removed") in the log, and a rule change is tested by replaying every stored log against every checkpoint.

**The risk.** The situation lines must come from logged events, not live checks. Small models will write to the wrong slot sometimes, so the tool must name the slot back.

### 10. Do not redo what has not changed

**What it is.** Memoization is remembering a function's answer for given inputs so it need not be computed again. Build tools decide what is stale by a fingerprint of the inputs, or the fingerprint plus the stored output (Bazel). Salsa, inside the Rust language server, marks inputs durable or volatile, with one version number per level, so an edit to a volatile input skips every durable query in one comparison.

**Where it comes from.** Incremental computation (Build Systems à la Carte, 2018; Acar 2005; rust-analyzer's Durable Incrementality).

**What Nerd Genie would do.** The repeat guard blocks an identical call; fingerprints make it precise. Each result line stores the tool, the arguments, and for reads a hash of what was read. A repeat with an unchanged fingerprint is answered in one line, "unchanged since r3", for ten tokens instead of two thousand; after an edit the read runs. Tonight's four reads of one file become one read and three "unchanged" lines, a stuck signal the harness can count. Second, give the five prompt layers one version number each; the `task` tool may bump only the last, and a test asserts the first four never move in forty rounds.

**The risk.** Only pure tools qualify: read, search, web fetch, browser_read. Shell and browser actions depend on the world and must never be short-circuited.

### 11. Checkpoints are a tree, a rewind is a branch, and the files get a checkpoint too

**What it is.** Git keeps every saved version of a folder; a save is a commit, and a branch is only a name for one. Saving again from an old commit makes a fork; nothing is erased. Anthropic's harness for long-running agents commits the files after every green test run. A 2026 study found most code-agent failures came after the agent reached the right code and then thrashed it; a commit at the green point recovered every such case.

**Where it comes from.** Version control (Git); long-running agent harnesses (Anthropic 2025; Kim et al. 2026; Dubey 2026).

**What Nerd Genie would do.** `/tasks 17 back 3` makes checkpoint c17.43 with parent c17.39 and leaves c17.40 to c17.42 in the log, so the review can compare the two paths. Pair it with a file checkpoint: a git commit after every green test run, named for the record checkpoint. Then the stuck ladder has a real rewind: drop the looping results from the window, keep one failure line, revert the files to the last green commit, call once more; if the loop returns, stop. Tonight's 62-green state would be a named commit the successor task could point at.

**The risk.** The side panel must show which branch is live. Allow one rewind per stuck event, or the rewind is itself a loop.

### 12. The checklist: killer items only, at a pause point

**What it is.** Gawande's checklists hold five to nine "killer items", the ones dangerous to skip and still skipped, read at a fixed pause point. Two kinds: DO-CONFIRM (work, then stop and confirm each item) and READ-DO (read a step, do it). The nineteen-item surgical checklist cut deaths from 1.5 to 0.8 percent. In 2025, scoring a checklist pulled from the instructions beat reward models on every benchmark tested.

**Where it comes from.** Medicine and aviation (Gawande; Haynes et al. 2009); alignment research (Viswanathan et al. 2025).

**What Nerd Genie would do.** The done list and the stop list are checklists already. The done check is DO-CONFIRM at the pause before "done"; skill steps are READ-DO. A stop line that never fires across many tasks is noise, so the review asks whether to drop it. And split the proof classes: a line about what the user sees ("the piece moves left") needs an end-to-end result, a checked browser action or a screenshot, and the `task` tool refuses to mark it done with a unit test id. A unit test is a small automatic check of one piece of code; sixty-two of them proved sixty-two lines tonight, and none was "the game plays".

**The risk.** End-to-end checks are slow and sometimes flaky. Cap the retries and fall back to a hand-off screenshot.

### 13. Commander's intent: one line of permitted initiative

**What it is.** Army doctrine defines commander's intent as purpose, key tasks, and end state, and expects "disciplined initiative" inside it when the orders no longer fit. Boyd's OODA loop (observe, orient, decide, act) is usually sold as speed, but Richards, who worked with Boyd, says that with a shared orientation "actions can actually flow from orientation without the need for explicit decisions".

**Where it comes from.** Military doctrine (ADP 6-0; FM 5-0) and Boyd, through Richards.

**What Nerd Genie would do.** The why line is the intent, and skills that run without a model call are Boyd's implicit guidance. Missing is the middle: when a step fails and no skill fits, what may the model do before it asks? One line under Goal, written at planning time and visible to the user: "If the plan breaks: try one different approach, then ask." The harness counts the attempts. Tonight that line would have read "if the click tool fails, take a screenshot and hand the window to the user".

**The risk.** Initiative never overrides the ask-me-first list. Intent sits below permissions.

### 14. Seven things, and the four that matter last

**What it is.** Miller in 1956 found that immediate memory holds about seven chunks, "almost independent of the number of bits per chunk"; Cowan in 2001 put the true limit at three to five. Models have their own version: "Lost in the Middle" showed they read the start and the end of a context best. Manus, a production agent, rewrites a short to-do list at the end of the context on every step.

**Where it comes from.** Psychology (Miller; Cowan); language model research (Liu et al. 2023); practice (Manus).

**What Nerd Genie would do.** The caps of five done lines and ten plan steps are chunk limits already. Measure every section in lines, not tokens, and make each line one chunk. Then add one harness-written final line to every prompt: "Step 3 of 5: post it. Last result r9: page unchanged. Next: confirm the compose box." The four chunks that matter most, the ask, the why, the current step, and the next step, are the last thing the model reads before it writes. About twenty tokens, and it never touches the cached front.

**The risk.** Small models may need more scaffolding, not less; the cap is on count, not clarity.

### 15. Number every state, and refuse a stale write

**What it is.** A payment channel lets two parties trade signed updates privately and touch the blockchain only to open, close, or dispute. Each update carries a sequence number, "newer states trump older ones", and Lightning punishes anyone who presents an old state.

**Where it comes from.** Blockchains (Lightning paper, section 3; Castro's PBFT thesis, whose replicas work only above the last confirmed checkpoint).

**What Nerd Genie would do.** Every prompt carries the checkpoint number it was built from, and the harness stamps every `task` tool call with it. A write built on an older checkpoint than the current one, because a correction landed mid-turn, is refused, and the model gets the fresh record. `/tasks 17 back 3` moves only between checkpoints the background replay of idea 2 has confirmed.

**The risk.** None new. One integer per call, filled by the harness.

## The survey

One short section per field, oldest first, each ending with the lesson for an agent.

### The stored program and the program counter

In 1945 von Neumann described a machine that keeps its instructions in the same memory as its data. The whole state of a running program then comes down to memory plus a handful of registers, one of which, the program counter, says which instruction comes next. An operating system pauses a program by saving those registers into a small process control block, and resumes it days later by loading them back. Lesson: anything resumable needs a small fixed record with one field that says "where I am". The plan with its check marks is the program counter; the record is the control block.

### State machines and statecharts

A finite state machine is a list of the states a thing can be in and the allowed moves between them. Harel's statecharts of 1987 added states inside states, regions that run side by side, and a history state: "when you come back into this group, return to the sub-state you left". A Nerd Genie task has a few states, running, waiting, paused, stopped, done, failed, and today they live in prose. Lesson: put the states in a table, write one test per move, and make the history state "pick up from the last checkpoint". Tonight the move out of "stopped" went to "new task" instead of to the history state.

### Databases: the write-ahead log, checkpoints, and never writing in place

A transaction is a group of changes that all happen or none do. The write-ahead rule says the log line describing a change must be on disk before the change itself. ARIES (1992) made this industrial: every log line has a sequence number, recovery runs analysis, redo, then undo, and a checkpoint records where recovery may start. SQLite's WAL mode is the same shape in one file. MVCC keeps old row versions until no reader needs them; the LSM tree merges writes downward through levels, never updating in place. Lesson: log first, checkpoint with a position, move data downward through tiers, and never rewrite a tier.

### Event sourcing and the save-game problem

Fowler's rule: "capture all changes to an application state as a sequence of events", so the state is "purely derivable from the event log". It buys a full rebuild, a query at any past moment, and replay after a bug fix. Its cost is the save-game problem: replaying every event is slow, so systems keep snapshots and replay only the events after the latest one. Microsoft adds that snapshots "are an optimization, not a replacement for the eventstream", and that events should record intent, not resulting state. Lesson: the log is the truth, the record is a cached view, and the view must say which event it is current to.

### Replicated state machines: Paxos, Raft, and PBFT

Schneider's 1990 tutorial: if every copy of a service is a deterministic state machine and all copies apply the same inputs in the same order, they stay identical. Paxos and Raft agree on the order. Raft faces the growing log directly: "the entire current system state is written to a snapshot on stable storage, then the entire log up to that point is discarded". A follower that fell behind gets the snapshot, never the whole log. PBFT handles copies that lie: a checkpoint is stable once enough copies confirm the same fingerprint. Lesson: a model that has not seen a task for days is a slow follower. Send it the snapshot, and truncate only below a confirmed checkpoint.

### Freezing a process to disk

Supercomputer jobs run for days on hardware that fails, so they write restart dumps; DMTCP does this in about two seconds across 128 cores. CRIU freezes a Linux process, writes its memory, open files, sockets, and timers to disk, and restores it "exactly as it was", even on another machine. It is also honest that it "cannot (yet) save and restore every single bit" and lists what changes after a restore. Lesson: what does not survive a freeze is the outside world: connections, other programs, a browser tab. The record must list that state, and the harness must re-check it on resume.

### Erlang: supervisors and let it crash

Armstrong's 2003 thesis separates two roles: "One process, the worker process, does the job. Another process, the supervisor process, observes the worker. If an error occurs in the worker, the supervisor takes actions to correct the error." The worker does not handle errors it does not understand; it crashes. Supervisors form trees, each with a restart strategy and a limit that stops infinite loops. Lesson: the harness is the supervisor, the model call is the worker, and the record is what the supervisor knows. A failing worker restarts with the failure written down, not with the whole history.

### Git, hash trees, and persistent data structures

Okasaki showed in 1996 that data structures which never change in place can be as fast as ones that do; Clojure's vector copies only the path to the changed leaf, so old and new versions share the rest. Merkle's 1987 tree names each node by the hash of its children, so one hash at the top vouches for everything below. Git combines both: files, folders, and commits are named by content hash, and each commit points at its parent. Lesson: name things by their content, chain checkpoints to their parents, and a rewind becomes a branch instead of an erasure.

### Build systems and incremental computation

Memoization remembers an answer for given inputs. Acar's self-adjusting computation tracks which parts of a program read which inputs, so a change re-runs only what depends on it. Build Systems à la Carte sorts build tools by how they tell what is stale: dirty bits, fingerprints of inputs, or fingerprints plus stored outputs. Salsa adds durability levels with one version counter each, so "my local edit cannot affect the standard library" is a single comparison. Lesson: record what each result depended on, and stale checking becomes cheap and correct. Group state by how often it changes, and version each group.

### Blackboards and rule systems

Nii's 1986 survey traces blackboard systems to Hearsay-II. Corkill's picture is specialists around a blackboard, each adding a contribution when it sees enough, with a separate controller that picks the next one from estimates of quality and cost. Production systems are sets of if-then rules; Forgy's Rete algorithm made them fast by matching rules only against what changed. Lesson: one shared board, contributors who declare what they expect to add, a controller that is not an expert, and checks driven by what changed. The record is the board, the model the specialist, the harness the controller.

### The Army order, Boyd's loop, checklists, and kanban

The five-paragraph operations order fixes a shape so anyone can write one and anyone can check one, even when the plan has fallen apart. Commander's intent is purpose, key tasks, and end state, and it lets people act correctly when the orders stop fitting. Boyd's OODA loop, read through Richards, is about orientation more than speed. Gawande's checklists work at pause points and hold five to nine killer items. Ohno's kanban is state on a card: nothing moves without the card. Lesson: fix the format, keep the request in the asker's words, write down why, list the alarms in advance, check at pause points, and let a small card carry the state.

### Working memory

Miller in 1956: the span of immediate memory is about seven items, "almost independent of the number of bits per chunk", and the way past it is recoding into larger chunks. Baddeley and Hitch in 1974 split working memory into an executive, a loop for sound, and a sketchpad for images. Cowan in 2001 argued the true limit is "three to five chunks" once rehearsal is blocked. Lesson: a record is read by something with a small span, person or model. Count chunks, not tokens; make each line one chunk; keep the four that matter closest to the point of decision.

### Bitcoin, Ethereum, and DigiByte: small state, one root, fast sync

Bitcoin's state is the set of unspent coins, each created once and spent once; the whitepaper already planned to discard old spent history. Ethereum switched to accounts with a counter and storage, because multi-stage work needs slots that change, and commits to the whole state with one 32-byte root hash in every block header. Nobody serious replays from the first block any more: Geth's full sync took eight to ten days, so clients start from a checkpoint that "all nodes on the network agree" on and verify in the background. DigiByte, built on Bitcoin Core, scaled by tightening the loop, fifteen-second blocks and five independent mining algorithms, not by changing the state model. Lesson: keep state as facts added once and never edited; commit to all of it with one fingerprint; start from a trusted small state and verify later; spread trust across independent checkers.

### Rollups, channels, rent, and merge-safe data

Optimistic rollups accept a result unless someone submits a fraud proof within about seven days, then re-run only the disputed step; validity rollups attach a proof checked at once; zkSync posts only the slots that changed. Payment channels number every update and punish anyone who presents a stale one. Solana charges rent, a deposit in proportion to an account's size, so bytes have a price. A CRDT is a data type whose copies can be edited separately and always merge to the same result; the record's corrections, results, decisions, and failures are grow-only sets of that kind. Lesson: publish the diff, not the transcript; accept claims with a challenge path; number every state; put a price on bytes; choose data whose merge is obvious.

### Workflow engines and Redux: deterministic replay and continue-as-new

Kreps' 2013 essay states the principle: two identical deterministic programs given the same inputs in the same order end in the same state, so the log "squeezes all the non-determinism out of the input stream". Temporal and Durable Functions rebuild a workflow by re-running its code against its history and fail when the code stops matching. When the history grows too long they do not summarize; they roll over into a new run with the state passed as input. An engineering note on chat agents built on Temporal reaches Nerd Genie's own conclusion: keep the transcript outside the history. Lesson: the log holds intent, one pure function applies it, the checkpoint is a cache, and a task that outgrows its record gets a successor, not a squeeze.

### Agent memory research

CoALA gives the vocabulary: working, episodic, semantic, and procedural memory, which map onto Nerd Genie's working context, log, memory files, and skills. MemGPT treats the model as an operating system paging between a fixed main context and archival storage. Generative Agents score memories by recency, relevance, and importance. Voyager and Anthropic's Agent Skills store how-to knowledge found by description. The honest summary: structured, itemized memory backed by raw text has the best evidence; rewriting a context over and over collapses it; a free-form scratchpad in every prompt did not help on long tasks; and stored experiences can spread their own errors. Lesson: never summarize in place, keep a pointer from every fact to the raw event, put the most recent task's report first, and measure whether memory helps.

## Sources

Grouped by the report each came from. Every link is copied from that report.

### From the classical survey (state-classical.md)

- von Neumann, EDVAC report (1945): https://cs.carleton.edu/faculty/jondich/courses/cs208_s19/assignments/16_files/edvac.pdf
- OSTEP, chapter 4, The Process: https://pages.cs.wisc.edu/~remzi/OSTEP/cpu-intro.pdf
- Harel, Statecharts (1987): https://www.sciencedirect.com/science/article/pii/0167642387900359
- Mohan et al., ARIES (1992): https://web.stanford.edu/class/cs345d-01/rl/aries.pdf
- SQLite, Write-Ahead Logging: https://www.sqlite.org/wal.html
- Fowler, Event Sourcing: https://martinfowler.com/eaaDev/EventSourcing.html
- Fowler, Memory Image: https://martinfowler.com/bliki/MemoryImage.html
- Young, CQRS Documents: https://cqrs.wordpress.com/wp-content/uploads/2010/11/cqrs_documents.pdf
- Schneider, The State Machine Approach (1990): https://www.cs.cornell.edu/fbs/publications/SMSurvey.pdf
- Lamport, Paxos Made Simple (2001): https://lamport.azurewebsites.net/pubs/paxos-simple.pdf
- Ongaro and Ousterhout, Raft, section 7: https://raft.github.io/raft.pdf
- CRIU: https://criu.org/Main_Page
- DMTCP: https://github.com/dmtcp/dmtcp
- Armstrong, thesis (2003): https://erlang.org/download/armstrong_thesis_2003.pdf
- Erlang/OTP, Supervisor Behaviour: https://www.erlang.org/doc/system/sup_princ.html
- Okasaki, Purely Functional Data Structures (1996): https://www.cs.cmu.edu/~rwh/students/okasaki.pdf
- Merkle, hash trees (1987): https://link.springer.com/chapter/10.1007/3-540-48184-2_32
- Pro Git, Git Objects: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
- Acar, Self-Adjusting Computation (2005): https://www.researchgate.net/publication/35457276_Self-adjusting_computation
- Mokhov, Mitchell, Peyton Jones, Build Systems à la Carte (2018): https://www.microsoft.com/en-us/research/wp-content/uploads/2018/03/build-systems.pdf
- rust-analyzer, Durable Incrementality: https://rust-analyzer.github.io/blog/2023/07/24/durable-incrementality.html
- PostgreSQL MVCC and vacuum: https://postgrespro.com/blog/pgsql/5967918
- O'Neil et al., The Log-Structured Merge-Tree (1996): https://www.cs.umb.edu/~poneil/lsmtree.pdf
- Nii, Blackboard Systems (1986): https://ojs.aaai.org/aimagazine/index.php/aimagazine/article/view/537
- Corkill, Blackboard Systems (1991): https://mas.cs.umass.edu/Documents/Corkill/ai-expert.pdf
- Forgy, Rete (1982): https://www.sciencedirect.com/science/article/abs/pii/0004370282900200
- Richards, Boyd's OODA Loop: https://ooda.de/media/chet_richards_-_boyds_ooda_loop.pdf
- ADP 6-0, Mission Command (2019): https://armypubs.army.mil/epubs/DR_pubs/DR_a/ARN34403-ADP_6-0-000-WEB-3.pdf
- FM 5-0: https://armypubs.army.mil/epubs/DR_pubs/DR_a/ARN35403-FM_5-0-000-WEB-1.pdf
- Gawande, The Checklist Manifesto, notes: https://sive.rs/book/ChecklistManifesto
- Haynes et al., A Surgical Safety Checklist (2009): https://www.nejm.org/doi/abs/10.1056/NEJMsa0810119
- Ohno, kanban and the supermarket: https://artoflean.com/reference/supermarket/
- Miller, The Magical Number Seven (1956): https://labs.la.utexas.edu/gilden/files/2016/04/MagicNumberSeven-Miller1956.pdf
- Baddeley's model of working memory: https://en.wikipedia.org/wiki/Baddeley%27s_model_of_working_memory
- Cowan, The magical number 4 (2001): https://www.cambridge.org/core/services/aop-cambridge-core/content/view/44023F1147D4A1D44BDC0AD226838496/S0140525X01003922a.pdf/the-magical-number-4-in-short-term-memory-a-reconsideration-of-mental-storage-capacity.pdf

### From the blockchain and distributed systems survey (state-distributed.md)

- Bitcoin whitepaper, sections 7 and 8: https://bitcoin.org/bitcoin.pdf
- Ethereum yellow paper, section 4.1: https://ethereum.github.io/yellowpaper/paper.pdf
- EIP-1186, eth_getProof: https://eips.ethereum.org/EIPS/eip-1186
- Optimistic rollups: https://ethereum.org/en/developers/docs/scaling/optimistic-rollups/
- ZK rollups: https://ethereum.org/en/developers/docs/scaling/zk-rollups/
- zkSync state diffs: https://docs.zksync.io/zksync-protocol/rollup/data-availability
- Poon and Dryja, Lightning Network paper, section 3: https://lightning.network/lightning-network-paper.pdf
- Geth v1.10.0 release post: https://blog.ethereum.org/2021/03/03/geth-v1-10-0
- Weak subjectivity: https://ethereum.org/en/developers/docs/consensus-mechanisms/pos/weak-subjectivity/
- Bitcoin Optech, AssumeUTXO: https://bitcoinops.org/en/topics/assumeutxo/
- DigiByte: https://www.digibyte.org/en-us/
- Solana accounts and rent: https://solana.com/docs/core/accounts
- CRDTs: https://crdt.tech/
- Castro, PBFT thesis, sections 2.3.4 and 5.3: https://www.microsoft.com/en-us/research/wp-content/uploads/2017/01/thesis-mcastro.pdf
- Kafka design, Log Compaction: https://kafka.apache.org/43/design/design/
- Kreps, The Log (2013): https://engineering.linkedin.com/distributed-systems/log-what-every-software-engineer-should-know-about-real-time-datas-unifying
- Redux, three principles: https://redux.js.org/understanding/thinking-in-redux/three-principles
- Temporal, Continue-As-New: https://docs.temporal.io/workflow-execution/continue-as-new
- Temporal, deterministic constraints: https://docs.temporal.io/workflow-definition
- Ably, conversation history in a Temporal AI agent: https://ably.com/temporal/managing-conversation-history-in-a-temporal-ai-agent-chat
- Durable Functions, eternal orchestrations: https://learn.microsoft.com/en-us/azure/azure-functions/durable/durable-functions-eternal-orchestrations
- Azure Architecture Center, Event Sourcing pattern: https://learn.microsoft.com/en-us/azure/architecture/patterns/event-sourcing

### From the agent harness survey (agent-harnesses.md)

- Advani, Characterizing False Success in LLM Agents (2026): https://arxiv.org/abs/2606.09863
- Mansoor, Phadke, Rana, Verified Tool Calls (2026): https://arxiv.org/abs/2608.02645
- Dubey, Real-Time Detection and Repair of LLM Agent Failures (2026): https://arxiv.org/abs/2608.02464
- Kim et al., Coherence Collapse (2026): https://arxiv.org/abs/2603.24631
- Anthropic, Effective harnesses for long-running agents (2025): https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents
- Ji, Context Engineering for AI Agents, Manus (2025): https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- Liu et al., Lost in the Middle (2023): https://arxiv.org/abs/2307.03172
- An, Verbatim Chunks Beat Extracted Artifacts (2025): https://arxiv.org/abs/2601.00821
- Zhang et al., Agentic Context Engineering (2025): https://arxiv.org/abs/2510.04618
- Khanal, Tao, Zhou, Beyond pass@1 (2026): https://arxiv.org/abs/2603.29231
- Ouyang et al., ReasoningBank (2025): https://arxiv.org/abs/2509.25140
- Xiong et al., How Memory Management Impacts LLM Agents (2025): https://arxiv.org/abs/2505.16067
- Sumers et al., CoALA (2023): https://arxiv.org/abs/2309.02427
- Packer et al., MemGPT (2023): https://arxiv.org/abs/2310.08560
- Park et al., Generative Agents (2023): https://arxiv.org/abs/2304.03442
- Wang et al., Voyager (2023): https://arxiv.org/abs/2305.16291
- Anthropic, Agent Skills (2025): https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills
- Viswanathan et al., Checklists Are Better Than Reward Models (2025): https://arxiv.org/abs/2507.18624
