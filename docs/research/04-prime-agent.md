# Prime Agent — code-reading update (2026-09-02)

Repo: `/Users/jt/Code/prime-agent`, HEAD `0ba0423c5` (v0.9.1). Prior report: `/Users/jt/Desktop/HARNESS.md` (2026-08-06). This note covers only what changed and the questions asked. All paths are relative to the repo root. Every code claim has a `file:line`. "Not verified" means I did not confirm it.

One fact the prior report did not state: Prime Agent is a hard fork of Mario Zechner's `pi-mono` (`packages/coding-agent/README.md:16`, `:485`). The packages still ship as `@earendil-works/pi-ai`, `pi-tui`, `pi-agent-core` (`packages/ai/package.json:2`, `packages/tui/package.json:2`, `packages/agent/package.json:2`). The TUI, the provider layer and the agent loop are inherited. Prime's own work is the kernel, the daemon and the always-on features.

---

## 1. What changed since 2026-08-06

149 commits, v0.7.1 → v0.9.1, +64,136 / −25,388 lines. 100 commits by one engineer (Sebastian Müller), 27 by a second (Seth Karten). Buckets are approximate, by commit subject.

| Area | Commits | What it was about |
|---|---|---|
| Daemon / supervisor / worker / attach | ~40 | Recovery ownership, process identity, spawn ledger, direct TUI↔worker transport, roster/agents view, idle eviction |
| TUI / interactive UX | ~27 | Mermaid, Ctrl+J diffs, Ctrl+P, expand/collapse, queue editing, OSC 8 links, `--resume` back |
| Chore / CI / release / docs | ~25 | 6 releases, issue templates, Bugbot rules, changelog fragments, analytics |
| Kernel / Python runtime | ~17 | **IPython replaced by a minimal CPython REPL**, async `bash()`, dill snapshots, kernel-owned MCP |
| Provider layer (`packages/ai`) | ~15 | 5 catalog regenerations, Anthropic cache marker fix, reasoning-level metadata, MCP OAuth |
| Goal / heartbeat / autonomous / refine / compaction | ~13 | Goal survives compaction, resume after compaction, refinement status, `session_before_refine` hook |
| Refactors and test-seam removal | ~8 | Deleting test-only seams, single-sourcing state |
| ACP (editor protocol) | ~5 | Resident session lifecycle |

### The ten changes that matter

| # | Commit | Why it matters |
|---|---|---|
| 1 | `61eb64748` Minimal CPython REPL runtime (`rlm.repl`) | The "persistent IPython kernel" is gone. Zero `zmq`/`ipykernel`/`jupyter` references remain in non-test source. The kernel is now `python -m rlm.repl`, a JSON-lines subprocess (`packages/coding-agent/src/core/kernel/repl-manager.ts:1-3`; protocol in `prime-agent-runtime/src/rlm/repl.md:1-7`). |
| 2 | `0940833b7` Async `bash()` in the kernel | `bash(command) -> BashHandle` returns immediately; the model ends its turn and reads later (`prime-agent-runtime/src/rlm/bash.py:652`, `:108`; prompt text at `packages/coding-agent/src/core/prompts/rlm.ts:37`). |
| 3 | `173d845a5` Direct session transport TUI↔worker | The TUI gets a single-use ticket from the supervisor and connects straight to the worker's socket (commit body). Removes the supervisor from the hot path. |
| 4 | `74c8d39ee` Harden daemon startup and recovery ownership | One `processIdentity()` oracle for liveness; a hung worker is probed 10 rounds (~2.5 min) then parked "failed" (commit body). |
| 5 | `97b994c3d` + `06e4a19dc` Supervisor-owned RLM spawn ledger | Sub-agent family truth lives in one ledger (`packages/coding-agent/src/modes/daemon/rlm-ledger.ts`, 884 lines). |
| 6 | `f8f02221e` Kernel-owned MCP runtime | MCP servers are Python objects inside the kernel, not LLM tools (`prime-agent-runtime/src/rlm/mcp.py:1`). |
| 7 | `114a1d6af` + `20b54977a` Resume after compaction; goal survives compaction | A post-compaction continuation queue (`packages/coding-agent/src/core/agent-session.ts:834`, `:1215-1217`, `:2788-2806`). |
| 8 | `274cbb84f` + `108eff32e` Refinement status + `session_before_refine` hook | `/refine` is now observable and interceptable (`packages/coding-agent/src/core/extensions/types.ts` lists `session_before_refine`, `refine_complete`). |
| 9 | `9e49b73dd` + `824a9ee3b` RLM depth default 2; per-subagent reasoning level | `settings-manager.ts:139`: "unset falls through to RLM_MAX_DEPTH, then 2". |
| 10 | `032e3ee74` "remove redundant comments" | **Deleted both RLM-1 TODOs.** `rg RLM-1` returns nothing. `git log -S'RLM-1'` shows they left in this commit. |

