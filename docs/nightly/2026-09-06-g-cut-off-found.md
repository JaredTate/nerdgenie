# Nightly set, 2026-09-06

Local Qwen 3.8 through the daemon on 19091, binary ed7c7a95, home ~/work/ng-nightly/homes/2026-09-06-0402.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks | cut off at cap |
|---|---|---|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 0 | 7 | 60% | 0.3k | 0 | 0 | 0 |
| 02-node-tests | done | pass | 5 | 1 | 8 | 66% | 0.8k | 1 | 0 | 0 |
| 03-fix-the-test | done | pass | 8 | 1 | 5 | 78% | 1.0k | 0 | 0 | 0 |
| 04-page | done | pass | 15 | 2 | 7 | 77% | 2.4k | 3 | 0 | 0 |
| 05-tetris | job 2 waiting 8 of 9 tasks done next: task t9 now | FAIL: no page answer named a playing state | 224 | 83 | 22 | 89% | 199.8k | 47 | 22 | 14 |

The tenth run of the set, and the first on a home made fresh beside the work folder. The cut-off column was added after the run, measured off the log with the run report's new count; the four small asks had none. The game's ninth task was put down by hand at 05:33, in its fifth straight round cut off at the output cap, once the pattern below was clear, so the line reads "waiting".

**What the run found.** Fourteen of the game job's 224 rounds ended at the output cap of 8,192 tokens: 115k of its 200k output tokens and about half an hour of its 83 minutes, each round two minutes of generation kept nowhere. The frontend task (t7) wrote the whole game file in one write call, hit the cap three rounds running, was offered the three options each time, and on the fourth round took "answer the user" with "Now the main game logic. Let me write it carefully", which closed the task done with `style.css` written and no `main.js`; the play-test task (t8) found no game file, tried to write it, and ended the same way; the regression task (t9) was doing it again when it was stopped. Before that, t8 spent ten rounds writing its done list with the results it expected to produce later, r70 to r74 on a record at r20, each refused with "check it against the result list". Built from it, test first, by 05:25: the loop reads the finish before the parse and tells the model it was cut off, how many tokens in, that none of it was kept, and to write a long file in parts of at most three hundred lines; an answer on a record with no done list and a plan step still open is sent back to the plan; the record's refusal of a proof not written yet names the next label and says to write the line bare and pin it later; the `task` tool's done-list field and the rules say the same. The eleventh run measures them.
