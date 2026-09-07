# Nerd Genie — Agent Instructions

**Nerd Genie** is an open-source AI agent that runs on any Linux machine. You talk to it in a terminal or through Signal. It works with any language model, large or small, local or cloud, and it does not forget what it is doing, because it keeps a task record shaped like an Army operations order instead of re-reading its whole conversation every turn.

## Get up to speed (read in this order)

1. `NERDGENIE.md` — the plain-words explanation: what Nerd Genie is, how its state works, why it is better than the other agents, and a check table that ties every claim to a design section, a brief, and a test.
2. `docs/NERDGENIE_PLAN.md` — the design. What it is, what is new, what we took from other agents, how the loop works, the four kinds of state, what the model is told.
3. `ARCHITECTURE.md` — how the code is put together today, one page. The build log of what each wave built is `docs/ARCHITECTURE_HISTORY.md`.
4. `REPO_MAP.md` — where everything lives. Generated; never edit by hand.
5. `TESTING.md` and `CONTRIBUTING.md` — how it is tested and how a change is made and reviewed.

Deeper references: `docs/HARNESS_V2.md` (the comparison of other agents) · `docs/research/` (the studies with line-level citations into the other code bases) · `docs/reference/` (copies of reference files from projects not on disk) · `docs/THIRD_PARTY.md` (the projects whose designs were ported, and their licenses) · `PROMPT_TEMPLATE_GUIDE.md` and the `EX_PROMPT_*` files (how to write an ask, with seven worked examples).

## Hard rules

- **Keep it simple, and build only what is needed now.** The simplest thing that passes the test is the right thing. No abstraction until its second use exists. No configuration option until a real user needs it. No feature because it might be useful later. A package has one job that fits in one sentence in its `doc.go`.
- **Tests first.** Write the test, watch it fail, write the code, watch it pass. The commit history shows the order. Four kinds for every package: unit, integration (build tag `integration`), functional (`test/functional/`), and fuzz (`testing.F` on anything that parses outside text). Coverage above ninety percent, except the terminal screen and the two TypeScript workers, which must be above seventy.
- **Go for the agent, TypeScript for `worker/browser` and `worker/desktop`, nothing else.** Standard library first. Any new dependency needs a one-line reason in `docs/DEPENDENCIES.md` and the orchestrator's yes before it is added.
- **Borrow designs, not code.** Read the reference file your brief names, understand it, write it fresh in Go. Never copy lines. Name the borrowed design and its path in a comment at the top of the file.
- **Plain English.** Identifiers say what they are. Comments are complete sentences. Error messages say what went wrong and what to do. Documents follow the same rule, with technical terms explained on first use. The style checker in `make check` enforces the code half, with one carve-out: a doc comment may begin with the lowercase name it documents, as Go's own convention does. The orchestrator enforces the document half at every gate.
- **Bound everything.** Every loop has a limit, every wait a timeout, every buffer a cap, every outside call a failure path.
- **Every cross-wave interface lives in `internal/contract`.** Fakes in `internal/testkit` and real implementations are written against the same lines. Never define an interface two packages share anywhere else.
- **Never touch a package another worker owns this wave, and never edit `cmd/nerdgenie/main.go` or `cmd/nerdgenie/serve.go`.** Your brief names your package. A new subcommand goes in its own file under `cmd/nerdgenie/`, a slash command is exported as a `contract.Command` value, and the orchestrator registers both. If you need something from a neighbor, it is already in `contract` or your brief is wrong; stop and report.
- **Nothing on the user's ask-me-first list without a yes, nothing secret in the model's context, everything logged.** These are product rules and code rules at once. The permission function, the vault resolver, and the event log exist to enforce them; do not route around them.
- **Keep the docs honest.** When your brief changes a package's job, interface, or dependencies, update the matching section of `ARCHITECTURE.md` in the same branch. After adding or moving files, run `make repo-map`. `make check` fails if either is stale.

## Where the work happens

All building and testing happen on one Linux development machine. This repository lives at `~/Code/nerdgenie`, and the reference projects the design borrowed from live beside it when they are needed: `~/Code/openclaw`, `hermes-agent`, `prime-agent`, `opencode`, `zeroclaw`, and `homerecon`. A path written as `~/Code/x` in any document means that folder under the developer's home. The orchestrator and the workers run there. Nothing is built anywhere else. The sandbox, the service manager, the visible Chrome window, and the local model only exist there.

