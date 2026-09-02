# OpenClaw architecture: what is worth keeping

Date: 2026-09-02. Repo: `/Users/jt/Code/openclaw` at `752a983` (committed 2026-09-02 08:54). `package.json` says version `2026.8.1`; `CHANGELOG.md:6` lists `2026.8.3 (Unreleased)`, so the checkout is ahead of the 2026.8.2 release. Nothing was installed or modified. All paths below are relative to the repo root. "Est." means I computed it with a script or by summing literals, not by running the program.

This file does not repeat the August HARNESS.md material (two-loop split, tainting, CVE history, 11-layer permissions). It answers the nine questions in the brief and ends with a component map, a "steal this" list, a "do not do this" list, and the 20% answer.

## 0. Size snapshot (non-test `.ts`, any file with `.test` in its name excluded)

| Tree | Files | Lines | Note |
|---|---|---|---|
| `src/` | 8,729 | 1,880,521 | plus roughly 2.9M lines of tests |
| `extensions/` | 5,145 | 980,302 | 153 directories, 151 with `openclaw.plugin.json` |
| `ui/` | 1,447 | 338,098 | Control UI |
| `packages/` | 619 | 98,056 | agent-core, protocol, sdk shells |

Largest `src/` areas: `agents/` 407k, `gateway/` 311k, `infra/` 166k, `commands/` 132k, `plugins/` 129k, `auto-reply/` 107k, `config/` 94k, `cli/` 84k, `cron/` 34k, `daemon/` 15k.

## 1. Runtime map: startup

### The launch chain

`openclaw.mjs` -> `src/entry.ts` -> (four workaround modules) -> `src/cli/run-main.ts` -> `src/cli/gateway-cli/run.ts` -> `src/gateway/server.ts` -> `server-start.ts` -> `server-kernel.ts` + `server-startup-bootstrap.ts` + `server-core-runtime.ts` + `server-startup-finish.ts` + `server-startup-post-attach.ts`.

`openclaw.mjs:15-31` first checks for a half-finished package install (`.openclaw-lifecycle-pending`). `openclaw.mjs:33-60` and `node-version.mjs:5-9` enforce Node floors 22.22.3 / 24.15.0 / 25.9.0 and recommend Node 26 (`openclaw.mjs:35`).

`src/entry.ts` (339 lines) then does, in order: a main-module guard so the bundler cannot start two gateways (`entry.ts:109-121`), install the ESM resolve fast path (`:124`), normalize env and argv (`:128-133`), a runtime guard (`:134-141`), a compile-cache respawn check (`:144-153`), enable the compile cache (`:155-157`), a second respawn check (`:168-182`), container/profile arg parsing (`:183-206`), the `--version` fast path (`:208`), and only then the real CLI (`:209`, `:304-312`).

### What the four "fast path" files work around

| File | Problem it papers over | Evidence |
|---|---|---|
| `src/entry.esm-resolve-fast-path.ts` | Node re-reads the package `exports` map for every file resolution. The package has "300+ subpath exports" (I count 326 in `package.json`) and a gateway boot crosses "~15k module edges", costing "~0.1ms per resolution and multiple seconds". The hook short-circuits relative `./x.js` imports inside `dist/`. | `:4-12` (header comment), `:115-128` (the hook) |
| `src/entry.compile-cache.ts` | V8 parse/compile time of the big dist. It turns on `node:module` `enableCompileCache` into `$TMPDIR/node-compile-cache/openclaw/<version>/<mtime-size>` (`:88-97`, `:203-215`). In a source checkout it instead respawns the process with the cache disabled (`:111-148`). | `:19`, `:127-141` |
| `src/entry.respawn.ts` | Node prints `ExperimentalWarning` for `node:sqlite`. The fix is to respawn the whole process with `--disable-warning=ExperimentalWarning` (`:19`, `:143-151`). It also respawns to inject `NODE_EXTRA_CA_CERTS` (`:126-141`) and on Windows to raise `--stack-size=8192` (`:22`, `:108-123`). | `:76-163` |
| `src/entry.version-fast-path.ts` | `openclaw --version` used to load the Commander graph. Now it imports only `./version.js` and `./infra/git-commit.js` (`:45-53`). | `:5-63` |

The entry is a maze of `await import(...)` (`src/entry.ts:43-50`, `:290`, `:306`) so each code path loads as little as possible; `src/gateway/server.ts:1-6` does the same for the server "without paying the full startup dependency graph."

A startup tracer with ~40 named phases (`src/cli/startup-trace.ts:47`, env `OPENCLAW_GATEWAY_STARTUP_TRACE`) exists because startup time was a recurring complaint.

### Module count estimates

