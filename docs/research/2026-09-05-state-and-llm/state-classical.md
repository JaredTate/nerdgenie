# How computer science has kept state for seventy years, and what Nerd Genie should borrow

This report surveys the classical ideas about state: the small amount of information about the past that a system needs in order to act correctly next. For each field it says what the idea is in plain words, what problem it solved, and what it teaches an agent whose memory must fit in a record of about three thousand tokens. The design already borrows event sourcing, process control blocks, paging, save files, the Army order, the OODA loop, and the after-action review; this report goes deeper and adds the mechanisms it does not yet name.

Two terms are used throughout. A **log** is a list that only grows: new lines go on the end and old lines are never changed. A **snapshot** is a copy of the whole current state at one moment. Nerd Genie's event log is the log, and its task record is the snapshot.

## 1. Top ideas, ranked by likely impact

### 1. Write the log position into the record, so recovery is "load the record, replay the tail"

**What it is.** A Raft snapshot carries the index of the last log entry it covers, so a server knows which entries to discard and which to replay. ARIES and SQLite checkpoints record the same thing, and everything before it can be truncated.
**Why it matters.** A record without a log position is a photograph with no date: after a crash you cannot tell which events it already contains, so you replay everything or apply one twice.
**How Nerd Genie would use it.** One header field: the id of the last event the record reflects. Recovery is "load the newest checkpoint, replay events after that id." `/tasks 17 back 3` is "load checkpoint N-3."
**Risk or cost.** Replay must never re-run a tool with side effects. Fowler's warning: "if these events cause update messages to be sent to external systems, then things will go wrong." Replay is a read of the log, never a re-execution.
**Source.** Raft paper, section 7; ARIES; SQLite WAL documentation; Fowler, Event Sourcing.

### 2. On resume, re-verify the Situation instead of trusting it

**What it is.** CRIU, the Linux tool that freezes a process to disk and restores it later, keeps a page called "What can change after C/R": connections, timers, and process ids may not survive. ARIES's first recovery pass, analysis, rebuilds what was in flight before trusting any page.
**Why it matters.** The Situation lists a browser tab, changed files, and a running command. All three are outside state that can vanish while a task waits three days. "Tab t1 is on x.com/compose" may be a lie on resume.
**How Nerd Genie would use it.** On any resume from a checkpoint, before the first model call, the harness re-checks every Situation line it can (tab present, file exists, process alive) and rewrites it with what it found.
**Risk or cost.** A few hundred milliseconds. A tab that is gone must become a failure line with a cause, not a silent rewrite.
**Source.** CRIU project page and limitations pages; ARIES analysis pass.

### 3. Fingerprint every result's inputs, so "never run the same call twice" becomes "never run it twice unless the inputs changed"

**What it is.** Build Systems à la Carte sorts build tools by how they decide what is stale: a dirty bit, a verifying trace (a hash of the inputs), or a constructive trace (the hash plus the stored output). Bazel's action cache maps a hash of an action's inputs to its outputs. Early cutoff means a step that re-runs but produces the same output triggers nothing downstream.
**Why it matters.** The guard blocks an identical call to stop loops, but re-reading a file after an edit is right and re-reading it unchanged is waste; a content hash tells them apart.
**How Nerd Genie would use it.** Each result line stores a fingerprint: tool, arguments, and for reads a hash of what was read. A repeat with an unchanged fingerprint is answered in one line, "unchanged since r3," for ten tokens instead of two thousand.
**Risk or cost.** Only pure tools qualify: read, search, web fetch, browser_read. Shell and browser actions depend on the world, not just their arguments, and must never be short-circuited.
**Source.** Mokhov, Mitchell, Peyton Jones, Build Systems à la Carte; Bazel action cache; Acar, self-adjusting computation.

### 4. Give each prompt layer a durability level and a version number, and test that a volatile write never bumps a durable version

**What it is.** Salsa, the incremental engine inside rust-analyzer, marks inputs durable (the standard library) or volatile (the file being typed in), with one version counter per level. Editing a volatile input bumps only the volatile counter, so every durable query is skipped in one comparison.
**Why it matters.** The prompt order already puts stable layers first for the cache; Salsa turns the habit into an invariant with a number on it.
**How Nerd Genie would use it.** Five layers, five counters: persona, tools, job summary, record goal and rules, record work and lessons. The `task` tool may only bump the last. A test asserts the first four are unchanged after forty rounds, and the cost line names the counter that moved when the cache hit rate drops.
**Risk or cost.** A few integers and a test. The danger is building a query framework where a counter will do.
**Source.** rust-analyzer blog, Durable Incrementality; Salsa documentation.

