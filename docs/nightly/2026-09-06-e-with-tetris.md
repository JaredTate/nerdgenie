# Nightly set, 2026-09-06, fifth run: the work folders beside the home, and the Tetris build as the fifth ask

Local Qwen 3.8 through the daemon on 19091, binary 3d41fe61, home ~/work/ng-nightly/home.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens |
|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 0 | 7 | 61% | 0.3k |
| 02-node-tests | done | pass | 4 | 1 | 8 | 61% | 0.3k |
| 03-fix-the-test | done | pass | 8 | 1 | 8 | 66% | 1.5k |
| 04-page | done | pass | 24 | 2 | 5 | 90% | 3.4k |
| 05-tetris | stopped | FAIL: no page answer named a playing state | 94 | 31 | 20 | 79% | 34.2k |

4 of 5 checks passed.

The first run with the work folders where the file tools reach them, at 01:19 on the binary with the whole night's changes. The four small asks passed in 43 rounds. The Tetris build, the whole game as one ask with no person watching, stopped after 94 rounds and 31 minutes with one of nine plan steps done and one test red: the model took the ask as a single task with a nine-step plan rather than a job, spent the middle of the run on a line-clear test it had diagnosed correctly, wrote that failure and then, told by the stall line to write what the rounds showed, wrote the same failure six more times, each refused as already held, until the same-call guard stopped the task. Twenty-two of its rounds only wrote the record; no mark was made from a first line. The report's numbers: 101 calls (read 43, task 23, shell 20), 9 replies with more than one call, 2.7M tokens in at 79% cached, 34k out.

Two harness lessons, both built the same hour: the stall line must not ask for a failure the record already holds, and a long ask with a plan of many steps is a job.
