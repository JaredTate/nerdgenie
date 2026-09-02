# Coeus Architecture

This document records how the code is put together and what each wave built. It is read by every worker before starting a brief and updated by any worker whose brief changes a package's job, its interface, or its dependencies. The orchestrator adds a wave section at the end of every wave. The design this code implements is `docs/COEUS_PLAN.md`; when the two disagree, the design is the intent and this document is the fact, and the orchestrator reconciles them at the wave gate.

Nothing is built yet. Everything below the "Shape" section describes what will exist once its wave is done, and is marked planned until then.

## Shape

One Go program owns everything: the message queue, the turn loop, the permission function, the task record, the memory, the scheduled jobs, and one SQLite database file. It starts two helper programs when it needs them, a browser worker and a desktop worker, both written in TypeScript, both speaking JSON-RPC over standard input and output. Two thin screens, the terminal and Signal, attach to the program over a local socket and own no state.

```
terminal ──┐                                   ┌── browser worker (TypeScript, Playwright, real Chrome)
           ├── local socket ──▶ coeus (Go) ────┤
signal ────┘   (JSON lines)        │           └── desktop worker (TypeScript, cua-driver)
                                   │
                              one SQLite file: event log, task and job records, memory index
                              plus markdown files: persona, memory, skills
```

## Packages and their one-sentence jobs

Packages are listed in build order, and a package may import only packages listed above it. Anything two packages both need is in `internal/contract`.

| Package | Job | Wave |
|---|---|---|
| `internal/contract` | Every interface, type, and constant that crosses a wave boundary, with no dependencies | 0 |
| `internal/testkit` | Every fake, the golden-file helper, and the forty-step fixture data | 0 |
| `internal/log` | The append-only event log in SQLite | 1 |
| `internal/record` | The task record: parse, print, enforce its rules, checkpoint | 1 |
| `internal/config` | The configuration file and the home folder layout | 1 |
| `internal/lint` | The plain-English style checker, used only by `make check` | 0 |
| `internal/provider` | Turn a prompt into a streamed reply through the Anthropic or OpenAI-compatible API, with retries and the fallback chain | 1 |
| `internal/repair` | Find the tool calls in a model reply, however the model wrote them | 1 |
| `internal/context` | Build the working context from the layers, sized to the model | 2 |
| `internal/loop` | Run one turn: orient, call, guard, permit, run, update, repeat; the done-check and the after-action review | 3 |
| `internal/tool` | The tool registry and the built-in tools, one folder each | 2 |
| `internal/permission` | Decide allow, ask, or deny for a tool call | 2 |
| `internal/sandbox` | Run a command inside bwrap and Landlock | 2 |
| `internal/channel` | The queue, the event stream, and the local socket | 3 |
| `internal/command` | The command registry and the core slash commands | 3 |
| `internal/tui` | The terminal screen | 3 |
| `internal/signal` | The signal-cli client, linking, pairing, and the Signal channel | 3 |
| `internal/vault` | The encrypted secret store, the resolver, TOTP, the sudo password, redaction | 2 |
| `internal/reliability` | Leases, ledgers, sentinels, the breaker, the watchdog feed, backups | 4 |
| `internal/memory` | The memory files, the search index, the hint, and zero-token capture | 4 |
| `internal/skill` | The skill folder format, loading, learning, replay | 4 |
| `internal/browser` | The Go side of the browser: worker lifecycle, login, handoff | 5 |
| `internal/job` | Job records, the task list, and the scheduler | 4 |
| `internal/desktop` | The Go side of the desktop worker | 6 |
| `internal/update` | Update, rollback, migrations | 6 |
| `internal/replay` | Re-run any logged task as a test | 6 |
| `cmd/coeus` | The binary; one file per subcommand; `main.go` and `serve.go` are the orchestrator's | 3 onward |
| `worker/browser` | The TypeScript browser worker | 5 |
| `worker/desktop` | The TypeScript desktop worker | 6 |

