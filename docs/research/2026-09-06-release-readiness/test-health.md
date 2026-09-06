# Nerd Genie test health, 6 September 2026

Repository: `/home/jared/Code/coeus` at commit `5df37ea6` (detached HEAD, working tree clean; the branch that `main` tracks). Toolchain: Go 1.27.1, staticcheck 2026.2.1, Node 24.18.0, npm 11.16.0, vitest 4.1.11, TypeScript 7.0.2. Machine: 32 cores, 42 GB.

The repository was not changed. Everything written by this measurement is under `/tmp/claude-1000/-home-jared/09f06b9f-a0be-401c-831d-adb99bc858bb/scratchpad/testreport/` (logs, the coverage profile `all.out`, the two vitest coverage folders, this report). The only things the runs wrote inside the repository tree are Go's own build and test caches and vitest's cache under `node_modules/.vite`, which are outside git.

Rules honoured: no `make check`, `make test`, `make fuzz` or `scripts/fuzz.sh`; nothing tagged `live`; no process killed; nothing under `/home/jared/work` or the Desktop touched. A live agent run was using Chrome on the desktop the whole time (`bin/nerdgenie serve` and `tui` were running, with a Chrome on `/home/jared/work/ng-show`), so `NERDGENIE_HEADLESS_TESTS=1` was set for every Go run and both worker suites; the browser tests honour it in `internal/browser/process.go`, `test/functional/browserworker_integration_test.go` and `worker/browser/test/harness.ts`.

One deliberate deviation: `scripts/gate/fuzz_test.go` holds three integration-tagged tests (`TestTheFuzzScriptRunsEveryTargetItFinds`, `TestTheFuzzScriptFailsWhenATestBinaryWillNotBuild`, `TestTheFuzzScriptLooksBehindTheIntegrationTag`) that copy `scripts/fuzz.sh` and run it, one second per target, on a throwaway fixture module. That is still the forbidden script, so every integration-tagged run below carries `-skip 'TestTheFuzzScript'`, and `scripts/coverage.sh` was run as a copy in the scratchpad that differs from the original in exactly two lines: `cd /home/jared/Code/coeus` instead of `cd "$(dirname "$0")/.."`, and that same `-skip` flag on its `go test -tags integration -cover` line. The three skipped tests exercise a shell script, not the harness, and `scripts/gate` has no coverable statements, so the coverage table is unaffected.

## 1. Layout, as the documents describe it

From `docs/WORK_PLAN.md` Part 1 ("The four kinds of tests", "The test framework and the contracts") and the Testing section of `CLAUDE.md`:

| Kind | Where | How it is told apart |
|---|---|---|
| Unit | `*_test.go` beside every source file; `worker/*/test/*.test.ts` | Runs everywhere, needs nothing installed |
| Integration | The highest package in the dependency order | `//go:build integration`; the real SQLite file and a temporary home |
| Functional | `test/functional/` | Drives the agent through the same socket the terminal uses, with the fakes in `internal/testkit` (scripted model, fake provider server, fake channel, clock, temporary home, fake permission decider, fake signal-cli, fake search server, fake browser worker, fake desktop, golden helper, forty-step fixture) |
| Fuzz | `testing.F` targets in the same test files; `fast-check` in the workers | Run by `scripts/fuzz.sh` (5 s under `make test`, 60 s under `make fuzz`); not run here |
| Live | `//go:build live` | Needs the llama-server daemon on 19091 and the two signed-in command-line programs; not run here |

`make test` runs the integration-tagged set for every package except `internal/browser` and `test/functional`, then `test/functional` untagged, then the fuzz smoke. `make test-browser` runs the browser, desktop and functional packages with the integration tag and `NERDGENIE_HEADLESS_TESTS=1`. `make check` adds gofmt, vet, staticcheck, the style checker, the repo-map drift test and `scripts/coverage.sh` (threshold 90 percent, 70 for `internal/tui`, `cmd/nerdgenie` measured but not gated). The workers' `npm test` is `build`, `typecheck`, then `vitest run --coverage` with a 70 percent floor on lines, statements, functions and branches (`worker/*/vitest.config.ts`); the desktop worker excludes `src/main.ts` from its floor.

## 2. Suite results

| Suite | Command (all run from the repository root with `NERDGENIE_HEADLESS_TESTS=1` and `/usr/local/go/bin:$HOME/go/bin` on the path) | Packages | Top-level tests run | Passed | Failed | Skipped | Wall time | Exit |
|---|---|---|---|---|---|---|---|---|
| Unit + functional + replays | `go test -count=1 -v ./...` | 59 (58 ok, 1 FAIL) | 3758 (plus 1583 subtests) | 3749 | 1 | 8 | 86 s | 1 |
| Integration | `go test -count=1 -v -tags integration -skip 'TestTheFuzzScript' ./...` | 59 (58 ok, 1 FAIL) | 3877 | 3863 | 1 | 13 | 86 s | 1 |
| `go vet ./...` | as written | 59 | – | – | – | – | 20 s | 0, no output |
| `go vet -tags integration ./...` | extra, as the tagged files are otherwise unvetted | 59 | – | – | – | – | 15 s | 0, no output |
| `staticcheck ./...` | as written | 59 | – | – | – | – | 65 s | 0, no output |
| `go run ./scripts/stylecheck ./...` | as written | – | – | – | – | – | 4 s | 0, no output |

