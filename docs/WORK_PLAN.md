# COEUS WORK PLAN

**How Coeus gets built, from an empty folder to a working agent, in waves of five workers, with every test written before the code it proves.**

This is the plan the orchestrator follows. The design it builds is in `COEUS_PLAN.md`. Read that first. The comparison behind the design is in `HARNESS_V2.md`, and the reasons behind every borrowed idea are in `docs/research/`. This plan was written on a Mac and reviewed by an adversarial pass that found 24 distinct blockers; all of them are fixed in this version and listed at the end.

---

## Part 1: The rules

These rules apply to every wave, every worker, and every line of code. A worker who cannot follow one of them stops and reports instead of working around it.

### Where the work happens

All building and all testing happen on one Linux development machine, `jared-rosie`, where this repository lives at `/home/jared/Code/coeus` and every reference project lives beside it under `/home/jared/Code/`. The orchestrator runs there. The workers run there. The tests run there, because the sandbox, the service manager, the visible Chrome window, and the local models only exist on Linux. The machine that wrote this plan does not build anything. Wave 0 makes the development machine ready.

### Who does what

The **orchestrator** is a Fable 5.1 model. It reads this plan, writes the brief for each worker, checks each worker's result against the brief, runs the full test suite after every wave, and decides whether the wave passed. It never writes feature code itself.

A **worker** is an Opus 4.8 model running at extra-high effort. Each worker gets one brief, one package, and one wave. A worker reads the reference code it is pointed at, writes the tests, writes the code that passes them, and reports back with the test output. Five workers run at once. A worker never touches a package that another worker in the same wave owns.

### The three living documents

Three files at the root of the repository are context for every worker and must be kept true. They follow the pattern of the HomeRecon project, where the same three files have kept a much larger codebase legible to agents for months.

- **`CLAUDE.md`** is the first thing any agent reads. It holds the reading order, the hard rules, and the commands. It changes rarely and only by the orchestrator.
- **`ARCHITECTURE.md`** records how the system is put together and what each wave built. Every worker whose brief changes a package's job, its interface, or its dependencies updates the matching section in the same branch. The orchestrator adds a "Wave N" section at the end of every wave, three paragraphs at most.
- **`REPO_MAP.md`** is generated from the tree by `make repo-map` and never edited by hand. A test fails if it is out of date. Every worker regenerates it after adding or moving files.

A worker's brief is not done until `ARCHITECTURE.md` is true for that package and `REPO_MAP.md` is regenerated.

### The seven rules of the code

1. **Keep it simple.** The simplest thing that passes the test is the right thing. No abstraction is added until the second use of it exists. No configuration option is added until a real user needs it.
2. **Tests come first.** A worker writes the test, watches it fail, then writes the code, then watches it pass. Code without a test does not merge. The commit history shows the order, and the orchestrator checks it.
3. **Go for the agent, TypeScript for the browser and desktop workers, nothing else.** The Go code uses the standard library wherever the standard library will do. Every outside library is listed in `docs/DEPENDENCIES.md` with a one-line reason, and a worker who wants to add one asks the orchestrator first. The starting list is in wave 0.
4. **One package, one job.** Each Go package has one purpose that fits in one sentence at the top of its `doc.go` file. A package that needs two sentences is two packages.
5. **Plain English everywhere.** Every identifier says what it is. Every comment is a complete sentence. Every error message tells the reader what went wrong and what to do about it. Every log line can be read by someone who has never seen the code.
6. **Bound everything.** Every loop has a limit. Every wait has a timeout. Every buffer has a cap. Every outside call can fail, and the code says what happens when it does.
7. **Borrow designs, not code.** The other agents are written in TypeScript, Python, and Rust. A worker reads the reference file it is pointed at, understands the design, and writes it fresh in Go. Nothing is copied line by line. The borrowed design is named in a comment at the top of the Go file, with the path to the reference, and the reference project's license is listed in `THIRD_PARTY.md`.

### The four kinds of tests

Every Go package has all four before it is called done. The two TypeScript workers have the equivalents named in their briefs.

| Kind | What it proves | How it is written |
|---|---|---|
| **Unit test** | One function does what its name says, for normal inputs, edge inputs, and bad inputs | Standard Go `testing`, table-driven. One test file per source file |
| **Integration test** | Two or more packages work together through their real interfaces, with the real database and the real filesystem in a temporary folder | In the package that sits highest in the dependency order, marked with the build tag `integration` |
| **Functional test** | A whole feature works the way a user would see it: a message goes in, the right reply comes out, the right things happen on disk | In `test/functional/`, driven through the same socket the terminal uses, with a fake model and fake outside services |
| **Fuzz test** | The code survives input it was never designed for | Go's built-in `testing.F`, on every function that parses text from outside the program |

### Two tiers of model testing

The fake model proves the harness logic. It does not prove the agent works with a real model, so there is a second tier.

- **Tier one, every commit:** the scripted fake model from the test framework. Fast, deterministic, runs in CI.
- **Tier two, every wave gate:** the same functional tests run against two real models. One is Opus 4.8 through the Anthropic API, for hard tasks like browser flows. The other is the local Qwen 3.8 model served by Ollama on the development machine, for the small-model path. These tests are tagged `live`, run only on the development machine, and their results are recorded in `docs/PROGRESS.md` with the token cost of each. A wave does not pass until the tier-two run is green on both models for every feature that wave added.

### The test framework

The framework is built in wave 1, before any feature, and every later test uses it. It lives in `internal/testkit/` and provides exactly these things. Every fake implements an interface that is defined one wave before the fake is built, so the fake and the real thing are always written against the same contract.

