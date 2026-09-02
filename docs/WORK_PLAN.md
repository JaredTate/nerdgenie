# COEUS WORK PLAN

**How we build Coeus, from an empty folder to a working agent, in waves of five workers, with every test written before the code it proves.**

This is the plan the orchestrator follows. The design it builds is in `COEUS_PLAN.md`. Read that first. The comparison behind the design is in `HARNESS_V2.md`, and the reasons behind every borrowed idea are in `docs/research/`.

---

## Part 1: The rules

These rules apply to every wave, every worker, and every line of code. They are not suggestions. A worker who cannot follow one of them stops and reports instead of working around it.

### Who does what

The **orchestrator** is a Fable 5.1 model. It reads this plan, writes the brief for each worker, checks each worker's result against the brief, runs the full test suite after every wave, and decides whether the wave passed. It never writes feature code itself.

A **worker** is an Opus 4.8 model running at extra-high effort. Each worker gets one brief, one package, and one wave. A worker reads the reference code it is pointed at, writes the tests, writes the code that passes them, and reports back with the test output. Five workers run at once. A worker never touches a package that another worker in the same wave owns.

### The seven rules of the code

1. **Keep it simple.** The simplest thing that passes the test is the right thing. If a worker is choosing between two designs and one has fewer moving parts, the one with fewer moving parts wins. No abstraction is added until the second use of it exists. No configuration option is added until a real user needs it.
2. **Tests come first.** A worker writes the test, watches it fail, then writes the code, then watches it pass. Code without a test does not merge. A test that was written after the code is rewritten.
3. **Go for the agent, TypeScript for the browser worker, nothing else.** The Go code uses the standard library wherever the standard library will do. Every outside library is listed in `go.mod` with a one-line reason in `docs/DEPENDENCIES.md`, and a worker who wants to add one asks the orchestrator first.
4. **One package, one job.** Each Go package has one purpose that fits in one sentence at the top of its `doc.go` file. A package that needs two sentences is two packages.
5. **Plain English everywhere.** Every identifier says what it is. Every comment is a complete sentence. Every error message tells the reader what went wrong and what to do about it. Every log line can be read by someone who has never seen the code.
6. **Bound everything.** Every loop has a limit. Every wait has a timeout. Every buffer has a cap. Every outside call can fail, and the code says what happens when it does.
7. **Borrow designs, not code.** The other agents are written in TypeScript, Python, and Rust. A worker reads the reference file it is pointed at, understands the design, and writes it fresh in Go. Nothing is copied line by line. The borrowed design is named in a comment at the top of the Go file, with the path to the reference.

### The four kinds of tests

Every package has all four before it is called done.

| Kind | What it proves | How it is written |
|---|---|---|
| **Unit test** | One function does what its name says, for normal inputs, edge inputs, and bad inputs | Standard Go `testing`, table-driven. One file per source file, named `x_test.go` |
| **Integration test** | Two or more packages work together through their real interfaces, with the real database and the real filesystem in a temporary folder | In the package that sits highest in the dependency order, marked with the build tag `integration` |
| **Functional test** | A whole feature works the way a user would see it: a message goes in, the right reply comes out, the right things happen on disk | In `test/functional/`, driven through the same API the terminal uses, with a fake model and fake outside services |
| **Fuzz test** | The code survives input it was never designed for | Go's built-in `testing.F`, on every function that parses text from outside the program: tool calls, messages, records, snapshots, configuration |

### The test framework

The framework is built in wave 1, before any feature, and every later test uses it. It lives in `internal/testkit/` and it provides exactly these things:

- **A fake model.** It is scripted: a test gives it a list of replies, and it returns them in order. It can be told to emit tool calls, malformed tool calls, tool calls written as text, empty replies, and replies that stop with an unknown finish reason. It records every prompt it was sent, so a test can assert on what the model saw. It reports token counts so the cost accounting can be tested.
- **A fake channel.** It is a channel in the sense of section 6 of the plan. A test pushes messages in and reads replies, previews, and handoffs out.
- **A fake clock.** Every package takes a clock as an input, never calls the real time directly. A test moves the clock by hand, so timeouts and schedules are tested in milliseconds instead of minutes.
- **A temporary home.** A fresh, empty Coeus home folder with a fresh database, created for each test and deleted after it, so tests never share state and can run in parallel.
- **A fake signal-cli, a fake model provider server, and a fake browser worker.** Each one speaks the real wire protocol over a local socket, so the real client code is exercised, and each one can be told to fail, stall, or return garbage.
- **Golden files.** A helper that compares a result to a saved file in `testdata/` and, when a flag is set, rewrites the saved file. Used for prompts, records, and snapshots.
- **A recorded-task replayer.** It takes a task log from the event log and re-runs it against the current code with the recorded tool results, which is the "any failed task becomes a test" promise from the plan.

Every fake has its own unit tests. The framework is the only place fakes live; a test that needs a new fake adds it here, not in its own package.

### How a wave runs

