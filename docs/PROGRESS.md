# Coeus progress

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

### To wire at the wave 3 gate, in the orchestrator's two files

Subcommand values for the table in `cmd/coeus/main.go`: `sandboxEntrySubcommand` (`sandbox_entry.go`, hidden), `askpassSubcommand` (`askpass.go`). Command values and constructors for `serve.go`: `vault.NewCommand(store)` (terminal only); `permission.New(config, clock)`; `provider.New(alias, options)` with `provider.WithRetries` and `provider.NewChain`, all on `clock.System()`; `log.Open(home.DatabaseFile())`; `config.Load(home)` and `config.Doctor`; `sandbox.New` from the configuration's roots. The fix worker's finding 18 lands the exit-code mapping in `main.go` first.

### Human trial

None until wave 3.