- **A fake model.** It is scripted: a test gives it a list of replies, and it returns them in order. It can emit tool calls, malformed tool calls, tool calls written as text, empty replies, and replies with an unknown finish reason. It records every prompt it was sent and reports token counts.
- **A fake model provider server.** It speaks both wire protocols over a local socket, so the real client code is exercised, and it can be told to fail, stall, rate-limit, or return garbage.
- **A fake channel.** It implements the channel interface from wave 0. A test pushes messages in and reads replies, previews, and handoffs out.
- **A fake clock.** Every package takes a clock as an input and never calls the real time directly. A test moves the clock by hand.
- **A temporary home.** A fresh, empty Coeus home folder with a fresh database, created for each test and deleted after it.
- **A fake permission decider.** It answers allow, ask, or deny from a script, so any test can exercise the permission path without a human.
- **A fake tool registry.** It holds scripted tools that return fixed results, so the loop can be tested before any real tool exists.
- **A fake memory store.** An in-memory implementation of the memory interface from wave 0.
- **A fake signal-cli, a fake search server, and a fake browser worker.** Each speaks the real wire protocol over a local socket. The browser worker's protocol is written in wave 0 as `worker/browser/PROTOCOL.md`, so the fake and the real worker follow the same document.
- **Golden files.** A helper that compares a result to a saved file in `testdata/` and rewrites the saved file when a flag is set.
- **A recorded-task replayer.** It takes a task from the event log and re-runs it against the current code with the recorded tool results.
- **The forty-step fixture.** A scripted task with forty tool rounds, a correction at step twelve, a stop condition at step thirty, and three assertions at the end: the ask and the corrections are byte-for-byte identical, the done-check passes, and every result id is still readable. This fixture is the proof that the record and the context builder work on any model, and it runs at every wave gate in both tiers.

### How a wave runs

1. The orchestrator writes five briefs, one per worker, and puts them in `docs/briefs/wave-N/`. A brief names the package, the job in one paragraph, the interface it must implement if one already exists, the reference files to read with their paths, the tests that must exist, and the definition of done.
2. The five workers run at once, each in its own branch, each on the development machine.
3. Each worker reports with the test output, the list of files, the line count, the `ARCHITECTURE.md` section it updated, and any question it could not answer.
4. The orchestrator merges the five branches, runs `make check`, reads every new file, and runs the tier-two live tests for the wave's features.
5. If everything passes, the orchestrator adds the wave section to `ARCHITECTURE.md`, writes three lines in `docs/PROGRESS.md`, and starts the next wave. If anything fails, it writes a fix brief for one worker and does not start the next wave until the fix passes.
6. **Someone tries it.** Every wave from wave 4 onward ends with a human using the agent for ten minutes and writing down what was confusing. That note goes in `docs/PROGRESS.md` and becomes briefs in the next wave.

### Definition of done, for every package

- Every exported function has a unit test with normal, edge, and bad inputs.
- Every function that parses outside text has a fuzz test that has run for at least one minute without a failure.
- The integration test passes against the real database in a temporary home.
- `make check` reports nothing: `go vet`, `staticcheck`, `gofmt`, the style checker, and the repo-map drift test.
- The package's `doc.go` has a one-sentence purpose and a paragraph a new reader could understand.
- No function is longer than sixty lines. No file is longer than five hundred lines.
- The borrowed design, if any, is named at the top of the file with the reference path.
- Test coverage of the package is above ninety percent by line, except the terminal screen and the two TypeScript workers, which must be above seventy percent, because screen code is tested by looking at it.
- `ARCHITECTURE.md` is true for the package, and `REPO_MAP.md` is regenerated.

---

## Part 2: The layout, and the interfaces that come first

```
coeus/
  CLAUDE.md               the reading order, the hard rules, the commands
  ARCHITECTURE.md         how the system is put together, updated every wave
  REPO_MAP.md             generated from the tree, never edited by hand
  THIRD_PARTY.md          the projects whose designs were ported, with their licenses
  Makefile                build, test, check, live, repo-map, install
  go.mod                  the module and the pinned Go version
  cmd/coeus/              the one binary and its subcommands
  internal/
    contract/             wave 0: every interface that crosses a wave boundary
    testkit/              wave 1: the test framework
    log/                  wave 1: the event log
    record/               wave 1: the task record
    config/               wave 1: configuration and the home folder
    provider/             wave 1: the model providers
    repair/               wave 1: tool-call repair
    context/              wave 2: the working context builder
    loop/                 wave 2: the turn loop and the guard
    review/               wave 2: the done-check and the after-action review
    tool/                 wave 2: the built-in tools, one folder each
    permission/           wave 2: the permission function
    sandbox/              wave 3: bwrap and Landlock
    channel/              wave 3: the queue, the event stream, the local socket
    command/              wave 3: the command table
    tui/                  wave 4: the terminal screen
    signal/               wave 4: the signal-cli client, linking, pairing
    vault/                wave 4: the encrypted secret store
    reliability/          wave 4: leases, ledgers, sentinels, the breaker
    memory/               wave 5: the memory files, the index, capture
    skill/                wave 5: the skill folder, learning, replay
    browser/              wave 6: the Go side of the browser
    schedule/             wave 7: scheduled jobs
    desktop/              wave 7: the Go side of the desktop worker
    update/               wave 7: install, update, backup, restore
    replay/               wave 7: any task becomes a test
    lint/                 wave 1: the style checker
  worker/browser/         wave 6: the TypeScript browser worker; PROTOCOL.md from wave 0
  worker/desktop/         wave 7: the TypeScript desktop worker
  test/functional/        whole-feature tests
  test/fixtures/          recorded pages, the forty-step task, golden files
  scripts/                the installer, the repo-map generator, CI helpers
  docs/                   the plan, the design, the research, the references, the briefs
```

Packages import only downward in this list. The orchestrator rejects any import that goes the wrong way. Nothing in the same wave imports a neighbor; anything two workers both need is in `internal/contract`, written in wave 0.

### `internal/contract`: written first, changed rarely

Every interface that crosses a wave boundary lives in one package with no dependencies, so that a fake in wave 1 and a real implementation in wave 4 are written against the same lines. It holds:

- `Model`: send a prompt, stream back text and tool calls, report tokens in, out, and cached.
- `Tool`: name, description, input fields with types, permission class, and a run function. The permission classes are read, write, execute, network, and irreversible.
- `Channel`: receive, send, send a file, preview and collect a decision, masked prompt, health.
- `Permission`: decide allow, ask, or deny for a tool call, with the three answers once, always for the session, and reject with a reason.
- `Memory`: search, get, save, and the three-line hint.
- `Clock`: now, sleep, and a ticker.
- `Store`: the event log's write and read shapes.
- The record's Go types.
- The configuration struct, with every field and its default.
- The exit codes: 75 means restart me, 78 means bad configuration, stop.
- The user-tool protocol: a user's own tool is any executable in `~/.coeus/tools/`; run with `--describe` it prints its name, description, input fields, and permission class as JSON; run with a JSON object on standard input it prints its text result on standard output. That is the whole protocol.

---