1. The orchestrator writes five briefs, one per worker. A brief names the package, the job in one paragraph, the reference files to read with their paths, the tests that must exist, and the definition of done.
2. The five workers run at once. Each one works in its own branch.
3. Each worker reports with the test output, the list of files, the line count, and any question it could not answer.
4. The orchestrator merges the five branches, runs the whole test suite, runs `go vet`, `staticcheck`, and `gofmt`, and reads every new file.
5. If everything passes, the wave is done and the next wave starts. If anything fails, the orchestrator writes a fix brief for one worker and does not start the next wave until it passes.
6. After every wave, the orchestrator writes three lines in `docs/PROGRESS.md`: what was built, what the test count is now, and what is next.

### Definition of done, for every package

A package is done when all of these are true. The orchestrator checks each one.

- Every exported function has a unit test with normal, edge, and bad inputs.
- Every function that parses outside text has a fuzz test that has run for at least one minute without a failure.
- The integration test for the package passes against the real database in a temporary home.
- `go vet`, `staticcheck`, and `gofmt` report nothing.
- The package's `doc.go` has a one-sentence purpose and a paragraph that a new reader could understand.
- No function is longer than sixty lines. No file is longer than five hundred lines.
- The borrowed design, if any, is named at the top of the file with the reference path.
- Test coverage of the package is above ninety percent by line.

---

## Part 2: The layout

```
coeus/
  cmd/coeus/            the one binary: serve, tui, update, and the /commands
  internal/
    testkit/            the test framework, wave 1
    log/                the event log, wave 1
    record/             the task record, wave 1
    context/            builds the working context from the layers, wave 2
    provider/           the model providers: anthropic, openai-compatible, wave 2
    loop/               the turn loop and the guard, wave 2
    permission/         the permission function and the policy file, wave 3
    tool/               the tool contract and the built-in tools, wave 3
    sandbox/            bwrap and Landlock, wave 3
    channel/            the channel contract; terminal and signal underneath, waves 4 and 5
    tui/                the terminal screen, wave 4
    signal/             the signal-cli client, wave 5
    vault/              the encrypted secret store, wave 5
    memory/             the memory files, the search index, the hint, wave 6
    skill/              the skill folder format, recording, and replay, wave 6
    browser/            the Go side: launching Chrome, talking to the worker, wave 7
    schedule/           scheduled jobs, wave 8
    desktop/            the Go side of the desktop worker, wave 8
    update/             the updater with rollback, wave 8
  worker/browser/       the TypeScript browser worker, wave 7
  worker/desktop/       the TypeScript desktop worker, wave 8
  test/functional/      whole-feature tests
  docs/                 this plan, the design, the research, the references
```

Packages depend only downward in this list. `loop` may use `record` and `log`; `record` may not use `loop`. The orchestrator rejects any import that goes the wrong way.

---

## Part 3: The waves

Each wave lists five briefs. Each brief has the package, the job, the reference files to read, the tests that must exist, and what done means for that brief. Reference paths that start with `~/Code/` are the other projects on disk. Paths that start with `docs/reference/` are copies kept in this repository because the original was cloned only for the research.

### Wave 1: The foundation

Nothing in this wave calls a model or talks to a user. It is the ground everything else stands on.

**Brief 1.1: the test framework.** Package `internal/testkit`. Build every fake listed under "The test framework" above, with its own tests. The fake model must be able to script a full conversation. The fake clock must satisfy an interface that every other package will take. The temporary home must create and destroy a complete Coeus home folder. Done when a sample test in `test/functional/` can push a message through the fake channel, get a scripted reply from the fake model, and assert on both, in under one second.

**Brief 1.2: the event log.** Package `internal/log`. An append-only table in SQLite, using the pure-Go driver `modernc.org/sqlite`, holding every message, tool call, tool result, permission decision, and record change, each with a sequence number, a timestamp, a task id, and a kind. Write is a single insert. Read is by task, by kind, by id, or by range. A replay function hands events back in order. The database opens in WAL mode with one writer. Reference: the design of Hermes' delivery ledger at `~/Code/hermes-agent/gateway/delivery_ledger.py` (the shape of "write the obligation before acting"), and OpenClaw's persisted queue at `~/Code/openclaw/src/infra/outbound/delivery-queue-storage.ts`. Tests: unit tests for every read shape; an integration test that writes ten thousand events and replays them in order; a test that kills the process mid-write and confirms the log is intact on reopen; a fuzz test on the event decoder.

**Brief 1.3: the task record.** Package `internal/record`. The record format from section 4 of the plan, as a Go struct, with a parser and a printer that round-trip exactly. The rules from "The rules for the record" are enforced in code: the ask, intent, and corrections are immutable after creation; a fact must carry a source; a failure must carry a cause; a step marked done must name a result; the record folds finished steps and old results into single lines when it passes its size cap. Checkpoints are saved to the event log on every change and can be reloaded by number. Reference: Prime's goal state at `~/Code/prime-agent/packages/coding-agent/src/core/goals.ts` and its harness state with rollback at `~/Code/prime-agent/packages/coding-agent/src/core/refinement/refinement.ts`; the example record in section 4 of the plan is the golden file. Tests: round-trip on the golden record; a unit test for each rule that must reject a bad edit; the forty-step test, which applies forty changes and asserts the ask and corrections are byte-identical at the end and every result id is still readable; a rewind test; a fuzz test on the parser.