I wrote a script that walks `import ... from` edges (ignoring `import type`) from a set of entry files and resolves them to `.ts` files.

| Command | Internal modules loaded (est.) | Lines (est.) | Method |
|---|---|---|---|
| `openclaw --version` | ~134 | ~16.7k | static closure of `src/entry.ts` + `version.ts` + `infra/git-commit.ts` |
| `openclaw gateway` (lower bound) | ~2,441 | ~499k | static closure of `src/gateway/server-start.ts` |
| `openclaw gateway` (upper bound) | ~7,918 | ~1.81M | static + dynamic closure from `entry.ts`, `run-main.ts`, `gateway-cli/run.ts`, `gateway/server.ts`; 57 external packages reachable |

The real number sits between the two gateway bounds because dynamic imports are conditional. Either way, a gateway start touches thousands of modules; `--version` touches about a hundred.

### Gateway startup order (from the trace marks)

`server-startup-bootstrap.ts:91-494`: `process.bootstrap` -> `state.ownership` -> `state.runtime-imports` -> `state.schema-preflight` -> `runtime.network-bootstrap` -> `runtime.agent-cli` -> `config.snapshot` -> `control-ui.seed` -> `agents.github-profile-cleanup` -> `config.final-snapshot` -> `worker-environments.store-import` -> `plugins.bootstrap-imports` -> `startup.maintenance` -> `plugins.bootstrap`.
`server-startup-finish.ts:148-344`: `gateway.ws-attach` -> `http.listen` -> `http.bound` -> `runtime.post-attach` -> `ready`.
`server-startup-post-attach.ts:625-870` (after `ready`): hooks, session recovery, model runtime and auth, reply runtime, then channels (`channel-start` at `:730`) and plugin services.

Two details matter for a rewrite. Channels start after the port is bound. Cron does not start at boot: `server-startup-finish.ts:218` passes `startCron: false` and `server-cron-lazy.ts:144,162` starts it on first use. `src/gateway/boot.ts:37,104-158` runs a workspace `BOOT.md` through a throwaway agent session at startup, a self-check idea worth keeping.

`src/daemon/` (15k lines) is not a daemon. It installs and manages the OS service: launchd, systemd, and Windows `schtasks` files are all there (`ls src/daemon`).

## 2. The agent loop

`packages/agent-core` is no longer ~1,600 lines. It is 6,944 non-test lines: `agent-loop.ts` 1,776, `agent.ts` 700, `types.ts` 651, plus a `harness/` tree with compaction (`compaction.ts` 1,040). The loop itself is still a pure function that takes callbacks in `AgentLoopConfig` (`types.ts:288-363`), which is the property worth copying.

### What happens per turn (`runLoop`, `agent-loop.ts:295-535`)

1. Read steering messages once before anything (`:312-316`).
2. Outer `while(true)` for follow-ups (`:347`); inner `while(hasMoreToolCalls || pending)` (`:351`).
3. Inject pending messages into the transcript; a user message clears taint (`:361-385`).
4. Stream one assistant response (`:392-401`).
5. On `error`/`aborted` stop reasons: emit `turn_end` and `agent_end`, return (`:404-411`).
6. If the stop reason is `toolUse`, run the tool batch (`:414-423`). Taint the turn if any result is network-sourced (`:425`). Steering produced during the batch becomes the next pending set (`:427`).
7. If the batch reported a critical loop, remember it (`:428-430`); on `terminateRun`, push a fixed failure message and end (`:444-462`).
8. `prepareNextTurn` may swap model or thinking level (`:464-486`).
9. `shouldStopAfterTurn` can end the run; otherwise check steering again (`:492-507`).
10. When the inner loop ends, drain `getFollowUpMessages()`; re-check steering; if nothing, `agent_end` (`:516-534`).

There is still no maximum-turn cap. A search for `maxTurns|maxIterations|turnLimit|maxToolTurns|maxSteps` in `packages/agent-core/src` and `src/agents/embedded-agent-runner` returns nothing. The loop stops only when the model stops calling tools, a loop detector fires, the caller aborts, or the outer runner's retry budget runs out.

### Steering vs follow-up

Both are callbacks: `getSteeringMessages` (`types.ts:315`) is polled at every checkpoint inside a run; `getFollowUpMessages` (`types.ts:328`) is polled only after the inner loop would have stopped. The gateway side keeps two arrays on the session (`src/agents/sessions/agent-session-prompting.ts:627-634`). What an inbound chat message does while a run is active is a config choice, `messages.queue.mode`: `steer`, `followup`, `collect`, or `interrupt` (`src/config/zod-schema.core.ts:678-681`; help text `src/config/schema.help.automation.ts:275`). Queue cap defaults to 20 with drop policies `summarize|old|new` (`schema.help.automation.ts:280-283`); the queue itself is `src/auto-reply/reply/queue/enqueue.ts` (424 lines).