## Part 3: The waves

Each wave lists five briefs. Each brief has the package, the job, the reference files to read, the tests that must exist, and what done means. Reference paths that start with `~/Code/` mean `/home/jared/Code/` on the development machine, where every reference project is cloned at the exact commit the research read. Wave 0 makes those clones. Paths that start with `docs/reference/` are copies kept in this repository because the original was cloned only for the research.

### Wave 0: Ready to build

One worker, not five, because everything else depends on it. After this wave, `make check` passes on an empty codebase and every interface exists.

**Brief 0.1: the development machine, the skeleton, and the contracts.** Install Go and signal-cli on the development machine, confirm bwrap, ripgrep, Chrome, Node, and Ollama are present, and confirm Landlock is enabled in the kernel. Clone the five reference projects into `/home/jared/Code/` at the exact commits the research read, because every brief cites file paths and line numbers from those commits:

| Project | Clone from | Commit |
|---|---|---|
| OpenClaw | `https://github.com/JaredTate/openclaw.git` | `752a983480e4` |
| Hermes | `https://github.com/JaredTate/hermes-agent.git` | `95f62ca3bfcf` |
| Prime | `https://github.com/JaredTate/prime-agent.git` | `0ba0423c5c18` |
| OpenCode | `https://github.com/JaredTate/opencode.git` | `69c172e8a7c0` |
| ZeroClaw | `https://github.com/JaredTate/zeroclaw.git` | `73ff732e66c7` |

The OpenClaw clone already on the machine is a month behind that commit and must be moved to that commit. HomeRecon is already there and current. Create `go.mod` with the module path and a pinned Go version, a `Makefile` with `build`, `test`, `check`, `live`, `repo-map`, and `install` targets, a GitHub Actions workflow that runs `make check` on every push on a Linux runner with bwrap installed, `THIRD_PARTY.md` listing OpenClaw, Hermes, Prime, OpenCode, ZeroClaw, browser-use, Moltis, and Codex with their licenses and the note that designs were ported and the files in `docs/reference/` are verbatim copies under their own licenses, a `LICENSE` file, `docs/DEPENDENCIES.md` with the starting list (the pure-Go SQLite driver, the age encryption library, the TOTP library, the cron parser, Bubble Tea, and the systemd notify helper), the whole of `internal/contract`, `worker/browser/PROTOCOL.md` describing the JSON-RPC methods the browser worker will speak over standard input and output (open, read, click, type, press, scroll, act, tabs, login-fill, and the snapshot and diff shapes), the repo-map generator in `scripts/` with its drift test, and the first versions of `CLAUDE.md`, `ARCHITECTURE.md`, and `REPO_MAP.md`. Reference: the HomeRecon repository's `CLAUDE.md`, `ARCHITECTURE.md`, and `scripts/repo-map/generate-repo-map.mjs` for the shape of the three living documents and the drift test. Tests: `make check` passes; the drift test fails when a file is added without regenerating the map; every interface in `contract` compiles and has a doc comment.

### Wave 1: The foundation

After this wave, the test framework exists, the event log and the record work, and the agent can talk to a real model provider through a fake server.

**Brief 1.1: the test framework.** Package `internal/testkit`. Every fake listed under "The test framework" above, each implementing its `contract` interface, each with its own tests, plus the golden-file helper, the replayer, and the forty-step fixture in `test/fixtures/`. Done when a sample test in `test/functional/` pushes a message through the fake channel, gets a scripted reply from the fake model, and asserts on both in under one second.

**Brief 1.2: the event log.** Package `internal/log`. An append-only table in SQLite through the pure-Go driver, holding every message, tool call, tool result, permission decision, and record change, each with a sequence number, a timestamp, a task id, and a kind. Write is one insert. Read is by task, by kind, by id, or by range. A replay function hands events back in order. The database opens in write-ahead mode with one writer. Reference: Hermes' delivery ledger at `~/Code/hermes-agent/gateway/delivery_ledger.py` for "write the obligation before acting," and OpenClaw's persisted queue at `~/Code/openclaw/src/infra/outbound/delivery-queue-storage.ts`. Tests: every read shape; ten thousand events replayed in order; a process killed mid-write leaves an intact log; a fuzz test on the event decoder.

**Brief 1.3: the task record.** Package `internal/record`. The record format from section 4 of the design as Go types from `contract`, with a parser and a printer that round-trip exactly. The rules are code: the ask, intent, and corrections are immutable after creation; a fact must carry a source; a failure must carry a cause; a step marked done must name a result; the record folds finished steps and old results into single lines past its cap of three thousand tokens; checkpoints save to the event log on every change and reload by number. Reference: Prime's goal state at `~/Code/prime-agent/packages/coding-agent/src/core/goals.ts` and its state with rollback at `packages/coding-agent/src/core/refinement/refinement.ts`; the example record in section 4 of the design is the golden file. Tests: round-trip on the golden record; each rule rejects a bad edit; the forty-step fixture from testkit passes; rewind; a fuzz test on the parser.

**Brief 1.4: configuration, the home folder, and the style checker.** Packages `internal/config` and `internal/lint`. Configuration is one file, `~/.coeus/config.toml`, read once at startup into the `contract` struct, with every field and default written in the struct's doc comments: the model alias and its provider, base address, and key reference; the fallback chain; the Signal account and the paired list location; the browser profile path; the sandbox roots; the backup path; and the caps from the design. Any wrong value produces an error naming the line. The home folder layout is created by `init` in wave 3; this brief defines it and provides a `doctor` function that reports what exists and never changes anything. The style checker reads a Go tree and reports exported identifiers under three characters, comments that are not complete sentences, functions over sixty lines, files over five hundred lines, and error strings that do not say what to do; it is wired into `make check`. Tests: defaults from an empty file; every field with a wrong type; the doctor on an empty home and a full one; a fixture tree with one style violation of each kind; a fuzz test on the parser.

