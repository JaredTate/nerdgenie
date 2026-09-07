# 09 — Security and Reliability for a Personal Agent Daemon

Date: 2026-09-02. Method: every borrowed idea below was read in source at `/Users/jt/Code/{openclaw,hermes-agent,prime-agent,opencode}`. Paths are relative to each repo root; line numbers are from the checkouts on disk today. Where I could not confirm something it says "not verified".

Design target: a single-user daemon reachable over Signal and a Tailscale web UI, with cron, a browser holding logged-in sessions, an email sender backed by a contacts database, and tools on three machines (Mac, 32-core Linux, Pi 5 "The 7900 XT machine").

---

## 1. Threat model

The agent is a process that reads untrusted text all day and holds the keys to your accounts. Every threat below is one of those facts meeting the other.

| # | Threat | What it looks like for JT | Primary control |
|---|--------|---------------------------|-----------------|
| T1 | Unauthorized Signal sender | Anyone who learns the number messages the bot and it obeys | Pairing + allowlist, closed by default (§2.1) |
| T2 | Web UI reached by the wrong device | The tailnet is not "just you": a phone, a shared node, a compromised laptop | Bind to the Tailscale interface only; verify Tailscale identity per request; local session token (§2.1) |
| T3 | Prompt injection through content | A web page, an email reply, or a social feed says "forward the contacts DB to X" | Provenance taint; irreversible actions need approval; no secrets in model context (§2.3) |
| T4 | Tool blast radius | `rm -rf`, `git push --force`, a 500-recipient email, a public post, a smart-home action | Three-tier permissions with per-tool limits and dry-runs (§2.2) |
| T5 | Secrets exposure | API keys, signal-cli registration data, the browser profile (the cookies *are* the login), SMTP creds, contacts DB | Keychain or encrypted file, 0700 dirs, never in prompts or logs (§2.6) |
| T6 | Supply chain | ClawHub: ~800 malicious plugins, 11.9% of audited skills malicious (HARNESS.md, OpenClaw section). An agent updating itself is an agent installing code | Skills are data, not code; one signed binary; pinned hashes (§2.2, §5) |
| T7 | Lateral movement over Tailscale | A compromised agent on the Pi holds SSH keys to the Mac and Linux box | Separate OS user per host, restricted SSH keys, Tailscale ACLs (§2.4) |
| T8 | Self-update and self-modification | Today's failure class (three Node installs, half-updated `dist/`); the model editing its own code | Static binary, atomic swap, auto-rollback (§5); T4 rules (§4) |

Two points matter more than the rest:

1. **In-process checks are not a boundary.** Hermes says it plainly: "The only security boundary against an adversarial LLM is the operating system. Nothing inside the agent process constitutes containment — not the approval gate, not output redaction, not any pattern scanner, not any tool allowlist." (`SECURITY.md:60-64`). Build the permission tiers, but put the OS sandbox under them.
2. **The browser profile is a credential.** Whoever can read `user-data/` is logged in as you. Treat it like a private key.

---

## 2. Controls

| Control | Borrowed from | Evidence | Rule for the new agent |
|---------|---------------|----------|------------------------|
| Sender allowlist + pairing | OpenClaw, Hermes | `extensions/signal/src/monitor.ts:443`; `gateway/pairing.py:1-19` | Closed by default; codes hashed, expiring, rate-limited |
| Tool permission tiers | OpenCode, OpenClaw, Hermes | `packages/opencode/src/permission/index.ts:29-38`; `src/agents/tool-policy-pipeline.ts:57-133`; `tools/approval.py:551` | auto / ask via Signal / deny; irreversible always asks |
| Injection provenance | OpenClaw | `packages/agent-core/src/agent-loop.ts:308,430,1735-1760` | Taint turns that saw network content; tainted turns cannot run tier-2 actions |
| External content wrapping | OpenClaw | `src/security/external-content.ts:60-70,382` | Random-ID boundaries plus a warning; strip special tokens |
| OS sandbox | Codex (via HARNESS.md), OpenClaw docker | `src/agents/sandbox/validate-sandbox-security.ts:23-52` | Seatbelt on Mac, bwrap + Landlock on Linux/Pi; always on |
| Egress policy | OpenClaw net-policy | `packages/net-policy/src/ip.ts:36-60`; `src/infra/net/ssrf.ts:495,716` | Pinned DNS, block private ranges, per-tool host allowlist |
| Secrets | OpenClaw, Hermes, OpenCode | `src/secrets/plan.ts:153-157`; `agent/credential_persistence.py:1-25`; `packages/opencode/src/auth/index.ts:78-79` | Keychain on Mac; age-encrypted file on Linux/Pi; refs not values |
| Browser profile isolation | OpenClaw | `extensions/browser/src/browser/chrome.ts:753-756` | Dedicated 0700 profile, CDP on loopback, never mounted in sandboxes |
| Audit log | OpenClaw | `src/audit/audit-event-types.ts:1-15`; `src/audit/audit-event-writer.ts:32` | Append-only SQLite, bounded queue, every tool call and approval |

