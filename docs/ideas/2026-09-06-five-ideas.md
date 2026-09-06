# Five ideas for the harness, 6 September 2026, for the owner's approval

Written from three reports made the same afternoon (`docs/research/2026-09-06-behaviour/`): the model's measured behaviour across the day's three logs (32 tasks, 2,173 rounds, 685 minutes of model time, 85 million prompt tokens of which 88 percent were cached), the repository's own research documents, and forty-five sources from the web. None of these is built. Each holds on any task, a book as much as a build.

The measured shape of a round: about 3 seconds fixed, 2.4 seconds per thousand uncached prompt tokens, 17 seconds per thousand written tokens. Prompt processing is 58 percent of all model time, and more than half of it (54 percent of uncached tokens, 215 minutes) was re-reading the prompt for no reason the prompt's growth explains.

## 1. A cache-shaped prompt for the whole run

**What.** Three changes that keep the daemon's cache warm. The front of the prompt becomes byte-stable across every task of a run: the job summary and the pins move below the cache line, so a new task re-reads two thousand tokens instead of twelve. Every side call the harness makes (the done check, the job's planning and review, the memory pass) goes to a second daemon slot, so it cannot evict the task's cache. And a picture rides as the last message and is dropped without rewriting any earlier result.

**Evidence.** 25 rounds lost the whole cache with the prompt unchanged and no harness event between (829k tokens); 25 of 32 task starts re-read the 10 to 14 thousand token front (351k); 18 cache fall-backs coincided with a picture leaving the window and rewriting an older result (410k).

**Gain.** About 1.6 million tokens, roughly 60 of 685 minutes, and every task's first rounds faster. **Risk.** MTP's speed gain shrinks with two slots (measured gone at four); the second slot's checkpoints cost about 150 MB each. **Measure.** The cache-events table per run: losses with the prompt unchanged should go to zero and task-start uncached tokens under three thousand. **Cost.** Two to three days.

## 2. Cut, do not wipe

**What.** When the guard fires, the harness cuts only the messages since the last progress signal and keeps everything before them byte for byte, instead of clearing the conversation. When the window hits its cap it opens a fresh window the way a pick-up does instead of dropping the oldest half. A fresh window carries the last six results in full, not two.

**Evidence.** All twelve rewinds and seventeen stop-and-continue restarts were followed by re-orientation: 121 rounds, 820k tokens, 33 minutes, with the model reading back 2.6 results by id after a restart and 1.6 after a rewind. Five half-drops cost 105k.

**Gain.** Around 30 minutes per day of runs and, more important, the model keeps the thread of the rounds that were working. **Risk.** The loop's cause may sit before the cut; the ladder's stop still covers that. **Measure.** Uncached tokens and reads by id in the three rounds after each rewind and restart. **Cost.** One to two days.

## 3. The harness keeps the books

**What.** An optional `expect` line on shell, write and edit, checked by four plain rules (a test count, an exit code, a contained string, parses). A miss becomes a failure line with its cause written by the harness; a hit marks the step it proves. The task tool's required fields become required in its schema. And the tail ends with one harness line: "Step 3 of 5, next: ...", because the last thing the model reads is what it answers, which today's plan regression proved.

**Evidence.** 297 rounds (13.7 percent, 98 minutes) did nothing but write the record; 53 of 345 record writes were refused for a missing field; 47 of 165 test rounds were the model rerunning tests the harness had just run.

**Gain.** If half the bookkeeping rounds fold into working rounds, about 150 rounds and 50 minutes, and every failure in the record carries a real cause. **Risk.** Vague expectations are ignored and said so once; the model may parrot the tail line. **Measure.** Record-only rounds and refused record writes per task, already columns in the nightly table. **Cost.** Two days.

## 4. Long-lived processes as first-class citizens

**What.** The shell tool gets `serve`: start a process detached and answer at once with the port it opened, read from the kernel's socket table the orientation block already reads. A `check` answers whether a port answers and how fast. And an identical command within a few rounds whose output hash has not changed is answered "same as r12" by the harness without running it.

**Evidence.** 131 poll-only rounds (526k tokens, 28 minutes) were all polls of servers that never finish; 84 rounds repeated an identical command (511k, 33 minutes), one health probe 26 times in one task.

**Gain.** About 60 minutes per day of runs on tasks with a server, and the same shape covers a GPU render or a training job. **Risk.** A process that fails after opening its port; the check catches it. **Measure.** Poll-only rounds and identical-command rounds per task. **Cost.** One to two days.

## 5. The landing checklist, read by an independent co-pilot

**What.** Before "done", the harness confirms what it can itself (files parse, the last test run was green and came after the last edit, no plan step open, no stop line reported, and for a page: something changed after an action and no page errors) and labels every done line `[v]` when the harness computed the proof and `[o]` when only the model judged it. Then one call with tools off, denied the transcript, given only the artifacts and the done lines, answers as the person would: yes, or which line and what is missing.

**Evidence.** Three of the last eight nightly runs closed "done" on a job that did not work; the web sources measured that a verifier which sees the generating model's history rescues a third as much as one that does not.

**Gain.** Reliability, not tokens: one round per task, and whole nights recovered. **Risk.** The same weights judging; the labels and the harness-computed items keep it from being the only judge, and rejections are capped at two per gate. **Measure.** `[o]` lines per task and tasks sent back, against the nightly check column. **Cost.** Two to three days.

## Smaller things the numbers also point at

Nudge at five rounds without progress instead of ten (recovery followed the nudge within one to seven rounds every time; about 15 minutes a day). Cap the Failures list as the results list is capped (two tasks died at the record's size rule). Log the same-call guard's refusals and the nudge as events (they are invisible today). Take a screenshot only when asked (18 pictures for 7 resizes in one task).