**Brief 1.5: the model providers and tool-call repair.** Packages `internal/provider` and `internal/repair`. Provider: two implementations of `contract.Model`, the Anthropic API and the OpenAI-compatible API with a base address, with cache markers placed where section 4 of the design says, retries of three with backoff honoring the retry-after header and never on context overflow, and a fallback chain from the configuration. Repair: takes the raw text of a reply and returns the tool calls in it whether they arrived structured, as JSON in a code fence, as a `<tool_call>` block, or as a near-miss name; two failed parses in a row mean the text is the answer. Reference: Prime's provider types and two API files at `~/Code/prime-agent/packages/ai/src/types.ts`, `providers/anthropic.ts`, and `providers/openai-completions.ts`, and its overflow detection at `packages/ai/src/utils/overflow.ts`; OpenCode's retry policy at `~/Code/opencode/packages/opencode/src/session/retry.ts`; Hermes' local-model rules at `~/Code/hermes-agent/plugins/model-providers/custom/__init__.py` and `agent/model_metadata.py`; ZeroClaw's parser at `~/Code/zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs`; OpenClaw's grammar at `~/Code/openclaw/packages/tool-call-repair/src/grammar.ts`; OpenCode's invalid-call path at `~/Code/opencode/packages/opencode/src/tool/invalid.ts`. Tests: both APIs against the fake provider server, including streamed tool calls, a stalled stream, a rate limit with retry-after, a server error, and an overflow; the fallback chain moves on; a golden file per envelope shape; name repair; the two-failures rule; fuzz tests on both parsers.

### Wave 2: The loop, the tools, and the permissions

After this wave, the agent runs a full turn through the fake channel with real tools on real files.

**Brief 2.1: the working context builder.** Package `internal/context`. The layered prompt from section 4 of the design: harness rules, persona, tools, the stable part of the record, the live part, pinned evidence, recent messages, the memory hint. The model instruction text from section 5 of the design is a constant in this package, and a test asserts it is under three hundred words. Window sizes come from the model's context length. Tiered folding moves the oldest unpinned exchange into the record as one line. Nothing above the cache line changes during a task. The cost line is written every turn. Reference: Hermes' tiers and cache points at `~/Code/hermes-agent/agent/prompt_caching.py` and `agent/prompt_cache_boundary.py`; OpenClaw's cache boundary in `~/Code/openclaw/src/agents/system-prompt.ts` (search for `SYSTEM_PROMPT_CACHE_BOUNDARY`); Hermes' verbatim user messages in `~/Code/hermes-agent/agent/context_compressor.py`; OpenCode's re-read of instruction files at `~/Code/opencode/packages/opencode/src/session/instruction.ts`. Tests: a golden prompt for a 24k model and a 200k model from the same task; nothing above the cache line changes across ten turns; folding keeps every exchange readable by id; the cost line matches the fake provider's counts.

**Brief 2.2: the turn loop, the guard, and the review.** Packages `internal/loop` and `internal/review`. The loop from section 3 of the design with the ten rules as code, against the `contract` interfaces so every dependency is a fake in tests: the identical-call detector across steps with a window of twenty, the malformed-call path through `repair`, the round cap with the final tools-off call, the stop conditions, the taint flag, a mid-turn message written as a correction, a question that ends the turn waiting. The review package: the done-check that refuses to close on any false line, then the four after-action questions, with the fourth answer handed to `contract.Memory` and, when it describes a procedure, offered as a skill. Reference: OpenClaw's loop at `~/Code/openclaw/packages/agent-core/src/agent-loop.ts` and `types.ts`; ZeroClaw's turn engine at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/agent/turn/mod.rs` and its loop detector at `agent/loop_detector.rs`; OpenClaw's retry refund at `~/Code/openclaw/src/agents/embedded-agent-runner/run/retry-budget.ts`; OpenCode's detector at `~/Code/opencode/packages/opencode/src/session/processor.ts`; Prime's verifier gates in `~/Code/prime-agent/packages/coding-agent/src/core/autonomous.ts`. Tests: a scripted conversation per rule; the detector across steps and inside one response; the cap; suspend and resume; a done-check that fails on one line; the review hands off the fourth answer; a fuzz test on the reply handler.

**Brief 2.3: the tool registry and the file tools.** Packages `internal/tool` and `internal/tool/file`. The registry loads built-in tools and any executable in `~/.coeus/tools/` through the user-tool protocol from `contract`, rejects a description over forty words, and applies the output cap of four thousand characters on local models and sixteen thousand on cloud, spilling the rest to a file named in the result. The file tools: `read` with offsets, line numbers, and a continue hint; `write`; `edit` with exact match and the fallback matchers; `search` on ripgrep with a fifty-row cap. `write` and `edit` refuse any path outside the configured roots, and the roots never include the vault, the browser profile, or `~/.ssh`. Reference: OpenCode's registry at `~/Code/opencode/packages/opencode/src/tool/registry.ts`, its descriptions in the `.txt` files beside each tool, its truncation at `tool/truncate.ts`, and `tool/read.ts`, `tool/edit.ts`, `tool/grep.ts`, `tool/glob.ts`; Hermes' single search tool in `~/Code/hermes-agent/tools/file_tools.py`. Tests: a user tool in a folder registers and runs; a long description is rejected; overflow spills; golden results per tool; every edit fallback; a write outside the roots is refused; a fuzz test on the edit matcher.

**Brief 2.4: the permission function.** Package `internal/permission`. The `contract.Permission` implementation: rules of tool, pattern, and action from the policy file, last match wins, default ask; shell commands reduced to a readable prefix; the three answers; irreversible actions always produce a preview; a tainted turn cannot pass an irreversible action; standing approvals attached to skills with a limit and an expiry; unattended runs fail closed and report. Reference: OpenCode's engine at `~/Code/opencode/packages/opencode/src/permission/index.ts`, its rule format at `packages/core/src/v1/config/permission.ts`, and its prefix table at `permission/arity.ts`; Hermes' unattended rule in `~/Code/hermes-agent/tools/approval.py`. Tests: a table of rules and calls; prefix reduction; the taint rule; an expired standing approval; an unattended ask fails closed; a fuzz test on the pattern matcher.

**Brief 2.5: the task, memory, skill, schedule, and web tools.** Package `internal/tool/` with one folder each. `task` updates the plan, adds a fact with its source, records a decision or failure with its reason, and pins a result, and it is the only way the model writes the record. `memory` and `skill` and `schedule` are thin tools over the `contract` interfaces, so they work against fakes now and real packages later. `web` does search and fetch-as-text with a pinned DNS lookup and a block on private address ranges, and wraps fetched text in random-id boundaries. The search provider is a self-hosted SearXNG instance, chosen because it needs no key; the address is a configuration field and the fake search server in testkit stands in for it. Reference: OpenClaw's pinned lookup at `~/Code/openclaw/src/infra/net/ssrf.ts` and its wrapper at `src/security/external-content.ts`; Hermes' web extraction in `~/Code/hermes-agent/tools/web_tools.py`. Tests: every `task` operation and every record rule it must respect; the thin tools round-trip through fakes; a fetch of a fixture page; a redirect to a private address is refused; a fuzz test on the HTML-to-text step.

### Wave 3: The sandbox, the channel, and the first commands

After this wave, the agent runs commands safely and has a socket a screen can attach to.

**Brief 3.1: the sandbox.** Package `internal/sandbox`. Wraps a command in bwrap with a Landlock ruleset and a seccomp filter, keeps the vault, the browser profile, and `~/.ssh` outside, allows network, and reports itself unavailable if bwrap is missing. Reference: ZeroClaw's wrapper at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/security/landlock.rs` and its trait at `security/traits.rs`; Codex's rules at `docs/reference/codex/landlock.rs`. Tests: a sandboxed read of `~/.ssh` fails; a sandboxed network call succeeds; the package reports unavailable without bwrap.