**Brief 1.4: configuration and the home folder.** Package `internal/config`. One file, `~/.coeus/config.toml`, read once at startup, with a typed struct, defaults for everything, and a plain error naming the line for anything wrong. The home folder layout: the database, the persona files, the skills folder, the vault, the browser profile, the backups. A `doctor` function that reports which parts exist and which are missing, and never changes anything. Tests: defaults with an empty file; every field with a wrong type; the doctor on an empty home and a full one; a fuzz test on the parser.

**Brief 1.5: the plain-English style checker.** Package `internal/lint`, used only in tests. A checker that reads a Go source tree and reports: any exported identifier under three characters, any comment that is not a complete sentence, any function over sixty lines, any file over five hundred lines, any error string that does not say what to do. Wired into `make test` so every later wave is checked automatically. Tests: a fixture tree with one violation of each kind, and a clean tree.

### Wave 2: The loop

After this wave, the agent can hold a conversation with a fake model through a fake channel and pass the forty-step task.

**Brief 2.1: the model providers.** Package `internal/provider`. One interface: send a prompt, stream back text and tool calls, report tokens in and out and how many were served from cache. Two implementations: the Anthropic API, and the OpenAI-compatible API with a base address. The provider places cache markers where section 4 of the plan says. Retries live here: three tries with backoff, honoring the provider's retry-after header, and never on a context-overflow error. A fallback chain moves to the next configured model after the tries run out. Reference: Prime's provider types and its two API files at `~/Code/prime-agent/packages/ai/src/types.ts`, `providers/anthropic.ts`, and `providers/openai-completions.ts`; Prime's overflow detection at `packages/ai/src/utils/overflow.ts`; OpenCode's retry policy at `~/Code/opencode/packages/opencode/src/session/retry.ts`; Hermes' local-model rules at `~/Code/hermes-agent/plugins/model-providers/custom/__init__.py` (send `think=false` only to Ollama-shaped addresses) and `agent/model_metadata.py` (prefer the Modelfile context length). Tests: unit tests against the fake provider server for both APIs, including streamed tool calls, a stalled stream, a 429 with retry-after, a 500, and an overflow; a test that the fallback chain moves on; a fuzz test on the stream parser.

**Brief 2.2: the tool-call repair layer.** Package `internal/repair`. Takes the raw text of a model reply and returns the tool calls in it, whether they arrived as structured calls, as JSON in a code fence, as a `<tool_call>` block, or as a near-miss on a tool name. Fixes names that are close to a real tool. Returns a clear error the model can act on for anything else. Two failed parses in a row and the text is treated as the answer. Reference: ZeroClaw's parser crate at `~/Code/zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs` (the six envelope shapes), OpenClaw's grammar at `~/Code/openclaw/packages/tool-call-repair/src/grammar.ts`, OpenCode's invalid-call handling at `~/Code/opencode/packages/opencode/src/tool/invalid.ts`. Tests: a golden file per envelope shape; name repair cases; the two-failures rule; a fuzz test that must never panic on any input.

**Brief 2.3: the working context builder.** Package `internal/context`. Builds the prompt in the layered order from section 4 of the plan: harness rules, persona, tools, the stable part of the record, the live part, pinned evidence, recent messages, the memory hint. Sizes the pinned and recent windows from the model's context length. Implements tiered folding: when the recent window is full, the oldest unpinned exchange becomes one line in the record. Never rewrites anything above the cache line during a task. Emits the per-turn cost line. Reference: Hermes' three tiers and four cache points at `~/Code/hermes-agent/agent/prompt_caching.py` and `agent/prompt_cache_boundary.py`; OpenClaw's cache boundary at `~/Code/openclaw/src/agents/system-prompt.ts` (search for `SYSTEM_PROMPT_CACHE_BOUNDARY`); Hermes' rule that user messages are kept verbatim at `~/Code/hermes-agent/agent/context_compressor.py` (search for the verbatim user-message logic); OpenCode's re-read of instruction files every step at `~/Code/opencode/packages/opencode/src/session/instruction.ts`. Tests: a golden prompt for a 24k model and a 200k model from the same task; a test that nothing above the cache line changes across ten turns; a folding test that fills the window and confirms every folded exchange is still readable by id; the cost line matches the fake provider's counts.

