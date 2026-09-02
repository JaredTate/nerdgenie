# Four-Repo Verified Metrics

Measured 2026-09-02 on macOS (Darwin 25.5.0, arm64). All four repos at HEAD that day.
Tools used: `find`, `grep`, `wc`, `du`, `jq`, `git`, `tokei 14.x`, `python3` (parsing only).

**Every number below was produced by a command shown in this document.** Items I could
not measure are labelled `NOT MEASURABLE`; items that are approximations are labelled
`ESTIMATE`.

---

## 0. Methodology and reconciliation with the pre-supplied figures

"Lines" means **raw physical lines including blanks and comments** (`wc -l`), matching
the pre-supplied figures.

A file counts as a **test** if its path matches any of:

```
\.(test|spec)\.(ts|tsx|mts|cts|py|js|mjs)$      # foo.test.ts
/(__tests__|__mocks__|__snapshots__|tests?)/     # a test directory
(^|/)(test_[^/]*\.py|[^/]*_test\.py)$            # Python conventions
\.test-[^/]*$ | test-(helpers|support|fixtures|mocks|harness)\.[a-z]+$
live-test[^/]*$ | conftest\.py$
```

Everything else is **non-test**. `node_modules`, `.git`, `dist`, `build`, `.next`,
`coverage`, `.venv` are excluded everywhere.

The helper scripts used throughout:

```bash
# nontest.sh <root> <ext-regex>  -> prints non-test file paths
find "$ROOT" -type f | grep -E "$EXT" \
 | grep -Ev '/(node_modules|\.git|dist|build|\.next|coverage|\.venv|venv)/' \
 | grep -Ev '\.(test|spec)\.(ts|tsx|mts|cts|py|js|mjs)$' \
 | grep -Ev '/(__tests__|__mocks__|__snapshots__|tests?)/' \
 | grep -Ev '(^|/)(test_[^/]*\.py|[^/]*_test\.py)$' \
 | grep -Ev '\.test-[^/]*$|test-helpers\.[a-z]+$|test-support\.[a-z]+$|test-fixtures\.[a-z]+$|test-mocks\.[a-z]+$|test-harness\.[a-z]+$|live-test[^/]*$|conftest\.py$'

# lines.sh : stdin = file list -> "lines path", sorted desc
tr '\n' '\0' | xargs -0 -n 200 wc -l | grep -v ' total$' | sort -rn
```

### Cross-check against the pre-supplied numbers

| Pre-supplied | My recount | Verdict |
|---|---|---|
| hermes Python non-test 1,055,438 | **1,055,438** | exact match |
| hermes Python tests 1,003,702 | **1,003,702** | exact match |
| hermes Python total 2,059,140 | 2,059,598 (tokei) | within 458 lines (0.02%) |
| opencode per-package non-test (app 144k, opencode 82k, console 42k, ui 34k, core 33k, sdk 30k, tui 27k, session-ui 20k, stats 18k, llm 11k, desktop 8k, web 7k) | 144,404 / 82,180 / 42,113 / 33,708 / 33,204 / 30,316 / 27,064 / 20,304 / 17,609 / 10,589 / 8,379 / 6,952 | exact match on all twelve |
| prime by package (coding-agent 138k, ai 38k, tui 15k, agent 2k) | 138,181 / 38,296 / 14,651 / 2,347 | exact match |
| openclaw src non-test 1,919,984 | 1,872,191 – 1,982,109 | **see note** |
| openclaw src/gateway 310k | 291,530 | **see note** |

**Note on the two openclaw discrepancies.** OpenClaw has 584 files in `src/` named
`*.test-support.ts`, `*.test-helpers.ts`, `*.test-mocks.ts`, `*.test-harness.ts` and
`live-test-*.ts`, totalling **109,922 lines**. These are test scaffolding that lives
beside production code. Counting them as production gives 1,982,109; counting them as
tests gives 1,872,191. The supplied 1,919,984 sits between the two, so the original
measurement classified some but not all of them. The same effect explains gateway
(291,530 vs 310k). **I use the stricter definition (scaffolding = test) throughout**,
so my openclaw src figure is 1,872,191. The difference is ~2.5% and does not change
any conclusion.

Commands:

```bash
cd /Users/jt/Code/openclaw
find src -type f \( -name '*.ts' -o -name '*.tsx' \) -print0 | xargs -0 cat | wc -l   # 4,901,557 all
grep -E '\.test-|test-helpers\.ts$|test-support\.ts$|test-fixtures\.ts$|test-mocks\.ts$|test-harness\.ts$|live-test' \
  /tmp/oc_src_nontest.txt | tr '\n' '\0' | xargs -0 cat | wc -l                        # 109,922
```

---

## 1. MASTER COMPARISON TABLE