**Brief 3.2: the shell tool.** Package `internal/tool/shell`. Runs a command through the sandbox in its own process group with a timeout; after ten seconds returns a job id the model can poll, tail, or kill; the `escalate` field with a reason produces a preview through the permission function and, on approval, runs outside the sandbox with `sudo -A` and the vault's sudo entry through `SUDO_ASKPASS`. Reference: OpenClaw's yield-then-background at `~/Code/openclaw/src/agents/bash-tools.schemas.ts`; Codex's escalation field at `docs/reference/codex/shell_spec.rs`; Hermes' sudo path in `~/Code/hermes-agent/tools/terminal_tool.py` (search for `sudo_password`). Tests: a one-second command; a thirty-second command polled and killed; an escalation that produces a preview and does not run until approved; a fuzz test on the command parser.

**Brief 3.3: the channel core.** Package `internal/channel`. The queue on disk that every channel feeds and the loop drains; the event stream every channel subscribes to; the local socket the terminal attaches over, speaking newline-delimited JSON with the message types listed in `ARCHITECTURE.md`. Reference: ZeroClaw's channel trait at `~/Code/zeroclaw/crates/zeroclaw-api/src/channel.rs`; OpenCode's event bus in `~/Code/opencode/packages/opencode/src/session/processor.ts`. Tests: two fake channels receive the same events; a queued message survives a restart; a client attaches over the socket and streams a reply.

**Brief 3.4: the command table and `init`.** Packages `internal/command` and the `init` subcommand in `cmd/coeus`. The commands from section 12 of the design, defined once. `coeus init` is first-run onboarding: it creates the home folder with the right permissions, writes the default persona files with a one-line explanation in each, asks which model to use with a short menu (a local Ollama server it detects, a local LM Studio server it detects, or a cloud provider), asks for the API key through a masked prompt and stores it in the vault or, before the vault exists in wave 4, in a file only the user can read, runs `doctor`, and prints the five commands a new user needs. It takes under two minutes and asks at most six questions. Reference: Hermes' setup flow at `~/Code/hermes-agent/hermes_cli/setup.py` (the question shapes, not the length) and ZeroClaw's quickstart at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/quickstart/mod.rs`; OpenClaw's 4,091-line `scripts/install.sh` as the example of what to avoid. Tests: every command through the fake channel; `init` on an empty home produces a working config and a passing `doctor`; `init` on an existing home changes nothing without a flag.

**Brief 3.5: the binary, the service, and the installer.** Package `cmd/coeus` and `scripts/install.sh`. `coeus serve`, `coeus tui`, `coeus doctor`, `coeus init`, `coeus install` (writes the systemd user unit once, with the watchdog line and the exit codes from `contract`, pointing at a symlink that never moves), `coeus uninstall` (stops the service, removes the unit, and leaves the home folder unless asked). The installer is a shell script under two hundred lines: it checks the distribution, installs the three prerequisites it can (bwrap, ripgrep, signal-cli) with the system package manager or a pinned download, tells the user plainly which ones it cannot (Chrome), downloads the release binary for the architecture, verifies its checksum, places it under `~/.coeus/releases/`, links `current`, and runs `coeus init`. Reference: OpenClaw's unit at `~/Code/openclaw/src/daemon/systemd-unit.ts`, its boot check at `src/gateway/boot.ts`; Hermes' exit codes at `~/Code/hermes-agent/gateway/restart.py`; ZeroClaw's service install at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/service/mod.rs`. Tests: the unit file is a golden file; `serve` answers `/readyz` within one second; a second `serve` refuses while the first holds the lock; the installer runs on a clean Ubuntu container and on a clean Debian container in CI and ends with a passing `doctor`.

### Wave 4: The terminal, Signal, the vault, and reliability

After this wave, a person can use Coeus in a terminal and from a phone. This is the first wave a human tries.

**Brief 4.1: the terminal screen.** Package `internal/tui` on Bubble Tea. A thin client over the socket from wave 3. First frame at once; a spinner only after five hundred milliseconds that stays three seconds; streaming as deltas; inline approvals with once, always, reject; a masked prompt for secrets; screenshots inline where the terminal supports it. Reference: OpenCode's startup rules at `~/Code/opencode/packages/tui/src/component/startup-loading.tsx`, keybinds at `packages/tui/src/config/keybind.ts`, and approval prompt at `packages/tui/src/routes/session/permission.tsx`; Hermes' masked prompt in `~/Code/hermes-agent/cli.py` (search for `_sudo_password_callback`). Tests: a golden first frame; spinner timing on the fake clock; an approval round-trip; the masked prompt never echoes. Coverage target seventy percent.

**Brief 4.2: the signal-cli client and linking.** Package `internal/signal`. Starts and supervises `signal-cli daemon --http`, waits for its health endpoint, reads events over SSE with reconnect at two to sixty seconds and a forced reconnect after two minutes of silence, sends over JSON-RPC in four-thousand-character chunks, sends typing indicators, downloads attachments into a cache capped at two hundred megabytes. `coeus signal link` runs `signal-cli link -n coeus`, renders the returned link as a QR code in the terminal, waits for the phone to scan it, and confirms. Reference: ZeroClaw's channel at `~/Code/zeroclaw/crates/zeroclaw-channels/src/signal.rs`; OpenClaw's daemon supervision at `~/Code/openclaw/extensions/signal/src/daemon.ts`, client at `src/client.ts`, reconnect at `src/sse-reconnect.ts`, and its linking documentation at `docs/channels/signal.md`; Hermes' health probe in `~/Code/hermes-agent/gateway/platforms/signal.py`, formatting in `signal_format.py`, rate limiter in `signal_rate_limit.py`, and its Signal setup text in `hermes_cli/gateway.py` (search for `_setup_signal`). Tests: against the fake signal-cli: message in and reply out; a dropped stream reconnects; a stalled stream is forced; an attachment; chunking; the link flow against a scripted fake; a fuzz test on the event decoder.