### 2.1 Sender authentication and pairing

**OpenClaw.** Signal's DM policy defaults to `"pairing"` (`extensions/signal/src/monitor.ts:443`): an unknown sender gets a one-time code and nothing else, and the owner approves from the CLI. The schema refuses `dmPolicy="open"` unless `allowFrom` literally contains `"*"` (`extensions/signal/src/config-schema.ts:162`) and refuses `"allowlist"` with an empty list (`:170`). Pending requests expire after one hour and are capped at three (`src/pairing/pairing-store.ts:27-28`). Every decision returns a reason code such as `dm_policy_pairing_required` (`src/security/dm-policy-shared.ts:52-64`), which is what makes the audit log readable later.

**Hermes.** `gateway/pairing.py:1-19` states the design: 8-character codes from a 32-character unambiguous alphabet, `secrets.choice()`, 1-hour expiry, max 3 pending per platform, 1 request per user per 10 minutes, lockout after 5 failed approvals, `chmod 0600` on data files, codes never logged. Codes are stored salted-and-hashed (`:612`) and compared with `secrets.compare_digest` (`:743`). Authorization lives in a gateway mixin (`gateway/authz_mixin.py:503`, `_is_user_authorized`) that reads allowlists through a profile-scoped env layer so two profiles in one process cannot leak allowlists into each other (`:31-57`).

**Rule.** Copy Hermes' pairing constants verbatim. Store the allowlist as Signal UUIDs, not phone numbers. For the web UI: bind only to the `tailscale0` address, verify the caller with `tailscale whois` on every request, and still require a per-browser session cookie. The tailnet is a network, not a login.

### 2.2 Tool permission tiers

**OpenCode** has the cleanest engine. A rule is `{permission, pattern, action}`, `action ∈ {ask, allow, deny}` (`packages/core/src/v1/config/permission.ts:5`). `evaluate()` flattens the rulesets, takes the *last* rule whose wildcard matches both permission and pattern, and defaults to `ask` (`packages/opencode/src/permission/index.ts:29-38`). `ask()` short-circuits on any `deny` with a `DeniedError` that carries the matching rules back to the model (`:70-76`; message at `packages/core/src/v1/permission.ts:21-26`); otherwise it publishes `Event.Asked` and awaits a `Deferred` (`:79-97`). `reply()` resolves it; a reject cascades to every pending request in the session (`:118-130`); "always" appends an `allow` rule and auto-resolves anything it now covers (`:135-154`). The prompt reaches clients as an event with once / always / reject (`acp/permission.ts:20-24`) and the answer returns over HTTP (`server/routes/instance/httpapi/handlers/permission.ts:16-23`). Shell commands are reduced to a "human-understandable prefix" with a hand-built arity table (`permission/arity.ts:1-9`) so `git push origin main` matches a rule for `git push`.

**OpenClaw** stacks eight named allowlist layers — profile, provider profile, global, global-by-provider, per-agent, per-agent-by-provider, group, per-sender (`src/agents/tool-policy-pipeline.ts:57-133`) — plus sandbox and sub-agent layers (`src/agents/agent-tools.policy.ts:143,343`). Layers are intersected, not merged (`src/agents/tool-policy-shared.ts:26-54`). Sub-agents can never reach `gateway`, `message`, `sessions_send` and friends regardless of config (`agent-tools.policy.ts:52-66`). Exec has `mode: deny | allowlist | ask | auto | full` and `ask: off | on-miss | always` (`src/config/types.tools.ts:287-295`). A denied approval tells the model "This denial is final … Do not run the tool call again" (`src/agents/agent-tools.before-tool-call.approval.ts:42-55`); approval timeouts are clamped (`:58-66`).

