# 16 — Browser agent deep dive and the Nerd Genie browser spec

Date: 2026-09-02. Author: research pass for Nerd Genie. Code paths are cited as `file:line`. Web claims carry URL + date. "Not verified" means I could not confirm it from code or a primary source.

Repos read: OpenClaw at `/Users/jt/Code/openclaw` (extensions/browser, 13.8k lines of src), Hermes at `/Users/jt/Code/hermes-agent`, browser-use cloned to `scratchpad/ext/browser-use` (commit 564007d, 2026-09-01), ZeroClaw and Moltis in `scratchpad/ext`.

---

## 0. The short version

- Every serious browser agent in 2026 runs the same loop: **text snapshot with element refs → model picks a ref and an action → harness acts → harness re-reads the page**. Screenshots verify; they are not the main input.
- OpenClaw (Playwright) has the best snapshot/ref machinery. Hermes (`agent-browser` CLI) has the best "use the real profile" story. browser-use has the best **agent loop** (per-step self-evaluation, memory, plan, `done` with a success flag, a judge).
- The benchmark leaders (Online-Mind2Web 90–99%, June 2026) share three things: a frontier model, a hybrid DOM+screenshot view, and explicit verification of the previous step before choosing the next one. They also run in cloud browsers with CAPTCHA solving, which Nerd Genie must not do.
- For social accounts, the safety rule is not fingerprint tricks. It is: **real Chrome binary, JT's own persistent profile per persona, headed, home IP, human pacing and daily caps, never copy cookies around**. Chrome 136 (2025-03) and DBSC (2026) both push in that direction.
- Recommendation for Nerd Genie: **a long-lived spawned `playwright-core` worker (patchright as an opt-in swap) attached over CDP to a real Chrome that Nerd Genie launches with the persona's `--user-data-dir`**. Runner-up: Vercel's `agent-browser` Rust daemon attached the same way. Not recommended: chromedp in-process (you would rewrite Playwright's actionability and snapshot layers; OpenClaw needed 14k lines for that).

---

## 1. OpenClaw: how a browser turn works

### 1.1 Launch and attach

OpenClaw launches its own Chrome on a dedicated profile under `~/.openclaw/browser/<profile>/user-data` (`extensions/browser/src/browser/chrome.ts:753-755`). Launch args (`chrome.ts:775-812`): `--remote-debugging-port`, `--user-data-dir`, `--no-first-run`, `--disable-sync`, `--disable-background-networking`, `--disable-component-update`, `--disable-features=Translate,MediaRouter`, `--password-store=basic`; macOS adds `--use-mock-keychain` (793), headless adds `--headless=new --disable-gpu` (796-797), Linux adds `--disable-dev-shm-usage` (803). It never changes an existing profile's cookie encryption source, which would make old cookies unreadable (`chrome.ts:1011-1015`), and it themes the profile so JT can tell the window apart (`chrome.ts:1154`).

On Linux without `DISPLAY`/`WAYLAND_DISPLAY` managed profiles default to headless (`docs/tools/browser.md:377-378`). Playwright then attaches over CDP (`connectBrowser` in `pw-session-connection.ts`; new pages are tracked via `context.on("page")` at `pw-session-connection.ts:374`).

There are three profile kinds: `openclaw` (managed, isolated), `user` (attach to the real signed-in Chrome via Chrome DevTools MCP), and `chrome` (an extension relay driving the signed-in Chrome; "the only signed-in-browser mode that works with nobody at the computer", `docs/tools/browser.md:407`).

### 1.2 The snapshot the model sees

Two formats. The `ai` format calls Playwright's `page.ariaSnapshot({ mode: "ai" })` (`pw-tools-core.snapshot.ts:282-283` and `415-417`). The raw `aria` format pulls `Accessibility.getFullAXTree` over CDP (`pw-tools-core.snapshot.ts:238-256`) and formats it itself. Both produce YAML-like lines with `[ref=eN]` markers. Real example from the tests (`browser-tool.test.ts:5285`):

```
- button "Sign in" [ref=e1]
- button "Sign out" [ref=e2]
```

and from `cdp.test.ts:955-957`:

```
  - button "Literal [ref=e2]\t\b" [ref=e1]
  - Iframe [ref=e2]
```

Refs inside iframes are frame-qualified (`f2e3`, see `pw-role-snapshot.ts:483-486`). Duplicates get `[nth=N]` (`pw-role-snapshot.ts:364-366`). The `interactive` filter keeps only lines whose role is in `INTERACTIVE_ROLES` (`pw-role-snapshot.ts:335-345`). Size caps: 40,000 chars for a full AI snapshot, 8,000 chars and depth 6 in "efficient" mode (`browser/constants.ts:43-47`). `navigate` returns an efficient interactive snapshot inline so the model does not need a second call (`browser-tool.snapshot.ts:487-503`). Repeated snapshots mark newly appeared ref-bearing elements with `[new]` and return `newElements` (`browser-tool-description.ts:33`; identity keys in `pw-role-snapshot.ts:46-60`).

### 1.3 How a click resolves a ref

`refLocator` (`pw-session-actions.ts:78-111`) normalizes `@e5` / `ref=e5` / `e5`. In `aria` refs mode it returns `page.locator("aria-ref=e5")`, Playwright's own self-resolving selector (line 90; explained at `pw-session-contracts.ts:135-136`). In the default `role` mode it looks up the stored `{role, name, nth}` and builds `getByRole(role, { name, exact: true }).nth(n)` (lines 104-109). An unknown ref throws `Unknown ref "e5". Run a new snapshot and use a ref from that snapshot.` (lines 94-96). Then `clickViaPlaywright` calls `locator.click({ timeout })` (`pw-tools-core.interactions.actions.ts:103-107`); an optional `delayMs` hovers first and sleeps (85-95). Typing uses `locator.fill` unless `slowly` is set, which uses `locator.type(text, { delay: 75 })` (`pw-tools-core.interactions.actions.ts:286-291`).

### 1.4 Waiting after an action

There is no network-idle wait. Every interaction is wrapped by a navigation guard: after the Playwright action returns, it listens for `framenavigated` for a 250 ms grace window (`BROWSER_ACTION_NAVIGATION_GRACE_MS`, `act-policy.ts:39`; the listener loop at `pw-tools-core.interactions.navigation.ts:293-346`). If a main-frame navigation happened it runs the SSRF/policy checks on the final URL. Interaction timeout defaults to 8 s (min 500 ms, max 60 s); explicit `wait` actions default to 20 s and cap at 120 s (`act-policy.ts:29-33`). The prompt tells the model to avoid blind waits and to wait for UI state instead (`skills/browser-automation/SKILL.md`, "Avoid blind waits").

### 1.5 Errors and stale refs

Playwright errors are rewritten into model-friendly text (`pw-tools-core.shared.ts:62-80`): a strict-mode violation becomes `Selector "e5" matched N elements. Run a new snapshot to get updated refs`; a visibility timeout becomes `Element "e5" not found or not visible. Run a new snapshot to see current page elements.` Refs are cleared on main-frame navigation (`pw-session-state.ts:317-352`). A snapshot taken while the frame navigates throws `Frame changed while its browser snapshot was being captured; retry.` (`pw-tools-core.snapshot.ts:311`). The skill file gives the recovery recipe: "Snapshot the same targetId again. Find the current visible control. Retry once with the new ref. If the UI moved to a blocker state, report the blocker instead of looping."

### 1.6 Screenshots and vision

`screenshot` supports `labels=true`, which draws numbered boxes and returns an `annotations` array `{ref, number, role, name?, box}` so the model can map numbers to refs (`SKILL.md` section 3; drawing helper `browser/screenshot-annotate.ts:1-40`). For text-only models, screenshots are described by the configured image model (`docs/tools/browser.md:261-268`).

### 1.7 Dialogs, downloads, uploads, tabs, frames

Dialogs are observed via `page.on("dialog")` (`pw-session-state.ts:287-288`) and answered later through the `dialog` action (`pw-session-state.ts:384-404`); a blocked action reports `blockedByDialog`. `download` clicks a ref and captures the file; `waitfordownload` arms a waiter (`pw-tools-core.downloads.ts:373-425`); uploads use the `filechooser` event (`pw-tools-core.downloads.ts:106, 230-234`). Tabs get stable handles (`t1`) and labels (`browser-tool-description.ts:30`). The `frame` parameter scopes a snapshot to one iframe (`browser-tool.schema.ts:216`; `pw-tools-core.snapshot.ts:438`).

### 1.8 Profile and cookies

`importprofile` (macOS only) reads the system Chrome `Network/Cookies` SQLite file (`browser/system-profiles.ts:80-84, 190-202`) after one Keychain prompt and copies cookies into a fresh managed profile. Local storage and IndexedDB are not copied, and DBSC-bound Google sessions may still need re-auth (`SKILL.md`, "Existing User Browser"). The login doc is blunt: "Do not give the model your credentials: automated logins often trigger anti-bot defenses and can lock the account" and "Sandboxed browser sessions are more likely to trigger bot detection" (`docs/tools/browser-login.md:11-17`).

### 1.9 What the prompt tells the model

Tool schema: 24 actions and 14 act kinds (`browser-tool.schema.ts:19-61`). Key lines from `browser-tool-description.ts`:
- "When using refs from snapshot (e.g. e12), keep the same tab" (line 30)
- "For stable, self-resolving refs across calls, use snapshot with refs=\"aria\"" (line 32)
- "navigate returns the loaded page's compact snapshot inline ... do not call snapshot after navigate" (line 34)
- "Use snapshot+act for UI automation. Avoid act:wait by default" (line 35)

And from `SKILL.md`: "Read before you click", "Act narrowly", "After a single act that triggers navigation, and after modal changes or form submissions, snapshot again", "If the page needs login, permission, captcha, 2FA ... stop and tell the user exactly what is needed."

OpenClaw also ships `extensions/cua-computer` ("Experimental CUA Driver computer control", `openclaw.plugin.json:8`), a screenshot+coordinate desktop driver, separate from the browser tool.

---

## 2. Hermes: how a browser turn works

### 2.1 Launch and attach

Hermes does not talk CDP itself for actions. `_run_browser_command` shells out to the `agent-browser` CLI (`tools/browser_tool.py:3788-3800`). Sessions are per task. Backends: local Chromium via agent-browser, cloud providers (Browserbase etc., `agent/browser_provider.py`), Camoufox over REST (`tools/browser_camofox.py:1-20`), and Lightpanda.

The interesting part is `use_real_profile` (`browser_tool.py:1513-1531`, a consent flag re-read on every call). When on, `_real_profile_cdp` (1643-1922) copies the user's active Chrome profile's auth files — `Cookies`, `Network/Cookies`, `Login Data`, `Login Data For Account`, `Web Data`, `Preferences`, `Local State` (`hermes_cli/browser_connect.py:560-566`; IndexedDB skipped, 525) — into a Hermes-owned dir, launches the **real Chrome binary** on it with `--remote-debugging-port=0` and `--headless=new` by default (1765-1822), reads `DevToolsActivePort` (1835-1850), and attaches agent-browser with `--cdp <port>` (1875). The docstring says why: the copy "sidesteps the Chrome ≥136 default-profile remote-debugging block", and launching Chrome directly avoids agent-browser's forced `--use-mock-keychain`, which would drop keychain-encrypted cookies (1653-1660, 1755-1763).

### 2.2 The snapshot

`browser_snapshot` runs `agent-browser snapshot -c` (compact) unless `full=True` (`browser_tool.py:4504-4525`). agent-browser's format is the Playwright-style tree with `[ref=eN]` (README, https://github.com/vercel-labs/agent-browser, fetched 2026-09-02):

```
- heading "Example Domain" [ref=e1] [level=1]
- button "Submit" [ref=e2]
- textbox "Email" [ref=e3]
```

The threshold is 15,000 chars (`DEFAULT_SNAPSHOT_THRESHOLD`, line 293). Over that, `_truncate_snapshot` cuts at line boundaries, writes the full tree to `cache/web/browser-snapshot-<hash>.txt`, and appends `[... N more lines truncated — full snapshot: read_file path=... offset=... limit=200]` (4165-4212). All browser output passes `_redact_browser_output`, which force-redacts secret-looking strings (4214-4237). `browser_navigate` auto-takes a compact snapshot after loading so the model can act immediately (4480-4495). The CDP supervisor merges pending dialogs and the frame tree into the snapshot (4585-4595).

### 2.3 Click, type, wait

`browser_click` prefixes `@` and runs `agent-browser click @e5` (4608-4646). `browser_type` uses agent-browser `fill` ("clears then types", 4674) — instant, not human paced. `browser_scroll` scrolls 500 px (4706-4753). There is no post-action settle wait in Hermes itself; whatever waiting exists lives inside agent-browser (its changelog v0.27.2 says clicks scroll off-viewport elements into view and handle dialogs; https://agent-browser.dev/changelog, fetched 2026-09-02). Actionability waits beyond that: not verified.

### 2.4 Errors, anti-bot, vision

Errors are passed through as `{"success": false, "error": ...}`. After navigation, the page title is matched against `["access denied", "blocked", "bot detected", "verification required", "are you a robot", "captcha", "cloudflare", "just a moment", ...]` and a `bot_detection_warning` is added suggesting delays or a Browserbase stealth plan (4449-4465). Without proxies it adds `stealth_warning: "Running WITHOUT residential proxies..."` (4470-4477). `browser_vision(question, annotate)` takes a screenshot, attaches it natively if the model has vision, else asks an auxiliary vision model and returns text; `annotate=True` overlays numbered `[N]` labels (5507-5530). The tool description tells the model to use it "especially for CAPTCHAs, visual verification challenges, complex layouts" (2985).

### 2.5 Dialogs, frames, escape hatches

`tools/browser_supervisor.py` keeps one persistent CDP WebSocket per task, subscribes to `Page`/`Runtime`/`Target` events on every attached session including OOPIFs, and exposes pending dialogs and the frame tree (1-18). Default policy is `must_respond` with a 300 s timeout (78-83). `browser_dialog(action=accept|dismiss, prompt_text)` answers (`tools/browser_dialog_tool.py:29-57`). `browser_cdp` is a raw CDP passthrough "for operations not covered by the main browser tool surface — handling native dialogs, iframe-scoped evaluation, cookie/network control" (`tools/browser_cdp_tool.py:1-15`).

### 2.6 Tool surface and prompt

Ten tools: `browser_navigate, browser_snapshot, browser_click, browser_type, browser_scroll, browser_back, browser_press, browser_get_images, browser_vision, browser_console` (`browser_tool.py:6503-6622`), plus `browser_dialog` and `browser_cdp`. The system prompt does not carry a browsing lesson; the only browser-specific line is a prior: when `computer_use` is present and "the app underneath is a browser, that means driving the user's browser rather than opening yours with browser_navigate" (`agent/prompt_builder.py:757-761`). The `computer-use` skill teaches a set-of-marks flow: capture → click by element index → re-capture (`skills/autonomous-ai-agents/computer-use/SKILL.md`).

---

## 3. browser-use: how a turn works, and what it does that the others do not

### 3.1 Launch

browser-use launches Chromium/Chrome with a long arg list (`browser_use/browser/profile.py:162-202`) that includes `--disable-blink-features=AutomationControlled` (201), `--disable-background-timer-throttling` and `--disable-renderer-backgrounding` (166-167, so agents keep working in background tabs), `--disable-ipc-flooding-protection` (180, for tight CDP loops), and `--headless=new` when headless (124-125). `user_data_dir` (557) and `profile_directory` (714) select a profile; when a real profile is given it is **copied to a temp dir** first (850-897). `keep_alive` keeps the browser after the run (640). Since 2025 it talks raw CDP, not Playwright ("Closer to the Metal: Leaving Playwright for CDP", https://browser-use.com/posts/playwright-to-cdp, not dated on the page).

### 3.2 The page representation

Per step it pulls three CDP trees — `DOM.getDocument(depth:-1, pierce:true)` (`dom/service.py:583-584`), `DOMSnapshot.captureSnapshot` with computed styles, paint order and rects (571-575), and `Accessibility.getFullAXTree` (379) — merges them, and serializes only clickable/scrollable/iframe nodes plus visible text (`dom/serializer/serializer.py:976-1183`). Line format: `[index]<tag attr=value />` with child text indented below; `*[` marks elements new since the last step (1070); `|scroll element[` (1067), `|IFRAME|` (1112), `Open Shadow ... Shadow End` (1131-1145); and hints like `... (3 more elements below - scroll to reveal): <button> "Next" ~1.2 pages down` (1174-1183). Attributes kept: `title,type,checked,id,name,role,value,placeholder,alt,aria-label,aria-expanded,aria-checked,pattern,...` (`dom/views.py:18-40`). Text under an open modal is dropped by paint-order filtering (1149-1159; `profile.py:704`).

The message wrapper adds `<page_stats>` ("Page appears empty - consider waiting", pending network requests, counts of links/interactive/iframes/shadow roots), `<page_info>` with "1.2 pages above, 3.4 pages below — scroll down to reveal more content", `[Start of page]`/`[End of page]` markers, a tab list with 4-char ids, and a PDF-viewer notice (`agent/prompts.py:224-314`).

### 3.3 Actions

Registered actions (`browser_use/tools/service.py`): `search` (458), `navigate` (502), `go_back` (583), `wait` (597), `input` (774, "Clears existing text by default"), `upload_file` (860), `switch` (1004), `close` (1030), `extract` (1059, "LLM extracts structured data from page markdown... expensive"), `search_page` (1299, "like grep. Zero LLM cost"), `find_elements` (1336, CSS query), `scroll` (1371, pages), `send_keys` (1476), `find_text` (1495), `screenshot` (1516), `save_as_pdf` (1556), `dropdown_options` (1671), `select_dropdown` (1699), `write_file`/`replace_file`/`read_file` (1746-1788), `evaluate` (1823), `done` (2002/2039), `click` by index or coordinates (2118/2135).

Clicking is pure CDP: `DOM.scrollIntoViewIfNeeded` (`watchdogs/default_action_watchdog.py:768-771`), geometry, then `Input.dispatchMouseEvent` mouseMoved → mousePressed → mouseReleased with 50–80 ms sleeps (907-947). Typing dispatches key events with a 10 ms delay per keystroke (1211-1212). Waits: `minimum_wait_page_load_time` 0.25 s, `wait_for_network_idle_page_load_time` 0.5 s, `wait_between_actions` 0.1 s (`profile.py:691-694`), and the state builder checks pending network requests before serializing (`watchdogs/dom_watchdog.py:244-274`).

### 3.4 The agent loop

Output per step: `thinking`, `evaluation_previous_goal`, `memory`, `next_goal`, optional `current_plan_item`/`plan_update`, then up to 5 actions (`agent/views.py:381-397`; `max_actions_per_step` 5, `max_failures` 5, `use_judge` True, `enable_planning` True at `views.py:59-84`). `multi_act` drops queued actions when the page changed (`agent/service.py:2730-2828`). After 3 consecutive failures a replan nudge is injected (1470-1477); at 75% of the step budget it is told to consolidate (1548-1565). `done` carries `success: bool`, and the prompt forbids `success=true` unless every requirement is verified ("Partial results with success=false are more valuable than overclaiming success", `agent/system_prompts/system_prompt.md`). A judge model grades the trace (`agent/judge.py:1`); history is capped and compacted (`agent/message_manager/service.py:116-180, 234-248`).

Screenshots are optional per step; when present the harness draws numbered bounding boxes in Python (`browser/python_highlights.py:1-5`), and the prompt calls the screenshot "your GROUND TRUTH".

### 3.5 What browser-use does that OpenClaw and Hermes do not

1. **Forced self-evaluation every step** (`evaluation_previous_goal` with a Verdict) plus a running `memory` field. OpenClaw and Hermes never ask the model to grade its last action.
2. **`*[` new-element marks** tied to combobox rules ("type, then WAIT for the suggestions dropdown"). OpenClaw's `[new]` exists but only across repeated snapshots.
3. **Scroll awareness** (`pages above/below`, hidden-element hints) and **load heuristics** from pending network requests.
4. **Action batching that aborts on page change.**
5. **`done` with a success flag, a pre-done checklist, a judge, and plan/todo files.**
6. **Free local tools** (`search_page`, `find_elements`) instead of re-reading the page through the LLM.
7. **Paint-order filtering** hides text under a modal.
8. **CAPTCHA and stealth are outsourced to their cloud** (`watchdogs/captcha_watchdog.py:1-5`; README:261). No local stealth code exists. JS dialogs are auto-accepted (`watchdogs/popups_watchdog.py:14`), which is wrong for a personal agent.

Benchmark claims: README lines 130-132 cite their own 100-task "BU Bench" and "#1 on the Odysseys leaderboard with an 87.4% average" (rubric average, not perfect-task rate). Steel's Online-Mind2Web board lists "Browser Use Cloud (bu-max)" at 97.0% (Mar 2026) but with a custom agentic judge (https://leaderboard.steel.dev/leaderboards/online-mind2web/, updated 2026-06-29).

---

## 4. The others, briefly

**Stagehand (Browserbase).** Three primitives: `act("click add to cart")`, `extract(schema)`, `observe()`. `observe()` returns candidate actions `{description, method, arguments, selector: "xpath=/html[1]/body[1]/..."}` that `act()` can replay later with no LLM call; it "handles iFrame traversal and shadow DOM", and `%password%` variables are "not shared with LLM providers" (https://docs.stagehand.dev/v3/basics/observe and `/act`, fetched 2026-09-02). v3 dropped Playwright for CDP; the observe-act cache is said to be "validated against a DOM hash" (secondary: https://scrapfly.io/blog/posts/stagehand-vs-browser-use, 2026; not in the official docs — not verified).

**Vercel agent-browser.** Rust CLI + Rust daemon speaking CDP directly; the Node/Playwright daemon was removed in v0.20.0 (2026-03-13); v0.36.0 shipped 2026-09-01; daemon memory 8 MB (https://agent-browser.dev/changelog and README, fetched 2026-09-02). Commands: `open, click, fill, type, press, scroll, wait, screenshot, get, find, select, hover, drag, upload, snapshot (-i -c -d -s), eval, tab, frame, dialog, cookies, state, batch`. Options: `--session`, `--profile` (v0.24.1 copies a Chrome profile to a temp dir), `--cdp`, `--auto-connect`, `--executable-path`, `--headed`, `--allowed-domains`. Default browser is Chrome for Testing; Linux ARM64/x64 native; "No built-in stealth mode."

**Anthropic computer use.** Tool type `computer_toolset_20260801` (no beta header on Opus 4.8+/Sonnet 5/Opus 5/Fable); 17 actions: `screenshot`, `zoom` (`region: [x0,y0,x1,y1]`), five click kinds with optional `coordinate`, `left_click_drag`, `mouse_move`, `left_mouse_down/up`, `cursor_position`, `scroll` (`scroll_direction`, `scroll_amount`), `type`, `key`, `hold_key`, `wait`. Images up to 2576 px / 4784 visual tokens; a screenshot costs ~1000–1800 tokens; keep ≤20 images per request; recommended web resolution 1280x800 (https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool, fetched 2026-09-02). The docs suggest `<robot_credentials>` tags for logins; Nerd Genie will not (section 8).

**Claude for Chrome.** Extension launched 2025-08-25; site-level permissions, confirmations before "publishing, purchasing, or sharing personal data", blocked categories; prompt-injection success fell from 23.6% to 11.2% after mitigations (https://claude.com/blog/claude-for-chrome, updated 2025-12-18). Secondary sources say it uses Chrome's debugger API for DOM and clicks plus screenshots (https://www.contextstudios.ai/blog/claude-code-chrome-extension-the-complete-guide-to-browser-native-ai-automation, 2026; not verified against Anthropic docs).

**OpenAI CUA / Operator.** Pure screenshot → reasoning → click/scroll/type loop; asks for confirmation before logins and CAPTCHAs; 58.1% WebArena, 87% WebVoyager, 38.1% OSWorld at launch (https://openai.com/index/computer-using-agent/, 2025-01-23).

**ZeroClaw.** `crates/zeroclaw-tools/src/browser.rs` is a facade over three backends: the `agent-browser` CLI (255-260, 499-509, `snapshot` at 600), a `rust-native` backend, and a `computer_use` sidecar (`ComputerUseConfig`, 18-27). **Moltis.** `crates/browser` uses chromiumoxide; `snapshot.rs` injects JS that collects interactive elements, numbers them, and stores `data-moltis-ref` on each element (56-167), then clicks by ref via center coordinates (257-269). Profiles persist via `user_data_dir(profile_path)` (`pool.rs:677`); it can launch an "Obscura" stealth sidecar (`pool.rs:972-973`).

---

## 5. Comparison table

| | OpenClaw | Hermes | browser-use | Stagehand | agent-browser | Anthropic computer use |
|---|---|---|---|---|---|---|
| Page representation | Playwright aria tree (`ai` mode) or raw AX tree; optional labeled screenshot | agent-browser aria tree, compact by default; screenshot on demand | Merged DOM+AX+layout, `[idx]<tag attrs>` text + optional boxed screenshot | Accessibility/XPath candidates via `observe`; DOM for `extract` | Aria tree with refs | Screenshot only (+zoom) |
| Reference scheme | `e12` / `f2e3` (Playwright aria-ref) or role+name+nth | `@e5` | Numeric index → backendNodeId | XPath selector | `@e5` | Pixel coordinates |
| Action set | 24 actions, 14 act kinds incl. batch, fill, drag, evaluate, pdf, emulate | 10 tools + dialog + raw CDP | ~25 actions incl. extract, search_page, files, evaluate, done | act / extract / observe / agent | ~40 CLI commands | 17 actions |
| Waiting | Playwright actionability + 250 ms navigation grace | Inside agent-browser (not verified) | 0.25 s min + 0.5 s network-idle + pending-request check | Playwright/CDP waits (not verified) | scroll-into-view + dialog handling (changelog) | None; model waits explicitly |
| Verification after action | Model must re-snapshot; `[new]` markers | Model must re-snapshot | Forced `evaluation_previous_goal`; screenshot "ground truth"; judge | `observe` before `act` | None | Docs: "take screenshots after each step" |
| Error recovery | Friendly errors + skill recipe (retry once, then report blocker) | Error string; bot-detection warning | max_failures 5, replan nudge after 3, alt-strategy rules | Cache miss → LLM again | Error JSON | Model decides |
| Profile persistence | Own profile dir per named profile; macOS cookie import; attach to real Chrome | Copies real profile's auth files to a Hermes-owned dir, launches real Chrome | `user_data_dir` copied to temp; `keep_alive` | Browserbase contexts or local | `--profile`, `--session --restore`, `--cdp` | N/A (your environment) |
| Stealth / human-likeness | None; `slowly` typing = 75 ms/char | None locally; Camoufox backend optional; warns about proxies | `AutomationControlled` flag off; 10 ms/key; cloud stealth | Browserbase stealth | None built in | None |
| Vision use | Optional labeled screenshot | `browser_vision` on demand, aux model fallback | Optional per step, boxed | Optional | Screenshot command | Always |
| Cost per step (approx.) | Efficient snapshot ≤8k chars ≈ 2k tokens; full ≤40k chars ≈ 10k | ≤15k chars ≈ 4k tokens | DOM text (capped, `agent/prompts.py:254`) + ~1.2k-token screenshot; typically 3–8k | Small (candidate list) | ≈ Hermes | 1–1.8k tokens per screenshot + history |
| Reliability evidence | None published | None published | Own BU Bench; Odysseys 87.4% rubric avg; Steel O-M2W 97% (custom judge) | Steel O-M2W 65% (Gemini 2.5 CU, Mar 2026) | None | Operator-style agents: 61% human-eval O-M2W (2025); GPT-5.4 native CU 93% (Mar 2026) |

---

## 6. Benchmarks: what the top scorers do differently

- **Online-Mind2Web** (300 tasks, 136 live sites; https://github.com/OSU-NLP-Group/Online-Mind2Web). Steel's board (updated 2026-06-29): Browser Use Cloud bu-max 97.0% (custom judge), GPT-5.4 native computer use 93.0%, ABP + Claude Opus 4.6 90.5%, TinyFish 90.0%, UI-TARS-2 88.2%, Yutori Navigator 78.7% (human eval), Gemini 2.5 Computer Use 69.0%, Stagehand 65.0%. Aside reports 297/300 with GPT-5.5 "high" thinking, 900 s per task, LLM-graded (https://github.com/at-inc/aside-benchmarks, run 2026-05-24). The benchmark's paper ("An Illusion of Progress", arXiv 2504.01382, 2025) found human-judged Operator at 61.3% with others near 30%, and WebJudge agreeing with humans only ~85% of the time. Read the 90%+ numbers as "frontier model + good judge + cloud infrastructure".
- **WebArena** (812 self-hosted tasks): WebTactix (DeepSeek v3.2) 74.3% (https://leaderboard.steel.dev/leaderboards/webarena/, 2026).
- **BrowseComp** (1,266 research questions): GPT-5.6 Sol 92.2%, Kimi K3 91.2%, Claude Opus 5 90.8% as of 2026-09-01 (https://leaderboard.steel.dev/leaderboards/browsecomp/). This measures search+reading, not clicking.
- **Odysseys** (200 long-horizon multi-site tasks, rubric-graded): Opus 4.6 44.5% and GPT-5.4 33.5% perfect-task success, with trajectory efficiency near 1% (arXiv 2604.24964, April 2026). Long tasks are still hard.
- **WebVoyager** (643 tasks, 15 sites): CUA 87% (Jan 2025). Mostly saturated.
- **Mind2Web-Live**: not tracked on the boards I checked — not verified.

What the leaders share: (1) a frontier model with thinking on; (2) a hybrid view: DOM/AX text for targeting, a screenshot for verification; (3) an explicit "did my last action work?" step and a judge; (4) retries with a different strategy rather than the same click; (5) plan/memory for long tasks; (6) cloud browsers with CAPTCHA solving and proxies — which Nerd Genie deliberately gives up, so Nerd Genie should expect lower raw scores on hostile sites and higher trust on JT's own accounts.

---

## 7. Human-likeness and account safety on social sites

The rule for LinkedIn, X, Instagram, Facebook, Reddit is: **behave like JT on JT's machine**. Not "hide that a program is running". Sources:

- **Chrome 136 (2025-03-17)**: `--remote-debugging-port`/`--remote-debugging-pipe` are ignored on the default user data dir; you must pass a non-default `--user-data-dir`, which also uses a different cookie encryption key (https://developer.chrome.com/blog/remote-debugging-port). So an agent cannot attach to JT's everyday profile over CDP; it needs its own profile dir, logged in once by hand, or an extension relay.
- **DBSC** (device-bound session credentials): GA on Windows in Chrome 146 (April 2026), then macOS; Google Workspace rollout from 2026-05-25; Linux not yet covered (https://developer.chrome.com/docs/web-platform/device-bound-session-credentials; https://pbxscience.com/chromes-dbsc-stops-cookie-theft-but-linux-is-left-behind/, 2026). Where DBSC is on, copied cookies die within minutes. Cookie copying (OpenClaw importprofile, Hermes profile snapshot) is a dead end for Google-family logins and will spread. Nerd Genie logs in inside the agent profile and keeps it.
- **Playwright's Chromium is not Chrome**: different TLS/JA3 and codec surface (https://github.com/feder-cr/invisible_playwright/wiki/chromium-is-not-chrome; https://cside.com/blog/headless-browser-detection, 2026). Use the installed Google Chrome binary.
- **The control channel is the loudest signal**: a May 2026 benchmark over 31 targets (Cloudflare Turnstile, DataDome, F5, Radware) found nodriver 0 blocked, Patchright 3, Camoufox 3, vanilla Playwright 5, and named Playwright's startup `Runtime.enable` as the main tell; system Chrome via `channel=chrome` beat bundled Chromium (https://ianlpaterson.com/blog/anti-detect-browser-benchmark-patchright-nodriver-curl-cffi/, 2026-05-13, updated 2026-08-15). Patchright is a drop-in Playwright fork that removes those leaks; nodriver drives Chrome over CDP directly; Camoufox is a Firefox fork with C++-level fingerprint spoofing (https://scrapewise.ai/blogs/playwright-stealth-2026, 2026).
- **Do they matter for a logged-in personal account?** Mostly no. LinkedIn looks at "browser fingerprints, IP and timezone consistency, action pacing, message similarity, and acceptance-rate patterns", DOM-injecting extensions, and "multi-tab actions and mouse events that could not physically come from one user" (https://www.linkednav.com/can-linkedin-detect-automation, 2026). 2025–2026 enforcement hit tools, not careful individuals (Apollo dropped native LinkedIn automation 2026-01-30; HeyReach's page removed March 2026, https://www.joinvalley.co/blog/linkedin-automation-safety-2026). Camoufox/patchright/nodriver exist for scraping at scale from rented IPs; for JT they are optional hardening for Cloudflare-fronted sites. A fresh fingerprint every session is itself suspicious on an account that has always used one machine.
- **Headed beats headless.** New headless shares the cookie store but still leaks window/screen signals and the "HeadlessChrome" tell (https://cside.com/blog/headless-browser-detection, 2026). the 5070 Ti machine has a display: run headed on a dedicated virtual desktop so JT can watch and step in.
- **No cloud proxies.** Home IP + the same timezone/locale as JT is the point.
- **Pacing.** Real LinkedIn users act "with delays between 40 seconds and several minutes", and daily caps exist (https://www.yalc.ai/blog/linkedin-automation-limits-and-rules/, 2026). Nerd Genie needs per-site pacing profiles and caps, and never two social tabs at once.
- **Session hygiene.** One profile per persona, never shared with scrapers; never export cookies; never log in from a second machine with the same profile; keep Chrome updated (fingerprint drift matches JT's real Chrome because it *is* JT's Chrome).

---

## 8. Credential vault and login flow

### 8.1 Primary store: the logged-in profile

Cookies are the login. Each persona has a persistent `--user-data-dir`. First login to each site is done by JT by hand in the agent's Chrome window (as OpenClaw's login doc recommends). After that the agent simply opens the site. Sessions on big sites last weeks to months; when one expires the agent hits the login page and moves to 8.2.

### 8.2 Vault for services used often

Format: one file `~/.nerdgenie/vault.enc`, ChaCha20-Poly1305 with a random 32-byte key at `~/.nerdgenie/vault.key` (0600), fresh 12-byte nonce per record — exactly ZeroClaw's design (`crates/zeroclaw-config/src/secrets.rs:1-22`, `FileKeySource` at 87-125, which also refuses symlinks). `age` with a passphrase would work too but needs the passphrase at daemon start; a key file is simpler for a headless daemon and JT can wrap that file with his login keyring later. Hermes's alternative, pulling from Bitwarden/1Password at startup with a 0600 TTL disk cache (`agent/secret_sources/_cache.py:1-19`, `bitwarden.py`, `onepassword.py`, `command.py`), is a good v2 if JT already uses one of those; the `command` source ("keepassxc-cli") is the cleanest bridge.

Entry shape: `name, domains[], username, password, totp_secret?, notes, allow_autofill (bool), last_used`. Added via the TUI: `/vault add linkedin` with masked input (password and TOTP secret never echoed, never logged). Referenced by name only. The model never receives values; every tool output passes a redactor that masks any vault value and secret-looking strings, like Hermes's `_redact_browser_output` (`browser_tool.py:4214-4237`).

### 8.3 `browser_login(site)` — the harness fills, not the model

Flow:
1. Model calls `browser_login(site: "linkedin")` (or the harness offers it when the URL matches a vault entry's domain and a password field is visible).
2. Harness checks the current page's origin is in the entry's `domains[]`. If not, refuse — this blocks phishing pages and prompt-injected redirects.
3. Harness finds fields by role/attributes: username = textbox with `autocomplete=username|email` or `type=email`, else the first visible textbox above the password box; password = `input[type=password]`. If the model passed refs, use them, but still require the password ref to be a `type=password` field.
4. Harness types with the human pacing profile (section 9.9), presses Enter or clicks the submit button, waits for settle.
5. If a TOTP field appears (`autocomplete=one-time-code`, label matches /code|verification|2fa/i) and the entry has a TOTP secret, harness computes RFC 6238 TOTP and fills it.
6. Result to the model: `{ ok, site, filled: ["username","password","totp"], url_after, blocked_by: null | "captcha" | "new_device_code" | "unknown_field" }`. No values.

Hand off to JT when: the site has no vault entry; a CAPTCHA/Turnstile is visible; the site asks for an emailed/SMS code (the model may propose reading the code from Gmail through the browser if JT allowed that persona to; otherwise ask); a password-change or "unusual activity" screen appears; or the login fails twice. The handoff sends a Signal message with a screenshot and, for codes, accepts the code back over Signal/TUI and types it — the model still never sees it.

### 8.4 sudo

Hermes caches a sudo password per session after a masked prompt (`tools/terminal_tool.py:247-340`), pipes it with `sudo -S -p ''` on stdin (764, 1035-1037) with a trailing newline (1112). Nerd Genie's equivalent: vault entry `sudo`. Run privileged commands with `SUDO_ASKPASS=/usr/libexec/nerdgenie-askpass sudo -A ...`, where the askpass helper reads the secret from the Nerd Genie daemon over a per-invocation unix socket token. This keeps the password off stdin (so nested pipelines still work) and out of the process list and model context. First sudo in a session requires a yes/no confirmation from JT over TUI/Signal; after that it is silent until the session ends. Never print sudo output before redaction.

---

## 9. THE Nerd Genie BROWSER SPEC

### 9.1 Engine choice for Linux

| Option | Primitives | Aria snapshot quality | Auto-wait | Real Chrome + profile attach | Maintenance | Linux/ARM | Verdict |
|---|---|---|---|---|---|---|---|
| **playwright-core worker** (Node, spawned once, JSON-RPC over stdio) | Everything: locators, `aria-ref`, frames, shadow DOM, dialogs, downloads, filechooser, mouse/keyboard with delays, network events | Best available; the `ai` mode with `[ref=eN]` and frame refs is what OpenClaw and Playwright MCP use (`pw-tools-core.snapshot.ts:282`) | Yes (visible, enabled, stable, receives events) | `connectOverCDP` to any Chrome, no bundled browser needed | Microsoft; monthly releases | Yes (npm, no browser download) | **Recommend** |
| **agent-browser daemon** (Rust, `--cdp` attach) | Fixed CLI command set incl. snapshot/refs, dialogs, frames, tabs, state | Same style as Playwright; reimplemented in Rust; edge cases still being fixed (v0.25.3 hidden radio inputs) | Partial (scroll-into-view, dialogs); full actionability not verified | Yes (`--cdp`, `--executable-path`, `--profile`) | Vercel; very active (v0.36 on 2026-09-01) | Native ARM64/x64 | **Runner-up** |
| **chromedp in-process (Go)** | Raw CDP only; you build snapshots, refs, waits, dialogs, downloads | You write it (Moltis did a JS-injected numbered list) | No (dev.to/rosgluk, glukhov.org, Feb 2026) | Yes | Community | Yes | Not recommended |

Why not chromedp: the value is in the 10k+ lines around CDP (OpenClaw's `browser/` is where the reliability lives). Why not agent-browser first: no control over mouse paths, keystroke cadence, or settle logic beyond its CLI, and its snapshot is still moving. Why playwright-core despite +85 MB / +135 ms (prior benchmark): it is one long-lived helper per persona, started once, not per call — noise on a 60 GB desktop. `patchright` (same API, removes `Runtime.enable` and other leaks) is a config-flag swap for Cloudflare-heavy sites; parity of its `ariaSnapshot({mode:"ai"})` is not verified.

Attach model: Nerd Genie launches `/usr/bin/google-chrome --user-data-dir=~/.nerdgenie/browser/<persona>/user-data --remote-debugging-port=0 --no-first-run --no-default-browser-check --disable-sync --disable-background-networking --disable-component-update --password-store=basic --window-size=1400,900` (headed on the 5070 Ti machine's display; `--headless=new` only for the throwaway persona), reads `DevToolsActivePort` like Hermes (`browser_tool.py:1835-1850`), and the worker does `chromium.connectOverCDP("http://127.0.0.1:<port>")`. No `--disable-blink-features=AutomationControlled` games; real Chrome with a real profile does not set `navigator.webdriver` unless driven by WebDriver.

### 9.2 Tool set (10 tools)

Descriptions are the model-facing text (≤40 words). Every tool returns `{ok, tab, url, title, summary, snapshot?}`; `snapshot` is the compact interactive view (cap 6,000 chars) unless noted.

1. **browser_open** — "Open a URL in the current or a new tab, wait for it to settle, and return the page snapshot. Use for navigation; do not call browser_read afterwards." Inputs: `url`, `new_tab?`, `tab?`. Output cap 6k chars.
2. **browser_read** — "Read the current page. mode=interactive (default, controls with @refs), full (all text and controls, 30k cap), text (article prose), find (lines matching query), screenshot (labeled image), zoom (region at full resolution)." Inputs: `mode`, `query?`, `region?`, `frame?`, `tab?`.
3. **browser_click** — "Click a control by @ref from the latest snapshot, or by x,y from a labeled screenshot. Returns what changed. If the page did not change, try a different control or a key press." Inputs: `ref | x,y`, `button?`, `double?`, `expect?`.
4. **browser_type** — "Type text into the field @ref like a person (paced). Clears the field first unless append=true. submit=true presses Enter. Never type passwords; use browser_login." Inputs: `ref`, `text`, `append?`, `submit?`, `expect?`.
5. **browser_press** — "Press a key or chord in the focused element: Enter, Escape, Tab, ArrowDown, Control+a. Use for dropdowns, dialogs and shortcuts." Inputs: `keys`, `repeat?`.
6. **browser_scroll** — "Scroll the page or a scrollable @ref by pages (default 1) up/down, or scroll @ref into view. Returns the new snapshot with [above]/[below] counts." Inputs: `direction?`, `pages?`, `ref?`.
7. **browser_act** — "Less common actions: select (option text), hover, drag (from,to), upload (ref, paths), dialog (accept|dismiss, text), wait_for (text|url|ref gone, ≤20s), back, forward, reload, download (ref)." Inputs: `kind` + kind-specific fields.
8. **browser_tabs** — "List tabs, switch to one, close one. Tabs have stable ids t1, t2... The snapshot header shows the active tab." Inputs: `action: list|switch|close`, `tab?`.
9. **browser_login** — "Log into the current site using the vault entry named `site`. The harness fills username, password and 2FA itself; you never see them. Only works on the entry's domains. Reports if a CAPTCHA or device check blocks it." Inputs: `site`, `username_ref?`, `password_ref?`.
10. **browser_handoff** — "Pause and ask JT to do something in the visible browser (CAPTCHA, verification code, unknown site login, risky action). Include what you need and what to check afterward. Resumes when JT replies." Inputs: `reason`, `instructions`, `timeout_min?`.

Not tools: skill record/replay (harness commands, 9.13), `evaluate` (kept off by default; add later behind a per-persona flag as OpenClaw does with `evaluateEnabled`).

### 9.3 Page representation

Hybrid: a compact accessibility tree with `@refs` (Playwright `ai` mode, post-processed) plus an optional labeled screenshot. Text the model sees for `browser_read` / after every action:

```
tab t2/3  https://www.linkedin.com/messaging/  "Messaging | LinkedIn"
viewport 1400x900  scroll 0.0 above / 2.3 below  dialogs: none  changed: +3 new, url same
- navigation "Primary"
  - link "Home" @e1
  - link "Messaging" @e3 [current]
  - button "Me" @e4 [expanded=false]
- main
  - textbox "Search messages" @e5
  - list "Conversations" (12 items, 4 shown)
    - link "Dana Ortiz — Re: coffee next week · 2h" @e6 [new]
    - link "Sam Lee — Thanks!" @e7
  - region "Dana Ortiz"
    - text "Sounds good, does Thursday work?"
    - textbox "Write a message…" @e9 [multiline] [new]
    - button "Send" @e10 [disabled]
- iframe "Ads" @f1 (collapsed; read with frame=f1)
… 14 more controls below the fold (browser_scroll down)
```

Rules: refs are Playwright aria-refs (`e9`, `f1e2`), so they self-resolve across calls on the same document without a lookup table. State attributes are kept (`[checked]`, `[disabled]`, `[expanded=…]`, `[selected]`, `[current]`, `[multiline]`, `[value="…"]` up to 60 chars). `[new]` marks elements not present in the previous snapshot of the same document (OpenClaw's identity keys, browser-use's `*[`). Long lists collapse to "(N items, k shown)" and the hidden count with the scroll hint (browser-use's hidden-elements hint). Text under an open dialog/modal is dropped (browser-use paint-order filtering; Playwright's `ai` mode already omits most offscreen content — not verified for modals). Prose is trimmed to 120 chars per text node in `interactive` mode, untrimmed in `text` mode.

### 9.4 The act loop

For `browser_click`, `browser_type`, `browser_press`, `browser_act`:

1. **Resolve.** `page.locator("aria-ref=e9")`. If it is not in the current document (stale) go to 9.5.
2. **Prepare.** Scroll into view if needed; Playwright actionability waits (visible, enabled, stable, not covered), 8 s budget.
3. **Act with pacing** (9.9): bezier mouse move to a point inside the box (not the exact center), 60–120 ms press, typing per character with jitter.
4. **Settle.** Wait for the first of: main-frame navigation (then `domcontentloaded`), or 300 ms with no DOM mutations and at most 2 in-flight requests for 500 ms; hard cap 3 s (5 s after navigation). This is browser-use's network-idle check plus OpenClaw's 250 ms navigation grace, made explicit.
5. **Re-snapshot and diff.** Compare with the pre-action snapshot: url, title, focused element, counts of new/removed refs, dialog opened, new tab opened, download started, aria-live/alert text. Produce `changed: url changed | +3 new | dialog | nothing`.
6. **Verify.** If the caller passed `expect` (`{url_contains, text, ref_gone, ref_visible}`), check it and report `verified: true|false`. Without `expect`, `changed: nothing` after a click is flagged: "No visible change. The control may need a different action."
7. **Retry (harness, silent).** On "nothing" after a click, retry once by coordinate; on "not visible", scroll into view and retry once. Then return; the model decides. Never the same action more than twice in a row (browser-use loop rule).
8. **Return** the result with the fresh compact snapshot so the next step needs no extra read (OpenClaw's inline snapshot after navigate, generalized).

### 9.5 Stale refs

A ref is stale when the document changed (navigation, SPA route, big re-render). The harness stores, at snapshot time, each ref's `{role, name, nth, frame}`. On a stale ref: re-snapshot; if exactly one element matches role+name(+nth) in the new document, use it and add `note: "re-resolved e9 by role/name"`; otherwise return `error: stale_ref` with the new snapshot. Refs from a previous tab are rejected with the tab id in the message. Batch actions abort on the first stale ref (OpenClaw batch semantics).

### 9.6 Per-step token budget

Target ≤ 3,000 input tokens of page state per step: compact snapshot ≤ 6,000 chars (~1,500 tokens), result summary ≤ 300 chars, screenshot (1280x800 JPEG q70) ≈ 1,100 tokens when requested. `full` mode ≤ 30,000 chars, and anything over is written to a scratch file with a `read_file offset=` pointer (Hermes's truncate-and-store). Keep only the last two snapshots verbatim in context; collapse older browser tool results to their one-line `summary` (Hermes/OpenClaw both let old snapshots bloat context; browser-use compacts). At 75% of the step budget, inject a consolidate warning.

### 9.7 When to use vision

The harness attaches a labeled screenshot automatically when: the snapshot has fewer than 10 refs on a page with more than 20 DOM elements (canvas/map/image-heavy); the same action produced `changed: nothing` twice; a CAPTCHA/Turnstile pattern is detected (title/text heuristics like Hermes 4449-4457, plus an iframe from `challenges.cloudflare.com`/`recaptcha`); a `select`/slider/drag failed; or the model asks (`browser_read mode=screenshot|zoom`). Labels are numbers drawn on the boxes with an `annotations` map to refs (OpenClaw). Coordinate clicks are allowed only from a labeled screenshot taken this step. For `text`-only local models (Ollama), screenshots go through a local vision model and come back as text (OpenClaw's media-model fallback).

### 9.8 Tabs, windows, popups

One Chrome window per persona, many tabs. Stable ids `t1, t2…`; the header shows `tab t2/3`. Refs are tab-scoped. A click that opens exactly one new tab auto-switches to it and says so in `changed`; more than one → listed, not switched. Popups are tracked via `context.on("page")`. The agent may hold at most one tab per social site open. Idle tabs older than 30 minutes are closed on task end (OpenClaw "tab hygiene").

### 9.9 Human pacing profile (per persona, per site)

Defaults: curved mouse paths of 200–600 ms; click hold 60–120 ms; typing 60–140 ms per character with 300–900 ms pauses at spaces and punctuation, plus a 2% typo-and-backspace on long free text (off for validated form fields); 800–2,500 ms between actions; a reading dwell of 1.5 s + 8 ms per visible character (cap 12 s) after each new page; wheel-step scrolling of 100–300 px. Site classes: `social` (LinkedIn, X, Instagram, Facebook, Reddit): 4–20 s between actions, dwell ×2, max 40 actions/hour and 250/day, active hours 08:00–23:00, one tab; `webapp` (Gmail, banking, admin panels): normal pacing, no caps; `utility` (docs, search): fast, no mouse curves, headless allowed on the throwaway persona. The harness enforces caps and reports `paused_until` to the model.

### 9.10 Persona / profile model

`~/.nerdgenie/browser/<persona>/{user-data, downloads, pacing.yaml, vault-scope}`. Personas: `jt` (JT's identity; social and personal accounts; headed; home IP; vault scope = all entries), `work` (if needed), `scratch` (no logins, headless allowed, may use patchright, no vault). One Chrome process per active persona, launched on demand, kept alive 30 minutes idle (browser-use `keep_alive`), never two personas on the same site at once. Profiles are never copied or exported; backups are file-level and encrypted. JT can open the persona window himself any time (`nerdgenie browser open jt https://…`) to log in or fix something; the agent sees the result on its next snapshot.

### 9.11 Downloads, uploads, dialogs, iframes, shadow DOM, PDF

- Downloads: `download` events are always captured into the persona's `downloads/` with a sanitized name and listed in the result as `downloads: [path]`; `browser_act kind=download ref=` waits for a specific one.
- Uploads: `browser_act kind=upload ref paths[]` via `filechooser`; paths must be inside `~/nerdgenie-share/` or a path JT sent in this conversation.
- Dialogs: policy `must_respond` (Hermes). A pending `alert/confirm/prompt/beforeunload` shows in the header (`dialogs: confirm "Leave page?"`) and the model answers with `browser_act kind=dialog`.
- Iframes: same-process frames appear inline with `fNeM` refs; cross-origin frames show as `- iframe "name" @f2 (collapsed)` and are read with `frame=f2`; known ad/tracker frames stay collapsed.
- Shadow DOM: open roots are pierced by Playwright locators; closed roots are reported as `[closed shadow]` and need coordinates.
- PDF: Chrome's viewer has no useful AX tree, so an `application/pdf` response is saved, run through `pdftotext -layout`, and returned as text (first 30k chars, file pointer, page count).

### 9.12 The handoff tool

`browser_handoff` brings the persona window to the front on the 5070 Ti machine, sends JT a Signal message with a 1280-px screenshot, the reason, and a numbered list of what to do, then blocks the task (default 15 minutes, max 2 hours). JT replies `done`, `code 123456`, or `abort`. Codes are typed by the harness into the field the model named (or the only visible one-time-code field). While handed off, the agent takes no browser actions on that tab.

### 9.13 Skill recording and replay

- **Record** (`/record start linkedin-inbox-triage` in the TUI, or the model proposes after finishing a task JT called repeatable). The harness logs each step: intent (from the model's next_goal), tool call, resolved element descriptor `{role, name, nth, css, text, near_text, frame, bbox}`, `expect` result, url pattern. On stop, the model writes `skills/linkedin-inbox-triage.yaml`: parameters, steps, per-step expectations, pacing class, allowed domains.
- **Replay** runs with no LLM: for each step, resolve the element by cascade (role+name → css → text → near_text within 150 px), act, wait for settle, check the expectation. This is Stagehand's observe-cache idea plus OpenClaw's batch semantics.
- **Self-heal**: on a failed resolve or expectation, the harness snapshots, asks the model (cheap tier) to find the element for the recorded intent, verifies the expectation passes, and proposes a diff to the skill. The diff is applied only after JT approves in the TUI/Signal ("selector for step 4 changed from 'button Reply' to 'button Respond' — accept?"). Three consecutive heals on one skill flag it for a full re-record.
- Skills carry `max_actions` and site class, so replays inherit pacing and caps.

### 9.14 What the prompt should tell the model (short)

Read before you click. Act on one intent per step; batch only form fields. State whether the last action worked before choosing the next. Never type a password; call browser_login. If you see a CAPTCHA, code request, or an unknown login page, call browser_handoff. Do not retry the same action more than twice. Prefer the site's own search and filters over scrolling. Report exactly what you did and what you could not verify.

---

## 10. Not verified / open questions

- agent-browser's actionability waits and its `type` cadence — only the changelog was checked.
- patchright's parity with playwright-core `ariaSnapshot({mode:"ai"})`.
- Whether Playwright's `ai` snapshot hides content under modals (browser-use does this explicitly).
- Stagehand's "DOM hash" cache validation is from secondary blogs, not the official docs.
- Claude for Chrome's internals (debugger API) are from secondary sources.
- Running the persona window headed while JT's own desktop session is locked: an Xvfb+VNC fallback may be needed on the 5070 Ti machine; untested.
- Mind2Web-Live current leaderboard: not found on the boards checked.