**The local model is not Ollama.** It is the Qwen 3.8 27B Uncensored GGUF at `~/llm/models/hauhau-Q4_K_P.gguf`, served by the TurboQuant `llama-server` on one graphics card, speaking the OpenAI-compatible API at `http://127.0.0.1:19091/v1` under the model name `local-coder` with a context of 131,072 tokens (halved from 262,144 on 6 September 2026 so that the vision projector fits on the card beside the model; at 262,144 with the projector the whole model fell off the card and ran twenty times slower). Sampling is baked into the daemon; never override its temperature. Start it with `setsid ~/llm/igo.sh Vulkan1 19091 131072 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &`; the script adds the projector and the context checkpoints itself (`MMPROJ=none` turns vision off) and trust only `curl -s http://127.0.0.1:19091/health` returning `{"status":"ok"}`. Never run `ollama pull`. Never kill any process by name pattern (`pkill -f`, `killall`, `pgrep -af`): a shell's own command line matches the pattern and dies with it. Kill exact process ids only, and check liveness by port. Port 8090 is reserved for another program on the development machine.

## Commands

- `make build` — the binary into `bin/nerdgenie` and the worker bundles into `bin/workers/`.
- `make test` — unit tests, integration tests, the functional suite against the fake model, and a five-second fuzz smoke per target through `scripts/fuzz.sh`.
- `make fuzz` — one minute of fuzzing per target. Runs before every wave gate and nightly.
- `make check` — `gofmt`, `go vet`, `staticcheck`, the style checker (`scripts/stylecheck`), the repo-map drift test, the coverage gate (`scripts/coverage.sh`: ninety percent per package, seventy for the terminal screen), and `make test`. CI runs this on every push. A wave does not pass until it is clean.
- `make live` — the functional suite and the forty-step fixture against three real models: the local Qwen 3.8 through the llama-server daemon on this machine, Opus 4.8 through `claude -p` on the user's Claude subscription, and GPT-5.5 through `codex exec` on the user's ChatGPT subscription. There are no API keys on this machine and none are wanted. Tagged `live`; development machine only; a program that is missing or not logged in, or a daemon that is down, is a failure, never a skip. Results go in `docs/PROGRESS.md` with token costs.
- `make release` — binaries for `linux/amd64` and `linux/arm64`, each packed with the two worker bundles and a pinned Node runtime so nobody has to install Node, into `dist/` with a `SHA256SUMS` the installer checks against and a `manifest.json` the updater reads. A version tag publishes them as a GitHub release.
- `make repo-map` — regenerate `REPO_MAP.md`.
- `make install` — build, then install the systemd user unit on this machine through `nerdgenie install`, which wave 3 built.

## Testing

- Naming is the contract: `*_test.go` runs everywhere and needs nothing installed · files with `//go:build integration` need the real SQLite file and a temporary home · `test/functional/*_test.go` drives the agent through the same socket the terminal uses, with the fakes from `internal/testkit` · `//go:build live` needs the development machine, the llama-server daemon on port 19091, and the two signed-in command-line programs.
- Every fake in `internal/testkit` has a contract test that also runs against the real thing under `live`, so fakes cannot drift.
- The forty-step fixture in `test/fixtures/` is the proof that the record and the context builder work on any model. It runs at every wave gate in both tiers and asserts three things: the ask and corrections are byte-for-byte identical at the end, the done-check passes, and every result id is still readable.
- New logic ships with its tests in the same branch. A test written after the code is rewritten.

## The three living documents

`docs/PROGRESS.md` is the fourth record: one section per wave with what was built, what the live runs cost, and what the human trial found. Read it to learn what the last wave actually did.

`CLAUDE.md` changes rarely and only by the orchestrator. `ARCHITECTURE.md` is updated by any worker whose change alters how a package works, and stays one readable page; what each wave built goes into `docs/ARCHITECTURE_HISTORY.md`. `REPO_MAP.md` is generated. These three files are the context every agent starts from, and keeping them true is part of every brief's definition of done.
