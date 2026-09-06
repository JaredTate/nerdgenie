# Nerd Genie progress

This file is the record of how the build went, one section per wave. Each section
holds three lines from the orchestrator at the wave gate saying what was built and
what it cost, the results of `make live` against the three real models with the
token cost of each run, and the notes from the human trial where the wave had one.
It is written as the work happens, not afterwards, so that the next wave starts
from what is true rather than from what was planned.

---

## Wave 0: ready to build

Brief `docs/briefs/wave-0/0.1-ready-to-build.md`. One worker.

### The development machine

Every check below was run on `jared-irene` by the wave-0 worker. The last column
is what was actually seen, not what was expected.

| What | The command used | What was seen |
|---|---|---|
| Go | `go version` | go1.27.1 linux/amd64, at `/usr/local/go` |
| staticcheck | `staticcheck --version` | staticcheck 2026.2.1 (0.8.1), at `~/go/bin` |
| signal-cli | `signal-cli --version` | signal-cli 0.13.23, at `~/bin/signal-cli` |
| bwrap | `bwrap --version` | bubblewrap 0.9.0, at `/usr/bin/bwrap` |
| ripgrep | `~/.local/bin/vendor/rg --version` | ripgrep 14.1.1 — **not on the PATH**; see the note below |
| Chrome | `google-chrome --version` | Google Chrome 151.0.7922.137 |
| Node | `node --version` | v24.18.0, through nvm |
| Landlock | `cat /sys/kernel/security/lsm` | `lockdown,capability,landlock,yama,apparmor,ima,evm`, so Landlock is on. Kernel 6.17.0-40-generic |
| The local model | `curl -s http://127.0.0.1:19091/health` | `{"status":"ok"}` |
| The local model's window | `curl -s http://127.0.0.1:19091/props` | `n_ctx` is 262144, model `~/llm/models/hauhau-Q4_K_P.gguf` |
| The local model answering | one chat completion for `local-coder` | replied "ready", 19 prompt tokens, 2 completion tokens, about 43 tokens a second on this short call |

**The local model** is the Qwen 3.8 27B Uncensored GGUF at
`~/llm/models/hauhau-Q4_K_P.gguf`, served by the TurboQuant `llama-server` on one
graphics card, speaking the OpenAI-compatible API at `http://127.0.0.1:19091/v1`
under the model name `local-coder`. It is **not** Ollama, and nothing in wave 0
touched Ollama.

**One thing differs from the brief's table.** There is no `rg` on the PATH. The
only ripgrep on this machine is the copy vendored inside other tools, at
`~/.local/bin/vendor/rg` (14.1.1) and `~/.cache/opencode/bin/rg` (15.1.0); the
`rg` a shell finds is a shell function, not a program, so `command rg` finds
nothing from a script. The `search` tool in brief 2.5 runs ripgrep as a program,
so ripgrep needs to be installed system-wide before wave 2. That needs `apt` and
therefore sudo, which the wave-0 worker does not have, so it is reported rather
than fixed.

### The reference projects

Verified with `git rev-parse --short=12 HEAD` in each folder. Nothing in them was
changed. All five match the table in `docs/WORK_PLAN.md` wave 0.

| Project | Folder | Commit | Matches the plan |
|---|---|---|---|
| OpenClaw | `/home/jared/Code/openclaw` | `752a983480e4` | yes |
| Hermes | `/home/jared/Code/hermes-agent` | `95f62ca3bfcf` | yes |
| Prime | `/home/jared/Code/prime-agent` | `0ba0423c5c18` | yes |
| OpenCode | `/home/jared/Code/opencode` | `69c172e8a7c0` | yes |
| ZeroClaw | `/home/jared/Code/zeroclaw` | `73ff732e66c7` | yes |
| HomeRecon | `/home/jared/Code/homerecon` | `77cced235069` | no commit is pinned for it |

### What wave 0 built

The skeleton (`go.mod`, the MIT licence, the `Makefile` with every target from
Part 1 of the plan, the continuous-integration workflow, `docs/DEPENDENCIES.md`,
and the four scripts), `internal/contract` in full, `internal/testkit` with a fake
and a contract check for every interface, `internal/lint` with its seven rules,
`scripts/repomap` with its drift test, the skeleton of `cmd/coeus`,
`worker/browser/PROTOCOL.md`, the forty-step fixture, and the sample functional
test. Nothing outside the standard library is imported.

### Coverage at the wave-0 gate

| Package | Line coverage | Threshold |
|---|---|---|
| `internal/contract` | 99.3% | 90% |
| `internal/lint` | 93.6% | 90% |
| `internal/testkit` | 91.0% | 90% |
| `scripts/repomap` | 92.5% | 90% |
| `scripts/stylecheck` | 95.5% | 90% |
| `cmd/coeus` | 96.3% | measured, not gated |

### Live results

None. There are no `live` tests until wave 1, and `make live` passes trivially.
The three real models are the local Qwen 3.8 through the llama-server daemon,
Opus 4.8 through `claude -p` on the user's Claude subscription, and GPT-5.5
through `codex exec` on the user's ChatGPT subscription; from wave 1 their results
and token costs go here. There are no API keys on the machine and none are wanted.

### Human trial

None. The first trial is at the wave-3 gate.

### The gate

Merged into `main` on 2026-09-02 as one merge commit after `make check` passed on the branch tip and again on `main` (every package above ninety percent, the fuzz smoke clean). The orchestrator read `internal/contract` in full and found it true to the design; the ripgrep gap the worker reported was closed by installing it system-wide (`/usr/bin/rg` 14.1.0). Three additions to `internal/contract` followed as orchestrator commits, because waves 3 and 4 need them: the job interface gained the calls that hand a finished task's report to its job and ask for the next task, the file-change event got a body shape for `/undo`, and the home layout got a Signal folder.

A reviewer with fresh eyes then read every other new file and found twenty-five problems, nine of them in safeguards later waves lean on: the coverage gate skipped untested packages, the forty-step fixture never made the model write to the record and never fired its stop, the fake provider server ignored the script's expectations and drifted from both wire protocols, the repo-map fixture could read the wrong tree, the sandbox contract check asserted nothing, and the borrowed-design rule was silent for seven of nine projects. Wave 0 does not pass its gate as merged. The fixes are brief `docs/briefs/wave-0/0.2-fix-the-wave-0-gate.md`, run as a fix wave alongside wave 1, and the gate closes when it merges.