**Brief 2.4: the turn loop and the guard.** Package `internal/loop`. The loop from section 3 of the plan: route, orient, call, guard, permit, run, update, repeat, with the ten rules as code. The guard: the identical-call detector counted across steps with a window of twenty, the malformed-call path through `repair`, the round cap with the final tools-off call, the stop conditions, and the taint flag. A mid-turn message is written as a correction and picked up at the next tool boundary. A question from the model ends the turn in the waiting state. The permission function is an interface here; wave 3 fills it in. Reference: OpenClaw's pure loop with hooks at `~/Code/openclaw/packages/agent-core/src/agent-loop.ts` and `types.ts` (the callback seams, steering at tool boundaries); ZeroClaw's turn engine at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/agent/turn/mod.rs` (the round cap and forced final answer) and its loop detector at `agent/loop_detector.rs`; OpenClaw's retry refund at `~/Code/openclaw/src/agents/embedded-agent-runner/run/retry-budget.ts`; OpenCode's doom-loop check at `~/Code/opencode/packages/opencode/src/session/processor.ts` (read it to see the gap the plan closes). Tests: a scripted conversation for each of the ten rules; the identical-call detector across steps and inside one response; the cap with the final answer; a question that suspends and a message that resumes; a fuzz test on the model reply handler.

**Brief 2.5: the after-action review and the done-check.** Package `internal/review`. When a task's plan is complete, the done-check asks the model to answer each "Done" line true or false with evidence and refuses to close the task on any false. Then the four review questions are asked, and the fourth answer is handed to the memory package as a proposed fact. Reference: section 4 of the plan; Prime's verifier gates in `~/Code/prime-agent/packages/coding-agent/src/core/autonomous.ts`. Tests: a done-check that passes, one that fails on one line and stays open, and one where the model tries to close without evidence; the review produces four lines and hands off the fourth.

### Wave 3: Tools, permissions, and the sandbox

After this wave, the agent can read, write, edit, search, and run commands safely.

**Brief 3.1: the tool contract and registry.** Package `internal/tool`. The contract from section 6 of the plan: name, description under forty words, typed inputs, permission class, run function returning text. A registry that loads the built-in tools and any tool found in `~/.coeus/tools/`. Every result carries the output cap from the plan and spills the rest to a file. Reference: OpenCode's registry at `~/Code/opencode/packages/opencode/src/tool/registry.ts` and its plain-text descriptions in the `.txt` files beside each tool; OpenCode's truncation at `tool/truncate.ts`. Tests: registration of a folder tool; a tool whose description is over forty words is rejected; output over the cap is spilled and the result names the file.

**Brief 3.2: the file tools.** Package `internal/tool/file`: `read`, `write`, `edit`, `search`. Read with offsets and line numbers and a hint that says how to continue; edit with exact-match plus the fallback matchers for whitespace and quotes; search backed by ripgrep with a fifty-row cap. Reference: OpenCode's `tool/read.ts`, `tool/edit.ts` (the nine fallback matchers), `tool/grep.ts`, and `tool/glob.ts`; Hermes' single search tool at `~/Code/hermes-agent/tools/file_tools.py` (search for `search_files`). Tests: golden results for each tool; every edit fallback; a read past the end of a file; a fuzz test on the edit matcher.

**Brief 3.3: the shell tool and the sandbox.** Packages `internal/tool/shell` and `internal/sandbox`. Shell runs a command in its own process group with a timeout; after ten seconds it returns a job id the model can poll, tail, or kill; the escalate field with a reason produces a preview and, on approval, runs outside the sandbox with sudo. The sandbox wraps every command in bwrap with a Landlock ruleset and a seccomp filter, keeps the vault, the browser profile, and `~/.ssh` outside, allows network, and turns the shell tool off if bwrap is missing. Reference: OpenClaw's yield-then-background at `~/Code/openclaw/src/agents/bash-tools.schemas.ts`; Codex's escalation field at `docs/reference/codex/shell_spec.rs` and its Landlock rules at `docs/reference/codex/landlock.rs`; ZeroClaw's Landlock wrapper at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/security/landlock.rs` and its sandbox trait at `security/traits.rs`; Hermes' sudo path at `~/Code/hermes-agent/tools/terminal_tool.py` (search for `sudo_password`). Tests: a command that finishes in one second; one that runs for thirty seconds and is polled and killed; a sandboxed command that tries to read `~/.ssh` and must fail; a sandboxed command that reaches the network and must succeed; the tool reports itself disabled when bwrap is absent; a fuzz test on the command parser.

**Brief 3.4: the permission function and the policy file.** Package `internal/permission`. Rules of tool, pattern, and action, last match wins, default ask. Shell commands reduced to a readable prefix so a rule for `git push` matches `git push origin main`. The three answers once, always for the session, and reject with a reason the model sees. Irreversible actions always produce a preview. A tainted turn cannot pass an irreversible action. Standing approvals attached to skills with a limit and an expiry. Reference: OpenCode's engine at `~/Code/opencode/packages/opencode/src/permission/index.ts`, its rule format at `packages/core/src/v1/config/permission.ts`, and its command prefix table at `permission/arity.ts`; Hermes' unattended-mode fail-closed rule in `~/Code/hermes-agent/tools/approval.py` (search for `unattended`). Tests: a table of rules and calls with the expected answer for each; the prefix reduction; the taint rule; an expired standing approval; a fuzz test on the pattern matcher.

