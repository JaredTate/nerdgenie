# Nightly set, 2026-09-06, fourth run: the first-line hint on the binary

Local Qwen 3.8 through the daemon on 19091, binary 0314afb2, home /home/jared/work/ng-nightly/home.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens |
|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 5 | 1 | 6 | 68% | 0.4k |
| 02-node-tests | done | pass | 8 | 1 | 7 | 66% | 1.2k |
| 03-fix-the-test | done | pass | 6 | 1 | 7 | 72% | 0.7k |
| 04-page | done | pass | 21 | 2 | 5 | 89% | 2.2k |

4 of 4 checks passed.

The same four asks at 01:06 on the binary that says, once, to put a mark on the first line after a round spent on nothing but a mark: 40 rounds across the four, every check passing. No round in this run was a lone mark, so the hint never fired and nothing here says whether the model takes it up; the asks are too small to mark steps at all. The Tetris run is the measure.

**Caveat found at 01:17:** the runner put every ask's work folder inside the agent's home, which the file tools refuse, so in this run the model wrote its files through the shell and the syntax check and the tests-after-a-change never fired. The rounds still compare with each other across the four runs; they do not compare with the runs after the fix, which put the work beside the home.
