# Coeus — Agent Instructions

**Coeus** is an open-source AI agent that runs on any Linux machine. You talk to it in a terminal or through Signal. It works with any language model, large or small, local or cloud, and it does not forget what it is doing, because it keeps a task record shaped like an Army operations order instead of re-reading its whole conversation every turn.

## Get up to speed (read in this order)

1. `docs/COEUS_PLAN.md` — the design. What it is, what is new, what we took from other agents, how the loop works, the three kinds of state, what the model is told.
2. `ARCHITECTURE.md` — how the code is put together and what each wave built. Updated every wave.
3. `REPO_MAP.md` — where everything lives. Generated; never edit by hand.
4. `docs/WORK_PLAN.md` — the goal, the rules for building, the four kinds of tests, the test framework, and the waves of briefs. Your brief is in `docs/briefs/wave-N/`, and it begins by telling you to read these same four files.

Deeper references: `docs/HARNESS_V2.md` (the comparison of other agents) · `docs/research/` (seventeen studies with line-level citations into the other code bases) · `docs/reference/` (copies of reference files from projects not on disk) · `THIRD_PARTY.md` (the projects whose designs were ported, and their licenses).

## Hard rules

- **Keep it simple, and build only what is needed now.** The simplest thing that passes the test is the right thing. No abstraction until its second use exists. No configuration option until a real user needs it. No feature because it might be useful later. A package has one job that fits in one sentence in its `doc.go`.
- **Tests first.** Write the test, watch it fail, write the code, watch it pass. The commit history shows the order. Four kinds for every package: unit, integration (build tag `integration`), functional (`test/functional/`), and fuzz (`testing.F` on anything that parses outside text). Coverage above ninety percent, except the terminal screen and the two TypeScript workers, which must be above seventy.
- **Go for the agent, TypeScript for `worker/browser` and `worker/desktop`, nothing else.** Standard library first. Any new dependency needs a one-line reason in `docs/DEPENDENCIES.md` and the orchestrator's yes before it is added.
- **Borrow designs, not code.** Read the reference file your brief names, understand it, write it fresh in Go. Never copy lines. Name the borrowed design and its path in a comment at the top of the file.
- **Plain English.** Identifiers say what they are. Comments are complete sentences. Error messages say what went wrong and what to do. Documents follow the same rule, with technical terms explained on first use. The style checker in `make check` enforces the code half; the orchestrator enforces the document half at every gate.
- **Bound everything.** Every loop has a limit, every wait a timeout, every buffer a cap, every outside call a failure path.
- **Every cross-wave interface lives in `internal/contract`.** Fakes in `internal/testkit` and real implementations are written against the same lines. Never define an interface two packages share anywhere else.
- **Never touch a package another worker owns this wave, and never edit `cmd/coeus/main.go` or `cmd/coeus/serve.go`.** Your brief names your package. A new subcommand goes in its own file under `cmd/coeus/`, a slash command is exported as a `contract.Command` value, and the orchestrator registers both. If you need something from a neighbor, it is already in `contract` or your brief is wrong; stop and report.
- **Nothing irreversible without a preview, nothing secret in the model's context, everything logged.** These are product rules and code rules at once. The permission function, the vault resolver, and the event log exist to enforce them; do not route around them.
- **Keep the docs honest.** When your brief changes a package's job, interface, or dependencies, update the matching section of `ARCHITECTURE.md` in the same branch. After adding or moving files, run `make repo-map`. `make check` fails if either is stale.

## Where the work happens

All building and testing happen on the Linux development machine, `jared-irene`. This repository lives at `/home/jared/Code/coeus`, and every reference project a brief cites lives beside it: `/home/jared/Code/openclaw`, `hermes-agent`, `prime-agent`, `opencode`, `zeroclaw`, and `homerecon`, each already cloned and checked out at the commit listed in `docs/WORK_PLAN.md` wave 0. A path written as `~/Code/x` in any document means `/home/jared/Code/x`. The orchestrator and the workers run there. Nothing is built anywhere else. The sandbox, the service manager, the visible Chrome window, and the local models only exist there.

## Commands

- `make build` — the binary into `bin/coeus` and the worker bundles into `bin/workers/`.
- `make test` — unit tests, integration tests, the functional suite against the fake model, and a five-second fuzz smoke per target.
- `make fuzz` — one minute of fuzzing per target. Runs before every wave gate and nightly.
- `make check` — `go vet`, `staticcheck`, `gofmt`, the style checker, the repo-map drift test, the coverage threshold, and `make test`. CI runs this on every push. A wave does not pass until it is clean.
- `make live` — the functional suite and the forty-step fixture against three real models: the local Qwen through Ollama, Opus 4.8 through Anthropic, and GPT-5.5 through OpenAI. Tagged `live`; development machine only; a missing key is a failure, never a skip. Results go in `docs/PROGRESS.md` with token costs.
- `make release` — binaries for `linux/amd64` and `linux/arm64`, the worker bundles, a checksum file, and a manifest, into `dist/`.
- `make repo-map` — regenerate `REPO_MAP.md`.
- `make install` — build and install the systemd user unit on this machine.

## Testing

- Naming is the contract: `*_test.go` runs everywhere and needs nothing installed · files with `//go:build integration` need the real SQLite file and a temporary home · `test/functional/*_test.go` drives the agent through the same socket the terminal uses, with the fakes from `internal/testkit` · `//go:build live` needs the development machine, the API key, and Ollama.
- Every fake in `internal/testkit` has a contract test that also runs against the real thing under `live`, so fakes cannot drift.
- The forty-step fixture in `test/fixtures/` is the proof that the record and the context builder work on any model. It runs at every wave gate in both tiers and asserts three things: the ask and corrections are byte-for-byte identical at the end, the done-check passes, and every result id is still readable.
- New logic ships with its tests in the same branch. A test written after the code is rewritten.

## The three living documents

`CLAUDE.md` changes rarely and only by the orchestrator. `ARCHITECTURE.md` is updated by any worker whose brief changes a package, and the orchestrator adds a wave section at every gate. `REPO_MAP.md` is generated. These three files are the context every agent starts from, and keeping them true is part of every brief's definition of done.
