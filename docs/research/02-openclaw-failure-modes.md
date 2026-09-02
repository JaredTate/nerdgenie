# OpenClaw failure modes: why it breaks on update, gets stuck, and runs slow

Root-cause study, 2026-09-02. Repo: `/Users/jt/Code/openclaw` at HEAD `752a983` (v2026.8.2 tagged 2026-08-31; 2026.8.3 unreleased). All paths below are relative to that repo unless they start with `f346846aa85^:`, which means "the file as it was before the Aug 30 fix commit", i.e. the code JT's installed updater was running. The prior report (`HARNESS.md`, section "5. OpenClaw") already covered the architecture, taint tracking, steering vs follow-up queues, and the CVE history. This document covers only failure modes.

Short version: OpenClaw is a 1.9-million-line always-on daemon with a 317-thousand-line operations layer wrapped around a 67-thousand-line agent loop. The operations layer changes faster than anyone can test it (11,082 commits by 377 authors in August), it swaps its own code under running processes, and it refuses to proceed whenever it cannot prove facts about the host. Those three things together explain every line in JT's screenshot.

---

## 1. Why updates break

### 1.1 The screenshot, line by line

| Screenshot text | Emitted by | What the check is for | Why it fires on a normal nvm install |
|---|---|---|---|
| `SERVICE_DEFINITION_UNKNOWN: [unsafe-permissions] The service directory (~/.config/systemd/user) or its nearest existing ancestor is group/world-writable...` | Check: `src/daemon/systemd-definition-mutation.ts:151-152` (`if (actual.mode & 0o022)`); message: `src/daemon/service-types.ts:97-107`; thrown by `assertServiceDefinitionWritable` at `src/daemon/service-types.ts:129-148` | On a shared host, if another user can write to the folder holding your systemd unit, they can change `ExecStart` and run code as you at the next restart. | Any of `~`, `~/.config`, `~/.config/systemd`, or `~/.config/systemd/user` with group- or world-write. If the leaf is missing, the nearest existing parent is tested (`src/daemon/systemd-definition-mutation.ts:130`). Debian and Ubuntu give each user a private group and a 002 umask, so a directory the user made by hand is 775: group-writable by a group containing only that user. Harmless, but `& 0o022` flags it. OpenClaw's own `mkdir` uses 0755 (`src/daemon/systemd-definition-mutation.ts:214`) so only user-made or tool-made directories trip it. Which directory on JT's box: not verified. |
| `Gateway install blocked:` | `src/cli/daemon-cli/install.ts:223`, inside `assertWritable` (`src/cli/daemon-cli/install.ts:205-225`) | Wraps the check above. Note `src/cli/daemon-cli/install.ts:170-180` also refuses to run under sudo and the message says not to use `chmod -R`, sudo, or `--force`. | There is no override. The user must guess which of four directories is meant. |
| `updated install refresh fail (...dist/index.js)` and `Failed to refresh gateway service environment from updated install` | The updater spawns the *new* version as `node dist/index.js gateway install --force` (`src/cli/update-cli/update-command-service-command.ts:56-73`, 60 s timeout at `src/cli/update-cli/update-command-service-command.ts:8`), called from `src/cli/update-cli/update-command-service.ts:348`; error text at `src/cli/update-cli/update-command-service-command.ts:77-79`; printed at `src/cli/update-cli/update-command-service.ts:369-372`. | Rewrites the systemd unit so it points at the new package and Node. | Fails because the child hit the permission check. |
| `Gateway: restarted and verified.` / `Daemon restarted successfully.` | `src/cli/update-cli/update-command-service.ts:273` and `src/cli/update-cli/update-command-service.ts:497`. The failure above matches `DEFINITION_DENIAL` (`src/cli/update-cli/update-command-service-command.ts:9`), so `src/cli/update-cli/update-command-service.ts:373-376` sets `preserveDefinition = true` and continues to a restart with the *old* unit file. | Designed so a "sealed" unit owned by a deployment tool is never rewritten. | The old unit still runs `/usr/bin/node`. The update "succeeded" while leaving the service on a runtime the updater had just warned about. |
| `Doctor failed: Error [ERR_MODULE_NOT_FOUND]: Cannot find module '.../dist/doctor-health-Bn9fZuuK.js' imported from .../dist/doctor-CZZ_7dPi.js` | JT's updater (8.1 or older) ran Doctor **inside the old updater process** right after the restart: `f346846aa85^:src/cli/update-cli/update-command-service.ts:1078-1086` (`await doctorCommand(defaultRuntime, ...)` then `Doctor failed: ${err}`). `src/commands/doctor.ts:158` does `await import("../flows/doctor-health.js")`. tsdown emits that as a hashed chunk. npm had already replaced `dist/`, so the chunk name the old code wanted no longer existed. | Nothing; it is a bug class the code knows about. `src/cli/update-cli/update-command.ts:635` says "Preload execution and recovery before the package swap can remove these chunks." `src/cli/update-cli/update-command-execution.ts:198` refuses to update from inside the gateway because "the live gateway may still lazy-load old chunks." `tsdown.config.ts:378-379` keeps a hand-maintained list of "stable filenames so rebuilt dist/ trees do not strand already-running gateways on stale hashed chunks." | Fixed on Aug 30 by moving Doctor to a fresh process (`src/cli/update-cli/update-command-fresh-doctor.ts:118-146`, commit `f346846aa85`, shipped in 8.2). JT's updater predates that fix. The same class recurs: `CHANGELOG.md:1564` (8.1, "stale hashed chunks") and `CHANGELOG.md:79` (8.3, another `ERR_MODULE_NOT_FOUND` in npm installs). |
| `Upgraded! Now with 23% more sass.` | Removed Aug 30 (`69f17266eb0`); taglines live in `src/cli/tagline.ts`. | Cosmetic. | Proves the running updater was older than 8.2. |
| `Current Node (~/.nvm/.../v24.18.0/bin/node) differs from the managed gateway service Node (/usr/bin/node). Using the managed service Node for this update` | `src/cli/update-cli/update-command.ts:393-402`; Node read from the unit's `ExecStart` by `src/cli/update-cli/update-command-service-plan.ts:140-148` and `src/cli/update-cli/update-command-service-plan.ts:159-183`. | If the unit runs `/usr/bin/node`, installing with a different Node could give the service native modules it cannot load. | The install script installs Node from nodesource (`scripts/install.sh:2343-2375`) and writes the unit with that absolute path. JT's shell uses nvm. Two Nodes on one box is the normal state for anyone who uses nvm. |
| `Update refused: package manager owner is unknown; no changes were made.` | `src/cli/update-cli/shared.ts:385-397`, calling `detectGlobalInstallManagerForRoot` at `src/infra/update-global.ts:1256-1312`. | Avoid running `npm i -g` when pnpm or Bun owns the install, which would leave two copies. | It runs `npm root -g` (`src/infra/update-global.ts:1271`) with the shell's PATH (`src/cli/update-cli/shared.ts:460-465` does not rewrite PATH). Under nvm that returns `~/.nvm/versions/node/v24.18.0/lib/node_modules`, not `~/.local/lib/node_modules`, so no match. Fallback looks for `~/.local/bin/npm` (`src/infra/update-global.ts:761-769`), which usually does not exist. Result: null, refuse. |

