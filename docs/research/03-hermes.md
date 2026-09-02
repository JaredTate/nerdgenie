# Hermes Agent, re-read on 2026-09-02

Repo: `/Users/jt/Code/hermes-agent`, HEAD `95f62ca3bf` (v2026.8.31 + 636 commits, pyproject 0.21.0). Read-only study. Every code claim carries a `file:line`. The August report (HARNESS.md Part 2.4 and Part 5) already covers the context ladder, micro-compaction, the verify-on-stop gate, the review fork, and progressive tool disclosure. This report covers what changed since, and the questions JT asked: why the interface is glitchy, how Signal and cron work, what the operations code teaches, and how local models are handled.

## 1. What changed since 2026-08-06

Headline: 6,440 commits in 28 days (230 per day, peak 410 on Aug 15). 870 author names; Teknium alone is 30.6%. Fix commits 3,680 (57%) versus feat 719 (11%), a 5:1 ratio. Seven tagged releases in four weeks. Tracked files grew from 8,529 to 11,239. Python under the core dirs grew 31% (592k to 775k lines).

| Area | Commits | % | fix | feat |
|---|---|---|---|---|
| Desktop app (Electron, Bot Mode, i18n, sdk) | 1,657 | 25.7 | 1,001 | 248 |
| Tests, docs, chore, fmt | 681 | 10.6 | 55 | 9 |
| Install, update, config, plugins, state, auth | 590 | 9.2 | 429 | 75 |
| Gateway core (sessions, lease, watchdog, serve, api) | 507 | 7.9 | 398 | 26 |
| Agent loop (compression, context, delegation, deadline, verify) | 430 | 6.7 | 314 | 37 |
| Tools (browser, computer-use, mcp, terminal) | 319 | 5.0 | 200 | 61 |
| TUI and CLI (tui, tui_gateway, ui-tui, cli, host, supervisor) | 315 | 4.9 | 232 | 39 |
| Chat platforms and relay (telegram, slack, discord, wecom...) | 283 | 4.4 | 185 | 30 |
| Providers and models | 221 | 3.4 | 144 | 36 |
| Cron | 209 | 3.2 | 140 | 21 |
| Memory and skills | 172 | 2.7 | 100 | 33 |
| Web dashboard (web, web_server, kanban) | 138 | 2.1 | 100 | 16 |
| Unscoped, merges, long tail | 918 | 14.3 | 382 | 88 |

By my own file-level count (`git log --since=2026-08-06 -- <path>`): `apps/desktop` 1,793 commits, `tui_gateway/` 299, `gateway/run.py` 268, `cron/` 200, `plugins/platforms` 182, `tui_gateway/server.py` 173, `cli.py` 129, `hermes_cli/web_server.py` 127, `agent/conversation_loop.py` 108, `ui-tui/` 80, `web/` 53. Signal files: 3 commits.

The ten most important changes:

| # | Change | Evidence | Why it matters |
|---|---|---|---|
| 1 | Bot Mode and Group Chats in the desktop app | `366d8814b8` (Aug 16), `e7433910e9` (Aug 30, +15,230), `cbc67b939f` adds `gateway/hosted_rooms.py` + 8 modules | Headline of v0.21.0; `plugin.js` fixed 140 times in 17 days then rewritten twice |
| 2 | Managed llama.cpp local runtime | `43e67d872f` (Sep 1, +16,471), new `hermes_cli/local_runtime/` | Local inference becomes a supervised one-click provider |
| 3 | Clean-room replacement of Anthropic office skills | `51570f4da7` (Aug 8, -50,429 lines) | License-risk fix, largest diff in the window |
| 4 | Core-tool deferral: 19 tools default, rest behind tool_search | `e16ad33a9d` (Aug 29); schemas 13.4K to 6.9K tokens | Halves per-call fixed token cost |
| 5 | Liveness watchdogs for silent hangs | `8a3b6f374d` startup; `0fe7abe37a` turn liveness; `083f8a6071` unified deadline | A gateway sat deadlocked ~30h with a live PID |
| 6 | One shared SessionDB writer | `db339f0051` (Sep 1), `hermes_state_registry.py` | 12 writers raced WAL checkpoints; 11+ incidents; `hermes_state.py` fixed 143 times |
| 7 | Agent drives the in-app browser | `5df1d0e113`, `c57581cd0d`; `f780cb36d8` removes `cua_browser_*` | Old separate-Chromium path deleted |
| 8 | Cron: notepad, monitors, incidents, fenced fire claims | `04e8a661f2`, `6dff2109aa`, `9de5460c12`, `acaafcc6bb`; `scheduler.py` 4,419 to 8,585 lines | Stateless one-shots became durable, dedup'd, race-safe jobs |
| 9 | Subagent steering, `/review`, `/plan`, `/btw`, `hermes pause` | `2a26693e22`, `12395e57b4`, `0f3fcacd3f`, `74a95a3ddf`, `5db1b72b1f` | Changes the command surface on every platform |
| 10 | Context accounting anchored on provider `usage.prompt_tokens` | `d3a1c46510` (Aug 28); compression scope 106 commits | Removes the chars/4 error behind many overflow bugs |

