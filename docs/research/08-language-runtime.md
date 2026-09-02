# 08 — Language and runtime for the Coeus daemon

Date: 2026-09-02. Host: Apple M2 Max (12 cores, 32 GB), macOS 26.5.1. Toolchains: cargo 1.84.0, go 1.26.5, bun 1.3.5, node 24.14.1, zig 0.16.0, Apple clang 21. All numbers below are **medians of 5 runs** unless marked otherwise. "Measured" means run on this Mac today. "Estimated" means extrapolated and says from what.

Bench sources live in `scratchpad/lang-bench/` (hello programs and a minimal HTTP+SQLite daemon per language) and `scratchpad/realistic/` (a dependency set close to what Coeus will actually need). Raw output: `scratchpad/results/{startup,rss,build}.txt`.

## 1. Measurements

### 1.1 How they were taken

- **Time to first output (TTFO)**: `tools/ttfo` forks the program, reads the first byte of stdout, and reports elapsed wall time. For daemons the first byte is the `listening <port>` line printed after the socket is bound and SQLite is open. This includes fork+exec and dynamic linking. Page cache warm.
- **Idle RSS**: `ps -o rss=` 2 s after launch with no requests. A second profile (`tools/bench-daemon.sh`) samples RSS after `/health`, after 250 requests (200 `/health` + 50 `/db` inserts), and 3 s later.
- **Clean build**: fresh target/cache directory, package registry already downloaded (no network in the timed window). Rust: `cargo build --release` into an empty `CARGO_TARGET_DIR`. Go: `go clean -cache` then `go build`. Bun: `bun build --compile`. Zig: fresh local and global cache dirs.
- Every daemon was killed after each run and `pgrep` confirmed nothing was left.

### 1.2 Hello world (measured)

| Program | Binary | TTFO | Clean build |
|---|---|---|---|
| C (`clang -O2`) | 33 KB | 1.83 ms | 0.04 s |
| Zig 0.16 (`-O ReleaseSmall -lc`) | 50 KB (linux-arm64 static: 63 KB) | 1.83 ms | 1.02 s (0.10 s warm cache) |
| Rust release (stripped) | 333 KB | 1.73 ms | 0.14 s |
| Rust `min` profile (opt z, LTO, panic=abort) | 302 KB | 1.76 ms | — |
| Go | 2.49 MB (stripped: 1.66 MB) | 2.26 ms (2.15 ms) | 1.71 s (0.05 s incremental) |
| Bun compiled | 59.9 MB | 7.87 ms | 0.07 s |
| Bun compiled `--minify` | 59.9 MB | 9.82 ms | — |
| `bun run hello.ts` | — | 8.01 ms | — |
| `bun hello.js` | — | 7.82 ms | — |
| `node hello.js` | (node binary 119 MB) | 30.4 ms | — |
| `bun --version` / `node --version` | — | 3.2 ms / 18.7 ms | — |

Reading: native binaries start in under 2 ms. Bun's compiled binary is 60 MB because it embeds the whole runtime, but it still starts in 8 ms. Node is 4x slower to start than Bun and 15x slower than Go.

### 1.3 Minimal daemon: HTTP server + SQLite, `/health` and `/db` routes (measured)

| Daemon | Binary (macOS arm64) | Linux arm64 artifact | TTFO to `listening` | RSS idle 2 s | RSS after 250 req | Threads | Clean build | Incremental |
|---|---|---|---|---|---|---|---|---|
| Rust (axum 0.8 + tokio + rusqlite bundled, 71 crates) | 2.96 MB | gnu 2.91 MB (dynamic), musl 2.88 MB (static) | 2.90 ms | 3.7 MB | 4.3 MB | 13 | 17.2 s | 0.97 s release, 0.50 s debug |
| Rust `min` profile | 1.46 MB | — | 2.48 ms | 3.0 MB | 3.4 MB | 13 | 16.1 s (1 run) | — |
| Go (net/http + modernc.org/sqlite, 10 modules) | 14.3 MB (stripped 9.66 MB) | 9.44 MB static | 8.23 ms | 19.1 MB | 19.7 MB | 8–9 | 6.0 s | 0.11 s |
| Bun compiled (`Bun.serve` + `bun:sqlite`) | 59.9 MB | 96.8 MB | 11.0 ms | 22.5 MB | 24.1 MB | 7 | 0.07 s | 0.07 s |
| `bun run bun-daemon.ts` | — | — | 11.1 ms | 25.1 MB | 26.5 MB | 7 | none | none |
| `node node-daemon.mjs` (node:sqlite) | — | — | 46.8 ms | 59.6 MB | 62.1 MB | 8 | none | none |

