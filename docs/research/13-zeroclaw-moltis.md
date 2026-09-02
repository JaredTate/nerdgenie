# 13 — ZeroClaw vs Moltis as a base for HARNESS v2

**Date:** 2026-09-02. **Method:** `git clone --depth 1` of both repos into the scratchpad (`ext/zeroclaw` @ `b7bc2b2`, 2026-09-02; `ext/moltis` @ `f640c9b`, 2026-09-02). tokei, grep, and file reads only. Nothing was built or run. GitHub numbers come from the public REST API today. File paths below are relative to each clone root. "Not verified" marks anything I could not confirm from source.

## 0. Short answer

Neither project is "lightweight" in fact. ZeroClaw is ~775k lines of Rust (about 394k outside tests) in 22 library crates, with a 43k-line config file and a 36k-line channel orchestrator. Moltis is ~407k lines in 75 workspace crates, pins a nightly toolchain, and ships 63 MB tarballs. Both are seven months old and moving fast.

Recommendation: **build from scratch, and borrow four or five small, well-tested modules from ZeroClaw** (Signal channel, cron scheduler, tool-call parser, self-update, Landlock/Seatbelt wrappers), plus **one pattern from Moltis** (CDP browser with a persistent Chrome profile). Do not use either as-is. Do not fork either. Details and file paths in section 5.

## 1. Size and shape

| Metric | ZeroClaw | Moltis |
|---|---|---|
| Rust files / lines / code (tokei, excl. target, vendor, node_modules) | 1,088 / 884,588 / **774,627** | 1,204 / 464,721 / **406,823** |
| Non-test Rust (approx: lines before `#[cfg(test)]` and outside `tests/`) | **~394k** (57% of lines are tests) | not computed precisely; tests are often split into sibling `tests.rs` files |
| `#[test]` / `#[tokio::test]` functions | 16,670 | 7,754 |
| Cargo manifests / workspace members | 41 manifests: 22 library crates in `crates/`, root binary, 3 apps (TUI `zerocode` 69k LOC, `zerorelay`, Tauri desktop), `xtask` (14k), 5 firmware crates, test fixtures | 80 manifests, 75 workspace members (`Cargo.toml` members list) |
| Largest crates (LOC) | runtime 256k, channels 165k, config 84k, tools 77k, providers 75k, gateway 40k, memory 27k | gateway 73k, tools 57k, providers 36k, chat 23k, agents 22k, httpd 21k, config 20k |
| Direct deps: root crate / unique across workspace | 30 external in root `[dependencies]`; **140** unique across all manifests | n/a root; **175** unique across all manifests; 244 `[workspace.dependencies]` |
| Cargo.lock packages | **1,274** | **1,376** |
| Edition / MSRV | 2024 / `rust-version = "1.96.0"` (`Cargo.toml:7,10`); no toolchain pin | 2024 / `rust-version = "1.91"` (`Cargo.toml:191,194`) but `rust-toolchain.toml` pins **`nightly-2026-06-20`** (needed for rustfmt `imports_granularity` and `clippy -Z unstable-options`, `rustfmt.toml:5-6`, `justfile:36`; no `#![feature]` in crates, so a stable build is plausible — not verified) |
| Release profile | `opt-level="z"`, fat LTO, `codegen-units=1`, strip, `panic="abort"` (`Cargo.toml [profile.release]`) | not checked |
| Latest release / assets | v0.8.4, 2026-08-02. Compressed tarballs: aarch64-linux-gnu **27.4 MB**, aarch64-linux-musl 27.0 MB, x86_64-linux-gnu 29.0 MB, aarch64-darwin 24.9 MB; also .deb, .msi, .dmg, AppImage (94 MB), SBOMs | 20260902.01 (today), 101 assets. aarch64-darwin tar.gz **63.0 MB**; aarch64 .rpm 49 MB; Arch pkg 63 MB; plus GPG + Sigstore sigs |
| Size claims in README | None in MB. "a single Rust binary" (`README.md:30`). Release profile comment mentions Pi 3 with 1 GB RAM | "One binary — sandboxed, secure, yours" (`README.md:7`); `--features lightweight` suggested for Raspberry Pi (`README.md:105`) |
| Targets | aarch64/x86_64 darwin; aarch64 linux gnu **and musl**; arm + armv7 gnueabihf; x86_64 linux gnu/musl; x86_64 windows; android experimental (`.github/workflows/release-stable-manual.yml`) | aarch64/x86_64 darwin; aarch64/x86_64 linux **gnu only**; no musl, no Windows (`.github/workflows/release.yml`) |

Top 10 largest Rust files, ZeroClaw:

| Lines | File |
|---|---|
| 43,402 | `crates/zeroclaw-config/src/schema.rs` |
| 35,762 | `crates/zeroclaw-channels/src/orchestrator/mod.rs` |
| 17,898 | `crates/zeroclaw-runtime/src/agent/loop_.rs` |
| 16,212 | `crates/zeroclaw-runtime/src/sop/engine.rs` |
| 14,085 | `apps/zerocode/src/chat.rs` |
| 12,849 | `crates/zeroclaw-runtime/src/rpc/dispatch.rs` |
| 12,465 | `src/main.rs` |
| 12,378 | `crates/zeroclaw-channels/src/telegram.rs` |
| 12,276 | `crates/zeroclaw-runtime/src/agent/agent.rs` |
| 10,631 | `crates/zeroclaw-providers/src/reliable.rs` |

