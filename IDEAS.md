# Ideas for Nerd Genie

Future ideas for the harness, none of them built. The owner approves an idea before it is built; an approved idea is built test first and lands in `docs/PROGRESS.md` with its measured effect. Every idea here has to hold on any task, a book as much as a build, a marketing agent on CLI tools as much as a game, and on the local model that runs it, Qwen 3.8 on one card.

The numbers come from `docs/research/2026-09-06-behaviour/`: the model's behaviour on 6 September 2026 across three logs, 32 tasks, 2,173 rounds and 685 minutes of model time. A round costs about 3 seconds fixed, 2.4 seconds per thousand uncached prompt tokens and 17 seconds per thousand written tokens; prompt reading is 58 percent of all model time, and more than half of that re-read the prompt for no reason the prompt's growth explains.

## The five put forward on 6 September 2026

### 1. A cache-shaped prompt for the whole run

Three changes that keep the daemon's cache warm. The front of the prompt becomes byte-stable across every task of a run: the job summary and the pins move below the cache line, so a new task re-reads about two thousand tokens instead of twelve. Every side call the harness makes (the done check, the job's planning and review, the memory pass) goes to a second daemon slot so it cannot evict the task's cache. A picture rides as the last message and is dropped without rewriting any earlier result.

- Evidence: 25 rounds lost the whole cache with the prompt unchanged and no harness event between (829k tokens); 25 of 32 task starts re-read the 10 to 14 thousand token front (351k); 18 cache fall-backs coincided with a picture leaving the window (410k).
- Gain: about 1.6 million tokens, roughly 60 of 685 minutes, and faster first rounds on every task.
- Risk: MTP's speed gain shrinks with two slots (measured gone at four); a second slot's checkpoints cost about 150 MB each.
- Measure: the cache-events table per run; losses with the prompt unchanged should go to zero and task-start uncached tokens under three thousand.
- Cost: two to three days.

### 2. Cut, do not wipe

When the guard fires, the harness cuts only the messages since the last progress signal and keeps everything before them byte for byte, instead of clearing the conversation. When the window hits its cap it opens a fresh window the way a pick-up does, instead of dropping the oldest half. A fresh window carries the last six results in full, not two.

- Evidence: all twelve rewinds and seventeen stop-and-continue restarts were followed by re-orientation: 121 rounds, 820k tokens, 33 minutes; the model read back 2.6 results by id after a restart and 1.6 after a rewind; five half-drops cost 105k.
- Gain: about 30 minutes a day of runs, and the model keeps the thread of the rounds that were working.
- Risk: the loop's cause may sit before the cut; the ladder's stop still covers that.
- Measure: uncached tokens and reads by id in the three rounds after each rewind and restart.
- Cost: one to two days.

### 3. The harness keeps the books

An optional `expect` line on shell, write and edit, checked by four plain rules: a test count, an exit code, a contained string, parses. A miss becomes a failure line with its cause, written by the harness; a hit marks the step it proves. The task tool's required fields become required in its schema. The tail ends with one harness line, "Step 3 of 5, next: ...", because the last thing the model reads is what it answers, which the plan regression of 6 September proved.

- Evidence: 297 rounds (13.7 percent, 98 minutes) did nothing but write the record; 53 of 345 record writes were refused for a missing field; 47 of 165 test rounds were the model rerunning tests the harness had just run.
- Gain: if half the bookkeeping rounds fold into working rounds, about 150 rounds and 50 minutes, and every failure in the record carries a real cause.
- Risk: vague expectations are ignored and said so once; the model may parrot the tail line.
- Measure: record-only rounds and refused record writes per task, already columns in the nightly table.
- Cost: two days.

### 4. Long-lived processes as first-class citizens

The shell tool gets `serve`: start a process detached and answer at once with the port it opened, read from the kernel's socket table the orientation block already reads. A `check` answers whether a port answers and how fast. An identical command within a few rounds whose output hash has not changed is answered "same as r12" by the harness without running it.