### 1.2 Root cause A: the update swaps code under running processes

OpenClaw is bundled into `dist/` as many hashed chunks. Both the gateway and the updater lazy-load chunks with `await import()` (773 dynamic import sites reachable from `server-start.ts`; 1,830 if every lazy branch is followed). `npm install -g` deletes the old `dist/` and writes a new one. Any process that was running old code and then reaches a lazy import asks for a filename that is gone. The project fights this with a stable-filename allowlist (`tsdown.config.ts:378-395`), a "preload before swap" comment (`src/cli/update-cli/update-command.ts:635`), a refusal to update from inside the service (`src/cli/update-cli/update-command-execution.ts:190-202`), and a fresh-process Doctor (`src/cli/update-cli/update-command-fresh-doctor.ts:118-146`). Each of those is a patch for one lazy import that bit someone. The next lazy import is one PR away.

### 1.3 Root cause B: the updater tries to prove things about the host that a user-space tool cannot know

Three checks in the screenshot are all "inspect the host, refuse if unsure":

- File permissions of four directories, with no override (`src/daemon/systemd-definition-mutation.ts:130-152`; `src/cli/daemon-cli/install.ts:170-180`).
- Which package manager "owns" the install, decided by asking whatever `npm` is first on PATH (`src/infra/update-global.ts:1256-1312`).
- Which Node the service uses, parsed out of the unit file's `ExecStart` (`src/cli/update-cli/update-command-service-plan.ts:140-148`).