Churn: `gateway/run.py` grew 27,087 to 34,561 lines in 28 days; `electron/main.ts` 12,038 to 17,963. The DCP context engine was added and reverted the same day (`d7072ab914` / `206f74baac`). 179 commits are "salvage" cherry-picks from stale PRs. Nothing big was split: `agent/conversation_loop.py` went 7,334 to 9,244 lines, `cli.py` 18,555 to 22,411. New top-level file `SOUL.md` (Aug 30, `c49fa88b80`).

## 2. Sizes

| Path | LOC | Note |
|---|---|---|
| `apps/` | 521,898 | Electron desktop + Tauri installer + shared; includes i18n and tests |
| `agent/` | 163,566 | `agent/*.py` alone is 141,012 |
| `tools/` | 152,003 | |
| `gateway/` | 131,692 | |
| `ui-tui/` | 97,135 | 64,512 in `src/` (with tests), 31,266 in the forked Ink |
| `web/` | 54,872 | Vite + React dashboard |
| `tui_gateway/` | 41,033 | Python JSON-RPC host for the TUI |
| `cli.py` | 22,411 | prompt_toolkit REPL, the default interface |
| `cron/` | 17,898 | |
| `providers/` | 638 | Base class only; real providers are 39 plugin dirs |

Ten largest source files: `gateway/run.py` 34,561; `cli.py` 22,411; `hermes_cli/web_server.py` 20,250; `tui_gateway/server.py` 18,495; `apps/desktop/electron/main.ts` 17,963; `hermes_state.py` 17,220; `hermes_cli/main.py` 15,182; `hermes_cli/kanban_db.py` 12,153; `agent/auxiliary_client.py` 11,839; `plugins/platforms/telegram/adapter.py` 11,358.

`run_conversation()` starts at `agent/conversation_loop.py:1996` and no other top-level `def` or `class` follows it, so it runs to end of file at line 9,244. About 7,250 lines in one function, up from about 6,100 in August. `gateway/run.py` has one class, `GatewayRunner`, at `gateway/run.py:7433` with 412 methods.

## 3. Interface stack

### 3.1 The surfaces

| Surface | Tech | Entry | Size |
|---|---|---|---|
| Classic CLI (default) | Python prompt_toolkit REPL | `cli.py:5185` `class HermesCLI` | 22,411 lines |
| TUI (`hermes --tui`) | TypeScript, React 19 on a forked Ink | `ui-tui/src/entry.tsx` spawns `python -m tui_gateway.entry` | 64k TS + 31k forked Ink + 41k Python host |
| Web dashboard | Vite, React, Tailwind, xterm.js | `hermes_cli/web_server.py` (FastAPI/uvicorn) serves `web/` | 20k Python + 55k TS |
| Desktop | Electron; launches `hermes serve` and talks WebSocket | `apps/desktop/electron/main.ts` | 18k main.ts, 141 electron modules |
| Messaging gateway | Python asyncio, 20 platform adapters | `gateway/run.py` | 34.5k lines |

The default is the classic CLI: `hermes_cli/config_defaults.py:1466` sets `"interface": "cli"`. The TUI is opt-in via `--tui` or `display.interface: tui` (`hermes_cli/main.py:362`). Both are maintained; `main.py:378` calls the CLI "the earlier cousin" of `entry.tsx`.

### 3.2 Process topology