**Brief 3.5: the web tool and the memory tool.** Packages `internal/tool/web` and `internal/tool/memory`. Web does search and fetch-as-text of public pages, with a pinned DNS lookup so a host cannot change its address mid-request and a block on private address ranges. Memory does search, get, and save against the memory package's interface, which wave 6 fills in; this wave ships the tool against a fake. Reference: OpenClaw's pinned lookup at `~/Code/openclaw/src/infra/net/ssrf.ts` and its external-content wrapper at `src/security/external-content.ts`; Hermes' web extraction budget in `~/Code/hermes-agent/tools/web_tools.py`. Tests: a fetch of a fixture page; a fetch that redirects to a private address and must be refused; the wrapper puts random boundaries around fetched text; a fuzz test on the HTML-to-text step.

### Wave 4: The terminal

After this wave, a person can use Coeus in a terminal against a real model. This is the first thing a human tries.

**Brief 4.1: the channel contract.** Package `internal/channel`. The six things every channel does, as an interface: receive, send, send a file, preview and collect approve or reject, masked prompt, health. A queue on disk that every channel feeds and the loop drains. The event stream that every channel subscribes to. Reference: ZeroClaw's channel trait at `~/Code/zeroclaw/crates/zeroclaw-api/src/channel.rs` (search for `pub trait Channel`); OpenCode's event bus and part deltas at `~/Code/opencode/packages/opencode/src/session/processor.ts`. Tests: two fake channels attached at once both receive the same events; a message that arrives while the loop is busy is queued and survives a restart.

**Brief 4.2: the command table.** Package `internal/command`. The commands from section 12 of the plan, defined once as name, aliases, description, and a function that returns text. Both channels call the same table. Reference: OpenCode's command registry at `~/Code/opencode/packages/tui/src/app.tsx` (search for `slashName`) and `packages/opencode/src/command/index.ts`. Tests: every command through the fake channel; an unknown command gets the help list; `/tasks 17 back 3` rewinds a real record.

**Brief 4.3: the terminal screen.** Package `internal/tui`, on Bubble Tea v2. A thin client of the running agent over a local socket. First frame immediately; a spinner only after five hundred milliseconds that stays at least three seconds; streaming as deltas; inline approvals with once, always, reject; a masked prompt for the vault; screenshots inline where the terminal supports it. Reference: OpenCode's startup rules at `~/Code/opencode/packages/tui/src/component/startup-loading.tsx`, its keybinds at `packages/tui/src/config/keybind.ts`, and its inline permission prompt at `packages/tui/src/routes/session/permission.tsx`; Hermes' masked sudo prompt at `~/Code/hermes-agent/cli.py` (search for `_sudo_password_callback`). Tests: a golden render of the first frame; the spinner timing against the fake clock; an approval round-trip; the masked prompt never echoes.

**Brief 4.4: the binary and the service.** Package `cmd/coeus`. `coeus serve` runs the agent; `coeus tui` attaches; `coeus doctor` reports; `coeus install` writes a systemd user unit once, with the watchdog line and the exit codes from the plan, pointing at a symlink that never moves. Reference: OpenClaw's unit at `~/Code/openclaw/src/daemon/systemd-unit.ts` and its boot self-check at `src/gateway/boot.ts`; Hermes' exit-code contract at `~/Code/hermes-agent/gateway/restart.py`. Tests: the unit file is a golden file; `serve` answers `/readyz` within one second of start; a second `serve` refuses to start while the first holds the lock.

**Brief 4.5: the first functional tests.** Folder `test/functional/`. Ten whole-feature tests through the fake channel and the fake model: a one-line question; a task that uses three tools; a task that asks a question and resumes; a task that hits the round cap; a task that trips the identical-call detector; a mid-turn correction; a permission ask answered once; a permission ask rejected with a reason; a preview of an irreversible action; a crash mid-turn followed by a restart that loses nothing. Done when all ten pass and each runs in under two seconds.

### Wave 5: Signal and the vault

After this wave, a person can use Coeus from a phone.

**Brief 5.1: the signal-cli client.** Package `internal/signal`. Starts and supervises `signal-cli daemon --http`, waits for its health endpoint, reads events over SSE with reconnect at two to sixty seconds and a forced reconnect after two minutes of silence, sends over JSON-RPC in four-thousand-character chunks, sends typing indicators, downloads attachments into a bounded cache. Reference: ZeroClaw's channel at `~/Code/zeroclaw/crates/zeroclaw-channels/src/signal.rs` (the whole shape); OpenClaw's daemon supervision at `~/Code/openclaw/extensions/signal/src/daemon.ts`, its client at `src/client.ts`, and its reconnect at `src/sse-reconnect.ts`; Hermes' health probe and echo filter at `~/Code/hermes-agent/gateway/platforms/signal.py`, its formatting at `signal_format.py`, and its rate limiter at `signal_rate_limit.py`. Tests: everything against the fake signal-cli: a message in and a reply out; a dropped stream that reconnects; a stalled stream that is forced to reconnect; an attachment; a message over four thousand characters split correctly; a fuzz test on the event decoder.