Each check is reasonable on a shared, locked-down server. Each is a false positive on a single-user Linux box with nvm. The refusal count is large: 26 distinct "refused" or "blocked" messages in `src/cli/update-cli`, `src/cli/daemon-cli`, and `src/daemon`. The reasons map alone has eight entries (`src/daemon/service-types.ts:104-121`): unsafe-permissions, invalid-artifact, symlink, foreign-owner, sealed-mount, system-owned, system-ownership-unverified, inspection-failed.

### 1.4 Root cause C: the update ships new refusals, so run N breaks run N+1

This is the exact sequence in the screenshot:

1. JT ran the 8.1 updater. Its owner detection fell back to npm when unsure: `f346846aa85^:src/cli/update-cli/shared.ts:378-384` ends with `return byPresence ?? "npm";`. So the update ran.
2. The update installed 8.2, which added the refusal on Aug 31: commit `38ecc4173b2` "fix(update): retain pnpm owner across launcher respawn", now at `src/cli/update-cli/shared.ts:392-396`.
3. JT ran `openclaw update` again. The new updater refused to touch the install it had itself just written.

The permission check is similar: `src/daemon/systemd-definition-mutation.ts` was created on Aug 28 (`83dce46a7da`, "preserve sealed service definitions during code updates") and the `[unsafe-permissions]` wording on Aug 28 (`5d8ef653cd7`), both shipped in 8.1. A box that updated fine in July hit a brand-new refusal in late August. No changelog entry mentions `unsafe-permissions`, `SERVICE_DEFINITION`, `world-writable`, or `nvm` (zero hits across the 8.1, 8.2, and 8.3 sections).

### 1.5 Root cause D: the operations layer is bigger than the product and churns daily

| Area (non-test `.ts`) | Files | Lines |
|---|---|---|
| `src/cli/update-cli/` | 38 | 9,273 |
| `src/daemon/` | 71 | 14,267 |
| `src/commands/doctor*` + `src/commands/doctor/` | 254 | 70,979 |
| `src/commands/` (all) | 532 | 129,698 |
| `src/infra/` (all) | 743 | 163,861 |
| Ops subtotal (update-cli + daemon + commands + infra) | 1,384 | 317,099 |
| `src/agents/embedded-agent-runner/` (the agent loop) | 259 | 67,066 |

Ratio: 4.7 lines of install, update, doctor, and service code for every line of agent loop. The updater coordinates parent and child processes through 22 distinct `OPENCLAW_UPDATE_*` environment variables (476 distinct `OPENCLAW_*` variables in `src/` overall). In August 2026 alone, 538 non-merge commits touched `src/cli/update-cli`, `src/daemon`, `src/commands/doctor*`, and `src/infra/update*`. Across the whole repo in August: 11,082 non-merge commits, 377 authors, 13,116 files changed. In the two days between the 8.2 tag and today's HEAD: 985 non-merge commits.

### 1.6 What a design that cannot fail this way looks like

