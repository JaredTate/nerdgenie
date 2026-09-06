# Nightly set, 2026-09-06

Local Qwen 3.8 through the daemon on 19091, binary d60da34c, home /home/jared/work/ng-nightly/homes/2026-09-06-0531.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks | cut off at cap |
|---|---|---|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 0 | 7 | 60% | 0.3k | 0 | 0 | 0 |
| 02-node-tests | waiting | pass | 5 | 1 | 7 | 68% | 0.6k | 1 | 0 | 0 |
| 03-fix-the-test | done | pass | 10 | 1 | 6 | 73% | 1.2k | 3 | 0 | 0 |
| 04-page | done | FAIL: the page never answered 1 | 78 | 6 | 5 | 93% | 8.9k | 6 | 2 | 0 |
| 05-tetris | job 2 waiting 2 of 6 tasks done next: task t3 now | FAIL: no page answer named a playing state | 84 | 16 | 12 | 88% | 27.7k | 17 | 2 | 0 |

3 of 5 checks passed.

The eleventh run of the set, on the binary carrying the tenth run's fixes (the cut-off line, the open-plan backstop, bare done lines) and, from 05:45, the rebuilt browser worker. The game was stopped by hand at 06:22 on its third task, so its line reads "waiting": by then the run had shown what it had to show, and the twelfth run started on the binary with the rest of the night's fixes.

**What the run found.** No round of the game job was cut off at the cap: its tasks wrote the engine in edits of a few hundred tokens each, where the tenth run's had written whole files. Two things it found were harness bugs, both fixed test first before the run was stopped. The page task saw 2 on every click of its counter for seventy-eight rounds, because the browser worker knew a change only as a new element, address, title, dialog, tab or download, read a counter going from 0 to 1 as "nothing changed", and clicked again at the element's place on the screen; the diff now carries the lines of text that appeared. And the harness read none of the game job's ten Vitest runs as a test run, so no rerun fired after a change, a run that went from 36 failing to 16 was "finished with exit code 1" both times, and the progress meter cleared the conversation in the middle of the hazards task after twenty rounds of real work; the test-state reader now knows Vitest. Smaller: two tasks pinned line 1 before writing a done list and read "the done list has 0 lines in it", which now says to write the list first; the node-tests task ended waiting on "Where to next?", which is an offer to carry on and reads as one inside a job; and the review's kept lessons included two headings, which the fourth-answer picker now skips.