**Brief 4.3: pairing and the Signal channel.** Packages `internal/signal/pairing` and `internal/channel/signal`. Pairing: eight-character codes from a thirty-two-character alphabet, one-hour expiry, three pending at most, one request per sender per ten minutes, lockout after five failures, codes stored salted and hashed and compared in constant time, approved ids stored as Signal ids. The channel: the six `contract.Channel` functions, one message per reply, a "still working" note once after five idle minutes, a preview as the actual text with approve and deny replies, a handoff with a screenshot, inbound photos and files saved to an inbox the model can read. Reference: Hermes' pairing at `~/Code/hermes-agent/gateway/pairing.py` and authorization at `gateway/authz_mixin.py`; OpenClaw's pairing store at `~/Code/openclaw/src/pairing/pairing-store.ts`. Tests: every pairing rule; the wave-3 functional tests repeated through the Signal channel against the fake signal-cli; an inbound photo is readable.

**Brief 4.4: the vault and redaction.** Package `internal/vault`. An `age`-encrypted file with a key file only the agent's user can read, entries of site, domains, username, password, and TOTP secret, entry only through the masked prompt, a resolver for `secret://name` that never returns a value to the model, a TOTP generator, the sudo entry, and a redaction function on every reply, log line, and tool result. `init` from wave 3 is updated to store the API key here. Reference: ZeroClaw's secrets at `~/Code/zeroclaw/crates/zeroclaw-config/src/secrets.rs`; Hermes' redaction in `~/Code/hermes-agent/tools/browser_tool.py` (search for `_redact_browser_output`). Tests: round-trip; loose key permissions are refused; the resolver; TOTP against a known vector; redaction on ten secret shapes; a fuzz test on the entry parser.

**Brief 4.5: reliability.** Package `internal/reliability`. The crash-loop breaker, one lease per session with a five-second wait and a reject on timeout, the delivery ledger with the duplicate marker and three attempts over twenty-four hours, the lifecycle sentinel with a quick integrity check after an unclean exit, the drain marker with a boot id and a thirty-minute expiry, one deadline primitive for the fifteen-minute turn and seven-minute tool limits, the systemd watchdog feed from the main loop, and a nightly encrypted backup of the database, the vault, and the browser profile to the configured path with `coeus backup` and `coeus restore` commands. Reference, one Hermes file each: `~/Code/hermes-agent/gateway/restart_loop_guard.py`, `gateway/turn_lease.py`, `gateway/delivery_ledger.py`, `gateway/lifecycle_ledger.py`, `gateway/drain_control.py`, `agent/deadline.py`, `gateway/session_db_recovery.py`. Tests: each mechanism on the fake clock; a kill mid-turn asserts the duplicate marker; a corrupt database is renamed and the backup restored; `backup` then `restore` into an empty home reproduces the original byte for byte.

**A human tries it.** After this wave, a person installs Coeus on a clean machine with the installer, runs `init`, links Signal, pairs a phone, and uses it for ten minutes. What confused them becomes wave-5 briefs.

### Wave 5: Memory and skills

After this wave, Coeus learns and remembers.

**Brief 5.1: memory files and the index.** Package `internal/memory`. `MEMORY.md` and `USER.md` with their caps, a `memory/` folder, a full-text index in SQLite over all of it plus every past message, a source and date on every fact, supersede instead of delete, search and get and save with atomic batches, and the three-line hint. Reference: Hermes' memory tool at `~/Code/hermes-agent/tools/memory_tool.py` and session search at `tools/session_search_tool.py`; OpenClaw's contract at `~/Code/openclaw/extensions/memory-core/src/memory-tool-contract.ts`. Tests: caps; supersede keeps the old fact searchable; an empty hint when nothing matches; the twenty-question memory test against a recorded session.

**Brief 5.2: harness-written memory.** Package `internal/memory/capture`. Zero-token capture from the event log: files changed, commands run, sites visited, jobs created, and any user message beginning with "no," "actually," "always," "never," or "don't," saved word for word as a correction. Tests: each rule; a correction is retrievable the same turn.

**Brief 5.3: the skill format and the router.** Package `internal/skill`. The folder format from section 8 of the design, a loader that puts names and one-liners in the prompt and loads bodies on use, the permissions block enforced by the harness, the changelog with rollback, and the router hook that matches trigger words and runs a skill without the model. Reference: Hermes' format at `~/Code/hermes-agent/tools/skills_tool.py` and ledger at `tools/skill_ledger.py`; OpenCode's loader at `~/Code/opencode/packages/opencode/src/skill/index.ts`. Tests: a golden folder loads; an over-limit step is refused; rollback; a trigger runs a skill with the fake model never called.

**Brief 5.4: learning skills.** Package `internal/skill/learn`. From documentation: the model reads a page and writes a skill with the commands it will use and one example each, then runs the smoke test. From a task: at the end, the model offers to save the steps, and approval saves a valid folder. From the after-action review: when the fourth answer describes a procedure, it is offered the same way. Reference: ZeroClaw's creator at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/skills/creator.rs`; Hermes' learn prompt at `~/Code/hermes-agent/agent/learn_prompt.py`. Tests: a scripted page produces a skill that passes its smoke test; a completed task produces an offer and approval saves it.

**Brief 5.5: memory and skill functional tests, and the wave-4 human notes.** Folder `test/functional/`. A fact saved and recalled in a later task; a correction that survives across sessions; a skill taught from documentation then triggered; the review writing to memory; the monthly memory test harness. Plus the fixes from the wave-4 human notes.

### Wave 6: The browser

After this wave, Coeus uses Chrome like a person. The worker is built first and alone because everything else waits on it.