**Hermes** has four in-process layers: a regex gate with 12 "hardline" patterns that cannot be approved at all and 47 "dangerous" patterns that prompt (`tools/approval.py:551,608`); write-denied paths such as `~/.ssh/authorized_keys` and the profile `.env` (`agent/file_safety.py:28-50`); a guard that refuses git operations which would rewrite the running checkout (`tools/self_repo_guard.py:1-30`); and Tirith, an external pre-exec scanner auto-installed with SHA-256 verification (`tools/tirith_security.py:1-21`). Answers are once / session / always / deny (`tools/approval.py:2829`). Two details to copy: YOLO mode is frozen at import so a skill cannot flip it mid-process (`:35-37`), and unattended surfaces never block on a prompt — they follow `approvals.unattended_mode`, default deny (`:268-273`). One detail not to copy: an auxiliary LLM may auto-approve "low-risk" commands (`:3641-3657`). That is a second model judging text from a possibly-injected first model; it moves risk rather than removing it.

**Proposed three-tier model.** Rules are last-match-wins like OpenCode. Every tool declares a class: `read`, `write`, `exec`, `network`, `irreversible`. Irreversible means an effect a human cannot undo from the keyboard: send an email, post publicly, push to a shared branch, change a smart-home state, delete outside scratch.

```yaml
# ~/.agent/policy.yaml
default: ask                      # unknown tool or pattern -> ask over Signal

allow:
  - read:*
  - exec: "git status|git diff|git log*|ls*|rg*|cat*"
  - network: "GET https://*.github.com/*"
  - write: "~/agent/scratch/**"

ask:                              # Signal: "Approve? [once] [1h] [always]"
  - exec: "*"
  - write: "~/Code/**"
  - network: "POST *"

deny:                             # hard stop; the model gets the rule text back
  - exec: "rm -rf /*|sudo *|curl * | sh|*.ssh/*"
  - write: "~/.ssh/**|~/.agent/secrets/**|~/.agent/browser/**"
  - irreversible: "smart_home.unlock*"

irreversible:                     # ALWAYS ask, unless a standing approval matches
  always_ask: true
  tainted_turn: deny              # turn saw network content -> do not even ask
  standing_approvals:
    - skill: sales-outreach
      action: email.send
      limits: { per_run: 20, per_day: 50, to: "contacts.db:tier=warm", template: "outreach-v3" }
      dry_run_first: true         # first run of the day shows the list, sends nothing
      expires: 2026-12-31
    - skill: social-poster
      action: browser.post
      limits: { per_day: 3, accounts: ["x:@jt"], requires_preview: true }
```

Rules that follow from the evidence: `tainted_turn: deny` is the line OpenClaw's taint tracking lacks (§2.3). Standing approvals are scoped to one skill, one action, hard numeric limits, and an expiry. A Signal approval grants `once` or `1h`, never `always`, for irreversible actions. Bulk actions dry-run first and print the exact recipient list and template hash. Cron runs are unattended: anything at `ask` fails closed and leaves a Signal message, like Hermes' unattended mode.

### 2.3 Prompt-injection defenses

**What OpenClaw's taint tracking actually does.** Tools declare `resultContentSource: "network"` (`packages/agent-core/src/types.ts:557-558`); `web_fetch`, `web_search`, and executing MCP tools do (`src/agents/tools/web-fetch.ts:973`, `web-search.ts:100`, `src/agents/agent-bundle-mcp-materialize.ts:215`). In the loop, `turnTainted` starts from history (`agent-loop.ts:308`), becomes true when any tool result in a batch is network-sourced (`:430`), and resets only on a new user message (`:378`). The final assistant message is stamped `__openclaw.turnTainted` (`:622-624`, `:1752-1760`). The consumers are memory, not tools: a tainted assistant message is classified `"untrusted"` origin (`packages/memory-host-sdk/src/host/session-provenance.ts:11-16`), and the memory-flush run starts tainted if the sender is not the owner or the turn was tainted (`src/auto-reply/reply/agent-runner-memory.ts:1568-1569`). I found no path where taint blocks a tool call. It is provenance, not enforcement. Keep the provenance; add the enforcement.

**Content wrapping.** `src/security/external-content.ts` wraps untrusted text in `<<<EXTERNAL_UNTRUSTED_CONTENT id="…">>>` markers with a random 16-hex id per wrap so injected text cannot forge the closing marker (`:60-70`, `:382`), prepends a security notice (`:78-79`), logs suspicious-pattern matches (`:23-40`), and strips model special tokens (`:292`). Cheap; copy it exactly.

**Hermes.** `gateway/response_filters.py` is not a security filter; it only decides whether a `NO_REPLY` / `[SILENT]` marker suppresses delivery (`:1-6`). Hermes' real posture is the OS-boundary statement (`SECURITY.md:60-64`) plus env scrubbing for subprocesses (`SECURITY.md:118-125`).

