# How the local 27B behaves inside the Nerd Genie harness: three logs of 6 September 2026

Logs read (copies in this folder): `run5.db` (2026-09-05 23:57 to 2026-09-06 19:55 UTC, 25 tasks, jobs 2 and 3), `run6.db` (20:14 to 20:37 UTC, 3 tasks, job 2), `live.db` (20:48 to 21:17 UTC, 4 tasks, job 2, still running when copied). All three are the Tater Tots Tetris build. Read-only; nothing outside the scratchpad was touched. The tables here are drawn from `tables.md` beside this file, which `an.py` produces from the copies; `rounds.tsv` holds one line per round.

Definitions used throughout:

- A **round** is one model call: a checkpoint whose "this turn" line reports tokens. The checkpoint the harness writes when a task opens (0.0k tokens), and the one it writes when a task closes or is resumed (a copy of the last cost line with no calls), are not rounds. `scripts/runreport` counts those copies as rounds and adds their tokens again: for run5 task 11 it reports 322 rounds and 8,991.6k tokens in where the log holds 309 model calls and 8,576k (12 copied checkpoints carry 415.9k, 4.6%). Calls, refusals and record-only rounds agree exactly between runreport and this report.
- **Wall time of a round** is the gap from the previous checkpoint (or the person's message, when one arrived in between) to the round's checkpoint. Gaps over 15 minutes (the 8-hour wait before task 1 was resumed, two waits of 10-21 minutes before tasks 14 and 15) are idle time and are left out of every sum.
- **Uncached tokens** = tokens in minus the daemon's own `timings.cache_n`. **Marginal tokens** of a round = uncached in + out; this is what the daemon actually had to process.
- **Record size** is the checkpoint text after its two header lines, at 4 characters a token. The harness's own estimate (words x 1.3) is a little smaller.

Totals across the three logs: 32 tasks, 2,173 rounds, 685 minutes of model time, 85.2M tokens in (88% cached, 9.97M uncached), 615k tokens out, 2,349 tool calls.

---

## 1. Rounds, wall time and tokens

### 1.1 Per task

| log | task | end state | rounds | model min | s/round median | p90 | tokens in (k) | cached % | uncached/round median (k) | out (k) |
|---|---|---|---|---|---|---|---|---|---|---|
| run5 | 1 (the ask; ran the whole build itself before job 2 started, resumed 8 h later on a complaint) | done | 248 | 108.7 | 18 | 54 | 12,258 | 89 | 3.8 | 91.3 |
| run5 | 2 | done | 21 | 1.2 | 3 | 5 | 306 | 94 | 0.8 | 2.4 |
| run5 | 3 | done | 10 | 1.8 | 8 | 41 | 186 | 75 | 1.6 | 1.3 |
| run5 | 4 | done | 8 | 1.9 | 10 | 44 | 164 | 71 | 1.3 | 1.3 |
| run5 | 5 | done | 8 | 2.2 | 10 | 47 | 175 | 70 | 1.9 | 1.8 |
| run5 | 6 | done | 32 | 4.8 | 6 | 20 | 1,140 | 94 | 1.0 | 7.9 |
| run5 | 7 | done | 9 | 1.3 | 5 | 22 | 138 | 75 | 1.1 | 1.2 |
| run5 | 8 | done | 7 | 0.7 | 3 | 20 | 88 | 81 | 0.9 | 0.7 |
| run5 | 9 (Chrome play-test) | failed | 131 | 50.1 | 21 | 35 | 6,829 | 90 | 5.1 | 27.5 |
| run5 | 10 (Chrome play-test, again) | failed | 205 | 86.8 | 23 | 40 | 6,342 | 77 | 7.6 | 67.8 |
| run5 | 11 (Chrome play-test, third try) | done | 309 | 99.8 | 18 | 31 | 8,576 | 75 | 6.6 | 47.5 |
| run5 | 12 (visual QA) | done | 45 | 18.0 | 23 | 50 | 2,443 | 90 | 4.3 | 12.2 |
| run5 | 13 (final regression) | done | 61 | 19.4 | 20 | 28 | 2,416 | 86 | 5.4 | 9.6 |
| run5 | 14 | done | 2 | 2.3 | 72 | 72 | 16 | 43 | 4.5 | 0.1 |
| run5 | 15 | stopped | 3 | 0.4 | 6 | 15 | 36 | 78 | 0.5 | 0.3 |
| run5 | 16 | done | 5 | 1.0 | 15 | 16 | 75 | 72 | 4.7 | 2.4 |
| run5 | 17 | done | 14 | 1.4 | 5 | 16 | 197 | 86 | 0.8 | 1.7 |
| run5 | 18 | done | 36 | 8.5 | 10 | 18 | 1,202 | 94 | 1.6 | 19.6 |
| run5 | 19 | done | 51 | 15.5 | 12 | 28 | 2,480 | 95 | 1.6 | 28.1 |
| run5 | 20 (yeti hazard) | done | 237 | 55.7 | 11 | 21 | 10,612 | 94 | 2.1 | 79.5 |
| run5 | 21 | done | 23 | 5.5 | 8 | 24 | 924 | 89 | 1.2 | 4.5 |
| run5 | 22 | done | 50 | 14.3 | 11 | 22 | 2,182 | 92 | 1.5 | 20.0 |
| run5 | 23 (game shell) | done | 108 | 35.6 | 15 | 33 | 6,497 | 95 | 1.9 | 42.4 |
| run5 | 24 (browser play-test) | done | 236 | 55.0 | 10 | 20 | 8,857 | 91 | 1.9 | 33.1 |
| run5 | 25 (visual QA at sizes) | stopped | 111 | 47.6 | 12 | 63 | 4,974 | 87 | 2.2 | 24.3 |
| run6 | 1 | done | 7 | 1.4 | 8 | 30 | 99 | 67 | 1.7 | 2.7 |
| run6 | 2 | done | 16 | 1.7 | 4 | 24 | 247 | 85 | 0.8 | 1.9 |
| run6 | 3 (engine) | running | 73 | 17.2 | 11 | 25 | 2,237 | 91 | 1.4 | 30.8 |
| live | 1 | done | 6 | 1.5 | 15 | 31 | 87 | 63 | 1.4 | 2.8 |
| live | 2 | done | 22 | 2.7 | 5 | 16 | 408 | 92 | 0.8 | 5.8 |
| live | 3 (engine) | done | 38 | 8.7 | 10 | 16 | 1,234 | 95 | 1.5 | 20.9 |
| live | 4 (game shell) | running | 41 | 12.2 | 13 | 32 | 1,772 | 93 | 1.9 | 21.3 |
| **all** | | | **2,173** | **685** | **14** | **33** | **85,196** | **88** | **2.6** | **615** |

Two tasks ended "failed": task 10 on the harness's own record-size promise ("everything in it but the ask would be about 3004 tokens and the limit is 3000", with a Failures list of 17 lines), task 9 on "the program codex could not be started: context canceled". Task 1 failed once as well, at 01:25, on the same size rule (3,003 of 3,000, with 220 result lines), was resumed 13 hours later on the person's complaint, and ended done. End states of the 32 tasks: 26 done, 2 failed, 2 stopped by the person (15, 25), 2 still running.

### 1.2 Per job

| log | job | tasks | task rounds | job checkpoints (no cost line) | model min | tokens in (k) | cached % | uncached (k) | out (k) |
|---|---|---|---|---|---|---|---|---|---|
| run5 | the ask outside a job (1, 14, 15, 16) | 4 | 258 | 0 | 112 | 12,385 | 89 | 1,351 | 94 |
| run5 | job 2 (tasks 2-13) | 12 | 846 | 84 | 288 | 28,802 | 82 | 5,155 | 181 |
| run5 | job 3 (tasks 17-25) | 9 | 866 | 61 | 239 | 37,924 | 92 | 2,936 | 253 |
| run6 | job 2 (tasks 2-3) | 2 | 89 | 31 | 19 | 2,484 | 90 | 246 | 33 |
| live | job 2 (tasks 2-4) | 3 | 101 | 24 | 24 | 3,414 | 94 | 221 | 48 |

Between one task's last checkpoint and the next task's first event a job spends 5-21 seconds (23 gaps measured; the two long ones are the person's idle time). The job's own model calls (the "review-task-N" lines it writes into MEMORY.md after each task) leave no cost line in the log, so their tokens are not in any table here.

### 1.3 Wall time per round

| bucket | rounds | share | minutes |
|---|---|---|---|
| 0-5 s | 206 | 9% | 13 |
| 5-10 s | 531 | 24% | 67 |
| 10-20 s | 754 | 35% | 179 |
| 20-30 s | 410 | 19% | 167 |
| 30-60 s | 196 | 9% | 127 |
| 60-120 s | 57 | 3% | 81 |
| 120-300 s | 18 | 1% | 44 |
| over 300 s | 1 | 0% | 7 |

Median 14 s (run5 14, run6 9, live 10), mean 19 s, p90 33 s, longest 437 s (run5 task 25 round 47: 77k tokens re-read from nothing after a restart). The 76 rounds over 60 s are 3.5% of rounds and 19% of the time; they are big file writes (3-8k tokens out) and rounds that re-read a whole window.

A least-squares fit over all 2,173 rounds gives **wall s = 3.2 + 2.37 per 1k uncached tokens in + 17.1 per 1k tokens out** (residual sd 12 s), i.e. about 421 tokens/s of prompt processing and 59 tokens/s of generation. Applied to the totals: prompt processing of uncached tokens ~395 min (58% of model time), generation ~175 min (25%), fixed per-round cost ~116 min (17%). The single biggest lever is therefore the uncached token count.

### 1.4 What the cache does and what breaks it

The prompt is laid out with the instructions, persona, tools, job summary, recent work and the record's goal and rules above the cache line, then memory, the conversation, and after the conversation the record's work and lessons, its results list, the memory hint and the standing line. So each round legitimately re-reads its own tail: the record below the line, last round's reply, last round's results. Everything beyond that is cache loss.

Cache events (a round's first round, a drop of tokens in by 40%+, a cached count of 0 or a fall of 3k+ while the prompt grew):

| cause | rounds | uncached tokens in those rounds (k) |
|---|---|---|
| first round of a task (the system prefix of 10-14k is re-read; only 7 of 32 first rounds reused it) | 32 | 351 |
| cache lost with the prompt unchanged (cached 0 while tokens in grew by under 2k; no harness event in between) | 25 | 829 |
| cache fell back inside the window (cached fell 3-30k while tokens in grew): 14 of 18 are in the browser tasks 1, 24 and 25 and coincide with a picture leaving the window, which rewrites an older result's text | 18 | 410 |
| the person stopped the task and said "continue": the harness opens a fresh window of 12-15k | 17 | 151 |
| rewind by the same-call guard ("stalled: asked for X over and over, so the conversation was cleared") | 12 | 93 |
| window trim at 200 messages (oldest half leaves; tokens in halves, e.g. 66.9k to 33.2k) | 5 | 105 |

The meter's own 20-round rewind never fired in these logs (0 events); all 12 rewinds came from the same-call guard. Where the person's messages did not clear the window (a mid-turn correction, 13 messages) the cache was kept.

**The daemon's checkpoint step decides the uncached cost more than anything the model does.** After any of the events above, the cached count sits at the fixed prefix (10.4k in job 2, 12-13.5k in job 3 and later, 4.7k in tasks 15-19) until the window has grown past the daemon's next context checkpoint. In run5 tasks 9-12 the cached count advanced in steps of 8.4-12.3k (median), so for 161 "catch-up" rounds (1,322k uncached tokens) the whole window beyond the prefix was re-read every round; uncached per round was 4.3-7.6k median in those tasks. From task 13 on the steps are 1.1-1.6k and uncached per round 1.5-2.2k. That matches the `--checkpoint-min-step 1024` change to igo.sh recorded for 6 September.

**Excess uncached tokens** (uncached minus the legitimate tail of record-below-the-line + last reply + last results + 300): 5,420k of the 9,974k uncached tokens, i.e. 54% of all prompt processing, about 215 minutes at 421 tokens/s, 31% of all model time. By task: 1,259k (task 11), 858k (10), 700k (1), 459k (25), 442k (9), 431k (24), 241k (13), 195k (20), 182k (12); tasks 9-13 alone hold 2,982k of it.

Uncached share across a task's life (tasks with 20+ rounds, deciles): median uncached per round rises from 2.1k in the first tenth to 4.2k in the ninth, while the record below the line grows from 0.43k to 1.79k; tokens in sits at 25k in the first tenth and 39-44k after. The record's growth explains about half the rise; the rest is the cache events above.

---

## 2. The tool mix

Calls in all three logs: shell 688, read 650, task 345, edit 212, browser_read 133, write 91, browser_click 70, browser_open 49, computer 30, browser_screenshot 30, search 26, browser_act 7, browser_resize 7, job 5, skill 3, memory 2, browser_handoff 1 (2,349 in all; 1.08 calls a round).

| log | task | rounds | calls | calls by tool | rounds of only `task` | poll-only rounds | reads of rN (repeats) | identical file re-reads | test runs by the model / of them right after a harness rerun | identical shell repeats | fresh windows after a stop and "continue" |
|---|---|---|---|---|---|---|---|---|---|---|---|
| run5 | 1 | 248 | 246 | shell 65, task 45, edit 36, read 36, write 22, computer 15, browser_open 9, browser_read 7, browser_click 6, search 3, job 1, skill 1 | 45 | 1 | 0 | 3 | 31 / 0 | 24 | 1 |
| run5 | 2 | 21 | 21 | task 16, shell 4, read 1 | 16 | 0 | 0 | 0 | 1 / 0 | 0 | 0 |
| run5 | 3 | 10 | 10 | shell 4, task 4, read 2 | 4 | 0 | 0 | 0 | 2 / 0 | 0 | 0 |
| run5 | 4 | 8 | 11 | shell 4, task 4, read 3 | 3 | 0 | 0 | 0 | 3 / 0 | 0 | 0 |
| run5 | 5 | 8 | 20 | task 11, read 6, shell 3 | 3 | 0 | 0 | 0 | 1 / 0 | 0 | 0 |
| run5 | 6 | 32 | 40 | read 16, task 9, edit 8, shell 7 | 6 | 0 | 0 | 0 | 4 / 0 | 0 | 0 |
| run5 | 7 | 9 | 12 | task 8, shell 3, read 1 | 5 | 0 | 0 | 0 | 1 / 0 | 0 | 0 |
| run5 | 8 | 7 | 10 | task 5, read 3, shell 2 | 3 | 0 | 0 | 0 | 2 / 0 | 0 | 0 |
| run5 | 9 | 131 | 132 | shell 93, read 19, edit 8, task 6, write 3, browser 3 | 6 | 47 | 0 | 1 | 0 / 0 | 4 | 0 |
| run5 | 10 | 205 | 216 | shell 122, read 39, write 25, task 22, browser 4, edit 2, search 2 | 22 | 30 | 16 (1) | 2 | 0 / 0 | 32 | 1 |
| run5 | 11 | 309 | 379 | read 174, browser_read 59, shell 53, task 49, browser_open 12, browser_click 10, computer 7, edit 6, search 5, browser_act 4 | 43 | 0 | 116 (41) | 13 | 9 / 0 | 9 | 8 |
| run5 | 12 | 45 | 48 | shell 15, task 14, read 10, browser_read 4, edit 2, browser_open 1, computer 1, browser_click 1 | 14 | 7 | 0 | 1 | 2 / 0 | 1 | 0 |
| run5 | 13 | 61 | 65 | browser_read 23, task 16, read 10, shell 7, computer 4, browser_open 2, browser_click 2, skill 1 | 14 | 0 | 0 | 0 | 1 / 0 | 0 | 0 |
| run5 | 14-16 | 10 | 11 | mostly task, shell, browser_open/screenshot, job | 3 | 0 | 0 | 0 | 0 / 0 | 0 | 0 |
| run5 | 17 | 14 | 14 | task 6, shell 5, write 3 | 6 | 0 | 0 | 0 | 1 / 0 | 0 | 0 |
| run5 | 18 | 36 | 53 | task 21, read 13, shell 9, edit 8, write 2 | 6 | 0 | 0 | 1 | 6 / 5 | 0 | 0 |
| run5 | 19 | 51 | 51 | shell 20, edit 17, task 6, read 5, write 3 | 6 | 0 | 0 | 0 | 13 / 5 | 0 | 0 |
| run5 | 20 | 237 | 244 | read 112, shell 70, edit 40, task 13, write 5, search 4 | 13 | 0 | 10 (0) | 5 | 33 / 16 | 6 | 3 |
| run5 | 21 | 23 | 25 | shell 8, read 8, task 7, edit 2 | 7 | 0 | 0 | 0 | 4 / 1 | 2 | 0 |
| run5 | 22 | 50 | 50 | shell 18, edit 13, read 12, task 5, write 1, memory 1 | 5 | 0 | 0 | 2 | 6 / 3 | 0 | 0 |
| run5 | 23 | 108 | 110 | shell 25, read 24, edit 19, task 12, write 8, browser_open 8, browser_read 8, browser_click 4, browser_screenshot 2 | 10 | 0 | 0 | 4 | 6 / 0 | 0 | 0 |
| run5 | 24 | 236 | 255 | shell 83, read 68, browser_click 40, task 22, browser_read 11, browser_screenshot 9, search 7, browser_open 5, edit 4, browser_act 3, memory 1, write 1, browser_handoff 1 | 18 | 46 | 14 (4) | 14 | 2 / 1 | 2 | 2 |
| run5 | 25 | 111 | 114 | read 30, browser_read 21, browser_screenshot 18, task 9, browser_open 7, browser_resize 7, edit 6, search 5, browser_click 4, computer 3, shell 2, skill 1, write 1 | 9 | 0 | 0 | 4 | 0 / 0 | 0 | 1 |
| run6 | 1 | 7 | 7 | task 3, shell 2, read 1, job 1 | 3 | 0 | 0 | 0 | 0 / 0 | 0 | 0 |
| run6 | 2 | 16 | 16 | task 5, shell 5, write 5, read 1 | 5 | 0 | 0 | 0 | 3 / 2 | 0 | 0 |
| run6 | 3 | 73 | 80 | read 30, shell 28, edit 15, task 4, write 3 | 3 | 0 | 0 | 7 | 18 / 8 | 4 | 1 |
| live | 1 | 6 | 6 | shell 2, task 2, job 2 | 2 | 0 | 0 | 0 | 0 / 0 | 0 | 0 |
| live | 2 | 22 | 23 | task 10, shell 6, write 6, edit 1 | 9 | 0 | 0 | 0 | 4 / 0 | 0 | 0 |
| live | 3 | 38 | 39 | shell 14, edit 11, read 8, task 4, write 2 | 4 | 0 | 0 | 0 | 6 / 3 | 0 | 0 (stopped and resumed before its first round) |
| live | 4 | 41 | 41 | read 16, edit 14, shell 6, task 4, write 1 | 4 | 0 | 0 | 1 | 6 / 3 | 0 | 0 |
| **all** | | **2,173** | **2,349** | | **297** | **131** | **156 (46)** | **58** | **165 / 47** | **84** | **17** |

What the columns say:

- **Bookkeeping-only rounds** (every call is `task`): 297 rounds, 13.7% of all rounds, 1,667k marginal tokens (16%), 98 minutes (14%). Short tasks are the worst: run5 task 2 spent 16 of 21 rounds writing the record; task 11 spent 43 of 309. Of the 345 `task` calls, 53 were refused (27 by the record's own rules such as "this done list has 10 lines and a task's done list holds at most 5", 42 across tools for a missing argument such as "this call names no done line"). Replies that batch several calls happened (54 rounds in task 11) but are the exception: 1.08 calls a round overall.
- **Poll-only rounds** (`shell` with action poll or tail): 131 rounds, 526k tokens, 28 minutes; 47 in task 9, 46 in task 24, 30 in task 10. 24 distinct commands were polled 71 times with a "still running" answer; the longest reported run was 57 s and the sum of the longest reports 930 s. The polled commands were almost all servers that never finish (`python3 -m http.server 8091`, `node qa/playtest.js` kept open), so the poll could only ever say "still running". The other 60 poll rounds found the command finished.
- **Reads of a result id** (`read r12`): 156 calls, 46 of them a second read of the same id; 116 in task 11 alone (the task with 8 stop-and-continue restarts). 66 of the 156 came within 3 rounds of a fresh window: 44 after the 17 stop-and-continue restarts (2.6 a restart), 19 after the 12 rewinds (1.6 a rewind), 3 after a window trim and none at a task start; the other 90 came later, mostly in task 11's browser loops. 70 rounds consisted of nothing but rN reads: 481k tokens, 20 minutes.
- **Repeated reads of the same file region** (identical `read` input already made in the task): 58 calls in 58 rounds, 318k tokens, 18 minutes. Same file, any range: `src/engine.js` was read 48 times in task 20 and 15 in run6 task 3, `main.js` 33 times in task 11, `qa/playtest.js` 20 times and `src/game.js` 17 times in task 24.
- **Test runs by the model**: 165 shell commands matched a test runner (143 of them were summarised by the harness as "tests: ..."), in 165 rounds, 462k tokens, 31 minutes. The harness itself reran the tests after 136 writes/edits and put "tests after this change" on the result. 47 of the model's runs came in the round right after such a result (84k tokens, 8 minutes); the rest followed reads, shell probes or a change the harness could not run tests for (tasks 1, 9-13 had no remembered test command: 0 harness reruns there).
- **Identical shell commands** (the same command text again within a task): 84 calls in 84 rounds, 511k tokens, 33 minutes. Worst: task 10 asked "is the server up?" with curl 11, 9 and 6 times (three variants), task 1 ran `node test/run.js | tail` 8, 7 and 5 times, task 11 launched `google-chrome --headless` with the same arguments 8 times. The same-call guard only sees three identical calls in a row with identical answers (or seven whatever the answer), and a curl to a server answers with a fresh timestamp.

---

## 3. Loops, stalls and interventions

The progress meter writes "rounds since progress: N" into the situation; a streak is a run of rounds with N > 0. The nudge is sent at N = 10 (its text is not in the log; it is inferred from the count), the meter's rewind at 20.

| log | task | streaks | rounds counted as no progress | tokens in them (k) | streaks reaching the nudge | rewinds written (round) | refused calls logged | rounds where every call was refused | refusal kinds |
|---|---|---|---|---|---|---|---|---|---|
| run5 | 1 | 5 | 5 | 30 | 0 | - | 21 | 21 | missing argument 10 (computer: "names no application"), browser/desktop failed 9, record refused 2 |
| run5 | 2-8 | 0 | 0 | 0 | 0 | - | 17 | 12 | record refused 8, missing argument 9 |
| run5 | 9 | 0 | 0 | 0 | 0 | - | 5 | 4 | browser failed 3, other 2 |
| run5 | 10 | 28 | 132 | 1,247 | 3 | 61, 99, 137, 157, 174 (all same-call) | 7 (+13 unlogged guard refusals) | 6 | poll of a dead id 5, browser failed 2 |
| run5 | 11 | 53 | 184 | 1,329 | 4 | 150, 290 (same-call) | 36 (+11 unlogged) | 33 | browser/desktop failed 21, record refused 9, missing argument 4 |
| run5 | 12 | 4 | 17 | 113 | 0 | - | 3 | 2 | |
| run5 | 13 | 10 | 22 | 120 | 0 | - | 8 | 6 | |
| run5 | 14-15 | 2 | 2 | 3 | 0 | - | 0 | 0 | |
| run5 | 16-19 | 25 | 31 | 105 | 0 | - | 8 | 3 | missing argument 7 |
| run5 | 20 | 42 | 66 | 214 | 0 | 76 (same-call) | 4 (+3 unlogged) | 4 | record refused 2, missing argument 2 |
| run5 | 21-22 | 16 | 24 | 166 | 0 | - | 4 | 4 | browser failed 4 |
| run5 | 23 | 26 | 43 | 134 | 0 | 71 (same-call) | 11 (+2 unlogged) | 11 | browser failed 9 |
| run5 | 24 | 33 | 40 | 314 | 0 | 115 (same-call) | 7 (+13 unlogged) | 5 | missing argument 5 |
| run5 | 25 | 27 | 44 | 385 | 0 | - | 6 | 6 | |
| run6 | 1-3 | 17 | 40 | 123 | 0 | 46, 51 (same-call, run6 task 3) | 3 (+6 unlogged) | 3 | |
| live | 1-4 | 19 | 37 | 120 | 0 | - | 5 | 4 | browser/read failed 3 |
| **all** | | **307** | **687** | **4,404** | **7** | **12** | **145 (+~55 unlogged)** | **124** | tool failed 58, missing argument 42, record refused 27, other 18 |

- **Rounds the meter counted as no progress**: 736 rounds carry a count above 0 (34% of rounds), 4,404k marginal tokens (42%), 273 minutes (40%). Of the 307 streaks, 167 were one round long, 108 reached 2-4, 25 reached 5-9 and 7 reached 10; the short ones are the ordinary cost of an edit that does not yet turn a test green.
- **Nudges**: 7 streaks reached 10, all in run5 tasks 10 and 11, 95 rounds and 742k tokens and 39 minutes in all. What those rounds were doing: task 10 rounds 86-98 (shell 11, write 2: curl and node probes of the server), 143-156 (shell 12), 176-189 (task 8, shell 5: writing failures); task 11 rounds 103-115 (read 6, task 2), 167-179 (browser_read 5, task 5), 188-204 (browser_read 13), 229-239 (mixed). Recovery came 3, 4, 4, 3, 3, 7 and 1 rounds after the nudge (mean 3.6), always by a reset of the count, never by the 20-round rewind.
- **Rewinds**: 12, all from the same-call guard, in tasks 10 (5), 11 (2), 20, 23, 24, run6 task 3 (2). The rewind round itself costs little (93k uncached in the 12 rounds) because the window drops to 14-16k; the cost is the re-orientation after it (see rN reads) and, in tasks 10-11, the 8k catch-up steps of the daemon's cache.
- **Refusals**: 145 refusals are in the tool results; the same-call guard's own refusals ("You have already called X with these exact arguments 2 times") are not logged as results at all, and show up only as a tool call with no result: about 55 such calls (task 10: 13, task 24: 13, task 11: 11, run6 task 3: 6, task 9: 5, task 20: 3), 12 of which ended in a rewind. 124 rounds had every call refused: 552k tokens, 49 minutes. The 42 missing-argument refusals are one-line schema problems (computer without an application 14 times, task without a done-line number or with nothing to write, browser_act with an empty batch); 27 were the record refusing a write (done list too long, failure already held, record over its size); 58 were browser or desktop tools failing (page busy after 3000 ms, Chrome timeout, desktop session ended).
- **The person's interventions**: 32 mid-task messages, 21 of them "continue" or "Continue. ..." typed a median of 4 s after a stop from the terminal ("I stopped this task, because the user asked the task to stop": 22 stop replies in run5, 1 each in run6 and live). After a stop the harness reopens the task with a fresh window (12-15k tokens in: the orientation block with the two newest results in full, then the ask), which is the 17 "fresh window" cache events above: task 11 8 times in 100 minutes (it received 12 mid-task messages, 10 of them "continue"), task 20 3 times, task 24 twice, tasks 1, 10, 25 and run6 task 3 once each. The other messages were corrections read mid-turn and kept the window.
- The person's messages themselves were short (median 4 s wait); the human cost is not in the wall time, the token cost is in the restarts.

---

## 4. The record

| log | task | rounds | record first / last / max (k tokens, est.) | goal last | situation last | plan last (lines) | results list last (lines, max lines) | failures last (lines) | record as % of tokens in (mean) | tokens in minus record, median (k) |
|---|---|---|---|---|---|---|---|---|---|---|
| run5 | 1 | 248 | 0.10 / 1.42 / 3.79 | 0.06 | 0.10 | 0.09 (5) | 0.54 (max 220 lines) | 0.44 (3) | 3.8 | 46.9 |
| run5 | 9 | 131 | 0.10 / 2.56 / 2.56 | 0.24 | 0.15 | 0.18 (6) | 1.88 (127 lines) | 0 | 3.0 | 51.2 |
| run5 | 10 | 205 | 0.14 / 3.53 / 3.53 | 0.26 | 0.14 | 0.20 (9) | 0.11 (max 100) | 2.17 (17) | 9.3 | 24.9 |
| run5 | 11 | 309 | 0.29 / 2.84 / 2.86 | 0.26 | 0.12 | 0.25 (8) | 1.26 (max 100) | 0.60 (8) | 9.1 | 23.7 |
| run5 | 20 | 237 | 0.09 / 1.42 / 1.54 | 0.13 | 0.13 | 0.17 (5) | 0.53 (40) | 0.27 (6) | 3.6 | 39.6 |
| run5 | 24 | 236 | 0.11 / 1.84 / 1.85 | 0.21 | 0.14 | 0.19 (8) | 0.55 (40) | 0.23 (4) | 4.1 | 33.2 |
| run5 | 25 | 111 | 0.10 / 1.36 / 1.37 | 0.20 | 0.16 | 0.14 (6) | 0.51 (40) | 0 | 2.8 | 47.2 |
| run6 | 3 | 73 | 0.09 / 1.15 / 1.16 | 0.19 | 0.10 | 0.09 (4) | 0.55 (40) | 0.22 (5) | 3.0 | 28.7 |
| live | 3 | 38 | 0.13 / 0.98 / 0.99 | 0.08 | 0.09 | 0.10 (3) | 0.55 (40) | 0.14 (3) | 2.0 | 33.2 |
| live | 4 | 41 | 0.09 / 1.18 / 1.18 | 0.24 | 0.13 | 0.14 (7) | 0.58 (40) | 0.09 (2) | 1.5 | 44.1 |

(All 32 tasks are in `tables.md`, section T4.)

- The record starts at 0.06-0.29k and ends at 0.2-1.8k in every task since the results list was capped at 40 lines (task 18 onward, and run6/live). Before that cap the results list was the part that grew: 220 lines in task 1 (record 3.79k, and the task failed when a harness write took it to 3,003 of 3,000 tokens), 127 in task 9, 100 in tasks 10-11. In task 10 it was the Failures list instead: 17 failures, 2.17k tokens, 61% of the record, and the task failed on the same size rule at 3,004.
- Across all rounds the record is 4.4% of tokens in on average (median 2.7%); the conversation and system prefix are the rest. Tokens in minus the record is 24-56k median in the long tasks, so the tail below the cache line that every round re-reads is 0.5-1.8k of record plus last round's traffic; the rest of the uncached tokens is cache loss (section 1.4).
- The plan is 0-9 lines and changes little after it is written; the situation line stays under 0.16k. The goal grows only when the model writes "why" and the done list (0.02 to 0.13-0.27k).

---

## 5. Replies (tokens out)

Out per round over all 2,173 rounds: median 100, mean 283, p90 500, max 8,200.

| out tokens | rounds | share | out tokens in bucket (k) |
|---|---|---|---|
| 0-100 | 194 | 9% | 0 |
| 100-200 | 1,078 | 50% | 108 |
| 200-400 | 573 | 26% | 131 |
| 400-800 | 222 | 10% | 112 |
| 800-1,600 | 56 | 3% | 53 |
| 1,600-3,200 | 13 | 1% | 28 |
| 3,200-8,100 | 35 | 2% | 167 |
| at the cap (8,100+) | 2 | 0% | 16 |

| round kind | rounds | out median | mean | p90 | max | wall s median |
|---|---|---|---|---|---|---|
| other tool calls | 1,419 | 100 | 252 | 400 | 5,900 | 15 |
| only `task` | 297 | 100 | 403 | 500 | 8,200 | 11 |
| edit | 211 | 100 | 328 | 600 | 6,000 | 11 |
| poll only | 131 | 0 | 39 | 100 | 500 | 10 |
| write | 91 | 200 | 578 | 900 | 6,300 | 18 |
| no tool call | 24 | 100 | 433 | 200 | 8,100 | 11 |

- Three rounds in four produce 100-400 tokens: a one-line thought and one call. The 50 rounds over 1,600 tokens are whole-file writes and cost 211k tokens out, about 60 minutes of generation at 59 tokens/s (9% of model time in 2% of rounds).
- **Cut-offs at the 8,192 cap: 2**, run5 task 1 rounds 154 and 155 (8.2k and 8.1k out, 189 and 188 s each, 37k marginal tokens, 6.3 minutes), one a `task` call and the next with no call at all; nothing of either round survived. None in run6 or live.
- Rounds with no tool call: 24 (204k tokens). They are the answer that ends a task, or a text-only reply mid-task; the model's text in the latter is not logged.

---

## 6. Browser and desktop use (run5 only; run6 and live made no browser call)

| task | browser/desktop calls | by tool | screenshot calls | refused | residual growth of tokens in after a screenshot round (median, n) | after other rounds (median, n) |
|---|---|---|---|---|---|---|
| 1 | 37 | browser_open 9, computer launch 9, browser_read 7, browser_click 6, computer screenshot 6 | 6 | 17 | 43 (6) | 81 (238) |
| 9 | 3 | browser_open 2, browser_click 1 | 0 | 2 | - | 136 (130) |
| 10 | 4 | browser_open 2, browser_click 2 | 0 | 2 | - | 84 (202) |
| 11 | 92 | browser_read 59, browser_open 12, browser_click 10, computer screenshot 7, browser_act 4 | 7 | 22 | 127 (7) | 96 (298) |
| 12 | 7 | browser_read 4, browser_open 1, computer screenshot 1, browser_click 1 | 1 | 0 | -134 (1) | 80 (42) |
| 13 | 31 | browser_read 23, computer launch 3, browser_open 2, browser_click 2, computer screenshot 1 | 1 | 4 | 21 (1) | 111 (58) |
| 14 | 2 | browser_open 1, browser_screenshot 1 | 1 | 0 | - | 817 (1) |
| 23 | 22 | browser_open 8, browser_read 8, browser_click 4, browser_screenshot 2 | 2 | 5 | 330 (2) | 136 (103) |
| 24 | 69 | browser_click 40, browser_read 11, browser_screenshot 9, browser_open 5, browser_act 3, browser_handoff 1 | 9 | 3 | 458 (9) | 167 (219) |
| 25 | 60 | browser_read 21, browser_screenshot 18, browser_open 7, browser_resize 7, browser_click 4, computer launch 2, computer screenshot 1 | 19 | 2 | 300 (18) | 207 (87) |

(residual growth = tokens in this round minus tokens in last round, last round's out and last round's result text at 4 chars a token; what is left is the picture plus the record's own growth)

- 327 browser/desktop calls in run5, 14% of its calls; 30 browser_screenshot and 15 computer screenshots; 58 of the 145 logged refusals were browser or desktop failures ("the page could not be read at all after 3000 milliseconds", "Chrome stopped working", "the desktop driver could not be reached", "this call names no application" 14 times in task 1).
- **Pictures**: the log does not store them (tool results carry only id, summary and text), so their cost is inferred. In the vision-on tasks (14 onward) a round after a browser_screenshot grew the next prompt by a median 370 tokens (mean 718) against 137 (mean 243) after any other round: about 230-480 tokens a picture here, under the ~800 the harness's comment expects, consistent with the small pages task 25 resized to (429x869, 469x873). Task 12's one screenshot result said "the picture itself is not shown to you", so vision was off in tasks 1-13: the 6+7+1+1 computer screenshots there returned window lists as text and cost no picture tokens. 46 rounds carried a screenshot call in all: 517k marginal tokens, 30 minutes.
- **Pictures leaving the window** cost more than the pictures: the harness keeps two and appends "the picture is no longer shown" to an older result when a third arrives, which changes an early message and drops the cache back to that point. Task 25 shows the cached count falling to 34.1k four times (rounds 25, 34, 38, 42; tokens in 55-75k) and to 13.5-20k three more times; task 24 twice. Those 18 fall-back rounds re-read 410k tokens (about 16 minutes).

---

## 7. What wastes rounds, tokens and time, most expensive first

Costs are for these three logs together (2,173 rounds, 10,588k marginal tokens, 685 minutes). Minutes for token-only items are at the fitted 421 tokens/s. Items overlap (a stalled round can also be a repeated command).

| rank | pattern | rounds | tokens | minutes | what the harness could do |
|---|---|---|---|---|---|
| 1 | **Prompt re-processing beyond the prompt's real growth** (excess uncached): coarse daemon checkpoints in tasks 9-13 (2,982k, 161 catch-up rounds), 25 cache losses with the prompt unchanged (829k), pictures leaving the window (410k), 5 window trims (105k), 25 of 32 task starts re-reading the 10-14k prefix | all | 5,420k of 9,974k uncached (54%) | ~215 (31% of all model time) | Keep the daemon's checkpoint step at 1k (already in igo.sh: uncached/round fell from 5-7k to 1.5-2k); drop a picture without rewriting the result's text (or keep pictures in a separate, last message); give the task loop its own daemon slot or find the second client that evicts it (25 losses, none with a harness event between); when a task starts, reuse the previous task's prefix (only 7 of 32 first rounds did) |
| 2 | **Rounds the progress meter counts as no progress**, of which the 7 streaks that reached the nudge | 736 (95 in nudged streaks) | 4,404k (742k) | 273 (39) | The nudge came at 10 and recovery followed in 1-7 rounds every time, so a nudge at 5 with the same words would have saved roughly 5 rounds in each of the 7 nudged streaks (~35 rounds, ~270k, ~15 min) if recovery came as fast, and might have shortened the 25 streaks that reached 5-9 (167 of the 307 streaks were one round long, 108 were 2-4); count a shell command whose answer differs only by a timestamp as no progress (the curl loops of task 10 ran 26 times) |
| 3 | **Bookkeeping-only rounds** (only `task` calls) | 297 (13.7%) | 1,667k | 98 | Let a `task` write ride on the same reply as the working call (the model batched in 54 of task 11's rounds when it did); mark steps and pin results from the harness's own test result instead of asking the model for a round; the 53 refused `task` calls are schema errors a required field would catch |
| 4 | **Refused calls**: 124 rounds with every call refused (145 logged + ~55 unlogged guard refusals) | 124 | 552k | 49 | Make `application`, `line`, `step` required in the tool schema (42 missing-argument refusals); retry a busy page inside browser_read for up to 3 s before refusing (21 page-busy refusals in task 11 alone); log the same-call guard's refusals as results so they can be counted |
| 5 | **Repeated identical shell commands** | 84 | 511k | 33 | Answer an identical command within N rounds with "same as r12, unchanged" from the harness when the output hash matches; a curl health probe should be one tool ("is port 8091 answering?") with a bounded answer |
| 6 | **Poll-only rounds** of commands that never finish | 131 | 526k | 28 | Run a server as a detached process and answer at once with "listening on 8091" (the 24 polled commands were servers; the longest honest wait was 57 s); make poll block server-side for up to 20 s instead of costing a model round |
| 7 | **Re-orientation after a fresh window** (person stop+continue 17, rewinds 12, task starts): first 3 rounds after a restart, and 70 rounds of only rN reads (156 rN reads, 46 repeats) | 51 + 70 | 339k + 481k | 13 + 20 | Open a fresh window with the last 6 results in full rather than 2 (the model read back 2.6 result ids a restart and 1.6 a rewind by hand); when the person's "continue" comes within a minute of the stop, keep the conversation and only add the message |
| 8 | **The model reruns tests the harness just ran** | 47 (of 165 test rounds) | 84k (462k) | 8 (31) | Put the failing test names and first assertion lines on the write result (the result says only "13 failing of 49"), and say "do not rerun" there |
| 9 | **Repeated reads of the same file region** | 58 | 318k | 18 | Answer an identical read with "unchanged since r33" unless the file changed; a pinned view of a hot file (engine.js was read 48 times in task 20) would cost its size once |
| 10 | **Screenshots**: 46 rounds; ~230-480 tokens a picture, plus item 1's picture fall-backs | 46 | 517k | 30 | Take the picture only when the model asks for it (task 25 took 18 for 7 resizes); keep pictures out of the messages that the cache covers |
| 11 | **Output cap cut-offs** | 2 | 37k | 6.3 | The two rounds were an 8k `task` write; cap plan/done-list writes at a few hundred tokens and refuse longer ones before generation ends |
| 12 | **Record size failures** (tasks 1 and 10 failed at 3,003 and 3,004 of 3,000 tokens) | 2 tasks lost | - | - | Already cut to 40 result lines from task 18 on; the Failures list needs the same cap (17 lines, 2.17k in task 10) |

Reliability, in numbers: of 32 tasks, 26 ended done, 2 failed (task 10 on the record's size rule, task 9 on a fallback program that could not start; task 1 failed the same size rule once and was later resumed to done), 2 were stopped by the person (15, 25), 2 were still running. The three Chrome play-test tasks (9, 10, 11) took 645 rounds, 237 minutes and 21.7M tokens in between them, 30% of everything, and were the only tasks with nudged stalls; the same play-test in job 3 (task 24) took 236 rounds and 55 minutes with 2 stop-and-continue restarts and 7 mid-task messages.

---

## 8. What could not be measured, and why

- **Pictures**: tool results in the log hold only id, summary and text; no picture or its size. Picture cost is inferred from the growth of tokens in.
- **Model calls that leave no checkpoint**: the job's own calls (its task planning and the "review-task-N" it writes into MEMORY.md after each task), done-check and report calls with the tools off. Their tokens and seconds are in no table, and they are the likeliest cause of the 25 "cache lost, prompt unchanged" rounds, which have no harness event between them and the round before. Another client of the same daemon would look the same.
- **The same-call guard's refusals and the nudge text**: neither is logged as an event. Guard refusals were counted as tool calls without a result (~55); nudges were read off "rounds since progress: 10".
- **The model's mid-task text**: a reply that ends a turn without ending the task is not stored, so what the model said before each of the 17 stop-and-continue cycles is unknown; the "stop" itself is only visible through the "I stopped this task" reply.
- **Prompt processing versus generation time**: the log has one wall time per round; the 421 and 59 tokens/s figures are a least-squares fit (residual sd 12 s), not measurements. Whether "out" includes hidden thinking tokens is not knowable from the log.
- **Tokens of the record**: estimated at 4 characters a token from the checkpoint text; the harness counts words x 1.3, and the record printed for the model is split into four pieces with headings, so the prompt's version is a little larger than the estimate.
- **The daemon's checkpoint settings** at each time: inferred from the size of the cached count's steps (8-12k in tasks 9-12, 1.1-1.6k later), not read from the daemon.
- **Idle time**: gaps over 15 minutes (three) are excluded; gaps of 4-256 s before the person's messages are counted in the following round's wall time, which inflates a few rounds (e.g. task 24 rounds 141 and 164) by that much.
- **Runreport's numbers** differ from these where a task has copied checkpoints: it counts the closing and resume checkpoints as rounds and adds their cost line again (4.6% of tokens in for task 11).

Report: `/tmp/claude-1000/-home-jared/09f06b9f-a0be-401c-831d-adb99bc858bb/scratchpad/analysis/behaviour.md`