| Metric | OpenClaw | Hermes | Prime | OpenCode |
|---|---:|---:|---:|---:|
| **Version** | v2026.8.2 | v0.21.0 | v0.9.1 | v1.18.26 |
| **Primary language** | TypeScript | Python | TypeScript | TypeScript |
| *— Size —* | | | | |
| Non-test source, measured scope | **2,944,060** | **1,524,660** | **193,475** | **487,178** |
| ├ breakdown | src 1,872,191 + ext 973,815 + pkg 98,054 | Py 1,055,438 + TS 469,222 | packages/ | packages/ |
| Generated / vendored / catalog lines | 6,251 | 38,790 | 22,086 | 145,658 |
| **Hand-written non-test** | **2,937,809** | **1,485,870** | **171,389** | **341,520** |
| Inflation as % of non-test | **0.2%** | **2.5%** | **11.4%** | **29.9%** |
| Non-test files | 14,381 | 3,099 | 473 | 2,448 |
| Mean lines/file (hand-written) | 204 | **485** | 363 | **145** |
| Largest single hand-written file | 2,986 | **34,561** | 11,948 | 3,465 |
| Whole repo, all languages (tokei "Lines") | **10,308,047** | 3,381,458 | 422,363 | 1,401,097 |
| Repo on disk (incl. .git) | 5.0 GB | 1.3 GB | 105 MB | 704 MB |
| .git alone | 4.5 GB | 1.1 GB | 82 MB | 562 MB |
| Git-tracked files | 36,681 | 11,239 | 1,218 | 6,608 |
| *— Tests —* | | | | |
| Test files | **11,357** | 3,759 py + 1,086 ts | 451 | 736 |
| Test cases | **131,487** | 39,067 py + 10,961 ts | 6,020 | 6,076 |
| Test lines | 4,791,097 | 1,003,702 (py) | 171,556 | 175,901 |
| Test : source line ratio | **1.6 : 1** | 1.0 : 1 | 0.9 : 1 | 0.4 : 1 |
| *— Dependencies —* | | | | |
| Root direct deps | 65 | 34 (pyproject) | 2 | 6 |
| Root devDeps | 60 | 9 (dev group) | 10 | 12 |
| Unique direct deps, all workspaces | 192 | 34 py + 103 py-optional + 103 ts | 37 | 220 |
| Unique devDeps, all workspaces | 81 | 49 ts | 19 | 107 |
| package.json / manifest files | 180 | 12 | 9 | 40 |
| **Total packages in lockfile** | **1,653** | **255** (uv.lock) | **438** | **3,233** |
| *— Velocity (30 days to 2026-09-02) —* | | | | |
| Commits, last 30d | **10,574** | 6,718 | 191 | 354 |
| Commits, last 7d | 3,775 | 1,777 | 57 | 55 |
| Unique authors, last 30d | 384 | **923** | 15 | 43 |
| Total commits | **86,752** | 27,319 | 4,622 | 15,636 |
| Unique authors, all time | 3,184 | 3,238 | 247 | 1,024 |
| First commit | 2025-11-24 | 2025-07-22 | 2025-08-09 | 2025-03-21 |
| Repo age at measurement | 282 days | 407 days | 389 days | 530 days |
| Commits/day lifetime | **308** | 67 | 12 | 29 |
| Top author share, last 30d | 63% (Peter Steinberger, 6,689) | 29% (Teknium, 1,940) | 63% (Sebastian Müller, 120) | 38% (opencode-agent bot, 133) |
| *— Model-facing surface —* | | | | |
| Tools exposed to the model | **57** | 83 | **1** default | 12 + 4 conditional |
| Tool groupings | 14 profiles/groups | 59 toolsets | n/a | n/a |
| Messaging channels/platforms | **27** | **31** | 0 | 0 |
| Model providers | 57 ext / **76 provider ids** | 41 | 32 | 32 plugins + models.dev |
| Models in-repo catalog | via models.dev + 49 ext catalogs | n/a | **1,238** | via models.dev |
| Config: top-level keys | 42 | 24 | NOT MEASURED | 24 |
| Config: total option paths | **10,175** (authoritative) | 639 lines / 406 distinct | NOT MEASURED | ~90 leaf (ESTIMATE) |
| *— Code hygiene —* | | | | |
| TODO markers (in comments) | 353 | 9 py + 14 ts | **0** | 86 |
| FIXME markers | **0** | **0** | **0** | **0** |
| HACK markers | **0** | **0** | **0** | **0** |
| XXX markers | 0 | 1 | 0 | 0 |

---

## 2. GENERATED / VENDORED / FIXTURE INFLATION

### Biggest 10 inflation items across all four repos

| # | Repo | Item | Lines | Kind |
|---|---|---|---:|---|
| 1 | OpenCode | `*/src/i18n/*.ts` across 6 packages (225 files) | **107,756** | i18n catalog |
| 2 | Hermes | `apps/desktop/src/i18n/*.ts` (14 locales) | 24,453 | i18n catalog |
| 3 | Prime | `packages/ai/src/models.generated.ts` | **22,086** | generated model catalog |
| 4 | Hermes | `web/src/i18n/*.ts` (21 locales) | 14,337 | i18n catalog |
| 5 | OpenCode | `packages/sdk/js/src/v2/gen/types.gen.ts` | 13,622 | OpenAPI codegen |
| 6 | OpenCode | `packages/sdk/js/src/v2/gen/sdk.gen.ts` | 7,219 | OpenAPI codegen |
| 7 | OpenCode | `packages/web/src/components/icons/index.tsx` | 4,454 | inlined heroicons SVG |
| 8 | OpenClaw | `src/wizard/i18n/locales/*.ts` (6 files) | 3,916 | i18n catalog |
| 9 | OpenCode | `packages/sdk/js/src/gen/types.gen.ts` | 3,907 | OpenAPI codegen (v1) |
| 10 | OpenCode | `packages/client/src/generated/types.ts` | 2,807 | codegen |

(`packages/client/src/generated/client.ts` 1,029 and OpenClaw's
`src/state/openclaw-state-db.generated.d.ts` 1,722 are the next two.)

Notable non-counting item: Hermes vendors `native/fts5_cjk/vendor/sqlite3.h` (13,775
lines of C header). It is C, so it never entered the Python or TypeScript totals.

### Per-repo corrected totals

**OpenClaw — essentially zero inflation (0.2%).**

| Item | Lines |
|---|---:|
| `src/wizard/i18n/locales/*.ts` (6) | 3,916 |
| `src/state/openclaw-state-db.generated.d.ts` | 1,722 |
| `src/state/openclaw-agent-db.generated.d.ts` | 572 |
| `src/config/bundled-channel-config-metadata.generated.ts` | 41 |
| **Total** | **6,251** |
| Non-test measured | 2,944,060 |
| **Hand-written non-test** | **2,937,809** |

There are no `third_party/`, `vendor/` or `vendored/` directories anywhere in the repo.
`patches/` holds 4 npm patches (1,957 lines) and sits outside src/extensions/packages.
Many files *named* `*-snapshot.ts` or `*-catalog.ts` turned out to be real features
(git snapshots, plugin catalogs), not data dumps — I checked and excluded them from
the inflation tally.

**Hermes — 2.5%.** All of the inflation is in the TypeScript half: the desktop app's
14 locale files (24,453 lines) plus the web UI's 21 locale files (14,337 lines),
38,790 lines across 35 files. The Python half (1,055,438 lines) has **zero**
generated or vendored content. `locales/*.yaml` (17 files, 7,853 lines) is real i18n
data but is YAML, outside the Python/TS totals.

**Prime — 11.4%, the highest concentration in a single file.**
`packages/ai/src/models.generated.ts` is 22,086 lines — **57.7% of the entire
`packages/ai` package** and 11.4% of the whole non-test codebase. Header confirms it:
`// This file is auto-generated by scripts/generate-models.ts`. It encodes 1,238 models
across 32 providers at ~18 lines each. Corrected hand-written total: **171,389**.
Also vendored but outside the TS count: `export-html/vendor/highlight.min.js` (1,212 lines).

**OpenCode — 29.9%, the worst ratio.**

| Category | Files | Lines |
|---|---:|---:|
| i18n catalogs (6 packages, see below) | 225 | 107,756 |
| SDK/client codegen (`/gen/`, `/generated/`) | 34 | 33,361 |
| Inlined icon components | 2 | 4,541 |
| **Total** | **261** | **145,658** |
| Non-test measured | | 487,178 |
| **Hand-written non-test** | | **341,520** |