- Evidence: 131 poll-only rounds (526k tokens, 28 minutes) were all polls of servers that never finish; 84 rounds repeated an identical command (511k, 33 minutes), one health probe 26 times in one task.
- Gain: about 60 minutes a day of runs on tasks with a server; a GPU render or a training job has the same shape.
- Risk: a process that fails after opening its port; the check catches it.
- Measure: poll-only rounds and identical-command rounds per task.
- Cost: one to two days.

### 5. The landing checklist, read by an independent co-pilot

Before "done", the harness confirms what it can itself: files parse, the last test run was green and came after the last edit, no plan step is open, no stop line was reported, and for a page, something changed after an action and no page errors. It labels every done line `[v]` when the harness computed the proof and `[o]` when only the model judged it. Then one call with tools off, denied the transcript and given only the artifacts and the done lines, answers as the person would: yes, or which line and what is missing.

- Evidence: three of the last eight nightly runs closed "done" on a job that did not work; the web sources measured that a verifier shown the generating model's history rescues a third as much as one that is not.
- Gain: reliability, not tokens; one round per task, whole nights recovered.
- Risk: the same weights judging; the labels and the harness-computed items keep it from being the only judge; rejections capped at two per gate.
- Measure: `[o]` lines per task and tasks sent back, against the nightly check column.
- Cost: two to three days.

## Smaller things the same numbers point at

- Nudge at five rounds without progress instead of ten. Recovery followed the nudge within one to seven rounds every time; about 15 minutes a day.
- Cap the Failures list as the results list is capped at forty lines. Two tasks died at the record's size rule with seventeen failure lines.
- Log the same-call guard's refusals and the nudge as events. Neither is visible in the log today, so neither can be counted.
- Take a screenshot only when the model asks for one. One task took eighteen pictures for seven resizes.
- Show six results in full on a fresh window, not two (part of idea 2, cheap on its own).

## The wider list, from the two research reports

Candidates not chosen for the five, kept here with where they come from. `docs/research/2026-09-06-behaviour/ideas-from-the-documents.md` and `ideas-from-the-web.md` hold the full arguments.

- Corrections ride in the tail until the next fresh window, so a steer keeps the cache warm. The logs disagree on the cost: thirteen mid-turn corrections kept the cache, one steer through a handoff did not. Measure first.
- Harness-written gists on every result line: the names in a file read, the first failing test, the exit code and first and last lines of a command. One task read the same file forty-eight times.
- A line-anchored edit that takes a range instead of the old span, so the model writes only the new text. Written tokens are most of a round now.
- Prompt-lookup drafting on the daemon (n-gram beside MTP) for the copied text in edits; judged by acceptance, not tokens per second.
- Batched masking of old tool results at a fixed cadence, on a checkpoint boundary, instead of one at a time. Two independent studies found masking matches summaries at half the cost.
- Runaway detection on the token stream: a repeated-tail check that stops a looping generation within seconds instead of at the 8,192-token cap. Two rounds hit the cap today, 6.3 minutes.
- Task-conditioned pruning of tool output with a small second model. Second-order on one card; the rules-engine half is idea 3's cousin.
- A trigger-scheduled memory pass with a small bank of facts and traps, fired on a tool error, a repeated command or a failed verification, never on a fixed cadence.
- Large inputs as a variable: a long log or spec goes to disk and is sliced by sub-calls, never pasted into the transcript.

## Rejected before, and why

`STATE_IMPROVE_INNOVATE_PLAN.md`, "What was rejected, and why", holds the list: compacting the conversation (measured worse than truncation), a smaller window as a build, multi-agent splits, tool masking by phase, a grammar on the call span, thinking at chosen moments (the owner's decision on 6 September: the model works fine as it is), state-root hashes and pure reducers, an intent line and a scored memory, fuzzy plan-step matching, job tasks inheriting failures.