### Loop recovery budget

Detection is in `src/agents/tool-loop-detection.ts:49-52`: history window 30 calls, unknown-tool threshold 10, critical threshold 20, global circuit breaker 30. The warning threshold is a fixed 10 (`src/agents/tool-loop-thresholds.ts:1`); the comment at `:4-5` says numeric tuning "was retired in #111382" on purpose. The only config is `tools.loopDetection.enabled` (`src/agents/tool-loop-detection-config.ts:15`). Recovery is one-shot: the first critical loop blocks the batch and tells the model "Do not repeat this exact tool action. Reassess the task" (`agent-loop.ts:1578`); a second critical loop in the same run terminates it (`:309-311`, `:1576`, `:1611-1616`, message at `:75-76`). The bridge from detector to loop is `src/agents/embedded-agent-runner/run/tool-loop-recovery.ts:16-76`.

### The outer loop and its retry budget

`src/agents/embedded-agent-runner/` is 260 files and 67,202 lines; `run-loop.ts` (720 lines) wraps `runLoop` with retries, auth-profile rotation, model failover, and compaction. Attempts per run are `24 + 8 * profiles`, clamped to 32..160 (`run/helpers.ts:111-121`). A retry counts against the budget unless it was a "progress_continuation" (a resume where tool results were truncated but not errored), in which case the count is refunded (`run/retry-budget.ts:22-39`).

### Tool-call repair

`packages/tool-call-repair` (3,564 lines) rescues tool calls that models print as text. It knows the legacy `[END_TOOL_REQUEST]` marker and the Harmony `<|channel|>`, `<|message|>`, `<|call|>` markers (`grammar.ts:1-8`), accepts an allowlist of tool names and a `maxPayloadBytes` cap (`contracts.ts:2-7`), and can promote a scrubbed text block into provider-native tool-call stream events (`index.ts:13-27`). It is used by the embedded runner (`src/agents/embedded-agent-runner/run/attempt-tool-call-text-promotion.ts`) and by outbound message paths (`src/infra/outbound/message-action-send.ts`). For local models this is the single most useful piece in the package tree.

## 3. Signal channel (`extensions/signal`)

72 non-test files, 12,481 lines. Largest: `monitor/event-handler.ts` 1,390, `client-container.ts` 939, `config-compat.ts` 738, `channel.ts` 716, `monitor.ts` 657. Runtime deps are `ws` and `zod` only (`extensions/signal/package.json`).

| Concern | How OpenClaw does it | Evidence |
|---|---|---|
| Transport | Spawns `signal-cli [--config X] [-a account] daemon --http host:port --no-receive-stdout [--receive-mode M] [--ignore-attachments] [--ignore-stories] [--send-read-receipts]` | `src/daemon.ts:176-207` |
| RPC | JSON-RPC 2.0 over HTTP POST `/api/v1/rpc`; health GET `/api/v1/check` | `src/client.ts:206-212`, `:243-244` |
| Inbound | SSE stream from GET `/api/v1/events` | `src/client.ts:348` |
| Reconnect | Backoff 1s to 10s, factor 2, jitter 0.2 | `src/sse-reconnect.ts:18-23`, `:101-113` |
| Daemon watchdog | Startup timeout, exit error surfaced as "recovering" status | `src/monitor.ts:514-561`, `:637-647` |
| Alt transport | A container/REST variant (`/v1/about`) | `src/client-container.ts:233` (not studied further) |
| Methods used | `send`, `sendTyping`, `sendReceipt`, `sendReaction`, `version` | `src/send.ts:388-401,451,483`; `src/send-reactions.ts:82`; `src/client-container.ts:821-927` |
| DM policy | `pairing` (default) / `allowlist` / `open` / `disabled`; `open` requires `allowFrom: ["*"]` | `src/monitor.ts:443-449`; `src/channels/plugins/dm-access.ts:17`; `src/config-schema.ts:162` |
| Pairing | Unknown sender gets a pairing prompt; approvals stored via `src/pairing/pairing-store.ts` (state DB; table names not line-verified) | `src/channel.ts:17,614` |
| Groups | Separate `groupPolicy`, mention patterns, case-preserved group ids | `src/monitor.ts:453-456`; `src/config-schema.ts:117` |
| Chunking | Default 4,000 chars, mode `length` or `newline` | `src/auto-reply/chunk.ts:33`; `src/monitor.ts:292,354` |
| Attachments in | `dataMessage.attachments[]` mapped by `contentType`, capped by `mediaMaxBytes` | `src/monitor/event-handler.ts:1185-1190`, `:1259-1262` |
| Inbound retry | Session-init conflicts retry at 1s, 2s, 4s | `src/monitor/event-handler.ts:107`, `:714-728` |
| Outbound retry | Group sends fan out per member; a blind retry would double-deliver, so it is avoided | `src/send.ts:98` (not verified beyond the comment) |