Moltis caps files at **1,500 lines** with a CI script (`scripts/check-file-size.sh:7`); its ten largest files are all 1,476–1,500 lines (`crates/httpd/src/server/runtime.rs`, `crates/chat/src/run_with_tools.rs`, `crates/tools/src/sandbox/file_system.rs`, `crates/gateway/src/pairing.rs`, `crates/web/src/api.rs`, …). That is a real readability advantage.

## 2. ZeroClaw deep dive

### 2.1 Agent loop

- **Entry points.** `run()` (interactive CLI) at `crates/zeroclaw-runtime/src/agent/loop_.rs:1192`; `process_message()` (one inbound channel message) at `loop_.rs:2844`; `agent_turn()` at `loop_.rs:789`. All feed the turn engine `run_tool_call_loop()` at `crates/zeroclaw-runtime/src/agent/turn/mod.rs:381`.
- **Per-turn steps.** Build system prompt (`agent/system_prompt.rs`, `build_system_prompt_for_turn` `loop_.rs:604`), inject memory (`agent/memory_inject.rs`), inject skills, call provider (`turn/provider_call.rs`), parse response (`turn/parse_response.rs`), gate on approval (`turn/approval_gate.rs`), execute tools (`agent/tool_execution.rs`), append history, repeat.
- **Budget.** `for iteration in 0..max_iterations` at `turn/mod.rs:569`. `DEFAULT_MAX_TOOL_ITERATIONS = 10` (`turn/mod.rs:97`), config default also 10 (`crates/zeroclaw-config/src/schema.rs:3557`). When exhausted, `finish_after_max_iterations` (`turn/max_iter.rs`) produces a bounded final answer.
- **Tool execution.** Parallel by default with `futures_util::future::join_all` (`agent/tool_execution.rs:489-515`). `should_execute_tools_in_parallel` (`:460`) forces serial if any tool needs approval or the batch contains `tool_search`.
- **Retries and repair.** Malformed tool-protocol output from the model is fed back as a correction up to `MAX_MALFORMED_TOOL_PROTOCOL_RETRIES = 2` (`turn/mod.rs:93`, feedback at `:1014-1060`). Provider retries live in `zeroclaw-providers/src/reliable.rs` (section 2.5). A loop detector hashes recent calls (window 20, 3 repeats) and injects a nudge (`agent/loop_detector.rs:11-26`).
- **Context management.** No LLM summarisation in the loop. `history_trim.rs:1-2`: "keep the most recent whole turns that fit the token budget, drop the rest, never cut a turn in half." On a context-window error the loop trims and retries once (`loop_.rs:2629-2676`, comment: "No summarization, no splicing"). `DEFAULT_MAX_HISTORY_MESSAGES = 50` (`agent/history.rs:12`). `history_pruner.rs` removes orphaned tool messages. Long-term compression happens off-loop in memory consolidation (2.4).
- **Streaming.** Provider SSE is consumed in `turn/stream_consume.rs` (1,318 lines) and guarded by `turn/stream_guard.rs`; README says failed streams fall back to non-streaming.
- **Tool receipts.** Each tool result gets an HMAC-SHA256 receipt with a per-session key so the model cannot fake a tool result (`agent/tool_receipts.rs:1-60`).

Verdict: the design is sound and split across ~25 files under `agent/turn/`, but `loop_.rs` (17.9k lines) and `agent.rs` (12.3k) are still god files.

### 2.2 Channel trait and Signal

- **Trait.** `pub trait Channel: Send + Sync + Attributable` at `crates/zeroclaw-api/src/channel.rs:619`. Methods: `name`, `send`, `send_final`, `listen(tx: mpsc::Sender<ChannelMessage>)`, `health_check`, `send_choice`, `start_typing`, `stop_typing`, `add_reaction`, `remove_reaction`, `request_approval`. Simple and worth copying.
- **Transport.** `crates/zeroclaw-channels/src/signal.rs` (2,135 lines, tests from `:953`). It does not link libsignal. It talks to the **signal-cli HTTP daemon** (`signal-cli daemon --http 127.0.0.1:8686`, `docs/book/src/channels/signal.md`). Outbound is JSON-RPC 2.0 `POST {http_url}/api/v1/rpc` with a 30 s timeout (`rpc_request`, `signal.rs:335-380`; `send` uses method `"send"` at `:584`). Inbound is SSE `GET /api/v1/events?account=…` (`listen`, `:637-760`).
- **Reconnect.** Exponential backoff 2 s → 60 s on connect error or non-2xx (`:647-686`); chunk errors break to reconnect (`:707`).
- **Allowlist.** `is_sender_allowed` (`:204`) uses shared `allowlist.rs:14` — exact E.164/UUID match or `"*"`. Peers are resolved from canonical config each message (`peer_resolver`, `:48-50`).
- **Groups.** `group_ids` filter and `dm_only` flag (`:301-320`); replies to groups use a `group:<id>` target prefix (`:17`, `:322-331`).
- **Attachments.** Not downloaded. Attachment-only messages are dropped when `ignore_attachments` is set (`:411-420`); every emitted `ChannelMessage` has `attachments: vec![]` (`:506`). Open issue #7891 "Add Signal media attachment support".
- **Typing.** `sendTyping` RPC (`:850-863`); stop is a no-op since signal-cli auto-expires (`:865`).
- **Health.** `GET /api/v1/check` (`:836`).
- **Extras.** Native polls for choice prompts (`send_poll` `:536`), reactions via an LRU of recent message targets, approval prompts over Signal with a 300 s timeout.
- **Availability.** Feature `channel-signal` is **not** in `default` or in the prebuilt "dist" feature list (`Cargo.toml:199-204`, `:224-231`). You must build from source with `--features channel-signal`.