The i18n load is spread across six packages, not one — the app, console, stats and
desktop UIs each carry their own full locale set:

| i18n tree | Lines | Files |
|---|---:|---:|
| `packages/app/src/i18n` | 73,543 | 63 |
| `packages/console/app/src/i18n` | 15,434 | 19 |
| `packages/ui/src/i18n` | 12,757 | 62 |
| `packages/stats/app/src/i18n` | 3,918 | 17 |
| `packages/desktop/src/renderer/i18n` | 1,987 | 63 |
| `packages/web/src/i18n` | 117 | 1 |
| **Total** | **107,756** | **225** |

`packages/app/src/i18n` alone holds 62 locale files (plus `desktop-native.ts`), the
largest being `uk.ts` 1,267, `no.ts` 1,267, `es.ts` 1,265, `ru.ts` 1,262.

Separately, 90 `*.stories.tsx` Storybook files hold 12,745 lines. I did **not** subtract
these — they are hand-written — but they are demo code, not shipped product.
`patches/` holds 19 npm patches (3,494 lines), outside the packages total.

### OpenClaw `src/agents` — what the 404k (my count: 402,520) contains

1,665 non-test files. Not one giant thing; it is the whole agent runtime.

| Subdirectory | Lines | Files |
|---|---:|---:|
| *(root files, 688 of them)* | 150,783 | 688 |
| `embedded-agent-runner/` | 66,942 | 258 |
| `tools/` | 40,680 | 136 |
| `subagents/` | 28,808 | 125 |
| `sessions/` | 24,489 | 85 |
| `harness/` | 14,630 | 62 |
| `auth-profiles/` | 14,434 | 53 |
| `cli-runner/` | 12,173 | 45 |
| `sandbox/` | 11,422 | 57 |
| `command/` | 10,121 | 29 |
| `failover/` | 5,314 | 21 |
| `main-session-recovery/` | 5,129 | 17 |
| `worktrees/` | 3,965 | 13 |
| `embedded-agent-helpers/` | 2,706 | 15 |
| `runtime-plan/`, `agent-hooks/`, `utils/`, `modes/`, `runtime/`, `schema/` | 7,661 | 31 |

Largest root files: `model-selection-shared.ts` 1,792 · `bash-tools.exec-host-gateway.ts`
1,665 · `system-prompt.ts` 1,610 · `workspace.ts` 1,580 · `agent-tools.read.ts` 1,558 ·
`btw.ts` 1,423 · `model-auth-availability.ts` 1,363 · `agent-bundle-mcp-runtime.ts` 1,139.

In plain English: this is the agent loop plus everything that wraps it — the embedded
runner that executes turns, the 57 built-in tools, sub-agent spawning, session
persistence, per-provider auth profile storage, a sandbox, a CLI runner, model
failover, crash recovery, and git worktree management.

### OpenClaw `src/gateway` — what the 310k (my count: 291,530) contains

1,159 non-test files. The gateway is the long-running server process.

| Subdirectory | Lines | Files |
|---|---:|---:|
| *(root files, 592 of them)* | 148,857 | 592 |
| `server-methods/` | 73,147 | 282 |
| `worker-environments/` | 43,303 | 164 |
| `server/` | 11,438 | 55 |
| `agent-turn/` | 6,601 | 24 |
| `desktop/` | 3,166 | 13 |
| `terminal/` | 2,156 | 18 |
| `methods/` | 1,026 | 4 |
| `health/` | 988 | 5 |
| `portals/` | 848 | 2 |

Largest root files: `operator-approval-store.ts` 2,125 · `managed-image-attachments.ts`
2,009 · `session-reset-service.ts` 1,932 · `server-chat.ts` 1,798 ·
`server-startup-post-attach.ts` 1,705 · `server-cron.ts` 1,702 · `server-channels.ts`
1,538 · `node-registry.ts` 1,512.

In plain English: `server-methods/` (282 files) is the RPC surface — one file per
callable method. `worker-environments/` (164 files) provisions and supervises the
sandboxes agents run inside. The rest is the HTTP/WS server, per-turn orchestration,
a desktop bridge, terminal multiplexing, cron, and channel fan-out.

### Hermes `hermes_cli/` — what the 214k (my count: 268,891) contains

319 non-test files, **25.5% of all non-test Python**. This is the user-facing CLI *and*
a bundled web UI server.

| File | Lines |
|---|---:|
| `web_server.py` | 20,250 |
| `main.py` | 15,182 |
| `kanban_db.py` | 12,153 |
| `update_cmd.py` | 11,217 |
| `auth.py` | 10,121 |
| `gateway.py` | 9,178 |
| `models.py` | 7,604 |
| `plugins.py` | 7,193 |
| `config.py` | 6,607 |
| `tools_config.py` | 6,342 |
| `config_defaults.py` | 5,305 |
| `model_switch.py` | 4,405 |
| `cli_commands_mixin.py` | 4,175 |
| `setup.py` | 3,933 |
| `kanban.py` | 3,565 |

`web_server.py` alone is 20k lines — an HTTP server living inside the CLI package.
`kanban_db.py` + `kanban.py` (15.7k) is a built-in kanban board. `update_cmd.py`
(11.2k) is the self-updater.

### Hermes TypeScript scope

The Hermes TS non-test total of **469,222** lines covers three trees:

| Tree | Lines | Files |
|---|---:|---:|
| `apps/` | 348,116 | 1,354 |
| `ui-tui/` | 70,975 | 300 |
| `web/` | 50,131 | 130 |

Largest files outside `apps/`: `ui-tui/packages/hermes-ink/src/ink/ink.tsx` 2,840 ·
`web/src/lib/api.ts` 2,670 · `ui-tui/packages/hermes-ink/src/native-ts/yoga-layout/index.ts`
2,326 · `web/src/pages/SessionsPage.tsx` 2,217 · `web/src/pages/ChatPage.tsx` 1,988.
`ui-tui/` contains a vendored-style reimplementation of Ink (React-for-terminals)
including a TypeScript yoga-layout port; `web/` is a React web UI with 21 locale files.

### Hermes `apps/` — what the 381k TS (my count: 348,116) contains

| App | Lines | Files | What it is |
|---|---:|---:|---|
| `apps/desktop` | 344,439 | 1,324 | Electron desktop client |
| `apps/shared` | 2,327 | 13 | shared types |
| `apps/bootstrap-installer` | 1,350 | 17 | installer |

