# Testing Nerd Genie

This is how Nerd Genie is tested and how you test a change to it. It is written for someone who has never seen the code. The rules come from `CLAUDE.md` and `docs/WORK_PLAN.md` Part 1; when this page and those disagree, they win.

## Tests come first

Write the test, watch it fail, write the code, watch it pass. The commit history shows the order: a commit that adds a test, then a commit that adds the code and makes it pass. A recent pair is `35f6af58 Test: the same test failing for a dozen runs draws a stuck line`, one test file, followed by `138bd4ae Loop: the same tests failing for a dozen runs draws a line naming them and the ways out`, the code. A test written after the code is rewritten: it proves only what the code happens to do.

## The four kinds of tests

Every Go package has all four before it is called done. The two TypeScript workers use Vitest and `fast-check` at the same standard.

| Kind | What it proves | Where it lives |
|---|---|---|
| Unit | One function does what its name says, for normal, edge, and bad inputs | `*_test.go`, table-driven, one test file per source file |
| Integration | Two or more packages work together through their real interfaces, with the real SQLite file and a temporary home | A file with `//go:build integration` on its first line |
| Functional | A whole feature works the way a user sees it: a message goes in, the right reply comes out, the right things happen on disk | `test/functional/` |
| Fuzz | The code survives input it was never designed for | A `testing.F` target on every function that parses text from outside the program |

**Naming is the contract.** The name of a file says what it needs.

- `*_test.go` with no build tag runs everywhere and needs nothing installed. `go test ./...` runs these and only these.
- `//go:build integration` needs the real SQLite file and a temporary home on the real filesystem.
- `test/functional/*_test.go` drives a real `nerdgenie serve` through the same socket the terminal uses, with the fakes from `internal/testkit` behind every model, channel, and outside service.
- `//go:build live` needs the development machine: the llama-server daemon on port 19091 and the two signed-in programs, `claude` and `codex`. There are no API keys. A missing program, a program that is not logged in, or a daemon that is down is a failure, never a skip, and the failure says what to do.

Fuzz targets sit in ordinary test files and are found by name: `FuzzDecodeEvent` in `internal/log`, `FuzzFind` in `internal/repair`. Seed them with the golden cases and the shapes that broke once.

## The fakes and their contract tests

`internal/testkit` holds a fake for everything real: a scripted model, a provider server speaking both wire protocols, a channel, a clock that moves only when a test moves it, a temporary home, the permission decider, the tool registry, the stores, a signal-cli, a search server, a browser worker speaking `worker/browser/PROTOCOL.md`, and a desktop. Every fake implements an interface in `internal/contract`, and every real implementation implements the same one.

Beside each fake is a `Check` function (`CheckModel`, `CheckChannel`, `CheckBrowserWorker`, and so on) that takes the interface rather than the fake and asserts what the contract promises. A unit test calls it on the fake and a live test calls it on the real thing, which is what keeps the two honest. A new fake needs its `Check`, called from both sides.

Every fake is bounded: a script that runs out fails with a clear error rather than blocking, a server has a timeout, and nothing reaches the network except a loopback server the test started.

## The forty-step fixture

`test/fixtures/forty-step/task.json` is a scripted task of forty tool rounds with a correction at step twelve and a stop condition at step thirty. It is the proof that the record and the context builder work on any model, and it asserts three things at the end: the ask and the corrections are byte-for-byte identical, the done-check passes, and every result id is still readable. It runs against the fake model in `make test` and against all three real models in `make live`. Do not edit it casually.

## Golden files

A golden file is the expected output of a test, kept under the package's `testdata/` folder. `testkit.Golden(t, "a_click.txt", output)` compares bytes against `testdata/a_click.txt` and fails with both versions printed when they differ.

To refresh, run the tests with `NERDGENIE_UPDATE_GOLDEN=1`. Read the difference first: a golden file rewritten without being read is a test that proves nothing. A change to a golden file must be explained in the commit message, saying what changed in the output and why the new output is right. A pull request that refreshes goldens with no reason is sent back.

## The gate

`make check` is the gate. It runs, in order: `gofmt`, `go vet`, `staticcheck`, the style checker, the repository-map drift test, the coverage gate, and then `make test`. It must be green before a change is reviewed.

**The coverage gate.** `scripts/coverage.sh` runs every test once with coverage and prints one row per package: covered, needed, result. The threshold is ninety percent of statements per package, and seventy for `internal/tui`, because a terminal screen is tested by a person looking at it. The two workers hold seventy in their `vitest.config.ts` thresholds, so `npm test` fails under it. A package with no test file, or one the report never mentions, is a failure, not a silence. To see the numbers, run `scripts/coverage.sh` on its own; for one package, `go test -coverprofile=cover.out ./internal/record && go tool cover -html=cover.out`.

