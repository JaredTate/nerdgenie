# 11 — Computer use, browser, skills, and outreach

Research date: 2026-09-02. Repos read (read-only): `/Users/jt/Code/openclaw`, `/Users/jt/Code/hermes-agent`, `/Users/jt/Code/opencode`, `/Users/jt/Code/prime-agent`. File:line cites are relative to those repos. Web claims carry a URL and were checked on 2026-09-02 unless marked "not verified".

Bottom line first:

1. OpenClaw and Hermes both landed on the same browser design: launch Chrome with a **dedicated `--user-data-dir`** and a CDP port, read pages as an **accessibility-tree snapshot with short refs**, act by ref, screenshot only when needed. JT already runs this (his `~/.openclaw/browser` and `~/.config/openclaw/chrome-openclaw` dirs are exactly OpenClaw's managed profile).
2. Neither solves anti-bot or 2FA. Both tell the model to **stop and ask the human**. Copy that.
3. Skills are just `SKILL.md` with `name`/`description` frontmatter, an index in the system prompt, body loaded on demand. Hermes self-writes skills after every turn; OpenClaw drafts proposals a human applies. Take OpenClaw's gate with Hermes's ledger.
4. For social posting, use APIs where they exist (Bluesky, Mastodon, Meta via App Review, YouTube, X pay-per-use), a scripted logged-in browser for LinkedIn/X UI, and vision computer-use only as a last resort.
5. Nothing new replaces the whole stack. The closest small binaries are **ZeroClaw** (Rust) and **nanobot** (Python); neither has a serious browser story. Vercel's **agent-browser** (Rust, raw CDP, `--profile`) is the best drop-in browser engine for a small agent.

---

## Part 1 — How OpenClaw and Hermes drive browsers and desktops today

### OpenClaw browser (`extensions/browser`, ~145k lines incl. tests; `src/` is 13.8k)

**How it launches or attaches Chrome.** Three transports, chosen per "profile":

- **Managed profile (default, named `openclaw`).** OpenClaw launches real Chrome itself with `--remote-debugging-port=<cdpPort>` and `--user-data-dir=<dir>` plus hardening flags (`--no-first-run`, `--disable-sync`, `--password-store=basic`, `--no-proxy-server`; `--headless=new --disable-gpu` when headless; `--use-mock-keychain` on macOS so cookies persist without keychain prompts) — `extensions/browser/src/browser/chrome.ts:763-800`. The profile dir is `<CONFIG_DIR>/browser/<profile>/user-data` (`chrome.ts:753-756`; `CONFIG_DIR` defaults to `~/.openclaw`, `src/utils.ts:141`). That is JT's 358 MB `~/.openclaw/browser`. It re-attaches to an already running managed Chrome by matching those two flags on the process command line (`chrome.ts:304-315`) and clears stale singleton locks (`chrome.ts:472`).
- **Attach over CDP with Playwright.** Once Chrome is up, `playwright-core` is required lazily (`playwright-core.runtime.ts:4-14`) and `chromium.connectOverCDP(transport)` is called (`pw-session-cdp-transport.ts:234`). So it is **Playwright over CDP, not Playwright's own launcher**, and there is a raw-CDP fallback when Playwright is missing (the bundled skill notes the fallback "does not support labeled screenshots at all", `extensions/browser/skills/browser-automation/SKILL.md`).
- **Existing-session ("user") profile.** Two ways to reach the user's real logged-in Chrome: (a) a Chrome MV3 extension that relays `chrome.debugger` frames over a loopback WebSocket to OpenClaw ("Securely relay eligible signed-in Chrome tabs", permissions `debugger, tabs, tabGroups, nativeMessaging`, `extensions/browser/chrome-extension/manifest.json`, `background.js:10-16`); (b) a `chrome-mcp` adapter that shells out to Google's `chrome-devtools-mcp@1.8.0` (`chrome-mcp-options.ts:13`, facade `chrome-mcp.ts`). Those profiles are read-mostly: no `batch`, no `emulate`, no request/error logs (`SKILL.md`).
- **Remote browser.** A `target` of `sandbox | host | node` (`browser-tool.schema.ts:62`) lets the tool proxy to a browser on another paired machine (`browser-node-proxy.ts`, `browser-node-routing.ts`).

**Tool surface exposed to the model.** One tool, `browser`, with 24 top-level `action`s: `doctor, status, start, stop, profiles, importprofile, tabs, open, focus, close, snapshot, screenshot, navigate, console, requests, errors, text, emulate, pdf, download, waitfordownload, upload, dialog, act` (`browser-tool.schema.ts:36-60`). `act` takes one of 14 `kind`s: `batch, click, clickCoords, type, press, hover, scrollIntoView, drag, select, fill, resize, wait, evaluate, close` (`:19-34`). Snapshots come in `aria` or `ai` format with `role`/`aria` refs (`:64-66`); the skill tells the model to prefer stable tab `label`s and to re-snapshot after navigation. Limits: 100 actions per batch, depth 5, waits ≤30 s, interaction timeout 8 s default / 60 s max (`browser/act-policy.ts:19-33`). Tabs opened by a chat session are closed when the session ends (`src/browser-lifecycle-cleanup.ts:22-44`).

**Keeping logins.** Persistence is the profile dir. Logins are meant to be done **by hand**: "sign in manually in the host browser's `openclaw` profile. Do not give the model your credentials: automated logins often trigger anti-bot defenses and can lock the account" and "Sandboxed browser sessions are more likely to trigger bot detection" (`docs/tools/browser-login.md`). `importprofile` copies cookies from a system Chrome profile into a managed one, optionally scoped to `domains` (`browser-tool.lifecycle.ts:158-171`); the macOS app has a `CookieSyncManager` and a profile-import UI (`apps/macos/Sources/OpenClaw/CookieSyncManager.swift`, `BrowserProfileImportModel.swift`).

**Anti-bot / 2FA.** There is **no** captcha or 2FA code in the extension (grep for `captcha|2fa|two-factor|stealth` returns nothing outside docs). Policy is in the prompt: "If the page needs login, permission, captcha, 2FA ... stop and tell the user exactly what is needed" (`browser-automation/SKILL.md`).

**Desktop control.** Separate from the browser:
- The model tool is `computer` (`docs/nodes/computer-use.md`). Actions are the Anthropic-style v2 list: `screenshot, left_click, right_click, middle_click, double_click, triple_click, mouse_move, left_click_drag, left_mouse_down, left_mouse_up, scroll, type, key, hold_key, wait, list_apps, list_windows, get_accessibility_tree, get_cursor_position, get_window_state, launch_app, kill_app, ...` (`src/plugins/computer-use-contract.ts:10-32`). The gateway sends `screen.snapshot` and `computer.act` to a paired node (`src/node-host/computer-command.ts:13-49`).
- On macOS the fulfiller is the Swift app: CGEvent clicks/drags and ScreenCaptureKit screenshots (`apps/macos/Sources/OpenClaw/ComputerScreenActionExecutor.swift:1-6,550,576`; `ScreenSnapshotService.swift:73` uses `SCScreenshotManager`) built on the **Peekaboo** automation kit (`apps/macos/Package.swift:23,70-71`). Requires Accessibility + Screen Recording; screenshots are "model-only, never auto-delivered to the chat channel" (`docs/nodes/computer-use.md`).
- The optional `cua-computer` plugin wraps `@trycua/cua-driver` for macOS/Windows/Linux ("Experimental CUA Driver computer control", `extensions/cua-computer/openclaw.plugin.json`; `src/driver-client.ts:1-80`), which adds background delivery (no cursor steal), `set_value`, `get_accessibility_tree`, recording, and its own isolated-profile browser family.
- `src/node-host/desktop-stream-command.ts` streams a VNC/RFB desktop (port 5900) to the web UI; `src/canvas` is an HTML widget canvas, not OS control.

### Hermes browser (`tools/browser_tool.py`, ~6.5k lines)

**How it launches or attaches.** Hermes does not embed Playwright. It shells out to Vercel's **`agent-browser` CLI** (`agent-browser@^0.26.0`, `browser_tool.py:978`), a Rust daemon that talks raw CDP and returns accessibility snapshots with `@e1`-style refs (`browser_tool.py:1-24`). Backends: local Chromium (default), Lightpanda engine (`:757`), Camoufox anti-detect Firefox via REST when `CAMOFOX_URL` is set (`tools/browser_camofox.py:1-22`), and cloud providers (Browser Use, Browserbase, Firecrawl) registered through a plugin ABC that returns a `cdp_url` per session (`agent/browser_provider.py:1-40`, `agent/browser_registry.py:1-30`). `/browser connect` attaches to your own Chrome/Brave/Edge on CDP port 9222 (`hermes_cli/browser_connect.py:1-30`). A per-task `CDPSupervisor` keeps one WebSocket open to watch dialogs and frames (`tools/browser_supervisor.py:1-16`).

**Tool surface.** Twelve separate tools: `browser_navigate, browser_snapshot, browser_click, browser_type, browser_scroll, browser_back, browser_press, browser_get_images, browser_vision, browser_console` (`browser_tool.py:2876-3003`), plus `browser_cdp` raw-protocol escape hatch (`tools/browser_cdp_tool.py:1-16`) and `browser_dialog` (`browser_supervisor.py:14-16`). Refs are `@eN`; `browser_vision` is the screenshot tool and is explicitly pitched for "CAPTCHAs, visual verification challenges" (`:2985`).

**Keeping logins.** Opt-in `browser.use_real_profile` (`:1513-1527`). When on, Hermes **copies** the user's default Chromium profile into a Hermes-owned dir and launches the real Chrome binary on that copy with `--user-data-dir=<copy> --remote-debugging-port=0` (`:1640-1660`, `:1784-1798`), because "the copy is a non-default dir, so it sidesteps the Chrome ≥136 default-profile remote-debugging block" (`:1650`). Revoking consent deletes the copies (`:1656-1666`). Cloud profiles persist on the provider.

**Anti-bot / 2FA.** Heuristics only: if the page title contains "are you a robot", "captcha", etc., the tool returns advice to enable residential proxies / `BROWSERBASE_ADVANCED_STEALTH` or accept that "some sites have very aggressive bot detection that may be unavoidable" (`:4453-4476`). Stealth is outsourced to Camoufox or the cloud vendor. No 2FA handling.

**Extension control.** A newer path lets a browser extension act as the "controller": `browser.extension_control.enabled` (default false) routes `browser_*` calls through a gateway broker with single-use tickets and scoped controllers (`tools/browser_extension_router.py:1-40`, `gateway/browser_control_broker.py:1-60`); screenshots move as one-shot SHA-256-checked artifacts (`gateway/browser_control_artifacts.py:1-30`).

**Desktop control.** One tool `computer_use` on cua-driver: "Background computer-use: does NOT steal the user's cursor or keyboard focus" (`tools/computer_use_tool.py:19-33`). Actions: `capture, click, double_click, right_click, middle_click, drag, scroll, type, key, set_value, wait, list_apps, list_windows, focus_app`; capture modes `som` (numbered overlays + AX tree), `vision`, `ax` (`tools/computer_use/schema.py:16-80`). Every mutating action goes through an approval callback with `approve_once | approve_session | always_approve | deny` (`tools/computer_use/tool.py:63-92`). macOS TCC grants attach to cua-driver's identity, not Hermes (`tools/computer_use/permissions.py:1-20`). `tools/desktop_ui.py` only talks to the Hermes desktop app's renderer.

### OpenCode

No browser or desktop tools. `packages/opencode/src/tool/` contains bash, edit, glob, grep, read, webfetch, websearch, skill, task, todo, lsp, question. A grep for `playwright|puppeteer|chromium|xdotool` hits only an unrelated snapshot module.

### Prime Agent

Also no native browser tool; browsing is expected through Python skills in its REPL (`README.md`, `packages/coding-agent/docs/skills.md`).

---

## Part 2 — Skills: how each learns and remembers

### Hermes

- **Format.** `SKILL.md` with YAML frontmatter, "agentskills.io compatible": `name` (≤64), `description` (≤1024), optional `version, license, platforms, prerequisites, metadata.hermes.tags/related_skills`, plus `references/`, `templates/`, `scripts/`, `assets/` (`tools/skills_tool.py:1-60`). Bundled: 58 skills in categories (`skills/social-media/xurl`, `skills/email/himalaya`, ...). User skills live in `~/.hermes/skills/`.
- **Loading (progressive disclosure).** Tier 1: the system prompt gets an index grouped by category with each description **cut at 60 chars** (`SKILL_PROMPT_DESC_LIMIT = 60`, `agent/skill_utils.py:1256`; builder `agent/prompt_builder.py:1781-2115`, cached snapshot). Tier 2: `skill_view(name)` loads the body. Tier 3: `skill_view(name, "references/x.md")`.
- **Creation by the agent.** `skill_manage` tool: `create, edit, patch, delete, write_file, remove_file` (`tools/skill_manager_tool.py:1-33`).
- **Learning after a task (self-write).** After every turn a **background review** forks the agent with only memory+skill tools and asks it to update the library (`agent/background_review.py:1-17`). The prompt is aggressive: "Be ACTIVE — most sessions produce at least one skill update ... A pass that does nothing is a missed learning opportunity"; preference order is patch a loaded skill → patch an umbrella skill → add a `references/` file (`:476-530`, `:615-650`). Bundled, hub, pinned and user-owned skills are off-limits to the reviewer.
- **Training by the user.** `/learn <what>` builds one prompt that makes the agent gather sources (a dir, URL, "what I just did in this chat") and author a skill via `skill_manage` under house rules (description ≤60 chars, `author: Hermes`) (`agent/learn_prompt.py:1-28`, `hermes_cli/commands.py:342`).
- **Safety and audit.** Every mutation is appended to `~/.hermes/skills/.curator_ledger.jsonl` with content-addressed backups and `hermes curator rollback` (`tools/skill_ledger.py:1-20`); usage counters drive stale/archived states (`tools/skill_usage.py:1-25`); hub installs are regex-scanned with trust levels builtin/trusted/community (`tools/skills_guard.py:1-15`).

### OpenClaw

- **Format.** Same `SKILL.md`. Required `name`, `description`; also `metadata`, `homepage`, `license`, `allowed-tools`, `user-invocable`, `disable-model-invocation`, `command-dispatch/command-tool/command-arg-mode` (`skills/skill-creator/SKILL.md`). `metadata.openclaw` carries `emoji`, `os`, `requires.bins`, and `install` recipes (`skills/things-mac/SKILL.md`). 52 bundled skills; 4 `custodian-skills` for ops tasks.
- **Loading.** A `Skill` record has `name, description, filePath, baseDir, disableModelInvocation` (`src/skills/loading/skill-contract.ts:5-20`); the compact catalog truncates descriptions at 220 chars (`:33`) and is rendered as `<available_skills><skill><name>..` XML in the system prompt (`src/skills/loading/workspace-skill-prompt.ts:160-186`). The body is read on demand. ClawHub installs go through `src/skills/lifecycle/clawhub-store.ts` and `library/import.ts:78`.
- **Learn-from-session.** The `src/skills/workshop/` "experience review" builds a prompt from the run's *actual* skill usage ("Skills actually used in this trajectory (authoritative runtime receipt)", `experience-review-prompt.ts:66-80`) and produces a **proposal**, not a live edit. `applyAutonomousSkillProposal` auto-applies only Workshop-owned skills; user-authored skills go to "awaiting operator review" (`autonomous-apply.ts:8-30`). Proposals and usage live in SQLite (`curator.ts:30-33`); a weekly "collection review" replaces the older curator (`curator.ts:27-28`). The model-facing `skill_workshop` tool has `create, read, prepare_patch, patch, update, revise, list, inspect, evaluate, apply, reject, quarantine`, body capped at 40,000 bytes (`src/agents/tools/skill-workshop-tool-schema.ts:51-123`).

### OpenCode and Claude Code (the convention)

OpenCode scans `~/.claude/skills/**/SKILL.md`, `~/.agents/skills/**`, project `.opencode/{skill,skills}/**`, and `skills.paths/urls` from config (`packages/opencode/src/skill/index.ts:22-26,180-230`). Frontmatter must have `name` and optional `description` (`:52-58`). The `skill` tool loads the body plus a sampled file list only when called, after a permission check (`packages/opencode/src/tool/skill.ts:8-60`; description in `skill.txt`). Prime Agent follows the same Agent Skills spec and adds Python-backed skills with a built-in `skill-creator` (`packages/coding-agent/docs/skills.md`).

### Proposal for the new agent

Directory: `~/.agent/skills/<name>/`

```
SKILL.md        # frontmatter + checklist body (≤ 6 KB)
run.ts          # deterministic script (Playwright), variables via argv/JSON
test.ts         # dry-run smoke test; must pass before the skill is "active"
fixtures/       # sample inputs, last known-good aria snapshot per step
CHANGELOG.md    # who/when/why for every edit (ledger)
```

Frontmatter (superset of agentskills.io so it stays portable):

```yaml
name: post-to-x
description: Post a text or image update to X from the marketing profile.   # ≤120 chars
triggers: [post on x, tweet, share on twitter]
tools: [browser, skill_run]
permissions:
  browser_profile: marketing        # which persistent profile
  domains: [x.com]                  # navigation allowlist
  max_actions: 40                   # per run
  irreversible: [click:Post]        # steps that need approval
  approval: first                   # never | first | always
  rate: 5/day
test: test.ts
version: 3
learned_from: session-2026-09-02T14:10
```

**Teach-me flow (demonstrate once).**
1. JT says "teach: post to X" (Signal or CLI). The agent opens the headed browser on the `marketing` profile and records while JT does the task once: CDP events, an aria snapshot before each click, and the final screenshot. (Playwright's `codegen --user-data-dir` does exactly this recording; Chrome DevTools recorder is the no-Playwright alternative.)
2. The agent writes `run.ts` with **three selectors per step** (role+name → visible text → CSS) and marks the variable fields. It writes the checklist in `SKILL.md` and names the irreversible step.
3. It runs `test.ts` in dry-run: everything up to the irreversible click, then a screenshot to JT over Signal. JT replies "ok" → version 1 saved with `approval: first`.
4. **Recall.** The system prompt carries only the index (`name`, `description`, `triggers`); the body loads via `skill_view`; scripts run via `skill_run`, which enforces `permissions` (domains, action cap, rate) outside the model.
5. **Self-improvement, gated.** When a run fails, the agent may propose a patch (a diff to `run.ts` + a fixture update) into a `proposals/` queue; it is applied only after JT approves, and every apply appends to `CHANGELOG.md` with a rollback snapshot (Hermes ledger idea, OpenClaw gate). Do not run an "always write something" reviewer after every turn; it produces noise.

---

## Part 3 — Reliability ladder for "use the browser like a human"

### Why browser agents are unreliable

- **Benchmark vs. production gap.** A vendor write-up lists the six failure modes public benchmarks skip: DOM selector drift, screenshot ambiguity, login state, modal interruptions, rate-limit cliffs, and irreversibility, and notes a 78% WebArena agent booking only 22% of carts live (https://futureagi.com/blog/evaluating-browser-use-agents-2026/, 2026). Online-Mind2Web (300 tasks on 136 live sites) is the honest test; general models score ~40% there (GPT-5 Medium 42.3%, Claude Sonnet 4 40.0% per https://benchmarkingagents.com/mind2web/ — secondary source, not verified against the paper), while tuned agents claim 97-99% (https://browser-use.com/posts/online-mind2web-benchmark; https://github.com/at-inc/aside-benchmarks).
- **Anti-bot.** Cloudflare and peers fingerprint CDP runtime objects, timing, and the TLS JA3/JA4 hash; Playwright's bundled Chromium "does not match any real Chrome release" (https://www.browserstack.com/guide/playwright-cloudflare, 2026). Camoufox patches fingerprints in Firefox C++ and reports 0% headless detection on standard tests (https://github.com/jo-inc/camofox-browser); a 651-verdict 2026 benchmark put nodriver first and Camoufox/Patchright mid-pack (https://ianlpaterson.com/blog/anti-detect-browser-benchmark-patchright-nodriver-curl-cffi/).
- **Chrome 136+** refuses `--remote-debugging-port` on the default profile; you must use a separate `--user-data-dir` (https://developer.chrome.com/blog/remote-debugging-port). This is why both OpenClaw and Hermes copy or create profiles.
- **Account bans.** LinkedIn removed HeyReach's page and banned its founder on 2026-03-25 and flags "cloud-proxy" automation infrastructure regardless of per-user limits (https://www.joinvalley.co/blog/linkedin-automation-safety-2026); a vendor sample reported ~40% of accounts on non-compliant tools restricted in Q1 2026 (https://northlight.ai/blog/what-happens-when-linkedin-bans-your-tool, vendor stat, not verified). Reddit's 2026 crackdown ended mass API posting (https://www.readyt.ai/blog/reddit-automation-crackdown-2026/).
- **2FA and login walls** cannot be automated safely; every mature agent hands off to the human here.

### The ladder

1. **Official API or CLI** (deterministic, no bans): use whenever it exists — `xurl`/X API, Meta Graph, Bluesky AT Protocol, Mastodon REST, YouTube Data API, Gmail API, `gh`, `himalaya`.
2. **Scripted browser on a persistent logged-in profile**, recorded once as a skill with selector fallbacks (Playwright/CDP, headed, real Chrome binary, residential IP = JT's home). This is where LinkedIn and X posting live.
3. **Vision computer-use** (screenshot → click) only for the step a script cannot express (a new modal, a native file dialog), and only with approval.

### Platform table (2026)

| Platform | Official write API? | Cost / gate | Ban risk for browser automation | Realistic route |
|---|---|---|---|---|
| X / Twitter | Yes | Free tier ended 2026-02-06; pay-per-use ≈ $0.015/post, $0.20/post with a link; Basic retired 2026-06-01, Pro closed (https://postproxy.dev/blog/x-api-pricing-2026/; not verified on developer.x.com) | Medium; sandboxed/headless sessions trigger detection (OpenClaw `docs/tools/browser-login.md`) | API for reads/posts at low volume; headed profile for anything the API charges for |
| LinkedIn | Only via partner-approved Community Management API (MDP) (https://www.blotato.com/blog/social-media-api) | Partner approval; not for individuals | **High**; 2026 enforcement incl. identity-verification unlocks | Scripted headed browser, ≤ handful of actions/day, human-like pacing, no cloud proxies |
| Instagram / Facebook | Yes, Graph API (2-step container → publish) | Free; needs Business/Creator account + Page + App Review, 2-4 weeks per permission (https://developers.facebook.com/docs/instagram-platform/overview/; https://www.netrows.com/blog/instagram-graph-api-guide-2026) | Medium-high | Do App Review once for your own accounts; browser only for Stories/features the API lacks |
| Threads | Yes, separate Threads API (text + media) | Free, App Review | Medium | API |
| Bluesky | Yes, AT Protocol; bots welcome, self-label as automated; ~5,000 pts/hour (https://docs.bsky.app/docs/starter-templates/bots) | Free | Low | API |
| Mastodon | Yes, REST; mark account "automated" (https://fedi.tips/why-are-some-accounts-marked-bot-on-mastodon/) | Free | Low | API |
| Reddit | Yes, OAuth, 60 rpm; commercial ≈ $0.24/1k calls (https://www.socialcrawl.dev/blog/reddit-data-api-2026) | Free non-commercial | High for posting bots | API with human-in-the-loop replies only |
| TikTok | Content Posting API | Free but audit; posts are SELF_ONLY until audited (https://www.postpeer.dev/blog/best-tiktok-posting-api) | High | API after audit, else manual |
| YouTube | Data API v3 | 10,000 quota units/day, upload ≈ 1,600 units (https://www.blotato.com/blog/social-media-api) | Low via API | API |
| Pinterest | Yes | Free, rate-limited | Medium | API |
| Schedulers | Postiz (AGPL, self-host, public API + MCP, 20+ networks) (https://postiz.com/blog/free-social-media-scheduling-api-n8n-postiz-buffer-blotato-alternative); Typefully (X+LinkedIn); Buffer new OAuth apps closed (https://zernio.com/blog/buffer-api) | Postiz free self-hosted | Low (they hold the API approvals) | **Postiz on the Pi** is the pragmatic multi-network path |
| Email | Gmail API | Workspace 2,000 recipients/day; ≥5k/day senders need SPF/DKIM/DMARC, <0.3% spam, List-Unsubscribe (https://www.gmass.co/blog/understanding-gmails-email-sending-limits/) | n/a | Gmail API |

### Sales outreach with a Rolodex

**SQLite schema (minimum).**
- `contacts(id, email UNIQUE COLLATE NOCASE, first_name, last_name, company, role, source, tags, timezone, status ['new','contacted','replied','won','lost'], do_not_contact INTEGER, consent_note, notes, last_contacted_at, created_at, updated_at)`
- `campaigns(id, name, template_path, daily_cap, send_window, status)`
- `drafts(id, contact_id, campaign_id, subject, body, personalization, approved_by, approved_at, UNIQUE(contact_id, campaign_id))`
- `sends(id, draft_id UNIQUE, contact_id, campaign_id, gmail_message_id, thread_id, sent_at, status, error)` with `UNIQUE(contact_id, campaign_id)` — the **idempotent ledger**: a send is inserted *before* the API call with status `sending`, updated after; a retry that finds the row skips.
- `events(id, contact_id, kind ['bounce','reply','unsubscribe','open'], at, raw)` and `suppressions(email UNIQUE, reason, at)` checked on every draft.

**Templating.** Mustache-style `{{first_name}}` with required-field validation; the model writes only a 1-2 sentence `personalization` block from `notes`, never the whole email, so tone stays fixed. **Dedupe:** normalize email (lowercase, trim, strip Gmail dots/+tags for matching), unique index, skip anyone contacted in the last N days or on `suppressions`. **Rate limits:** warm up 10→40/day, jitter 3-15 min, business hours in the recipient's timezone, hard stop at campaign `daily_cap`. **Approval over Signal:** the agent posts "5 drafts ready" with one-line previews; JT replies `send 1,2,5` or `edit 3: ...`; only approved IDs move to `sends`. **Gmail API vs SMTP:** Gmail API (`users.messages.send`, OAuth) gives thread ids, labels and reply/bounce detection through `history.list`; SMTP with an app password only sends. Use the API.

**Compliance (CAN-SPAM, two sentences).** Every commercial email needs accurate headers, an honest subject, a physical postal address, and a working opt-out that is honored within 10 business days; violations can cost up to $53,088 per email (https://www.ftc.gov/business-guidance/resources/can-spam-act-compliance-guide-business). Add a `List-Unsubscribe` header and write every opt-out into `suppressions` before anything else runs.

---

## Part 4 — Other agents JT may not know about

Legend: DBMC = Desktop / Browser / Messaging / Cron. Stars and dates as of 2026-09-02.

| Agent | URL | Lang | What it does | Local models | D/B/M/C | Maturity | Verdict |
|---|---|---|---|---|---|---|---|
| browser-use | https://github.com/browser-use/browser-use | Python | DOM+vision browser agent library; CLI 3.0 is Hermes's default driver | Yes (any LiteLLM/Ollama) | -/B/-/- | 106k stars; v0.13.6 2026-07-17 | Best Python library; wrong language if the agent is TS |
| Stagehand | https://github.com/browserbase/stagehand | TS (+Py, Go) | `act/extract/observe` SDK; v3 (Feb 2026) dropped Playwright for raw CDP; v4 runs as an extension | Yes via provider adapters | -/B/-/- | Very active (https://www.browserbase.com/blog/stagehand-v4) | Strong TS choice if you want AI-assisted selectors; pushes Browserbase cloud |
| Magnitude browser-agent | https://github.com/magnitudedev/browser-agent | TS | Vision-first browser agent | Yes | -/B/-/- | 4.1k; last update 2026-02-08; org pivoted to a local inference server | Dormant; skip |
| Skyvern | https://github.com/Skyvern-AI/skyvern | Python | Workflow platform, vision+XPath, MCP + "Skills" for Claude Code/OpenClaw (2026-03-03) | Yes (Ollama) | -/B/-/C | 22.9k; AGPL-3.0 | Heavy (Postgres, UI); overkill for one person |
| Nanobrowser | https://github.com/nanobrowser/nanobrowser | TS | Chrome extension multi-agent, BYO key | Yes (Ollama) | -/B/-/- | 13.7k; Apache-2.0 | Good for manual "do this in my browser"; no headless/cron |
| Claude in Chrome / Cowork | https://chromewebstore.google.com/detail/claude/fcoeoabgfenejglbffodgkkbkcdhcgfn | — | Extension is now a Cowork session; Claude also ships a built-in side-panel browser; open beta on paid plans, v1.0.90 2026-08-31 | No | -/B/-/- | Product | Use as a benchmark of UX, not a component |
| Anthropic computer-use reference | https://github.com/anthropics/claude-quickstarts/tree/main/computer-use-demo | Python | Dockerized X11 desktop loop; tool now `computer_toolset_20260801` (https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool) | No | D/-/-/- | Reference | Copy the loop shape for the `computer` tool |
| OpenAI Operator / Atlas / CUA | https://help.openai.com/en/articles/12591856-chatgpt-atlas-release-notes | — | Operator folded into ChatGPT Agent (2025-08-31); Atlas discontinued 2026-07-09, off 2026-08-09, replaced by "ChatGPT Work" (https://ppc.land/openai-kills-atlas-browser-folds-it-into-new-chatgpt-work-agent/) | No | D/B/-/- | Products in flux | Not a building block |
| Bytebot | https://github.com/bytebot-ai/bytebot | TS | Desktop agent in a Linux container | Yes | D/B/-/- | **Archived 2026-03-07** | Dead |
| UI-TARS Desktop / Agent TARS | https://github.com/bytedance/UI-TARS-desktop | TS/Electron | GUI agent app on ByteDance's UI-TARS model; remote computer/browser operators | Yes (UI-TARS weights) | D/B/-/- | Desktop v0.2.0 2025-06-12; CLI v0.3.0 2025-11-05 | Model is interesting, app is not |
| Cua (trycua) | https://github.com/trycua/cua | Python/TS/Rust | Sandboxes + **cua-driver** (open-sourced 2026-04-23): background input on macOS/Win/Linux; used by OpenClaw and Hermes | Partial | D/-/-/- | 22.1k; MIT | Adopt cua-driver for background desktop control |
| Open Interpreter | https://github.com/OpenInterpreter/open-interpreter | Rust (rewrite) | Code-running agent; "Computer Use" via agent-browser + trycua; v0.0.40 2026-08-20 | Yes | D/B/-/- | 68k stars (legacy) | Watch; young rewrite |
| Agent S (Simular) | https://github.com/simular-ai/Agent-S | Python | Research GUI agent; S3 72.6% OSWorld (2025-12-15) | Yes (UI-TARS via vLLM) | D/-/-/- | 12.2k; Apache-2.0 | Research quality, not a daemon |
| Magentic-UI / MagenticLite; UFO³ | https://github.com/microsoft/magentic-ui ; https://github.com/microsoft/UFO | Python | Human-in-the-loop web agent; Windows UI agent | Partly | D/B/-/- | Research (MIT) | Windows-only / research |
| Project Mariner → Gemini Agent, Chrome "auto browse" | https://gigazine.net/gsc_news/en/20260507-google-shuts-down-project-mariner/ | — | Mariner shut 2026-05-04; features moved into Gemini and Chrome for AI Pro/Ultra (US) | No | -/B/-/- | Product | Not a component |
| Manus | https://www.cnbc.com/2026/08/11/manus-china-meta-acquisition.html | — | Hosted general agent; Meta deal unwound, independent again 2026-09-01 | No | D/B/-/- | Product | Not self-hostable |
| Perplexity Comet | https://www.itechguides.com/perplexity-comet-browser-review-2026-is-it-worth-trying/ | — | Chromium browser with agent sidebar; free since 2026-03-18; Amazon injunction on logged-in shopping | No | -/B/-/- | Product | Not a component |
| agent-browser (Vercel) | https://github.com/vercel-labs/agent-browser | Rust + TS | CLI/daemon, raw CDP, aria snapshots with `@eN` refs, `--profile` persistent dir, `--session`, native linux-arm64 | n/a (no LLM) | -/B/-/- | 41.8k; Apache-2.0 | **Best small engine**; Hermes wraps it |
| ZeroClaw | https://github.com/zeroclaw-labs/zeroclaw | Rust | Single-binary assistant: 30+ channels (Telegram, Discord, Matrix, email), cron, shell, "browser automation", Ollama | Yes | ?/B/M/C | 32.7k (search result; not verified) | Closest "small binary" competitor; browser depth unknown |
| nanobot (HKUDS) | https://github.com/HKUDS/nanobot | Python | ~4k-line core, Telegram/Discord/Slack/WeChat/Email/Mattermost, cron, WebUI; web fetch/search only | Yes | -/-/M/C | 47.7k; v0.3.0 2026-07-24 | No real browser; Signal support not verified |
| PicoClaw (Sipeed) | https://github.com/sipeed/picoclaw | Go | <10 MB RAM assistant, Telegram/Discord | Yes | -/-/M/C | Released 2026-02-09 | Too thin |
| IronClaw (NEAR) | https://github.com/nearai/ironclaw | Rust | Sandboxed-tool "agent OS", WASM tools, encrypted secrets | Yes | -/-/M/C | Launched 2026 | Interesting security model; no browser story |

**"Claw desktop" (OpenClaw macOS app).** Yes, it does desktop control, and it is Swift: 556 Swift files, ~159k lines under `apps/macos/`. Peekaboo is the default fulfiller, CUA optional (`docs/nodes/computer-use.md`; https://docs.openclaw.ai/platforms/mac/peekaboo).

---

## Part 5 — Recommendation

### Tool set (11 tools)

| Tool | One line |
|---|---|
| `browser_open(url, label, profile?)` | Open or reuse a labeled tab in a named persistent profile |
| `browser_snapshot(label, query?, interactive?)` | Accessibility-tree text with `@refs`; the default way to "see" |
| `browser_act(label, actions[])` | Batch of `click/type/press/select/hover/scroll/wait` by ref or coords, 8 s auto-wait, stops on first error |
| `browser_screenshot(label, labels?)` | PNG with optional numbered overlays; use when snapshot is ambiguous |
| `browser_eval(label, js)` | Bounded `evaluate`; off unless the skill's permissions allow it |
| `browser_file(label, ref, path)` | Upload via file chooser or download to a fixed dir |
| `browser_tabs()` | List / close; hygiene before opening |
| `browser_handoff(label, reason)` | Pause, bring the window to front, ping JT on Signal, resume when he says "done" (login, 2FA, captcha) |
| `computer_screenshot()` / `computer_act(action, ...)` | Desktop last resort via Peekaboo CLI on the Mac or cua-driver; every act needs approval |
| `skill_run(name, args)` | Run a recorded skill's `run.ts` under its declared permissions |
| `email_send(draft_id)` | Only sends drafts marked approved in the ledger |

Shell already exists for CLIs (`gh`, `xurl`, `himalaya`, `postiz` API calls).

### Playwright vs raw CDP

Use **`playwright-core` in TypeScript**, attached with `connectOverCDP` to a Chrome you launched yourself (OpenClaw's exact pattern, `chrome.ts:763-800`, `pw-session-cdp-transport.ts:234`), or `launchPersistentContext(userDataDir, {channel: 'chrome'})` for the simple case (https://playwright.dev/docs/api/class-browsertype). Reasons: `ariaSnapshot()` gives the ref-style snapshot for free, `getByRole` selectors survive DOM churn, auto-wait, dialogs, downloads, tracing, and `codegen --user-data-dir` is the recorder for the teach-me flow. Raw CDP (what Stagehand v3 and agent-browser do) is faster but you re-implement all of that. If you want zero browser code, run **agent-browser** as a subprocess exactly as Hermes does and wrap its `snapshot/click/type` commands; it is a single Rust binary with `--profile` and native linux-arm64 builds. Playwright under Bun is not verified; run the browser worker under Node if Bun misbehaves.

### Persistent profile, safely

- One `--user-data-dir` per persona (`~/.agent/browser/marketing`, `.../personal`), mode 700, never the daily Chrome profile (Chrome 136+ blocks it anyway).
- Use the **real Chrome binary** (`channel: 'chrome'`), not Playwright's Chromium, so the TLS/UA fingerprint matches a real release; headed, on JT's home IP.
- Log in by hand once through `browser_handoff`; never let the model type passwords (OpenClaw `docs/tools/browser-login.md`). Import cookies only domain-scoped, like OpenClaw `importprofile ... domains`.
- CDP bound to `127.0.0.1` on a random port; clear stale `SingletonLock` on start (`chrome.ts:472`); nightly `tar` of the profile dir; a `doctor` action that reports "logged in?" per site by hitting a known URL and checking the aria snapshot.

### Headless vs headed on the Mac

Run **headed** with the window parked on a second Space or offscreen (`--window-position=-2000,0`); `--headless=new` is still detectable and social sites are the ones that care. Headless is fine for read-only scraping of public pages. Screenshots stay model-only, never auto-posted to Signal (OpenClaw's rule).

### The Pi (The 7900 XT machine)

Do not fight anti-bot from a headless ARM Linux box. Two roles:
1. **Remote browser on the Mac over Tailscale.** The Mac runs the Chrome worker and exposes CDP on its Tailscale IP (or the agent's own small WS bridge with a token, like OpenClaw's bridge auth `browser/bridge-server.ts` and Hermes's `/browser connect` to a `cdp_url`). The Pi agent calls `browser_*` tools that proxy there; if the Mac is asleep, `browser_*` returns "browser host offline" and the task queues.
2. **Local headless Chromium on the Pi** (Playwright supports Debian 12 arm64, https://playwright.dev/docs/release-notes) only for API-less public reads, plus **Postiz** self-hosted on the Pi for scheduled multi-network posting via its API.

That split keeps the reliable parts (APIs, Postiz, email ledger, cron, Signal) always-on on the Pi, and the fragile part (a logged-in human-looking browser) on the Mac where JT can take over in seconds.