- One executable file. Bundle to a single JS file or a Bun-compiled binary. There is no chunk to lazy-load, so there is no `ERR_MODULE_NOT_FOUND` after a swap.
- Versioned install directories with an atomic switch: `releases/<version>/` plus a `current` symlink. The running process keeps its old directory; the new process starts from the new one; rollback is flipping the link back. The old files are deleted on the next boot, never during an update.
- The updater is a 100-line script that never imports the app. It downloads, verifies a checksum, unpacks to a new directory, flips the link, restarts, and polls `/healthz` for the new version string. If health fails, it flips the link back.
- One runtime, pinned. The unit file and the CLI both run the same absolute path. No `which node`, no `npm root -g`, no PATH inspection.
- Host hygiene checks print a warning and continue. A check the user cannot override is a bug, not a feature, on a single-user machine.
- Doctor is a read-only report. Repairs are separate, explicit commands with a dry run.

---

## 2. Why it gets stuck

### 2.1 The concurrency model in plain English

Every agent turn is queued twice. First into a per-session lane named `session:<key>` that runs one task at a time (`src/agents/embedded-agent-runner/lanes.ts:6-9`; `src/agents/embedded-agent-runner/run-orchestrator.ts:155-156`). From inside that task it queues again into a global lane (`main`, `subagent`, `cron`, `nested`) whose width comes from config (`src/process/command-queue.ts:544-553`; `src/config/agent-limits.ts:5-6`). A second, separate in-memory map, `activeRunsByKey`, tracks one `ReplyOperation` per session (`src/auto-reply/reply/reply-run-registry.state.ts:32,46`). When a new message arrives while a run is active, the default is to "steer" it into the running turn (`src/auto-reply/reply/queue/settings.ts:31-36`); otherwise it is queued as a follow-up, capped at 20, in process memory (`src/auto-reply/reply/queue/state.ts:53-55,63`). Nothing about a turn is written to disk as a lease. The only file lock is the gateway's own pid lock (`src/infra/gateway-lock.ts:30-32`).

Two numbers set the ceiling. The default agent turn timeout is 48 hours (`src/agents/timeout.ts:13-14`). The lane task timeout is that plus 30 seconds (`src/agents/embedded-agent-runner/run/lane-runtime.ts:11,21-22`). Waiting for a lane has no timeout at all; `warnAfterMs` (default 2 s, `src/process/command-queue.ts:555`) only logs.

### 2.2 Where one stuck promise wedges a session

1. A tool that ignores the abort signal is never killed. The code says so: "JavaScript cannot cancel a running promise: a tool that never observes the signal keeps executing in the background" (`src/agents/agent-tools.abort.ts:20-27`). Abort only detaches the result.
2. Subprocess helpers have no default deadline. `runCommandWithTimeout` sets no timer when `timeoutMs` is omitted (`src/process/exec-runner.ts:108-109`). Every caller must remember.
3. In-process restarts (SIGUSR1) can skip `finally` blocks, "leaving stale active task IDs that permanently block new work" (`src/process/command-queue.ts:690-703`). The fix is a global lane reset, which exists because the wedge was observed.
4. A lane task with no run handle is released only after 5 minutes of no progress (`src/logging/diagnostic-stuck-session-recovery.runtime.ts:36-40` and `src/logging/diagnostic-stuck-session-recovery.runtime.ts:346-349`).
5. The stale-run clocks are pinned by two competing bugs. `BLOCKED_TOOL_CALL_ABORT_FLOOR_MS = 15 min` with the comment "lowering it reopens #88870, removing it reopens #96168"; `RUN_STALE_TAKEOVER_MS = 10 min` (`src/logging/diagnostic-run-activity-snapshot.ts:95-102`).
6. The user cannot help. A steered message never refreshes liveness, on purpose: "sub-10-minute user messages [would] re-arm a wedged run's staleness window forever" (`src/auto-reply/reply/reply-run-registry.message-injection.ts:209-210`). Heartbeats during an active run are dropped (`src/auto-reply/reply/queue-policy.ts:19-21`).
7. Queued work can vanish silently. The follow-up queue is memory-only (`src/auto-reply/reply/queue/state.ts:63`). At the cap with `drop: new`, the message is rejected with a log line (`src/auto-reply/reply/queue/enqueue.ts:198-209`). A completed follow-up with no route is logged and discarded (`src/auto-reply/reply/followup-delivery.ts:339-352`). On restart, in-flight runs are aborted with reason `restart` (`src/gateway/server-close.ts:401-403`).
8. Known deadlock paths are timed around, not removed. The pending-tool drain gives up after 30 s "to avoid session deadlock" (`src/auto-reply/reply/pending-tool-task-drain.ts:2,32,63`).