Top-level counts come from `--- PASS`, `--- FAIL` and `--- SKIP` lines at column zero in the `-v` logs (`unit.log`, `integration.log`); subtests are indented and not counted.

### 2.1 The one failing test

Both Go runs fail the same single test, and nothing else:

```
scripts/repomap  TestTheRepositoryMapIsNotStale  (drift_test.go:27)
    REPO_MAP.md is stale. Run "make repo-map" and commit the result.
    The generated map is 58739 bytes and the one on disk is 58730 bytes.
```

This is the repository-map drift check that `make check` also runs. It fails because `REPO_MAP.md` is nine bytes behind the tree at commit `5df37ea6` (the two most recent commits added `IDEAS.md` and research documents without regenerating the map). It is a documentation-freshness failure, not a defect in the harness; `make repo-map` and a commit clear it. Every other test in `scripts/repomap` (21 tests, 2 fuzz targets' seeds) passes.

### 2.2 Per-package results

Every package other than `scripts/repomap` is `ok` in both runs. Times are wall seconds as `go test` printed them.

| Package | Unit run | Integration run |
|---|---|---|
| cmd/nerdgenie | ok 5.3 | ok 5.0 |
| internal/browser | ok 0.2 | ok 3.2 (starts headless Chrome through the real worker) |
| internal/channel | ok 3.1 | ok 3.3 |
| internal/clock | ok 0.0 | ok 0.0 |
| internal/command | ok 0.3 | ok 0.4 |
| internal/config | ok 0.1 | ok 0.2 |
| internal/context | ok 0.2 | ok 0.3 |
| internal/contract | ok 0.0 | ok 0.0 |
| internal/desktop | ok 0.6 | ok 0.6 (its three screen-driving tests skip without `NERDGENIE_LIVE_DESKTOP=1`) |
| internal/job | ok 2.4 | ok 2.7 |
| internal/lint | ok 0.0 | ok 0.0 |
| internal/log | ok 2.0 | ok 2.1 |
| internal/loop | ok 1.9 | ok 1.7 |
| internal/memory | ok 2.3 | ok 2.3 |
| internal/orientation | ok 0.0 | ok 0.0 |
| internal/permission | ok 0.1 | ok 0.2 |
| internal/provider | ok 0.8 | ok 0.8 |
| internal/record | ok 0.2 | ok 0.3 |
| internal/reliability | ok 0.7 | ok 1.0 |
| internal/repair | ok 0.0 | ok 0.0 |
| internal/replay | ok 0.1 | ok 0.2 |
| internal/sandbox | ok 2.3 | ok 7.5 (38 integration tests: real namespaces, seccomp, resolver) |
| internal/signal | ok 0.6 | ok 0.7 |
| internal/skill | ok 0.2 | ok 0.2 |
| internal/skill/browser | ok 0.1 | ok 0.2 |
| internal/testkit | ok 3.5 | ok 3.4 |
| internal/tool | ok 0.4 | ok 0.3 |
| internal/tool/browseract, browserclick, browserhandoff, browserlogin, browseropen, browserread, browserresize, browsershot, browsertype | ok, each under 0.1 | ok, each under 0.1 |
| internal/tool/computer, edit, job, loose, memory, read, search, shell, skill, task, web, write | ok, each under 0.7 | ok, each under 0.2 |
| internal/tui | ok 1.9 | ok 1.8 |
| internal/update | ok 0.5 | ok 1.1 |
| internal/vault | ok 0.9 | ok 1.0 |
| scripts/fixturesite | ok 0.0 | ok 0.0 |
| scripts/gate | ok 5.3 | ok 0.9 (the three fuzz-script tests skipped by flag) |
| scripts/release | ok 3.6 | ok 2.1 (container test skips: no Docker or Podman daemon answers) |
| scripts/repomap | **FAIL** 0.1 | **FAIL** 0.1 |
| scripts/runreport | ok 0.0 | ok 0.0 |
| scripts/stylecheck | ok 0.0 | ok 0.0 |
| test/functional | ok 84.3 | ok 84.6 (plus the two real-worker browser flows, headless) |
| test/replays | ok 0.0 | ok 0.0 |

### 2.3 What was skipped, and why

Unit run, 8 skips: the five `t.Skip` stubs in `test/functional/browserflows_test.go` (see the phantom section); `internal/skill/browser/writeqa_test.go:19`, a generator that only runs with `NERDGENIE_WRITE_QA_SKILL=1`; `internal/tui/ground_test.go:139` and `internal/tui/pseudoterminal_test.go:101`, the child halves of two pseudo-terminal tests that only draw when the parent process starts them (the parent halves ran and passed).

Integration run, 13 skips: the eight above, plus `internal/desktop/integration_test.go` three times (they drive the real screen, only with `NERDGENIE_LIVE_DESKTOP=1`, which was deliberately not set), `internal/reliability/integration_test.go:51` (the helper-process half of the kill test), and `scripts/release/container_test.go:100` (no container daemon answers on this machine; `.github/workflows/install.yml` runs it). The release archives are present in `dist/`, so only the daemon was missing.

## 3. Coverage

### 3.1 The gate's own table (`scripts/coverage.sh`)

Command: `bash /tmp/.../testreport/coverage-copy.sh` (the two-line-different copy described at the top), with `NERDGENIE_HEADLESS_TESTS=1`. It finished in nine seconds, which means Go served the per-package `-cover` results from its test cache (the script does not pass `-count=1`; the cache is keyed on the test binaries and every input they read, so the numbers are the numbers for this tree). Exit 1. The only reason for the exit code is the `scripts/repomap` failure above: every gated package is at or above its threshold.

```
PACKAGE                                                   COVERED   NEEDED  RESULT
github.com/JaredTate/nerdgenie/internal/browser             90.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/channel             96.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/clock              100.0%      90%  ok
github.com/JaredTate/nerdgenie/internal/command             95.2%      90%  ok
github.com/JaredTate/nerdgenie/internal/config              96.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/context             97.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/contract            98.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/desktop             95.3%      90%  ok
github.com/JaredTate/nerdgenie/internal/job                 91.1%      90%  ok
github.com/JaredTate/nerdgenie/internal/lint                91.2%      90%  ok
github.com/JaredTate/nerdgenie/internal/log                 93.6%      90%  ok
github.com/JaredTate/nerdgenie/internal/loop                92.2%      90%  ok
github.com/JaredTate/nerdgenie/internal/memory              92.3%      90%  ok
github.com/JaredTate/nerdgenie/internal/orientation         94.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/permission          98.6%      90%  ok
github.com/JaredTate/nerdgenie/internal/provider            93.8%      90%  ok
github.com/JaredTate/nerdgenie/internal/record              94.9%      90%  ok
github.com/JaredTate/nerdgenie/internal/reliability         91.2%      90%  ok
github.com/JaredTate/nerdgenie/internal/repair              98.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/replay              94.3%      90%  ok
github.com/JaredTate/nerdgenie/internal/sandbox             94.6%      90%  ok
github.com/JaredTate/nerdgenie/internal/signal              92.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/skill               92.1%      90%  ok
github.com/JaredTate/nerdgenie/internal/skill/browser       94.1%      90%  ok
github.com/JaredTate/nerdgenie/internal/testkit             90.0%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool                92.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browseract     92.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browserclick    92.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browserhandoff    93.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browserlogin    92.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browseropen    97.2%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browserread    96.9%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browserresize    96.0%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browsershot    91.4%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/browsertype    93.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/computer       90.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/edit           91.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/job            95.6%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/loose          97.2%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/memory         93.3%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/read           91.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/search         94.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/shell          93.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/skill          95.8%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/task           97.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/web            93.7%      90%  ok
github.com/JaredTate/nerdgenie/internal/tool/write          91.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/tui                 93.1%      70%  ok
github.com/JaredTate/nerdgenie/internal/update              91.5%      90%  ok
github.com/JaredTate/nerdgenie/internal/vault               91.9%      90%  ok
github.com/JaredTate/nerdgenie/scripts/fixturesite          91.2%      90%  ok
github.com/JaredTate/nerdgenie/scripts/gate                  none        -  no statements
github.com/JaredTate/nerdgenie/scripts/release               none        -  no statements
github.com/JaredTate/nerdgenie/scripts/repomap                  -        -  TESTS FAILED
github.com/JaredTate/nerdgenie/scripts/runreport            91.0%      90%  ok
github.com/JaredTate/nerdgenie/scripts/stylecheck           95.5%      90%  ok
github.com/JaredTate/nerdgenie/cmd/nerdgenie                83.1%      90%  not gated
```

Two things worth noticing in the table. `internal/testkit` sits exactly on its 90.0 percent threshold, with no margin at all: one more uncovered statement there fails the gate. `cmd/nerdgenie` is at 83.1 percent, measured but not gated by design ("it is the wiring the orchestrator owns"). Nothing is between 90 and 91 except `testkit`; the next thinnest margins are `internal/tool/computer` (90.5) and `internal/browser` (90.7).

### 3.2 Whole-repository statement coverage

Command: `go test -count=1 -tags integration -skip 'TestTheFuzzScript' -coverprofile=all.out -coverpkg=./internal/...,./cmd/... ./internal/... ./cmd/... ./test/...` (53 packages ok, exit 0, 85 s), then `go tool cover -func=all.out | tail -1`:

```
total:  (statements)  92.8%
```

Counting the profile's blocks directly (each block covered if any of the 53 test binaries covered it) gives the same figure: **25,192 of 27,157 statements, 92.8 percent** across `internal/` and `cmd/`. The `-skip` flag changes nothing here because `scripts/gate` is neither measured nor run in this command.

| Target | Where the repository stands | Statements still to cover |
|---|---|---|
| 90 percent | 2.8 points above it | 0 (the floor is already cleared by 748 statements) |
| 95 percent | 2.2 points below it | 608 more statements must be covered (25,800 of 27,157) |

The profile's per-package figures differ a little from the gate table because the profile counts a package's statements as covered when any test binary ran them, so `internal/testkit` reads 91.8 percent here (the functional and tool suites exercise its fakes) against 90.0 percent in the gate's own-package measure.

Ten least-covered packages, from the profile:

| Package | Statements covered | Percent |
|---|---|---|
| cmd/nerdgenie | 1493 / 1796 | 83.1 |
| internal/tool/computer | 95 / 105 | 90.5 |
| internal/browser | 682 / 752 | 90.7 |
| internal/job | 1041 / 1143 | 91.1 |
| internal/lint | 320 / 351 | 91.2 |
| internal/reliability | 654 / 716 | 91.3 |
| internal/tool/browsershot | 32 / 35 | 91.4 |
| internal/tool/write | 75 / 82 | 91.5 |
| internal/tool/edit | 130 / 142 | 91.5 |
| internal/update | 537 / 586 | 91.6 |

Least-covered functions, from `go tool cover -func` (3,551 functions measured; fifteen are at zero, so the "ten least" are all zeros and the whole zero list is given, followed by the next five):

| Function | Coverage |
|---|---|
| cmd/nerdgenie/signalchannel.go:58 `useSignal` | 0.0 |
| cmd/nerdgenie/signalchannel.go:74 `theSignalProgram` | 0.0 |
| cmd/nerdgenie/signalchannel.go:86 `signalMessages` | 0.0 |
| cmd/nerdgenie/streaming.go:31 `wroteUnseen` | 0.0 |
| internal/browser/methods.go:137 `Resize` | 0.0 |
| internal/loop/run.go:186 `Read` | 0.0 |
| internal/provider/stream.go:65 `retryableConnection` | 0.0 |
| internal/skill/browser/walkrecord.go:123 `sayWhatWentWrong` | 0.0 |
| internal/testkit/golden.go:75 `mustRead` | 0.0 |
| internal/tool/shell/escalate.go:148 `killGroup` | 0.0 |
| internal/tool/usertool.go:129 `nameOf` | 0.0 |
| internal/tool/usertool.go:145 `note` | 0.0 |
| internal/tui/markdown.go:40 `fencedRow` | 0.0 |
| internal/tui/size.go:34 `terminalSize` | 0.0 |
| internal/tui/size.go:71 `writerFor` | 0.0 |
| cmd/nerdgenie/serving.go:134 `runDueJobs` | 7.1 |
| cmd/nerdgenie/serve.go:68 `runServe` | 7.5 |
| cmd/nerdgenie/signalchannel.go:23 `openTheSignalChannel` | 10.0 |
| internal/tui/panel.go:240 `cutRowWithEllipsis` | 16.7 |
| internal/tui/markdown.go:151 `splitLongWord` | 22.2 |

The zeros cluster where a real outside thing is needed: the Signal channel wiring, the real terminal size, killing a process group, the streaming retry path. `internal/browser.Resize` at zero is the one that looks like a plain gap, since the tool that calls it (`internal/tool/browserresize`) is at 96 percent against the fake.

## 4. The two TypeScript workers, headless

`npm test` in each worker is `npm run build && npm run typecheck && vitest run --coverage`. To write nothing into the repository the build step was left out (both `dist/main.js` files are newer than every file in their `src/`, which is the staleness rule the Makefile and the Go integration test use, and `worker/browser/test/process.test.ts` starts that very `dist/main.js`), the typecheck was run as written (both `tsc` passes are `--noEmit`), and vitest was pointed at a scratchpad coverage folder. Commands, run in each worker's folder with `NERDGENIE_HEADLESS_TESTS=1`:

```
npm run typecheck
npx vitest run --coverage --coverage.reportsDirectory=/tmp/.../testreport/cov-<worker>
```

| Worker | Typecheck | Test files | Tests | Failed | Skipped | Statements | Branches | Functions | Lines | Floor | Wall time | Exit |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| worker/browser | 0 | 20 passed | 331 passed | 0 | 0 | 88.67 | 81.14 | 88.70 | 89.08 | 70 on all four: met | 117 s (113 s in tests, one file at a time, each starting a headless Chrome) | 0 |
| worker/desktop | 0 | 9 passed, 1 skipped | 163 | 0 | 12 (the whole `fixturewindow.test.ts` block, which skips itself without `NERDGENIE_LIVE_DESKTOP=1`) | 78.02 | 71.28 | 76.47 | 77.47 | 70 on all four (src/main.ts excluded): met, branches by 1.3 points | 4.4 s (nothing starts a window; every test drives the fake driver) | 0 |

No test failed in either worker, so no rerun for flakiness was needed. The browser worker's lowest files are `main.ts` (0 percent: the process entry point, proved by the Go side starting the real worker), `pdf.ts` (70.4), `events.ts` (78.7) and `chrome.ts` (80.3).

The desktop worker's numbers are low for one reason: `src/cuadriver.ts`, the real driver over `@trycua/cua-driver`, is at 0.93 percent of statements (lines 33-286 never run) because the only tests that reach it are the twelve in `fixturewindow.test.ts`, which drive a real window and were deliberately skipped. Every other file is between 88.9 and 100 percent of lines; `worker.ts` has the thinnest branch coverage (62.5). Because the floor counts `cuadriver.ts`, the branch measure sits at 71.28 against the 70 floor: one more untested branch elsewhere in the worker would fail `npm test` on a machine without the live desktop switch. The browser worker's margins are wide (18 to 19 points on lines, statements and functions; 11 on branches).

## 5. Counts

### 5.1 Go

| What | Count | How it was counted |
|---|---|---|
| `_test.go` files | 749 | `find . -name '*_test.go'` outside `node_modules` |
| `func Test...` matches | 3,814 | `grep -rhoE "^func (Test|Fuzz)[A-Za-z0-9_]+" --include='*_test.go'`; five of these are `TestMain` (in `internal/log`, `internal/job`, `internal/desktop`, `internal/sandbox`, `internal/browser` integration files), so 3,809 are tests |
| `func Fuzz...` matches | 83 | same grep; fuzz targets in 35 packages, none in `test/` or live files; 15 crash-regression corpus entries checked in under `testdata/fuzz/` |
| Test and fuzz functions together, `TestMain` excluded | 3,892 | the grep list and a `go/ast` parse of every `_test.go` agree name for name |
| By area (tests / fuzz) | internal 3,413 / 79; cmd 238 / 2; scripts 68 / 2; test 90 / 0 | |
| Functional tests under `test/` | 90: 89 in `test/functional` (52 files), 1 in `test/replays` | of the 89, 6 are live-tagged (`livefixture_test.go` 3, `livetask_test.go` 3) and 2 are integration-tagged (`browserflows_integration_test.go`), the other 81 run untagged |
| Integration-tagged files | 36 files, 128 test functions, 6,511 lines | `grep -rl "go:build integration"`; the biggest holders are `internal/sandbox` (8 files, 38 tests), `internal/update` (10), `internal/job` (9), `internal/reliability` (8), `internal/vault` (7) |
| Live-tagged files | 7 files, 12 test functions, 1,287 lines | `internal/context` 2 files (2 tests), `internal/provider` 2 files (4), `test/functional` 3 files (6; `livehome_test.go` holds only helpers) |
| Golden files | 14 `*.golden` | all under `internal/command`, `internal/job`, `internal/sandbox` `testdata/` |
| Golden-file tests | 64 test functions call `testkit.Golden` directly, at 70 call sites in 43 files | the helper compares against any file under `testdata/`, so it also serves the 91 `.txt` frames and the 7 `.json` bodies |

Top-level runs in the two suites (3,758 untagged, 3,877 with the tag) are a little below 3,892 minus the tagged-out functions because fuzz targets run as one test each and the live files are never compiled.

### 5.2 TypeScript

| What | Count |
|---|---|
| Test files | browser 20, desktop 10 (plus `harness.ts`, `server.ts`, `fakedriver.ts` and the `pages/` fixtures) |
| `it(` calls | browser 233, desktop 0 |
| `test(` calls | browser 0, desktop 129 |
| `test.each` tables | desktop 5 (in `keys`, `marks`, `methods`, `session`, `wire`), 46 rows between them |
| `fc.assert` property tests | browser 12, desktop 6 |
| `.skip`, `.todo`, `.only`, `xit`, `xtest` | none in either worker |
| Tests vitest actually ran | browser 331; desktop 163 |

The runtime count is above the source count in the browser worker because several `it(` calls sit inside loops over fixtures.

## 6. Phantom tests

Method for Go: a `go/ast` program (`scratchpad/phantom/main.go`) parsed every `_test.go` file regardless of build tags and, for each `Test`/`Fuzz` function, counted calls to `.Error/.Errorf/.Fatal/.Fatalf/.Fail/.FailNow`, `.Skip*`, `.Log*`, and calls that pass the testing value on to a helper. 83 of 3,892 were flagged (no failing call, or a skip, or an empty or log-only body); a second pass reported the failing calls inside every helper those 83 lean on, and the flagged tests were read. Method for TypeScript: every `it(`/`test(`/`test.each(` block in `worker/*/test` was cut out by indentation and searched for `expect(`; TypeScript 7 ships no JavaScript parser API, so this was a text scan, and the five blocks it first flagged were read.

### 6.1 Real phantoms

| Where | What it is | Verdict |
|---|---|---|
| `test/functional/browserflows_test.go:28` `TestLoggingInToTheFixtureSiteThroughTheVault` | body is `t.Skip("waits for internal/browser (brief 5.2)")` | Real phantom: a placeholder that always skips. The comment above each says what the test will prove |
| `test/functional/browserflows_test.go:36` `TestPostingWithAPreviewTheUserApproves` | same one-line skip | Real phantom |
| `test/functional/browserflows_test.go:44` `TestHittingTheCaptchaPageHandsTheBrowserToTheUser` | same | Real phantom. `browserflows_integration_test.go:68` covers the captcha wall against the real worker, but this untagged stub was never replaced |
| `test/functional/browserflows_test.go:51` `TestReplayingARecordedSkillOnTheFixtureSite` | same | Real phantom |
| `test/functional/browserflows_test.go:59` `TestTheQualitySkillReportsOnTheFixtureSite` | same | Real phantom. `internal/browser` (brief 5.2) and `internal/skill/browser` (5.3) have long since merged, so the reason in the skip is stale; these five are the whole skip count of an ordinary run |
| `cmd/nerdgenie/interrupted_test.go:175` `TestMarkInterruptedIsQuietWhenThereIsNoRecord` | one statement, `markInterrupted(ctx, fakeStore, "999")`, nothing checked | Assertion-free: it proves only that the call returns without panicking. Real phantom by the definition; deliberate by its comment ("costs nothing: the write is skipped") but nothing checks that the write was skipped |
| `cmd/nerdgenie/serving_loops_test.go:8` `TestFeedingTheWatchdogIsQuietWithNoServiceManager` | calls `feedTheWatchdog` and nothing else | Assertion-free; a hang would only be caught by the ten-minute test timeout. The setup helper `anAgentWithASocket(t)` does fail if the agent cannot start, so it is not empty, but the property named in the title is not asserted |
| `cmd/nerdgenie/serving_loops_test.go:30` `TestTheDrainerAndTheJobDriverComeBackWhenTheContextIsDone` | calls `drain` and `runDueJobs` with a cancelled context | Assertion-free in the same way: "comes back at once" is not timed or checked |
| `cmd/nerdgenie/streaming_test.go:7` `TestStreamingThePiecesDoesNothingWithNoSocket` | two calls on an empty `agent{}` | Assertion-free; proves no nil-pointer panic and nothing else |
| `cmd/nerdgenie/streaming_test.go:15` `TestStreamingThePiecesReachesTheSocketWhenThereIsOne` | three calls, nothing read back from the socket | Assertion-free; the title promises the pieces reach the socket, the body never looks |
| `internal/permission/fuzz_test.go:50` `FuzzRulebookMatch` | the fuzz body calls `book.Match` and skips patterns the rulebook refuses; no check on the result | Crash-only fuzz target. Legitimate as a "never panics" target, but it asserts nothing about the answer, unlike its two neighbours which pass the result to `checkReadableForm` |
| `internal/skill/browser/writeqa_test.go:17` `TestWriteTheShippedQualitySkill` | skips unless `NERDGENIE_WRITE_QA_SKILL=1`; when run it writes `skills/qa` | Not a test but a generator wearing a test's name; it skips in every ordinary run. The comment says so |

No Go test has an empty body, and none is log-only apart from the five skip stubs.

### 6.2 False positives: tests that assert through a helper

All of these were flagged for having no `t.Error`/`t.Fatal` of their own, and every helper they call was confirmed to fail on its own (counts are the failing calls inside the helper):

| Tests | Helper that fails |
|---|---|
| 18 tests whose only check is `testkit.Golden(...)`: `internal/command/core_test.go:94`, `service_test.go:63`, `session_test.go:54`; `internal/job/command_test.go:60`; `internal/record/print_test.go:13,19`; `internal/repair/golden_test.go:172`; `internal/sandbox/arguments_test.go:33`, `seccomp_test.go:153`; `internal/testkit/golden_test.go:11`; `internal/tool/browserread/browserread_test.go:155`; `internal/tool/web/text_test.go:11`; `internal/tui/context_test.go:216`, `gate_test.go:142`, `look_test.go:337`, `panel_test.go:271,304`, `scroll_test.go:287` | `testkit.Golden` calls `t.Errorf` on a mismatch and `t.Fatalf` when the file cannot be read or rewritten (`internal/testkit/golden.go:38,48`) |
| `internal/permission/askmefirst_test.go:80,84,88,104` | `checkAskMeFirst` (2 failing calls) |
| `internal/config/check_test.go:99,125` | `eachIsRefused` (6) |
| `internal/record/parsebad_test.go:19,24,131` | `checkEveryBreakIsRefused` (2), `checkTheRuleIsNamed` (2) |
| `internal/provider/codexwire_test.go:133,139,153,165,202` | `wantSameBody` (1), `goldenCodexBody` (1), `codexBodyFor` (1) |
| `internal/job/next_test.go:423,470` | `mustComeBackAtOnce` (2), `mustStayAsleep` (1), `waitForSleeper` (1) |
| `internal/tui/scroll_test.go:120,136,169` | `expectRows` (1), `expectMark` (1) |
| `internal/channel/detach_test.go:142` | `assertRunningTaskStatus` (4), `waitForExactlyAttached` (1) |
| `internal/signal/stream_test.go:90` | `waitFor` (1) and `runStream` → `newTestClient` (1) |
| `internal/provider/codexlogin_test.go:238` | `mustAdviseSigningIn` (3), `mustNotLeak` (2) |
| `internal/provider/live_test.go:171,176` (live) | `runTheProgramSubtest` (9), `checkProgramIsThere` (1) |
| `internal/repair/fuzz_test.go:17` `FuzzFind` | `checkResult` (6) |
| `internal/permission/fuzz_test.go:12,29` `FuzzReduce`, `FuzzReduceShellCommand` | `checkReadableForm` (2) |
| `internal/browser/fuzz_test.go:14` `FuzzTheProtocolLineReader` | `readOneAnswerEveryWay` (1) |
| `internal/tui/frame_fuzz_test.go:22,42` | `drawsInsideTheTerminal` (1) |
| `test/functional/reliability_test.go:50` | `screen.waitForReplySaying` → `waitUntil` (3) |
| `test/functional/livefixture_test.go:41,45,49` and `livetask_test.go:34,38,42` (live) | `theFortyStepFixtureOnARealModel` (2 plus `checkTheFixtureAssertions`), `oneSmallTaskOnARealModel` (1 plus two check helpers) |

### 6.3 Conditional skips that are not phantoms

These have real assertions and skip only for a stated reason: `internal/permission/evasion_test.go:279` (the fuzzer picked an input that names no disguise), `internal/provider/codexlogin_test.go:301` (root can open a file with no permission bits), `internal/reliability/drain_test.go:104` (no boot identifier on this machine), `internal/reliability/integration_test.go:48` (helper-process half), `internal/sandbox/integration_resolver_test.go:22` (the machine cannot resolve the name itself), `internal/sandbox/integration_review_test.go:146` (not amd64), `internal/tool/edit/fuzz_test.go:13`, `internal/tool/shell/fuzz_test.go:13`, `internal/tool/web/fuzz_test.go:13,41`, `internal/tool/web/results_fuzz_test.go:17` (the fuzzer wrote more than the cap), `internal/tool/shell/bounds_test.go:195` (the fake sudo folder could be removed), `internal/tui/ground_test.go:137` and `pseudoterminal_test.go:99` (child halves), `scripts/release/container_test.go:97` (no daemon, or no archive), `scripts/repomap/generate_test.go:386` (git does not leave a conflicted file in the index thrice).

### 6.4 TypeScript

367 `it(`/`test(`/`test.each(` blocks across both workers. Every one of them contains an `expect(` call. The five that the first text scan flagged (`worker/desktop/test/keys.test.ts:6`, `marks.test.ts:48`, `methods.test.ts:91`, `session.test.ts:36`, `wire.test.ts:36`) are `test.each` tables where the scan stopped at the table's closing bracket; their callbacks all assert. No `.skip`, `.todo` or `.only` anywhere. `worker/desktop/test/fixturewindow.test.ts:53` is a `describe.skipIf(...)` block that skips unless `NERDGENIE_LIVE_DESKTOP=1` and a display are present; its tests are real and skipped for the same reason the Go desktop integration tests are.

### 6.5 Golden files

All 14 `*.golden` files under `testdata/` are referenced by a test. Eleven are named literally in a test file. Three are named by computed strings and were checked by reading the tests: `internal/sandbox/testdata/seccomp-amd64.golden` is `"seccomp-"+runtime.GOARCH+".golden"` at `seccomp_test.go:156`; `internal/command/testdata/nerdgenie-backup.service.golden` and `nerdgenie-backup.timer.golden` are `unit.Name+".golden"` in the loop at `service_test.go:76`. As a bonus the other 124 non-corpus files under `testdata/` were checked the same way: 87 are named literally in their package's tests; the other 37 are reached by computed names (`internal/tui`'s 26 sized frames such as `"panel-task-"+WxH+".txt"`, `internal/provider/testdata/codex/*.json` through `goldenCodexBody`, and the `internal/skill/testdata` skill folders read whole through `ReadFolder`). No orphan fixture was found.