---

## Wave 1: the foundation, and the wave 2 packages that needed only the contracts

Briefs `docs/briefs/wave-1/1.1` to `1.5`, plus `2.2`, `2.3`, and `2.4` from wave 2, which depend only on the contracts and so ran alongside. Five to ten workers at once, per the user's pacing rule, each in its own worktree, merged with `--no-ff` after `make check` passed on the branch and again on `main` in a clean checkout.

### Merged so far

| Package | Brief | Coverage | Notes from the gate |
|---|---|---|---|
| `internal/log` | 1.1 | 93.6% | A read that fills its cap of ten thousand events returns an error naming `ByRange`, never a silent truncation. The crash test kills a helper by exact pid mid-write and finds no gap. |
| `internal/config` | 1.3 | 96.3% | TOML keys are the snake_case tags on the contract (`default_model`, `base_address`); a squashed key is refused as unknown. The default work folder is `~/coeus`, and the whole home directory is refused as a root. |
| `internal/repair` | 1.5 | 98.5% | Five envelope shapes, name repair, the two-failures rule, and `<think>` blocks stripped before any shape is read. |
| `internal/provider` | 1.4 | 92.7% | Three providers: Anthropic, OpenAI-compatible, and `cli` for the two subscription programs. |
| `internal/permission` | 2.2 | 97.9% | The three shipped entries as data, the reduced form, and `RulingStop` for an unattended run. |
| `internal/sandbox` | 2.3 | 96.1% | Real bwrap and Landlock with zero skips, once an AppArmor profile for bwrap was installed on this machine (Ubuntu 24.04 blocks unprivileged user namespaces without one). |
| `internal/clock` | orchestrator | 100% | The real clock behind `contract.Clock`, which no brief owned. |

Still open from this batch: `1.2` (the record), which is rebasing after a log-key ruling, and `2.4` (the vault), which is applying three contract additions and a `/vault add` usability change.

### Live results

The provider contract test ran once against all three real models, from an empty scratch folder, with `internal/clock` as the clock:

| Model | Reached through | Text reply | Tool call | Cost |
|---|---|---|---|---|
| Qwen 3.8 (`local-coder`) | the llama-server daemon on port 19091 | 315 in, 290 cached, 24 out | 318 in, 290 cached, 25 out | free |
| Opus 4.8 | `claude -p`, the Claude subscription | 392 in, 0 cached, 36 out | 401 in, 0 cached, 39 out | about 0.8 cents |
| GPT-5.5 | `codex exec`, the ChatGPT subscription | 8565 in, 7552 cached, 20 out | 8568 in, 4480 cached, 112 out | no dollar figure reported |

The codex program sends about 8,300 tokens of its own environment context on every call regardless of the prompt. The whole run cost about 1.1 cents of subscription.

### Contract changes made at this gate, all by the orchestrator, each tests first

Snake_case TOML tags; the ask-me-first list and the user's permission rules in the configuration; `RulingStop`; the credential prints as `[secret]` on every path; `TerminalChannelName`; `Reply.Model` and `Usage.CostUSD`; the work folder as the default sandbox root and the rule that a root may not hold an excluded path; `RecordLogKey`, so task 17 and job 17 never share a stretch of the log; and the browser protocol's twelfth method `dialog`, a wall on a snapshot, a settled flag on a diff, and the written expectation rule, all from the browser worker's findings.

### The wave 1 gate review

A reviewer with fresh eyes read the six merged packages and found twenty-three things, each proven on a scratch copy. Log, configuration, repair, and the clock pass their gates as merged. Permission, sandbox, and providers do not: the reduced form that the ask-me-first list matches on could be evaded by a long command that truncates, by `sh -c`, by `/usr/bin/sudo`, and by a subshell; a symbolic link as a sandbox root bound the SSH keys for real; a moved agent home left the vault inside the fence; and a retried model call replayed streamed text. The contract changes those need are on `main` (linked roots resolved, the excluded paths following the agent home, an output cap per call), and the fixes are brief `docs/briefs/wave-1/1.6-fix-the-wave-1-gate.md`, run by one fix worker per package. Also merged since: the record (95.4%), the vault (91.9%), and the browser worker, which is TypeScript on Playwright against a real Chrome 151 with 279 tests and 88% coverage; its seven protocol findings became a twelfth method, `dialog`, a wall on a snapshot, a settled flag on a diff, and a written expectation rule.

### To wire at the wave 3 gate, in the orchestrator's two files

Subcommand values for the table in `cmd/coeus/main.go`: `sandboxEntrySubcommand` (`sandbox_entry.go`, hidden), `askpassSubcommand` (`askpass.go`). Command values and constructors for `serve.go`: `vault.NewCommand(store)` (terminal only); `permission.New(config, clock)`; `provider.New(alias, options)` with `provider.WithRetries` and `provider.NewChain`, all on `clock.System()`; `log.Open(home.DatabaseFile())`; `config.Load(home)` and `config.Doctor`; `sandbox.New` from the configuration's roots. The fix worker's finding 18 lands the exit-code mapping in `main.go` first.

### Human trial

None until wave 3.

## Waves 2 to 6: everything merged, the first human trial, and the security review

Every brief from waves 2 to 6 has a package on `main`: 61 packages under `internal/`, 131717 lines of Go of which 71722 are tests (2471 test functions), and 10005 lines of TypeScript in the two workers. The real turn loop is wired into `coeus serve` with the tools, the skills, the jobs, the memory, the record, and the browser worker; `make build` builds the two worker bundles; the coverage gate at the last green run had every package above ninety percent (the terminal screen at 91.0%, its floor is seventy) except `cmd/coeus` at 58.4%, which the gate does not count and the wiring brief must raise.

### The first human trial, on the local Qwen 3.8, 2026-09-02

The person opened the screen against `coeus serve` on the local model and typed a message; the reply came back through the real loop. Their notes, in their words, are brief 3.7 (`docs/briefs/wave-3/3.7-fix-the-first-trial.md`): no context measure in the header ("needs to look like opencode does that"), Escape does not interrupt the model, nothing shows the model is alive while it thinks, no browser tools, and how to see jobs and tasks. Causes found: the wiring sends only the model, task, state, health and command list in the status, so the header had nothing to draw; Escape sent stop only while a task ran and a plain reply is not a task; the worker bundles were not built. Four status fields and a record line were added to the contract for it, tests first, and the screen and wiring workers hold the rest.