What a from-scratch Signal integration must copy: spawn or attach to `signal-cli daemon --http`; poll `/api/v1/check` until ready; open the SSE stream and reconnect with jittered backoff; send with JSON-RPC `send`; default to pairing for unknown senders and keep an allowlist; chunk at 4,000 chars; send typing and read receipts; treat attachments as metadata plus a bounded download; and never retry a group send blindly. Ten behaviours, not 12,000 lines.

## 4. Cron (`src/cron`)

177 non-test files, 33,606 lines.

| Aspect | Detail | Evidence |
|---|---|---|
| Schedule kinds | `at` (ISO time), `every` (ms + anchor), `cron` (expr + tz + staggerMs), `stream` (fires when a gateway-owned watcher process exits) | `types.ts:27-45`; `src/agents/tools/cron-tool-schema.ts:36` |
| Payload kinds | `systemEvent` (text injected into the main session), `agentTurn` (a prompt run in an isolated session), `script` (trigger script; only if `cron.triggers.enabled`) | `types.ts:261-322`; `cron-tool-schema.ts:43-44` |
| Delivery | `none` / `announce` / `webhook`, with `channel`, `to`, `threadId`, `accountId`, and separate completion/failure destinations | `types.ts:77-120`; `cron-tool-schema.ts:45` |
| Persistence | SQLite tables `cron_jobs`, `cron_run_receipts`, `cron_job_runtime_authorities`, `cron_job_scratch` in the state DB | `src/state/openclaw-state-schema.sql:1420-1495` |
| Parser | `croner` | `schedule.ts:4` |
| Timer | One `setTimeout` armed to the earliest `nextRunAtMs`, clamped to 60s so clock jumps are caught | `service/timer-scheduler.ts:93-115`; `service/timer-execution-timeout.ts:25` |
| Failure | Backoff ladder 30s, 60s, 5m, 15m, 60m keyed by `consecutiveErrors`; auto-disable after 10 consecutive failures with a user-visible warning | `service/jobs-scheduling.ts:49-55`; `service/auto-disable.ts:14,50,76`; `types.ts:392-398` |
| Execution | `run-executor.ts:725-740` lazy-loads and calls `runEmbeddedAgent` from `src/agents/embedded-agent.js` (`isolated-agent/run-embedded.runtime.ts:4`) under session key `cron:<jobId>` (`session-reaper.ts:67`; `job-session-bindings.ts:45`) |
| Delivery path | `isolated-agent/delivery-dispatch.ts` applies a channel transform and either announces into a bound session or performs a real channel send (`:202`, `:764`) |
| Model tool | One `cron` tool with actions `status, list, get, add, update, remove, run, runs, next_check, wake` | `cron-tool-schema.ts:24-35` |
| Heartbeat | `heartbeat-monitor.ts`, `heartbeat-task.ts` live here too | `ls src/cron` |

The shape is right; the size is not. A rewrite needs the three time-based kinds, the backoff ladder, auto-disable with a user message, and one tool: a few hundred lines on top of `croner`.

## 5. Sessions, memory, and the system prompt

### Sessions

A session key is `agent:<agentId>:<mainKey>` (`src/routing/session-key.ts:206`), and all DMs share the main session by default (`dmScope` default `"main"`, `src/config/types.base.ts:225`). Transcripts are rows, not JSONL: `transcript_events(session_id, seq, event_json)` (`src/state/openclaw-agent-schema.sql:389-392`) with one `session_nodes` row per key (`:15-17`), in `~/.openclaw/agents/<agentId>/agent/openclaw-agent.sqlite` (subagent-reported path; state DB path verified at `src/state/openclaw-state-db.paths.ts:11-13`). Reset modes are `none|daily|idle`, default `none`, daily hour 4 (`src/config/sessions/reset-policy.ts:6,21-22`).

### System prompt

The builder is `buildAgentSystemPrompt` in `src/agents/system-prompt.ts:766` (file is 1,610 lines). Modes are `full`, `minimal` (subagents), `none` (identity line only) (`:78-83`). The prompt is split at `SYSTEM_PROMPT_CACHE_BOUNDARY` (`:1432`) into a stable prefix that is cached and a volatile suffix. Pieces in order (section starts reported by the sub-study; I spot-checked `:1196`, `:1207-1213`, `:1432`):

