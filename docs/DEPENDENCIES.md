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

Each library below is added to `go.mod` by the first wave that imports it; the wave 1 packages brought in the TOML parser and the SQLite driver.
uses only the standard library.

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
