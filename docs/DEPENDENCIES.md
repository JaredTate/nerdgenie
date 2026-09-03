# Dependencies

Coeus uses the Go standard library wherever the standard library will do. This
file is the complete list of outside libraries the project is allowed to use,
each with one line saying why the standard library cannot do the job.

**How to use this list.** The eight libraries below are pre-approved. A worker
who first needs one adds the `require` line to `go.mod` in the same branch as the
code that imports it, and needs no further permission. Anything **not** on this
list needs the orchestrator's yes before it is added, and a new row here with its
one-line reason. A library that stops being used is removed from `go.mod` and its
row is struck from this file at the next wave gate.

Each library below is added to `go.mod` by the first wave that imports it.
The wave 1 packages brought in the TOML parser and the SQLite driver, and
`internal/vault` in wave 2 brought in `filippo.io/age` and
`github.com/pquerna/otp`. Everything else built so far uses only the standard
library.

## The pre-approved list

| Library | First needed by | Why the standard library will not do |
|---|---|---|
| `modernc.org/sqlite` | `internal/log`, wave 1 | The one database file is SQLite, and this driver is pure Go, so `bin/coeus` stays a single static binary that needs no C compiler and no system library to build or to run |
| `github.com/BurntSushi/toml` | `internal/config`, wave 1 | The configuration file is `~/.coeus/config.toml`, and the standard library has no TOML parser |
| `filippo.io/age` | `internal/vault`, wave 2 | The vault file and the nightly backups are encrypted with age, and the standard library has no file-encryption format, only the primitives underneath one |
| `github.com/pquerna/otp` | `internal/vault`, wave 2 | Logging in to a site needs a time-based one-time password, and the standard library has no TOTP implementation |
| `github.com/robfig/cron/v3` | `internal/job`, wave 4 | A job's schedule may be a cron expression, and the standard library has no cron parser |
| `github.com/charmbracelet/bubbletea`, with `bubbles` and `lipgloss` | `internal/tui`, wave 3 | The terminal screen streams deltas, answers approvals inline, and redraws on resize, and the standard library has no terminal user-interface toolkit |
| `github.com/coreos/go-systemd/v22/daemon` | `internal/reliability`, wave 4 | The service unit has a watchdog line, and feeding the watchdog means talking to the systemd notify socket, which this library does correctly |
| `github.com/mdp/qrterminal/v3` | `internal/signal`, wave 3 | `coeus signal link` shows the linking URI as a QR code the user scans with a phone, and the standard library cannot draw one |

## The two TypeScript workers

`worker/browser` (wave 5) and `worker/desktop` (wave 6) are the only TypeScript
in the project, and they carry their own `package.json`. Their pre-approved
libraries are `playwright-core` for the browser worker, `@trycua/cua-driver` for
the desktop worker, and `vitest` with `fast-check` for the tests of both. The same
rule applies: anything else needs the orchestrator's yes and a row here.

### `worker/browser`, built

Pinned to exact versions, with no ranges, so that a build a year from now is the
build we tested.

| Package | Version | Why |
|---|---|---|
| `playwright-core` | 1.62.1 | Drives the real Chrome over the DevTools protocol. `playwright-core` rather than `playwright` because the worker attaches to the Chrome already on the machine and must never download a browser of its own |
| `typescript` | 7.0.2 | Compiles `src/` to `dist/` under strict settings |
| `vitest` | 4.1.11 | Runs the tests, including the ones that drive a real Chrome |
| `fast-check` | 4.9.0 | The property tests: any bytes on standard input, any string as an expectation, any sequence of refs |
| `@vitest/coverage-v8` | 4.1.11 | Approved by the orchestrator. Vitest cannot measure coverage without a coverage provider, and brief 5.1 requires `npm test` to fail under seventy percent. Test-time only; nothing it does reaches `dist/` |
| `@types/node` | 24.10.1 | Approved by the orchestrator. TypeScript cannot compile a Node program without Node's type declarations; `process`, `Buffer`, and `setTimeout` have no types otherwise. Types only, erased at build time; nothing it does reaches `dist/` |

### `worker/desktop`, built

Pinned to exact versions for the same reason, and
`worker/desktop/package-lock.json` pins what these libraries depend on in turn.

| Library | Version | Why the standard library will not do |
|---|---|---|
| `@trycua/cua-driver` | 0.23.2 | Driving the machine's own screen, mouse, and keyboard means talking to the display server and reading the accessibility tree the desktop publishes for screen readers, and Node has none of that; this is the driver the brief names, and it is loaded only when a desktop is really going to be driven |
| `typescript` | 5.9.3 | The worker is written in TypeScript and this compiles it, with every strict setting on |
| `vitest` | 4.1.11 | The test runner for the unit, property, and fixture-window tests |
| `@vitest/coverage-v8` | 4.1.11 | Measures the line coverage the seventy percent floor in `docs/WORK_PLAN.md` Part 1 is checked against, and fails `npm test` under it |
| `fast-check` | 4.9.0 | The property tests throw any expectation, any key combination, any accessibility tree, and any bytes on standard input at the worker, and Vitest has no property testing of its own |
| `@types/node` | 26.4.1 | The worker reads standard input, writes standard output, and starts processes, and those types are not in TypeScript itself |