Stable prefix: identity line; `## Tooling` with one `- name: summary` line per enabled tool (`:1207-1213`); deferred tool schemas; workflow hints (subagents, screen, automations); tool-call style and approvals; execution bias; `## Safety` and credential safety; `## OpenClaw Control`; `## Skills` catalog; `## Memory Recall`; `## Model Aliases`; `## Workspace`; `## Sandbox`; bootstrap notices; assistant output directives; `## Reasoning Format`; `# Project Context` with the workspace files; `## Silent Replies`.
Volatile suffix: `## Temporal Context`; approval-pending guidance; `## Authorized Senders`; UI presentation; `## Messaging` and the message tool; voice; `## Conversation Context`; `## Reactions`; watched sessions; `## Runtime` line with model and reasoning level.

Workspace files, in order: `AGENTS.md`, `SOUL.md`, `IDENTITY.md`, `USER.md`, `BOOTSTRAP.md`, `MEMORY.md` (`src/agents/workspace.ts:246-252`). Per-file cap 20,000 chars, total cap 60,000 (`src/agents/embedded-agent-helpers/bootstrap.ts:86-87`), with head/tail truncation. `TOOLS.md` and `HEARTBEAT.md` are retired and migrated by doctor commands (sub-study; not line-verified by me).

Cost (est., no snapshot fixture exists): ~12-14k chars (~3-3.5k tokens) with no workspace files; ~28-31k chars (~7-8k tokens) with the default templates; the skills catalog can add up to 18k chars. All prose literals in `system-prompt.ts` sum to ~62k chars, an upper bound.

### Memory

`extensions/memory-core` (153 files, ~37k lines per count) is `kind: "memory"`, `onStartup: false`, and exposes `intent`, `memory_get`, `memory_search` (`openclaw.plugin.json:1-22`). It indexes `MEMORY.md`, `USER.md`, and `memory/**/*.md` into the same per-agent SQLite file using FTS5 plus a `sqlite-vec` table. Defaults: chunks of 400 tokens with 80 overlap, 6 results, min score 0.35, hybrid 0.7 vector / 0.3 text, MMR 0.7, 30-day temporal decay, 50,000-entry embedding cache, provider `openai` (`src/agents/memory-search.ts:115-137`). Recall is tool-only: the prompt gets one instruction, "Before answering anything about prior work, decisions, dates, people, preferences, or todos: run memory_search" (`extensions/memory-core/src/memory-tool-contract.ts:107-120`); no snippets are injected automatically. `extensions/memory-lancedb` (3,518 lines) is optional, adds native LanceDB, and does auto-recall (sub-study).

Compaction defaults: `reserveTokens` 16,384 and `keepRecentTokens` 20,000; it triggers when `contextTokens > contextWindow - reserveTokens` (`packages/agent-core/src/harness/compaction/compaction.ts:173-177`, `:299-307`). `src/context-engine` (1,994 lines) is a pluggable interface whose default "legacy" engine delegates back to that compactor.

## 6. Plugins and extensions

The contract has two halves. A manifest, `openclaw.plugin.json` (`src/plugins/manifest.ts:30`), declares id, kind, tools, activation, and CLI commands; the `openclaw` key in `package.json` declares entry files and channel setup metadata (`extensions/signal/package.json`). At runtime a plugin receives an API object with 62 `register*` methods (`src/plugins/plugin-api.types.ts`, 459 lines): `registerTool` (`:204`), `registerHook` (`:208`), `registerHttpRoute` (`:213`), `registerChannel` (`:222`), `registerGatewayMethod` (`:230`), `registerCli` (`:242`), `registerService` (`:260`), `registerProvider` (`:274`), and thirteen media/speech/embedding provider kinds (`:280-298`).

Loading is lazy in the sense that matters: manifests are read for all plugins, but module import and registration are gated by "this final guard also blocks imports and registration outside the requested snapshot" (`src/plugins/loader-runtime-candidate.ts:92-104`). Of 151 manifests, 34 set `activation.onStartup: true`, 27 are channels, 79 are LLM providers, and 2 are memory engines (grep of manifests). Startup pays two phases, `plugins.bootstrap-imports` and `plugins.bootstrap` (`server-startup-bootstrap.ts:481,494`), plus a manifest cache (`src/plugins/plugin-cache.ts`). Per turn, 42 hook names exist (`src/plugins/hook-types.ts:99-141`) and each dispatch is gated by `hasHooks(name)` (`src/plugins/hook-runner-global.ts:82`), so an unused hook costs a map lookup.