## 7. Line counts and ratios

`wc -l` over explicit file lists (kept beside this report as `files.*.txt`), excluding `node_modules`, `dist`, `bin`, `REPO_MAP.md`, `docs/html`, `*.golden`, everything under `testdata/`, and `package-lock.json`. The 17 `.go` files under `testdata/` (lint fixtures) are excluded from every Go figure.

| Body of code | Files | Lines |
|---|---|---|
| Production Go: `internal/`, `cmd/`, `scripts/`, not `_test.go`, not `internal/testkit` | 493 | 74,669 |
| of which the harness alone (`internal/` + `cmd/`) | 483 | 73,848 |
| of which `scripts/` | 10 | 821 |
| Test-support Go: `internal/testkit`, not `_test.go` | 39 | 7,301 |
| Test Go: every `_test.go` (696 files, 110,926 lines, of which 39 files and 7,518 lines test testkit itself) plus `test/` (53 files, 6,899 lines: `test/functional` 6,846, `test/replays` 53) | 749 | 117,825 |
| Production TypeScript: `worker/*/src` | 45 | 6,691 |
| Test TypeScript: `worker/*/test` | 33 | 5,107 |
| Documents: `*.md` outside `docs/html`, without `REPO_MAP.md` | 129 | 21,101 (root 5,437; `docs/` 14,360; elsewhere, mostly `skills/` and the two `PROTOCOL.md`, 1,518) |