### 2.3 Cron and SOP scheduler

- **Job model.** `CronJob` at `crates/zeroclaw-runtime/src/cron/types.rs:150`: `Schedule::{Cron{expr,tz}, At, Every}` (`:101`), `JobType::{Shell, Agent}` (`:28`), `SessionTarget::{Isolated, Main}` (`:60`), `DeliveryConfig{mode, channel, to, thread_id, best_effort}` (`:116`), `allowed_tools`, `uses_memory`, `delete_after_run`, `next_run/last_run/last_status/last_output`.
- **Persistence.** SQLite via rusqlite at `<data_dir>/cron/jobs.db` (`cron/store.rs:1592`), tables `cron_jobs` and `cron_runs` (`:1727-1760`), output capped at 16 KB (`:12`). Declarative jobs from config are synced (`sync_declarative_jobs`).
- **Scheduler.** `scheduler::run` (`cron/scheduler.rs:272`) ticks on a tokio interval (min 5 s, `:22`; `MissedTickBehavior::Skip` `:279`). At startup it clears stale locks (`:335`) and either skips or catches up missed runs (`:415`, `:457`). Jobs are claimed with `UPDATE cron_jobs SET locked_at=… WHERE locked_at IS NULL` (`store.rs:817`), so two processes cannot double-run.
- **Failure handling.** `execute_job_with_retry` (`scheduler.rs:597-650`): `scheduler_retries` attempts, exponential backoff from `provider_backoff_ms` (min 200 ms) doubling to 30 s with jitter; policy violations are not retried. Shell jobs time out at 120 s (`:23`).
- **Delivery.** `deliver_and_classify_run_result` (`:130`) sends output to a channel; `best_effort` decides whether delivery failure marks the run failed. Signal is a valid delivery target (`cron/mod.rs:31-46`). Agent jobs cannot call the `cron_*` tools (`:25-31`), a nice self-modification guard.
- **SOP.** Separate 30.6k-line "Standard Operating Procedure" engine (`crates/zeroclaw-runtime/src/sop/`, `Sop` at `sop/types.rs:458`): step graphs with triggers (cron, channel, filesystem, AMQP), quorum approvals, deterministic steps, and agent-proposed procedures (`sop/procedural_memory.rs:31`). Powerful, but far beyond JT's needs.

### 2.4 Memory and skills