Sizes tell the story: `packages/plugin-sdk` + `packages/plugin-package-contract` are 378 lines of re-export shell; the real SDK is `src/plugin-sdk` at 53,748 lines and `src/plugins` at 118,141 lines. The 326 package exports are almost all `./plugin-sdk/*` subpaths. Security: "Native OpenClaw plugins run in-process with the Gateway. They are not sandboxed" (`docs/plugins/architecture.md:426-429`).

## 7. Control UI and apps

The web UI is Lit 3.3.3 built with Vite 8.2.2 (`ui/package.json:44,60`), 1,447 files and 338k lines. The route table at `ui/src/app-route-paths.ts:30-83` has 58 routes; the sidebar is dashboards, usage, automations (cron), tasks, sessions, activity, plugins, apps, portals, with chat as the landing page. It talks to the gateway over one WebSocket carrying `req|res|event` frames (`packages/gateway-protocol`), with token auth plus Ed25519 device pairing (sub-study).

For one user the features that matter all exist: chat, sessions list, cron editor with run history, settings, and usage/cost. Everything else (workboard, fleet, cloud workers, portals, skill workshop, plugin hub) is multi-user surface.

## 8. Dependencies

Root `package.json`: 65 `dependencies`, 60 `devDependencies`, 1 optional (`sqlite-vec`), 326 `exports`, engines `>=22.22.3 <23 || >=24.15.0 <25 || >=25.9.0`. `pnpm-lock.yaml` is lockfile v9.0 with ~1,650 resolved package entries (my awk count 1,653; sub-study 1,644). SQLite is the built-in `node:sqlite`; `better-sqlite3` does not appear anywhere.

Fifteen heavy or surprising entries (root unless noted): `playwright-core` (browser control), `koffi` (C FFI), `@lydell/node-pty` (native PTY), `web-tree-sitter` + `tree-sitter-bash` (shell parsing), `quickjs-wasi` (sandboxed JS), `@silvia-odwyer/photon-node` (WASM images), `rastermill` (native image resize), `clawpdf` (native PDF), `@trycua/cua-driver` (computer use), `@openclaw/fs-safe` with 8 platform binaries, `typescript` as a runtime dependency, `@earendil-works/pi-tui`, `express` and `undici` both, `@lancedb/lancedb` + `apache-arrow` (`extensions/memory-lancedb`), and `baileys` (`extensions/whatsapp`). Four packages are patched in `patches/`.

## 9. SQLite schema

Re-verified by counting `CREATE TABLE` in the two schema files: 121 tables in `src/state/openclaw-state-schema.sql` and 39 in `src/state/openclaw-agent-schema.sql`, up from 114 + 31 in August. Versions: state 15, agent 19 (`src/state/openclaw-state-db-contract.ts:15`, `src/state/openclaw-agent-db-contract.ts:24`).

Groups (state DB, sub-study counts): sessions/workers/transcripts 23; gateway runtime and config 21; auth/devices/pairing/push 13; workspaces/worktrees/fleet 12; channels and outbound delivery 11; skills library and workshop 11; approvals/exec policy/secrets 10; plugins/ClawHub/migrations 10; cron 5; audit/diagnostics 5. Agent DB: sessions 13, memory index 9, transcripts 7, conversations 2, auth 2, board 2, misc 4.

The ten a single-user Signal agent needs: `session_nodes`, `transcript_events`, `conversations`, `conversation_deliveries`, `session_pending_inputs`, `memory_index_chunks`, `memory_index_sources`, `cron_jobs`, `cron_run_receipts`, and a pairing/allowlist table. Plus one `schema_meta` per file. That is 11-12 tables, not 160.

## Component map