### 5. Store results and checkpoints by content hash, so checkpoints form a chain and diffs are free

**What it is.** Git names every file by the hash of its contents, every folder by a tree of hashes, and every commit by a tree plus a parent pointer; unchanged files are shared. Merkle's hash tree proves a whole structure unchanged by one hash at the top. Persistent data structures (Okasaki, Clojure) do the same in memory: a change copies only the path to the changed part.
**Why it matters.** One hash column buys three things: a page fetched twice is stored once; a checkpoint is a list of section hashes plus a parent, so "back 3" is a pointer move; and the diff between checkpoints is the sections whose hashes differ.
**How Nerd Genie would use it.** Each record section gets a hash. A checkpoint is seven hashes, the parent id, and the last event id from idea 1. `/tasks 17` can say "since checkpoint 12: plan changed, one failure added."
**Risk or cost.** Honest note: 200 copies of a 3,000-token record is under a megabyte, so storage is not the point; verification and diffing are. A hash per section is enough; do not build a persistent trie.
**Source.** Pro Git, Git Objects; Merkle 1987; Okasaki 1996; hyPiRion on Clojure's persistent vector.

### 6. Make the harness an explicit supervision tree with a restart limit at every level

**What it is.** In Erlang/OTP, workers do the job and supervisors watch them. A worker that meets an error it cannot handle crashes; the supervisor restarts it. Each supervisor has a restart intensity, "MaxR restarts in MaxT seconds," beyond which it gives up and its parent decides. Armstrong's rule: "Try to perform a task. If you cannot perform the task, then try to perform a simpler task."
**Why it matters.** Nerd Genie has the pieces: three provider retries, a crash breaker, a job that pauses after three failed tasks. One tree with one rule per level removes the special cases and gives each limit a config line and a test.
**How Nerd Genie would use it.** Four levels: tool call, turn, task, job, each with its own limit. Restart strategy too: when plan step 5 fails, later steps that depend on it are re-planned (OTP's rest_for_one) while steps 1 to 4 keep their result ids. "Try something simpler" is the instruction after a step fails twice: split it.
**Risk or cost.** A model is not deterministic, so a restart is not a replay; it must read the failure line and its cause or it repeats the crash. That is why failures live in the record, the supervisor's memory.
**Source.** Armstrong 2003 thesis, sections 4.3.2, 4.4, 5.2; Erlang/OTP Supervisor Behaviour.

### 7. Model the task's life as a statechart with history states

**What it is.** Harel's statecharts extend the plain state machine (a fixed list of states and allowed moves) with hierarchy, orthogonal regions running side by side, and broadcast. A history state means "when you come back into this group, return to the sub-state you left."
**Why it matters.** A task's states are described in prose: running, waiting for the user, waiting for approval, paused by a mid-turn message, stopped, done, failed. A statechart makes them a table, and the history state is "pick up from the last checkpoint."
**How Nerd Genie would use it.** One diagram in the design, one table in the code, one test per transition. Orthogonal regions name what runs beside the plan: the browser session, a long shell command, the budget clock.
**Risk or cost.** Over-formalizing; the value is a checklist of transitions, not a state-machine library.
**Source.** Harel 1987, Statecharts.

### 8. Measure the record in chunks, not tokens, and keep only the killer items

**What it is.** Miller found immediate memory holds about seven chunks and "seems to be almost independent of the number of bits per chunk." Cowan puts the true limit at three to five when rehearsal and recoding are blocked. Gawande's checklist rule: five to nine items, only the "killer items" that are dangerous to skip and still get skipped, checked at a fixed pause point.
**Why it matters.** The caps (five done lines, ten plan steps, a short stop list) are chunk limits already. The stop list is a killer-item list, and a stop line that never fires is noise that lowers compliance for the ones that matter.
**How Nerd Genie would use it.** Cap each section in lines, not tokens, and put the four chunks that matter most (the ask, the why, the current step, the next step) in the last lines the model reads before it writes. The done-check is a DO-CONFIRM checklist: work from memory, then stop and confirm each line against a result. Skill steps are READ-DO.
**Risk or cost.** Small models may need more scaffolding, not less. The cap is on count, not clarity.
**Source.** Miller 1956; Cowan 2001; Gawande, The Checklist Manifesto; Haynes et al. 2009.

### 9. Add one line of pre-authorized initiative: "if the plan breaks, do this"

**What it is.** Army doctrine defines commander's intent as purpose, key tasks, and end state, and expects "disciplined initiative" inside it when orders no longer fit. Richards' reading of Boyd adds that a shared orientation lets "actions flow from orientation without the need for explicit decisions," which Boyd called implicit guidance and control.
**Why it matters.** The "why" line is the intent, and skills that run without a model call are implicit guidance and control. Missing is the middle: when a step fails and no skill fits, what may the model do before it asks?
**How Nerd Genie would use it.** One line under Goal, written at planning time and visible to the user: "If the plan breaks: try one different approach, then ask." The harness enforces the count, and the after-action review compares intent with what the initiative did.
**Risk or cost.** Initiative never overrides the ask-me-first list. Intent sits below permissions.
**Source.** ADP 6-0, Mission Command; FM 5-0; Richards, Boyd's OODA Loop.

### 10. Every tool call carries an expectation, and the harness scores the miss

**What it is.** In a blackboard system, independent specialists tell a separate controller what they would add and at what cost before doing the work: "If I am executed, I'll generate contributions of this type, with these qualities, while expending these resources." Rete, the matcher inside rule systems, re-checks rules only against what changed.
**Why it matters.** The browser tools already carry an expectation. Generalizing it turns every result into a checked prediction, the cheapest failure detector there is: a miss is a failure line with a cause, written by the harness for free.
**How Nerd Genie would use it.** An optional `expect` field on every tool, one line. The harness checks it with plain rules (file exists, exit code zero, title contains a word) and writes "expected X, got Y" as a failure on a miss.
**Risk or cost.** About ten tokens per call; a small model may write vague expectations, so an empty field is allowed.
**Source.** Nii 1986; Corkill 1991; Forgy 1982.

### 11. Name the three storage tiers and the horizon below which nothing is rewritten

**What it is.** An LSM tree keeps new writes in memory, flushes them to sorted files, and merges downward in levels, never updating in place. MVCC keeps old row versions until no reader can see them; the oldest active reader sets the horizon below which a vacuum may clean.
**Why it matters.** Nerd Genie already has three tiers (table, record line, shelf) and never rewrites above the cache line. LSM and MVCC give the vocabulary and the guarantee: movement is downward and append-only, and the horizon is the cache line.
**How Nerd Genie would use it.** A test that every result moves table to record to shelf and never back except through `read`, and one "compaction" event in the log when the oldest half of the table is dropped, so the drop is visible and replayable.
**Risk or cost.** RocksDB's leveled compaction pays write amplification of ten to thirty times. Nerd Genie's tiers are pointers, not copies, so the price is nil, which is the reason never to rewrite a tier.
**Source.** O'Neil et al. 1996; RocksDB compaction wiki; PostgreSQL MVCC and vacuum.

## 2. Findings: the survey

### The stored program and the program counter

In 1945 von Neumann's "First Draft of a Report on the EDVAC" described a machine that keeps its instructions in the same memory as its data. The whole state of a running program then reduces to memory plus a handful of registers, and one of those, the program counter, is a single number saying which instruction comes next. An operating system pauses a program by saving those registers in a process control block and resumes by loading them back; OSTEP describes the block as holding "register contents, program counter, stack pointer, and other state needed to run the program." The lesson: a resumable thing needs a small fixed record with one field that says "where I am." The plan with its check marks is the program counter; the record is the process control block.

### Finite state machines and statecharts

A finite state machine is a list of states and the allowed moves between them. Harel's 1987 statecharts added hierarchy, orthogonal regions, and broadcast, plus the history state that returns you to the sub-state you left. Flat state diagrams explode in size for real systems. The lesson: a task has a small set of states, the history state is a checkpoint, and drawing it makes every transition testable.

### Transactions, write-ahead logs, ARIES, checkpoints, and truncation

A transaction is a group of changes that all happen or none do. The write-ahead rule says the log line describing a change must be on disk before the change itself. ARIES (1992) made this industrial: every log line has a sequence number, every page remembers the last line applied to it, and recovery runs three passes: analysis (what was in flight), redo ("repeat history," even for work about to be undone), then undo. A fuzzy checkpoint records which transactions and pages were dirty without stopping the world, and everything before it can be truncated. SQLite's WAL mode is the same shape in one file: changes append to a WAL, a checkpoint copies them back into the database, by default at 1,000 pages, and the docs state the trade-off plainly: frequent checkpoints favor reads, rare ones favor writes. The Young/Daly formula for how often a long computation should checkpoint grows with the checkpoint's cost; Nerd Genie's cost is microseconds, so it says what the design does: checkpoint on every change.

### Event sourcing, CQRS, and the save-game problem

Fowler's event sourcing: "capture all changes to an application state as a sequence of events," so the state is "purely derivable from the event log." It buys a full rebuild, a query at any past time, and replay after a fix. Its cost is the save-game problem: rebuilding by replaying every event is slow, so systems keep snapshots and replay only the events after the latest one. Fowler's Memory Image pattern, used by LMAX and Prevayler, is exactly "loading the latest snapshot and replaying any events since that snapshot," and Greg Young's CQRS Documents call them rolling snapshots. CQRS is the companion idea: separate the model that takes writes from the views that serve reads. The lesson: the record is a snapshot and a read view at once, and the log stays the truth.

### Replicated state machines: Paxos, Raft, and log compaction

Schneider's 1990 tutorial: if every copy of a service is a deterministic state machine and all copies apply the same inputs in the same order, they stay identical. Paxos (Lamport, 2001) and Raft (2014) are the protocols that agree on that order. Raft's section 7 faces the growing log directly: "Snapshotting is the simplest approach to compaction. In snapshotting, the entire current system state is written to a snapshot on stable storage, then the entire log up to that point is discarded." Each server snapshots independently; the snapshot carries the last included index and term and the cluster configuration; and "Incremental approaches to compaction, such as log cleaning and log-structured merge trees, are also possible." The lesson: the snapshot's metadata (which log entry it covers) is what makes it safe, and a slow follower, like a model that has not seen the task for days, is brought up to date with the snapshot, never the whole log.

### Checkpoint/restart in HPC and process migration

Supercomputer jobs run for days on hardware that fails, so they write restart dumps; DMTCP does it from user space, capturing threads, memory, and open files in about two seconds across 128 cores. CRIU freezes a Linux process, writes its memory, file descriptors, sockets, and timers to disk, and restores it "exactly as it was during the time of the freeze," even on another machine; it is also honest that it "cannot (yet) save and restore every single bit of tasks' state" and documents what changes after a restore and what cannot be checkpointed. The lesson: outside state (connections, other processes, a browser) is the part that does not survive, so the record must list it and the harness must re-verify it on resume.

### Erlang/OTP: supervisors, workers, and let it crash

Armstrong's 2003 thesis separates the two roles: "One process, the worker process, does the job. Another process, the supervisor process, observes the worker. If an error occurs in the worker, the supervisor takes actions to correct the error." The worker does not handle errors it does not understand; it crashes, and that is "let it crash." Supervisors form trees, and OTP's supervisor has restart strategies (one_for_one, one_for_all, rest_for_one), and a restart intensity that stops infinite loops. The state that matters lives in the supervisor and the messages, not the worker. The lesson: the harness is the supervisor and the model call is the worker; the record is what the supervisor knows; a failing worker restarts with the failure written down, not with the whole history.

### Persistent data structures, Merkle trees, and git

Okasaki's 1996 thesis showed immutable data structures can be as fast as mutable ones. Clojure's persistent vector is a 32-way tree; an update copies only the path from the root to the changed leaf ("we copy all nodes on the path down to the value we're about to update"), so old and new versions share everything else. Merkle's 1987 hash tree names each node by the hash of its children, so one hash at the top vouches for the whole structure. Git combines both: blobs, trees, and commits, all named by content hash, each commit pointing at its parent. The lesson: identity by content makes duplicates free, versions cheap, and tampering visible; a chain of checkpoints is a git history.

### Incremental computation and memoization

Memoization is remembering a function's answer for given inputs. Acar's self-adjusting computation (2005) tracks which parts of a program read which inputs so a change re-runs only what depends on it. Build Systems à la Carte (2018) sorts build tools on two axes, the scheduler (what order) and the rebuilder (what is stale), and names the rebuilders: dirty bits (Make), verifying traces (Shake), and constructive traces (Bazel's action cache). Salsa adds durability levels with one version counter each, so that "changing my local file can't affect the standard library" is a single comparison. The lesson: record what each result depended on, and stale checking becomes cheap and correct; group state by how often it changes and version each group.

### Databases: MVCC, LSM trees, compaction, materialized views

MVCC (multi-version concurrency control) gives every transaction a consistent view by keeping old row versions; in PostgreSQL each row carries the transaction that made it and the one that killed it, and a vacuum removes versions no reader can still see, bounded by the oldest reader's horizon. The LSM tree (O'Neil et al., 1996) takes writes in a memory component and merges them into sorted disk components, turning random writes into sequential ones; RocksDB's leveled compaction is the modern form, and its price is write amplification of ten to thirty times. A materialized view is a stored query result that Gupta and Mumick (1995) show how to maintain from the changes alone. The lesson: never update in place; move data downward through tiers; keep the record as an incrementally maintained view of the log.

### Blackboard architectures and production systems

Nii's 1986 survey traces blackboard systems to Hearsay-II, a 1970s speech understander. Corkill's metaphor: specialists around a blackboard, each adding a contribution when it sees enough information, with a separate control component that picks the next contribution from estimates of quality and cost. Production systems are rule sets; Forgy's 1982 Rete algorithm made them fast by matching rules only against changes to working memory. The lesson: shared state on one board, contributors that declare what they expect to add, a controller that is not an expert, and condition checks driven by what changed.

### OODA, operations orders, commander's intent, checklists, kanban

Boyd's OODA loop (observe, orient, decide, act) is often sold as speed, but Richards, who worked with Boyd, says "orientation is key," and that with a shared orientation "actions can actually flow from orientation without the need for explicit decisions," a link Boyd called implicit guidance and control. ADP 6-0 defines commander's intent as purpose, key tasks, and end state, and FM 5-0's five-paragraph order fixes the shape; Nerd Genie borrows both. Gawande's checklists work at pause points, hold five to nine killer items, and come in two kinds, DO-CONFIRM and READ-DO; the 19-item surgical checklist cut deaths from 1.5% to 0.8% and complications from 11% to 7% across eight hospitals. Ohno's kanban is state on a card: the card is the order, the pull is the signal, and nothing moves without it. The lesson across all four: fix the format, keep the request and the intent, list the alarms in advance, check at pause points, and let a small card carry the state.

### Working memory limits as a design constraint

Miller (1956): the span of immediate memory is about seven items and "seems to be almost independent of the number of bits per chunk"; the way past the limit is recoding, "building larger and larger chunks." Baddeley and Hitch (1974) split working memory into a central executive, a phonological loop, and a visuospatial sketchpad, later adding an episodic buffer. Cowan (2001) argued the true limit is "only three to five chunks," "averaging about four," once rehearsal and chunking are blocked. The lesson: a record is read by something with a small span, human or model; count chunks, not tokens; make each line one chunk; keep the four that matter closest to the point of decision.

## 3. Sources

- von Neumann, First Draft of a Report on the EDVAC (1945): https://cs.carleton.edu/faculty/jondich/courses/cs208_s19/assignments/16_files/edvac.pdf ; background: https://en.wikipedia.org/wiki/First_Draft_of_a_Report_on_the_EDVAC
- Arpaci-Dusseau, OSTEP, chapter 4, The Abstraction: The Process: https://pages.cs.wisc.edu/~remzi/OSTEP/cpu-intro.pdf
- Harel, Statecharts: A Visual Formalism for Complex Systems, Science of Computer Programming 8(3), 1987: https://www.sciencedirect.com/science/article/pii/0167642387900359
- Mohan et al., ARIES, ACM TODS 17(1), 1992: https://web.stanford.edu/class/cs345d-01/rl/aries.pdf ; summary: https://blog.acolyer.org/2016/01/08/aries/
- SQLite, Write-Ahead Logging: https://www.sqlite.org/wal.html
- Young/Daly checkpoint interval, overview: https://dl.acm.org/doi/10.1145/3549206.3549328
- Fowler, Event Sourcing: https://martinfowler.com/eaaDev/EventSourcing.html ; Memory Image: https://martinfowler.com/bliki/MemoryImage.html
- Young, CQRS Documents: https://cqrs.wordpress.com/wp-content/uploads/2010/11/cqrs_documents.pdf
- Schneider, Implementing Fault-Tolerant Services Using the State Machine Approach, ACM Computing Surveys 22(4), 1990: https://www.cs.cornell.edu/fbs/publications/SMSurvey.pdf
- Lamport, Paxos Made Simple (2001): https://lamport.azurewebsites.net/pubs/paxos-simple.pdf
- Ongaro and Ousterhout, In Search of an Understandable Consensus Algorithm (extended), section 7: https://raft.github.io/raft.pdf ; Ongaro, Consensus: Bridging Theory and Practice (dissertation, chapter 5): https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf
- CRIU: https://criu.org/Main_Page ; https://github.com/checkpoint-restore/criu
- DMTCP: https://github.com/dmtcp/dmtcp ; paper: https://ieeexplore.ieee.org/document/5161063/
- Armstrong, Making reliable distributed systems in the presence of software errors (2003): https://erlang.org/download/armstrong_thesis_2003.pdf
- Erlang/OTP, Supervisor Behaviour: https://www.erlang.org/doc/system/sup_princ.html
- Okasaki, Purely Functional Data Structures (1996): https://www.cs.cmu.edu/~rwh/students/okasaki.pdf
- hyPiRion, Understanding Clojure's Persistent Vectors: https://hypirion.com/musings/understanding-persistent-vector-pt-1
- Merkle, A Digital Signature Based on a Conventional Encryption Function, CRYPTO '87: https://link.springer.com/chapter/10.1007/3-540-48184-2_32
- Pro Git, Git Internals: Git Objects: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
- Acar, Self-Adjusting Computation (2005): https://www.researchgate.net/publication/35457276_Self-adjusting_computation
- Mokhov, Mitchell, Peyton Jones, Build Systems à la Carte, ICFP 2018: https://www.microsoft.com/en-us/research/wp-content/uploads/2018/03/build-systems.pdf ; Mitchell's notes: http://neilmitchell.blogspot.com/2018/07/inside-paper-build-systems-la-carte.html
- rust-analyzer, Durable Incrementality: https://rust-analyzer.github.io/blog/2023/07/24/durable-incrementality.html ; Salsa: https://github.com/salsa-rs/salsa
- Bazel, Remote Caching and glossary: https://bazel.build/remote/caching ; https://bazel.build/reference/glossary
- PostgreSQL MVCC and vacuum (Postgres Professional): https://postgrespro.com/blog/pgsql/5967918
- O'Neil et al., The Log-Structured Merge-Tree, Acta Informatica 33(4), 1996: https://www.cs.umb.edu/~poneil/lsmtree.pdf
- RocksDB, Leveled Compaction: https://github.com/facebook/rocksdb/wiki/Leveled-Compaction
- Gupta and Mumick, Maintenance of Materialized Views (1995): https://link.springer.com/chapter/10.1007/bfb0022063
- Nii, Blackboard Systems, AI Magazine 7(2) and 7(3), 1986: https://ojs.aaai.org/aimagazine/index.php/aimagazine/article/view/537
- Corkill, Blackboard Systems, AI Expert 6(9), 1991: https://mas.cs.umass.edu/Documents/Corkill/ai-expert.pdf
- Forgy, Rete, Artificial Intelligence 19, 1982: https://www.sciencedirect.com/science/article/abs/pii/0004370282900200
- Richards, Boyd's OODA Loop: https://ooda.de/media/chet_richards_-_boyds_ooda_loop.pdf
- ADP 6-0, Mission Command (2019): https://armypubs.army.mil/epubs/DR_pubs/DR_a/ARN34403-ADP_6-0-000-WEB-3.pdf ; FM 5-0: https://armypubs.army.mil/epubs/DR_pubs/DR_a/ARN35403-FM_5-0-000-WEB-1.pdf
- Gawande, The Checklist Manifesto (2009), notes: https://sive.rs/book/ChecklistManifesto
- Haynes et al., A Surgical Safety Checklist, NEJM 360(5), 2009: https://www.nejm.org/doi/abs/10.1056/NEJMsa0810119
- Ohno, Toyota Production System; kanban and supermarket: https://artoflean.com/reference/supermarket/ ; https://artoflean.com/reference/taiichi-ohno/
- Miller, The Magical Number Seven, Psychological Review 63, 1956: https://labs.la.utexas.edu/gilden/files/2016/04/MagicNumberSeven-Miller1956.pdf
- Baddeley's model of working memory: https://en.wikipedia.org/wiki/Baddeley%27s_model_of_working_memory
- Cowan, The magical number 4 in short-term memory, Behavioral and Brain Sciences 24(1), 2001: https://www.cambridge.org/core/services/aop-cambridge-core/content/view/44023F1147D4A1D44BDC0AD226838496/S0140525X01003922a.pdf/the-magical-number-4-in-short-term-memory-a-reconsideration-of-mental-storage-capacity.pdf