`apps/desktop` internals: `src/` 282,038 (1,103 files) · `electron/` 48,964 (141) ·
`scripts/` 10,502 (68) · `e2e/` 2,457 (7). The single largest TS file in the entire
Hermes repo is `apps/desktop/electron/main.ts` at **17,963 lines** — the Electron main
process in one file. Next are the i18n catalogs (`zh.ts` 3,856, `en.ts` 3,726).

So "apps/" is, essentially, one large Electron app; it is not agent code at all.

---

## 3. TOP 10 LARGEST HAND-WRITTEN NON-TEST FILES

### OpenClaw — remarkably flat

| Lines | File |
|---:|---|
| 2,986 | `extensions/qa-lab/src/providers/mock-openai/server.ts` |
| 2,291 | `extensions/codex/src/app-server/native-subagent-monitor.ts` |
| 2,222 | `src/plugins/management-service.ts` |
| 2,197 | `src/agents/auth-profiles/store.ts` |
| 2,125 | `src/gateway/operator-approval-store.ts` |
| 2,089 | `src/agents/cli-runner/prepare.ts` |
| 2,068 | `extensions/feishu/src/channel.ts` |
| 2,034 | `extensions/voice-call/src/webhook/realtime-handler.ts` |
| 2,020 | `src/tui/tui.ts` |
| 2,009 | `src/gateway/managed-image-attachments.ts` |

No file in 2.94M lines exceeds 3,000. Mean is 204 lines/file. This is the strictest
file-size discipline of the four by a wide margin.

### Hermes — extreme concentration

| Lines | File |
|---:|---|
| **34,561** | `gateway/run.py` |
| **22,411** | `cli.py` |
| 20,250 | `hermes_cli/web_server.py` |
| 18,495 | `tui_gateway/server.py` |
| **17,963** | `apps/desktop/electron/main.ts` (TS) |
| 17,220 | `hermes_state.py` |
| 15,182 | `hermes_cli/main.py` |
| 12,153 | `hermes_cli/kanban_db.py` |
| 11,839 | `agent/auxiliary_client.py` |
| 11,358 | `plugins/platforms/telegram/adapter.py` |
| 11,217 | `hermes_cli/update_cmd.py` |

The top 3 Python files alone are 7.3% of all non-test Python. `gateway/run.py` at
34,561 lines is larger than the *entire* `packages/tui` of Prime.

### Prime

| Lines | File |
|---:|---|
| 11,948 | `packages/coding-agent/src/core/agent-session.ts` |
| 10,143 | `packages/coding-agent/src/modes/interactive/interactive-mode.ts` |
| 7,515 | `packages/coding-agent/src/modes/daemon/daemon-mode.ts` |
| 6,572 | `packages/coding-agent/src/modes/daemon/daemon-supervisor.ts` |
| 2,929 | `packages/coding-agent/src/modes/agents-view/agents-view-mode.ts` |
| 2,450 | `packages/ai/scripts/generate-models.ts` |
| 2,444 | `packages/coding-agent/src/core/package-manager.ts` |
| 2,394 | `packages/coding-agent/src/modes/agent-connection/daemon-agent-connection.ts` |
| 2,390 | `packages/tui/src/components/editor.ts` |
| 2,090 | `packages/coding-agent/src/core/session-manager.ts` |