- **What is stored.** `MemoryEntry{key, content, category, session_id, score, namespace, importance, superseded…}` (`crates/zeroclaw-api/src/memory_traits.rs:18-32`); categories `Core | Daily | Conversation | Custom` (`:114-123`). Trait `Memory` at `:245` (store/recall/get/list/forget/purge, all agent-scoped).
- **Where.** Default SQLite `brain.db` under `<workspace>/memory/` (`crates/zeroclaw-memory/src/sqlite.rs:83`), table `memories` with an `embedding BLOB` plus an FTS5 BM25 index (`:234-256`). Other backends: markdown, Qdrant, Postgres (feature), Lucid (being retired, issue #9644), none.
- **Embeddings.** OpenAI-compatible HTTP only (`memory/src/embeddings.rs:68-120`) with a `custom:<url>` option (`:274`), so LM Studio or Ollama embeddings should work through their OpenAI-style endpoints (not verified).
- **Recall.** One pipeline in `agent/memory_inject.rs:1-45`: recall → decay or rerank → relevance filter → skip set → budget (5 recalled, 4 rendered, 800 chars each, 4,000 total) → wrapped in `[Memory context]`.
- **Consolidation.** LLM-driven: `memory/src/consolidation.rs:1-30` extracts `history_entry`, `memory_update`, `facts`, `trend`; helpers for dedup, conflict, decay, hygiene.
- **Role memory.** `agent/system_prompt.rs:27-43` injects workspace files `AGENTS.md`, `SOUL.md`, `TOOLS.md`, `IDENTITY.md`, `USER.md`, `BOOTSTRAP.md`, `MEMORY.md` with a per-file char cap. That is exactly the "durable role" JT wants, and it is a 100-line idea.
- **Skills.** `~/.zeroclaw/workspace/skills/<name>/SKILL.md` (`runtime/src/skills/mod.rs:58`). Loader accepts `SKILL.toml`, `manifest.toml`, or agentskills.io-style `SKILL.md` with YAML frontmatter (`:831-834`). `[[tools]]` entries become shell tools (`tools/skill_tool.rs`). Registries are git-cloned and synced weekly/daily (`skills/mod.rs:44-52`).
- **Does the agent write its own skills?** Yes, optionally. `SkillCreator::create_from_execution` (`skills/creator.rs:52`) turns a successful multi-step tool trace into a `SKILL.md` via a reflection prompt (`:12-19`), deduped by embedding cosine similarity. **Off by default** (`schema.rs:6391 enabled: false`, `max_skills: 500`). `SkillImprover` (`skills/improver.rs:11`) rewrites skills with cooldowns. This is the best "learnable skills" implementation I saw in either project.

### 2.5 Provider layer

- **Trait.** `ModelProvider` in `crates/zeroclaw-api/src/model_provider.rs`: capabilities, `convert_tools`, `simple_chat`, `chat_with_system`, `chat_with_history`, `list_models`, `stream_chat*`, `default_wire_api` ("responses" vs "chat_completions").
- **Implementations.** Anthropic (`providers/src/anthropic.rs`, 7.3k lines, SSE parser at `:1943`, streaming at `:2492`); OpenAI; generic OpenAI-compatible (`compatible.rs:26-29`, used for ~20 vendors, 300 s idle stream timeout `:24`, native tools `:3472`, streaming `:3485`); **Ollama** native `/api/chat` (`ollama.rs`: default `http://localhost:11434` `:19`, `num_ctx` 8192 `:25`, `num_predict` 2048 `:30`, 600 s timeout `:16`, `<think>` stripping `:354`); **LM Studio** as a compatible provider fixed at `http://localhost:1234/v1` (`factory.rs:33`, `:1601`). Plus Bedrock, Gemini, OpenRouter, Azure, Copilot, Codex OAuth, xAI, and more.
- **Fallback chains.** `ReliableModelProvider` (`reliable.rs:1649`) with a per-model `model_fallbacks` map (`:1659`, `:1707-1716`). Retries/backoff and API-key rotation on 429 come from `ReliabilityConfig` (`schema.rs:13304-13329`). Non-retryable errors are classified (`:460`), `retry-after` is parsed (`:647`), and streams can resume on the next candidate without replaying the failed chunk (`:194`). Task-locals record which candidate actually served the call (`:47-55`). This file is 10.6k lines; the concept is good but the implementation is heavy.
- **Tool-call parsing and repair for local models.** A standalone crate, `crates/zeroclaw-tool-call-parser/src/lib.rs` (5.5k lines, one file). It recognises six envelope shapes (`:19-26`), unwraps nested JSON-in-string arguments (`:27-35`, `:99`), extracts XML-tag calls (`:858`), detects malformed or incomplete protocol (`:647`, `:762-790`), strips `<eom>` markers (`:46`) and think tags. For non-native models the runtime injects a text tool protocol (`loop_.rs:1010`, `:1071`) and retries twice on malformed output (2.1). This is the single most reusable file in the repo.

### 2.6 Web dashboard

- **Stack.** axum server in `crates/zeroclaw-gateway` (routes at `src/lib.rs:1600-1700`). Chat over **WebSocket** with approval frames (`src/ws.rs:1-40`); event feed over **SSE** with a replay ring buffer (`src/sse.rs:1-25`); also JSON-RPC (`runtime/src/rpc/dispatch.rs`, 12.8k lines), OpenAPI, A2A, ACP.
- **Frontend.** React 19 + Vite + Tailwind + CodeMirror SPA (`web/package.json`), 33k lines TSX + 22k TS. Served from `web/dist` at runtime, or embedded with `include_dir` under the `embedded-web` feature (`gateway/src/static_files.rs:15-19`). Whether prebuilt tarballs embed it is not verified.
- **Pages.** Dashboard, AgentChat, ChatWorkspace, AgentsList, Cron, Skills, Sops, SopCanvas, Runs, RunDetail, Tools, Config, Logs, Doctor, Pairing, Canvas, Integrations, AcpConsole, quickstart (`web/src/pages/`).
- **Auth.** `PairingGuard` (`crates/zeroclaw-config/src/pairing.rs`): a one-time pairing code with a TTL, SHA-256-hashed bearer tokens persisted across restarts, and per-client brute-force lockout. `/api/*` requires the bearer (`gateway/src/lib.rs:1412`). Optional WebAuthn passkeys (feature). `/health` is public.
- **Tailscale.** `crates/zeroclaw-runtime/src/tunnel/tailscale.rs:30-40` shells out to `tailscale serve` (tailnet) or `tailscale funnel` (public). Config `tunnel_provider = "tailscale"` (`schema.rs:13911-13981`).

### 2.7 Security

- **Autonomy.** `ReadOnly | Supervised (default) | Full` (`crates/zeroclaw-config/src/autonomy.rs:19-27`).
- **Approval gates.** `runtime/src/approval/mod.rs:1-40`: `Yes | No | Always | ReplaceWith`, session allowlists, audit. Channels can carry approvals (Signal `:895`), and the WebSocket UI does too. Shell commands go through an allowlist and risk gate in `config/src/policy.rs` (which even detects unsafe output redirects, `:1272`).
- **Sandboxes.** `runtime/src/security/mod.rs:3-33` wires Landlock (`landlock.rs:1-31`, pure-Rust crate, workspace + rw/ro/wo extra roots), macOS Seatbelt via `/usr/bin/sandbox-exec` (`seatbelt.rs:7`), Bubblewrap, Firejail, Docker; `detect::create_sandbox` picks one. Caveat: `sandbox-landlock` and `sandbox-bubblewrap` are **not** in `default` or the dist list; only the `ci-all` meta-feature enables them (`Cargo.toml:376`). Prebuilt Linux binaries therefore likely run without Landlock (not verified beyond Cargo metadata).
- **Receipts.** HMAC tool receipts (2.1).
- **Secrets.** ChaCha20-Poly1305 with a 0600 key file at `~/.zeroclaw/.secret_key`; config stores only ciphertext (`config/src/secrets.rs:1-21`); optional 1Password backend (`:41`).
- **Network policy.** No global egress policy found. Per-tool domain allowlists plus an SSRF guard for private hosts (`schema.rs:7809-7842` for browser, `:7894` for `http_request`, `tools/src/helpers/domain_guard.rs`).
- **Other.** Prompt-injection guard, untrusted-content framing, leak detector, emergency stop, signed audit log, WebAuthn (`security/` dir, 15.7k lines).

### 2.8 Browser and desktop automation

- `crates/zeroclaw-tools/src/browser.rs` is a facade over three backends (`:56-70`, `resolve_backend` `:359-431`): (a) `agent_browser` — shells out to the **npm `agent-browser` CLI** (`:256-260`, `:500-554`), so Node is required; (b) `rust_native` — **fantoccini WebDriver**, feature `browser-native`, needs a running chromedriver at `native_webdriver_url` (`schema.rs:7825`); (c) `computer_use` — an HTTP sidecar at `127.0.0.1:8787/v1/actions` (`:46`) that is not in this repo (not verified).
- Persistent logged-in profile: only `browser_delegate.rs` passes `CHROME_USER_DATA_DIR` to an external CLI (`:47-49`). Nothing drives a real Chrome profile natively.
- Desktop: `screenshot.rs` wraps `screencapture`/`scrot` (`:15-17`); AppleScript exists only inside the Tauri desktop app (`apps/tauri/src/capabilities/applescript.rs`). No xdotool, enigo, or CGEvent in core.

### 2.9 Update, supervision, health

- **Self-update.** `src/commands/update.rs`: fetch GitHub release (`:119`), download and verify against `SHA256SUMS` (`:157`, `:430`), copy current binary to `.bak` (`:247`), swap, smoke-test, roll back on failure (`:275`, `:313`). Checksums only; no signature check in the updater (release page publishes SBOMs and a "verification archive", not verified). The dashboard can trigger an upgrade and the process re-spawns itself when unsupervised (`runtime/src/restart.rs:1-16`).
- **Supervision.** `zeroclaw service install` registers systemd / launchd / Windows Service / OpenRC (`README.md:69`; `runtime/src/service/mod.rs:158`, `:332`).
- **Health.** `GET /health` and `/api/health` (`gateway/src/lib.rs:1613`, `:1851`) backed by a component registry with restart counts (`runtime/src/health/mod.rs:9-24`). Channels restart with backoff (`schema.rs:13318-13323`); WebSocket channels have a stall watchdog (`infra/src/stall_watchdog.rs`); a heartbeat engine runs `HEARTBEAT.md` tasks; `zeroclaw doctor` exists.

### 2.10 Code quality

- **Structure.** 22 crates, but `zeroclaw-runtime` (256k lines) is a kitchen sink: agent, cron, SOP, RPC, security, skills, tunnels, calendar, subagents, hardware. Five files over 12k lines.
- **Errors.** `anyhow` everywhere (488 files vs 21 with `thiserror`). `AGENTS.md` bans `unwrap()` in production paths; I count ~153 non-test `unwrap()` calls (approximate). Good discipline.
- **Async.** tokio multi-thread, `async-trait`, reqwest with rustls.
- **Unsafe.** 243 `unsafe` blocks/fns outside firmware; 166 are `std::env::set_var` in tests (edition 2024 makes that unsafe). About 74 remain in non-test code (config env handling, audit, browser, image tools).
- **Licensing.** `MIT OR Apache-2.0` (`Cargo.toml:8`). `cargo-deny` allowlist has no GPL/AGPL (`deny.toml:28-45`). No `presage` or `libsignal` in `Cargo.lock`; signal-cli (GPL-3) is a separate process over HTTP, so it is not linked. Heavy deps: wasmtime (plugins), tauri (desktop app only), lettre, ring, rusqlite bundled.
- **LLM maintainability.** `AGENTS.md` (66 lines) is strict and useful ("single source of truth", no snapshots of live config). But every user-facing string must go through Fluent i18n keys (`AGENTS.md` "User-Facing Text"), which adds friction to each small change.
- **Tests.** 16,670 test functions; CI runs fmt, clippy `-D warnings`, cargo-deny, cargo-audit, SBOM, Trivy.

### 2.11 Velocity and issue health

- Repo created 2026-02-13. **373 commits in the last 30 days**. 404 contributors; top contributor has 914 commits, so no single owner.
- Releases: 0.8.0 (06-12), 0.8.1 (06-19), 0.8.2 (06-26), 0.8.3 (07-16), 0.8.4 (08-02): a 1–3 week cadence, but nothing in the last month.
- **506 open issues, 308 open PRs**, 178 open issues labelled bug.
- Notable open items: #7462 "74 test failures on Windows" (19 comments); #9328 verifiable-intent skips credential-chain verification; #9348 WhatsApp Web answers every DM in business mode; memory architecture is being redesigned in three RFCs (#6850, #9103, #9048) and the Lucid backend is scheduled for removal at 0.9.0 (#9644). Signal: #7891 attachments, #9774 `sourceUuid`-only senders silently dropped, #9158 note-to-self. No open "hang" issues; one "stuck" (Discord typing after reload, #9198).

## 3. Moltis (lighter pass)

- **Facts.** 2,841 stars, MIT, created 2026-01-29. 124 commits in 30 days; 62 open issues, 21 open PRs; 64 contributors. **One person (penso) has 3,600 commits; the next has 61**, and "claude" is the fourth-largest committer. `AGENTS.md:8` describes it as the "Rust version of openclaw". Releases are date-versioned and near-daily.
- **Crate map** (`crates/`): gateway 73k (HTTP/WS/RPC/auth), tools 57k (sandboxes: docker, Apple container, Firecracker, cgroup, wasm, Daytona, Vercel — `tools/src/sandbox/mod.rs:5-22`), providers 36k (anthropic, openai, openai_compat, ollama, **embedded llama.cpp** under `local_gguf`/`local_llm`), chat 23k, agents 22k, httpd 21k, config 20k, cli 11k; channels telegram 10k, matrix 8.6k, slack 8.6k, whatsapp 6.6k, nostr 6.4k, discord 5.9k, msteams 5k, **signal 1.7k**; skills 8.3k; memory 6.8k + memory-zvec 5.9k; browser 7.5k; voice 7.6k; cron 4.7k; auth 3.5k; tailscale 0.7k; plus caldav, telephony, home-assistant, connectors (gmail, himalaya), importers (openclaw, claude, codex, hermes), swift-bridge + iOS/macOS apps (15k Swift).

| Feature | Moltis evidence |
|---|---|
| Agent loop | `crates/agents/src/runner/{streaming,non_streaming}.rs`; parallel tools via `join_all` (`streaming.rs:1026`); `AgentLoopLimits{max_iterations}` (`runner/helpers.rs:74`), config `agent_max_iterations` (template example 25, `config/src/template.rs:405`); context-window overflow triggers LLM compaction and retry (`chat/src/run_with_tools.rs:1059-1079`, `agent_loop.rs:743`) |
| Signal | Same signal-cli HTTP daemon design (`crates/signal/src/lib.rs:1`; `/api/v1/check|events|rpc` at `client.rs:61,84,117`); allowlist + group policy (`config.rs:30-39`); SSE backoff 2→60 s (`plugin.rs:282-346`); typing (`outbound.rs:176`); attachments not ingested (`inbound.rs:175-176`) |
| Cron | `crates/cron`: `CronSchedule`, `CronPayload`, `SessionTarget` (`types.rs:19-65`); JSON file, memory, or sqlx SQLite stores (`lib.rs:2-14`); delivery + rate limits (`service.rs`); heartbeat wake (`heartbeat.rs`) |
| Memory | Markdown files → chunks → embeddings → SQLite hybrid FTS + vector (`memory/src/lib.rs:1`, `manager.rs:24-31`); local embeddings via llama-cpp-2 FFI (`lib.rs:10-12`); optional zvec store |
| Skills | agentskills.io `SKILL.md` (`skills/src/lib.rs:3-4`), ClawHub install, bundled skills; no auto-creation from traces found (not verified) |
| Providers | Anthropic, OpenAI, compat, Ollama, embedded GGUF; LM Studio catalog entry (`providers/src/model_catalogs.rs:335-337`) |
| Web UI | askama server pages (login, onboarding, share) + **Preact/Vite/Tailwind** SPA (`crates/web/ui/package.json`), 42k TSX; WebSocket protocol with handshake, scopes, roles (`httpd/src/ws.rs`); GraphQL |
| Auth | SQLite credential store: passwords, passkeys, API keys, sessions (`crates/auth/src/lib.rs:3-7`); loopback-locality detection feeds auth decisions (`locality.rs:1-5`) |
| Tailscale | `crates/tailscale` shells out to `tailscale serve|funnel` (`manager.rs:72-82`); NetBird too |
| Sandbox | Container-first (Docker, Apple container, Firecracker). No Landlock, bwrap, or Seatbelt |
| Browser | **chromiumoxide CDP**, managed Chrome with `persist_profile: true` by default and `profile_dir` (`browser/src/types.rs:470-552`); host, sandboxed, or sidecar launch (`pool.rs:374-392`); Chrome/Chromium/Brave/Edge detection |
| Desktop automation | None |
| Email | Gmail REST connector and Himalaya CLI wrapper; README calls the agent tools read-only (`README.md:131`); no `lettre`; no contacts DB |
| Self-update | `gateway/src/updater.rs`: install-method detection, SHA-256 + **GPG signature** check (`:345-350`) |
| Service | launchd / systemd-user / portable supervisor (`cli/src/service_commands.rs:3-5`) |
| Code style | `thiserror` per-crate `error.rs` (73 files) plus anyhow (250); 50 `unsafe`; 1,500-line file cap; 7,754 tests |

**Is its size a worse base than ZeroClaw?** Line for line it is smaller and better factored (75 focused crates, a hard file-size cap, typed errors). But it is a worse base for three reasons: (1) bus factor of one; (2) it bundles llama.cpp, wasmtime, Swift bridges, voice, and telephony into a 63 MB artifact; (3) nightly toolchain and gnu-only Linux builds. It is a good place to read a clean CDP browser implementation, and little else for JT's list.

## 4. Fit against JT's requirements

| Requirement | ZeroClaw | Moltis |
|---|---|---|
| Signal | ✅ signal-cli JSON-RPC + SSE, allowlist, groups, typing, reconnect (`channels/src/signal.rs`); ⚠️ no attachments; ⚠️ source build needed (`channel-signal` not in dist) | ✅ same design, smaller (`crates/signal`, 1.7k); ⚠️ no attachments |
| Web UI over Tailscale | ✅ axum + React SPA, WS + SSE, pairing-token auth; `tailscale serve` tunnel (`tunnel/tailscale.rs`) | ✅ Preact SPA, passkeys/passwords, `tailscale serve/funnel` crate |
| Cron / scheduled tasks | ✅ SQLite jobs, locks, retries, delivery to Signal (`runtime/src/cron/`) | ✅ SQLite/JSON stores, delivery, heartbeat (`crates/cron`) |
| Durable role memory | ✅ SOUL/IDENTITY/USER/AGENTS/MEMORY.md injection (`system_prompt.rs:27-43`) + SQLite memory | ✅ markdown memory workspace, per-agent |
| Learnable, remembered skills | ✅ SKILL.md/SKILL.toml + auto-creation from traces (`skills/creator.rs:52`, off by default) + improver | ⚠️ SKILL.md load/install only; no self-authoring found |
| Browser automation with persistent logged-in profile | ⚠️ needs npm `agent-browser` or chromedriver; profile only via env to an external CLI (`browser_delegate.rs:47-49`) | ✅ chromiumoxide CDP with `persist_profile` default (`browser/src/types.rs:470-552`) |
| Desktop automation (last resort) | ⚠️ screenshots only; AppleScript only in the Tauri app | ❌ none |
| Email outreach from contacts DB | ⚠️ SMTP send via lettre in `email_channel.rs:16-19`, IMAP tools; ❌ no contacts DB | ⚠️ Gmail/Himalaya connectors, read-oriented; ❌ no contacts DB |
| LM Studio / Ollama | ✅ Ollama native (`ollama.rs`), LM Studio compat (`factory.rs:1601`) | ✅ Ollama, compat, plus embedded llama.cpp |
| Anthropic | ✅ native, streaming, tools (`anthropic.rs`) | ✅ native |
| Pi 5 arm64 | ✅ aarch64 gnu + musl tarballs (27 MB); ⚠️ dist lacks Signal and Landlock | ⚠️ aarch64 gnu only, 60+ MB; `lightweight` feature hint |
| Single static binary | ⚠️ musl build exists; dashboard embedding not verified for dist; browser needs Node or chromedriver | ⚠️ gnu only; llama.cpp + wasmtime inside |
| Atomic update / rollback | ✅ backup, sha256, smoke test, rollback (`commands/update.rs:245-315`); ❌ no signature check | ✅ sha256 + GPG (`updater.rs:345-350`); rollback not verified |
| Sandboxing | ✅ Landlock, Seatbelt, bwrap, Firejail, Docker (`security/`); ⚠️ not enabled in prebuilt binaries | ⚠️ container-only (Docker/Apple/Firecracker); no kernel sandboxes |
| Self-healing | ✅ health registry, channel backoff, stall watchdog, service install, self-respawn | ✅ service install, portable supervisor; watchdogs not verified |
| Easy for Claude to maintain | ❌ 775k lines, 43k-line schema, 36k-line orchestrator, i18n on every string | ⚠️ 1,500-line cap and typed errors help; 75 crates, nightly, one maintainer hurt |

## 5. Recommendation: build, and borrow

**Do not use either as-is.** ZeroClaw's prebuilt binaries omit Signal and Landlock, so JT would be building from source anyway, then carrying a 775k-line dependency to get maybe 15k lines of value. Moltis has the right browser design but the wrong shape (fat binary, nightly, one maintainer).

**Do not fork either.** Both move too fast to track (373 and 124 commits per month) and too slowly to trust (ZeroClaw has 506 open issues and no release in a month; Moltis has one author). A fork of ZeroClaw would inherit five god files and an i18n requirement JT does not need. ZeroClaw's own memory subsystem is mid-redesign (three open RFCs).

**Build from scratch, borrowing these modules** (all MIT OR Apache-2.0 from ZeroClaw, MIT from Moltis; keep the license headers):

| Module to lift | Path (in clone) | LOC (non-test approx) | Why |
|---|---|---|---|
| Signal channel | `zeroclaw/crates/zeroclaw-channels/src/signal.rs` (first 952 lines; tests after `:953`) plus `allowlist.rs` | ~950 + 40 | Complete signal-cli JSON-RPC/SSE client with allowlist, groups, reconnect, typing, polls, approvals. Replace `zeroclaw_log::record!` with `tracing`, and add attachment download (a `getAttachment` RPC) |
| Channel trait | `zeroclaw/crates/zeroclaw-api/src/channel.rs:619-720` | ~100 | Clean, small trait to implement for Signal and the web UI |
| Cron scheduler | `zeroclaw/crates/zeroclaw-runtime/src/cron/{types,schedule,store,scheduler}.rs` | ~2,500 (of 10,585 with tests) | SQLite job store with claim locks, missed-run policy, retry/backoff, delivery hooks. Drop the SOP and `agent_alias` plumbing |
| Tool-call parser | `zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs` | ~2,000 (of 5,486) | Envelope detection and repair for local models. Already a standalone crate; could be depended on directly |
| Self-update | `zeroclaw/src/commands/update.rs` + `runtime/src/restart.rs` | ~500 | Download, sha256, `.bak`, smoke test, rollback, respawn. Add minisign/Sigstore verification (Moltis `updater.rs:433-484` shows a GPG approach) |
| Sandbox wrappers | `zeroclaw/crates/zeroclaw-runtime/src/security/{landlock,seatbelt,traits,detect}.rs` | ~1,300 (landlock 1,079 incl. tests) | Landlock ruleset for the Pi/Linux box, `sandbox-exec` profile for the Mac, one `Sandbox` trait |
| Role files + memory inject | `zeroclaw/crates/zeroclaw-runtime/src/agent/system_prompt.rs:1-60`, `memory_inject.rs` header | pattern only | Copy the idea: SOUL/IDENTITY/USER/MEMORY.md plus a budgeted `[Memory context]` block |
| Skill creator | `zeroclaw/crates/zeroclaw-runtime/src/skills/{creator,document,frontmatter}.rs` | ~1,500 | Reflection prompt and dedup for self-authored SKILL.md; keep the agentskills.io frontmatter format so skills are portable |
| CDP browser | `moltis/crates/browser/src/{manager,detect,snapshot,types}.rs` | ~3,500 | chromiumoxide with a persistent `--user-data-dir`, DOM snapshot with numbered refs, click/type/scroll/evaluate. Skip `container.rs`, `pool.rs`, `browserless.rs` |
| Tailscale serve | either `zeroclaw/.../tunnel/tailscale.rs` (~150) or `moltis/crates/tailscale` (~660) | small | Shell out to `tailscale serve --https=443 localhost:PORT`; trivial to rewrite |

**What to write fresh:** the agent loop (ZeroClaw's is correct but 30k lines; a 10-iteration loop with parallel `join_all`, trim-on-overflow, and HMAC receipts is ~600 lines), the provider layer (one `Provider` trait, Anthropic native, OpenAI-compatible for LM Studio, Ollama native; `reliable.rs` is 10k lines to express "try the next model"), the web UI (a small axum + SSE page, not a 55k-line React app), the contacts DB and outreach tools (neither project has them), and desktop automation (neither has it).

**Risks to state in HARNESS_V2.md:**

- *Contributor churn.* ZeroClaw: 404 contributors, AI-assisted PRs welcomed, 308 open PRs. Borrowed files may be rewritten upstream within weeks; pin the commit (`b7bc2b2`) and treat lifted code as owned, not synced. Moltis: one maintainer; if penso stops, it stops.
- *Unsafe.* Low in both. ZeroClaw's ~74 non-test unsafe sites are in modules JT would not lift. Moltis's llama-cpp FFI is `unsafe impl Send/Sync` (`memory/src/lib.rs:10-12`) and stays behind.
- *Dependency weight.* 1,274 and 1,376 locked crates. The lifted set pulls only tokio, reqwest (rustls), serde, rusqlite, cron, landlock, chromiumoxide, sha2, and hmac. Watch chromiumoxide (last checked version 0.8 in Moltis; it is a large crate and has been slow to release, not verified).
- *Licensing.* Clean. Both permissive; `cargo-deny` in ZeroClaw allows no GPL (`deny.toml:28-45`); no `presage`/`libsignal` in either lock. signal-cli itself is GPL-3 but runs as a separate process, which is the standard, safe arrangement. Moltis vendors `agent-client-protocol` (upstream Apache-2.0; the vendored manifest's license line was not confirmed).
- *"Lightweight" is marketing.* ZeroClaw's 27 MB compressed arm64 tarball is fair for what it contains, but the source is 775k lines and the runtime crate alone is 256k. Moltis's 63 MB tarball is not lightweight. Neither README states a binary size or RAM figure.
- *Prebuilt gaps.* If JT ever wants to "just install" ZeroClaw on the Pi, note again that Signal, Landlock, and (possibly) the embedded dashboard are not in the tarball (`Cargo.toml:199-204`).

## Appendix: not verified

Whether ZeroClaw dist tarballs embed the dashboard; whether the `computer_use` sidecar is public; whether Moltis builds on stable; Moltis's update rollback; Moltis's non-test `unwrap()` count (2,855 includes sibling `tests.rs` files); the vendored `agent-client-protocol` license line.