Notes:
- Rust's 13 threads are tokio's multi-thread runtime spawning one worker per core. A daemon should use `worker_threads = 2` or the current-thread runtime; that would drop RSS a little further.
- Rust cross-compile: `cargo zigbuild` (installed at `/opt/homebrew/bin/cargo-zigbuild`) produced both the gnu and musl arm64 binaries. **Plain `cargo build --target aarch64-unknown-linux-musl` fails** here: `libsqlite3-sys: sqlite3.c: fatal error: 'stdio.h' file not found` (no cross sysroot). Installed rustup targets: `aarch64-apple-darwin`, `aarch64-unknown-linux-gnu`, `aarch64-unknown-linux-musl`. So Rust cross works, but only with the extra tool.
- Go cross-compile: `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build` — 7.2 s from a clean cache, 0.11 s warm. No extra tools. Static binary.
- Bun cross-compile: `bun build --compile --target=bun-linux-arm64` — 0.01 s once the Linux runtime is cached (it is copied, not compiled). The Linux binary is 97 MB versus 60 MB on macOS with Bun 1.3.5. Bun 1.4 (released 2026-08-20) claims 17% smaller Linux binaries and half the startup memory; not measured here because JT's installed Bun is 1.3.5.
- Go daemon RSS is flat at 19 MB because the Go runtime reserves heap arenas up front; it does not grow with the trivial load.

### 1.4 Realistic dependency sets (measured, built today)

These approximate what Coeus needs: HTTP client with TLS, WebSocket, SQLite, cron parser, OS keyring, CLI parsing, logging. Go also pulls in chromedp, the official MCP SDK and the official Anthropic SDK because they were cheap to add.

| Set | Deps | Binary | Linux arm64 | RSS idle 2 s | Clean build (Mac) | Incremental | Cross build |
|---|---|---|---|---|---|---|---|
| Rust: tokio, axum+ws, reqwest (rustls), serde, rusqlite bundled, croner, keyring, tracing, clap, tokio-tungstenite | 239 crates | 6.19 MB | 5.94 MB musl static | 9.7 MB | 24.2–24.8 s release; 12.8 s debug | 1.27 s release; 0.64 s debug | 15.5 s (zigbuild, musl, from clean) |
| Go: net/http, coder/websocket, robfig/cron, modernc.org/sqlite, chromedp, modelcontextprotocol/go-sdk, anthropic-sdk-go, go-keyring | 66 modules | 23.5 MB | 23.3 MB static | 19.4 MB | 9.5–10.6 s | 0.14 s | 10.1 s (from Mac cache) |
| Bun: playwright-core 1.62.1 + croner 10 + bun:sqlite | 2 packages (43 MB node_modules) | 66.6 MB | 103 MB | **107 MB** | 0.25 s (needs `--external chromium-bidi`, see 3.C) | — | 0.12 s |

Two things happened during the Rust build that are worth recording because they are the kind of friction Claude will hit:
1. The first `cargo fetch` failed: several current crates need edition 2024 (cargo >= 1.85) and JT's cargo is 1.84 (November 2024). Fixed with `resolver = "3"` + `rust-version = "1.84"` (the MSRV-aware resolver). `rustup update` would also fix it. Go 1.26 and Bun 1.3.5 hit nothing similar.
2. My 18-line probe called `croner::Cron::new(..)`, the 2.x API. The resolver picked 3.0.1, where it is `str::parse::<Cron>()`. One extra compile-fix cycle. Rust crate APIs move between minor versions more than Go's do.