Two things the trial cost that were not the product. The desktop worker was blamed for waking the GNOME screen reader; the security review traced it to the browser integration tests starting a real Chrome with the session bus, which brings up the accessibility bridge. The screen reader setting is now locked off through dconf on this machine, Chrome is started with `NO_AT_BRIDGE=1` and no session bus, and the browser and desktop integration tests left `make test` for their own target that runs Chrome headless. And the orchestrator closed two terminal windows it had opened by killing a parent pid that turned out to be the terminal server, which closed every terminal on the desktop; the rule is now in its memory.

### Driving the Tetris prompt through the socket

The person's acceptance test is a two-thousand-word prompt asking for a complete Tetris game with tests, built on the Desktop and played in Chrome. The orchestrator drives it through a socket script that attaches, sends the prompt, answers previews, and logs every envelope, so the model's tool calling can be read from the event log. Three runs so far:

1. The model wrote the done list as a list of strings; the task tool refused every one of seven identical tries, and the summary the record kept said "updated the record: one change" each time. Fixed in `internal/tool/task`, tests first: a done line may be a plain string, and a refusal names the shape.
2. The model wrote the why in the `text` field; refused three times. Fixed the same way: the why is taken from `text`, a list may be one string, the done list one line, a line number a string.
3. The record was built correctly (why, done list, stop list) and the model moved to shell calls. It read `$HOME` inside the fence, which is a scratch folder, and built the project there instead of on the person's Desktop, which was a sandbox root. The fence's home must be the user's real home path with only the roots present under it; the sandbox worker holds it with an integration test.

Run 3 kept going: after the record, the model wrote the engine and its tests, ran them, and worked the failures down by hand, one edit and one test run at a time; at forty-five tool calls its whole engine suite passed (fifty-one tests, none skipped), and it went on to write the hazard tests before the code. About forty seconds per call on the local model, half of it re-reading context that the tail layout now keeps cached.

Also found on the way and assigned: no tool line reaches the screen, so tool calls are invisible; every checkpoint and tool result event carries a zero time; a read-only command such as `/status` from a second screen gets no answer while the model is busy; a failed record write is summarised as a change; `browseract` cannot yet carry a scroll step although the worker and the contract can.

### The security review (brief 6.5)

Twenty-three findings in `docs/SECURITY_REVIEW.md`, with a failing test for each. One critical: a page could save a skill whose `site:` line held a star, which became a standing approval matching every call, including the three shipped ask-me-first entries. Fixed in `internal/skill` (a site is a bare host name, the pattern is escaped and anchored, an approval covers only browser calls on that host). The rest are held by one worker per package: permission (standing approvals never outrank the ask-me-first list or the unattended stop; flags before `-rf`; wrappers such as `nohup`, `xargs`, `env -i sh -c`), sandbox (limits, loopback, user namespaces, PATH and HOME, DNS), the web guard (every non-public range, allow-listed hosts still checked, redirects), the browser profile outside the fence, results marked as data in the working context, Signal (a secret split across messages, per-sender pairing lockout), and the askpass helper under sudo. The fix branches carry the review's failing tests, so they merge together once all pass, and `main` stays green.

### Live results

Not yet run for waves 2 to 6 beyond the provider contract test and the context builder's live test, both green on all three models at the wave 1 gate. The whole `make live` runs at the wave 6 gate.

### The fix waves after the review, 2026-09-03

All twenty-three security findings are fixed and merged, one worker per package, each with the review's failing test first: the skill permission slip, the permission ordering and the shell disguises, the sandbox (DNS inside the fence, the network off by default with one setting the shell tool turns on, process and memory limits, the user-namespace filter, the fence's home as the person's real home path), the web guard's ranges and redirects, the browser profile outside the fence, results marked as data in the working context, Signal's redaction and pairing, and the askpass helper under sudo. The fresh-eyes review of the wave 4 to 6 packages (brief 6.7, eighty-nine findings) followed: the loop, the file tools, the shell and task tools, the desktop, the browser skill, reliability, jobs, the updater and replay, the context builder and the record, and the wiring each had a worker; all but the loop, the file tools, and the wiring have merged as this is written. Two contract changes were made for them, tests first: every desktop action carries what the model expected, and the skill store is told who saved a skill.

Two numbers from the context builder after the layout change: consecutive rounds of the forty-step fixture share 84, 86, and 87 percent of the prompt at rounds 10, 20, and 30, where the reviewer had measured 48, 38, and 33; a memory save mid-task now leaves 90 percent in place instead of 21.

### Live results after the fix waves, 2026-09-03