Also notable: `0ba0423c5` (#879, today): process identity used `ps -o lstart=`, which prints in local time, so a macOS timezone change made the daemon look like a different process. Fix pins `TZ=UTC LC_ALL=C` (`packages/coding-agent/src/core/session-lease.ts:145-158`).

---

## 2. The daemon + attach model, in plain English

### The process tree

```
prime-agent (TUI)  ──socket──▶  supervisor  ──spawns──▶  worker (one per session)  ──spawns──▶  python -m rlm.repl
                    ╰── after #1926: single-use ticket, then direct socket to the worker ──╯
```

**Starting.** `main.ts` decides whether to use the daemon (`packages/coding-agent/src/main.ts:221-243`). If none is running, `ensureInteractiveDaemonRunning` spawns `node <cli> --mode daemon --daemon-socket <path>` with `detached: true`, `stdio: "ignore"`, then `child.unref()` (`packages/coding-agent/src/cli/daemon-launch.ts:397-420`). There is no launchd or systemd unit; the first TUI launch is what starts it. The socket is `$TMPDIR/prime-agent-<uid>/daemon.sock` (`packages/coding-agent/src/modes/daemon/daemon-socket.ts:279-282`, `:74`); a named pipe on Windows (`:71-73`).

**Finding it.** A `proper-lockfile` lock on the socket path stops two supervisors starting (`daemon-socket.ts:77-100`). The supervisor writes an owner record: `pid`, `processStartId`, `token`, `generation`, `socketPath`, `appVersion`, `phase` (`packages/coding-agent/src/modes/daemon/daemon-supervisor-ownership.ts:33-49`). Identity is pid **plus** process start time so a recycled pid is not mistaken for the daemon (`session-lease.ts:145-180`). That start time is what #879 broke and fixed.

**Workers.** The supervisor spawns one worker per session with its own socket, a 32-byte auth token, a descriptor JSON, a recovery journal and an orphan-process journal (`packages/coding-agent/src/modes/daemon/daemon-supervisor.ts:2842-2854`). The worker is `runDaemonMode` (`daemon-mode.ts:460`): "The daemon owns live AgentSessionRuntime instances and exposes a small JSONL protocol over a local socket. Clients can attach/detach from sessions without disposing the underlying agent loop" (`daemon-mode.ts:1-7`).

**Attach and detach.** `DaemonAgentConnection` (`packages/coding-agent/src/modes/agent-connection/daemon-agent-connection.ts`, 2,394 lines) implements the same `AgentConnection` interface as the in-process one (`in-process-agent-connection.ts`, 697 lines), so the TUI does not know which it is talking to. Detaching leaves the session running. Sessions with no attached client are evicted after `idleEvictionMinutes` (default 90, or `"off"`) (`packages/coding-agent/src/core/settings-manager.ts:140`; sweep logic `daemon-supervisor.ts:198-200`, `:628-632`). Empty drafts are dropped when the last viewer leaves (#1946).

**Persistence (still a JSONL tree).** Sessions live in `~/.prime/agent/sessions/` (`packages/coding-agent/src/config.ts:525-531`, `:620-626`), one `<sessionId>.jsonl` per session (`packages/coding-agent/src/core/session-manager.ts:282`), format version 3 (`:32`). Every entry has `parentId: string | null` (`:97`); the active path is walked leaf-to-root through `parentId` (`:446-450`). Entry types: `message`, `thinking_level_change`, `service_tier_change`, `model_change`, `compaction`, `branch_summary`, `custom`, `child_usage_attributed`, `label`, `session_info`, `session_state`, `agent_status`, `git_state`, `custom_message` (`:101-208`). Beside the JSONL sits `kernel-state.dill` + `kernel-state.json` (`packages/coding-agent/src/core/kernel/state-snapshot.ts:41-47`), written 1.5 s after each cell (`kernel/shared.ts:8`; `repl-manager.ts:1440-1441`), capped at 256 MB total and 16 MB per variable (`state-snapshot.ts:12-14`), one `dill` pickle per top-level name so one bad object does not sink the snapshot (`:1-8`; `prime-agent-runtime/src/rlm/repl.py:619-641`). Scheduled jobs live in a per-session `scheduled-jobs.json` (`packages/coding-agent/src/core/cron-jobs.ts:112`).

**Crash and restart.**

| Failure | What happens | Evidence |
|---|---|---|
| Kernel dies | Kernels exit when their owner dies (#1559). On resume a fresh kernel is spawned and variables are revived from dill; the model is told what was dropped. | `state-snapshot.ts:1-8`; tool description `tools/ipython.ts:617` |
| Worker hangs | Supervisor probes via `processIdentity()`; after 10 defer rounds (~2.5 min) it parks the worker "failed" and leaves the process alive for manual `retry_worker`. | commit `74c8d39ee` body |
| Command lost mid-flight | Recovery journal records `received` / `result` / `acknowledged` per command so a reconnect can replay or dedupe. | `daemon/command-recovery-journal.ts:5-38` |
| `bash()` children orphaned | Orphan journal records pid + start id per kernel so the host can reap them. | `core/orphan-process-journal.ts:9-18` |
| Supervisor dies | TUI sees socket close (`StaleDaemonError`, `main.ts:21`); next launch relaunches. | `daemon-launch.ts:397-420` |
| Reboot | Sockets in `$TMPDIR` vanish; sessions on disk survive; `prime-agent doctor --fix` removes stale sockets and stops idle orphans. | `cli/command-registry.ts:83-86` |
| Two processes open one session | `SessionLease` with `{token, pid, processStartId}` throws `SessionAlreadyActiveError`. | `session-lease.ts:10-34` |

### Verdict: is this the right pattern for a personal agent?

**The idea, yes. The implementation, no.**

The idea fits JT's case exactly: the agent must run with no terminal open, Signal and a web tab must attach to the same live session, and detaching must not stop work. Prime proves the shape: UI is a thin client over a connection interface, the session keeps running, state is on disk so restart is resume.

The implementation is too much. The daemon area alone is 33,281 lines (`daemon-mode.ts` 7,515; `daemon-supervisor.ts` 6,572; protocol 1,438; `daemon-ps.ts` 1,277; update-restart 611). The complexity comes from three things a personal agent does not need: (a) a three-process tree per session, forced by a Python kernel that can hang and must be killable; (b) upgrading a running daemon in place (`daemon-update-restart.ts`); (c) protocol versioning between mismatched TUI and daemon builds (`AGENTS.md` "Daemon Protocol Changes": every wire change needs a capability gate and old/new tests). A single-user agent on a Pi can be **one** process under systemd with `Restart=always`, one append-only store, one WebSocket over Tailscale, and a Signal adapter as just another client. Crash recovery becomes "systemd restarts it and it re-reads the JSONL". You keep the attach/detach semantics and the idle-eviction idea; you drop the supervisor, the worker sockets, the tickets and the recovery journals.

---

## 3. TUI (`packages/tui`)

**Built on:** nothing. It is a custom differential renderer ("Minimal TUI implementation with differential rendering", `packages/tui/src/tui.ts:2`). A `Component` is `render(width): string[]` plus optional `handleInput(data)` and `invalidate()` (`tui.ts:53-78`). `TUI extends Container` (`:304`) and diffs lines against the last frame. Not React, not Ink. Runtime deps: `chalk`, `marked`, `mime-types`, `get-east-asian-width`; optional `koffi` (FFI, Windows-only, to read `kernel32` for Shift+Tab — `packages/tui/src/terminal.ts:362-378`) (`packages/tui/package.json:41-50`).

**Size:** 14,644 non-test lines, 13,102 test lines. Largest: `editor.ts` 2,390 (full multi-line editor with kill ring), `tui.ts` 1,992, `utils.ts` 1,382, `keys.ts` 1,381 (Kitty keyboard protocol), `markdown.ts` 969, `latex.ts` 834, `autocomplete.ts` 748, `fullscreen.ts` 715, `terminal.ts` 614, `terminal-image.ts` 423 (Kitty and iTerm2 image protocols, `:1`, `:57-69`).

**Worth copying:**
- Component = `render(width) -> string[]`. Trivially testable, no virtual DOM. (`tui.ts:53-78`)
- Nonblocking transcript on the alternate screen with a docked editor (`tui.ts:676`).
- Mermaid → Unicode box drawing via `grok-mermaid`, 77 lines total (`interactive/components/mermaid.ts:1-25`). Cheap and delightful; works over SSH.
- Terminal image protocol detection (`terminal-image.ts:57-69`) for Hue/Arlo snapshots in a terminal.
- `Ctrl+J` toggles inline diffs, `Ctrl+P` toggles agent-to-agent messages, and tool calls / thinking / a2a messages expand independently (commits `e64fbcbf3`, `f8d73abe1`, `d1b072686`).
- Edit queued messages in place; queue survives interrupt (`71ca6cfd1`).

**Heavy:**
- `interactive-mode.ts` is 10,143 lines, 362 methods, 106 imports, plus 58 component files. This is the "1000-line switch" smell from the HN thread, still present.
- `agent-session.ts` is 11,948 lines. It is the god object: prompt, tools, compaction, goals, autonomous, cron, sub-agents, usage.
- 528 KB of PNG assets (`interactive/assets`) and a vendored `highlight.min.js` (122 KB) + `marked.min.js` (39 KB) for HTML export (`core/export-html/vendor`).
- `latex.ts` (834 lines) in a coding-agent TUI is scope creep.

For a Signal + web agent the TUI is optional; if wanted, lift the editor and the Component interface.

---

## 4. Provider layer (`packages/ai`)

| Metric | Value |
|---|---|
| Non-test, non-generated lines | 13,723 |
| Generated catalog (`models.generated.ts`) | 22,086 lines, 1,238 models |
| Provider IDs in catalog | 32 (openrouter 292, vercel-ai-gateway 224, amazon-bedrock 119, prime-inference 106, … anthropic 13, openai 41) |
| **Actual API implementations** | **9**: `anthropic-messages`, `openai-completions`, `openai-responses`, `azure-openai-responses`, `openai-codex-responses`, `mistral-conversations`, `google-generative-ai`, `google-vertex`, `bedrock-converse-stream` (`packages/ai/src/providers/register-builtins.ts:343-392`) |
| Largest provider files | `openai-codex-responses.ts` 1,289; `anthropic.ts` 1,264; `openai-completions.ts` 1,242; `amazon-bedrock.ts` 967 |
| Runtime deps | `@anthropic-ai/sdk`, `@aws-sdk/client-bedrock-runtime`, `@google/genai`, `@mistralai/mistralai`, `openai`, `typebox`, `partial-json`, `proxy-agent`, `undici`, `zod-to-json-schema` (`packages/ai/package.json`) |

**How it normalizes.** One event union for every provider (`packages/ai/src/types.ts:275-287`): `start`, `text_delta`, `thinking_delta`, `toolcall_start`, `toolcall_delta`, `toolcall_end`, `done`, `error`, each carrying a `partial: AssistantMessage`. A `Tool` is `{name, description, parameters: TypeBox TSchema}` (`types.ts:255`); `ToolCall` at `:194`, `ToolResultMessage` at `:241`. `transform-messages.ts` (210 lines) rewrites the unified history per target model, e.g. replacing images with a placeholder for non-vision models (`:12-13`). Anthropic `cache_control` goes on system, tools and the last user message (`providers/anthropic.ts:966-982`, `:1188-1205`, `:1239`). Context overflow is detected in one place (`utils/overflow.ts:117`) and stream failures classified in one place (`utils/stream-failure.ts:66`).

**OAuth trick: still there.** `packages/ai/src/utils/oauth/anthropic.ts:27-28`: `const CLIENT_ID = decode("OWQxYzI1MGEtZTYxYi00NGQ5LTg4ZWQtNTk0NGQxOTYyZjVl")`. I decoded it: `9d1c250a-e61b-44d9-88ed-5944d1962f5e`, Claude Code's OAuth client ID. Used at `:199`, `:246`, `:353`. GitHub Copilot and OpenAI Codex OAuth flows sit beside it (`utils/oauth/index.ts:10-18`). Same advice as before: use `ANTHROPIC_API_KEY`.

**Local models.** Supported through `~/.prime/agent/models.json` with `api: "openai-completions"` and a `baseUrl` (`packages/coding-agent/docs/models.md:3`, `:19-37`; Ollama example uses `http://localhost:11434/v1` and `apiKey: "ollama"`). `compat` flags turn off `developer` role and `reasoning_effort` for servers that lack them (`:41-53`). The registry applies `baseUrl` overrides (`packages/coding-agent/src/core/model-registry.ts:148`, `:189`, `:241`, `:557`). No Ollama-native client; it rides the OpenAI-compatible path, which is the right call.

**Verdict.** The core is clean and copyable: `types.ts` (472) + `stream.ts` (59) + `api-registry.ts` (98) + one file per API. The bloat is the catalog (22k generated lines, mostly aggregator re-listings) and four vendor SDKs as hard deps. A personal agent needs two APIs: `anthropic-messages` and `openai-completions`. That is ~2,500 lines plus types, and `openai-completions` covers Ollama, LM Studio, vLLM, OpenRouter and Groq. Copy those; do not import the package (its published name is inherited from `pi`, and Prime's README says not to use the npm path — `packages/coding-agent/README.md:16`).

---

## 5. The kernel / REPL

- **Still the only LLM tool.** `export type ToolName = "ipython"` (`packages/coding-agent/src/core/tools/index.ts:46`); `createAllToolDefinitions` returns just `{ipython}` (`:52-55`). Schema is `{code: string}` (`tools/ipython.ts:144-148`), `executionMode: "sequential"` (`:620`). `bash.ts` and `edit.ts` exist in the same directory but are used only to render cells in the TUI (`interactive/components/tool-execution.ts:57-61`) and are exported for extensions (`src/index.ts:253-254`). The model never sees them.
- **Not IPython any more.** The tool name and prompt still say "IPython", but the process is `python -m rlm.repl`: one persistent `__main__` on one asyncio loop, newline-delimited JSON on stdin/stdout, protocol v3, requests `execute`, `interrupt`, `host_reply`, `snapshot`, `restore`, `list_names`, `shutdown` (`prime-agent-runtime/src/rlm/repl.md:1-40`; `repl-manager.ts:1-3`, `:51`). The Python runtime is 4,581 lines (`repl.py` 1,166, `bash.py` 890, `harness.py` 820, `mcp.py` 658).
- **The RLM-1 TODO is gone.** Removed in `032e3ee74` under "remove redundant comments". Nothing replaced it. The kernel got *more* investment after the comment was deleted (the `rlm.repl` rewrite landed a week later). The honest reading now: the persistent kernel is the product, not a bridge.
- **Inside the kernel:** `bash()` handle with `pid`, `running`, `tail`, `poll`, `kill`, `await` (`prompts/rlm.ts:37`); `await rlm('task', name=, model=)` returns at admission and never returns the child's answer (`prompts/rlm.ts:152-153`; `__init__.py:92-101` via `host_request("rlm.run")`); MCP servers as Python objects (`mcp.py:1`); harness state CRUD as `rlm.harness.create_memory(...)` etc. (`prompts/rlm.ts:47`).

### Does a persistent REPL belong in the new agent?

No, not in v1, and later only as an opt-in tool for one session type. Prime's kernel pays for itself when the working set is big and the session is long: a dataframe stays in `df` while context sees `(4000000, 37)`. A personal assistant's work is short and event-driven: a Signal message, a cron tick, "turn off the lights", "what's on today". Nothing accumulates between turns that a file cannot hold. What the kernel costs is large and visible in this repo: a third process to supervise, an orphan journal for its children (`orphan-process-journal.ts`), cold-boot handling (#1587), a "wait or kill the busy kernel" dialog (`tools/ipython.ts:151-158`), debounced dill snapshots, and 3,500 lines of runtime — all with full user permissions and no gate. On a Pi 5 that is memory and a bigger blast radius. What is worth keeping is the *shape* of `bash()`: start work, get a handle, end the turn, read the result later. That works with a stateless `python` tool (timeout, rlimits, scratch dir) and a `bash` tool. Give the agent several small typed tools (`bash`, `read`/`write`/`edit`, `http`, `signal_send`, `calendar`, `hue`, `python`) so a permission gate has names to bind to. If a data-analysis role appears later, add a persistent kernel for that session type only.

---

## 6. Transferable vs not

### Steal this

| Idea | Evidence | Why |
|---|---|---|
| Attach/detach over a connection interface; session outlives the UI | `daemon-mode.ts:1-7`; `agent-connection/types.ts` (768 lines) defines `AgentConnection` implemented by both in-process and daemon | Signal and web both attach to one live session |
| Idle eviction with a default (90 min) and `"off"` | `settings-manager.ts:140`; `daemon-supervisor.ts:628-632` | Bounded memory on a Pi |
| Process identity = pid + start time | `session-lease.ts:145-180` | Any child you spawn; cheap, and #879 shows the pitfall (pin TZ/locale) |
| Session = append-only JSONL, `parentId` tree, typed entries | `session-manager.ts:97`, `:101-208`, `:282` | Forkable history, no DB, `tail -f`-able |
| `/heartbeat` as a cron job: `once` / `cron` / `interval`, `steer` vs `follow_up` delivery, deferred when busy | `cron-jobs.ts:16-27`, `:112-118`, `:1167-1200`, `:1350`; scheduler runs in the daemon `daemon-mode.ts:598` | This is JT's cron requirement, already designed. Copy the types. |
| `/goal`: `{objective ≤4000 chars, tokenBudget, tokensUsed, timeUsedSeconds, continuationsUsed, status}` stored as a session entry and survives compaction | `goals.ts:4-26`; #1316 | Durable role/objective without a DB |
| `/autonomous` budgets + verifier gates | `autonomous.ts:52-62`: 3 continuations, 12 turns, 80k tokens, 30 min; gates are shell commands with 3 retries | "Self-fixing" needs a stop rule and a checker |
| Billing vs context accounting | `agent-session.ts:1032-1040`: child usage is added to the bill but the parent's `totalTokens` (context) is unchanged | Correct and three lines |
| Compaction tells the summarizer what survives | `compaction/compaction.ts:451` | Replace "kernel" with "files under ~/agent/scratch" |
| Harness state as small JSON with kinds `prompt`/`memory`/`skill`/`subagent`, `local` vs `global` scope, and a refinement log | `refinement.ts:14-46`; `harness.py:22-30` | Durable memory of the agent's role; `refinements.jsonl` is an audit trail |
| Extension hooks with a `block` result | `extensions/types.ts:907-911`, `:1031`; 28 events incl. `tool_call`, `session_before_compact`, `turn_end` | The one gate that exists; make it first-class |
| `doctor --fix` and `shutdown --force` as CLI verbs | `cli/command-registry.ts:83-92` | Cheap ops hygiene |
| Local models via OpenAI-compatible `baseUrl` + `compat` flags | `docs/models.md:19-53` | No Ollama-specific code needed |

### Avoid this

| Anti-pattern | Evidence |
|---|---|
| No permission surface | `rg 'autoApprove\|allowedTools\|permissionMode\|askForApproval'` over `packages/coding-agent/src` and `packages/agent/src`: **0 hits**. Only `tool_call.block` exists, and it fires on a whole `{code}` string. |
| Single-tool design | `tools/index.ts:46`. There is nothing finer than "run Python" to approve. |
| Sandbox example that cannot apply | `examples/extensions/sandbox/index.ts:1-11` wraps a `bash` *tool* the model cannot call; 0 mentions of `ipython`. Inherited from `pi`, dead in Prime. |
| Claude Code client ID for subscription login | `utils/oauth/anthropic.ts:27-28` |
| God objects | `agent-session.ts` 11,948 lines; `interactive-mode.ts` 10,143; `daemon-mode.ts` 7,515; `daemon-supervisor.ts` 6,572 |
| Three processes per session with recovery journals, orphan journals, peer tickets | §2 above |
| Phone-home analytics on by default | `telemetry.ts:13`, opt-out `:208-211` |
| 22k-line generated model catalog | `models.generated.ts` |

---

## 7. Sizes

Repo: 106 MB with `.git`, 23 MB without (no `node_modules` present). Python runtime 4,581 lines. TypeScript ≈ 373k lines, of which 171,556 are tests (451 files) and 22,086 generated; **hand-written non-test TS ≈ 180k lines**.

| Package | All `.ts` lines |
|---|---|
| coding-agent | 283,473 |
| ai | 55,864 (22,086 generated) |
| tui | 28,521 |
| agent | 5,528 |

Ten largest source files (tests excluded):

| Lines | File |
|---|---|
| 22,086 | `packages/ai/src/models.generated.ts` |
| 11,948 | `packages/coding-agent/src/core/agent-session.ts` |
| 10,143 | `packages/coding-agent/src/modes/interactive/interactive-mode.ts` |
| 7,515 | `packages/coding-agent/src/modes/daemon/daemon-mode.ts` |
| 6,572 | `packages/coding-agent/src/modes/daemon/daemon-supervisor.ts` |
| 2,929 | `packages/coding-agent/src/modes/agents-view/agents-view-mode.ts` |
| 2,450 | `packages/ai/scripts/generate-models.ts` |
| 2,444 | `packages/coding-agent/src/core/package-manager.ts` |
| 2,394 | `packages/coding-agent/src/modes/agent-connection/daemon-agent-connection.ts` |
| 2,390 | `packages/tui/src/components/editor.ts` |

**Not installed.** `which prime-agent` → not found. `pi` on PATH is a pyenv shim, unrelated. No `node_modules` in the checkout, so it cannot be built here either. Startup time and binary size: not measured. `install.sh` requires Node ≥ 20.6 (`install.sh:896-899`); a `bun build --compile` path exists (`packages/coding-agent/package.json` `build:binary`) but release size is not verified.

---

## Bottom line for HARNESS_V2

Prime Agent is now a daemon-hosted, kernel-centred coding agent, not the "IPython experiment" of August. The kernel was rewritten as a plain CPython JSON-lines REPL and the RLM-1 hedge was deleted. For JT's agent, take the *ideas* — attach/detach, cron-as-heartbeat, goal state, autonomous budgets with gates, JSONL session tree, billing-vs-context accounting, harness state JSON, local models via one OpenAI-compatible path — and none of the *machinery*. Build one process, several small tools, a real permission gate, and no persistent kernel.