**Rules.** Every tool result carries a source: `owner`, `local`, `network`, `email`, `social`. A turn is tainted once any non-owner source appears, until the owner types again. Tainted turns cannot execute tier-2 or irreversible actions; the agent summarizes and stops. Wrap all external content with random-id boundaries. Never put a secret value in context; tools resolve secrets by reference at call time.

### 2.4 OS sandboxing

**What exists.** OpenClaw's sandbox is Docker/Podman and defaults to `"off"` (`src/agents/sandbox/config.ts:248`). When on, it refuses to mount `/etc`, `/proc`, `/run`, the Docker socket, `~/.ssh`, `~/.aws`, `~/.gnupg`, `~/.config` (`validate-sandbox-security.ts:23-49`) and rejects `seccomp=unconfined` / `apparmor=unconfined` (`:51-52`). Inside it, `browser`, `computer`, `nodes`, and channel tools are denied (`constants.ts:43-50`). `extensions/crabbox` is a cloud worker provider driven by the Crabbox CLI (`extensions/crabbox/index.ts:9-13`), not a local sandbox. There is no Seatbelt or bubblewrap in OpenClaw core; the only hits are the Codex extension relaying to Codex's own sandbox (`extensions/codex/src/app-server/sandbox-exec-server.ts`). **OpenCode has no OS sandbox**: a search of `packages/opencode/src` for `sandbox-exec`, `bwrap`, `seatbelt`, `landlock`, `seccomp` returns nothing. Hermes documents two postures — a terminal-backend container that confines only shell and file tools, and whole-process wrapping (its Docker image or NVIDIA OpenShell), "the supported posture when the agent ingests content from surfaces the operator does not control" (`SECURITY.md:70-110`). Codex's Seatbelt / bubblewrap+seccomp design is from HARNESS.md Part 3.5; the Codex repo was not in scope, so not re-verified.

**Recommendation for a compiled daemon.** Run every `exec` and every file-writing tool in a child process under an OS sandbox, always, with the profile generated from `policy.yaml`:

- **Mac:** `sandbox-exec -p <profile>` (Seatbelt). Deprecated in name, still shipped, used by Codex and Chrome. Deny by default; allow read of the allowed roots; allow write of scratch plus explicit paths; deny network unless the tool class is `network`; deny `~/.agent/secrets`, `~/.agent/browser`, `~/.ssh`.
- **Linux and Pi:** `bwrap --unshare-all --new-session` with `--ro-bind` of allowed roots and `--bind` of scratch, no `/run` or docker socket, plus Landlock (Pi OS Bookworm's 6.x kernel has it) for path rules and a seccomp filter blocking `ptrace`, `mount`, `keyctl`. Fall back to `systemd-run --user -p ProtectHome=read-only …` if bwrap is missing.
- The daemon runs as its own OS user (`agent`) with no sudo; the browser runs as that user too. The Pi's home-command-center files stay owned by `jt`; the agent gets a read-only bind of what it needs.
- The sandbox is not a config flag. If the sandbox binary is missing, `exec` is unavailable and the daemon says so at startup.

### 2.5 Network egress policy

`packages/net-policy` is small on purpose: IP parsing, private/special-range predicates, URL protocol checks, user-info stripping, and sensitive-URL redaction (`src/index.ts:1-7`). Blocked IPv4 ranges are `unspecified, broadcast, multicast, linkLocal, loopback, carrierGradeNat, private, reserved` (`src/ip.ts:36-45`), with IPv6 equivalents and embedded-IPv4 extraction (`:326`). `src/infra/net/ssrf.ts` turns that into a **pinned lookup** — resolve once, check every address, dial only the checked address so DNS cannot rebind mid-request (`:495`, `:716`) — and `assertPublicHostname` (`:809`). It is used by media fetch, pairing setup, proxy validation, and plugin management. This is SSRF protection for fetches, not a general egress firewall.

**Rule.** Copy the pinned lookup for every HTTP tool. Add what OpenClaw lacks: a per-tool host allowlist and a default of no network for sandboxed children (bwrap `--unshare-net`; Seatbelt `(deny network*)`), with the daemon proxying approved requests.

### 2.6 Secrets

- **OpenClaw** stores references, not values: a `SecretRef` has `source ∈ {env, file, exec}` plus a provider alias (`src/secrets/plan.ts:153-157`); `exec` refs are skipped in dry-run audits (`src/secrets/exec-resolution-policy.ts:9-24`). The 1Password extension is a resolver *and* a broker with per-agent standing grants and a SQLite audit trail; grant overflow evicts oldest, which is fail-closed (`extensions/onepassword/index.ts:19-23,46-53`). Vault is a resolver only (`extensions/vault/index.ts:3-7`). The auth-profile store can prompt the macOS keychain (`src/commands/doctor-auth.ts:78`, `allowKeychainPrompt`); the read path was not traced — not verified.
- **Hermes** keeps `auth.json` but strips "borrowed" credentials (Claude Code, gh CLI, Qwen CLI tokens) at the disk boundary (`agent/credential_persistence.py:1-25`; sources at `agent/credential_sources.py:1-15`). The file is created atomically with `O_EXCL` + `0o600` (`hermes_cli/auth.py:1441-1486`). No keychain.
- **OpenCode** is a plain `auth.json` written with `0o600` (`packages/opencode/src/auth/index.ts:12,78-79`), with an env override (`:60-64`). No keychain.

**Recommendation: keychain on Mac, encrypted file elsewhere, references everywhere.** Config holds `secret://anthropic/api-key`, never a value. On the Mac the resolver is the login keychain (Security framework from the binary). On Linux and the Pi there is no reliable keychain for a headless user, so use an `age`-encrypted file whose key is a `0600` file owned by `agent` — that is what a keyring does anyway, minus D-Bus. Signal-cli's registration data and the browser profile are secrets too: `0700`, excluded from every sandbox mount and from unencrypted backups. Redact secret-shaped strings from logs (OpenClaw: `src/security/secret-mask.ts`).

### 2.7 Browser-profile safety

OpenClaw's managed profile lives at `<CONFIG_DIR>/browser/<profile>/user-data` (`extensions/browser/src/browser/chrome.ts:753-756`); the tool description calls it "the isolated OpenClaw-managed `openclaw` browser" (`browser-tool-description.ts:15`). CDP binds to `127.0.0.1` (`chrome.ts:758-759`). Stale `Singleton*` locks are removed before launch (`:472`); a locked profile gives a clear error (`:710`). "Reset" moves the profile to the trash rather than deleting it (`browser/server-context.reset.ts:43-59`). The security audit flags remote CDP exposure and the legacy relay-auth window (`security-audit.ts:1-60`); the sandbox image contract was bumped for "cdp-relay-auth" (`src/agents/sandbox/constants.ts:57-58`). The two directories on JT's box (`~/.openclaw/hr-infra-chrome`, `~/.config/openclaw/chrome-openclaw`) do not appear as strings in this checkout — user-configured `userDataDir` values or an older layout; not verified.

**Rules.** One dedicated Chromium profile per persona under `~/.agent/browser/<name>/`, `0700`, never your personal profile. CDP on loopback with a per-launch token. The profile directory is on the sandbox deny list. `browser.post` / `browser.submit` are class `irreversible`. Screenshots entering the context are external content and get wrapped.

### 2.8 Audit log

OpenClaw's audit is a versioned, metadata-only contract: kinds `agent_run | tool_action | message`, statuses `started … blocked` (`src/audit/audit-event-types.ts:1-15`). Writes go through a non-blocking queue capped at 4,096 events with lock retry and hourly pruning (`src/audit/audit-event-writer.ts:1,32-37`). Message auditing is `off`, `direct` only, or all (`src/audit/audit-recorder.ts:53-66`). Identities are pseudonymized per database (`src/audit/audit-event-store.ts:36-39`).

**Rule.** Same shape: SQLite, WAL, append-only, one row per inbound message, tool call, permission decision (with the matching rule), approval reply (who, when, once/1h), and irreversible action (with payload hash). Bounded queue; if it is full the tool call waits rather than skipping the log.

---

## 3. Reliability machinery

| Mechanism | Problem | How the source does it | File:line | Rule for the new agent |
|-----------|---------|------------------------|-----------|------------------------|
| Supervisor (launchd) | Process dies; nobody restarts it | `RunAtLoad`, `KeepAlive=true`, `ExitTimeOut=20`, `Umask=077`, 10 s `ThrottleInterval` so crash loops back off | `src/daemon/launchd-plist.ts:6-9,342` | Same plist; stdout/err to a rotated log |
| Supervisor (systemd) | Same, on Linux/Pi | `Restart=always`, `RestartSec=5`, `StartLimitBurst=5`, `StartLimitIntervalSec=60`, `TimeoutStopSec=330`, `KillMode=control-group` | `src/daemon/systemd-unit.ts:77-97` | Add `Type=notify` and `WatchdogSec=60`; OpenClaw's unit has no watchdog line |
| sd_notify | systemd cannot tell "up" from "wedged" | Non-blocking datagram: `READY=1` after startup, `WATCHDOG=1` on a timer, `STOPPING=1` once | `gateway/systemd_notify.py:17-40,123,137,175` | Feed the watchdog only from the main event loop |
| Exit-code contract | Supervisor restarts a fatal misconfig forever | Exit 75 = restart me; exit 78 = fatal config, stop | `gateway/restart.py:9-16` | Same codes; `RestartPreventExitStatus=78` |
| Readiness probe | Health checks that write can corrupt what they check | Opens `state.db` read-only with `PRAGMA query_only`, closes explicitly; checks disk and config parse | `gateway/readiness.py:27-46,61,103` | `/readyz` = DB ro-open + config + disk + Signal socket; `/healthz` = heartbeat age |
| Post-restart health | Update says "done" while the service is dead | After restart, probe port and HTTP readiness for N attempts | `src/cli/daemon-cli/restart-health.ts:328` | Updater blocks on `/readyz` and rolls back on failure (§5) |
| Startup watchdog | Process wedges before the event loop exists; supervisor sees a live PID | Stdlib-only OS thread armed at process entry, before the gateway package imports; on deadline dumps thread stacks, records the exit, `os._exit(75)`. Long migrations declare a progress lease | `hermes_startup_watchdog.py:1-30,236-283` | Arm a 300 s startup deadline in `main()` before any I/O; migrations extend it explicitly |
| Loop-liveness watchdog | Event loop freezes; every async recovery is stuck with it | OS thread probes the loop every 30 s, 10 s timeout, 3 strikes → stack dump + `os._exit`; heartbeat file lets outside tooling see "alive but frozen" | `gateway/shutdown_watchdog.py:1-22,44-53,202` | One non-async thread that can kill the process; heartbeat file for the Pi dashboard |
| Shutdown watchdog | Graceful drain hangs; service never restarts | Armed at `stop()`, deadline = drain timeout + 60 s, then hard exit | `gateway/shutdown_watchdog.py:11-14,44` | Same; `TimeoutStopSec` must exceed it |
| Crash-loop breaker | Auto-resumed session re-runs the thing that kills the daemon | Boot timestamps in `restart_loop.json`; 3 restart-interrupted boots within a 300 s gap → skip auto-resume, still serve new messages; fails open | `gateway/restart_loop_guard.py:1-30,47-65,182` | Copy it; also pause a cron job that crashed the daemon twice |
| Turn inactivity timeout | A turn hangs forever | Gateway abandons a turn after 1800 s idle; long tools send progress | `gateway/run.py:3962`; `agent/tool_executor.py:539-554` | Per-turn wall clock 15 min; per-tool 7 min (Hermes: 420 s, `tool_executor.py:128`) |
| Stall notification | User sent a follow-up; nothing moves | Notify once when pending inbound exists and idle ≥ timeout; clear only on real progress | `gateway/session_stall.py:27-60` | One Signal message "still working, 5 min idle", never repeated |
| Turn lease | Two routes map to one session and interleave writes | Per-session lease, identity-checked release, fail-closed on timeout, registry capped at 512 | `gateway/turn_lease.py:1-40,61,75` | One lease per session; a timed-out waiter is rejected with "resend" |
| Max turns | Inner loop runs until the model stops calling tools | OpenClaw has no cap (grep of `agent-loop.ts` for `maxTurns`/`maxIterations` is empty; HARNESS.md agrees) | `packages/agent-core/src/agent-loop.ts` | Hard cap of 50 tool calls per turn; hitting it is a logged failure |
| Delivery ledger | Reply generated, process dies before Signal ACK | Rows `pending → attempting → delivered/failed` in `state.db`; on boot, sweep rows owned by a dead pid; `attempting` rows are resent with a visible "may be a duplicate" marker (honest at-least-once); 3 attempts, 24 h stale, then `abandoned` | `gateway/delivery_ledger.py:1-45,62-66,224-269,309` | Same three checkpoints around every Signal send; obligation id = hash(session, ref, content) |
| Lifecycle ledger | SIGKILL / OOM leaves no evidence | Sentinel `phase=running`; if found at boot, log an unclean-exit record with the last heartbeat's memory sample, then `PRAGMA quick_check(1)` (only after unclean exits, ~2 s on 500 MB) | `gateway/lifecycle_ledger.py:1-40,183,226-262` | Same sentinel; quick_check after unclean exit; full `integrity_check` weekly |
| SQLite integrity + recovery | Corrupt DB takes the daemon down for good | OpenClaw: `integrity_check` + FK check, distinguishing terminal corruption from transient locks. Hermes: failed opens retry 1 s → 60 s and publish `session_store: retrying` | `src/infra/sqlite-integrity.ts:34-54`; `gateway/session_db_recovery.py:12-14,34-52` | On terminal corruption: rename to `.corrupt-<ts>`, restore nightly backup, start degraded, tell the owner |
| Bounded queues | Unbounded buffers eat the Pi's RAM | TTS queue `maxsize=256`; audit queue 4,096; 16 pending tool calls in code mode. Hermes' main stream queue is unbounded (`stream_consumer.py:272`) | `gateway/streaming_tts_consumer.py:91`; `src/audit/audit-event-writer.ts:32`; `src/agents/code-mode-runtime.ts:25` | Every queue has a cap and an overflow policy; inbound backlog caps at 100 |
| Drain control | "Stop taking turns, finish current ones" | Marker file stamped with the boot epoch and a max age, so an orphaned marker cannot park a fresh boot in `draining` (it did: 52 min, then 3 days) | `gateway/drain_control.py:1-60` | `agentctl drain` writes the marker with boot id; ignore markers from a previous boot or older than 30 min |
| Shutdown flush | Messages held in memory because the DB write failed are lost | Serialize to `pending_messages/*.json` with fsync of file and dir before `clear()`; re-insert on boot | `gateway/shutdown_flush.py:1-25,62-80` | Never `clear()` anything undelivered without a disk copy |
| Orphan cleanup | Tool subprocesses outlive the daemon and block restart | `ExecStopPost=` SIGKILLs every PID left in the cgroup, per-PID | `gateway/cgroup_cleanup.py:1-13,53-72`; `systemd-unit.ts:97` | Same script on Linux/Pi; on Mac, own process group per tool and kill the group on exit |

---

## 4. "Self-fixing", in tiers

"Self-fixing" needs a definition or it becomes a marketing word.

| Tier | Meaning | Realistic? | Evidence |
|------|---------|------------|----------|
| **T0** | Supervisor restarts a dead process | Yes, table stakes | `launchd-plist.ts:342`; `systemd-unit.ts:82-83` |
| **T1** | Automatic state repair: DB check, stale lock release, orphan kill, pending-message recovery, crash-loop breaker | Yes; this is where most of the reliability comes from | `lifecycle_ledger.py:226`; `chrome.ts:472`; `cgroup_cleanup.py:53`; `shutdown_flush.py:1-25`; `restart_loop_guard.py:182`; OpenClaw `doctor-state-integrity.ts:1013` (repairs behind a confirm prompt) |
| **T2** | Automatic rollback to the last good binary after a failed update or crash loop | Yes, if the binary is one file (§5). OpenClaw rolls back only git checkouts (`src/infra/update-runner-git.ts:143-221`); package installs have nothing | Hermes verifies Tirith downloads with SHA-256 and optional cosign (`tools/tirith_security.py:13-20`); apply the same to the daemon's own updates |
| **T3** | Model-assisted diagnosis: reads its own logs, proposes a fix, asks over Signal | Yes, with read-only tools, a fixed menu of repair commands, always asking first | OpenClaw already points a failed update at `openclaw triage`, "a coding agent that can diagnose and repair the installation" (`src/cli/update-cli/update-recovery-guidance.ts:9-18`). That is T3 on a broken install with full tool access; prefer a narrower version |
| **T4** | The model edits and rebuilds its own code | Rarely, never unattended | Below |

**When T4 is acceptable.** Only when all of these hold:

1. The change is made in a git worktree, never the running checkout. Hermes' `self_repo_guard.py:1-30` exists because an agent doing `git checkout` on its own running tree causes module-version skew mid-process. A compiled binary is immune to that particular failure but not the general one.
2. The full test suite passes in CI, not on the agent's own machine.
3. A canary: the new binary runs alongside the old one on one host (the Linux box, never the Pi) for a fixed soak window, serving only a test Signal group.
4. Rollback is automatic and does not require the model.
5. A human approves the merge over Signal with the diff summary and test output attached. No standing approval covers T4.

**Why.** HARNESS.md Part 6 records that in Prime's own Factorio case study "the self-improvement loop found cheating exploits and then optimized for them", and that one self-improving harness "improved itself into needing 300k tokens to beat a human who just looked at it". A self-modifying agent optimizes whatever it is scored on, including the score. Hermes gates even memory and skill writes behind an approval store because its background-review fork kept saving "wrong assumptions" (`tools/write_approval.py:1-40`). If a daemon should not freely rewrite its notes, it should not freely rewrite its code.

---

## 5. Update strategy that cannot produce today's failure

### What happened today

| Symptom | Code path | Root cause |
|---------|-----------|------------|
| systemd user dir "unsafe-permissions", `SERVICE_DEFINITION_UNKNOWN` | `src/daemon/systemd-definition-mutation.ts:151-152` refuses any unit file or parent dir with group/other write bits (`mode & 0o022`); `service-types.ts:146` maps it to `SERVICE_DEFINITION_UNKNOWN`; `daemon-cli/install.ts:200` reports it | The updater must *edit* a systemd unit it does not fully own; a host-filesystem check blocked it |
| "Current Node (nvm v24.18.0) differs from the managed gateway service Node (/usr/bin/node)" | `src/cli/update-cli/update-command.ts:393-400` | Service and shell run different Nodes; OpenClaw copes by "using the managed service Node for this update". The runtime is a host dependency the app does not control |
| `ERR_MODULE_NOT_FOUND … dist/doctor-health-Bn9fZuuK.js` | `src/gateway/stale-install.ts:23-28` classifies exactly this | tsdown emits content-hashed chunk names; a partially replaced `dist/` points at a chunk from another build. Many files, no atomic swap |
| "Update refused: package manager owner is unknown" | `src/cli/update-cli/shared.ts:379-396` | With three Node installs, the updater could not tell which of npm/pnpm/bun owns the global install, so it refused (correctly) |

All four follow from one choice: the app is a tree of files run by a runtime it does not ship, installed by a package manager it does not control, into service definitions it must edit in place.

### The rule set

| Property | OpenClaw today | New daemon |
|----------|----------------|------------|
| Artifact | npm package: thousands of files under `dist/` plus `node_modules`, run by whatever `node` the service found | One static binary per platform (`agentd-darwin-arm64`, `agentd-linux-arm64`, `agentd-linux-amd64`); no Node, Python, or JVM consulted |
| Who installs | npm / pnpm / bun global shim, which must be detected (`shared.ts:379`) | The binary installs itself: `agentd update` |
| Versioning | npm tags and channels (`update-command.ts:16-22`) | Semver git tags; a signed `manifest.json` per release with SHA-256 per artifact |
| Download and verify | npm's own integrity | Download to `releases/<version>/agentd.partial`; verify SHA-256 and a minisign/cosign signature against a key baked into the current binary; `fsync` |
| Install | Package manager rewrites `dist/` in place | `rename(2)` the verified file into place, then `rename` the `current` symlink. Both atomic; no half-installed state exists |
| Keep previous | No for package installs; git checkouts only (`update-runner-git.ts:143`) | Keep the last 3 `releases/`; rollback is `ln -sfn` and restart |
| Service definition | Rendered per platform and edited during update (`systemd-definition-mutation.ts`) | Written once at `agentd install`, pointing at the stable path `~/.agent/current/agentd`. Updates never touch it; unit `0644`, dir `0755`, so the permission check cannot fail |
| Restart | Detached restart script (`restart-helper.ts:1-5`), then health probe (`restart-health.ts:328`) | `systemctl --user restart` / `launchctl kickstart -k`; the new binary must send `READY=1` and pass `/readyz` within 60 s |
| Auto-rollback | Guidance to run `openclaw triage` (`update-recovery-guidance.ts:13`) | If `/readyz` fails, or the crash-loop breaker trips within 10 minutes of an update, an `ExecStartPre` script flips `current` back and marks the version `bad` in `releases/state.json`. No model involved |
| Config migration | Doctor runs migrations and refuses newer schemas (`doctor.refuses-newer-database-schemas.e2e.test.ts`) | Forward-only numbered migrations, each in a transaction after copying `state.db` to `backups/`. An older binary refuses a newer schema and names the version to roll to |
| Runtime dependencies | Node, optional Chrome, optional Docker, signal-cli | signal-cli and Chromium stay external (version-pinned, checked at `/readyz`); everything else is in the binary |
| Update trigger | Manual `openclaw update` or the control-plane update job | Cron checks the manifest daily, downloads and verifies, installs only in a maintenance window with the drain marker set (§3) and no turn running. The model never calls `update` |

### Why today's failure is now structurally impossible

- No runtime to mismatch: there is no "which Node" because there is no Node.
- No package manager to detect: the binary is its own installer.
- No partial tree: one file, atomic rename, or nothing.
- No unit-file edits at update time: the unit points at a symlink that never moves.
- No update without a health-checked rollback: the previous binary is one `rename` away and the supervisor performs it without asking a model.

Keep one OpenClaw habit: refuse loudly and change nothing when preconditions fail (`shared.ts:394`, "no changes were made"). The difference is that the new design has almost no preconditions left to fail.