## The contracts (planned, wave 0)

`internal/contract` holds the interfaces every fake and every real implementation share. Their full definitions live in the code with doc comments; this is the list.

- `Model`: send a prompt, stream text and tool calls back, report tokens in, out, and cached.
- `Tool`: name, description under forty words, typed input fields, permission class, run function returning text.
- `Channel`: receive, send, send a file, preview and collect a decision, masked prompt, health.
- `Permission`: decide allow, ask, or deny; the answers once, always for the session, reject with a reason.
- `Memory`: search, get, save, hint.
- `Command`: name, help line, run function; each package exports its slash commands as values and `serve.go` registers them.
- `Skill`: list, load, run, save.
- `Job`: create, add a task, list, run now, pause, switch off.
- `Sandbox`: run a command inside the fence.
- `Secrets`: resolve a reference, get the sudo password, redact text.
- `BrowserWorker`: the methods in `worker/browser/PROTOCOL.md`.
- `Desktop`: launch, screenshot, click, type, key, drag, clipboard.
- `Clock`: now, sleep, ticker.
- `Store`: the event log's write and read shapes.
- The record types, the configuration struct with defaults, and the exit codes (75 restart me, 78 bad configuration).
- The user-tool protocol: an executable in `~/.coeus/tools/` answers `--describe` with JSON and takes a JSON object on standard input.

## The local socket (planned, wave 3)

The terminal and any future screen attach to the running program over a Unix socket at `~/.coeus/run/coeus.sock`, speaking newline-delimited JSON. Message types from the screen: `message`, `command`, `approve`, `deny`, `secret` (for the masked prompt), `attach`, `detach`. Message types from the program: `delta`, `reply`, `preview`, `ask`, `handoff`, `status`, `error`. The socket is itself a channel. The Signal channel does not use it; it runs inside the program and feeds the same queue.

## The browser worker protocol (planned, wave 0 document, wave 5 code)

`worker/browser/PROTOCOL.md` defines the JSON-RPC methods the Go side calls: `open`, `read`, `click`, `type`, `press`, `scroll`, `act`, `tabs`, `loginFill`, `screenshot`, and `health`, and the snapshot and diff shapes every method returns. The fake worker in `testkit` and the real worker implement the same document.

## Data on disk (planned)

```
~/.coeus/
  config.toml            read once at startup
  coeus.db               the one SQLite file
  persona/               SOUL.md, USER.md, MEMORY.md
  memory/                markdown files, indexed
  skills/<name>/         SKILL.md, steps or script, test, changelog
  tools/                 the user's own tools, one executable each
  vault.age, vault.key   the secret store and its key, mode 0600
  browser/<profile>/     Chrome user data, mode 0700, outside the sandbox
  inbox/                 files and photos received over Signal
  releases/<version>/    installed binaries; `current` is a symlink
  run/                   the socket and the lock
  backups/               nightly encrypted archives
```

## Test architecture

Four kinds of tests, described in `docs/WORK_PLAN.md`: unit, integration, functional, fuzz. Two tiers of model: the scripted fake on every commit, and three real models (the local Qwen 3.8 through the llama-server daemon on the development machine, Opus 4.8 through Anthropic, and GPT-5.5 through OpenAI) at every wave gate under the `live` tag. Every fake has a contract test against the real thing. The forty-step fixture is the proof of the record and the context builder.

## Repository map

`REPO_MAP.md` is generated from the tree by `make repo-map`. The generator omits dependency folders, build outputs, test artifacts, and version-control internals. A drift test in `make check` fails when the map is stale.

## Wave log

Each wave gate adds a section here: what was built, what changed in the interfaces, what the next wave depends on. Three paragraphs at most.

### Wave 0 (planned)

The development machine readied, the skeleton, the contracts, the browser protocol document, the repo-map generator, and the first versions of the three living documents.
