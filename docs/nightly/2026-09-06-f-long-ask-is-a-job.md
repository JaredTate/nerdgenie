# Nightly set, 2026-09-06, sixth run: a long ask is a job

Local Qwen 3.8 through the daemon on 19091, binary e1b1261d, home ~/work/ng-nightly/home.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks |
|---|---|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 1 | 8 | 61% | 0.3k | 0 | 0 |
| 02-node-tests | done | pass | 4 | 1 | 8 | 61% | 0.5k | 0 | 0 |
| 03-fix-the-test | done | pass | 9 | 1 | 6 | 81% | 1.3k | 3 | 3 |
| 04-page | done | pass | 25 | 3 | 8 | 65% | 3.0k | 14 | 1 |
| 05-tetris | done | FAIL: no page answer named a playing state | 4 | 1 | 16 | 51% | 1.5k | 1 | 0 |

4 of 5 checks passed.

At 02:02 on the binary that holds a task on a long ask to six plan steps. The model made the game a job of twelve tasks in its fourth round, as the rule intends, and the hand-off ended the ask's own task done. The runner then took that ending as the ask's ending, ran the check and stopped its serve under the job with none of its tasks begun, so this row measures four rounds and a check that could not pass. The runner now waits for a job an ask became and serves a fresh home each run; the seventh run started at 02:12.