**Brief 5.2: pairing and the sender list.** Package `internal/signal/pairing`. Unknown senders get an eight-character code from a thirty-two-character alphabet; codes expire in an hour; three pending at most; one request per sender per ten minutes; lockout after five failures; codes stored salted and hashed and compared in constant time; the approved list stores Signal ids, not phone numbers. Reference: Hermes' pairing at `~/Code/hermes-agent/gateway/pairing.py` and its authorization at `gateway/authz_mixin.py`; OpenClaw's pairing store at `~/Code/openclaw/src/pairing/pairing-store.ts` and its Signal policy in `extensions/signal/src/monitor.ts`. Tests: every rule above as a unit test; a fuzz test on the code parser.

**Brief 5.3: the Signal channel.** Package `internal/channel/signal`. The six channel functions on top of the client: one message per reply, a "still working" note once after five idle minutes, a preview as the actual post text with the approve and deny replies, a handoff with a screenshot, inbound photos and files saved to an inbox the model can read. Tests: the functional tests from wave 4 repeated through the Signal channel against the fake signal-cli.

**Brief 5.4: the vault.** Package `internal/vault`. An `age`-encrypted file with a key file the agent's user alone can read. Entries: site, domains, username, password, TOTP secret. Entry only through a masked prompt in the terminal. A resolver that turns `secret://name` into a value at call time and never returns it to the model. A TOTP generator. The sudo entry feeding `SUDO_ASKPASS`. A redaction function that replaces secret-shaped strings in every reply, log line, and tool result. Reference: ZeroClaw's secrets at `~/Code/zeroclaw/crates/zeroclaw-config/src/secrets.rs` (encrypted file with a 0600 key); Hermes' redaction in `~/Code/hermes-agent/tools/browser_tool.py` (search for `_redact_browser_output`). Tests: round-trip; the key file permissions are checked and a loose key is refused; the resolver; TOTP against a known vector; redaction on a fixture with ten secret shapes; a fuzz test on the entry parser.

**Brief 5.5: reliability.** Package `internal/reliability`. The crash-loop breaker (three restart-interrupted boots in five minutes skips auto-resume but keeps serving), one lease per session with a five-second wait and a reject on timeout, the delivery ledger with the "may be a duplicate" marker and three attempts over twenty-four hours, the lifecycle sentinel that detects an unclean exit and runs a quick integrity check, the drain marker with a boot id and a thirty-minute expiry, one deadline primitive for the fifteen-minute turn and seven-minute tool limits, and the systemd watchdog feed from the main loop. Reference, one Hermes file per feature: `~/Code/hermes-agent/gateway/restart_loop_guard.py`, `gateway/turn_lease.py`, `gateway/delivery_ledger.py`, `gateway/lifecycle_ledger.py`, `gateway/drain_control.py`, `agent/deadline.py`, `gateway/session_db_recovery.py`. Tests: each mechanism against the fake clock; the kill-mid-turn functional test now asserts the duplicate marker; a corrupt database file is renamed and the nightly backup restored.

### Wave 6: Memory and skills

After this wave, Coeus learns and remembers.

**Brief 6.1: memory files and the index.** Package `internal/memory`. `MEMORY.md` and `USER.md` with their character caps, a `memory/` folder of markdown, and a full-text index over all of it plus every past message, using SQLite FTS5. Every fact carries a source and a date. Supersede instead of delete. Search, get, and save with atomic batches. The three-line memory hint from the top matches. Reference: Hermes' memory tool and caps at `~/Code/hermes-agent/tools/memory_tool.py`, its session search at `tools/session_search_tool.py`; OpenClaw's memory contract at `~/Code/openclaw/extensions/memory-core/src/memory-tool-contract.ts`. Tests: the caps are enforced; supersede keeps the old fact searchable; the hint is empty when nothing matches; the twenty-question memory test against a recorded session.

**Brief 6.2: harness-written memory.** Package `internal/memory/capture`. Zero-token capture from the event log: files changed, commands run, sites visited, jobs created, and any user message beginning with "no," "actually," "always," "never," or "don't," saved word for word as a correction. Tests: each capture rule; a correction is retrievable by search the same turn.

**Brief 6.3: the skill format and the registry.** Package `internal/skill`. The folder format from section 8 of the plan, a loader that puts names and one-liners in the prompt and loads bodies on use, the permissions block enforced by the harness, the changelog with rollback, and the router hook that runs a skill without the model when a trigger matches. Reference: Hermes' skill format at `~/Code/hermes-agent/tools/skills_tool.py`, its ledger at `tools/skill_ledger.py`, its `/learn` prompt at `agent/learn_prompt.py`; OpenCode's skill loader at `~/Code/opencode/packages/opencode/src/skill/index.ts`. Tests: a golden skill folder loads; a skill with a step over its limit is refused; rollback restores the previous version; a trigger runs the skill through the fake channel with the fake model never called.