**Brief 6.1: the browser worker.** Folder `worker/browser/`, TypeScript on Node with `playwright-core`, implementing `worker/browser/PROTOCOL.md` from wave 0 exactly. Launches the real Chrome binary with a dedicated profile folder and a loopback DevTools port with a per-launch token, attaches with `connectOverCDP`, and speaks JSON-RPC over standard input and output. Provides the snapshot (tree with refs, new-element marks, below-the-fold count, collapsed lists, dialog state), the labeled screenshot, click, type with per-key pacing, press, scroll, and the act batch, each with the settle wait and the diff, plus the login-fill method that takes a username, a password, and a code and types them without ever returning them. Stale refs are re-found by role and name, then text. A wall detector reports a login form, a two-factor prompt, or a captcha by matching the page's roles and text against a fixture list. TypeScript tests use Vitest with the same four kinds, fuzzing through `fast-check`, coverage seventy percent. Reference: OpenClaw's launch and profile at `~/Code/openclaw/extensions/browser/src/browser/chrome.ts`, attach at `extensions/browser/src/browser/pw-session-cdp-transport.ts`, limits at `extensions/browser/src/browser/act-policy.ts`, and schema at `extensions/browser/src/browser-tool.schema.ts`; browser-use's serializer at `docs/reference/browser-use/serializer.py` and views at `docs/reference/browser-use/views.py`; Moltis's manager and snapshot at `docs/reference/moltis/manager.rs` and `snapshot.rs`. This brief runs alone for the first half of the wave; briefs 6.2 through 6.5 start against the fake worker and switch to the real one when 6.1 lands. Tests: against recorded fixture pages: every method; a stale ref re-found; a click with no change reported; a settle timeout; a dialog; a download; a new tab; a snapshot under the cap on a large page; the wall detector on login, two-factor, and captcha fixtures.

**Brief 6.2: the Go side of the browser.** Package `internal/browser`. Starts the worker on the first browser tool call, keeps it alive thirty minutes, restarts it if it dies, turns the seven browser tools from section 9 of the design into protocol calls, applies the pacing profile and the per-site daily action budget. Tests against the fake worker: spawned once and reused; a dead worker restarted with the interruption reported; the daily budget refuses and says so.

**Brief 6.3: login and handoff.** Packages `internal/browser/login` and `internal/browser/handoff`. Login takes a site name, finds the vault entry, checks the page is on one of the entry's domains, calls the worker's login-fill with the credentials and a fresh TOTP code, and reports success or the wall it hit. Handoff brings the window forward, sends a screenshot and a numbered list through the current channel, and blocks the task until the user replies done, a code, or abort. Tests against the fake worker: a login on a fixture; a wrong domain refused; a handoff round-trip through the fake channel.

**Brief 6.4: browser skills.** Package `internal/skill/browser`. Recording of each step's intent, element descriptor, and expectation; replay through the cascade with no model call; self-heal that asks the model for the element matching the intent, verifies, and proposes a one-line patch for approval. Reference: the observe-then-act cache idea in `docs/research/16-browser-agent-spec.md`. Tests: record a three-step fixture flow and replay it with the fake model never called; break one step and confirm a patch is proposed and not applied.

**Brief 6.5: browser functional tests and the visual QA skill.** Folder `test/functional/` and `skills/qa/`. Log in to a fixture site through the vault; post with a preview and approval; hit a captcha fixture and hand off; replay a recorded skill; the QA skill walks a fixture app and produces a report with screenshots. Tier-two live: the same flows against Opus 4.8 and the local Qwen on a real fixture site served on the development machine.

**A human tries it.** After this wave, a person logs into a real site once by hand and asks Coeus to do something there.

### Wave 7: Always on

After this wave, Coeus runs jobs, uses the desktop, and updates itself.

**Brief 7.1: scheduled jobs.** Package `internal/schedule`. One job object with at, every, or cron and a timezone, a prompt or a command, a delivery target with a validated failure lane, and state. One timer clamped to sixty seconds. Claim with an atomic update. Backoff of thirty seconds to sixty minutes, auto-disable after ten failures with a message, one incident per distinct error, a sixteen-kilobyte notepad, a monitor mode that hashes output and wakes the model only on change, and a rule that a job can never call `schedule` or restart the agent. The `schedule` tool from wave 2 now talks to this package. Reference: ZeroClaw's cron types, store, and scheduler at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/cron/types.rs`, `store.rs`, and `scheduler.rs`; OpenClaw's backoff and auto-disable at `~/Code/openclaw/src/cron/service/jobs-scheduling.ts` and `service/auto-disable.ts`; Hermes' incidents, notepad, monitor, and restart guard at `~/Code/hermes-agent/cron/incidents.py`, `cron/notepad.py`, `cron/monitor.py`, and `cron/lifecycle_guard.py`; Prime's heartbeat at `~/Code/prime-agent/packages/coding-agent/src/core/cron-jobs.ts`. Tests: every rule on the fake clock; two processes cannot claim one job; an unattended ask stops and reports.

**Brief 7.2: the desktop worker.** Folder `worker/desktop/` on `@trycua/cua-driver` and package `internal/desktop`. Launch or focus an app, screenshot with numbered marks, click, type, key, drag, clipboard, with the browser's pacing and act-and-assert loop, an app granted once per session, a preview for every irreversible action. Reference: Hermes' computer-use tool at `~/Code/hermes-agent/tools/computer_use/` and the driver contract in `~/Code/openclaw/extensions/cua-computer/`. Tests against a fixture window: each action; an ungranted app is refused. Coverage seventy percent.

**Brief 7.3: the updater.** Package `internal/update`. `coeus update` checks the release manifest, downloads the binary for the architecture, verifies its checksum, places it in `releases/<version>/`, keeps the previous three, switches the symlink, restarts the service, requires `/readyz` within sixty seconds, and switches back on failure. Forward-only numbered database migrations, each in a transaction after a backup. An older binary refuses a newer schema and names the version to use. A user's data is never touched by an update except through a migration. Reference: ZeroClaw's updater at `~/Code/zeroclaw/src/commands/update.rs`; OpenClaw's `~/Code/openclaw/src/cli/update-cli/` as the example of what to avoid. Tests: a binary that fails readiness rolls back within sixty seconds; a failing migration leaves the backup intact; an update over a live install preserves every task, memory, and skill.