`make live` on main, the two live suites that exist (the provider contract test and the context builder's forty-step fixture) against all three real models: green, with the context suite taking 98 seconds on the local model. The functional suite against the real models, which `CLAUDE.md` promises under the same target, is not built yet; a worker was started for it and cut off by the session limit, and it resumes when the limit lifts. The loop and file-tool sections of brief 6.7 merged after their workers were cut off; what each still owes is written in the orchestrator's memory and in the wiring worker's list.

### Streaming to the screen, 2026-09-03

With the workers locked out by the session limit, the orchestrator built brief 6.8 itself, tests first in each package: the provider streams each attempt's words as they arrive and calls a reset before the next attempt's first word (`Options.OnReset`); the channel's `SendDelta` redacts across pieces with a held tail of sixty-four runes, gathers pieces for thirty milliseconds, and withdraws shown text the instant a secret is recognised inside it; the screen takes a withdrawn reply down; the wiring fills the loop's `Deltas` and the provider's reset. The functional suite proves pieces of the answer reach the screen before the reply and are the start of it.

The desktop is wired the same night (finding 48): opened beside the browser at start, closed with it, handed to the computer tool, with a functional test that a binary with no worker bundle says so.

For the wave 5 and 6 trial: `docs/TRIAL.md` writes the checklist as commands, `go run ./scripts/fixturesite` serves the fixture site for a person, and `scripts/trial/bad-release.sh` stages a good release and a bad one so the rollback can be watched.

After streaming and the desktop wiring landed: `make live` green again on all three real models (the provider suite 46 seconds, the context fixture 38 seconds, the functional suite 110 seconds), and `make test-browser` green headless on the shared fixture site with the screen reader settings unchanged and no Orca process before or after.

### The turn loop after the wave 6 review

The loop worker's section landed with a test per finding: a stop line the model wrote no longer fires on a tool result that repeats two of its words (the model declares its own stop lines with `stop_now`); polling a long command is no longer "the same call over and over"; a command in backticks in a done line goes through the permission function and the log as the shell call it is; the model can pin evidence in front of itself (`pin_evidence`, four at most); a question without a question mark waits for the person in two calls, not six; an unattended task offers no skill to nobody; fifteen bounds are pinned by literals; a budget that runs out costs one report and one call; a done line only the answer can prove names `reply` and closes when the answer is given; and every tool call runs inside the reliability guard's deadline.

Bubble Tea 2 replaced Bubble Tea 1 and lipgloss on 2026-09-03, a dependency change the orchestrator approved: version 1 asked the terminal for its background colour from package `init` and read the answer one byte at a time with a five-second wait per byte, eating whatever the person typed in the first seconds on a terminal that never answers. A real pseudo-terminal test in `internal/tui` types at one second and sees the letters; the golden frames are byte-for-byte unchanged, and nothing in the binary can put a blocking question to a terminal again.

### The live tier, 2026-09-03

`make live` now runs the functional suite against the three real models through a real `coeus serve`: one task that needs a tool ("write hello coeus to greeting.txt and read it back"), asserted on the file, the record's round trip, and the done-check, and the forty-step fixture with its three assertions held before and after a real model call. Numbers from the first full run:

| test | model | wall | tokens in (cached) | out | cost |
|---|---|---|---|---|---|
| fixture | local Qwen 3.8 | 3 s | 4,319 (3,728) | 31 | free |
| fixture | Opus 4.8 | 3 s | 3,960 (3,721) | 47 | $0.0064 |
| fixture | GPT-5.5 | 8 s | 11,666 (7,552) | 113 | not reported |
| one-tool task | local Qwen 3.8 | 20 s | 19,652 (16,139) | 264 | free |
| one-tool task | Opus 4.8 | 17 s | 14,353 (3,721) | 641 | $0.1288 |
| one-tool task | GPT-5.5 | 38 s | 63,417 (32,640) | 1,049 | not reported |

Two findings came with it. GPT failed the one-tool task about half the time because the codex program ran with its own shell tool on and the model wrote the file itself inside the program's read-only sandbox, bypassing the harness; the program now runs with `shell_tool` and `unified_exec` off, as the claude side already did, and the test passed three times running. And no model writes a done list for a small task, so the task ended "waiting" rather than "done"; the live home carries a persona line asking for one, and the loop worker holds the rule that an answer with no question and no done list closes the task with the answer as its proof.

The browser worker now reports a person's own clicks, typing lengths, and navigations over the protocol, and `/walk record` writes them down until `/walk stop`.

A message now carries on the task that is waiting for it: an answer to the model's question resumes the same task, and "continue" picks up one stopped at its budget or by Escape, one task back per screen and never further (three functional tests and eight unit tests, `cmd/coeus/resuming.go`).

### The loop's last brief 6.6 items

The model may ask for several tools in one reply (the instruction text says so in one sentence and the forty-step fixture's round 39 does it); one checkpoint is saved per model call and only the first carries the ask, so a forty-round task with a two-thousand-word ask writes 105 kilobytes of checkpoints where it wrote 3.1 megabytes; an answer that asks nothing closes a task that wrote no done list, with the answer as its one done line; and a stopped task the person continues gets a fresh budget while a waiting one keeps what it had. The fuzzer found the Signal splitter could hand signal-cli an empty piece for a reply of line ends; every piece now holds something a person can see.

The fourth Tetris run, on its own home with the network in the fence, found the four-gigabyte address-space bound the security review asked for: Node's test runner aborted inside the fence, because its engine reserves far more virtual space than it uses. The bound is gone, with a test pinning its absence and the reason; the process count and the two folder sizes stay. Qwen had already worked around it on its own by running the tests in forked processes.

### The fifth Tetris run and the night of 5 September 2026

The whole-game ask ran five times on the local Qwen 3.8 27B over the day. Runs one to four found the browser tool lying about a click, no page errors in any result, a guard that stopped instead of rewinding, a follow-up message that started a blank task, and a job whose tasks never started because the task that made it kept working; each was fixed with a test that was red first. Run five started at 17:57 on the fixes and is the first run measured by `scripts/runreport`, which reads one task's numbers off the log: rounds, minutes, calls by tool, replies with more than one call, rounds that only wrote the record, rounds that only ran the tests, tokens in and cached and out, rewinds and failures. The numbers below are from the log itself.

| task | what | ended | rounds | minutes | s/round | record-only | test-only | batched | tokens in (cached) | out |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | the person's task: engine, hazards, UI, then the browser | failed at its 223rd result, the record's size cap on the harness's own write | 223 | 87 | 24 | 42 (19%) | 30 (13%) | 0 | 11.5M (90%) | 85k |
| 2 | job task t1, scaffolding, verified on the existing folder | done | 23 | 2 | 4 | 16 | 1 | 0 | 323k (94%) | 2.7k |
| 3 | t2, the engine, verified | done | 12 | 2 | 11 | 4 | 2 | 0 | 209k (77%) | 1.7k |
| 5 | t3, the dragon | done | 10 | 2 | 15 | 3 | 0 | 6 | 202k (74%) | 2.2k |
| 6 | t5, the line-clear effects, tests added | done | 34 | 5 | 9 | 6 | 4 | 5 | 1.19M (95%) | 8.3k |
| 7 | t6, hazard safety | done | 11 | 2 | 9 | 5 | 1 | 1 | 157k (77%) | 1.5k |
| 8 | t7, the full suite green | done | 9 | 1 | 7 | 3 | 2 | 3 | 102k (83%) | 0.9k |
| 9 | t8, the Chrome play-test, first try | cut off by the 20:33 restart and, under the old rule, failed | 132 | 50 | 23 | 6 | 0 | 1 | 6.83M (90%) | 27.5k |
| 10 | t8 again, on the new binary | failed at 22:05 on the record's size, seventeen failures on it, five rewinds | 208 | 92 | 26 | 22 | 0 | 9 | 6.41M (77%) | 68.1k |
| 11 | t8 a third time, over nine sittings and as many harness fixes | done at 23:47, every done line proved; two rewinds, eight failures | 322 | 102 | 19 | 43 | 5 | 54 | 8.99M (75%) | 49.5k |
| 12 | t9, the visual QA pass at five sizes | done at 00:05, the board's clipping at small sizes found and fixed, no restart, no steer | 47 | 18 | 23 | 14 | 2 | 3 | 2.52M (89%) | 12.8k |
| 13 | t10, the final regression cycle | done at 00:25: 164 tests green, dragon, both yetis and the three clears re-driven through the page, restart verified, console clean | 63 | 20 | 19 | 14 | 1 | 3 | 2.47M (86%) | 10.2k |

What the numbers say. The engine and its 152 tests took 41 minutes and the person's task never batched a call, so a fifth of its rounds wrote only the record and an eighth only ran the tests, which is what the self-running tests and the checker after every change now fold away. The job's seven verification tasks took fifteen minutes between them at four to fifteen seconds a round on fresh windows of ten to forty thousand tokens, which is the hand-off working. The play-test task is where the run stands: the game's Start button runs a ghost-piece loop whose condition never changes, the page hangs, and the model, told so by the browser tool, has spent two hours writing play-test drivers through the shell rather than reading the draw code; the meter and the rewinds bound it, the record holds every stall, and the page question (`browser_read`'s `ask` field) is the tool it was missing, live since 21:33.

Fixed on this run, each with its test: the record trims its oldest result lines rather than failing the task; a task the program cuts off ends stopped and a job puts it down with its record; the shell tool refuses a kill by name pattern; the call fingerprint leaves out the fields that only say why; the hung-page message names the page's own script and what to look for; the progress meter and its ladder; the tests that run themselves and the checker after every change; the probe rule's two missed shapes; a cancelled call that no longer falls through the model chain; and the page question. What is open: the model's own reasoning on the hang, vision (the projector file on this machine is a broken link), the step mark on the model's first line, and the nightly set of tasks that would give every one of these a number before and after.

### The rest of the night: t8 in five sittings, 21:55 to 22:50

The play-test task of job 2 (t8, the eighth of ten) was the task that would not end, and every sitting of it taught the harness something, each built test-first and deployed by a restart with yolo turned on and the task picked up by "continue". The one bug the whole time was the ghost-piece `while` at main.js:514, whose body counts a number its condition never reads; the model diagnosed the sound engine, the render loop and its own drivers before it.

| sitting | what the model did | what the harness lacked | built |
|---|---|---|---|
| task 10, r150–r189 | wrote the same failure ten times over as F8–F16 while the meter said no progress; died at 3,004 tokens on a harness write, the failures alone 1,765 | no cap on a lesson's length or count, no refusal of a lesson already held | `ErrFailureAlreadyWritten` (four in five words shared is the same lesson); a lesson is one line of 160 runes, the newest eight stay, labels count on from the last held |
| task 11, sitting 1 | clicked Start with the honest hung-page message, then went back to Puppeteer drivers | the loop list covered the files this task changed, and this task had only changed its drivers | the list reads the scripts the page itself loads, from the local server or the disk, never another machine |
| sitting 2 | got twelve counted `for` loops of the renderer; the `while` at 514 fell off the end | line order, cap of twelve | `while` loops first, "and N more for loops" |
| sitting 3 | read the three whiles and went back to the sound engine; asked for r25 from line 798 and got the whole script, 13k tokens | nothing said which while; a past result ignored the offset | the while whose body never touches its condition is marked and listed first; a past result reads from an offset like a file |
| sitting 4 | re-read the marked result four times; the screen drew a green check on the refused click | the list on one result is read once; the pick-up lost the browser line; the tool line cut the word refused | the marked loop is the situation's browser line every round and survives a failed open; a pick-up replays the log's browser results; a tool line keeps "refused" on its end |
| sitting 5, 22:48 | read main.js:500–540, grepped the `while`, read `canPlace` in the engine, "confirm the ghost loop bug", and at 22:51 edited the loop so the cells advance with `ghostY`; at 22:53 the click on Start took the overlay away, and at 22:54 `browser_read` with `ask` answered `{"hasEngine":true,"state":"NORMAL","hasActive":true,"ticks":114}`: the game runs, forty minutes and four restarts after the click first hung | | |

| sittings 6 to 9, 23:05 to 23:44 | drove the play-test through `browser_read`'s ask: moves, drops, pause, restart, the dragon, both yetis, the three line clears, each answered with new state and no console errors; the meter read fourteen rounds without progress while it did; wrote a press step with the key under `text` three times; wrote a 1,919-character ask against a cap of 500; marked one step done four times; pinned four done lines and read the four results back; asked for a screenshot four times; and could not pin its last done line at all once the results list had trimmed, because a pin rewrites the whole list and line one's r228 had left it | the meter's list of reads, the press step's field names, the ask cap, a refusal of a step already done, the proof shown on a pin, a picture that went nowhere, and the proof rule | a browser read with a new intent is progress; a key under `text` or `press`; a 2,000-character ask; `ErrStepAlreadyDone` naming the next step; "r301 says: tests: all passing" on every pin and mark; a screenshot saved under `run/screenshots` with a line saying the model reads words in its place; any label the record reached is proof |

At 23:47 task 11 closed done, every done line pinned and every plan step marked, with the report that the core loop, the dragon, both yetis and the three line clears were verified through the page's own state and the suite was green: t8 done, job 2 at eight of ten, t9 next. The one steer a person typed after the loop hunt was the hidden dev panel and the ask field; the rest was the harness and the model.

Task 12, t9 (the visual QA pass across five sizes), ran from 23:47 to 00:05 with no restart and no steer: it wrote a Puppeteer script that measured the layout at each size, found the board clipping on narrow and short viewports, changed the stylesheet twice, measured again, ran the suite green and closed with every line proved. Eighteen minutes, one edit counted as progress by nothing, which is why the first change of a file now counts.

**Job 2 closed done at 00:25 on 6 September, ten of ten tasks**, six hours and twenty minutes after the person's ask at 18:05 the evening before: the engine and its 164 tests, the two hazards, the three line clears, the play-test through the browser, the visual QA at five sizes and the final regression, every task's done list proved and the game left running on port 8091. The first three tasks of the play-test cost the harness builder nine restarts and two steers; the last two tasks ran eighteen and twenty minutes each with neither.

### After the job: the round's cost, measured, and the tail cut, 00:40 to 01:10

The daemon's own log gave the numbers the plan had guessed at: prompt processing at 340 to 600 tokens a second, generation at 60 to 80. Every thousand uncached tokens in a request is about two seconds of the round, and the uncached part is everything after the first changed token: the round's own new messages and then the tail behind them, the record's second part, the results list, the memory hint and the cost line. In task 11 that was about seven thousand tokens, fourteen of its nineteen seconds. So the tail was cut where it was largest: the results list from a hundred lines to the newest forty, the lessons from eight of each kind at 160 runes to six at 120, and one read from a quarter of a megabyte to sixteen kilobytes or four hundred lines, because the model read its whole thirty-thousand-character script twenty times to find one function each time, fifteen seconds of prompt processing per read. Idea one's second half went in as well, the step mark on the reply's first line, with a one-line hint after the first round a task spends on nothing but a mark, since the rule in the instructions alone was not taken up.

The nightly runner had a flaw of its own, caught at 01:17 on the fifth run's log: it put each ask's work folder inside the agent's home, which the file tools refuse, so the model wrote through the shell and the syntax check and the tests-after-a-change never fired; the folders now go beside the home, inside the sandbox root, and the four tables before the fix carry the caveat. The nightly set ran three times more as the number before and after: 84 rounds across the four asks on the night's last binary, 39 with the first-line mark, 35 with the shorter tail and the read caps, every check passing every time and every ask done in about a minute. The asks are too small for the tail to matter much; the long task is where the cut shows, and the next Tetris run is its measure. The tables are in `docs/nightly/`.

**The unattended Tetris build, 01:24 to 01:55.** With the runner fixed, the whole game went in as the nightly set's fifth ask on the night's binary, nobody watching and no steer: the model took it as one task under a nine-step plan rather than a job, spent the middle of the run on a line-clear test it had diagnosed correctly, wrote that failure and then, told by the stall line in its window to write what the rounds showed, wrote the same failure six more times, each refused as already held, until the same-call guard stopped the task at 94 rounds with one step of nine done. Two things went in within the hour: the nudge after a fresh failure names it, quotes its cause and asks for a change and no more failures; and a task on an ask over six hundred words holds six plan steps rather than ten, so a whole game is refused as a task and made as a job, whose hand-off ends the task that makes it. The sixth run, with both, started at 02:02: the model made the game a job of twelve tasks in its fourth round, and the runner, which took the ask's own task ending for the ask's ending, stopped its serve under the job; the runner now waits for a job an ask became and serves a fresh home each run. The seventh launch was refused because that fresh home sat inside the sandbox root, and the eighth, at 02:53, ran the game as a job of eleven tasks until its first task's review lost the job's report: the fact it saved under `review-task-6` collided with one an earlier run had left in the copied persona files, memory refused it, and the refusal ended the task with an error before the job was told. A lesson that cannot be kept now costs the lesson and nothing more, and every review's fact carries the moment of the review in its id. The ninth launch followed at 03:56.

Also built in the same hours: the nightly set (`scripts/nightly`, four asks with a check each, the Tetris build as the optional fifth, `make nightly`). Run at 00:27 once the daemon's slot was free, on a home of its own and the night's last binary: four of four checks passed in seven minutes, the table is in `docs/nightly/2026-09-06.md`, and it is the number before for every change from here.

### The tenth nightly run and the output cap, 04:02 to 05:35 on 6 September

The tenth run of the set was the first on a home made fresh beside the work folder (`docs/nightly/2026-09-06-g-cut-off-found.md`). The four small asks passed in five minutes, as before. The game went in as a job of nine tasks, the first seven of which ended done, and the check still failed, because no game file was ever written: the frontend task wrote the whole of `main.js` in one write call, the reply hit the output cap of 8,192 tokens three rounds running, the harness read each cut-off call as unreadable arguments and offered the three options, and on the fourth round the model took the first, "answer the user", with "Now the main game logic. Let me write it carefully", which closed the task as done on its own answer, since its done list had been refused and a record with no done list takes the answer as the whole of it. The play-test task found no game file, tried to write it, and ended the same way, and the regression task was doing it again when it was put down by hand at 05:33. Measured off the log afterwards: fourteen of the job's 224 rounds ended at the cap, 115k of its 200k output tokens and about half an hour of its 83 minutes, each two minutes of generation kept nowhere. Before that, the play-test task had spent ten rounds writing its done list with the results it expected to produce later, r70 to r74 on a record at r20, each refused with "check it against the result list", guessing a new label every round.

Built from it, test first, between 05:05 and 05:35, each with the round in the log that asked for it: the record's refusal of a proof not written yet names the next label and says to write the line bare and pin it with `pin_result` later; the `task` tool's done-list field says a line is a plain string whose proof is pinned later; the rules' "every done line must point at the result proving it" became "write done lines bare, and mark each done later", within the 500-word cap; the loop reads the finish before the parse, keeps none of a reply cut off at the cap, and tells the model how many tokens in it was cut, that nothing was kept, and that a long file goes in parts of at most three hundred lines, the first by write and the rest by edit (`loop/cutoff.go`); and an answer on a record with no done list and a plan step still open is sent back to the plan, naming the open steps, three times and then failed, without the three options, because "answer the user" is the option that closed the frontend task (`loop/openplan.go`). The run report counts the rounds cut off at the cap and the nightly table has the column. The eleventh run started at 05:31 on the binary with all of it, to measure whether the local model splits a file on the first telling.

### The eleventh run, the double click and the Vitest blindness, 05:31 to 05:56

The eleventh run went in on the binary with the tenth's fixes (`docs/nightly/2026-09-06-h-double-click-and-vitest.md`). The game job's rounds were never cut off at the cap: its tasks wrote the engine in edits of a few hundred tokens, where the tenth's had written whole files. What the run found instead were two older harness bugs, each fixed test first while it ran. The page task saw its counter go to 2 on every click for seventy-eight rounds, because the browser worker knew a change only as a new element, address, title, dialog, tab or download, read 0 becoming 1 as "nothing changed", and clicked again at the element's place on the screen; the tenth run's page task had met the same and got round it with a click dispatched through the ask field. The diff now carries the lines of text that appeared, a number is a meaningful word however short, and a verdict not met says what the text now says (`worker/browser`, the counter fixture and test). And the harness read none of the game job's ten Vitest runs as a test run, because the test-state reader knew Node's runner, Jest, pytest and Go and not Vitest's "Tests  16 failed | 56 passed (72)": no rerun fired after a change, a run that went from 36 failing to 16 was "finished with exit code 1" both times, and the progress meter cleared the conversation in the middle of the hazards task after twenty rounds of real implementation. The reader now knows Vitest, files that could not load included. Smaller, from the same log: the `task` tool's refusal of a pin on an empty done list says to write the list first; "where to next" is an offer to carry on inside a job; the review's fourth answer skips headings and questions, because the memory held two headings as lessons; the run report counts refusals by tool (the tenth run's game job had 23 of its 62 task-tool calls refused); the strip's count of a call in progress includes the model's thinking and its tool calls, which the providers now report as written unseen; and a reply cut off at the cap is sent back at most twice in a row, the third heard as it stands, so a long plain answer cannot loop. The run was stopped by hand at 05:54 on the game's third task, and the twelfth started at 05:57 on the binary with all of it.

**The daemon's checkpoint spacing, found at 06:30.** Watching the twelfth run's engine task, the record's cached count sat flat for fourteen rounds at a time (21.3k, then 32.0k, then 40.6k) while the prompt grew, and the daemon's log showed its prompt processing per request growing round by round (5,893, 6,456, 7,268, 8,512, 9,052 tokens) and then dropping to 1,405 and growing again. The model is a hybrid, `qwen35`, SSM layers with full attention every few layers, so llama-server cannot roll its state back to any token; it keeps context checkpoints spaced `--checkpoint-min-step` apart, 8,192 by default, and when a round's prompt diverges from the last one, which every round's does at the start of the record's tail, it restores the newest checkpoint before the divergence and re-processes all that follows. Over seventy requests that was 888 seconds of prompt processing, 64 percent of the model's time, for a mean of 4,875 tokens a request, against 489 seconds of generation. Every cut to the tail made this week was worth a few hundred tokens a round; this is worth about four thousand. The change is one flag on the daemon's start line, `--checkpoint-min-step 1024`, which would bring the re-processing down to about the tail and the new round and a twenty-second round to about twelve, at the cost of the memory of up to thirty-two SSM-state checkpoints on a card that ran out of memory once this week. The start line is the owner's and is never overridden from here, so this is written up for that decision rather than made.

**The twelfth run, 05:57 to 06:31.** All four small asks passed, the page task in fifteen rounds where the eleventh's took seventy-eight, and the game job's rounds were never cut off at the cap. The meter stopped the engine task by its own hand: with two tests left red the model read its engine in windows, probed it with node scripts and searched it with grep for twenty rounds, none of which counted, so the conversation was cleared mid-search at round 66 and the task stopped at round 86, the job put down at one of six (`docs/nightly/2026-09-06-i-meter-stopped-the-engine.md`). Fixed test first: a look or a command whose answer the task has not seen before is progress. The thirteenth run went in on it at 06:33.

### Eyes on, and the checkpoint spacing changed, 08:45 to 09:25 on 6 September

With the owner's word ("fix vision asap"), the thirteenth run was stopped at eight of eleven tasks, five of five checks passing, and the daemon was restarted on a start line carrying the vision projector for this exact model (found in the same Hugging Face repository as the quant, 927 megabytes) and `--checkpoint-min-step 1024 --ctx-checkpoints 16`. The daemon reports vision on; a 1024 by 768 screenshot of the game cost 799 prompt tokens and the model named the score, level and lines on it. The harness half went in test first in the same hour: a picture in a tool's output rides with its result to a model whose alias says `vision = true`, as an image part on the OpenAI wire and an image block on the Anthropic one; `browser_screenshot` is a new tool, `read` hands a picture file back as the picture, and the desktop screenshot does the same; only the newest two pictures stay in the window; a model with no eyes is told the picture is not shown, as before. The live proof: asked on the show home to open the game and take a screenshot, the model described the start screen down to the O piece in the preview and the four HUD numbers. The browser tool and skill now say to take a `browser_screenshot` to see the page.

**The projector's memory, 09:30 to 09:50.** With the projector on the same start line the first run went at a fifth of the old speed on prompt and generation alike, and nothing in the daemon's log said why: the whole model had fallen off the card into host memory (the daemon's process at 21 gigabytes, the card at two). Without the projector the model sat on the card at 21 gigabytes and a 4k prompt took 6.5 seconds cold, 0.6 cached, 1.4 with a changed tail, which is also the checkpoint spacing working as hoped. At a context of 131,072 instead of 262,144 the projector fits beside the model and the speed is the same, so the daemon runs at 131,072 now, both homes' configs say so, and a picture request answers in about ten seconds. The fourteenth run went back in on that.


**The fourteenth run, 09:25 to 09:40, and the fifteenth.** On the fitted daemon the four small asks passed in under a minute each but the page task at two, with 1.0k and 0.8k uncached tokens a round at five seconds a round, against 3.6k to 5.0k at sixteen to twenty the night before: the checkpoint spacing works. The game failed before it began: the model made it a job of one task, the scaffold, and the job finished after it (`docs/nightly/2026-09-06-k-fourteenth-one-task-job.md`). The `job` tool now refuses a job for a long ask that lists fewer than three tasks, and the fifteenth run went in on that at 09:32.

### The fresh build on the owner's screen, and two browser faults, 10:55 to 12:30 on 6 September

The owner asked for a run he could watch ("if you test, I must see running nerdgenie on my screen"), the run folder was deleted and the Desktop game folder emptied, and the Tetris ask went to the show home with its screen in a terminal window and the agent's own Chrome on the desktop. The daemon carries the vision projector at a 131,072 context, and the harness's `browser_screenshot` tool put the game's picture in front of the model for the first time. The model wrote its own headless QA script in `tools/`, and its twenty-three checks pass.

**The twenty-five second click, reproduced through the test harness.** The live worker kept answering "Chrome stopped working: the click method was still running after 25000 milliseconds" on the Start button, and the Go side restarted the browser under the model, four times in one task. Through the test harness in fast pacing the same click took seven seconds; in human pacing it took the full twenty-five and failed the same way. The game draws sixty frames a second, every look at it is slow, and a click with two looks and a second click does not fit. The worker now asks the page a trivial question when its deadline rings and, when the page answers, waits the same time again; only a page that cannot answer is restarted. Committed as `57911044`, the show serve restarted on it at 12:02 between the page task and the play-test task.

**What the model looked at and did not see.** My own screenshots of the built page, headless and then in a visible Chrome on the desktop, showed the GAME OVER card on load with the dragon warning lit, while the model's headless QA said the start screen was up and the game-over overlay hidden. Both were right: the `hidden` attribute was set, and the stylesheet's `.overlay { display: flex }` overrode it, so all three overlays were drawn and the last one won. The model's own `browser_screenshot` at 11:39 showed the same card, its outline listed all three overlay headings, and it went on. The scan now marks such an element `hiddenYetDrawn` and the read tool prints `(marked hidden, yet drawn: a style rule overrides the hidden attribute)` after it, so the outline says what a person sees. Whether the model fixes the page from that mark, without a steer, is the test of the play-test and visual QA tasks that follow.

**The empty desktop, 12:40.** "Why is the browser it is using not open on screen? I always want the harness to drive a browser on screen like a human." The serve had the display and its Chrome is headed; the cause was that the model had made no browser call since the restart. It had written two Playwright scripts, `tools/headless-qa.js` and `qa/playtest.js`, each launching a headless Chrome, and was play-testing through them off the screen, in a task whose ask said "in Chrome as a human player would" and under a skill that said never to launch Chrome from the shell. A steer put it back on the browser tools within a round, and its Chrome came up on the desktop. Then the rule was made one the harness enforces: a line in the standing instructions (the text trimmed by twenty-four words to stay under five hundred), the skill's never-do line rewritten with the library names, and the write and edit tools refusing any content that starts a browser with no window, with a refusal that names the tools to use instead. The first outline the model read after the restart carried the new mark on every drawn overlay; what it does with it is still to be seen.

**The steer, and what it showed, 12:32 to 12:50.** The model chased its own watchdog's `frames=0` for ten rounds, since nothing incremented the counter it had written, and then asked for a browser handoff so a person would bring the window forward. One message with two facts, the counter and the overlay rule, and it removed the watchdog and added `.overlay[hidden] { display: none; }` within three rounds. My own look in a visible Chrome then showed the start card alone and a game that plays, and still two nodes drawn that their markup hides: the dragon badge and the touch controls. The badge has no role and never reaches the outline, so a per-element mark cannot say it. The scan now counts every such node on the page and the read tool prints the count above the elements with the one rule that fixes them all.

**The count line, tested live, 13:10.** The play-test task finished at 13:07 with its five done lines green, played through the browser tools on the desktop, and one real bug found and fixed on the way: the bomb dev control ended the game instead of clearing a line. The visual QA task opened the page, read the count line above the outline, asked the page for the computed display of the badge and the touch controls, wrote `[hidden] { display: none !important; }` into the stylesheet, bumped the stylesheet's cache-busting version, and reloaded to check, all in its first ten rounds and without a word from me. The per-element mark alone had not moved it; the page-level count with the fix named did.

**`browser_resize`, 13:25 to 13:50.** The visual QA task's ask is five window sizes, and the browser tools had no way to set one. The model tried `window.resizeTo` from the page, wrote a Playwright script for it, which the write tool refused as a headless launch, and then took a desktop screenshot to drag the Chrome window by hand. The worker now has a `resize` method and the model a `browser_resize` tool: width and height in pixels, bounded, the page read again at that size. The skill says to resize, then screenshot. Deployed into the running task with one line saying the tool exists. Proven at 13:36: the first call, "Size 1 of 5: large desktop 1920x1080", answered "the page is now 1920 by 1080 pixels" with the page read again at that size. The meter then read "rounds since progress: 6" against the task that had just gone through five sizes in two minutes, because a resize result did not count; it now counts like a read that says something new.

**The fresh window opens oriented, built in the afternoon.** Measured over the day: after each of the four serve restarts the model re-read about six earlier results by their ids (`read r37`, `read r42`, ...) before doing anything, because the fresh window carried only their one-line summaries; and at the start of tasks it ran `ls` on the work folder and `ss -ltnp | grep 8091` to learn what exists and what is listening, a round each. The harness now hands those facts over up front, once, as the first message after the ask and after a rewind: the working folder's entries, the listening ports from the kernel's socket tables, and, on a pick-up or a rewind, the newest two results in full. The block is bounded at 6,000 characters, about 1,500 tokens on a busy folder, and sits in the cached part of the conversation from the second round on. Built test first in `internal/orientation` and `internal/loop/freshwindow.go`.
**The poll that waits, 14:00.** The play-test task started its own test script, got the shell tool's ticket after ten seconds, and asked "is it done yet?" every five seconds: fourteen rounds of "still running" for one run, each round a full prompt to the model. A poll now waits up to twenty seconds for the command before answering, so a command that finishes in that window answers the first poll with its result. The wait is counted on the tool's clock, and the test proves a poll answers when the command finishes rather than when the clock does.

**The fresh run's first hour, 14:15 to 15:10.** Twelve tasks written, the scaffold done with every done line pinned to a result. Two faults found by watching the screen. First, no task after the first wrote a plan, where the run before had written one in sixteen of sixteen: the orientation block had been placed as its own message after the ask, so the last thing the model read before its first reply was a folder listing and a port list. The block now comes first and the ask last, on a task start, a pick-up and a rewind alike, with a test on the order. Second, after every write the harness reran the tests and reported "9 failing of 49" with no names, because Jest without --verbose names a failing test only in its "● suite › test" header, which the reader did not know; the model ran Jest itself four times in a row through grep to learn the names. The reader now takes those headers, so the summary line names them and the rerun is not needed.

**Two widenings, 15:20.** The owner asked for an honest look at whether the day's changes fit any task, a book or a C++ build as much as a game. Two did not quite: the count line for drawn hidden elements told the model the one rule that fixes them, which assumes the page is its own, and now says so; and the headless guard refused any content naming `--headless`, which would have refused a chapter on crawlers, and now refuses only the option in a file that launches a browser or the flag on a line that runs one, with prose and settings tested as written.