Each Rust target directory is ~650 MB on disk (release and debug each).

### 1.5 Playwright import cost (measured, 5 runs)

| Process | TTFO to `listening` | RSS idle 2 s |
|---|---|---|
| Bun compiled daemon without Playwright | 11.0 ms | 22.5 MB |
| Bun compiled daemon with `import { chromium } from "playwright-core"` | 147.8 ms | 106.8 MB |
| `bun run` same script | 149.0 ms | 116.7 MB |
| `node` + playwright-core (import only) | 240.3 ms | 131.8 MB |

Importing playwright-core costs ~85 MB and ~135 ms before Chrome is even launched. This alone decides the browser-worker question (section 4).

### 1.6 Pi 5 build-time estimates

Anchors: Raspberry Pi 5 Geekbench 6 multi-core 1,604 ([raspberrypi.com](https://www.raspberrypi.com/news/benchmarking-raspberry-pi-5/)) versus M2 Max 12-core 14,757 ([browser.geekbench.com](https://browser.geekbench.com/processors/apple-m2-max)) = 9.2x. A Rust release build measured on a CM5 (same SoC) took 69.7 s versus 13.7 s on a 16-core Ryzen AI Max+ 395 = 5.1x ([tinycomputers.io](https://tinycomputers.io/posts/rust-compilation-performance-benchmark-report.html)). Compiles do not parallelize perfectly, so I use **5–9x** the M2 Max time.

| Build on Pi 5 (estimated) | Time |
|---|---|
| Go, Coeus-sized (66 modules, ~15k lines), clean | 1–2 min |
| Go, incremental | 1–3 s |
| Rust, 239-crate set with a tiny main, clean release | 2–4 min |
| Rust, ~15k-line app with ~150 deps, clean release | 5–12 min (Mac estimate 60–90 s: 25 s deps + 30–60 s for the app crate itself) |
| Rust, same app, incremental release after a one-file fix | 1–4 min (the app crate recompiles as a unit unless split into workspace crates) |
| Rust, same app, incremental debug | 15–60 s |
| Bun, any size | `bun build --compile` ~0.1–0.5 s; or run the `.ts` directly, no build |

Rust on the Pi also needs the rustup toolchain (~1 GB) and 650 MB+ of target directory per profile. ZeroClaw at 775k LOC is not something a Pi rebuilds; that is an hour-class build (estimated).

## 2. Comparison

| | Rust | Go | TS on Bun (compiled) | TS on Bun (script) | C++ | Zig |
|---|---|---|---|---|---|---|
| Startup, daemon (measured) | 2.9 ms | 8.2 ms | 11.0 ms | 11.1 ms | ~2 ms (C proxy; not built) | ~2 ms (hello only) |
| Idle RSS, realistic daemon (measured) | 9.7 MB | 19.4 MB | 22.5 MB | 25 MB | ~5 MB (estimated) | ~3 MB (estimated) |
| Binary, realistic daemon (measured) | 6 MB | 23 MB | 60–67 MB (97–103 MB Linux) | n/a + 60 MB runtime | few MB (est.) | <1 MB (est.) |
| Cross to linux-arm64 from Mac | Works with `cargo zigbuild`; plain cargo fails on C deps | Built in, 10 s, static | Built in, instant, 97 MB | n/a (install Bun on Pi) | Painful without zig cc | Built in, trivial |
| Async model | tokio async/await; `Send` + lifetimes across `.await` | goroutines + channels; blocking style; `context` for cancel | single-threaded event loop, async/await, Workers | same | Asio / callbacks; no standard | pre-1.0 async (removed/reworked) |
| Crash safety | No UB; panics isolated per task if unwinding kept; `unwrap()` is the usual sin | Nil-pointer panics kill the process unless `recover()`ed; data races possible (`-race`) | Uncaught exception / unhandled rejection kills process; runtime type errors slip past tsc; Bun itself is young | same, plus runtime version drift on `bun upgrade` | UB, segfaults, sanitizers needed | UB possible; safety checks in Debug/ReleaseSafe |
| Clean build, Mac (measured) | 25 s | 10 s | 0.07–0.25 s | none | seconds (est.) | 1 s (hello) |
| Clean build, Pi 5 (estimated) | 5–12 min | 1–2 min | <1 s | none | minutes with CMake | seconds |
| Incremental, Mac (measured) | 1.3 s release / 0.6 s debug | 0.14 s | 0.07 s | none | — | 0.10 s |
| Claude fluency (my judgment) | Good but slowest loop: borrow checker in async code, trait bounds, `Arc<Mutex<>>` plumbing, crate API drift; multi-round fixes | Very good: one way to do things, gofmt, clear compile errors, verbose but mechanical error handling | Best: most training data, JT reads it, instant feedback, Bun APIs well known | same | Good code, costly debugging (UB, build systems) | Weakest: little training data, std API churn (see below) |
| Toolchain stability seen today | cargo 1.84 too old for edition-2024 crates | none | none | none | — | `std.posix.write` no longer exists in 0.16 (`bad.zig` fails) |

Library availability for Coeus's requirements (status checked today; URLs in section 5):

| Need | Rust | Go | TypeScript |
|---|---|---|---|
| HTTP client + SSE streaming | reqwest 0.12.28 `bytes_stream`, hand-parse SSE (30 lines) | net/http + bufio, hand-parse | `fetch` + ReadableStream, hand-parse; both runtimes |
| WebSocket server | axum `ws` feature (tokio-tungstenite) | coder/websocket v1.8.15 (Jun 2026) | `Bun.serve` built in |
| SQLite | rusqlite 0.40.2 (Aug 2026), bundled C | modernc.org/sqlite v1.58.0 (Sep 1 2026), pure Go, no cgo | `bun:sqlite` built in (node: `node:sqlite`) |
| Cron parsing | croner 4.0.0 (Aug 31 2026) | robfig/cron v3.0.1 (Jan 2020; dormant, stable, 5,585 importers) or adhocore/gronx | croner 10.0.1 |
| TUI | ratatui 0.30.2 (Jun 2026) | bubbletea v2 (2026) | @opentui/core 0.5.10 (Bun-first, Zig core, ships linux-arm64 native pkg); Ink (Node/React) |
| signal-cli JSON-RPC + SSE | trivial | trivial | trivial |
| Anthropic / OpenAI / Ollama / LM Studio | No official Anthropic Rust SDK (community crates only); hand-roll over reqwest | anthropic-sdk-go and openai-go, both official | @anthropic-ai/sdk, openai, both official |
| MCP client | rmcp 3.2.0 (Aug 31 2026), official | modelcontextprotocol/go-sdk v1.7.0, official, with Google | @modelcontextprotocol/sdk, official |
| systemd notify / launchd | sd-notify 0.5.0 (Mar 2026) | coreos/go-systemd `daemon.SdNotify` | hand-roll or exec `systemd-notify`; launchd needs nothing |
| Keychain | keyring 4.2.0 (Aug 29 2026) | zalando/go-keyring v0.2.8 | @napi-rs/keyring 2.0.0 (Aug 31 2026; binds the Rust crate) |
| CDP browser control | chromiumoxide 0.9.1 (Feb 2026; 3.8M downloads) | chromedp v0.16.0 (Jul 2026; 2,179 importers); rod v0.116.2 (Jul 2024, stale) | playwright-core 1.62.1; puppeteer |
| Desktop automation | enigo 0.6.1 (Aug 2025) + xcap 0.9.8 (Aug 2026) | robotgo (Mar–Jun 2026 releases; pure-Go backends, CGO_ENABLED=0) | @trycua/cua-driver 0.23.2 (Aug 31 2026; MIT; Rust core; native pkgs for darwin-arm64 and linux-arm64-gnu); nut.js (core OSS, image/OCR plugins paid) |
| Gmail API | google-gmail1 7.0.0 (Jan 2026; auto-generated, community) or hand-rolled REST | google.golang.org/api/gmail/v1, official | googleapis, official |

Headless-Pi caveat for all three keyring options: there is no Secret Service daemon on a headless Linux box, so on the Pi secrets end up in a 0600 file regardless of language. The keyring library only helps on the Mac.

C++ and Zig, briefly. C++ matches Rust on size and startup and has the widest raw library base, but Claude's fix loop is worse than Rust's: memory bugs show up at runtime, not compile time, and CMake/vcpkg add friction for cross-builds. There is no official MCP or Anthropic C++ SDK. Zig produces the smallest binaries and cross-compiles trivially, but it is 0.16 and pre-1.0; the standard library API changed under a two-line program written this morning, and there is no Zig TUI/CDP/MCP ecosystem worth the name. Neither is a serious option for a project that Claude has to maintain.

## 3. Three architectures

Shared facts: signal-cli is an external JVM/GraalVM daemon in all cases. Chrome itself costs 150–400 MB when running, in all cases. The daemon's job is to be small and boring; the browser work is bursty.

### A. All-Rust daemon + TS/Playwright browser worker sidecar

- **Numbers (measured)**: 6 MB binary, 3 ms start, ~10 MB idle. Sidecar adds ~107 MB (Bun) or ~132 MB (Node) only while a browser task runs.
- **Claude write/fix**: hardest of the three. Async Rust means `Send` bounds, lifetimes across `.await`, `Arc<Mutex<_>>` for shared state, and trait-heavy library APIs (axum extractors, tower layers). Claude gets there, but compile-error rounds are longer and crate APIs drift (croner today). Two languages in the repo.
- **Self-fix on the Pi**: Rust must be installed on the Pi (~1 GB). A one-file fix to a 15k-line app costs an estimated 1–4 min in release mode; a clean build 5–12 min. Feasible but slow enough that the agent's own repair loop feels broken. Cross-build from the Mac is the realistic path and needs `cargo-zigbuild`.
- **Update/rollback**: single static musl binary; `rename(2)` over the old file, `systemctl restart`, keep `coeus.prev` for rollback. Best in class.
- **Browser**: sidecar via JSON-RPC over stdio. Rust could instead use chromiumoxide (alive, 0.9.1) and drop the sidecar, but Claude knows Playwright far better and the persistent-profile story is smoother in Playwright.
- **Biggest risk**: Claude's token cost per fix, plus toolchain drift (cargo 1.84 could not build today's crates). The 10 MB memory win over Go buys nothing on an 8 GB Pi.

### B. Go daemon + Bun/TS browser worker

- **Numbers (measured)**: 23 MB static binary, 8 ms start, 19 MB idle, 10 s clean build on the Mac, 0.14 s incremental. Sidecar as in A.
- **Claude write/fix**: easy. Go has one idiom, the compiler rejects unused imports and variables, and the streaming loop (goroutine per stream, channel to the WebSocket writer) is textbook. Verbose `if err != nil` costs tokens but never costs a round trip. Official Anthropic, OpenAI, MCP and Gmail SDKs exist.
- **Self-fix on the Pi**: Go on the Pi is one 100 MB tarball. Clean rebuild ~1–2 min (estimated), incremental seconds. Cross-build from the Mac is a one-line env change with no extra tools.
- **Update/rollback**: same as A. Static binary, no runtime dependency.
- **Browser**: Bun/TS worker with playwright-core, spawned on demand, killed when idle. Alternative: chromedp in Go (v0.16.0, actively maintained) for a single-language repo, accepting that Claude writes Playwright better than chromedp and that Playwright's auto-waiting is nicer.
- **Biggest risk**: two languages if the worker is TS. Mitigation: the worker is small (a few hundred lines wrapping Playwright and cua-driver behind a JSON-RPC stdio protocol) and rarely changes. Second risk: nil-pointer panics in Claude-written Go; `recover()` middleware on every goroutine entry point and `go vet` + `staticcheck` in the build.

### C. Bun single binary for everything, Playwright in-process

- **Numbers (measured)**: as specified, this fails requirement (f). Importing playwright-core takes the daemon from 11 ms / 22.5 MB to 148 ms / 107 MB idle. It also does not compile as-is on Bun 1.3.5: `bun build --compile` stops on `chromium-bidi/...` resolution and needs `--external chromium-bidi` (fine for Chrome-only CDP, but it is a workaround).
- **C-prime (fixed)**: Bun daemon without any Playwright import (11 ms, 22.5 MB, 60 MB binary, 97 MB on Linux) plus the same binary re-executed with `--browser-worker` as a subprocess when a browser task arrives. One language, one binary, still meets (f).
- **Claude write/fix**: easiest. JT already uses Bun; Claude's TypeScript is its strongest language; `bun test` and `tsc --noEmit` give sub-second feedback. Every requirement has an official or Bun-built-in library.
- **Self-fix on the Pi**: none needed. `bun build --compile` in 0.1 s, or run the source.
- **Update/rollback**: single 97 MB binary; same rename-and-restart. The runtime version is frozen inside the binary, which is good: no `bun upgrade` surprise.
- **Browser**: Playwright support on Bun only became official in Bun 1.4 (2026-08-20). `connectOverCDP` was broken on Bun from April 2024 until then, Playwright's maintainers refused a Bun-specific workaround (PR #34546 closed unmerged), and the `chromium.launch()` stdio-pipe fix landed only recently (Bun PR #31829; its regression tests in #35417 were still unmerged today). It works now, but it is two weeks old. Running the worker under Node instead (Playwright's supported runtime) costs 240 ms and 132 MB per spawn, which is acceptable for an on-demand worker.
- **Biggest risk**: runtime maturity. Bun 1.4 is a large rewrite; regressions are likely over the next few months, and a JS daemon has no compiler catching the class of runtime type errors that Go and Rust catch. For a daemon that must run for weeks, that is the trade.