**Brief 6.4: learning skills from docs and from tasks.** Package `internal/skill/learn`. Two paths: the model reads documentation and writes a skill with the commands it will use and one example each, then runs the smoke test; and, at the end of a task, the model offers to save the steps as a skill, which the user approves. Reference: ZeroClaw's skill creator at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/skills/creator.rs` (the reflection prompt and the dedupe). Tests: a scripted docs page produces a skill that passes its own smoke test; a completed task produces an offer, and approval saves a valid folder.

**Brief 6.5: memory and skill functional tests.** Folder `test/functional/`. A task that saves a fact and a later task that recalls it; a correction that survives across sessions; a skill taught from docs then triggered by a message; the after-action review writing to memory; the monthly memory test harness with its twenty questions.

### Wave 7: The browser

After this wave, Coeus uses Chrome like a person. This is the centerpiece, and it is the only wave with TypeScript in it.

**Brief 7.1: the browser worker.** Folder `worker/browser/`, TypeScript on Node, `playwright-core`. Launches the real Chrome binary with a dedicated profile folder and a loopback DevTools port with a per-launch token, attaches with `connectOverCDP`, and speaks JSON-RPC over stdio to the Go side. Provides: snapshot (the tree with refs, new-element marks, the below-the-fold count, collapsed lists, and dialog state), labeled screenshot, click, type with per-key pacing, press, scroll, and the act batch, each with the settle wait and the diff. Stale refs are re-found by role and name, then text. Reference: OpenClaw's launch flags and profile at `~/Code/openclaw/extensions/browser/src/browser/chrome.ts`, its attach at `extensions/browser/src/browser/pw-session-cdp-transport.ts`, its action limits at `extensions/browser/src/browser/act-policy.ts`, and its tool schema at `extensions/browser/src/browser-tool.schema.ts`; browser-use's page serializer at `docs/reference/browser-use/serializer.py` (new-element marks, scroll hints) and its per-step evaluation fields at `docs/reference/browser-use/views.py`; Moltis's manager at `docs/reference/moltis/manager.rs` and snapshot at `docs/reference/moltis/snapshot.rs`. Tests: against recorded page fixtures served locally: every action; a stale ref that is re-found; a click that produces no change and reports it; a settle that times out; a dialog; a download; a new tab; a snapshot under the character cap on a large page.

**Brief 7.2: the Go side of the browser.** Package `internal/browser`. Starts the worker on the first browser tool call, keeps it alive thirty minutes after the last, restarts it if it dies, and turns the seven browser tools from section 9 of the plan into worker calls. Applies the human pacing profile and the per-site daily action budget. Tests: the worker is spawned once and reused; a dead worker is restarted and the tool reports the interruption; the daily budget refuses the next action and says so.

**Brief 7.3: login and handoff.** Packages `internal/browser/login` and `internal/browser/handoff`. Login takes a site name, finds the vault entry, checks the page is on one of the entry's domains, types the username and password and the TOTP code into the fields the model pointed at, and reports success or the wall it hit. Handoff brings the window to the front, sends a screenshot and a numbered list through the current channel, and blocks the task until the user replies done, a code, or abort. Tests: a login against a fixture page; a login attempt on the wrong domain is refused; a handoff round-trip through the fake channel.

**Brief 7.4: browser skills.** Package `internal/skill/browser`. Recording: each step's intent, element descriptor (role and name, then visible text), and expectation are logged while the user or the model drives. Replay: resolve each step by the cascade, act, settle, check, with no model call. Self-heal: on a failed step, ask the model for the element matching the intent, verify, and propose a one-line patch for the user to approve. Reference: Stagehand's observe-then-act cache idea, described in `docs/research/16-browser-agent-spec.md`. Tests: record a three-step fixture flow and replay it with the fake model never called; change the fixture so one step fails and confirm a patch is proposed and not applied.

**Brief 7.5: browser functional tests.** Folder `test/functional/`. Log in to a fixture site through the vault; post a message with a preview and approval; hit a captcha fixture and hand off; replay a recorded skill; the visual QA skill walks a fixture app and produces a report.

### Wave 8: Always on

After this wave, Coeus runs jobs, uses the desktop, and updates itself.

**Brief 8.1: scheduled jobs.** Package `internal/schedule`. One job object with at, every, or cron and a timezone, a prompt or a command, a delivery target with a separately validated failure lane, and state. One timer clamped to sixty seconds. Claim with an atomic update. Backoff of thirty seconds to sixty minutes, auto-disable after ten failures with a message to the user, one incident per distinct error, a sixteen-kilobyte notepad per job, a monitor mode that hashes output and wakes the model only on change, and a rule that a job can never call `schedule` or restart the agent. Reference: ZeroClaw's job types, store, and scheduler at `~/Code/zeroclaw/crates/zeroclaw-runtime/src/cron/types.rs`, `store.rs`, and `scheduler.rs`; OpenClaw's backoff and auto-disable at `~/Code/openclaw/src/cron/service/jobs-scheduling.ts` and `service/auto-disable.ts`; Hermes' incidents, notepad, monitor, and restart guard at `~/Code/hermes-agent/cron/incidents.py`, `cron/notepad.py`, `cron/monitor.py`, and `cron/lifecycle_guard.py`; Prime's heartbeat-as-job at `~/Code/prime-agent/packages/coding-agent/src/core/cron-jobs.ts`. Tests: every rule against the fake clock; two processes cannot claim one job; an unattended job that hits an ask stops and reports.

