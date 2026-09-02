# News Since August 2026: OpenClaw, Hermes Agent, OpenCode, Prime Agent

**Compiled:** 2026-09-02
**Covers:** 2026-08-01 through 2026-09-02
**Prior report:** `/Users/jt/Desktop/HARNESS.md` (2026-08-06)

Every fact below has a source URL and a date. Where I could not confirm something, it says **not verified**.

Sources are of two kinds: **local repo clones** (`/Users/jt/Code/openclaw`, `hermes-agent`, `opencode`, `prime-agent` — read only, nothing modified) and **live web sources** (GitHub's public API, project blogs, news sites, Hacker News). The clones are the strongest evidence.

---

## 1. "OpenClaw 2.0" — is it real?

**Yes. It is a real name, but not a real version number.**

The official release notes page is titled, word for word, **"v2026.8.1 (AKA OpenClaw 2.0)"**. This is in the local clone at `/Users/jt/Code/openclaw/docs/releases/2026.8.1.md` and live at https://docs.openclaw.ai/releases/2026.8.1. The page's own keywords list includes "OpenClaw 2.0".

So JT is right. There is no `2.0.0` tag. The repo stays date-versioned. But the project itself brands v2026.8.1 as 2.0.

### The blog post behind the name

- **"OpenClaw 2.0, Accidentally"** — https://openclaw.ai/blog/openclaw-2-accidentally
- Author: **Hannes Rudolph**, Community Manager, OpenClaw Foundation
- Published: **2026-08-30**

The post's argument is that nobody set out to build a 2.0. A small effort to fix installation and improve the browser UI grew until it touched everything. Key numbers from the post:

- **933 contributors**, of whom **569 were first-time contributors**
- **"over 16,000 pull requests"**
- That is **"roughly 50% of all pull requests ever merged into OpenClaw"**
- Before this: **106 releases in 230 days**
- Then: **nearly seven weeks without shipping**

The maintainers frame the seven-week pause as a deliberate quality decision — they wanted it to work "for people starting from scratch and people upgrading an existing Claw rather than shipping quickly and handing them an update that broke what they already had."

**Note a discrepancy.** The blog says 933 contributors. The release-notes file in the repo says something different:

> **Release scale:** 16,977 pull requests, 698 direct commits, and 987 contributors.
> — `/Users/jt/Code/openclaw/docs/releases/2026.8.1.md`, line 25 area

I could not reconcile 933 vs 987. Both are official. Treat the range as "roughly 950 contributors, roughly 17,000 PRs".

### Release timeline

Tag dates: `git log -1 --format=%ci <tag>` in the local clone. Publication dates: https://github.com/openclaw/openclaw/releases. Both observed 2026-09-02; they differ by up to a day because tagging and publishing are separate steps.

| Version | Tag date (local clone) | Published (GitHub) | Scale | Headline |
|---|---|---|---|---|
| v2026.6.11 | 2026-06-29 | not verified | not verified | Reliability fixes for replies, sends, reconnects, sessions |
| v2026.7.1 | 2026-07-13 | not verified | not verified | Control UI + onboarding overhauls, iOS/Android, Gateway recovery |
| v2026.8.1-beta.1 | 2026-08-10 | not verified | — | First 2.0 beta |
| v2026.9.1-beta.1 | 2026-08-28 | 2026-08-28 (pre-release) | — | **Mislabeled.** GitHub notes this was "mistakenly published as version 2026.9.1-beta.1" — the content is actually 2026.8.1-beta.4 |
| **v2026.8.1** | 2026-08-30 | **2026-08-31** | 16,977 PRs / 698 direct commits / 987 contributors | "OpenClaw 2.0" |
| **v2026.8.2** | 2026-08-31 | **2026-09-01** | **784 PRs / 10 direct commits / 134 contributors** | Home agent, Linux desktop companion, background sessions |
| v2026.8.3 | — | **Unreleased** | — | In `CHANGELOG.md` as "(Unreleased)" |

The mislabeled `v2026.9.1-beta.1` tag is worth flagging on its own. A release-engineering slip that publishes a beta under the wrong minor version is exactly the kind of thing JT is complaining about.

**Cadence, plainly:** 106 releases in 230 days before the pause (about one every 2.2 days), then a seven-week freeze, then two stable releases one day apart.

### What v2026.8.1 actually changed

From `/Users/jt/Code/openclaw/docs/releases/2026.8.1.md`, organized into 16 sections (Installation, Web UI, Updates, Messaging, Memory, Skills, Native Apps, Models, Automations, Browser, Plugins, Security, Quality-of-Life, Other Fixes, Maintainer). The headline items:

- **Rebuilt Control UI.** The release notes say it "now feels more familiar to anyone who uses apps like ChatGPT, Claude, Gemini, or Perplexity" — conversations in the sidebar, work in the centre, no separate Overview page.
- **Sessions and transcripts moved into SQLite.** This is the breaking change. The notes carry a warning box: downgrading to a file-backed release requires restoring archived legacy transcript artifacts first, and sessions created after migration will not appear in older releases.
- **Per-session permission modes** — read-only, guarded, workspace, or full access. Full access is admin-only. Non-retroactive: existing sessions keep the old global posture.
- **Team-scoped local Secret Store** separating Protected values from Agent-readable environment values.
- **Plugin capability review** on managed external installs, bound to the specific artifact.
- **Conversation search**, sessions on paired devices and cloud workers.

Secondary reporting adds that Gateway startup dropped to about **575 ms from roughly 1.6 s**, and that three migrations need planning: sessions to SQLite, the OpenProse plugin removed, and `codex/` model routes migrated to `openai/` via `openclaw doctor --fix`. Source: https://www.open-claw.sh/blog/openclaw-2-0-whats-new and https://cellcog.ai/blog/openclaw-2-0/ (both accessed 2026-09-02). These are third-party blogs, not the project — treat the 575 ms figure as **reported, not independently verified**.

### What v2026.8.2 changed

From `/Users/jt/Code/openclaw/docs/releases/2026.8.2.md`. It is a much smaller release — 784 PRs, 134 contributors — and reads as a cleanup of 8.1:

- A **Home button** in the Control UI that opens a personal Home agent beside current work
- A **Linux desktop companion**
- **Background sessions** you can start without switching pages
- **Browser control without a running Gateway**
- **Four new Control UI themes**
- Fedora/RPM installs can keep distro-owned Node while OpenClaw picks a managed runtime meeting its SQLite requirements
- Linux Gateway install now refuses an unsafe sudo-to-root setup
- **ClawDock helpers retired** — existing users must remove the helper source line themselves

### What v2026.8.3 is adding (unreleased)

From `/Users/jt/Code/openclaw/CHANGELOG.md`, section `## 2026.8.3 (Unreleased)`:

- **Model setup capability review** on macOS and Control UI during activation (#133793)
- **Channel plugin ingress monitors** — a shared plugin SDK monitor; IRC, Synology Chat and Google Chat migrated to it
- **TUI fuzzy selectors** delegated to `pi-tui`, removing a local matcher fork
- **GPT-5.6 Ultra and runtime switching** — Sol, Terra and Luna across OpenClaw and Codex engines (#98021)
- **Meta provider** with bundled `muse-spark-1.1` (#102873)
- **Gateway host status** in Control UI Settings — host, OS, runtime, uptime, CPU, memory, disk (#100478)
- **Logbook work journal** — a disabled-by-default bundled plugin turning paired-node screen snapshots into a private timeline
- Fixes directly relevant to JT's complaints: a `ERR_MODULE_NOT_FOUND` fix for packaged `crabbox:*` commands; restoring Gateway health checks under **Bun 1.4**; fixing browser actions on **Node 24.16+**; reducing packaged cold-start memory in CLI plugin listing; a `--no-restart` fix for update/Doctor

---

## 2. OpenClaw community pain, last 60 days

All counts below come from `https://api.github.com/search/issues` queries against `repo:openclaw/openclaw`, run **2026-09-02**. Search counts are approximate by GitHub's own design.

### Volume

| Metric | Count | Query window |
|---|---|---|
| Open issues (excluding PRs) | **3,683** | as of 2026-09-02 |
| Issues opened in last 30 days | **4,803** | `created:>=2026-08-03` |
| Issues opened in last 60 days | **9,236** | `created:>=2026-07-03` |
| Open issues + PRs (`open_issues_count`) | 6,061 | as of 2026-09-02 |
| Stars / forks / watchers | 388,622 / 81,603 / 1,753 | as of 2026-09-02 |

Read that first block twice. **More issues were opened in the last 30 days (4,803) than are currently open in total (3,683).** The project is closing issues faster than it accumulates them, but the raw inflow is roughly 160 new issues per day.

### By search term

| Term | Open issues matching | Notes |
|---|---|---|
| `doctor` | **460** | biggest single cluster |
| `systemd` | **233** | |
| `stuck` | **211** | 117 of these created since 2026-07-03 |
| `hang` | **139** | |
| `"update failed"` | **64** | |
| `"memory usage"` | **21** | |
| `ERR_MODULE_NOT_FOUND` | **4** | |
| `nvm` | not verified | GitHub rate-limited the query |
| `Signal` | not verified | rate-limited |
| `slow` / `"slow startup"` | not verified | rate-limited |
| `CPU` (title match, since 2026-08-01, with update/doctor/memory) | 135 combined | |
| Title match install/upgrade/nvm/systemd/Signal, since 2026-08-10 | **51** | |

### Representative issues

All confirmed via the GitHub API on 2026-09-02. Every one of these is open.

| # | Title | Created | URL |
|---|---|---|---|
| 136383 | Control UI regression: tab initiating turn stops updating in real time | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136383 |
| 136311 | memory-core: Gateway reacquires reindex lock on every start, making the index unrepairable; **19 GB of orphaned memory-reindex-* temp DBs accumulate** | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136311 |
| 136284 | Legacy `.tmp-<uuid>` memory-core shadow files leak forever, invisible to cleanup | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136284 |
| 136203 | Windows de-DE 2026.8.2 upgrade leaves Doctor maintenance blocked and legacy workspace state behind | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136203 |
| 136175 | **Full local memory reindex saturates CPU and blocks diagnostics** | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136175 |
| 136123 | Windows (pt-BR): `doctor --fix` permanently blocked by schtasks locale parsing | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136123 |
| 136025 | Doctor restores retired plugin aliases after successful canonical update | 2026-09-02 | https://github.com/openclaw/openclaw/issues/136025 |
| 135668 | cliAgents runtime: Claude Code AskUserQuestion tool not bridged to chat channels, **session appears stuck** | 2026-09-01 | https://github.com/openclaw/openclaw/issues/135668 |
| 135502 | **Signal** and MS Teams omit plugin-command authorization | 2026-09-01 | https://github.com/openclaw/openclaw/issues/135502 |
| 135299 | Upgrade to 2026.8.1 unusable for system-npm installs with external plugins and non-root gateway user | 2026-09-01 | https://github.com/openclaw/openclaw/issues/135299 |
| 135084 | Memory-source provenance repair runs inline in first turn after upgrade — **system-prompt stage blocked ~285 s** | 2026-09-01 | https://github.com/openclaw/openclaw/issues/135084 |
| 135038 | 2026.8.1 upgrade: **gateway unstartable** (session-store + exec-approvals gates don't self-heal) | 2026-09-01 | https://github.com/openclaw/openclaw/issues/135038 |
| 134993 | **Gateway pegs one CPU core** (busy loop in filesystem discovery) after 2026.8.1 upgrade | 2026-09-01 | https://github.com/openclaw/openclaw/issues/134993 |
| 134726 | Managed update 2026.7.1-2 → 2026.8.1 fails global install verify on stock npm config | 2026-09-01 | https://github.com/openclaw/openclaw/issues/134726 |
| 134619 | **Upgrade to 8.1 Completely Breaks Existing Installation and Major Features** | 2026-09-01 | https://github.com/openclaw/openclaw/issues/134619 |
| 134616 | The upgrade of OpenClaw from 2026.7.1 to 2026.8.1 has been unsuccessful | 2026-09-01 | https://github.com/openclaw/openclaw/issues/134616 |

The pattern is unmistakable. Roughly half of these are **upgrade breakage from 2026.7.1 → 2026.8.1**, and the rest are **memory/CPU/disk resource problems** — a 19 GB temp-DB leak, a pegged CPU core, a 285-second startup block.

### Third-party release health tracker

**ClawStat.us** (https://clawstat.us/, assessed **2026-09-02**) is an independent site that answers "should you update?". Its verdict on **v2026.8.2**: *"Update with care (medium confidence)"*, with the advice that users on **v2026.7.1-2 should stay there** unless 8.2 fixes something they specifically hit. It cites "confirmed regressions — silent message loss, ssh hangs, Linux app crashes" and lists:

- **#136148** (critical) — Linux desktop app crashes with SIGABRT, blank windows
- **#136113** (high) — CLI backend loses about **44% of responses when stdout exceeds ~50 KB**
- **#136183** (high) — SSH command executor hangs during server banner wait
- **#135347** (critical) — memory reindexing corrupts shared agent database
- **#136290** (critical) — session restart recovery executes stale operations hours later

An independent site existing purely to tell people whether an OpenClaw release is safe to install is itself a data point about the project's reliability reputation.

### Hacker News

**"OpenClaw 2.0, Accidentally"** — https://news.ycombinator.com/item?id=49505310, submitted around **2026-08-31**, **148 points, 173 comments** (observed 2026-09-02).

The dominant themes were bloat, breakage, security and — notably — the contributor statistics being read as a *negative*. Representative comments:

> "It's not only that, all this complexity leads to stuff breaking with every update"

> "OpenClaw is a vibecoded mess. Updates break default installation flow all the time"

> "569 first time contributors yea i shall continue to not use openclaw"

> "Open claw: aka. open door to a remote privilege escalation potentially granting root access to your computer"

> "The feedback I got... felt very 'trust me bro, I've got this' — which made me trust it less"

Commenters also pointed at a Google Trends graph showing interest "fell off a cliff after March," with people moving to Hermes or Claude Code. I could not independently verify the Trends data — **not verified**.

### Press

**The Register**, **2026-08-31**, Brandon Vigliarolo: *"OpenClaw 2.0 pours glitter on slow-burning security dumpster fire"* — https://www.theregister.com/ai-and-ml/2026/08/31/openclaw-20-pours-glitter-on-slow-burning-security-dumpster-fire/5293492

The argument is that 2.0 spent its effort on accessibility and a ChatGPT-like browser UI while leaving security gaps open. It quotes the release notes against themselves on three points:

1. Shared cloud sessions "are not tenant isolation or a security boundary"
2. Protected credentials in the Secret Store "depend on the filesystem permissions of OpenClaw's state directory" — no encryption at rest
3. The new untrusted-code sandbox exists but is **off by default**

I verified all three claims directly against the local `docs/releases/2026.8.1.md` Security and Privacy section. They are accurate quotes.

**Reddit and Discord:** I found aggregator articles referencing Reddit sentiment but could not open primary Reddit threads from August 2026 about OpenClaw bloat. **Not verified.**

---

## 3. OpenClaw security since August

**Short version: nothing major and new appears to have landed in the August–September window. The big incidents are all from earlier in 2026.**

### New CVEs and advisories

The GitHub Security Advisories page (https://github.com/openclaw/openclaw/security/advisories, fetched 2026-09-02) shows its most recent published batch dated **2026-06-30** — ten advisories including "Plugin install commands could allow non-owner persistence" (High, GHSA-7vrr-rp4x-4g76) and "MCP loopback could expose owner-only tools to non-owner runs" (High, GHSA-52xj-c9p8-78cv).

**No advisories published in August or September 2026 were visible.**

The community-maintained tracker **joylarkin/openclaw-security-news** (https://github.com/joylarkin/openclaw-security-news) — a collection of government warnings and vendor advisories — had its **most recent entry dated 2026-07-20**. No August or September entries.

Earlier 2026 CVE record, for context (all pre-August, all patched):

| CVE | Severity | Disclosed | Patched | What |
|---|---|---|---|---|
| CVE-2026-25253 | CVSS 8.8 | 2026-01-31 | 2026.1.29 | 1-click RCE via cross-site WebSocket hijacking |
| CVE-2026-32922 | CVSS 9.9 | 2026-03-29 | not verified | Pairing token → full admin + RCE |
| CVE-2026-44112 | CVSS 9.6 | 2026-05-15 | 2026-04-23 | TOCTOU filesystem write escape ("Claw Chain", Cyera) |
| CVE-2026-44115 | CVSS 8.8 | 2026-05-15 | 2026-04-23 | Execution allowlist env-var disclosure |
| CVE-2026-44118 | CVSS 7.8 | 2026-05-15 | 2026-04-23 | MCP loopback privilege escalation |
| CVE-2026-44113 | CVSS 7.7 | 2026-05-15 | 2026-04-23 | TOCTOU filesystem read escape |

Claw Chain source: https://www.cyera.com/blog/claw-chain-cyera-research-unveil-four-chainable-vulnerabilities-in-openclaw (2026-05-15).

A **CVE-2026-41301** (auth bypass, versions 2026.3.22–2026.3.30, fixed 2026.3.31) appears in SentinelOne's vulnerability database at https://www.sentinelone.com/vulnerability-database/cve-2026-41301/. A search summary attributed it to September 2026, but the affected versions are from March. **Publication date not verified.**

A claim that OpenClaw passed **138 CVEs** in 2026 appears at https://www.betterclaw.io/blog/openclaw-security-2026. I could not verify this against NVD. **Not verified.**

### ClawHub and plugin supply chain

- **ClawHavoc** — named by **Koi Security on 2026-02-01**; Antiy CERT identified **1,184 malicious skills** (Trojan/OpenClaw.PolySkill). First malicious upload 2026-01-27, surge 2026-01-31. Registry was cut to 3,498 skills after cleanup. Payloads included the Atomic macOS Stealer variant taking browser credentials, keychains, Telegram data, SSH keys and crypto wallets. Source: https://cyberpress.org/clawhavoc-poisons-openclaws-clawhub-with-1184-malicious-skills/ (published 2026-02-19). **This is pre-August — it is context for the ~800 malicious plugins in the August HARNESS.md report, not new.**

- **Scope squatting** — **Manifold Security**, published **2026-09-02** (today): https://www.manifold.security/blog/scope-squatting-clawhub-plugins. They found **23 code-executing plugins published under the official `@openclaw/` and `@clawhub/` scopes by unaffiliated accounts**. Of 1,508 total plugins, 557 carried an owner scope, but not all scopes are ownership-verified. Examples: `@openclaw/security-gate`, `@clawhub/aisa-twitter-api`. The plugins were not malicious in the analyzed versions, but had capabilities including autonomous payment actions, host-level `git`/`gh` commands, config export, and third-party API egress.
  **Timeline:** reported to ClawHub 2026-06-17 → removed 2026-06-19 → ClawHub added a namespace-claims dispute process. **The publication is new; the finding and fix are from June.**

- A separate report of **23 ClawHub plugins abusing official org scopes** at https://cybersecuritynews.com/23-clawhub-plugins-abuse-official-org-scopes/ appears to describe the same Manifold research.

### Mitigations shipped

From the Security and Privacy section of `docs/releases/2026.8.1.md`. This is a substantial hardening pass, and it is fair to say so:

- **Plugin capability review** on managed external installs, bound to the exact artifact, asking again when an update requests more authority
- **ClawHub GitHub installs now require a full commit SHA** instead of a mutable branch or tag
- Skill security verdicts stay attached to the exact publisher and version
- **Per-session permission modes** (read-only / guarded / workspace / full); per-turn exec restrictions can tighten but never loosen
- **Durable approval records** — first valid answer settles it, reconnecting cannot revive a completed request
- **Secret Store** with Protected vs Agent-readable separation, Vault/1Password references, destination-bound substitution
- Text from search, fetch, MCP, plugins and Browser is **bounded, normalized and marked as untrusted external content** before the model sees it
- Network policy blocks unspecified and local-use NAT64 targets by default
- Contributor-controlled code is prepared inside a designated untrusted-code sandbox
- v2026.8.3 (unreleased) adds capability review to macOS and Control UI model setup

**But the release notes also state the limits candidly:** Secret Store values are **not encrypted at rest**; symlink containment is checked before the operation rather than atomically, "leaving a remaining window"; opt-in roles "limit collaboration inside one trusted OpenClaw installation rather than creating isolation between hostile tenants"; browser navigation enforcement leaves "popups, Service Workers, background requests, some redirects, and remote backends outside that boundary."

### Bans and regulators

No **new** ban or regulator statement in August–September 2026 that I could find. **Not verified.**

Prior actions, for context:
- **China** — CNCERT warning 2026-03-12 (https://www.theregister.com/2026/03/12/china_cert_openclaw_security_warning); ban on government computers and state-owned enterprises including major banks, ~2026-03-11 (Bloomberg, Tom's Hardware)
- **Singapore IMDA** — warning against unrestricted access to sensitive files and production systems, 2026-05-14 (https://theonlinecitizen.com/2026/05/14/singapore-warns-against-unrestricted-use-of-open-claw-ai-agents-on-sensitive-systems)
- **South Korea** — Kakao restricted use on work devices; Naver issued an internal ban
- **Meta/WhatsApp ban** — as recorded in the 2026-08-06 HARNESS.md

### Exposed instances

No updated exposure count published in August 2026 that I could find. **Not verified.** Earlier figures ranged widely by methodology: 40,000+ (Infosecurity, early 2026), 63,070 confirmed live by Censys on 2026-03-31, 135,000 (SecurityScorecard/Bitdefender), 220,000+ (Penligent). The spread reflects banner-matching vs application-layer fingerprinting, not a real 5x change.

---

## 4. Hermes Agent since 2026-08-06

**Repo:** https://github.com/NousResearch/hermes-agent — "The agent that grows with you". Created 2025-07-22. MIT. Primary language Python.

### Stats (GitHub API, 2026-09-02)

| Metric | Value |
|---|---|
| Stars | **239,895** |
| Forks | 49,063 |
| Watchers | 937 |
| Open issues + PRs | 38,454 |
| **Open issues only** | **12,965** |
| Issues opened since 2026-08-03 | **5,018** |
| Contributors (local `git shortlog -sn --all`) | **3,197** |
| Commits since 2026-08-03 (local) | **6,719** |

For comparison: Hermes has **3.5x more open issues than OpenClaw** (12,965 vs 3,683) on 62% of the stars.

### Releases since 2026-08-06

All from https://api.github.com/repos/NousResearch/hermes-agent/releases (fetched 2026-09-02). Tag dates cross-checked against `git log` in the local clone.

| Tag | Semver | Published | PRs rolled up | Headline |
|---|---|---|---|---|
| v2026.8.3 | v0.20.0 | 2026-08-03 | — | **"The Herald Release"** — streaming conversational voice with barge-in and wake word, Agent-to-Agent protocol (A2A v1.0), outbound webhooks, grounded research citations, desktop artifacts with live preview, plugin SDK |
| v2026.8.13 | v0.20.1 | 2026-08-13 | **~656** | "broad stabilization-and-fixes rollup spanning the desktop app, gateway platforms, installers, tool system, and provider catalogs" |
| v2026.8.16 | v0.20.2 | 2026-08-16 | ~397 | Multi-gateway Connections registry, MCP health checks and deep links, Windows update probes, Telegram DM topics |
| v2026.8.16.2 | v0.20.3 | 2026-08-17 | ~125 | MCP 2.x SDK migration with stateless protocol, bundled Bot Mode plugin, CommandCode provider plugin, subprocess Python runtime hardening |
| v2026.8.18 | v0.20.4 | 2026-08-18 | ~74 | Desktop glass/translucency, tabbed SESSIONS\|BOTS sidebar, NVIDIA SkillEvaluator Tier 1 security scanning on skill installs |
| v2026.8.19 | v0.20.5 | 2026-08-19 | ~323 | Bot Mode group-room threads, keyless web tier, CLI polish wave (fuzzy model picker, command palette), cron jobs get persistent memory |
| v2026.8.27 | v0.20.6 | 2026-08-27 | ~525 | Consent-gated real-profile browsing, 50+ vendor MCP catalog, TTL result caching |
| v2026.8.31 | v0.21.0 | 2026-08-31 | — | **"The Pantheon Release"** — Bot Mode built in with named agents, group chats, inter-agent communication; live subagent orchestration; MCP command center dashboard; agent drives the desktop browser directly |

**Eight releases in 29 days.** That is a faster cadence than OpenClaw's pre-freeze pace, with roughly 2,100 PRs rolled up across the patch releases alone.

### The TUI — and how old it is

The `ui-tui` directory in the local clone is **an Ink/React app, first committed 2026-04-02**:

```
2026-04-02 19:07:53 -0500  2ea5345a7b  feat: new tui based on ink
```

So the TypeScript TUI is **about five months old** as of 2026-09-02. Details from `ui-tui/package.json`:

- Package name `hermes-tui`, **version `0.0.1`**, marked `private: true`
- React **19.2.7**, a local `@hermes/ink` workspace package, `nanostores`, `ink-text-input`, `unicode-animations`
- Last commit 2026-09-02; **89 commits since 2026-08-01**

A five-month-old `0.0.1` private package, still changing 89 times a month, is a reasonable technical explanation for "the interface is glitchy and unreliable."

### TUI / interface issue themes

GitHub API, `repo:NousResearch/hermes-agent is:issue is:open in:title TUI created:>=2026-08-01`, run 2026-09-02: **120 open issues** with "TUI" in the title, opened in the last month alone.

| # | Title | Created |
|---|---|---|
| 100842 | TUI input box height too small in Hermes Teal (Large) theme | 2026-09-02 |
| 100747 | Windows: Ctrl+C is a no-op once the TUI unwinds | 2026-09-01 |
| 100537 | TUI gateway doesn't support per-prompt system-level context | 2026-09-01 |
| 100167 | **TUI client heartbeat dead-timeout drops slow-model generations** | 2026-09-01 |
| 99831 | TUI `/hatch` fails after 45 s slash-worker timeout | 2026-08-31 |
| 99788 | TUI Gateway Status panel reports Signal/A2A as 'Not configured' | 2026-08-31 |
| 99773 | Classic CLI looks noisy vs omp; TUI composer + tool cards | 2026-08-31 |
| 99436 | TUI kanban poller drops review_requested/changes_requested | 2026-08-31 |
| 99388 | **TUI SIGSEGV crash during streaming (V8 code space use-after-free)** | 2026-08-31 |
| 99277 | Stray 'l' typed into composer on dashboard TUI reattach | 2026-08-31 |
| 99032 | **TUI submit silently sends collapsed paste placeholder to model** | 2026-08-31 |
| 98655 | TUI backspace deletes entire Thai grapheme cluster | 2026-08-30 |

Full list: https://github.com/NousResearch/hermes-agent/issues?q=is%3Aissue+is%3Aopen+in%3Atitle+TUI

Note the character of these: a **SIGSEGV during streaming**, a **heartbeat timeout that kills slow generations**, **stray characters typed into the composer on reattach**, and **silently sending a paste placeholder instead of the pasted text**. These are exactly the "glitchy and unreliable" symptoms JT described, and they are documented by other users.

### Direction — is the Python core being replaced?

**No.** Evidence from the local clone (2026-09-02):

- File counts by extension: **5,074 `.py`** vs 2,006 `.ts` and 816 `.tsx`. Python still dominates roughly 2:1.
- `tui_gateway/` is **entirely Python** (`entry.py`, `host_supervisor.py`, `event_publisher.py`, `methods_*.py`, …). The TypeScript TUI talks to a Python gateway.
- `cli.py` is a single **1,047,067-byte** file. `AGENTS.md` is 95,906 bytes. `cli-config.yaml.example` is 113,427 bytes.
- **`/Users/jt/Code/hermes-agent/native/`** contains exactly one thing: **`fts5_cjk/`** — a small C SQLite FTS5 tokenizer for Chinese/Japanese/Korean full-text search (`build.sh`, `fts5_cjk.c` at 9,634 bytes, a README, and a `vendor/` dir). Files dated 2026-08-06 in this clone, unchanged since.
- **There is no `Cargo.toml` anywhere in the repo.** No Rust rewrite.

So `native/` is not a rewrite beachhead. It is one C extension for CJK search tokenization. The architecture is: Python core + Python gateway + a young TypeScript/Ink TUI + Electron-ish desktop apps under `apps/desktop`.

---

## 5. OpenCode

**The repo moved.** The upstream remote in the local clone is now **https://github.com/anomalyco/opencode** (previously `sst/opencode`). SST is part of Anomaly Innovations.

### Stats (GitHub API, 2026-09-02)

| Metric | Value |
|---|---|
| Stars | **203,206** |
| Forks | 26,476 |
| Watchers | 790 |
| Open issues + PRs | 5,584 |
| Created | 2025-04-30 |
| Language | TypeScript |
| License | MIT |

### Releases — v1.18.x

From https://github.com/anomalyco/opencode/releases (fetched 2026-09-02):

| Version | Published | Headline |
|---|---|---|
| v1.18.26 | 2026-09-01 | Claude 5 sessions tolerate stale thinking blocks; Bedrock GPT-5.6 accepts `none` reasoning effort |
| v1.18.25 | 2026-08-28 | Azure auth works without requiring Bun |
| v1.18.24 | 2026-08-28 | Bedrock reasoning no longer cached as unreplayable; Azure CLI sign-in via Entra ID |
| v1.18.23 | 2026-08-25 | Cloudflare AI Gateway routing for third-party providers |
| v1.18.22 | 2026-08-24 | Device login links, `textVerbosity` handling |
| v1.18.21 | 2026-08-21 | Continue on unknown finish reasons; Vertex AI REP multi-region routing |
| v1.18.20 | 2026-08-21 | Subagent tool-call failures now resumable; network retry improvements |
| v1.18.19 | 2026-08-20 | Native passthroughs for Cloudflare AI Gateway |
| v1.18.18 | 2026-08-13 | Kimi system prompt; xhigh reasoning for xAI |
| v1.18.17 | 2026-08-12 | Session compaction; PDF attachments for Copilot models |

**Ten releases in 21 days**, roughly one every two days. Crucially, look at what they are: provider fixes, auth fixes, routing fixes. Small, boring, non-breaking. Compare with OpenClaw's 17,000-PR mega-release.

### The TUI stack — an important correction

**Many published comparison articles are wrong about this.** Articles as recent as 2026 still say OpenCode's TUI is "a Go binary using Bubble Tea" (e.g. https://nimbalyst.com/blog/claude-code-vs-codex-vs-opencode-definitive-comparison/). That has not been true for ten months.

From the local clone, verified 2026-09-02:

- `git ls-files | grep -c '\.go$'` returns **0**. There are no Go files in the repository.
- The commit that removed them: **`f68374ad22`, dated 2025-11-02, message: `DELETE GO BUBBLETEA CRAP HOORAY`**
- `packages/tui/package.json` today depends on:
  - **`@opentui/core` 0.4.5**
  - **`@opentui/keymap` 0.4.5**
  - **`@opentui/solid` 0.4.5**
  - **`solid-js` 1.9.10** (with a local patch at `patches/solid-js@1.9.10.patch`)
  - plus `effect`, `fuzzysort`, `remeda`, `diff`, `opentui-spinner`
- Runtime is **Bun 1.3.14** (`packageManager` field in root `package.json`)
- Source is `.tsx` — `packages/tui/src/app.tsx`, `index.tsx`, `keymap.tsx`, `runtime.tsx`

So: **the current OpenCode TUI is TypeScript + SolidJS rendered through OpenTUI, running on Bun.** They wrote their own terminal rendering engine (OpenTUI) rather than adopting an existing one. If JT likes OpenCode's interface best, OpenTUI + Solid is the specific stack to look at.

### Desktop app

**Shipped, in beta, Electron-based.** `packages/desktop/package.json` (version 1.18.26) uses `electron-vite` for dev/build and `electron-builder` for packaging, with `package:mac`, `package:win` and `package:linux` targets. Note: some third-party articles describe a **Tauri** desktop app — the repository says **Electron**. The repository is authoritative.

Desktop v2 became the default in **v1.18.0** (July 2026), including upgrade handling for the new layout, first-launch onboarding, and a setting to switch back to the old layout during transition. Tabs persist across restarts, with a Home tab toggle. Source: https://byteiota.com/opencode-v118-desktop-v2/ and https://www.explainx.ai/blog/opencode-desktop-tabs-sessions-worktrees-july-2026 (both accessed 2026-09-02).

### Server / remote / attach

This is OpenCode's architectural signature: **a persistent client/server split**. A background server owns AI communication and session state in local SQLite. The TUI, desktop app and IDE extensions are all just clients. Sessions survive SSH drops and terminal disconnects — reconnect and the server is still mid-task. Claude Code, by contrast, exits when the terminal closes.

From the local clone:
- `packages/server/` — `api.ts`, `auth.ts`, `cors.ts`, `routes.ts`, `handlers/`, `middleware/`, `pty-environment.ts`
- `packages/sdk-next/` — composes a **scoped in-process host** above Client, Core and Server
- `packages/protocol/`, `packages/schema/`, `packages/client/` — a layered contract with codegen
- Also present: `packages/console`, `packages/containers`, `packages/enterprise`, `packages/slack`, `packages/session-ui`, `packages/storybook`, `packages/codemode`

Desktop can attach to a remote `opencode serve` over a private network such as Tailscale. It is not flawless — https://github.com/anomalyco/opencode/issues/6835 reports "Desktop crashes when opening project after attach to remote server."

### Architecture write-ups

The best architecture document I found is **in the repo itself**: `/Users/jt/Code/opencode/CONTEXT.md`, titled **"OpenCode Session Runtime"** (32,094 bytes). It is a genuine design document with sections on Language, Relationships, Client contract architecture, and — refreshingly — **"Flagged ambiguities"** listing unsettled decisions. Notable content:

> "SDK executes Server's assembled `HttpRouter` in memory. It opens no listener and performs no network I/O, while preserving Server routing, middleware, codecs, handlers, and errors."

> "The beta OpenCode Client currently uses plural consumer-facing capability groups such as `sessions`; whether the stable Session namespace should instead be singular `session` must be settled before stabilization."

The codebase is built on **Effect** (the TypeScript effect system) throughout — `effect-drizzle-sqlite` and `effect-sqlite-node` are dedicated packages.

I did **not** find a published long-form architecture blog post, podcast or conference talk by Dax Raad (thdxr) or the SST team specifically about OpenCode's internals in the Aug–Sept 2026 window. **Not verified.** The changelog at https://opencode.ai/changelog is the main public narrative.

---

## 6. Prime Agent since August

**Repo:** https://github.com/PrimeIntellect-ai/prime-agent — "A self-improving RLM agent for coding workflows and long-running autonomous tasks."

### Stats (GitHub API, 2026-09-02)

| Metric | Value |
|---|---|
| Stars | **19,654** |
| Forks | 2,143 |
| **Open issues** | **78** |
| Created | 2026-05-08 |
| Public release | 2026-08-05 |
| Language | TypeScript |

**78 open issues.** Against OpenClaw's 3,683 and Hermes's 12,965. It is a far younger and far smaller project, and the numbers reflect that.

### Releases

From https://api.github.com/repos/PrimeIntellect-ai/prime-agent/releases (fetched 2026-09-02); local clone confirms `package.json` version **0.9.1** and tags through `v0.9.1`.

| Version | Published | Headline |
|---|---|---|
| v0.7.4 | 2026-08-19 | Agent visibility improvements; session and worker process robustness |
| v0.8.0 | 2026-08-21 | **Hardened OAuth credential binding to prevent token replay**; MCP server lifecycle management; generic MCP API for persistent HTTP/stdio servers |
| v0.8.1 | 2026-08-26 | Syntax highlighting for multi-line strings in expanded Python tool-call view; **default RLM recursion depth changed from 1 to 2**; Cloudflare AI Gateway default model updated |
| v0.9.0 | 2026-09-01 | Background kernel output handling; protocol interrupt management during REPL state restore; snapshot writer atomic file operations |
| v0.9.1 | 2026-09-01 | Agents view Inactive section fix; saved-session catalog loads progressively |

Recent local commits confirm the focus on daemon and TUI stability: `fix(coding-agent): stabilize daemon process identity (fixes #879) (#1971)`, `fix(coding-agent): replace TUI process after update (#1631)`.

### Did RLM-1 ship?

**No evidence that a model named "RLM-1" exists. Not verified — and most likely a naming confusion.**

**RLM** stands for **Recursive Language Model** — it is an *architecture*, not a model name. It was introduced by Alex Zhang in October 2025 and adopted by Prime Intellect. Prime Intellect's own trained models are branded **INTELLECT-3**, not RLM-1. Sources: https://www.primeintellect.ai/blog/rlm and https://www.primeintellect.ai/blog/prime-agent (accessed 2026-09-02).

Prime Agent implements RLM as a harness: `rlm(...)` spawns real child agents inside a persistent Python REPL, treating context as variables and subagents as function calls. That is a scaffolding technique, not a model release. The paper is **arXiv:2608.23552**, "Prime Agent: A Self-Improving RLM Harness" (https://arxiv.org/abs/2608.23552).

### Reception

- Hacker News thread on the release reached **249 points and 65 comments by 2026-08-07**
- Roughly **16,000 stars in the first week** — about 8,500 in a single week — making it one of August 2026's fastest-climbing agent repos; now 19,654
- Reported benchmark: **Opus 5 running in Prime Agent scores 95.5% on ARC-AGI-3, above a 95.4% human-expert baseline** (Prime Intellect blog; independent confirmation **not verified**)
- Criticism in the threads: **token cost of self-improvement loops**, **code quality** (commenters noted multiple files near 10,000 lines), and an **installer that wrote into the Homebrew directory with no uninstall path**

Sources: https://www.marktechpost.com/2026/08/06/prime-intellect-releases-prime-agent/, https://www.remio.ai/post/prime-agent-hit-hacker-news-but-its-self-improving-harness-is-the-real-story, https://www.opensourceforu.com/2026/08/prime-intellect-prime-agent/

---

## 7. Community consensus, one paragraph each

### (a) OpenClaw's bloat

The consensus is that OpenClaw's size is now its defining problem, and that the maintainers' own headline statistics are read by outsiders as evidence *against* the project rather than for it. When the 2.0 blog post advertised 16,000 pull requests from 933 contributors — 569 of them first-timers — the top Hacker News reaction was "569 first time contributors yea i shall continue to not use openclaw" (https://news.ycombinator.com/item?id=49505310, 2026-08-31, 148 points, 173 comments). The most-repeated sentence in that thread was some version of "all this complexity leads to stuff breaking with every update," and one commenter called it flatly "a vibecoded mess." The numbers support the feeling: 4,803 issues opened in 30 days, 3,683 open right now, 460 of them mentioning `doctor` and 211 mentioning `stuck`, plus concrete resource bugs like a Gateway that pegs a CPU core in a filesystem-discovery busy loop (#134993) and a memory reindexer that leaves 19 GB of orphaned temp databases behind (#136311). An independent site, ClawStat.us, now exists purely to tell people whether an OpenClaw release is safe to install, and on 2026-09-02 its advice was to stay on v2026.7.1-2. Fairly, the maintainers are not in denial — they took a seven-week release freeze specifically to reduce upgrade breakage, and 8.1's security section is unusually honest about its own limits — but the upgrade path from 2026.7.1 to 2026.8.1 still generated a wall of "completely breaks existing installation" reports.

### (b) Hermes vs OpenClaw

The comparison writeups converge on a framing rather than a winner: **OpenClaw thinks in organizations of agents; Hermes thinks in one agent that improves over time** (https://composio.dev/content/openclaw-vs-hermes-agent, https://kilo.ai/openclaw/vs-hermes, both accessed 2026-09-02). OpenClaw wins on breadth — more integrations, a 13,700-skill registry, multi-model support; Hermes wins on memory, personalization and cost efficiency. Aggregated Reddit sentiment reportedly splits roughly four ways with about 35% staying on OpenClaw for the integrations, a growing group running both, and the single biggest complaint being not the choice of agent but **the difficulty of self-hosting either one**. The raw reliability data does not favor Hermes as strongly as its reputation suggests: Hermes carries **12,965 open issues to OpenClaw's 3,683** despite having 62% of the stars, and **120 open issues with "TUI" in the title were opened in the last month alone**, including a SIGSEGV crash during streaming (#99388), a heartbeat timeout that kills slow-model generations (#100167), and a bug where submitting silently sends a collapsed paste placeholder to the model instead of the actual text (#99032). JT's judgment that the Hermes interface is glitchy is well-corroborated. The likely structural reason: the Ink/React TUI is only five months old (first commit 2026-04-02), is still versioned `0.0.1`, and took 89 commits in the last month while bolted onto a Python gateway and a 1-megabyte `cli.py`.

### (c) OpenCode vs Claude Code vs Codex CLI as interfaces

The terminal-agent field has consolidated to these three, and on interface craft specifically the verdict is close to unanimous: **OpenCode wins the TUI outright** — "the best-looking, best-engineered terminal in the category, and it built the rendering engine to get there, no contest on craft" (https://nimbalyst.com/blog/claude-code-vs-codex-vs-opencode-definitive-comparison/). Claude Code wins on reach rather than polish, since it runs in the terminal, a desktop app, and VS Code and JetBrains extensions, so it meets people where they already work. Codex CLI occupies the OpenAI-native lane, and the interesting rivalry is Codex vs OpenCode over vendor lock-in and what "open" means, since OpenCode supports 75+ providers via Models.dev including local Ollama and LM Studio. OpenCode's architectural advantage is real and directly relevant to a lightweight rebuild: a persistent client/server split where a background server owns session state in SQLite and the TUI, desktop and IDE extensions are all thin clients, so sessions survive SSH drops and terminal disconnects while Claude Code dies with its terminal. **One important correction for HARNESS_V2.md:** most published comparisons — including ones written in 2026 — still describe OpenCode's TUI as "a Go binary using Bubble Tea." That is stale. The repository contains **zero Go files**; Bubble Tea was deleted on **2025-11-02** in commit `f68374ad22` (message: "DELETE GO BUBBLETEA CRAP HOORAY"), and today's TUI is **TypeScript + SolidJS on the project's own OpenTUI renderer (`@opentui/core`, `@opentui/keymap`, `@opentui/solid`, all 0.4.5, with `solid-js` 1.9.10), running on Bun 1.3.14**. Anyone copying OpenCode's interface should copy OpenTUI + Solid, not Bubble Tea.

---

## Quick reference: the four projects side by side

All figures observed **2026-09-02**.

| | OpenClaw | Hermes Agent | OpenCode | Prime Agent |
|---|---|---|---|---|
| Repo | openclaw/openclaw | NousResearch/hermes-agent | anomalyco/opencode | PrimeIntellect-ai/prime-agent |
| Stars | 388,622 | 239,895 | 203,206 | 19,654 |
| Forks | 81,603 | 49,063 | 26,476 | 2,143 |
| **Open issues** | **3,683** | **12,965** | 5,584* | **78** |
| Issues opened, last 30d | 4,803 | 5,018 | not verified | not verified |
| Created | 2025-11-24 | 2025-07-22 | 2025-04-30 | 2026-05-08 |
| Core language | TypeScript/Node | **Python** | TypeScript | TypeScript |
| Latest version | 2026.8.2 | v0.21.0 (v2026.8.31) | v1.18.26 | v0.9.1 |
| Latest release date | 2026-09-01 | 2026-08-31 | 2026-09-01 | 2026-09-01 |
| Releases in Aug 2026 | 2 stable (+betas) | 8 | ~13 | 5 |
| TUI stack | own TUI, moving to `pi-tui` | **Ink + React 19**, 5 months old, v0.0.1 | **OpenTUI + SolidJS** on Bun | own TUI (TypeScript) |
| Desktop app | Yes (macOS/Win/Linux) | Yes (`apps/desktop`) | Yes, **Electron**, beta, v2 default | No |
| Client/server split | Gateway | Python `tui_gateway` | **Yes — persistent server + thin clients** | Daemon-backed sessions |
| License | Other | MIT | MIT | MIT |

\* OpenCode's 5,584 is `open_issues_count`, which includes pull requests. Its issues-only figure is **not verified**.

---

## Things I could not confirm

For honesty, here is everything marked "not verified" in one place:

1. Primary Reddit threads from Aug–Sept 2026 about OpenClaw bloat — only aggregator summaries found
2. Any new OpenClaw CVE or GitHub Security Advisory published in Aug–Sept 2026 (the advisories page's most recent batch is 2026-06-30)
3. Publication date of CVE-2026-41301 (affected versions are from March; a search summary claimed September)
4. The claim of "138 CVEs" for OpenClaw in 2026
5. Any updated count of internet-exposed OpenClaw instances after early 2026
6. Any new government/regulator statement or platform ban after the Meta/WhatsApp action
7. The 575 ms Gateway startup figure (third-party blogs only, not the project's own notes)
8. Google Trends data showing OpenClaw interest falling after March
9. A published long-form OpenCode architecture blog post, podcast or talk in the Aug–Sept window
10. Any model named "RLM-1" — RLM is an architecture; Prime Intellect's models are INTELLECT-3
11. Independent confirmation of the ARC-AGI-3 95.5% result
12. OpenClaw issue counts for the terms `nvm`, `Signal`, and `slow` (GitHub rate-limited those queries)
13. OpenCode's issues-only count (excluding PRs)
14. GitHub publication dates for OpenClaw v2026.6.11 and v2026.7.1

---

*Sources are linked inline throughout. Local clones were read only; no repository was modified.*