| Area | Path | Approx lines | Purpose | Single-user verdict |
|---|---|---|---|---|
| Launch shims | `openclaw.mjs`, `src/entry*.ts` | 1,100 | Node checks, respawns, caches, fast paths | Skip; symptoms of size |
| CLI | `src/cli`, `src/commands` | 216k | Commander tree, doctor, setup wizards | Skip; keep ~10 commands |
| Gateway | `src/gateway` | 311k | WS/HTTP server, auth, RPC methods, startup phases | Keep the idea (one process, WS to UI); rewrite at 1/100 size |
| Service install | `src/daemon` | 15k | launchd/systemd/schtasks | Keep two templates, skip the rest |
| Inner loop | `packages/agent-core` | 6.9k | Pure loop, queues, taint, compaction | Keep the design |
| Outer loop | `src/agents/embedded-agent-runner` | 67k | Retries, failover, auth rotation, prompt build | Keep retry budget + refund; skip failover matrix |
| Tool-call repair | `packages/tool-call-repair` | 3.6k | Rescue text-emitted tool calls | Keep, trimmed to 300 lines |
| Queues/dispatch | `src/auto-reply` | 107k | Inbound debounce, queue modes, reply delivery | Keep 4 queue modes + cap; skip the rest |
| Channel core | `src/channels` | 47k | Policy, pairing, routing shared by channels | Keep DM policy enum |
| Signal | `extensions/signal` | 12.5k | signal-cli bridge | Keep 10 behaviours |
| Cron | `src/cron` | 34k | Scheduler, receipts, delivery, heartbeat | Keep model + backoff + auto-disable |
| Sessions/state | `src/sessions`, `src/state`, `src/config/sessions` | ~29k + schema | SQLite stores, reset policy | Keep SQLite, 3 tables, reset modes |
| Memory | `extensions/memory-core` | 37k | FTS5 + vec hybrid over markdown | Keep FTS5 + tool-only recall; vec optional |
| LanceDB memory | `extensions/memory-lancedb` | 3.5k | Native vector store | Skip |
| Context engine | `src/context-engine` | 2k | Pluggable compaction interface | Skip the interface; keep the compactor |
| Plugin core | `src/plugins`, `src/plugin-sdk` | 172k | Manifests, loader, 62 registration kinds, 42 hooks | Skip; use a 5-function tool interface |
| Config | `src/config` | 94k | Zod schema, help text, migrations | Skip; one typed file |
| Infra | `src/infra` | 166k | Net, proxy, TLS, updates, process | Skip |
| Control UI | `ui/` | 338k | Lit app, 58 routes | Keep 4 pages |
| Native apps | `apps/*` | n/a | Companion nodes | Skip |
| Other channels | 26 more in `extensions/` | large | Telegram, WhatsApp, Slack, ... | Skip |
| Providers | 79 in `extensions/` | large | LLM adapters | Keep 3: Anthropic, OpenAI-compatible, Ollama |

## Steal this

1. **A pure loop with callback seams.** `runLoop` takes `getSteeringMessages`, `getFollowUpMessages`, `shouldStopAfterTurn`, `prepareNextTurn`, `beforeToolBatch` (`packages/agent-core/src/types.ts:288-363`). Everything operational plugs in from outside. That makes the loop testable and swappable.
2. **Two queues, four modes.** Steering interrupts; follow-up waits (`agent-loop.ts:312-316`, `:516`). Mode `steer|followup|collect|interrupt` with a cap of 20 and a summarize-on-drop policy (`src/config/zod-schema.core.ts:678-681`; `schema.help.automation.ts:275-283`). A person on Signal types while the agent works; this handles it.
3. **Fixed loop thresholds and one-shot recovery.** 10/20/30 with no knobs (`src/agents/tool-loop-detection.ts:49-52`; `tool-loop-thresholds.ts:1-5`), one recovery message, then stop (`agent-loop.ts:1578`, `:1611-1616`). Knobs were removed on purpose.
4. **Retry budget that refunds progress.** `recordRunRetry` decrements the count when the retry only continued work (`run/retry-budget.ts:35-39`). Retries that make progress should not exhaust the budget.
5. **Text-to-tool-call repair for local models.** Marker grammar plus allowlist and payload cap (`packages/tool-call-repair/src/grammar.ts:1-8`, `contracts.ts:2-7`). Ollama and LM Studio models leak tool calls as text; this fixes it in one place.
6. **Cron backoff ladder and auto-disable with a message.** 30s to 60m (`src/cron/service/jobs-scheduling.ts:49-55`), disable after 10 and tell the user why (`service/auto-disable.ts:14,50`). A single 60s-clamped timer (`timer-execution-timeout.ts:25`) survives sleep and clock jumps.
7. **Signal via signal-cli's HTTP daemon.** JSON-RPC send plus SSE receive with jittered backoff (`extensions/signal/src/client.ts:206-212,348`; `sse-reconnect.ts:18-23`). No native Signal library needed.
8. **Pairing as the default DM policy.** Unknown senders must be approved (`extensions/signal/src/monitor.ts:443`; `src/channels/plugins/dm-access.ts:17`).
9. **A prompt cache boundary.** Stable prefix first, volatile facts (date, runtime line) last (`src/agents/system-prompt.ts:1432`). This is what makes provider prompt caching pay off.
10. **Workspace markdown with caps.** Six named files, 20k per file, 60k total, head/tail truncation (`src/agents/workspace.ts:246-252`; `embedded-agent-helpers/bootstrap.ts:86-87`). Role memory as files the user can edit.
11. **Memory as FTS5 in the same SQLite file, recalled by a tool.** Hybrid defaults and min score (`src/agents/memory-search.ts:115-137`); one prompt instruction (`memory-tool-contract.ts:107-120`). No auto-injection means no injected garbage.
12. **Startup trace marks and a BOOT.md self-check.** `OPENCLAW_GATEWAY_STARTUP_TRACE` phases (`src/cli/startup-trace.ts:47`) and `src/gateway/boot.ts:104-158`. Cheap to add on day one; impossible to retrofit later.
13. **`node:sqlite`, no native driver.** Zero hits for `better-sqlite3`; build never breaks on a native module.