**Brief 8.2: the desktop worker.** Folder `worker/desktop/`, TypeScript on `@trycua/cua-driver`, and package `internal/desktop`. Launch or focus an app, screenshot with numbered marks, click, type, key, drag, clipboard, with the same pacing and act-and-assert loop as the browser, an app granted once per session, and a preview for every irreversible action. Reference: Hermes' computer-use tool at `~/Code/hermes-agent/tools/computer_use/` and the cua-driver contract in `~/Code/openclaw/extensions/cua-computer/`. Tests: against a fixture window: each action; an app that was not granted is refused.

**Brief 8.3: the updater.** Package `internal/update`. Build into `releases/<version>/`, keep the previous three, switch a symlink, restart, require `/readyz` within sixty seconds, and switch back on failure. Forward-only numbered database migrations, each in a transaction after a backup. An older binary refuses a newer schema and names the version to use. Reference: ZeroClaw's updater at `~/Code/zeroclaw/src/commands/update.rs` (backup, swap, smoke test, rollback). Tests: install a binary that fails readiness and confirm the rollback within sixty seconds; a migration that fails leaves the backup intact.

**Brief 8.4: replay as test and nightly checks.** Package `internal/replay`. Take any task from the event log and re-run it against the current code with the recorded tool results, as a Go test the user can invoke with one command. A nightly job that runs the memory test and the recorded skill tests and reports one line. Tests: replay a recorded failing task, apply a fixture fix, replay again and pass.

**Brief 8.5: the final functional pass and the release.** Folder `test/functional/` and `docs/`. Every functional test from every wave runs green on a clean Linux machine from a fresh clone with one command. The forty-step task runs against a small local model and a cloud model with the same three assertions. `docs/PROGRESS.md` is complete. The README explains the logic the way section 1 of the plan does, and it tells a new user how to install and pair in ten lines.

---

## Part 4: What the orchestrator watches for

These are the ways this kind of project usually goes wrong, and what to do about each.

- **A worker adds a library because it is convenient.** Reject it unless the reason is written in `docs/DEPENDENCIES.md` and the standard library truly cannot do the job.
- **A worker writes the code first and the test after.** The commit history shows it. Reject and ask for the test to be written to fail first.
- **A worker copies code instead of porting the design.** Rust and TypeScript idioms in Go are the sign. Reject and point at the reference again.
- **A package grows past its one sentence.** Split it before the next wave.
- **A test passes by accident.** The orchestrator changes one thing the test is supposed to catch and confirms the test fails.
- **A fake drifts from the real thing.** Every fake has a contract test that also runs against the real service when it is available, marked with the build tag `live`.
- **The prose slips.** The style checker from brief 1.5 runs in `make test`. A wave with violations does not pass.

---

## Part 5: The code bases to read

Every reference path in this plan is listed here with what it is for, so a worker can find it in one place.

| Project | Where it is | What a worker reads it for |
|---|---|---|
| OpenClaw | `~/Code/openclaw` | The loop with hooks, the tool-call repair grammar, the loop detector, the retry refund, the cache boundary, the Signal daemon and client, the cron backoff, the pinned DNS lookup, the external-content wrapper, the delivery queue, the pairing store, the boot check, the systemd unit, the browser launch and attach |
| Hermes | `~/Code/hermes-agent` | The three prompt tiers and cache points, verbatim user messages in compaction, the memory tool and caps, session search, pairing, the restart-loop guard, the turn lease, the delivery ledger, the lifecycle sentinel, the drain marker, the deadline primitive, the sudo prompt, cron incidents and notepad and monitor, the Signal platform, the skill ledger, the learn prompt |
| Prime | `~/Code/prime-agent` | The provider types and two API files, overflow detection, goals, autonomous budgets, refinement with rollback, cron as heartbeat |
| OpenCode | `~/Code/opencode` | The permission engine and prefix table, the tool registry and text descriptions, read, edit, grep, glob, truncation, invalid calls, the retry policy, instruction re-reading, the command registry, the startup spinner rules, keybinds, the inline approval prompt, the skill loader |
| ZeroClaw | `~/Code/zeroclaw` | The channel trait, the Signal channel, the sender allowlist, the cron types and store and scheduler, the tool-call parser, the turn cap, the loop detector, the Landlock wrapper, the updater, the skill creator, the encrypted secrets |
| browser-use | `docs/reference/browser-use/` | The page serializer with new-element marks and scroll hints, the per-step evaluation fields |
| Moltis | `docs/reference/moltis/` | The Chrome manager with a persistent profile, the snapshot with numbered refs |
| Codex | `docs/reference/codex/` | The shell tool spec with the justification field, the Landlock rules |
| The research | `docs/research/` | Seventeen studies with line-level citations, and two fact-check reviews, for any question a reference file does not answer |