### 2.3 The safety nets and how long each one waits

| Net | Waits | Where |
|---|---|---|
| CLI backend no-output watchdog | 3 to 10 min fresh, 1 to 3 min on resume | `src/agents/cli-watchdog-defaults.ts:1-14` |
| Diagnostic "stuck session" warning | 2 min | `src/logging/diagnostic.ts:84` |
| Stuck-session abort (no progress) | 5 min | `src/logging/diagnostic-stuck-session-recovery.runtime.ts:37` |
| Reply-run stale takeover | 10 min | `src/logging/diagnostic-run-activity-snapshot.ts:102` |
| Blocked tool call abort floor | 15 min | `src/logging/diagnostic-run-activity-snapshot.ts:99` |
| Orphaned run context sweep | 30 min | `src/infra/agent-run-registry.ts:738-739` |
| Agent turn timeout | 48 h | `src/agents/timeout.ts:13` |
| Waiting in a lane | never | `src/process/command-queue.ts:555` |
| Gateway shutdown drain | 325 s, unit `TimeoutStopSec=330` | `src/cli/gateway-cli/run-loop.ts:487-492`; `src/daemon/systemd-unit.ts:88` |

The root cause in one sentence: there is no single owner of "is this turn alive?". At least five independent timers read different clocks, each added for a specific issue number, and the best case for a wedged session is 5 to 15 minutes of silence before anything reacts.

Restart recovery exists but is partial. SIGTERM drains active work for up to 325 s and then proceeds (`src/cli/gateway-cli/run-loop.ts:623-671`). Main sessions interrupted while holding a transcript lock are resumed after restart (`src/agents/main-session-recovery/main-session-restart-recovery.ts:1-3`). Three unclean boots in five minutes stop channel auto-start (`src/infra/gateway-boot-lifecycle.ts:25-26`); systemd adds `StartLimitBurst=5` per 60 s and `Restart=always` (`src/daemon/systemd-unit.ts:77-83`). The outbound delivery queue is the one thing done right: it is in SQLite, replayed on boot, retried 5 times with backoff 5 s to 10 min (`src/infra/outbound/delivery-queue-recovery.ts:94`; `src/infra/delivery-recovery.shared.ts:18`).

### 2.4 What the changelog says

Across the 8.1, 8.2, and 8.3 sections: "recovery" 105 times, "restart" 80, "timeout" 20, "stall" 18, "lock" 13, "hang" 9, "deadlock" 3, "stuck" 3. Examples of the same wedge being fixed again: "prevent late-aborting prompts from reacquiring orphaned session locks after teardown" (`CHANGELOG.md:188`, 8.3); "restore merging of concurrent first writes after a startup optimization regressed lock ordering" (`CHANGELOG.md:51`, 8.3); "release Doctor's database handles before restarting, so completed session migrations no longer leave the Gateway blocked by Doctor's own database leases" (`CHANGELOG.md:304`, 8.2); "let follow-up work continue after stuck-session recovery" (`CHANGELOG.md:1352`, 8.1); "bound opening handshakes so stalled TCP peers cannot hang channel startup indefinitely" (`CHANGELOG.md:1573`, 8.1).

---

## 3. Fragility map from the changelog

Method: every top-level bullet under `### Fixes` in `CHANGELOG.md` for 2026.8.3 (lines 6-210), 2026.8.2 (211-1141), and 2026.8.1 (1142-18633), bucketed by subsystem. Cells are plus or minus 3 because some entries straddle buckets.