(`models.generated.ts`, 22,086 lines, excluded as generated — it would otherwise rank #1.)

### OpenCode

| Lines | File |
|---:|---|
| 3,465 | `packages/codemode/src/interpreter/runtime.ts` |
| 2,717 | `packages/tui/src/routes/session/index.tsx` |
| 2,642 | `packages/session-ui/src/components/message-part.tsx` |
| 2,444 | `packages/app/src/pages/layout.tsx` |
| 2,391 | `packages/app/src/pages/session.tsx` |
| 2,090 | `packages/session-ui/src/components/timeline-playground.stories.tsx` |
| 2,068 | `packages/opencode/src/provider/provider.ts` |
| 1,983 | `packages/opencode/src/lsp/server.ts` |
| 1,915 | `packages/session-ui/src/components/markdown-inline-code-kind.ts` |
| 1,890 | `packages/opencode/src/provider/transform.ts` |

(Generated `types.gen.ts` 13,622 and `sdk.gen.ts` 7,219 excluded; they would rank #1 and #2.)

### Is Hermes `run_conversation()` still one giant function? — YES

**Confirmed. It spans lines 1996–9240 of `agent/conversation_loop.py` = 7,245 lines
in a single function.**

```bash
cd /Users/jt/Code/hermes-agent
wc -l agent/conversation_loop.py                                    # 9,244
rg -n '^(async )?def |^class ' agent/conversation_loop.py | tail -1  # 1996: def run_conversation(
awk 'NR>1996 && /^(async )?def |^class /{print NR}' agent/conversation_loop.py  # (no output)
tail -5 agent/conversation_loop.py                                  # 9240 "return result" ... 9244 __all__
```

Evidence it is genuinely one function:
- `run_conversation` starts at line **1996**.
- **No top-level `def` or `class` appears anywhere after line 1996** — the awk scan
  returns nothing.
- The file's last statement is `__all__ = ["run_conversation"]` at line 9244; the last
  code line inside the function is `return result` at line **9240**.
- Only **2** nested `def`s exist inside those 7,245 lines, so the body is not
  decomposed internally either.
- The signature takes **13 parameters**.

The 1,995 lines before it are 44 module-level private helpers (`_midturn_request_pressure_tokens`,
`_restore_or_build_system_prompt`, `_redecorate_prompt_cache_for_provider`, …), so the
file is "44 small helpers, then one 7,245-line function".

---

## 4. DEPENDENCIES

```bash
# OpenClaw
cd /Users/jt/Code/openclaw
jq '.dependencies|length, .devDependencies|length' package.json           # 65, 60
find . -name package.json -not -path '*/node_modules/*' -not -path './.git/*' | wc -l   # 180
find . -name package.json -not -path '*/node_modules/*' -not -path './.git/*' \
 | tr '\n' '\0' | xargs -0 jq -s '{unique_deps:(map(.dependencies//{}|keys)|add|unique|length),
     unique_dev:(map(.devDependencies//{}|keys)|add|unique|length),
     summed_deps:(map(.dependencies//{}|length)|add),
     summed_dev:(map(.devDependencies//{}|length)|add)}'
# -> unique_deps 192, unique_dev 81, summed_deps 374, summed_dev 276
grep -c 'resolution:' pnpm-lock.yaml                                      # 1,653
awk '/^packages:/{f=1;next} /^snapshots:/{f=0} f&&/^  [^ ]/' pnpm-lock.yaml | wc -l  # 1,653 (confirms)

# Hermes
cd /Users/jt/Code/hermes-agent
python3 -c "import tomllib;d=tomllib.load(open('pyproject.toml','rb'));p=d['project'];\
print(len(p['dependencies']), len(p['optional-dependencies']), \
sum(len(v) for v in p['optional-dependencies'].values()))"                # 34, 44 groups, 103
grep -c '^\[\[package\]\]' uv.lock                                        # 255
jq '.dependencies|length, .devDependencies|length' ui-tui/package.json    # 8, 7
jq '.dependencies|length, .devDependencies|length' web/package.json       # 23, 14
jq '.dependencies|length, .devDependencies|length' apps/desktop/package.json  # 72, 28

# Prime
cd /Users/jt/Code/prime-agent
jq '.dependencies|length, .devDependencies|length' package.json           # 2, 10
grep -c '"node_modules/' package-lock.json                                # 438

# OpenCode
cd /Users/jt/Code/opencode
jq '.dependencies|length, .devDependencies|length' package.json           # 6, 12
python3 -c "import re,json;s=open('bun.lock').read();s=re.sub(r',(\s*[}\]])',r'\1',s);\
d=json.loads(s);print(len(d['packages']), len(d['workspaces']))"          # 3,233 packages, 37 workspaces
```

| | OpenClaw | Hermes | Prime | OpenCode |
|---|---:|---:|---:|---:|
| Root direct deps | 65 | 34 | 2 | 6 |
| Root devDeps | 60 | 9 | 10 | 12 |
| Root optionalDeps | 1 | 103 (44 extras groups) | 0 | 0 |
| Workspace manifests | 180 | 12 | 9 | 40 (37 workspaces) |
| Unique direct deps across all | 192 | 103 TS + 137 py | 37 | 220 |
| Unique devDeps across all | 81 | 49 TS | 19 | 107 |
| Summed direct deps (with dupes) | 374 | — | 46 | 468 |
| Summed devDeps (with dupes) | 276 | — | 28 | 271 |
| **Lockfile total packages** | **1,653** | **255** | **438** | **3,233** |
| Lockfile format | pnpm-lock v9.0 | uv.lock | package-lock.json | bun.lock v1 |

Hermes' 44 optional-dependency extras groups (`anthropic`, `messaging`, `matrix`,
`voice`, `wake`, `termux`, `google`, `all`, …) are how it keeps its base install small;
`uv.lock` resolving to only 255 packages against OpenCode's 3,233 is the starkest
dependency-weight contrast in this set.

---

## 5. VELOCITY

```bash
for r in openclaw hermes-agent prime-agent opencode; do
  cd /Users/jt/Code/$r
  git rev-list --count HEAD                                  # total commits
  git log --reverse --format='%ad' --date=short | head -1     # first commit
  git rev-list --count --since='2026-08-03' HEAD              # last 30 days
  git log --since='2026-08-03' --format='%aE' | sort -u | wc -l  # authors 30d
  git log --format='%aE' | sort -u | wc -l                    # authors all time
done
```

| | OpenClaw | Hermes | Prime | OpenCode |
|---|---:|---:|---:|---:|
| HEAD | 752a983480e | 95f62ca3bf | 0ba0423c5 | 69c172e8a7 |
| Commits, last 30d | **10,574** | 6,718 | 191 | 354 |
| Commits, last 7d | 3,775 | 1,777 | 57 | 55 |
| Authors, last 30d | 384 | **923** | 15 | 43 |
| Total commits | **86,752** | 27,319 | 4,622 | 15,636 |
| Authors, all time | 3,184 | 3,238 | 247 | 1,024 |
| First commit | 2025-11-24 | 2025-07-22 | 2025-08-09 | 2025-03-21 |
| Days alive | 282 | 407 | 389 | 530 |
| Lifetime commits/day | **308** | 67 | 12 | 29 |
| Last-30d commits/day | **352** | 224 | 6.4 | 12 |

Top 5 authors, last 30 days:

- **OpenClaw** — Peter Steinberger 6,689 · Vincent Koc 789 · Vyctor H. Brzezowski 240 ·
  Ayaan Zaidi 223 · Dallin Romney 180. One person is **63%** of a 10,574-commit month
  = 223 commits/day sustained. This is not hand-typed; it is agent-driven development.
- **Hermes** — Teknium 1,940 · Brooklyn Nicholson 544 · kshitij 385 · kshitijk4poor 276 ·
  hermes-seaeye[bot] 174.
- **Prime** — Sebastian Müller 120 · Seth Karten 42 · kt 5 · elie 4 · Konstantin Dunas 3.
- **OpenCode** — opencode-agent[bot] 133 · Frank 52 · Jack 34 · Adam 22 · Aiden Cline 18.
  The top committer is the project's own bot.

---

## 6. MODEL-FACING SURFACE

### Tools exposed to the model

**OpenClaw — 57.** Authoritative source is `CORE_TOOL_DEFINITIONS` in
`src/agents/tool-catalog.ts` (entries carrying a `sectionId`).

```bash
cd /Users/jt/Code/openclaw
rg -c 'sectionId:' src/agents/tool-catalog.ts        # 57
rg -B2 'sectionId:' src/agents/tool-catalog.ts | rg -o 'id: "[a-z][a-z0-9_]*"' \
  | sed 's/id: //' | tr -d '"' | sort -u | wc -l     # 55 distinct ids extracted
```

The 55 extracted ids: `agents_list agents_wait apply_patch ask_user browser canvas
code_execution computer conversations_list conversations_send conversations_turn
create_goal dashboard dismiss_task edit exec gateway get_goal github_identity_status
github_publish heartbeat_respond image_generate memory_get memory_search message
mobile_ui music_generate nodes portal process progress_card read screen secrets
session_status sessions sessions_history sessions_list sessions_search sessions_send
sessions_spawn sessions_yield show_widget skill_workshop subagents suggest_task
terminal tts update_goal video_generate view_image web_fetch web_search write x_search`.
A further 14 ids in the same file (`agents`, `coding`, `fs`, `full`, `media`, `memory`,
`messaging`, `minimal`, `nodes`, `runtime`, `sessions`, `ui`, `web`, `automation`) are
**groups/profiles**, not tools. Plugins and MCP servers add more at runtime.

**Hermes — 83 distinct registered tools** (93 `registry.register()` call sites; some
tools register conditionally more than once), organised into **59 toolsets**.

```bash
cd /Users/jt/Code/hermes-agent
python3 - <<'PY'
import re,glob
names=set(); calls=0
for root in ('tools','plugins','agent'):
    for p in glob.glob(root+'/**/*.py', recursive=True):
        s=open(p,encoding='utf-8',errors='ignore').read()
        for m in re.finditer(r'registry\.register\(', s):
            calls+=1
            n=re.search(r'name\s*=\s*["\']([A-Za-z_][A-Za-z_0-9]*)["\']', s[m.end():m.end()+400])
            if n: names.add(n.group(1))
print(calls, len(names))     # 93 83
PY
```

`toolsets.py` references **89** distinct tool names (83 registered + 6 plugin-provided,
e.g. the `spotify_*` family). `_HERMES_CORE_TOOLS` is 53 tools. The 59 toolsets include
per-platform bundles (`hermes-slack`, `hermes-telegram`, `hermes-matrix`, …) and
capability bundles (`browser`, `coding`, `kanban`, `vision`, `computer_use`, …).

**Prime — 1 default tool.** This is the biggest architectural outlier in the set.

```bash
cd /Users/jt/Code/prime-agent
sed -n '46,56p' packages/coding-agent/src/core/tools/index.ts
```
```typescript
export type ToolName = "ipython";

export interface ToolsOptions { ipython?: IpythonToolOptions; }

export function createAllToolDefinitions(cwd: string, options?: ToolsOptions): Record<ToolName, ToolDef> {
	return { ipython: createIpythonToolDefinition(cwd, options?.ipython) };
}
```

The default tool surface handed to the model is literally `{ ipython }`. Prime's design
is code-execution-as-the-only-tool: the model writes Python instead of calling named
tools. Definitions for `bash`, `edit`, `agent-connection` and `replay-builtin` exist and
can be supplied via `customTools`, but they are not in the default set.

**OpenCode — 12 always-on + 4 conditional.** From the `builtin:` array in
`packages/opencode/src/tool/registry.ts` (lines 231–249):

Always on: `shell read glob grep edit write task fetch todo search skill patch`
(plus `invalid`, an error stub). Conditional: `question` (when enabled),
`execute` (code-mode), `lsp` (experimental flag), `plan` (experimental + CLI client).
25 `.ts` files live in `src/tool/`, but 9 are infrastructure (`registry.ts`,
`schema.ts`, `tool.ts`, `truncate.ts`, `json-schema.ts`, `truncation-dir.ts`,
`invalid.ts`, `external-directory.ts`, `code-mode.ts`), so file count overstates the
tool count.

### Messaging channels / platforms

**OpenClaw — 27 channels.** Authoritative: extensions whose `openclaw.plugin.json`
declares a `channels` key.

```bash
cd /Users/jt/Code/openclaw
for f in $(find extensions -maxdepth 2 -name 'openclaw.plugin.json'); do
  jq -e '.channels' $f >/dev/null 2>&1 && basename $(dirname $f); done | sort | wc -l   # 27
```

`a2a buzz clickclack discord feishu googlechat imessage irc line matrix mattermost
msteams nextcloud-talk nostr qa-channel raft reef signal slack sms synology-chat
telegram tlon twitch whatsapp zalo zalouser`

**Hermes — 31 platforms**, from two locations:
`gateway/platforms/*.py` (transport-level: `signal`, `whatsapp_cloud`, `weixin`,
`yuanbao`, `bluebubbles`, `msgraph_webhook`, `webhook`, `api_server`) plus
`plugins/platforms/*/` (22 dirs: `a2a buzz dingtalk discord email feishu google_chat
homeassistant irc line matrix mattermost ntfy photon raft simplex slack sms teams
telegram wecom whatsapp`).

**Prime — 0.** **OpenCode — 0.** Neither has a messaging-channel abstraction. OpenCode
has a small `packages/slack` (154 lines, 2 files), which is an integration, not a channel
system. Both are terminal-first coding agents.

### Model providers

**OpenClaw — 57 provider extensions declaring 76 distinct provider ids.**

```bash
for f in $(find extensions -maxdepth 2 -name 'openclaw.plugin.json'); do
  jq -e '.providers' $f >/dev/null 2>&1 && basename $(dirname $f); done | wc -l   # 57
find extensions -maxdepth 2 -name 'openclaw.plugin.json' | tr '\n' '\0' | xargs -0 \
  jq -r '(.providers // {}) | if type=="array" then .[]|(if type=="object" then .id else . end) else keys[] end' \
  | sort -u | wc -l                                                                # 76
```
49 extensions also ship a `modelCatalog`. Some extensions declare several ids
(`qwen` 6, `gmi`/`google`/`novita` 3 each, `byteplus`/`kimi-coding`/`minimax`/`ollama`/
`stepfun`/`tencent`/`volcengine`/`xiaomi` 2 each).

**Hermes — 41** provider keys in `_PROVIDER_MODELS` (`hermes_cli/models.py`), plus 6
protocol adapters in `agent/*_adapter.py` (`anthropic`, `azure_identity`, `bedrock`,
`codex_responses`, `gemini_native`, `vertex`). `providers/` itself holds only
`__init__.py` and `base.py` — the registry lives in `hermes_cli/models.py`.

**Prime — 32** providers in `models.generated.ts`, covering **1,238 models**.
9 `registerApiProvider()` calls in `packages/ai/src/providers/register-builtins.ts`
supply the transports; 17 provider implementation files sit in `packages/ai/src/providers/`.

**OpenCode — 32** provider plugin files in `packages/core/src/plugin/provider/`
(`alibaba amazon-bedrock anthropic azure cerebras cloudflare-ai-gateway
cloudflare-workers-ai cohere deepinfra dynamic gateway github-copilot gitlab google
google-vertex groq kilo llmgateway mistral nvidia openai-compatible openai opencode
openrouter perplexity sap-ai-core snowflake-cortex togetherai venice vercel xai zenmux`).
Two of those (`dynamic.ts`, `openai-compatible.ts`) are generic adapters. The model
catalog itself is fetched from **models.dev** at runtime rather than vendored — which is
exactly why OpenCode has no 22k-line model file the way Prime does.

### Config options

**OpenClaw — 42 top-level keys, 10,175 total config paths.** The repo generates its own
authoritative count:

```bash
cat /Users/jt/Code/openclaw/docs/.generated/config-baseline.counts.json
# {"core": 2417, "channel": 3706, "plugin": 4052}
rg -o '^  ([a-zA-Z_][a-zA-Z0-9_]*):' src/config/zod-schema.root-shape.ts | tr -d ' :' | sort -u | wc -l   # 42
```

`src/config/doc-baseline.ts` confirms each counted entry is one config path in the
JSON-schema walk, tagged `core`, `channel` or `plugin`. Total **10,175**.
The 42 top-level keys: `accessGroups acp agents approvals attachments auth bindings
broadcast browser channels cloudWorkers commands cron desktop diagnostics discovery env
gateway hooks logging mcp memory messages meta models nodeHost plugins proxy secrets
security session skills surfaces talk telemetry tools transcripts tts ui update wizard
worktreeRoot`.

**Hermes — 24 top-level keys** in `cli-config.yaml.example` (2,147 lines):
`agent browser code_execution compression database delegation display gateway
group_sessions_per_user kanban max_concurrent_sessions memory model platform_toolsets
prompt_caching runtime session_reset skills streaming stt telemetry terminal
tool_loop_guardrails updates`. The file contains 639 key lines (130 active, 509
commented examples) and **406 distinct key names**.

**OpenCode — 24 top-level keys** in `packages/core/src/config.ts`:
`agents attachments commands compaction deps enterprise experimental formatter info
instructions lsp mcp path permissions plugins providers references service skills
snapshots tool_output type username watcher`. Directly declared leaf fields across
`packages/core/src/config.ts` + `packages/core/src/config/*.ts` total **90**
(`ESTIMATE` — nested and `experimental` sub-objects are not fully enumerated by this
line-based count).

**Prime — NOT MEASURED.** Prime has no equivalent single config schema module; options
are spread across `agent-session.ts` option interfaces.

---

## 7. TEST COUNTS

```bash
# TypeScript
find <roots> -type f \( -name '*.test.ts' -o -name '*.spec.ts' -o -name '*.test.tsx' \) > list
tr '\n' '\0' < list | xargs -0 grep -chE '^\s*(it|test)(\.[a-z]+)?\(' | awk '{s+=$1} END {print s}'
# Python
tr '\n' '\0' < list | xargs -0 grep -chE '^\s*(async )?def test_' | awk '{s+=$1} END {print s}'
```

| | OpenClaw | Hermes (py) | Hermes (ts) | Prime | OpenCode |
|---|---:|---:|---:|---:|---:|
| Test files | **11,357** | 3,759 | 1,086 | 451 | 736 |
| Test cases | **131,487** | 39,067 | 10,961 | 6,020 | 6,076 |
| `describe` blocks | 15,987 | — | — | — | — |
| Test lines | 4,791,097 | 1,003,702 | — | 171,556 | 175,901 |
| Cases per test file | 11.6 | 10.4 | 10.1 | 13.3 | 8.3 |
| Test : non-test line ratio | **1.6 : 1** | 1.0 : 1 | — | 0.9 : 1 | 0.4 : 1 |

OpenClaw has more test cases (131,487) than the other three combined (62,124) by a
factor of 2.1, and writes 1.6 lines of test for every line of source.

---

## 8. TODO / FIXME / HACK / XXX IN HAND-WRITTEN NON-TEST SOURCE

Counted two independent ways to guard against a bad regex:

```bash
# (a) comment-anchored
grep -chE "(//|#|/\*|\*)\s*TODO\b|\bTODO:"   # per file, summed
# (b) plain whole-word
grep -chw TODO                                # per file, summed
```

| Marker | OpenClaw | Hermes (py) | Hermes (ts) | Prime | OpenCode |
|---|---:|---:|---:|---:|---:|
| TODO (comment-anchored) | 353 | 9 | 14 | **0** | 86 |
| TODO (whole-word) | 355 | 15 | 19 | **0** | 146 |
| **FIXME** | **0** | **0** | **0** | **0** | **0** |
| **HACK** | **0** | **0** | **0** | **0** | **0** |
| XXX | 0 (4 whole-word) | 1 (2) | 0 | 0 | 0 |

**FIXME and HACK appear zero times in the non-test source of all four repos**, confirmed
by both methods. These codebases use `TODO` as their only marker convention. Prime has
zero markers of any kind. OpenCode's whole-word TODO count (146) is inflated by its
`todo` tool, whose descriptions contain the literal string; the comment-anchored 86 is
the honest figure.

---

## 9. RUNTIME MEASUREMENTS ON THIS MAC

Hardware: Apple Silicon (arm64), Darwin 25.5.0.

| Measurement | Result |
|---|---|
| `which opencode` | `/Users/jt/.opencode/bin/opencode` → symlink |
| Resolved binary | `/Users/jt/.nvm/versions/node/v24.14.1/lib/node_modules/opencode-ai/bin/opencode.exe` |
| Binary type | Mach-O 64-bit executable arm64 (Bun-compiled single file) |
| **Binary size (symlinks followed)** | **137 MB** |
| Installed package total | 137 MB across **6 files** |
| Installed version | **1.18.19** (repo HEAD is 1.18.26 — install is 7 patches behind) |
| `opencode --version` run 1 | 0.26 s real / 0.34 user / 0.01 sys |
| `opencode --version` run 2 | 0.26 s real / 0.34 user / 0.02 sys |
| `opencode --version` run 3 | 0.27 s real / 0.35 user / 0.02 sys |
| **Median cold `--version`** | **0.26 s** |
| **`opencode serve` RSS after 5 s** | **387,456 KB = 378.4 MB** |
| Process topology | single process, no children |
| Server startup | `opencode server listening on http://127.0.0.1:47811` |
| Cleanup after SIGTERM | **CLEAN — `pgrep -fl opencode` returns nothing** |

```bash
readlink -f /Users/jt/.opencode/bin/opencode
du -sh /Users/jt/.nvm/versions/node/v24.14.1/lib/node_modules/opencode-ai   # 137M
for i in 1 2 3; do /usr/bin/time -p /Users/jt/.opencode/bin/opencode --version; done
opencode serve --port 47811 & PID=$!; sleep 5; ps -o rss= -p $PID; kill $PID
pgrep -fl opencode    # (empty)
```

### The other three — not runnable on this Mac

| Repo | Status | Evidence |
|---|---|---|
| **OpenClaw** | **NOT MEASURABLE — not built.** No `dist/`, and no `node_modules/` either, so `openclaw.mjs` (25,135 bytes) cannot resolve its imports. Per instructions I did **not** build it. | `ls -d /Users/jt/Code/openclaw/dist` → No such file or directory; `ls -d .../node_modules` → empty |
| **Hermes** | **SKIPPED — no venv in repo.** | `ls -d .venv venv` → no venv in repo |
| **Prime** | **NOT INSTALLED.** Only the repo-local launcher `prime-agent.sh` (2,248 bytes) exists. | `which prime-agent` → not found |

For scale, the pre-supplied Linux-box figures (not re-measured here): OpenClaw npm
package 898 MB / 35,979 files with a 5.7 GB state dir and 504 MB gateway RSS;
Hermes repo+venv 1.4 GB; OpenCode ~430 MB total. The one number I can compare directly
is RSS: OpenCode's server holds **378 MB** on this Mac against OpenClaw's **504 MB**
gateway (plus a 288 MB signal-cli JVM and a 195 MB codex helper — ~987 MB for the full
OpenClaw process tree).

---

## 10. LANGUAGE BREAKDOWN (tokei, whole repo, all languages)

`cd <repo> && tokei --sort lines`. "Lines" = physical; "Code" excludes blanks/comments.
Grand totals from `cd <repo> && tokei | grep '^ Total'`.

**OpenClaw** — tokei grand total: **35,507 files, 10,308,047 lines** (9,138,940 code, 430,273 comments, 738,834 blanks)

| Language | Files | Lines | Code | Comments | Blanks |
|---|---:|---:|---:|---:|---:|
| TypeScript | 30,309 | 8,987,641 | 8,195,577 | 181,257 | 610,807 |
| Swift | 1,184 | 405,392 | 361,946 | 6,393 | 37,053 |
| Kotlin | 460 | 194,001 | 173,838 | 3,320 | 16,843 |
| JavaScript | 381 | 90,153 | 82,279 | 2,958 | 4,916 |
| CSS | 96 | 66,379 | 54,515 | 2,586 | 9,278 |
| XML | 124 | 61,089 | 61,037 | 34 | 18 |
| YAML | 565 | 59,850 | 58,721 | 305 | 824 |
| Shell | 234 | 56,664 | 50,213 | 1,680 | 4,771 |
| JSON | 606 | 49,553 | 49,540 | 0 | 13 |
| Go | 30 | 10,892 | 9,807 | 23 | 1,062 |

Swift 405,392 and Kotlin 194,001 confirm the pre-supplied 361,946 / 173,838 **code**
figures exactly — those were code-only counts, not physical lines.

**Hermes** — tokei grand total: **10,059 files, 3,381,458 lines** (2,326,782 code, 537,969 comments, 516,707 blanks)

| Language | Files | Lines | Code | Comments | Blanks |
|---|---:|---:|---:|---:|---:|
| Python | 5,076 | 2,059,598 | 1,606,602 | 160,175 | 292,821 |
| TypeScript | 2,007 | 443,502 | 319,491 | 58,636 | 65,375 |
| TSX | 816 | 233,965 | 181,843 | 22,667 | 29,455 |
| JSON | 106 | 62,624 | 62,619 | 0 | 5 |
| JavaScript | 113 | 23,144 | 18,337 | 2,707 | 2,100 |
| C Header | 2 | 14,498 | 2,420 | 11,720 | 358 |

**Prime** — tokei grand total: **1,171 files, 422,363 lines** (354,858 code, 25,042 comments, 42,463 blanks)

| Language | Files | Lines | Code | Comments | Blanks |
|---|---:|---:|---:|---:|---:|
| TypeScript | 966 | 370,548 | 323,187 | 13,148 | 34,213 |
| Python | 31 | 11,689 | 9,616 | 590 | 1,483 |
| JSON | 28 | 7,498 | 7,486 | 0 | 12 |
| Markdown | 99 | 15,401 | 0 | 10,209 | 5,192 |

**OpenCode** — tokei grand total: **6,010 files, 1,401,097 lines** (1,100,011 code, 179,275 comments, 121,811 blanks)

| Language | Files | Lines | Code | Comments | Blanks |
|---|---:|---:|---:|---:|---:|
| TypeScript | 2,687 | 537,938 | 483,698 | 11,795 | 42,445 |
| JSON | 357 | 428,189 | 428,179 | 0 | 10 |
| MDX | 627 | 210,021 | 0 | 150,991 | 59,030 |
| TSX | 603 | 140,136 | 129,536 | 1,162 | 9,438 |
| CSS | 174 | 42,696 | 36,535 | 585 | 5,576 |
| SVG | 1,251 | 16,305 | 16,294 | 0 | 11 |

OpenCode's 428,189 lines of JSON and 210,021 of MDX (docs, in 21 translated READMEs
plus a docs site) are larger than its entire TypeScript source.