## 4. Recommendation

**Build the Coeus daemon in Go (Architecture B), with a small Bun/TypeScript automation worker as a separately spawned sidecar for Playwright and cua-driver.**

Reasoning, in order of weight:

1. **Claude's fix loop is the real cost.** The prior study showed harness overhead is ~0.1% of a turn; here the daemon's own CPU is irrelevant. What costs tokens is compile-fix rounds. Go and TypeScript both keep that loop to seconds and usually one round. Rust does not, and today's session showed why: a stale cargo could not resolve current crates, and a crate API moved under a two-line probe.
2. **A daemon wants a boring runtime.** Go's net/http, goroutines and static binaries have run daemons for fifteen years. Bun's daemon story is credible (11 ms, 22 MB) but its Playwright support is two weeks old and the runtime is mid-rewrite. Go loses nothing on the numbers that matter: 8 ms start and 19 MB idle are 25x under the OpenClaw baseline (504 MB) and 20x under OpenCode (378 MB).
3. **Self-repair on the Pi is a Go strength.** Estimated 1–2 min clean rebuild, seconds incremental, one tarball to install, and cross-build from the Mac in 10 s with no extra tools. Rust needs cargo-zigbuild on the Mac and minutes per fix on the Pi.
4. **Every requirement has an official or first-class Go library**: anthropic-sdk-go, openai-go, modelcontextprotocol/go-sdk, google.golang.org/api/gmail, coder/websocket, modernc.org/sqlite (no cgo, so the arm64 cross-build stays trivial), chromedp as the fallback browser path, robotgo as the fallback desktop path.