Ratios of test lines to production lines:

| Scope | testkit counted as test code | testkit counted as production code | testkit left out of both |
|---|---|---|---|
| Go | 125,126 / 74,669 = **1.68** | 117,825 / 81,970 = **1.44** | 117,825 / 74,669 = 1.58 |
| TypeScript | 5,107 / 6,691 = **0.76** | same | same |
| Overall | 130,233 / 81,360 = **1.60** | 122,932 / 88,661 = **1.39** | 122,932 / 81,360 = 1.51 |

Against the harness alone (`internal/` + `cmd/` production Go, 73,848 lines), with testkit counted as test code, the ratio is 1.69.

## 8. Summary

- The Go tree passes everything except one repository-map freshness check (`scripts/repomap.TestTheRepositoryMapIsNotStale`, nine bytes of drift), in both the untagged and the integration-tagged run; vet, staticcheck and the style checker are silent.
- Coverage is 92.8 percent of statements across `internal/` and `cmd/`; every gated package clears its floor, with `internal/testkit` sitting exactly on 90.0 and `cmd/nerdgenie` ungated at 83.1. Reaching 95 percent needs 608 more covered statements.
- Both workers pass headless, browser at 88.7 percent statements (331 tests), desktop at 78.0 percent (163 tests), both above the 70 floor.
- Twelve Go tests are phantoms in the strict sense: five permanently-skipped stubs in `test/functional/browserflows_test.go` whose stated reason is stale, five assertion-free smoke tests in `cmd/nerdgenie`, one crash-only fuzz target, and one generator that skips unless asked. 71 other flagged tests assert through helpers that were confirmed to fail. No TypeScript test lacks an `expect`.
- There are 3,892 Go test and fuzz functions in 749 files and 367 TypeScript test blocks in 30 files; test code outnumbers production code 1.6 to 1 overall (1.7 to 1 in Go, 0.76 to 1 in TypeScript).

