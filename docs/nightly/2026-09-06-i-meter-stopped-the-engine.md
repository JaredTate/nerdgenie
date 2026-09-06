# Nightly set, 2026-09-06

Local Qwen 3.8 through the daemon on 19091, binary d78cbfd5, home /home/jared/work/ng-nightly/homes/2026-09-06-0557.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens | record-only rounds | first-line marks | cut off at cap |
|---|---|---|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 0 | 7 | 61% | 0.3k | 0 | 0 | 0 |
| 02-node-tests | done | pass | 4 | 1 | 8 | 61% | 0.3k | 0 | 0 | 0 |
| 03-fix-the-test | done | pass | 11 | 1 | 6 | 75% | 1.2k | 3 | 0 | 0 |
| 04-page | done | pass | 15 | 1 | 5 | 87% | 1.7k | 6 | 3 | 0 |
| 05-tetris | job 2 waiting 1 of 6 tasks done next: task t2 now | FAIL: no page answer named a playing state | 105 | 31 | 17 | 83% | 34.7k | 15 | 0 | 0 |

4 of 5 checks passed.

The twelfth run of the set, on the binary with every fix of the night up to the Vitest reader, and the rebuilt browser worker from the start. All four small asks passed, the page task in fifteen rounds and one minute where the eleventh's took seventy-eight and six on the double click. The game job's rounds were never cut off at the cap, and the model chose Node's own runner this time, so every test run was read.

**What the run found.** The progress meter stopped the engine task by its own hand. With two tests left red, the model read its five-hundred-line engine in eighty-line windows, probed it with small node scripts and searched it with grep for twenty rounds, and none of it counted: a read of a file counted once however many parts were read, and a command counted never, so the meter cleared the conversation in the middle of the search at round 66, the model started the same search over in a cleared window, and at round 86 the second stall stopped the task and put the job down at one of six. Fixed test first before the thirteenth run: a look or a command whose answer the task has not seen before is progress, and the same answer again is not, whatever the call was worded, which is what the nudge after a failure still stands on. The other finding of the run is not the harness's to fix: the daemon's context checkpoints, 8,192 tokens apart on this hybrid model, made every round re-process a mean of 4,875 tokens, 64 percent of the model's time, and the one flag that changes it is written up in the plan for the owner.
