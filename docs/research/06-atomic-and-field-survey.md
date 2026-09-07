# 06 — "Atomic Agent" identified, plus a field survey of lightweight agents, Signal options, and local model tool calling

Research date: **2026-09-02**. Every claim below carries a URL. Where I could not confirm something I write **not verified** instead of guessing.

Short version up front:

1. There are **two** different projects called "Atomic Agent". The one JT is comparing is almost certainly **AtomicBot-ai/atomic-agent** (TypeScript), not the older Python `atomic-agents` framework. His complaint is half right.
2. Someone has already built the thing JT is describing. Two of them, in Rust, with Signal **and** cron **and** a web UI: **ZeroClaw** and **Moltis**. He should read their code before writing his own.
3. Signal has no official bot API. Every option wraps the same underlying thing. The safest architecture is **signal-cli in JSON-RPC daemon mode**, spoken to over HTTP, from whatever language you like.
4. Local tool calling is fine now if you pick the model carefully. Small, recent, tool-trained models beat big general ones. A harness still needs a text-protocol fallback.

---

## Part A — Which "Atomic Agent"?

### A.1 Every candidate found

| Project | URL | Language | Size / traction | Latest release | Activity |
|---|---|---|---|---|---|
| **AtomicBot-ai/atomic-agent** — best match | https://github.com/AtomicBot-ai/atomic-agent | TypeScript | 2.5k stars, 237 forks, 719 commits, 15 open issues | **v0.5.4, 2026-09-01** | Very active. 7 releases between 2026-08-23 and 2026-09-01 |
| Eigenwise/atomic-agents (formerly BrainBlend-AI) | https://github.com/BrainBlend-AI/atomic-agents (redirects to Eigenwise) | Python | 6.2k stars, 536 forks, 4 open issues | **v2.10.2, 2026-08-24** (https://pypi.org/project/atomic-agents/) | Active. Maintainer Kenny Vaneetvelde. MIT |
| AtomicBot-ai/atomic-hermes ("Hermes Agent") | https://github.com/AtomicBot-ai/atomic-hermes | Python + React | 176 stars, 5,715 commits | not verified | Last updated **2026-05-08**. PolyForm Noncommercial |
| bububa/atomic-agents | https://github.com/bububa/atomic-agents | Go | 21 stars, 182 commits | not verified | Go port of the Python framework. MIT |
| gpt-multi-atomic-agents | https://pypi.org/project/gpt-multi-atomic-agents/ | Python | v0.1.6 | not verified | Third-party router built on the Python framework |
| Observable "Atomic Agents" | https://observablehq.com/@gjmcn | JavaScript | — | 2022 | Unrelated agent-based-modelling library |

Neither `atomic-agent` nor `atomicagent` was found on npm or crates.io. AtomicBot installs via `curl -fsSL https://atomicagent.io/install | sh`, so it probably is not published to either registry — **not verified** (npmjs.com returned HTTP 403 to automated fetching).

### A.2 Why AtomicBot-ai/atomic-agent is the match

The decisive clue is JT's own comparison list. The **AtomicBot-ai org** also ships **`atomic-hermes`** — which is "Hermes Agent" — and **`atomicbot`**, described as "The Fastest Way to Run OpenClaw". The org profile reads "Your personal, open-source AI assistant based on OpenClaw" (https://github.com/orgs/AtomicBot-ai/repositories, fetched 2026-09-02). He has been browsing this one ecosystem, and picked several projects out of it.

### A.3 What AtomicBot's atomic-agent actually is

A local-first agent runtime. It "runs the control loop and all state on your machine": browses, edits files, runs approved shell commands, remembers across sessions in SQLite, and calls MCP tools. It constrains completions using **GBNF grammars** into "a JSON array of tool calls". Requires Node >= 25.7. Runs on macOS Apple Silicon, Linux x64/arm64, Windows x64. (README, fetched 2026-09-02)

Its own README labels it: **"Developer preview. APIs, commands, config, and behavior are still moving."**

**Local model support — JT's headline is wrong, but his experience was probably real.** It supports four local paths: a managed llama.cpp (their "TurboQuant" fork), an external `llama-server` via `ATOMIC_AGENT_LLAMA_URL`, and **Ollama and LM Studio as OpenAI-compatible presets with no API key**. Cloud providers: OpenAI, Anthropic, Gemini, OpenRouter, MiniMax.

But the README carries this caveat word for word:

> "Tool calling works over this path, so the agent loop runs normally, but it is only as reliable as the model you picked. **Very small models emit malformed tool calls more often.**"

And that path is **brand new**. Issue #66 "Please add support to other LLM runners" closed **2026-08-07**. Issue #69 "Add built-in presets for common OpenAI-compatible providers" closed **2026-08-10**. Issue #283 (Ollama Cloud `<think>` tag corruption) closed **2026-08-31**. Release v0.5.1 (**2026-08-31**) shipped "think-tag handling for native-tools providers like Ollama Cloud" and "automatic Ollama detection". If JT tried it before late August 2026, LM Studio and Ollama support genuinely was absent or rough.