| Bucket | 8.3 (unreleased) | 8.2 | 8.1 | Total |
|---|---|---|---|---|
| update/install/doctor | 18 | 27 | 30 | 75 |
| gateway lifecycle/restart/recovery | 11 | 8 | 19 | 38 |
| channels | 27 | 10 | 54 | 91 |
| Control UI/apps | 38 | 12 | 67 | 117 |
| agent loop/sessions/compaction | 37 | 18 | 100 | 155 |
| providers/models | 17 | 5 | 32 | 54 |
| security | 7 | 4 | 20 | 31 |
| other | 12 | 8 | 27 | 47 |
| **Total Fixes** | **167** | **92** | **349** | **608** |

Update/install/doctor is the largest bucket in 8.2 (27 of 92, 29 percent). Update plus gateway lifecycle is 38 percent of 8.2's fixes. About 24 of 608 entries use "restore" wording, meaning something that worked stopped working; the same Bun 1.4 breakage is "restored" in both 8.2 (`CHANGELOG.md:259`) and 8.3 (`CHANGELOG.md:47`).

| Keyword (prose sections only) | 8.3 | 8.2 | 8.1 | Total |
|---|---|---|---|---|
| deadlock | 2 | 1 | 0 | 3 |
| stall | 3 | 1 | 14 | 18 |
| stuck | 1 | 0 | 2 | 3 |
| hang | 0 | 1 | 8 | 9 |
| timeout | 5 | 3 | 12 | 20 |
| lock | 5 | 0 | 8 | 13 |
| recovery | 20 | 26 | 59 | 105 |
| restart | 15 | 19 | 46 | 80 |
| regress | 1 | 0 | 0 | 1 |

Release scale, verbatim: `docs/releases/2026.8.2.md:15` "784 pull requests, 10 direct commits, and 134 contributors"; `docs/releases/2026.8.1.md:23` "16,977 pull requests, 698 direct commits, and 987 contributors". What that velocity implies: 8.2 was a short release and still merged 784 PRs; 8.1 merged one PR every 30 minutes around the clock for a month. No review process holds behaviour stable at that rate. The 8.3 section is 205 lines long and already has 167 fixes, a higher fix density per line than either shipped release. Every `openclaw update` pulls in hundreds of behaviour changes to the install and service code itself, so "breaks on every update" is the expected outcome, not bad luck.

---

## 4. Why it is slow

| Measure | Value | Source |
|---|---|---|
| Files statically imported when the gateway starts | 3,536 of 8,703 non-test `src/` files (41 percent) | transitive import walk from `src/gateway/server-start.ts` |
| Same, if every lazy `import()` fires | 6,436 (74 percent) | same walk, following dynamic imports |
| Files imported by the bare CLI entry | 1,377 | walk from `src/entry.ts` |
| Workarounds that exist because startup is slow | version fast path, ESM resolve fast path, compile-cache respawn | `src/entry.version-fast-path.ts`, `src/entry.esm-resolve-fast-path.ts`, `src/entry.compile-cache.ts:1` |
| `CREATE TABLE` statements / distinct tables | 318 / 238 | `src/state/openclaw-state-schema.sql` (121 tables), `src/state/openclaw-agent-schema.sql` (41 per agent), plus plugins |
| Files that open SQLite | 246 | `node:sqlite` imports, non-test |
| Per-turn SQLite touch points (by module name) | session store, transcript index, ingress queue, delivery queue, auth profiles, audit events, session-state events, memory search | `src/config/sessions/session-accessor.sqlite-*.ts`, `src/channels/message/ingress-queue.ts`, `src/infra/delivery-queue-sqlite-bound.ts`, `src/agents/auth-profiles/sqlite.ts`, `src/audit/*`. Exact write count per turn: not measured. |
| Plugin hook names / hook checkpoints per turn | 42 / about 20 | `src/plugins/hook-types.ts:99-141`; each guarded by `hasHooks()` |
| Taint checks | 16 files, 75 occurrences | small; not a cost driver |
| System prompt builder | 1,610 lines; about 45,000 chars of prompt text in the union of sections (roughly 11,000 tokens at 4 chars per token; a rendered prompt is a subset) | `src/agents/system-prompt.ts:766` `buildAgentSystemPrompt` |
| Workspace files injected every turn | AGENTS.md, SOUL.md, IDENTITY.md, USER.md, BOOTSTRAP.md, MEMORY.md; 20,000 chars each, 60,000 total (about 15,000 tokens) | `src/agents/workspace.ts:1192-1223`; `src/agents/embedded-agent-helpers/bootstrap.ts:89-94`; re-read per turn with an mtime guard at `src/agents/bootstrap-cache.ts:51-52` |
| `setInterval` sites (non-test) | 79 (gateway 16, agents 10, infra 6, channels 5, extensions 44) | `rg "setInterval\(" src/gateway src/channels src/cron src/infra src/agents src/auto-reply extensions` |
| `setTimeout` sites (non-test) | 708 | same dirs |

