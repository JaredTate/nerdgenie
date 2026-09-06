# The nightly set

Four real asks, run one after another against the local model on a home of their own, each followed by a check that says pass or fail without a person, each measured by `scripts/runreport`. It is the number before and after for every change to the harness. The Tetris build is the optional fifth, an hour or more on its own.

    make build
    scripts/nightly/run.sh /home/jared/work/ng-nightly/home            # the four
    scripts/nightly/run.sh /home/jared/work/ng-nightly/home --with-tetris

The home is any initialised Nerd Genie home (`nerdgenie init` once), with the work folder beside it as its sandbox root, which is where each ask's fresh folder goes: `nerdgenie init -work-folder /home/jared/work/ng-nightly/work` for the home above. Never run the set beside a live run: the local daemon has one slot. The results go to `docs/nightly/<date>.md`, one row per ask: how the task ended, the check, the rounds, the minutes, the seconds a round, the cache share and the output tokens. Copy the row that matters into `docs/PROGRESS.md` when a change is being judged.

| ask | what it proves | the check |
|---|---|---|
| `01-hello` | one file, one run, one answer | `hello.py` prints today's date |
| `02-node-tests` | write code and tests and make them green | at least three tests, `node --test` green |
| `03-fix-the-test` | fix the code and not the tests | the suite is green and the test file is byte for byte the fixture's |
| `04-page` | build a page, serve it, click it, ask it | the log holds a `browser_read` with `ask` answered `1` |
| `05-tetris` | the whole game, end to end | tests green and the page answered a playing state |

Every live task that fails becomes the next ask: put its ask under `asks/`, its check under `checks/`, and any starting files under `fixtures/<name>/`, which the runner copies into the work folder before the ask is sent.
