# Nightly set, 2026-09-06

Local Qwen 3.8 through the daemon on 19091, binary e81621a6, home ~/work/ng-nightly/homes/2026-09-06-0633.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks | cut off at cap |
|---|---|---|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 5 | 0 | 4 | 86% | 0.4k | 0 | 0 | 0 |
| 02-node-tests | done | pass | 4 | 0 | 3 | 85% | 0.5k | 0 | 0 | 0 |
| 03-fix-the-test | done | pass | 9 | 1 | 4 | 83% | 1.2k | 1 | 0 | 0 |
| 04-page | done | pass | 12 | 1 | 6 | 83% | 1.9k | 2 | 5 | 0 |
| 05-tetris | job 2 waiting 8 of 11 tasks done next: task t9 now | pass | 471 | 143 | 18 | 92% | 165.2k | 93 | 41 | 0 |

5 of 5 checks passed.

The thirteenth run, on the binary with the progress-meter fix. Five of five checks passed, the Tetris check included, with the game job stopped by hand at eight of eleven tasks so that the daemon could be restarted with vision on. No round was cut off at the cap, no conversation was rewound, and the play-test found a freeze in its own game (an unbounded ghost-piece loop the hung-page report named by line) and fixed it. The polish task spent thirty rounds trying to screenshot its canvas, which is what the vision work that followed is for; the play-test's Start click was reported as changing nothing and then clicking twice, which the worker's second look now prevents.