Gateway intervals while idle: 30 s maintenance tick and 60 s health refresh (`src/gateway/server-maintenance.ts:183,200`), 25 s WebSocket keepalive (`src/gateway/websocket-keepalive.ts:16`), 15 s SSE heartbeat (`src/gateway/sessions-history-http.ts:443`), six separate 60 s reconcilers (`src/gateway/server-lifetime-sidecars.ts:35,62`; `src/gateway/server-worker-placement-startup.ts:676`; `src/gateway/github-oauth-lifecycle.ts:621`; `src/gateway/auth-rate-limit.ts:201`; `src/gateway/worker-environments/service.ts:528`), a 5 s terminal reassert (`src/gateway/terminal/output-flow-control.ts:135`), and a 25 ms desktop resume poll (`src/gateway/desktop/observe-bridge.ts:278`). Per active run: two 60 s heartbeats (`src/agents/embedded-agent-runner/run/tool-activity-heartbeat.ts:21`; `src/agents/embedded-agent-runner/run/attempt-session-prepare.ts:199`). Hot polls: a 1 ms stream pump (`src/agents/tool-search-code-mode-child.ts:198`) and 25 ms polls (`src/agents/model-provider-auth.ts:615`; `src/agents/prepared-model-catalog-worker.ts:214`). Discord adds a 250 ms ready poll; voice adds 20 ms timers.

So a turn starts with something like 10,000 to 25,000 tokens of scaffolding before conversation history (not measured on a live box), after a process that loaded 3,500 modules, against a database with 162 core tables, with about 20 hook checkpoints, on a host running 16 background intervals.

---

## 5. Complexity metrics

Fifteen largest non-test source files (TypeScript and JavaScript; the Swift and Kotlin apps are larger still, topping out at `apps/shared/OpenClawKit/Sources/OpenClawProtocol/GatewayModels.swift` at 26,220 lines):

| File | Lines |
|---|---|
| `ui/src/i18n/locales/en.ts` | 6,840 |
| `extensions/qa-lab/src/providers/mock-openai/server.ts` | 2,986 |
| `extensions/codex/src/app-server/native-subagent-monitor.ts` | 2,291 |
| `src/plugins/management-service.ts` | 2,222 |
| `src/agents/auth-profiles/store.ts` | 2,197 |
| `src/gateway/operator-approval-store.ts` | 2,125 |
| `src/agents/cli-runner/prepare.ts` | 2,089 |
| `extensions/feishu/src/channel.ts` | 2,068 |
| `extensions/voice-call/src/webhook/realtime-handler.ts` | 2,034 |
| `src/tui/tui.ts` | 2,020 |
| `src/gateway/managed-image-attachments.ts` | 2,009 |
| `src/auto-reply/reply/export-html/template.js` | 2,009 |
| `ui/src/pages/cron/view.ts` | 1,989 |
| `src/infra/update-managed-service-handoff.ts` | 1,984 |
| `packages/tool-call-repair/src/stream-normalizer.ts` | 1,979 |