**Brief 7.4: replay as test and the nightly self-check.** Package `internal/replay`. Any task from the event log re-runs against the current code with the recorded tool results as a Go test the user invokes with `coeus replay <task>`. A nightly job runs the memory test and the recorded skill tests and reports one line. Tests: replay a recorded failing task, apply a fixture fix, replay again and pass.

**Brief 7.5: the security review and the release.** A worker with fresh eyes reviews the sandbox, the vault, the sudo path, the network guard, the pairing system, and the browser profile handling against the threat list in section 11 of the design, and files a finding for anything that fails. The final functional pass runs every test from every wave on a clean Ubuntu and a clean Debian container with `make check` and `make live`. `docs/PROGRESS.md` is complete. The README explains the logic the way section 1 of the design does and tells a new user how to install, run `init`, link Signal, and pair, in ten lines.

---

## Part 4: What the orchestrator watches for

- **A worker adds a library because it is convenient.** Reject it unless the reason is in `docs/DEPENDENCIES.md` and the standard library cannot do the job.
- **A worker writes the code first and the test after.** The commit history shows it. Reject.
- **A worker copies code instead of porting the design.** Rust and TypeScript idioms in Go are the sign. Reject and point at the reference again.
- **A package grows past its one sentence.** Split it before the next wave.
- **A test passes by accident.** The orchestrator changes one thing the test should catch and confirms the test fails.
- **A fake drifts from the real thing.** Every fake has a contract test that also runs against the real service under the `live` tag.
- **The prose slips.** The style checker runs in `make check`. A wave with violations does not pass.
- **The living documents go stale.** `REPO_MAP.md` drift fails `make check`. `ARCHITECTURE.md` is read by the orchestrator at every wave gate against the code.
- **A human is never asked.** Waves 4, 6, and 7 each end with a person using it. No exceptions.

---

## Part 5: The code bases to read

| Project | Where it is | What a worker reads it for |
|---|---|---|
| OpenClaw | `/home/jared/Code/openclaw` at `752a983480e4` | The loop with hooks, the tool-call repair grammar, the loop detector, the retry refund, the cache boundary, the Signal daemon and client and linking docs, the cron backoff, the pinned DNS lookup, the external-content wrapper, the delivery queue, the pairing store, the boot check, the systemd unit, the browser launch and attach, and the installer and updater as examples to avoid |
| Hermes | `/home/jared/Code/hermes-agent` at `95f62ca3bfcf` | The three prompt tiers and cache points, verbatim user messages, the memory tool and caps, session search, pairing, the restart-loop guard, the turn lease, the delivery ledger, the lifecycle sentinel, the drain marker, the deadline primitive, the sudo prompt, cron incidents and notepad and monitor, the Signal platform and setup text, the skill ledger, the learn prompt, the setup wizard's question shapes |
| Prime | `/home/jared/Code/prime-agent` at `0ba0423c5c18` | The provider types and two API files, overflow detection, goals, autonomous budgets, refinement with rollback, cron as heartbeat |
| OpenCode | `/home/jared/Code/opencode` at `69c172e8a7c0` | The permission engine and prefix table, the tool registry and text descriptions, read, edit, grep, glob, truncation, invalid calls, the retry policy, instruction re-reading, the command registry, the startup spinner rules, keybinds, the inline approval prompt, the skill loader |
| ZeroClaw | `/home/jared/Code/zeroclaw` at `73ff732e66c7` | The channel trait, the Signal channel, the sender allowlist, the cron types and store and scheduler, the tool-call parser, the turn cap, the loop detector, the Landlock wrapper, the updater, the service install, the quickstart, the skill creator, the encrypted secrets |
| HomeRecon | `/home/jared/Code/homerecon` | `CLAUDE.md`, `ARCHITECTURE.md`, and `scripts/repo-map/` for the three living documents and the drift test |
| browser-use | `docs/reference/browser-use/` | The page serializer with new-element marks and scroll hints, the per-step evaluation fields |
| Moltis | `docs/reference/moltis/` | The Chrome manager with a persistent profile, the snapshot with numbered refs |
| Codex | `docs/reference/codex/` | The shell tool spec with the justification field, the Landlock rules |
| The research | `docs/research/` | Seventeen studies with line-level citations, and three review files, for any question a reference does not answer |

---

## Part 6: What the review found and how this version fixes it

An adversarial review of the first draft found 24 distinct blockers. Each is fixed above.

| Blocker | Fix |
|---|---|
| Waves 2, 3, 4, and 7 had briefs importing packages built by neighbors in the same wave | Every cross-wave interface moved to `internal/contract` in wave 0; provider and repair moved to wave 1; the tool contract, permission, and all thin tools moved to wave 2; the channel core and socket moved to wave 3 ahead of the terminal; the browser worker runs alone in the first half of wave 6 |
| The fake channel and fake browser worker were built before their interfaces existed | Interfaces in wave 0; `worker/browser/PROTOCOL.md` in wave 0 |
| Tests needed Linux but development was on macOS | All work on the Linux development machine; CI on a Linux runner; wave 0 readies the machine |
| No CI, Makefile, `go.mod`, license, or third-party notes | All in brief 0.1 |
| Three of eighteen tools were never built (`task`, `skill`, `schedule`) | Brief 2.5 |
| No installer, no prerequisites list, no first-run `init`, no uninstall | Briefs 3.4 and 3.5 |
| No Signal QR linking | Brief 4.2 |
| No nightly backup or restore | Brief 4.5 |
| No login-wall or captcha detector | Brief 6.1 |
| No search provider named | SearXNG, self-hosted, in brief 2.5 |
| No user-tool protocol | Defined in `contract`, wave 0 |
| No configuration fields listed | Brief 1.4 |
| The forty-step fixture was invoked but never written | Brief 1.1 |
| `write` and `edit` were not path-limited | Brief 2.3 |
| The model instruction text had no owner | Brief 2.1 |
| No security review | Brief 7.5 |
| No human ever tried it | After waves 4, 6, and 7 |
| Fakes the briefs needed that testkit lacked | Permission decider, tool registry, memory store, search server added |
| TypeScript workers had no test rules | Vitest, fast-check, seventy percent, in briefs 6.1 and 7.2 |
| The browser wave was one worker for a week and four waiting | Split into a first half and a second half against the fake |
| No real-model testing | Tier two: Opus 4.8 and local Qwen at every wave gate |
| The living documents were not in the plan | Part 1, "The three living documents," and every definition of done |