Exact libraries (maintenance status checked today):

| Role | Library | Status |
|---|---|---|
| HTTP server, SSE out, routing | `net/http` (stdlib) | — |
| WebSocket server | `github.com/coder/websocket` v1.8.15 | released Jun 15 2026; ISC |
| HTTP client + SSE in (Anthropic, OpenAI-compatible, LM Studio, Ollama) | `net/http` + hand-rolled SSE parser; `github.com/anthropics/anthropic-sdk-go` for Anthropic-specific features | official SDK; hand-rolled path keeps one provider abstraction |
| SQLite | `modernc.org/sqlite` v1.58.0 | released Sep 1 2026; BSD-3; pure Go |
| Cron | `github.com/robfig/cron/v3` v3.0.1 | Jan 2020, dormant but stable and tiny; fall back to `github.com/adhocore/gronx` if a bug appears |
| MCP client | `github.com/modelcontextprotocol/go-sdk` v1.7.0 | official; v1 API-compatibility guarantee |
| TUI (terminal client) | `charm.land/bubbletea/v2` | v2 shipped 2026 |
| systemd notify | `github.com/coreos/go-systemd/v22/daemon` | stable |
| Keychain (Mac) | `github.com/zalando/go-keyring` v0.2.8 | active; file fallback on the headless Pi |
| Gmail | `google.golang.org/api/gmail/v1` + `golang.org/x/oauth2` | official, Google-maintained |
| Browser (fallback in-process) | `github.com/chromedp/chromedp` v0.16.0 | released Jul 14 2026; MIT |
| Desktop (fallback in-process) | `github.com/go-vgo/robotgo` | releases Mar/Jun 2026; pure-Go backends |
| Worker: browser | `playwright-core` 1.62.x on Bun 1.4+ (or Node 24 if Bun misbehaves) | Playwright-on-Bun official since Bun 1.4, 2026-08-20 |
| Worker: desktop | `@trycua/cua-driver` 0.23.2 | released Aug 31 2026; MIT; darwin-arm64 and linux-arm64 native packages |