## Do not do this

1. **Respawn the process to set a Node flag.** `src/entry.respawn.ts:19,143-151` restarts Node to silence one warning; `:126-141` restarts again for CA certs. Every launch may run two processes.
2. **Ship 326 subpath exports and then hook the module resolver to survive them.** `src/entry.esm-resolve-fast-path.ts:4-12` documents "~15k module edges" and "multiple seconds". The fix for slow resolution was a resolver hook, not fewer modules.
3. **A compile-cache dance with its own respawn.** `src/entry.compile-cache.ts:111-148`. If startup needs a V8 cache, the program is too big.
4. **Let the inner loop run without a turn cap.** No `maxTurns` anywhere in `packages/agent-core` or the embedded runner. Combined with retry budgets up to 160 attempts (`run/helpers.ts:111-114`), "gets stuck" is a design outcome.
5. **1.88M lines in `src/`; a 35,588-line CHANGELOG with 723 lines about startup.** Size makes every update risky, which is the core complaint.
6. **160 SQLite tables for a chat agent.** 121 + 39 with schema versions 15 and 19 and a retirements file. Each version bump is a migration that can fail on a Pi.
7. **In-process plugins with 62 registration points and 42 hooks, no sandbox.** `src/plugins/plugin-api.types.ts`; `docs/plugins/architecture.md:426-429`. Extensibility here is the attack surface.
8. **A 1,610-line prompt builder emitting ~45 sections.** `src/agents/system-prompt.ts`. Default cost is an estimated 7-8k tokens per turn before the conversation starts; every section is a chance for the model to misread a rule.
9. **Bundle 27 channels and 79 providers in one process.** Even lazy-loaded, they cost manifests, config schema, docs, and tests on every release.
10. **Native and FFI dependencies in the root.** `koffi`, `node-pty`, `rastermill`, `clawpdf`, `fs-safe` binaries, `typescript` at runtime. Each one is a build break waiting for a new Node or a new ARM Linux.
11. **Test scaffolding larger than the code.** One test-utils file is 3,367 lines; `src/` tests total ~2.9M lines. The code cannot be tested without a mock of itself.

## The 20% that delivers 80%

For one person talking to the agent on Signal, the value is: a reliable inbound/outbound Signal bridge; a loop that calls one model, runs a handful of tools, and stops; a role that persists in editable files; sessions that survive restarts; cron with backoff; searchable memory; and a small web page to read transcripts and edit jobs. That is roughly:

| Piece | OpenClaw lines | Clean rewrite (est.) |
|---|---|---|
| Signal bridge (daemon spawn, RPC, SSE, pairing, chunking) | 12,481 | 500-700 |
| Inner loop + queues + loop detector + taint flag | 6,944 + parts of 67k | 400-500 |
| Provider adapters: Anthropic, OpenAI-compatible (covers LM Studio, Ollama) | 79 plugins | 300-400 |
| Tool-call text repair (markers, allowlist, cap) | 3,564 | 200-300 |
| Tools: shell, read/write file, web fetch, message, cron, memory_search | ~50 tools | 400-500 |
| Sessions + transcripts in SQLite (3 tables) + reset modes | 29k + schema | 200-300 |
| Prompt builder + 4 workspace files with caps | 1,610 + 2,700 | 150-200 |
| Cron: at/every/cron, backoff, auto-disable, one tool | 33,606 | 400-500 |
| Memory: FTS5 over markdown, one tool | 37,020 | 150-250 |
| Config (one typed file) + CLI (start, stop, status, pair, logs) | 94k + 216k | 300-400 |
| Web UI: chat, sessions, cron, settings (server-rendered or tiny SPA) | 338k | 800-1,200 |
| Service files (launchd, systemd) + startup self-check | 15k | 100-150 |

Total: about 4,000-5,500 lines of TypeScript, plus the UI. Against ~3.3M non-test lines in the repo, the 20% is well under 1%. Every row has a cited reference implementation above.

Not verified in this study: measured startup wall-clock times (no `dist/` build present), the exact rendered system prompt size (no snapshot fixture), the memory-lancedb auto-recall default, the state-DB pairing table line numbers, and the precise count of gateway RPC methods (`src/gateway/server-methods-list.ts` is 99 lines; ~52 entries by pattern).