## 9. Every command, in order, with its outcome

All run from `/home/jared/Code/coeus` unless the path says otherwise, with `PATH=$PATH:/usr/local/go/bin:$HOME/go/bin` and, for every test run, `NERDGENIE_HEADLESS_TESTS=1`. `$S` is `/tmp/claude-1000/-home-jared/09f06b9f-a0be-401c-831d-adb99bc858bb/scratchpad/testreport`.

| # | Command | Outcome |
|---|---|---|
| 1 | Reading: `docs/WORK_PLAN.md` (Part 1 test sections), `CLAUDE.md` Testing, `Makefile`, `scripts/coverage.sh`, `scripts/stylecheck/main.go`, both `worker/*/package.json`, both `vitest.config.ts`, both `tsconfig.test.json`, `.gitignore`, and the heads of `scripts/gate/fuzz_test.go`, `scripts/release/container_test.go`, `internal/update/liveinstall_integration_test.go`, `test/functional/browserworker_integration_test.go`, `worker/desktop/test/session.test.ts` | Layout learned; the fuzz-script tests and the Chrome-starting tests identified |
| 2 | `go version; staticcheck --version; node --version; npm --version; ps aux | grep -E "llama-server|chrome|nerdgenie"` | Go 1.27.1, staticcheck 2026.2.1, Node 24.18.0, npm 11.16.0; a live `nerdgenie serve`, a `tui` and two Chromes seen, none touched |
| 3 | `docker info` / `podman info` (10 s timeout) | neither answers, so the container test will skip |
| 4 | `go list ./... > $S/packages.txt` | 59 packages |
| 5 | `go test -count=1 -v ./... > $S/unit.log` | exit 1; 58 ok, `scripts/repomap` FAIL; 3749 pass, 1 fail, 8 skip; 86 s |
| 6 | `go vet ./...` | exit 0, no output |
| 7 | `go vet -tags integration ./...` | exit 0, no output |
| 8 | `staticcheck ./...` | exit 0, no output |
| 9 | `go run ./scripts/stylecheck ./...` | exit 0, no output |
| 10 | `go test -count=1 -v -tags integration -skip 'TestTheFuzzScript' ./... > $S/integration.log` | exit 1; 58 ok, `scripts/repomap` FAIL; 3863 pass, 1 fail, 13 skip; 86 s |
| 11 | `sed` the two-line copy of `scripts/coverage.sh` to `$S/coverage-copy.sh`; `diff` confirmed only lines 18 and 42 differ | – |
| 12 | `bash $S/coverage-copy.sh > $S/coverage-table.log` | exit 1 because of `scripts/repomap`; table above; 9 s (served from the Go test cache) |
| 13 | `go test -count=1 -tags integration -skip 'TestTheFuzzScript' -coverprofile=$S/all.out -coverpkg=./internal/...,./cmd/... ./internal/... ./cmd/... ./test/...` | exit 0, 53 packages ok, 85 s |
| 14 | `go tool cover -func=$S/all.out > $S/all-func.txt; tail -1` | `total: (statements) 92.8%` |
| 15 | `python3 scratchpad/phantom/pkgcov.py $S/all.out` | per-package figures from the profile; 25,192 / 27,157 |
| 16 | `cd worker/browser && npm run typecheck` | exit 0 |
| 17 | `cd worker/browser && npx vitest run --coverage --coverage.reportsDirectory=$S/cov-browser` | exit 0; 20 files, 331 tests passed; statements 88.67, branches 81.14, functions 88.70, lines 89.08; 117 s |
| 18 | `cd worker/desktop && npm run typecheck` | exit 0 |
| 19 | `cd worker/desktop && npx vitest run --coverage --coverage.reportsDirectory=$S/cov-desktop` | exit 0; 9 files passed, 1 skipped; 163 tests passed, 12 skipped; statements 78.02, branches 71.28, functions 76.47, lines 77.47; 4.4 s |
| 20 | `grep -rhoE "^func (Test|Fuzz)[A-Za-z0-9_]+" --include='*_test.go'` and the per-area, per-tag variants; `grep -rl "go:build integration"`, `grep -rl "go:build live"`; `find -name '*.golden'`; `grep -rhoE "it\("` / `test\("` over `worker/*/test` | the counts in section 5 |
| 21 | `go build` and run of `scratchpad/phantom/main.go` (modes `all`, `candidates`, `funcs`) over the repository | 3,892 functions parsed, 83 flagged, helper failing calls listed |
| 22 | `node scratchpad/phantom/tsphantom2.mjs` and the corrected Python scan over `worker/*/test` | 367 blocks, 0 without `expect(`, 0 skip/todo |
| 23 | per-golden-file `grep -rl` by basename and stem; the same over the other 124 `testdata/` files; reading `seccomp_test.go:153`, `service_test.go:63-76`, `look_test.go:337`, `gate_test.go:142`, `codexwire_test.go:100`, `skill/folder_test.go` | all 14 golden files referenced; no orphan fixture |
| 24 | `find` into `$S/files.*.txt` and `xargs -d '\n' cat | wc -l` over each list | the line counts in section 7 |

Log files: `unit.log`, `integration.log`, `vet.log`, `vet-integration.log`, `staticcheck.log`, `stylecheck.log`, `coverage-table.log`, `coverage-all.log`, `all.out`, `all-func.txt`, `browser-typecheck.log`, `browser-vitest.log`, `desktop-typecheck.log`, `desktop-vitest.log`, `phantom-all.txt`, `phantom-candidates.txt`, `funcs.txt`, `tsphantom.txt`, all under `$S`.
