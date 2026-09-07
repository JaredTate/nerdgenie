# Nightly set, 2026-09-06, second run: the binary with the mark on the first line

Local Qwen 3.8 through the daemon on 19091, binary e54f8bbf, home ~/work/ng-nightly/home.

| ask | ended | check | rounds | minutes | s/round | cache | out tokens |
|---|---|---|---|---|---|---|---|
| 01-hello | done | pass | 4 | 0 | 7 | 59% | 0.3k |
| 02-node-tests | done | pass | 12 | 1 | 5 | 76% | 1.5k |
| 03-fix-the-test | done | pass | 11 | 1 | 6 | 75% | 1.6k |
| 04-page | done | pass | 12 | 2 | 8 | 62% | 1.7k |

4 of 4 checks passed.

The same four asks an hour after the first run, on the binary that marks a step from the reply's first line. Rounds fell from 84 to 39 across the four, but not because of the mark: not one first line carried one, and the marks were made through the task tool as before, more of them in the same reply as other calls. One run each way is one sample; the set is cheap enough to run again after every change, and it should be.

**Caveat found at 01:17:** the runner put every ask's work folder inside the agent's home, which the file tools refuse, so in this run the model wrote its files through the shell and the syntax check and the tests-after-a-change never fired. The rounds still compare with each other across the four runs; they do not compare with the runs after the fix, which put the work beside the home.