What JT gives up with Go:
- About 10 MB of idle RSS and 17 MB of binary versus Rust. Irrelevant on the Pi 5 or the Mac.
- Rust's compile-time guarantees against data races and nil. Go has `-race` and `go vet`; use them in CI.
- One-language purity. The worker is TypeScript. If that bothers him, chromedp and robotgo make the whole thing Go at some ergonomic cost.
- The zero-build loop of Bun. Ten seconds is close enough.

**Runner-up: Bun single binary (C-prime), never C as specified.** Pick it if JT decides one language and one repo matters more than runtime maturity, or if the browser and desktop automation grow into most of the code (both are TypeScript-first: Playwright and cua-driver). The daemon must not import playwright-core; the browser worker is the same binary re-executed in worker mode. Expect to pin the Bun version and to chase a Bun regression or two per year.

**Rust is not recommended for v1.** Its advantages (10 MB idle, 6 MB binary, no GC) do not matter at this scale, and its costs (Claude's fix loop, Pi rebuild minutes, toolchain drift, no official Anthropic SDK) do. Revisit only if a target smaller than a Pi 5 appears or JT wants to learn Rust for its own sake.

**Scripting REPL in the daemon: no for v1.** Same conclusion as the prior study. If the model needs to run code later, it is a tool that spawns `bun -e` or `python3` as a subprocess with a timeout, not an embedded interpreter. Embedding (goja, Deno core, rlua) drags in exactly the memory and crash surface the daemon is trying to avoid.

**Browser worker: sidecar, not in-process, in every architecture.** The measurements settle it: playwright-core costs 85 MB and 135 ms just to import, before Chrome. A sidecar keeps the daemon at 19 MB, lets Chrome or Playwright hang or crash without taking down Signal and cron, restarts independently, and can run on the Mac where the logged-in Chrome profile lives while the daemon runs on the Pi. Protocol: JSON-RPC over stdio, worker spawned on first browser tool call, killed after an idle timeout.

## 5. Sources

- chromiumoxide: https://crates.io/crates/chromiumoxide (0.9.1, 2026-02-25)
- rmcp (official Rust MCP SDK): https://crates.io/crates/rmcp and https://github.com/modelcontextprotocol/rust-sdk (3.2.0, 2026-08-31)
- rusqlite: https://crates.io/crates/rusqlite (0.40.2, 2026-08-08)
- ratatui: https://crates.io/crates/ratatui (0.30.2, 2026-06-19)
- croner-rust: https://crates.io/crates/croner (4.0.0, 2026-08-31)
- keyring: https://crates.io/crates/keyring (4.2.0, 2026-08-29)
- sd-notify: https://crates.io/crates/sd-notify (0.5.0, 2026-03-09)
- enigo: https://crates.io/crates/enigo (0.6.1, 2025-08-28); xcap: https://crates.io/crates/xcap (0.9.8, 2026-08-01)
- google-gmail1: https://crates.io/crates/google-gmail1 (7.0.0, 2026-01-01)
- No official Anthropic Rust SDK: https://github.com/anthropics/anthropic-sdk-python/issues/1559
- Go MCP SDK: https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0 and v1.0.0 compatibility note https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.0.0
- coder/websocket: https://pkg.go.dev/github.com/coder/websocket (v1.8.15, 2026-06-15); background https://coder.com/blog/websocket and golang/net re-recommendation of gorilla https://github.com/golang/net/commit/2914f46773171f4fa13e276df1135bafef677801
- modernc.org/sqlite: https://pkg.go.dev/modernc.org/sqlite (v1.58.0, 2026-09-01)
- robfig/cron: https://pkg.go.dev/github.com/robfig/cron/v3 (v3.0.1, 2020-01-04)
- chromedp: https://pkg.go.dev/github.com/chromedp/chromedp (v0.16.0, 2026-07-14); rod: https://pkg.go.dev/github.com/go-rod/rod (v0.116.2, 2024-07-12)
- robotgo: https://github.com/go-vgo/robotgo and https://pkg.go.dev/github.com/go-vgo/robotgo
- Anthropic Go SDK: https://github.com/anthropics/anthropic-sdk-go
- Bubble Tea v2 / Ratatui comparison: https://www.glukhov.org/developer-tools/comparisons/tui-frameworks-bubbletea-go-vs-ratatui-rust/
- OpenTUI: https://github.com/anomalyco/opentui and https://www.npmjs.com/package/@opentui/core (0.5.10; platform packages incl. linux-arm64)
- croner (npm): https://www.npmjs.com/package/croner (10.0.1)
- @trycua/cua-driver: https://www.npmjs.com/package/@trycua/cua-driver (0.23.2, 2026-08-31) and https://github.com/trycua/cua/tree/main/libs/cua-driver
- @napi-rs/keyring: https://www.npmjs.com/package/@napi-rs/keyring (2.0.0, 2026-08-31)
- nut.js pricing (core OSS, plugins paid): https://nutjs.dev/
- Bun single-file executables and cross-compile targets: https://bun.com/docs/bundler/executables
- Bun 1.4 release notes (Playwright support, 2026-08-20): https://bun.com/blog/bun-v1.4
- Playwright-on-Bun history: https://github.com/oven-sh/bun/issues/9911 (connectOverCDP, opened 2024-04), https://github.com/microsoft/playwright/pull/34546 (closed unmerged 2025-02-03), https://github.com/oven-sh/bun/issues/15679 (chromium.launch hang), https://github.com/oven-sh/bun/pull/35417 (stdio-pipe regression tests; fix in #31829)
- Pi 5 Geekbench 6: https://www.raspberrypi.com/news/benchmarking-raspberry-pi-5/ ; M2 Max: https://browser.geekbench.com/processors/apple-m2-max ; Rust build on CM5: https://tinycomputers.io/posts/rust-compilation-performance-benchmark-report.html