**The style checker.** `go run ./scripts/stylecheck ./...` enforces the plain-English rule on code: every comment on a declaration is a complete sentence; every exported name has a doc comment; no function runs past sixty lines and no file past five hundred; an error message has at least four lowercase words and no full stop; no identifier is a bare letter or a jargon abbreviation, with `t`, `b`, `f`, `r`, and `w` allowed in tests and request handlers; a file that borrows a design names the reference path at its top; every package has a `doc.go`. Every violation prints as `path:line: rule: what to do`.

**The repo-map drift test.** `REPO_MAP.md` is generated. `go test ./scripts/repomap/...` regenerates it and compares byte for byte. After adding or moving files, `git add` them (the generator lists what git tracks), run `make repo-map`, and commit the result.

## How to run each thing

| Command | What it runs |
|---|---|
| `go test ./...` | Unit tests only. Needs Go 1.27 and nothing else |
| `make test` | Unit and integration tests, the functional suite against the fake model, and a five-second fuzz smoke per target (`scripts/fuzz.sh 5s`) |
| `make check` | The whole gate, then `make test`. Slow; run it before you open a pull request |
| `make fuzz` | One minute of fuzzing per target. Runs before every gate and nightly |
| `make test-browser` | The browser and desktop integration tests with `NERDGENIE_HEADLESS_TESTS=1`, so no Chrome window lands on your screen. `make test` leaves `internal/browser` out for that reason |
| `make live` | `go test -tags live` on the functional suite and `internal/`. Development machine only |
| `cd worker/browser && npm test` | Builds, type-checks, and runs Vitest with coverage. The fixture pages in `test/pages` are served on a loopback port the operating system picks, one file at a time, because two Chrome windows on one display make results depend on timing |
| `cd worker/desktop && npm test` | The same for the desktop worker. Its fixture-window tests drive a real window and run only when `NERDGENIE_LIVE_DESKTOP=1` asks for them by name |

The worker's Chrome is visible by default, which is rule two in `PROTOCOL.md`; `--headless` is passed only by the tests, and only when `NERDGENIE_HEADLESS_TESTS` is set.

## Phantom tests

A phantom test is one that cannot fail. Three shapes: a test with no assertion; a skipped test; and a test that passes whatever the code does, such as one that checks a value it took from the code under test. Reviewers reject all three. The check from `docs/WORK_PLAN.md` Part 4 is to change one thing the test should catch and confirm it fails. Do that yourself before you push.

## Writing a test for a bug

1. Reproduce it first. Write the test that fails the way the bug fails. If you cannot make it fail, you have not found the bug yet.
2. Name the test after the behaviour, in plain words, as a sentence with the spaces taken out: `TestAWriteThatStartsAHeadlessBrowserIsRefused`, `TestAPollWaitsForTheCommandAndHandsBackWhatItDid`, `TestTheRepositoryMapIsNotStale`. In Vitest: `it("marks an element that its markup hides but a style rule draws")`. The name says what is true, not which function is called.
3. Commit the failing test, then the fix, so the history shows the order.
4. If the behaviour is one that `NERDGENIE.md`'s check table names, update the row so the claim points at your test.

## Test data

- Fixtures live beside the package in `testdata/`, under `test/fixtures/` (the site and the forty-step task), or under `worker/browser/test/pages`.
- A test never touches the developer's home. Use the temporary home from `testkit`, built on the real filesystem with the right modes and removed at the end.
- A unit test never reaches the network. Anything that must be served is served from a loopback port the test itself started, and the server serves nothing outside its fixture folder.

## Flaky tests

A race in a test is fixed, not retried. Use the fake clock rather than `time.Sleep`, bound every wait, and count the sleepers before you advance the clock, as the `shell` poll tests do. There is one documented exception: in `worker/browser/test/actions.test.ts`, a page that reloads itself forever races the worker's first read inside the browser, and that one test carries `{ retry: 2 }` with the reason in the comment above it. Any other retry needs the same comment and a reviewer's yes.

## If you run a local model

A local model daemon serves one card and has one slot. Do not run `make fuzz`, `make check`, or the nightly set beside a live run: on 5 September 2026 the kernel killed the daemon under that load and the run fell over to a cloud model silently. Never kill a process by name pattern; kill exact process ids and check liveness by port. Never override the daemon's sampling.

## Checklist before you push

- [ ] The test came first and the history shows it.
- [ ] Every new parser of outside text has a fuzz target.
- [ ] Every test asserts something, none is skipped, and each fails when the code is wrong.
- [ ] `make check` is green and `make fuzz` ran clean.
- [ ] Coverage did not drop under ninety (seventy for `internal/tui` and the workers).
- [ ] Any golden change was read and is explained in the commit.
- [ ] Fixtures are under `testdata`, `test/fixtures`, or `test/pages`; nothing touches a real home or the network.
- [ ] `ARCHITECTURE.md` is true and `make repo-map` was run.