TUI: `node entry.js` (screen) <-> newline-delimited JSON-RPC over stdin/stdout <-> `python -m tui_gateway.entry` (sessions, tools, model calls). `ui-tui/README.md:3` states the split: "TypeScript owns the screen. Python owns sessions, tools, model calls, and most command logic." Optionally a third process: with `dashboard.turn_isolation` on, agent turns move to a `python -m tui_gateway.compute_host` child "so compute-heavy agent threads do not contend with the serving process' event loop for the same GIL" (`tui_gateway/host_supervisor.py:1-7`). Default off (`hermes_cli/config_defaults.py:1719`).

Web chat: browser xterm.js <-> WebSocket `/api/pty` <-> uvicorn `web_server.py` <-> a POSIX PTY running `hermes --tui` (Node) <-> stdio <-> `tui_gateway.entry` (Python). The Python side then opens a second WebSocket back to the dashboard (`/api/pub`) so tool events can reach the browser sidebar via `/api/events`. `tui_gateway/event_publisher.py:3-8`: events "fire on *that* gateway's transport, three processes removed from the dashboard server itself." That is four processes and three transports for one chat message. `hermes_cli/web_server.py:16235-16240` and `web/src/pages/ChatPage.tsx:7-12` document the same chain.

Desktop: Electron main spawns `hermes serve` (`apps/desktop/electron/backend-command.ts:3-21`) and the renderer opens `ws://127.0.0.1:<port>/api/ws` (`main.ts:12375`). `tui_gateway/ws.py:1-12` reuses the same `dispatch()` as stdio, so the desktop runs the same 18k-line Python server over WebSocket instead of stdio. The messaging gateway is yet another process (`hermes gateway run`), coordinated through a control socket (`gateway/control_socket.py:2`).

### 3.3 Why it feels glitchy: root causes

1. **Screen and state live in different processes joined by a pipe, and the pipe is the failure domain.** Every token, tool event, and status update becomes a JSON line, is parsed in Node, becomes React state, and is diffed to ANSI by Ink. The server has 109 `_emit(` sites and per-token `reasoning.delta` events (`tui_gateway/server.py:8419`). If a stdout write fails the gateway exits (`tui_gateway/entry.py:454-470`, `:502-512`). A dedicated crash log exists because this happens (`server.py:72`).

2. **A stdin hack that should not need to exist.** Tool subprocesses inherit fd 0. If a child sets `O_NONBLOCK`, the flag lands on the shared open file description; the gateway's next `readline()` returns `''` and it dies as if the TUI closed (`tui_gateway/_stdin_recovery.py:1-9`). The fix checks the flag (`:104-112`), calls `os.set_blocking(0, True)` (`:130`), wraps fd 0 in `socket.fromfd` to clear `SO_RCVTIMEO` (`:136-146`), and gives up after 10 recoveries a minute (`:45`, `:118-125`). The right fix is never sharing fd 0 with children.

3. **Crash budgets everywhere, which means crashes are expected.** The TUI respawns a dead gateway at most 3 times per 60s (`ui-tui/src/app/gatewayRecovery.ts:7-8`). The compute host respawns 3 times per 5 minutes with `min(5, 0.25 * 2^n)` backoff (`host_supervisor.py:147`, `:582-591`) and waits 10s for hello (`:427`). The client's ready timer attaches a stderr tail so users can tell "wrong python" from "missing dep" (`ui-tui/src/gatewayClient.ts:356-372`).