**Open bugs that support "too broken":** #307 (2026-09-01) TUI dies with V8 heap OOM after ~11 hours; #121 (2026-08-12) TUI heap OOM during 25+ tool-call sessions; #300 (2026-09-01) installer 404s on Intel Macs; #112 (2026-08-11) local llama-server probing not disabled when a cloud provider is active; #103 (2026-08-11) OpenAI SSE tool calls without `index` merged incorrectly; #230 (2026-08-22) Telegram approval keyboard missing buttons.

**Messaging, cron and daemon — this part is a real strength.** Cron: yes, "durable deferred turns, cron schedules, intervals, webhooks, and agent-created reminders". Daemon: yes, a managed chat daemon persists, and `atomic-agent serve` exposes an OpenAI-shaped `/v1/chat/completions`. Messaging: **Telegram only** in the README, with owner pairing and inline approval buttons. **No Signal channel, no web UI.** The atomicagent.io marketing page claims Telegram, Discord, Signal, WhatsApp and Slack, which contradicts the README — treat Signal support as **not verified**.

### A.4 The other reading — the Python framework

Worth flagging, because JT's complaint fits it almost word for word. Eigenwise/atomic-agents calls itself "an **extremely lightweight** and modular framework". It is **a library, not an application**: no messaging, no cron, no daemon, nothing to run. It is built on **Instructor + Pydantic**, so every call demands schema-valid structured output — exactly the thing small local models fail at.

Its Ollama failure history is long and documented — issues #25 (2024-11-08), #32 (2024-11-12), #30 "Instructor does not support multiple tool calls" (2024-11-13), #69, #70 (both Jan 2025) and #102 (2025-04-02), all about Ollama examples not working. All now closed. But **#282** is still open (**2026-08-28**): "OpenAI-compat: AtomicAgent Mode.JSON on factory and AgentConfig, not default tools mode" — the default tools mode still does not suit OpenAI-compatible local endpoints.