| Metric | Value |
|---|---|
| `src/` non-test `.ts` | 8,901 files, 1,940,534 lines |
| `src/` test `.ts` | 6,794 files, 2,918,389 lines |
| Test files, whole repo | 12,633 files, 5,275,282 lines |
| `src/gateway/` non-test | 1,148 files, 290,364 lines |
| `src/agents/` non-test | 1,673 files, 403,652 lines |
| Top-level config keys | 43 (`src/config/zod-schema.root-shape.ts:45`); the TS type has 44 (`src/config/types.openclaw.ts:82`) |
| Doctor files | 254 files, 70,979 lines |
| Update handoff file alone | `src/infra/update-managed-service-handoff.ts`, 1,984 lines |

Note the update machinery has a single 1,984-line file for handing an update from one process to another, which is longer than most complete agent harnesses.

---

## 6. Ten rules a replacement must follow

1. Ship one file. OpenClaw's `ERR_MODULE_NOT_FOUND` came from a lazily imported hashed chunk that the package swap deleted (`src/commands/doctor.ts:158`; `src/cli/update-cli/update-command.ts:635`).
2. Never run the update from inside the thing being updated. The 8.1 updater ran Doctor in-process after the swap (`f346846aa85^:src/cli/update-cli/update-command-service.ts:1078-1086`) and 8.2 had to move it to a fresh process to stop the crash.
3. Install into a new versioned directory, flip a symlink, and keep the old directory until the next boot. The project maintains a hand-written list of "stable filenames so rebuilt dist/ trees do not strand already-running gateways" (`tsdown.config.ts:378-379`), which is what you need when you overwrite in place.
4. One runtime, one absolute path, used by both the service and the CLI. The Node-skew logic (`src/cli/update-cli/update-command.ts:393-402`; `src/cli/update-cli/update-command-service-plan.ts:140-148`) exists only because the installer's nodesource Node (`scripts/install.sh:2343`) and the user's nvm Node differ.
5. Host hygiene checks warn; they never block, and every block has an override. The permission check refuses with no flag to bypass (`src/daemon/systemd-definition-mutation.ts:151-152`; `src/cli/daemon-cli/install.ts:170-180`) and does not say which of four directories it means.
6. Know your own install path from your own binary; never ask the package manager who owns you. The owner check shells out to whatever `npm` is on PATH (`src/infra/update-global.ts:1271`) and refused a working install (`src/cli/update-cli/shared.ts:392-396`).
7. Never let an update add a new refusal that the previous version would not have raised without a migration note and an escape hatch. The owner refusal landed Aug 31 (`38ecc4173b2`) and the old code defaulted to npm (`f346846aa85^:src/cli/update-cli/shared.ts:384`), so the first update installed the rule that refused the second.
8. One liveness clock per turn, owned by the turn, set at the model or tool call, and the reaper kills the process group. OpenClaw has at least five timers from 2 min to 48 h (`src/logging/diagnostic.ts:84`; `src/logging/diagnostic-stuck-session-recovery.runtime.ts:37`; `src/logging/diagnostic-run-activity-snapshot.ts:99,102`; `src/agents/timeout.ts:13`) and abort only detaches a tool instead of killing it (`src/agents/agent-tools.abort.ts:20-27`).
9. Every subprocess gets a default timeout and its own process group. `runCommandWithTimeout` has no deadline unless the caller passes one (`src/process/exec-runner.ts:108-109`).
10. Queues that carry user messages live on disk and survive restart; queues in memory are for nothing the user would miss. The follow-up queue is process memory (`src/auto-reply/reply/queue/state.ts:63`), in-flight runs are aborted on restart (`src/gateway/server-close.ts:401-403`), and drops are only logged (`src/auto-reply/reply/followup-delivery.ts:339-352`); copy the outbound delivery queue instead, which is persisted and replayed (`src/infra/outbound/delivery-queue-storage.ts:1-2`).

And one rule about size, because it is the cause behind the causes: keep the operations code smaller than the agent loop and change it rarely. OpenClaw's ratio is 4.7 to 1 the wrong way, 538 commits touched update, daemon, and doctor code in August, and update/install/doctor was 29 percent of 8.2's fixes.
