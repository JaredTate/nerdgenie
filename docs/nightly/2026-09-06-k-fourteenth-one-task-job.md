# Nightly set, 2026-09-06

Local Qwen 3.8 through the daemon on 19091, binary 898d318e, home /home/jared/work/ng-nightly/homes/2026-09-06-0925.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks | cut off at cap |
|---|---|---|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 0 | 4 | 87% | 0.3k | 0 | 0 | 0 |
| 02-node-tests | done | pass | 8 | 0 | 4 | 86% | 1.1k | 2 | 0 | 0 |
| 03-fix-the-test | done | pass | 10 | 1 | 3 | 88% | 1.2k | 2 | 0 | 0 |
| 04-page | done | pass | 22 | 2 | 5 | 90% | 3.0k | 7 | 3 | 0 |
| 05-tetris | job 2 done 1 of 1 tasks done | FAIL: Node.js v24.18.0 | 18 | 2 | 6 | 88% | 2.2k | 5 | 2 | 0 |

4 of 5 checks passed.

The fourteenth run, the first with the model's eyes on and the daemon at a 131k context with checkpoints every 1,024 tokens. The four small asks passed in under a minute each but the page task. The game failed before it began: the model made it a job of one task, the scaffold, and the job finished after it, so nothing else was built; the check failed on the folder's empty test runner. The job tool now refuses a job for an ask over six hundred words that lists fewer than three tasks. The daemon's first launch this run had the projector at a 262k context, which pushed the whole model into host memory at a fifth of its speed; the run was stopped at its first ask and relaunched once the context was halved.