4. **Reconnect replay is bounded and restart-sensitive.** WebSocket clients get a 512-event ring per session (`tui_gateway/event_replay.py:35-37`); "a long turn emits ~hundreds of token events; this covers several minutes" (`:33-34`). Sequence counters reset on restart, so an epoch was added because clients otherwise "believe they missed nothing" (`:24-31`). An event-replay PR was reverted in the window (#96118).

5. **Terminal emulation in a browser cannot reconnect cleanly.** `web_server.py:17718-17721`: "A fresh xterm cannot reliably reconstruct the TUI from an arbitrary bounded tail of alternate-screen, differential ANSI output." The sidecar back-channel is "best-effort" and "silent" on failure, dropping when its 256-item queue fills (`event_publisher.py:15-19`, `:30`).

6. **The GIL forced a second scheduler.** Rather than keep the UI out of the model process, a child process was added, plus a route table classifying 13 RPCs as `turn-path`, `run-concurrent`, or `idle-gated` (`host_supervisor.py:31-44`). Two schedulers over one conversation is where stale-state bugs come from: "race", "stale", and "orphan" appear 125, 125, and 155 times in `tui_gateway/*.py`; "GIL" 22 times.

7. **A forked terminal framework.** `8760faf991` (2026-04-11) "feat: fork ink and make it work nicely" created a 31,266-line private Ink (`ui-tui/packages/hermes-ink`). Its recent commits are per-terminal patches: Ghostty kitty protocol, Terminal.app dim state, SGR 22 reset, Vietnamese Telex IME, Alt+Enter. The code carries `forceRedraw` (`ui-tui/src/types/hermes-ink.d.ts:144`), a `flickers` diagnostic (`:55`), a resize coalescer named for the "flickering remount storm" (`ui-tui/src/lib/resizeCoalescer.ts:14`), an FPS overlay (`ui-tui/src/config/env.ts:73-75`), and `evictInkCaches` + `forceRedraw` on session resume (`ui-tui/src/app/sessionResumeView.ts:2-7`).

8. **Threads and locks in an 18k-line server.** `tui_gateway/server.py` spawns threads at 21 sites, holds 31 locks, and runs RPCs on a `ThreadPoolExecutor` (`:409`). It was fixed 118 times in 28 days. Python renders some content (`tui_gateway/render.py:1-5`) and TypeScript renders the rest (`markdown.tsx`), so two renderers can disagree.

9. **Launching a chat can turn into a JS build.** `hermes --tui` checks whether `dist/entry.js` is stale (`hermes_cli/main.py:2438-2445`); the normal flow is "npm install if needed, always esbuild, then node dist/entry.js" (`:2666`, `:2716`).

10. **Surface count.** Two terminal UIs, a 17-route web dashboard (`web/src/App.tsx:139-217`), an Electron app with its own ownership, release-gate, bundle-skew, crash-forensics and quit-guard modules (`apps/desktop/electron/`), and 20 messaging platforms. Interface-scoped fix commits since Aug 6: 1,188.

## 4. Signal

| Aspect | Hermes | Citation |
|---|---|---|
| Transport | signal-cli in HTTP daemon mode; Hermes never spawns it | `gateway/platforms/signal.py:1-12`, default `http://127.0.0.1:8080` at `:289` |
| Receive | SSE `GET /api/v1/events?account=` with `timeout=None` via httpx | `signal.py:456-465`, parse at `:488-500` |
| Send | JSON-RPC 2.0 `POST /api/v1/rpc`: `send`, `sendTyping`, `sendReaction`, `getAttachment` | `:987-1000`, `:1206`, `:1282`, `:913` |
| Linking | Manual `signal-cli link -n "HermesAgent"`; setup wizard only prints the command | `hermes_cli/gateway.py:7864-7865` |
| Allowlist | Env vars `SIGNAL_ALLOWED_USERS`, `SIGNAL_GROUP_ALLOWED_USERS`; unknown sender gets a pairing code when no allowlist, silent drop otherwise | `gateway/authz_mixin.py:1075-1085` |
| Groups | `chat_id = "group:<id>"`; mention gate via `SIGNAL_REQUIRE_MENTION` | `signal.py:641`, `:649-664` |
| Attachments | `getAttachment` base64, 100 MB cap, ffmpeg remux of AAC voice notes; outbound passes a local path, so daemon and Hermes share a filesystem | `:912-931`, `:66`, `:936-946`, `:1526-1532` |
| Rate limit | Global token bucket for attachments: 50 tokens, 1 per 4s, delay not drop, 2 attempts | `signal_rate_limit.py:34-36`, `:247-296` |
| Formatting | Markdown to `textStyles` (BOLD, ITALIC, STRIKETHROUGH, MONOSPACE), UTF-16 offsets; 8,000-char split | `signal_format.py:13-25`, `signal.py:67`, `:1108-1160` |
| Reliability | SSE reconnect 2s to 60s; health probe every 30s, forced reconnect after 120s silence; echo filter via outbound timestamp LRU | `:70-73`, `:505-515`, `:523-558`, `:336-348` |
| Streaming | None. `SUPPORTS_MESSAGE_EDITING = False`; one send at the end | `:282`, `gateway/run.py:30560-30576` |
| Receipts | No read or delivery receipts sent or consumed | receipt envelopes return early at `:619-620` |

Compared with OpenClaw (`/Users/jt/Code/openclaw/extensions/signal`): both drive signal-cli's HTTP daemon with JSON-RPC sends and an SSE event stream (`src/client.ts:206-212`, `:305-306`). The difference is process ownership: OpenClaw can spawn and supervise `signal-cli daemon --http` itself (`src/daemon.ts:175-206`, `src/monitor.ts:488-542`), or use an external daemon, or the bbernhard REST container; Hermes always expects you to run the daemon.

## 5. Cron

Job model: `create_job()` at `cron/jobs.py:2332-2354` takes prompt, schedule, repeat, deliver, origin, skills, model/provider/base_url overrides, script, `no_agent`, `monitor_script`/`monitor_url`, `reasoning_effort`, and `failure_deliver`. `parse_schedule()` (`jobs.py:962`) yields `once`, `interval`, or `cron` from "30m", "every monday 9am", 5-field cron, or an ISO timestamp; croniter is optional (`:992`).

Persistence: `~/.hermes/cron/jobs.json` with advisory file locking (`jobs.py:4`, `:22`, `:85`); output to `~/.hermes/cron/output/{job_id}/{timestamp}.md` (`:5`). A SQLite `executions.db` records every attempt with `job_id, process_id, pid, process_started_at, status IN (claimed, running, completed, failed, unknown)` (`cron/executions.py:46-59`); interrupted attempts become `unknown` "only after their exact owner process is proved gone" (`:3-5`).

Scheduler: `tick()` (`cron/scheduler.py:8137`) runs every 60s from a gateway thread under `.tick.lock` (`:1-9`); a provider interface lets a plugin replace the trigger (`cron/scheduler_provider.py:1-19`). Overlap: an in-flight registry (`:869`) skips a running job; `sweep_stale_inflight` (`:1054`) force-releases old claims because a job that never released "was skipping every fire with 'already running'" (`:1204`). Missed runs: a catch-up window up to 2 hours, then fast-forward (`jobs.py:1199-1233`). Timeouts: `HERMES_CRON_TIMEOUT` 600s inactivity (`scheduler.py:1410-1426`), scripts 3,600s (`:4121`). `hermes pause` writes `$HERMES_HOME/ESTOP` and the tick skips dispatch (`agent/estop.py:1-6`).

Delivery and failure: `_deliver_result` (`scheduler.py:3166`) routes to `origin`, `local`, or `platform:chat_id`. `_preflight_check_delivery` (`:5451`) validates platform credentials before scheduling. `failure_streak` is persisted per job (`jobs.py:3221-3225`) and a nudge is appended to failure notices (`scheduler.py:153`). Failures are grouped into incidents keyed by `(job_id, error signature)` so an acked failure does not re-ping until the error text changes (`cron/incidents.py:1-20`). The HEAD commit (`95f62ca3bf`) makes preflight also validate the `failure_deliver` lane, because "a typo'd failure platform would otherwise only surface when a failure occurs, exactly when the notice must not be lost," and normalizes it in the dashboard update path so an unnormalized value cannot reach `jobs.json`. The bug class: two write paths (create and dashboard-update) with different validation.

Other pieces: a per-job notepad capped at 16 KB per value and 64 KB per job because it is prompt-injected every run (`cron/notepad.py:8-13`); monitor mode hashes a script's output and skips the LLM when unchanged (`cron/monitor.py:1-17`); `lifecycle_guard.py:1-14` rejects jobs whose prompt contains `hermes gateway restart` after one put a gateway in "a SIGTERM-respawn loop every ~10 seconds"; blueprints and suggestions are a form layer over the same `create_job` (`cron/blueprint_catalog.py:1-20`).

## 6. Memory and context: what is new

- **Verify-on-stop is now off by default.** `hermes_cli/config_defaults.py:255-267`: "the verification narrative proved more noise than signal"; migrations v31/v32 switched existing installs off. `max_verify_nudges` is 3 (`:254`). The August report called this a distinctive edge; the project has retreated from it.
- **Micro-compaction is still off by default** (`config_defaults.py:927`; `docs/micro-compaction.md:17`, `:173-177`). Defrag threshold 2,000 tokens (`:946`).
- **Context size now anchors on provider-reported `usage.prompt_tokens`** and only estimates the last turn (`d3a1c46510`). This replaced chars/4 across the whole history.
- **Memory files.** `MEMORY.md` (agent notes) and `USER.md` (user facts) under the memory dir, loaded and sanitized into a snapshot at session start (`tools/memory_tool.py:6-8`, `:247-258`). `SOUL.md` is loaded from `HERMES_HOME` as identity (`agent/prompt_builder.py:2215-2248`); a top-level `SOUL.md` template landed Aug 30 (`c49fa88b80`). `MemoryManager` allows only one external memory plugin at a time to avoid schema bloat (`agent/memory_manager.py:1-9`).
- **Review fork.** Still a daemon thread replaying the conversation in a forked `AIAgent` with a whitelist of memory and skill tools, inheriting the parent's prefix cache (`agent/background_review.py:1-16`, `:140`); on by default (`:296`); a separate model via `auxiliary.background_review.{provider,model}` (`:196-220`). New: skip the fork when the provider cannot emit tool calls (`37fd61d13b`), read-before-write of skills (`1ee30352ca`), and `agent/review_idle_queue.py:1-6`, which defers the review so it does not "monopolize the GPU the user's next prompt" needs on a local runtime. The prompt asks for "CLASS-LEVEL skills" not "one-session-one-skill entries" (`:481-483`).
- **Skills.** 13 bundled categories in `skills/`; a skill is a directory with `SKILL.md` and YAML frontmatter (`agent/skill_utils.py:147`, `:175`). Aug 30 moved 15 skills to optional and cut the index 26% (`c49fa88b80`).
- **Session recall.** FTS5 over all messages (`hermes_state.py:5-11`), exposed via `tools/session_search_tool.py`.
- **New guards in `agent/`:** `deadline.py:1-6` (one primitive replacing "at least six site-local deadline mechanisms"), `turn_liveness.py:1-6` (abort turns that stall between tool_calls and execution), `empty_response_guard.py:1-6` (retry budget after a user was "charged ~$2.33" for empty completions), `repetition_guard.py:1-6`, `native_compaction.py:1-6`, `prompt_cache_boundary.py:1-6`, `transcript_repair.py:1-4` (extracted "to keep the godfiles narrow"), `side_question.py:1-5` (`/btw`).

## 7. Local models

Providers are declarative profiles: `providers/base.py:1-9` says a `ProviderProfile` "declares everything about an inference provider in one place" and does not own clients or streaming. There are 39 profiles under `plugins/model-providers/`. LM Studio is first-class (`provider: lmstudio`, default `http://127.0.0.1:1234/v1`); Ollama, vLLM, and llama.cpp are aliases for `custom` with a `base_url` (`cli-config.yaml.example:70-76`).

The `custom` profile handles Ollama quirks: `ollama_num_ctx` maps to `extra_body.options.num_ctx`, and `think=False` is sent only when the URL looks like Ollama (port 11434 or hostname `ollama`), because strict hosts reject unknown fields (`plugins/model-providers/custom/__init__.py:6-10`, `:22-40`). LM Studio's per-model `capabilities.reasoning.allowed_options` is used to clamp reasoning effort so the server does not 400 (`agent/lmstudio_reasoning.py:1-8`).

Context length: a 3s cached probe (`agent/model_metadata.py:2361-2395`) asks Ollama `/api/show` and prefers Modelfile `num_ctx` over the GGUF training max, which "would... silently truncate" (`:2420-2450`), then LM Studio's native `/api/v1/models` `max_context_length` (`:2448`). Cached lengths are reconciled against the live server because "vLLM/Ollama operators can restart with a new `--max-model-len` / `num_ctx`" (`:943-975`).

Tool calling: Hermes relies on the server's native `tool_calls`. I found no runtime parser that executes `<tool_call>` text on OpenAI-compatible endpoints. `agent/agent_runtime_helpers.py:140-165` builds a `<tool_call>` prompt, but for trajectory export; `:994-1010` strips stray `<tool_call>` and Gemma-style blocks from content ("Ported from openclaw/openclaw#67318"). Only the ACP bridge parses text tool calls (`agent/acp_openai_bridge.py:211`). `supports_tools` (`agent/models_dev.py:851`) feeds the picker, not the loop (not verified further). Inline `<think>` blocks are extracted when no structured `reasoning_content` arrives (`agent/chat_completion_helpers.py:2330-2377`).

New Sep 1: a managed llama.cpp runtime with hardware detection, GGUF catalog, and a supervisor (`hermes_cli/local_runtime/`, `43e67d872f`).

## 8. Operations lessons

| File | Problem it solves | Rule for the new agent |
|---|---|---|
| `gateway/restart_loop_guard.py:14-26`, `:47-60` | Boot, auto-resume the session that killed you, die again. Persisted counter (3 restarts, 300s gap) skips the resume, not the service; fails open | A replay after restart needs a persisted loop counter that disables the replay, never the service |
| `gateway/startup_watchdog.py:3-4`, `hermes_startup_watchdog.py:11-25`, `:97-103` | Deadlock before the event loop exists; 30h in `futex_wait` with a live PID. Stdlib-only thread, `os._exit(75)` after 300s unless progress leases extend it | Arm the first watchdog before importing anything heavy; keep its fire path import-free |
| `gateway/shutdown_watchdog.py:3-21`, `:46-57` | Frozen asyncio loop mid-drain; no asyncio recovery can run. OS thread dumps stacks and hard-exits after 3 missed probes | Never run recovery on the thing being recovered |
| `gateway/session_stall.py:5-8`, `:27-60` | Queued message, no progress. Notify once at 300s from the one shared progress clock | One progress clock, notify once, unknown means still stalled |
| `gateway/turn_lease.py:3-17`, `:62-75` | Busy guards keyed by routing key, transcripts by session id; interleaved turns wedge `user;user` alternation. Lease per resolved session id, 5s wait, reject on timeout | Lock on the resolved owner; reject on timeout rather than run unlocked |
| `gateway/delivery_ledger.py:3-5`, `:27-32`, `:63-66` | A generated reply not yet confirmed sent is lost without a trace. Row before send; `pending -> attempting -> delivered/failed`; dead-owner rows resent with a "may be a duplicate" marker | Persist the obligation before the send and label possible duplicates |
| `gateway/session_db_recovery.py:57`, `:117-128` | A failed SQLite open (NFS locking) silently disabled `/resume`. Single-flight reopen, 1s to 60s backoff, health in status | Retry dependency opens with capped backoff; never cache a failure as permanent |
| `gateway/lifecycle_ledger.py:6-27` | No record of SIGKILL or OOM. `running` sentinel at boot, `exited` on every clean path; `running` at next boot means unclean death | Sentinels let the next boot tell a crash from a stop |
| `gateway/drain_control.py:5-6`, `:28-62`, `:86` | No control channel into a running gateway; a file marker gates drain; orphan markers parked gateways for 52 minutes and 3 days. Marker now carries a boot-id epoch and 1h expiry | A file that gates service must carry identity and expiry |
| `gateway/scale_to_zero.py:10-27`, `:72` | Fly judged idle by inbound connections and suspended mid-job. Gateway owns the idle decision (2 min quiet), quiesces, then suspends itself | When the platform cannot see your work, own the idle decision |
| `gateway/memory_monitor.py:7-14` | RSS logger every 300s. Not wired: no caller outside tests; the config key it documents does not exist | A monitor nobody starts is documentation; check the call site |
| `agent/deadline.py:1-6` | Six site-local timeout mechanisms drifted apart (#85125) | One deadline primitive from day one |
| `cron/lifecycle_guard.py:1-14` | A cron job that restarts the gateway creates a 10-second SIGTERM loop | The agent must not be able to schedule its own restart |
| `hermes_state_registry.py` (`db339f0051`) | 12 call sites each opened a SQLite writer; WAL checkpoints raced; 11+ incidents | One writer per database file |

## 9. Steal this

| Idea | Why | Where |
|---|---|---|
| Delivery ledger with duplicate marker | The reply is the one thing you cannot lose silently | `gateway/delivery_ledger.py:3-5`, `:27-32` |
| Lifecycle sentinel (running/exited) | Cheapest possible crash detector | `gateway/lifecycle_ledger.py:14-27` |
| Startup watchdog on an OS thread, stdlib only | Catches hangs that asyncio can never see | `hermes_startup_watchdog.py:11-25` |
| Turn lease keyed on resolved session id, fail-closed | Prevents interleaved transcripts | `gateway/turn_lease.py:17`, `:34-37` |
| Restart-loop guard that fails open | Breaks resume storms without taking the service down | `gateway/restart_loop_guard.py:23-31` |
| Cron executions ledger with owner pid + start time | Distinguishes "crashed" from "still running" without heuristics | `cron/executions.py:46-59` |
| Cron incidents keyed by error signature | One ping per distinct failure, not per run | `cron/incidents.py:1-20` |
| Cron notepad with hard size caps | Jobs carry state across runs without bloating prompts | `cron/notepad.py:8-13` |
| Monitor-mode hash suppression | Cheap poll first, LLM only on change | `cron/monitor.py:1-17` |
| `failure_deliver` lane validated at preflight | Failure notices must never be the thing that fails | `95f62ca3bf`, `cron/scheduler.py:5451` |
| Two memory files, MEMORY.md and USER.md, plus SOUL.md identity | Small, inspectable, git-able | `tools/memory_tool.py:6-8`, `agent/prompt_builder.py:2215-2248` |
| Review fork with a tool whitelist and idle deferral on local GPUs | Learning without touching the live context or blocking the user | `agent/background_review.py:1-16`, `agent/review_idle_queue.py:1-6` |
| Prefer Ollama `num_ctx` over GGUF training max; reconcile cache against live server | Avoids silent truncation on local models | `agent/model_metadata.py:2420-2450`, `:943-975` |
| Send `think=false` only to Ollama-shaped URLs | Strict OpenAI-compatible hosts reject unknown fields | `plugins/model-providers/custom/__init__.py:22-40` |
| Signal SSE health probe: 30s check, 120s silence forces reconnect; echo filter by outbound timestamp | Signal-cli SSE streams die quietly | `gateway/platforms/signal.py:72-73`, `:523-558`, `:336-348` |
| Context anchored on provider `usage.prompt_tokens` | The only accurate token count is the provider's | `d3a1c46510` |

## 10. Avoid this

| Pattern | Evidence | Consequence |
|---|---|---|
| Screen in one process, state in another, joined by stdio | `ui-tui/README.md:3`; `entry.py:487-512`; `_stdin_recovery.py:1-9` | Pipe faults become UI deaths; fd-sharing hacks; crash budgets |
| Forking a terminal UI framework | 31,266-line Ink fork, `8760faf991` | You now maintain per-terminal quirks forever |
| Terminal-in-browser for web chat | `web_server.py:16235-16240`, `:17718-17721` | Four processes per message; reconnect cannot redraw |
| A child process to escape the GIL, plus a route table | `host_supervisor.py:1-7`, `:31-44` | Two schedulers; 125 "race" and 155 "orphan" mentions |
| Giant files and one giant function | `run_conversation` 1996 to 9,244; `gateway/run.py` 34,561 lines, 412-method class; `cli.py` 22,411; `server.py` 18,495; `main.ts` 17,963 | 5:1 fix-to-feature ratio; the same files fixed 100+ times in 28 days |
| Two terminal UIs plus web plus desktop plus 20 platforms | `config_defaults.py:1466`, `main.py:378` | 1,188 interface fixes in 28 days; desktop is 26% of all commits |
| Verify-on-stop as a nag loop | `config_defaults.py:255-267` | Turned off by default after it "proved more noise than signal" |
| Micro-compaction that breaks the prompt cache | `config_defaults.py:927`, `docs/micro-compaction.md:17` | Still opt-in a year in |
| Config split across env vars and YAML | `SIGNAL_ALLOWED_USERS`, `HERMES_CRON_TIMEOUT`, `HERMES_SCALE_TO_ZERO` | Users cannot find or audit settings |
| 39 provider profiles | `plugins/model-providers/` | JT needs two: one OpenAI-compatible (LM Studio/Ollama) and one Anthropic |
| Cron as jobs.json + tick lock + in-flight registry + stale sweeps + three SQLite files | `cron/` 17,898 lines | One SQLite table and one in-process loop covers a single-user Pi |
| Multiple write paths with separate validation | HEAD commit `95f62ca3bf` | Every field needs one normalizer called from every path |

Not verified: whether `supports_tools` from models.dev gates anything in the runtime loop; the exact media cache path for Signal attachments; whether `TYPING_INTERVAL = 8.0` or the "~2s" comment wins in `signal.py:69` versus `:1251`.