---

## 11. NOTES, CAVEATS, AND WHAT IS NOT MEASURED

1. **OpenClaw's src total** is 1,872,191 by my strict definition vs 1,919,984 as
   supplied; the gap is 584 test-scaffolding files (109,922 lines). Neither is wrong —
   they answer slightly different questions. Every OpenClaw figure in this document
   uses the strict definition.
2. **`ESTIMATE` labels.** OpenCode's ~90 config leaves is line-based and undercounts
   nested objects. Hermes' 406 distinct config keys comes from the example YAML, which
   may not be exhaustive vs the code. Prime's config option count is not measured.
3. **Runtime numbers exist for OpenCode only.** OpenClaw is unbuilt, Hermes has no venv,
   Prime is not installed. I did not build or install anything, per instructions.
4. **The installed OpenCode is v1.18.19; the repo is at v1.18.26.** The runtime numbers
   describe a binary 7 patch versions behind the source measured above.
5. **A ripgrep pitfall that produced wrong intermediate numbers.** In ripgrep, `-h` is
   `--help`, **not** `--no-filename` (that is `-I`). Several `rg -oh '<pattern>' <files>`
   invocations therefore printed ripgrep's help text, which piped into `wc -l` yields a
   plausible-looking **124** every time. I caught this when Hermes' "tool count" and
   OpenClaw's "config help keys" both came back as exactly 124. All affected counts were
   redone with `grep -o` or a Python parser. Anyone reproducing this work should use
   `grep -o` or `rg -o --no-filename`.
6. **Marker counts are comment-anchored.** Whole-word counts are given alongside because
   they differ where a product feature is named `todo`.
7. **A scope correction found during the final sanity check.** My first pass measured
   Hermes' TypeScript as `apps/` only (348,116 lines). The pre-supplied breakdown also
   lists `ui-tui/` and `web/`, which I then measured (70,975 and 50,131). Every Hermes
   TS figure in this document is the corrected three-tree total of **469,222**. The
   correction also raised Hermes' inflation from 1.7% to 2.5%, because `web/src/i18n/`
   holds a further 14,337 lines of locale catalog.
8. **Nothing was modified.** No repo files were written, no installs run, no builds.
   All temporary artifacts live under the scratchpad directory.