Community traction is thin. HN Algolia returns four items total, max 2 points, including a Show HN (2024-07-14) with 1 point and 0 comments (https://hn.algolia.com/api/v1/search?query=atomic-agents). Reddit r/LocalLLaMA discussion: searched, none found — **not verified**.

### A.5 Verdict on JT's complaint

- **"Doesn't work with local models"** — inaccurate for AtomicBot's agent, accurate in spirit. It is the *most* local-first option on his list, but LM Studio/Ollama support landed only 2026-08-07 to 2026-08-31, and the README itself admits small models emit malformed tool calls.
- **"Too broken"** — accurate. Self-declared developer preview, 7 releases in 10 days, two unfixed OOM crashes, a broken Intel Mac installer.
- **"Too lightweight"** — inaccurate for AtomicBot (719 commits, cron, MCP, HTTP server, Telegram). Exactly right for the Python framework, which has no daemon, scheduler, or messaging at all.

**Either way it fails his interface requirement**: Telegram not Signal, and no web UI. `atomic-hermes` in the same org *does* have a 16-platform gateway including Signal plus cron, but it is macOS-only, **PolyForm Noncommercial** licensed, and last touched **2026-05-08**.

---

## Part B — Field survey

### B.1 The headline: two Rust projects already are what JT wants

This is the most important finding in the whole report. JT should not start from zero.

| Project | URL | Language | Size | Signal? | Cron? | Web UI? | Local models? | Stars / license |
|---|---|---|---|---|---|---|---|---|
| **ZeroClaw** | https://github.com/zeroclaw-labs/zeroclaw | Rust (edition 2024) | Release tarballs 24.8–29.2 MB; aarch64 Linux build published | **Yes** | **Yes** | **Yes** | **Yes** | 32,712 stars, MIT OR Apache-2.0, pushed 2026-09-02 |
| **Moltis** | https://github.com/moltis-org/moltis | Rust, ~270K LOC across 59 crates | macOS app bundle ~128 MB, Linux x86_64 tarball ~66 MB | **Yes** | **Yes** | **Yes** | **Yes** | 2,841 stars, MIT, pushed 2026-09-02 |

**ZeroClaw**, v0.8.4 published **2026-08-02**, is a single Rust binary that connects LLM providers to 30+ channels. Providers include Anthropic, OpenAI, **Ollama**, and "any OpenAI-compatible endpoint", with fallback chains and routing. Scheduling comes via "Standard Operating Procedures" with cron and event triggers, and there is a "Gateway + dashboard" web UI for "chat, memory browsing, config editing, cron management, and tool inspection". Security: supervised autonomy by default with approval gates, plus **OS-level sandboxes (Landlock / Bubblewrap / Seatbelt / Docker)** and cryptographic tool receipts. v0.8.4 spans 262 commits from 49 contributors.

**The single most valuable detail for JT** is how ZeroClaw's Signal channel works. From the API docs (https://docs.rs/zeroclawlabs/latest/zeroclaw/channels/signal/struct.SignalChannel.html, crate v0.6.9, 2026-07-07):

> "Connects to a running `signal-cli daemon --http <host:port>`. Listens via SSE at `/api/v1/events` and sends via JSON-RPC at `/api/v1/rpc`."

It does not shell out per message and does not use a Rust Signal library. It talks to one long-lived signal-cli daemon over HTTP. Config fields: `http_url`, `account`, `group_id`, `allowed_from`, `ignore_attachments`, `ignore_stories`. The `Channel` trait requires send, listen, health check, and typing indicators. **That is the architecture to copy.** Note also `allowed_from` — an allowlist is the right default for a personal agent.

**Moltis** is heavier but broader: Telegram, WhatsApp, Signal, Discord, Teams, Slack, Matrix, Nostr, plus a web UI with a WebSocket API and a mobile PWA with push notifications. Cron scheduling plus CalDAV and email. Memory is SQLite-backed vector + full-text search with per-agent workspaces and auto-compaction. Sandboxing via Docker/Podman, Apple Container, and WASM tools. Encryption at rest with XChaCha20-Poly1305 and Argon2id; passkey/WebAuthn auth. Latest release tag **20260902.01, published 2026-09-02**. Homebrew tap and multi-arch Docker (amd64/arm64) available. Background write-up: https://pen.so/2026/02/12/moltis-a-personal-ai-assistant-built-in-rust/

The second detail worth copying is ZeroClaw's provider config. Every model provider is declared under `[providers.models.<name>]` in `~/.zeroclaw/config.toml`, where `<name>` is your own alias. You then write `default_model = "claude"` and `fallback_providers = ["claude", "local"]` (https://github.com/zeroclaw-labs/zeroclaw/blob/master/docs/book/src/providers/configuration.md). Cloud-primary with local failover — or the reverse — is one line of config, not a code path. An Ollama provider is `kind = "ollama"`, `base_url = "http://localhost:11434"`; LM Studio is a custom OpenAI-compatible provider pointed at `http://localhost:1234/v1`. Its onboarding wizard also detects Docker/Podman and rewrites `localhost` to `host.docker.internal`, which is a small touch that removes a very common setup failure.

**IronClaw** (https://github.com/nearai/ironclaw, Rust, 12,603 stars, Apache-2.0 OR MIT, pushed 2026-09-02) is the third to look at. Telegram/Slack/Discord only — **no Signal**. But it has a "Routines Engine" for cron schedules, event triggers and webhooks, a Web Gateway with SSE/WebSocket streaming, and a **WASM sandbox with capability-based permissions** alongside a Docker sandbox. The capability-scoped WASM tool model is the most interesting security idea in the family.

**Caveat on all of these:** "lightweight" is marketing. A widely-cited comparison table (https://github.com/T31K/awesome-openclaw-alternatives) lists ZeroClaw at a 3.4 MB binary and <5 MB RAM. The actual v0.8.4 release assets are 24.8–29.2 MB compressed. Treat those blog and README performance numbers as **not verified**. Moltis at ~270K LOC is not small by any measure.

### B.2 The wider "claw" family

The `openclaw/openclaw` repo was created **2025-11-24** and now has **388,622 stars** (TypeScript, pushed 2026-09-02). Widely-repeated blog claims that it "hit GitHub on 2026-01-30" are wrong about the repo date — that is probably when it went viral. It spawned a large ecosystem within weeks. Verified repos:

| Project | URL | Language | Stars | Last push | License | Notes |
|---|---|---|---|---|---|---|
| nanoclaw | https://github.com/nanocoai/nanoclaw | TypeScript | 30,682 | 2026-09-02 | MIT | Runs agents in containers for isolation. WhatsApp, Telegram, Slack, Discord, Gmail. No Signal listed |
| picoclaw | https://github.com/sipeed/picoclaw | Go | 29,935 | 2026-08-27 | MIT | 19+ channels, 30+ providers incl. Ollama and vLLM, cron tool. **No Signal.** v0.2.9, 2026-05-28 |
| ironclaw | https://github.com/nearai/ironclaw | Rust | 12,603 | 2026-09-02 | Apache-2.0 | "Agent OS focused on privacy, security and extensibility" |
| nullclaw | https://github.com/nullclaw/nullclaw | **Zig** | 8,060 | 2026-07-19 | MIT | Smallest of the family. Claimed 678 KB binary (**not verified**) |
| microclaw | https://github.com/microclaw/microclaw | Rust | 731 | 2026-08-28 | MIT | **Signal included** among 14 chat platforms; Ollama + local servers; scheduled tasks. v0.5.4 |
| librefang | https://github.com/librefang/librefang | Rust | 365 | 2026-09-02 | MIT | "Agent operating system written in Rust" |
| tinyclaw | https://github.com/warengonzaga/tinyclaw | TypeScript | 289 | 2026-08-31 | GPL-3.0 | Small personal companion |
| nanoclaw-py | https://github.com/ApeCodeAI/nanoclaw-py | Python | 188 | 2026-07-28 | MIT | ~500 lines. Good for reading, not running |
| ironclaw (other) | https://github.com/JoasASantos/ironclaw | Rust | 82 | 2026-02-18 | — | Different project, same name |

Name collisions are rampant — there are at least three distinct "ironclaw" projects and two "zeroclaw" repos (`zeroclaw-labs/zeroclaw` with 32.7k stars is the live one; `elev8tion/zeroclaw` also exists). Verify the org before trusting a blog link.

Other Rust single-binary assistants found by GitHub search, smaller but instructive: `PantherApex/Panther` (67 stars, Telegram/Discord daemon, 2026-03-08), `ginkida/rustyhand` (20 stars, "one binary, 37 agents, 26 LLM providers, 37 channels", 2026-08-11), `Mgrsc/zerda` (11 stars, CLI + Telegram + MCP, 2026-07-28), `chinkan/RustFox` (Telegram, sandboxed tools, scheduling, MCP), and `hajekad/signal-bot-crawly` (pure Rust Signal bot with **zero external crates** — a hand-written HTTP client, JSON parser and scheduler; interesting as a reading exercise, not as a dependency).

### B.3 The coding agents

| Project | URL | Language | Size | Stars / license | Steal this | Weakness | Messaging + cron in one binary? |
|---|---|---|---|---|---|---|---|
| **goose** | https://github.com/block/goose | Rust | repo ~945 MB | 53,838, Apache-2.0, pushed 2026-09-02 | **The recipe + scheduler pair.** A recipe is a YAML file bundling instructions, required extensions, parameters and prompt. The recipe decides which tools load, not the model. Then `goose schedule add --schedule-id daily-report --cron "0 9 * * *" --recipe-source ./recipes/daily-report.yaml`. Core ships only a handful of platform tools; everything else arrives via extensions/MCP | Scheduler has had rough edges — issue #3882 "Background Scheduled Jobs Run in Chat Mode, Blocking Tool Execution" (opened 2025-08-06, now closed); issue #5045 "UI Scheduler Issues" (state **not verified**). Two schedulers coexist: legacy `tokio-cron-scheduler` and Temporal, and each run spins up a fresh Agent | Cron **yes**. Messaging **no** |
| **openai/codex** | https://github.com/openai/codex | Rust | ~549K LOC (secondary source) | 120,933, Apache-2.0, pushed 2026-09-02 | **The sandbox.** macOS delegates to `sandbox-exec` with a Seatbelt profile; Linux layers Landlock LSM over seccomp-bpf, with Bubblewrap for filesystem namespaces and seccomp blocking network. Code at `codex-rs/linux-sandbox/src/landlock.rs` | 14,901 open issues. Config churn — profiles moved to separate files in 0.134.0 | No / No. Local models **yes**: `codex --oss` targets Ollama or LM Studio; `model_provider`, `oss_provider` in `~/.codex/config.toml` |
| **pi** (was pi-mono, Mario Zechner) | https://github.com/badlogic/pi-mono → now **earendil-works/pi** | TypeScript | ~67 MB repo | 100,903, MIT, pushed 2026-09-02 | **Radical tool minimalism.** Default toolset is four tools: read, write, edit, bash. Everything else is skills, prompt templates, extensions or packages. Also the clean package split: `pi-agent-core` (agent loop + tool calling + state), `pi-tui` (flicker-free differential rendering), `pi-coding-agent` (CLI) | TypeScript, so no single static binary | No / No |
| **crush** | https://github.com/charmbracelet/crush | Go | ~29.7 MB repo | 27,860, NOASSERTION, pushed 2026-09-02 | Go single binary + LSP integration for real code intelligence | Non-standard license. Coding-only | No / No |
| **aider** | https://github.com/Aider-AI/aider | Python | ~137 MB repo | 48,675, Apache-2.0 | Tree-sitter repo map plus git-aware commits per edit | **Last push 2026-05-22** — over three months stale, 1,846 open issues | No / No |
| **OpenHands** | https://github.com/All-Hands-AI/OpenHands | TypeScript | ~407 MB repo | 85,955, MIT, pushed 2026-09-02 | Full runtime isolation model | Heavy. Wrong shape for a small always-on daemon | No / No |
| **nanobot** | https://github.com/nanobot-ai/nanobot | Go | ~4.7 MB repo | 1,337, Apache-2.0, pushed 2026-09-02 | Agents defined in `nanobot.yaml` plus one `.md` per agent; serves MCP over HTTP | Small community | No / No |

**Zed's agent panel** — the reusable idea is not the panel, it is **ACP (Agent Client Protocol)**: JSON-RPC 2.0 over stdin/stdout, Apache-2.0, released by Zed Industries August 2025, explicitly modelled on LSP (https://zed.dev/acp). A public registry launched with JetBrains in January 2026, and by August 2026 dozens of agents and several editors implement it. ZeroClaw already exposes an ACP channel. If JT wants his agent drivable from an editor later, implementing ACP is cheaper than writing plugins.

**Claude Code** — public architecture notes only. The consistently reported design point is a **single-threaded main loop with one flat message list**, chosen for debuggability over multi-agent swarms, with sub-agents spawned in a controlled way rather than competing personas, TODO-based planning, diff-based edits, and context compression (https://blog.promptlayer.com/claude-code-behind-the-scenes-of-the-master-agent-loop/, 2026-07-04; https://www.zenml.io/llmops-database/claude-code-agent-architecture-single-threaded-master-loop-for-autonomous-coding). Internal codenames circulating in third-party writeups (`nO` loop, `h2A` queue) come from reverse-engineering, not Anthropic — treat as **not verified**.

**Design points worth stealing, ranked:**

1. ZeroClaw's Signal channel shape — one signal-cli daemon, SSE in, JSON-RPC out, sender allowlist.
2. goose's recipe + cron pair — the schedulable unit is a YAML file, not a chat.
3. codex's sandbox layering — Seatbelt on macOS, Landlock + seccomp on Linux.
4. pi's four-tool default — read, write, edit, bash; everything else is optional.
5. Claude Code's single flat loop — resist multi-agent architecture until forced.
6. ACP — one JSON-RPC-over-stdio interface gets editor integration free.

---

## Part C — Signal integration in 2026

**There is still no official Signal bot API** (verified by search 2026-09-02; no announcement found). Every option below is unofficial. Signal's Terms of Service state users must not create accounts "through unauthorized or automated means" (https://signal.org/legal/).

| Option | URL | Language | Status | Linking model | Attachments / groups | ARM64 Linux | Resources | License |
|---|---|---|---|---|---|---|---|---|
| **signal-cli** | https://github.com/AsamK/signal-cli | Java (JRE 25 min) | **Alive.** v0.14.7, **2026-08-01**. 4.9k stars, 2,315 commits | Both: `link` as secondary device, or register a number with SMS/voice | Yes / Yes | Native lib **not bundled** for aarch64 — see below | JVM, or GraalVM native build | **GPLv3** |
| **presage** | https://github.com/whisperfish/presage | Rust | **Alive.** Last commit **2026-07-27** ("upgrade libsignal to 0.99.0"), 267 stars, 439 commits | Both: secondary device linking, and SMS/voice registration | Yes / Yes | Native Rust, so yes by compilation | Small | **AGPL-3.0** |
| **signal-cli-rest-api** | https://github.com/bbernhard/signal-cli-rest-api | Go wrapper + Java | **Alive.** v0.100, **2026-06-11**. 2.8k stars, 127 open issues | Inherits signal-cli | Yes / Yes | Docker; `native` mode unavailable on armv7 | Varies by mode | **MIT** |
| **signald** | https://signald.org/ | Java | **DEAD.** Site says: "this project is no longer actively maintained. Use signal-cli" | — | — | — | — | — |
| **signalmeow** (in mautrix-signal) | https://github.com/mautrix/signal | **Go** | **Alive.** Module published **2026-07-16**; recent versions on libsignal v0.58.3 | Device provisioning as a linked device | Yes / Yes | Yes | Go binary + libsignal-ffi | AGPL (mautrix) |

### C.1 signal-cli details that matter

Release v0.14.7 (**2026-08-01**) ships `signal-cli-0.14.7-Linux-native.tar.gz` (109.7 MB) — a **GraalVM native image, Linux only**, still marked experimental — plus `signal-cli-0.14.7-Linux-client.tar.gz` (2.8 MB), a thin client that talks to a daemon. Native libraries are bundled for **x86_64 Linux, Windows and macOS only**.

**The Raspberry Pi 5 problem is real but solved.** On aarch64 you get "Missing required native library dependency: libsignal-client", because libsignal-client is a Rust library compiled per architecture. The official wiki (https://github.com/AsamK/signal-cli/wiki/Provide-native-lib-for-libsignal) points at **https://github.com/exquo/signal-libs-build**, whose latest release **libsignal_v0.101.2, published 2026-08-28**, covers aarch64/armv7/i686/x86_64 Linux GNU, x86_64 musl, macOS aarch64 and x86_64, and Windows x86_64. You swap the `.so` inside the jar. Requires **glibc 2.30+**. This is an extra manual step on every libsignal bump — real ongoing maintenance cost.

Daemon mode is what you want: `signal-cli daemon --http <host:port>` exposes SSE events at `/api/v1/events` and JSON-RPC at `/api/v1/rpc`. One JVM, long-lived, language-agnostic.

### C.2 Ban and rate-limit risk — take this seriously

This risk went up sharply in 2026. signal-cli issue **#1911, "Failed to send message due to rate limiting", opened 2026-01-30, still open**: a user who ran reliably at ~1 message/second for half a year found that even at 1 message per 10 seconds they now hit limits after **10 messages per account**, and after solving a CAPTCHA get restricted again after a few more. Related: #1603, #1823, #1202 ("Failed to register: [413] Rate limit exceeded").

Practical rules:
- **Prefer linking as a secondary device over registering a number.** `signal-cli link -n "MyAgent"` then scan the QR from your phone. No registration, no CAPTCHA, no new number, and far less of an abuse signal.
- **But:** registering a phone number with signal-cli **de-authenticates the main Signal app for that number**. If you register instead of link, use a dedicated number.
- A personal agent replying only to you is low volume and low risk. Do not fan out to new contacts — that is the pattern that triggers 413s.
- Keep a sender allowlist (ZeroClaw's `allowed_from`) so a leaked number cannot be used to drive your agent.

### C.3 Recommendation

**For a Rust daemon: run signal-cli in JSON-RPC daemon mode and talk HTTP to it. Do not link presage directly.**

Reasoning: presage is genuinely good and actively maintained, but it is **git-only, not on crates.io**, so you pin a git SHA and own every libsignal break yourself. It is **AGPL-3.0**, which is a real consideration for anything you might later share. And its CDSI contact discovery depends on `libsignal-net`, which depends on BoringSSL, which conflicts with sqlcipher — the README documents workarounds. signal-cli is GPLv3 but you are talking to it over a socket, not linking it, so it stays a separate program. It has 4.9k stars against presage's 267, a stable release cadence, and the protocol churn is someone else's job. This is exactly what ZeroClaw chose, and ZeroClaw is the most successful Rust agent in this space.

Use presage only if a JVM on the target machine is unacceptable — and on a Pi 5 that is a fair concern, since you would also be dodging the libsignal-jni swap. If you go that way, budget real time for upstream breakage.

**For a TypeScript/Bun daemon: signal-cli daemon over JSON-RPC, or signal-cli-rest-api if you want it containerised.**

Same daemon, and from TypeScript you get an `EventSource` for `/api/v1/events` and `fetch` for `/api/v1/rpc` with no native modules at all — no node-gyp, no FFI, works identically under Bun and Node. Prefer raw signal-cli daemon over bbernhard's wrapper: the wrapper's default reception model is **polling** via a `receive` endpoint with an `AUTO_RECEIVE_SCHEDULE` cron, which adds latency your agent does not need, and it carries 127 open issues. Use bbernhard only if Docker packaging is worth more to you than latency.

**Avoid signald entirely — it is dead and its own homepage says so.**

---

## Part D — Local model runtimes and tool calling in 2026

### D.1 The three runtimes

| Runtime | OpenAI-compatible | Native API | Tool calling | Structured output | What to know |
|---|---|---|---|---|---|
| **LM Studio** | Yes, `/v1/chat/completions` and `/v1/responses` on `localhost:1234` | **Yes — `/api/v1/*`**, released stable in LM Studio 0.4.0. `/api/v0` is legacy | **Two tiers** — see below | `json_schema` | Native API adds stateful chat sessions, **MCP via API**, model load/unload/download, streaming load events, per-request context length. `lms` CLI. MLX engine on Apple Silicon |
| **Ollama** | Yes, `/v1` | `/api/chat` | Yes, with `tools` array and `tool_calls` in response. **Streaming + tool calls is supported** — accumulate `thinking`, `content` and `tool_calls` across chunks. Parallel calls supported | `format` param (https://ollama.com/blog/structured-outputs) | Docs publish **no compatibility matrix and no fallback story** (https://docs.ollama.com/capabilities/tool-calling). If a model's Go template never references `.Tools`, nothing is injected and tools silently do nothing |
| **llama.cpp `llama-server`** | Yes | — | Yes, per chat template | `response_format: json_schema`, plus **GBNF grammars** | On current `master`, `--jinja` is **on by default** (`common.h:638`); the function-calling doc still tells you to pass it and is stale. Unknown templates fall back to a **Generic** handler (`Chat format: Generic` in the log) which "may consume more tokens and be less efficient". Check `GET /props` → `chat_template_caps` |

**LM Studio's two tiers are the single most useful thing in this section**, because it is a worked example of the fallback JT needs to build (https://lmstudio.ai/docs/developer/openai-compat/tools):

- **Native**: the model's chat template is trained for tools; LM Studio parses its format. These get a hammer badge. Tested-native list is short — Qwen2.5-7B-Instruct, Llama-3.1-8B / 3.2-Instruct, Ministral-8B-Instruct-2410 (GGUF and MLX).
- **Default**: everything else. LM Studio injects a system prompt asking for `[TOOL_REQUEST]{...}[END_TOOL_REQUEST]`, then **converts `tool` role messages to `user` role** so templates lacking a `tool` role still work. Results vary by model.

That is exactly the text-protocol fallback, including the awkward-but-necessary role rewriting. Copy the shape.

### D.2 Benchmarks — and a warning about them

**BFCL's leaderboard page is JavaScript-only.** https://gorilla.cs.berkeley.edu/leaderboard.html (fetched 2026-09-02) renders scores client-side; the methodology and a "Last updated April 12, 2026" stamp are readable, **the scores are not**. Current version is **BFCL V4** (blog posts dated 2025-07-17), weighted Agentic 40% / Multi-Turn 30% / Live 10% / Non-Live 10% / Hallucination 10%. Secondary aggregators for BFCL V4 **contradict each other** — llm-stats.com and benchlm.ai barely share a model list, and llm-stats marks its rows *0 verified, 18 self-reported*. **Treat all secondhand BFCL numbers as not verified.**

Use **τ²-bench** instead — first-party runs, dated, 120 models: https://openrouter.ai/benchmarks/tau2-bench-airline (run stamped 2026-09-02). Note tau2-bench v1.0.1 (July 2026) changed grading, so results across that boundary are not comparable (https://github.com/sierra-research/tau2-bench).

Selected open-weight scores, τ²-bench airline, run 2026-09-02:

| Model | Score | Model | Score |
|---|---|---|---|
| Qwen3.5 397B A17B | 78.7% | gpt-oss-120b | 64.4% |
| Qwen3.5 122B A10B | 76.7% | gpt-oss-20b | 51.4% |
| Gemma 4 31B | 76.1% | Llama 4 Maverick | 44.6% |
| Qwen3.5 35B A3B | 73.3% | **Qwen3 14B / 32B** | **42.3% / 42.0%** |
| Gemma 4 26B A4B | 68.3% | Llama 3.1 8B Instruct | 30.9% |
| **Qwen3.5 9B** | **67.5%** | Qwen2.5 7B Instruct | 16.7% |

**The headline finding for JT: Qwen3.5-9B beats Qwen3-32B and gpt-oss-20b.** One model generation is worth more than 3× the parameters. And within a generation, Qwen3 14B ≈ Qwen3 32B — scaling bought nearly nothing for multi-turn tool use.

### D.3 Which models to actually run

| Family | Native tool template | Ollama `tools` tag | Small sizes worth using |
|---|---|---|---|
| **Qwen3.5** | Yes | Yes | **4B, 9B** — best small tool callers found |
| Qwen3 / Qwen3-Coder | Yes | Yes | 8B, 14B (weak on multi-turn, see above) |
| **Gemma 3** | **No — prompted only** | **No tag** | Emulated tool calling only |
| **Gemma 4** | **Yes — native** | Yes | **E4B, 12B** |
| Granite 4.1 | Yes | Yes | **3B (2.1 GB), 8B (5.3 GB)** |
| Mistral Small 3.2 / Nemo | Yes | Yes | Nemo 12B; Small 3.2 24B borderline |
| Hermes 4 | Yes (`<tool_call>`) | not listed | 14B |
| Llama 4 Scout/Maverick | Yes | Yes | none small |
| DeepSeek V4, GLM-5.x | Yes | Yes | none small |

**The Gemma trap is worth calling out.** Gemma 3 has no native function calling — no `tool` role, no tool tokens. Google's own representative confirmed this on the model card discussion (https://huggingface.co/google/gemma-3-12b-it/discussions/11), saying Gemma 3's instructability "allows for effective function calling by defining functions and output formats directly in user prompts". Ollama's gemma3 page carries a vision tag only. **Gemma 4 reverses this** with documented `apply_chat_template(tools=…)` support (https://ai.google.dev/gemma/docs/capabilities/text/function-calling-gemma4, updated 2026-06-04) — but its wire format is a **custom token DSL, not JSON**: `<|tool_call>call:get_current_weather{location:<|"|>Tokyo, JP<|"|>}<tool_call|>`. A harness that assumes JSON tool calls will break on it.

Also note: MLX tool parsers are fragile. mlx-lm issue #1125 (2026-04-08, still open) has `gemma-4-26b-a4b-it-4bit` throwing "No function provided" at the recommended sampling params — a parser/template mismatch. **Test your exact quant, not the family.**

### D.4 Hardware

**Mac.** macOS caps GPU-usable memory below installed RAM via Metal's `recommendedMaxWorkingSetSize`. Measured on a 32 GB Mac: **22,906 MB ≈ 70%** (https://blog.peddals.com/en/fine-tune-vram-size-of-mac-for-llm/). Machines at 64 GB+ get roughly 75% (https://github.com/ggml-org/llama.cpp/discussions/4167). Raise it with `sudo sysctl iogpu.wired_limit_mb=N` on macOS 15.0+. **Read your own value rather than trusting a percentage.**

| RAM | GPU ceiling | Q4_K_M / MLX-4bit | Context |
|---|---|---|---|
| 16 GB | ~10–11 GB | 3B–8B | 4k–16k |
| 32 GB | ~22 GB (measured) | 8B–14B; 27B tight | 16k–32k |
| 64 GB | ~48 GB | 27B–32B comfortable | 32k–64k |
| 128 GB | ~96 GB | 70B Q4, 100B+ MoE | 64k–128k |

That table is arithmetic derived from the two cited ceilings, **not measured** — rule of thumb Q4_K_M ≈ 0.6–0.8 GB per billion params. `lms load --estimate-only` prints a real estimate. LM Studio's MLX engine v1.8.5 added disk-backed KV checkpointing: 4-way parallel chat **16.78s vs 37.60s, 82% less RAM growth** on an M3 Max with Qwen3.6-27B (https://lmstudio.ai/blog/mlx-engine-agentic-workloads, 2026-06-05). Warning from llama.cpp docs: extreme KV quantization (`-ctk q4_0`) "can substantially degrade tool calling performance."

**Raspberry Pi 5 — be blunt: it cannot host the main model.** Best measured result found is Qwen3-30B-A3B Q3_K_S on a 16 GB Pi 5: **pp512 10.91 t/s, pp4096 8.62 t/s, tg128 7.99 t/s** (https://github.com/geerlingguy/ai-benchmarks/issues/47, 2026-01-07). Prefill is the killer, not generation: at 8.62 t/s a 3k-token system prompt plus tool schemas is minutes of reading *per turn*. Stratosphere measured a ~5,000-token prompt taking "roughly 15 minutes", and found function calling "failed in most of the considered models except BitNet B1.58 2B 4T and SmolLm2:1.7b" (https://www.stratosphereips.org/blog/2025/6/5/how-well-do-llms-perform-on-a-raspberry-pi-5, 2025-06-05) — the models fast enough are the ones that cannot emit reliable tool calls. No accelerator rescues it either: Jeff Geerling's AI HAT+ 2 (Hailo-10H) review concluded **"The Pi's built-in CPU trounces the Hailo 10H"** (https://www.jeffgeerling.com/blog/2026/raspberry-pi-ai-hat-2/, 2026-01-15).

**So on The 7900 XT machine: run the daemon, signal-cli, cron and SQLite on the Pi. Run the model on the Mac, or in the cloud.** Reach the Mac over Tailscale. Use a tiny Pi-local model only as a degraded fallback for classification or extraction.

### D.5 The fallback ladder — and a genuine surprise

**Rung 1 — fix the template before anything else.** Probe `GET /props` → `chat_template_caps` on llama.cpp, or the Ollama tag. If the log says `Chat format: Generic`, the model has no tool template and you are already in fallback.

**Rung 2 — structured output.** `response_format: json_schema` (llama.cpp, LM Studio) or Ollama's `format`. Ollama's docs still advise putting the schema in the prompt as well. LM Studio warns: "Not all models are capable of structured output, particularly LLMs below 7B parameters."

**Rung 3 — constrained decoding, used sparingly.** XGrammar is the 2026 default in vLLM, SGLang, TensorRT-LLM, MLC and OpenVINO GenAI (https://github.com/mlc-ai/xgrammar); XGrammar-2 claims >6× faster compilation (https://arxiv.org/abs/2601.04426). **llama.cpp does not use XGrammar** — it uses GBNF plus optional LLGuidance. JSONSchemaBench (https://arxiv.org/html/2501.10868v1, 10k schemas) found the best framework covers ~2× the schemas of the worst; Guidance ~7 ms TPOT vs ~30 ms; Outlines pays a 3–13 s compile cost.

**The surprise, and the most important finding in Part D: constraints fix syntax but damage competence.** "The Constraint Tax" (https://arxiv.org/abs/2605.26128, 2026-05-20; Qwen2.5-0.5B/1.5B and SmolLM2-1.7B, 15k generations) found hard schema decoding raised validity **61.5% → 100%** while dropping accuracy **19.7% → 11.0%**, with wrong-but-valid outputs rising 49.5% → 88.9%. On a calendar tool-call task, Qwen2.5-1.5B scored **91.5% executable accuracy with prompt-only JSON versus 48.0% under hard schema** — both at 100% validity. "Repair, Not Improvement" (https://arxiv.org/abs/2608.13959, 2026-08-14) found constrained decoding **negative in 4 of 6 cells** (worst −29.5pp) on 0.6–4B models. So: do not reach for grammars first.

**Rung 4 — if you must leave native tool calling, use Python code, not XML.** BFCL V4's format-sensitivity study (https://gorilla.cs.berkeley.edu/blogs/17_bfcl_v4_prompt_variation.html, 2025-07-17; 26 prompt variations × 39 models) found "models generally achieve higher accuracy with Python and JSON return formats than with either XML variant", ranking JSON > Python > XML, with small models degrading most from `<TOOLCALL>` tags. But CodeAct (https://arxiv.org/abs/2402.01030) reports up to +20% over JSON for *code actions*, and "The Bitter Lesson of Tool Calling" (https://arxiv.org/abs/2608.06370, 2026-08-06) found programmatic calls matched or beat JSON in 11 of 14 models on BFCL v4. This contradicts the common assumption that OpenClaw-style XML tool blocks help small models — **they do not.**

**Rung 5 — repair loops, where the real recovery lives.** "Structured Feedback Improves Repair" (https://arxiv.org/abs/2607.14167, 2026-07-15): Qwen2.5-Coder-14B went 14/50 → 36/50 (**+44pp**) and Llama-3.1-8B 8/50 → 29/50 (**+42pp**) with a 4-call cap. The ablation is the lesson — nearly all the gain came from feeding back **admissible alternatives**, not merely naming the error. At harness level, HarnessFix (https://arxiv.org/html/2606.06324v2, 2026-07-02) reports **+11.1% average** across GAIA, SWE-Bench, AppWorld and Terminal-Bench from schema narrowing, loop guarding and verification-gated finalization.

---

## What I would tell JT to do

1. **Do not start from scratch.** Read ZeroClaw first (Rust, Signal, cron, web dashboard, Ollama, sandboxing, MIT/Apache). It is his spec, already built, with 32.7k stars. Either use it or fork the parts he wants.
2. **Signal: run `signal-cli daemon --http`, link as a secondary device, and talk SSE + JSON-RPC to it.** Language-agnostic, survives protocol churn, and is what the most successful project in this space chose. Keep a sender allowlist. Do not register a fresh number unless you must — rate limiting got much harsher in 2026.
3. **On the Pi 5, host the daemon, not the model.** Prefill speed makes Pi-local agent models unusable.
4. **Default local model: Qwen3.5 9B or 4B.** Gemma 4 12B and Granite 4.1 8B are the backups. Avoid Gemma 3 for tool loops entirely.
5. **Build the fallback ladder in this order**: native tools → json_schema → prompted `[TOOL_REQUEST]`-style text protocol → repair loop with admissible alternatives. Put grammars near the bottom, not the top.
6. **"Atomic Agent" is AtomicBot's TypeScript agent.** It is genuinely broken today and Telegram-only, but its local-model support is newer than his complaint — worth one re-test, not a bet.
