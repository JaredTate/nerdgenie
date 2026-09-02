# 05 — OpenCode: interface study and "what to copy" spec

Date: 2026-09-02. Source: `/Users/jt/Code/opencode` at HEAD `69c172e8` (v1.18.26). Installed binary on this Mac: v1.18.19 (a week older; differences noted where they matter). Every code claim cites `file:line` relative to the repo root. "Not verified" marks inference.

## 1. Measurements

| Metric | Value | Notes |
|---|---|---|
| `opencode --version` wall time (3 runs) | 1.81 s cold, 0.29 s, 0.30 s. **Median 0.30 s** | Reports `1.18.19`. Cold run pays page-cache cost for a 137 MB file. |
| Binary | `/Users/jt/.nvm/versions/node/v24.14.1/lib/node_modules/opencode-ai/bin/opencode.exe`, **143,710,946 bytes (137 MiB)**, Mach-O 64-bit arm64 | `~/.opencode/bin/opencode` is a symlink to it. It is a single Bun-compiled executable (`packages/opencode/script/build.ts:169-192`), with the web UI (`build.ts:26-48`) and tree-sitter worker (`build.ts:51`) embedded. |
| `opencode serve` RSS at ~5.5 s, idle, no session | **413,088 KB (~403 MB)** | Logged `Warning: OPENCODE_SERVER_PASSWORD is not set; server is unsecured.` and `listening on http://127.0.0.1:4197`. `GET /global/health` returned `{"healthy":true,"version":"1.18.19"}`. Process killed afterwards; none left running. |
| `~/.local/share/opencode` total | 543 MB | XDG data dir (`packages/core/src/global.ts:11-14`). |
| `opencode.db` (+ `-wal`, `-shm`) | 384 KB + 708 KB + 32 KB, **SQLite**, 20 tables | Tables: session, message, part, todo, session_message, session_input, session_context_epoch, event, event_sequence, permission, project, project_directory, account, account_state, control_account, credential, session_share, workspace, data_migration, migration. Only 6 sessions / 15 messages in it (Aug 2026 use). |
| `storage/` | 330 MB, ~25,000 **JSON** files (`part/` 19,855 files 271 MB; `message/` 4,558; `session/` 169) | The older per-record JSON layout from January use; only `session_diff` is still written (`packages/opencode/src/session/revert.ts:77`). |
| `snapshot/` | 72 MB, 3 project hashes | Bare git dirs used for `/undo` (`packages/opencode/src/snapshot/index.ts:71`). |
| `log/` | 31 MB | |
| `bin/` | 109 MB (`vscode-eslint` LSP server) | Downloaded on demand. ripgrep goes to `~/.cache/opencode/bin` (`packages/core/src/ripgrep/binary.ts:97-106`). |
| `auth.json` | 327 bytes, mode 0600 | API keys, plain JSON (`packages/opencode/src/auth/index.ts:10`). |
| `~/.cache/opencode` | `bin/`, `node_modules/` (npm plugins installed on demand), `models.json` (models.dev cache), `skills/` | `packages/core/src/models-dev.ts:160-162`, `packages/opencode/src/plugin/loader.ts:94`. |
| Lockfile | **3,233 resolved packages**, 37 workspaces | `bun.lock`. |
| Direct deps, `packages/opencode` | 99 runtime + 21 dev | 19 of them are `@ai-sdk/*` provider packages. |
| Direct deps, `packages/tui` | 16 runtime + 3 dev | |
| LOC (`.ts/.tsx`, no tests) | core `packages/opencode/src` **81,173**; `packages/core/src` 32,961; `packages/tui/src` 27,055; `packages/app/src` (web UI) 138,696; `packages/ui/src` 33,519; `packages/session-ui/src` 20,295; `packages/sdk/js/src` 30,087 (generated); `packages/desktop/src` 7,564; `packages/server/src` 1,682; `packages/protocol/src` 1,582; `packages/plugin/src` 1,612 | |
| Prompt text | 37 `.txt` files, 1,832 lines | System prompts and tool descriptions live next to code (e.g. `tool/read.txt`, `session/prompt/anthropic.txt`). |
| Effect usage | 227 of 357 TS files in `packages/opencode/src` import `effect` | It is an Effect app, not a partial migration. |
| Tests | 251 `*.test.ts` files, `bun test` | `packages/tui/package.json:8`. |

## 2. Architecture in plain English

OpenCode is one process that contains a full HTTP API server. Every user interface is a client of that API. The terminal UI is just the most common client.

```
   TUI (opentui + Solid)      web app (Solid, embedded)     desktop (Electron)      opencode attach <url>
        │ in-process RPC          │ same origin /*               │ sidecar w/ password     │ HTTP + Basic auth
        ▼                         ▼                              ▼                          ▼
   ┌──────────────────────────────────────────────────────────────────────────────────────────┐
   │  packages/opencode  (Bun, Effect)                                                       │
   │   Effect HttpApi router ── 127 endpoints (server dir) + 61 (packages/protocol) = 188     │
   │     GET /event, /global/event  ───── SSE stream of every event (heartbeat 10 s)          │
   │     GET /pty/:id/connect       ───── WebSocket (terminal)                                │
   │     GET /doc                   ───── OpenAPI JSON  →  packages/sdk (generated client)    │
   │     /*                         ───── embedded web UI                                     │
   │   Event bus (PubSub) → GlobalBus → SSE fan-out to every attached client                  │
   │   Session engine · Tool registry · Permission service · Provider layer (Vercel AI SDK)   │
   │   SQLite (bun:sqlite + drizzle)  ~/.local/share/opencode/opencode.db                     │
   └──────────────────────────────────────────────────────────────────────────────────────────┘
```

Key facts (server-side details verified by the server subagent, file:line as reported):

- **Framework.** Effect `HttpApi` on `@effect/platform-node`, not Hono (`packages/opencode/src/server/server.ts:3-7`). Route groups: config, experimental, file, instance, mcp, project, project-copy, pty, question, permission, provider, session, sync, tui, workspace (`server/routes/instance/httpapi/api.ts:61-77`). Route count: 127 typed endpoints in the server dir, 61 more in `packages/protocol/src/groups` (the newer `/api/*` v2 contract), 188 total.
- **Events.** `GET /event` is a `text/event-stream` (`handlers/event.ts:69-85`). Every state change is an event: `session.*`, `message.updated`, `message.part.updated`, `message.part.delta`, `permission.asked/replied`, `question.asked`, `todo.updated`, `file.edited`, `lsp.updated`, `pty.*`, `tui.*`. Durable events are also written to the `event` table (`packages/core/src/event.ts:325-337`).
- **How the TUI connects.** Running plain `opencode` does not open a TCP port. `cli/cmd/tui.ts:210-215` starts a Bun `Worker`; the TUI's `fetch` is an RPC to that worker (`tui.ts:24-40`), which calls `Server.Default().app.fetch(request)` in memory (`cli/tui/worker.ts:42`). Events arrive over the same RPC channel (`tui.ts:42-50`). With `--port`/`--hostname` it switches to real HTTP (`tui.ts:233-249`). The TUI client is the generated SDK (`packages/tui/src/context/sdk.tsx:1,24-30`).
- **Remote and multi-client.** `opencode serve --port --hostname --mdns --cors` (`cli/cmd/serve.ts:6-24`, `cli/network.ts:6-33`) and `opencode attach <url> --password --session --continue` (`cli/cmd/attach.ts:7-61`). Auth is HTTP Basic from `OPENCODE_SERVER_PASSWORD` (`server/auth.ts:17-20`); with no password the middleware is a no-op (`middleware/authorization.ts:104-138`). Default bind `127.0.0.1`, port 4096 then any free port (`server.ts:117-122`). Several clients can attach at once: each request carries the project directory in `x-opencode-directory` (`middleware/workspace-routing.ts:87`; SDK `packages/sdk/js/src/v2/client.ts:25-26`) and every SSE subscriber gets its own queue (`handlers/event.ts:31-32`).
- **Desktop and web.** The desktop app (Electron) forks a sidecar that runs `Server.listen` with a random password and `cors: ["oc://renderer"]` (`packages/desktop/src/main/sidecar.ts:57-65`). The web app (`packages/app`, Solid) is built and embedded into the binary and served at `/*`; in production it talks to `location.origin` (`packages/app/src/entry.tsx:99-103`). `packages/ui` is the shared Solid component/theme library; `packages/session-ui` renders message parts/diffs for web and desktop; `packages/server` is the v2 API implementation mounted inside the main process, not a separate daemon.
- **Protocol docs.** `GET /doc` serves OpenAPI (`httpapi/server.ts:188-192`); `packages/sdk/openapi.json` is committed (1.06 MB, OpenAPI 3.1). `opencode generate` emits it; `@hey-api/openapi-ts` builds the JS SDK (`packages/sdk/js/script/build.ts:10-14`). `packages/protocol` is the typed contract; `packages/client` is an Effect-native client generated from it.
- **Storage.** SQLite via `bun:sqlite` + drizzle, WAL mode (`packages/core/src/database/database.ts:3-33`), 30 migrations. Path `$XDG_DATA_HOME/opencode/opencode.db` (`database.ts:43-55`).

For JT's agent: "API + event stream + thin clients" is exactly what Signal + web UI + cron need. Signal, web and CLI become three subscribers to one SSE feed.

## 3. The 10 UX decisions to copy

1. **Server in a worker, no socket, no wait.** The UI never blocks on "server listening". The API runs in a Bun Worker and is called through `postMessage` RPC (`packages/opencode/src/cli/cmd/tui.ts:24-50, 210-249`; `cli/tui/worker.ts:42`). The exact same UI can `attach` to a remote server later. Copy: build the core as a library with an in-memory `fetch`, and make the TCP listener optional.
2. **First frame before readiness, with anti-flicker.** `TimeToFirstDraw` is rendered immediately (`packages/tui/src/app.tsx:1108`). The "Loading plugins..." spinner appears only if startup exceeds 500 ms and then stays at least 3 s so it never blinks (`component/startup-loading.tsx:22, 46`). Copy both thresholds.
3. **Streaming as field deltas, not re-sent messages.** The server emits `message.part.delta {messageID, partID, field, delta}`; the client appends the string into a Solid store and only that part re-renders (`context/sync.tsx:398-415`). Full `message.part.updated` replaces by binary search on part id (`sync.tsx:376-396`). Events are processed immediately unless one was just flushed within 16 ms, in which case they batch (`context/sdk.tsx:68-80`). Copy: delta events keyed by (message, part, field).
4. **Native markdown, diff and syntax widgets.** Assistant text is a `<markdown>` element; tool diffs are a `<diff>` element with unified/split chosen by config `tui.diff_style` (`routes/session/index.tsx:1692-1699, 2401-2423`). Highlighting is tree-sitter in a worker (`parsers-config.ts`; `packages/opencode/script/build.ts:51`). A comment states the goal: "layout never shifts" (`index.tsx:1590`). Copy: render markdown to a layout tree, never to raw ANSI strings.
5. **Inline permission prompts with three answers.** The prompt is a component inside the session view (`routes/session/index.tsx:1298`), not a modal. Options are `Allow once / Allow always / Reject`, Escape rejects (`routes/session/permission.tsx:405-427`), `ctrl+f` toggles fullscreen (`config/keybind.ts:219`; `permission.tsx:543-558`). "Always" opens a stage that shows exactly which patterns will be allowed (`permission.tsx:138-169`). A reject can carry feedback text that the model sees (`packages/opencode/src/permission/index.ts:121-126`). An "auto" mode auto-replies for unattended runs (`context/sync.tsx:198-203`).
6. **One registry, three surfaces.** Each command is defined once with `title`, `slashName`, `slashAliases`, `keybind`, `category` (`app.tsx:575-586`, `keymap.tsx:260-275`). The same object drives the `ctrl+p` palette, `/` autocomplete and the keybinding. Copy this exactly; it is why the command set feels coherent.
7. **Leader key + one-keystroke mode switches.** Leader is `ctrl+x` (`config/keybind.ts:41`); `tab` cycles agents (build/plan), `f2` cycles recent models, `ctrl+t` cycles variants (`keybind.ts:122, 130-132`). A which-key panel lists what the leader can do (`feature-plugins/system/which-key.tsx`). No menus needed for the two most common switches.
8. **Undo that restores files.** `/undo` removes messages and reverts the working tree from a shadow git repo stored under the data dir (`packages/opencode/src/session/revert.ts:70-72`; `snapshot/index.ts:71-75`), `/redo` restores (`revert.ts:91-96`). Keys `<leader>u` / `<leader>r` (`keybind.ts:147-148`). Copy: snapshot before each turn; undo is a real rollback.
9. **`!` shell mode and `@` mentions in the same box.** Typing `!` at column 0 flips the prompt into shell mode and Enter runs the command through the session (`component/prompt/index.tsx:831-836, 1059-1061`); Escape or Backspace exits (`:846-858`). `@` opens a fuzzy picker for files (with line ranges) and agents, inserting a styled mention (`component/prompt/autocomplete.tsx:61, 178-192, 302-313`).
10. **Sessions as first-class objects.** Fuzzy session list (`/sessions`), quick slots `<leader>1..9`, pin, rename, delete, fork, timeline jump, child-session navigation (`keybind.ts:89-116`; `routes/session/index.tsx:510-560`). The sidebar shows context tokens, cost, todo list, LSP and MCP status (`feature-plugins/sidebar/context.tsx:17-32`, `sidebar/*.tsx`).

Supporting touches: 33 JSON themes (`packages/tui/src/theme/assets/opencode.json:1-49`); 5 s toasts (`ui/toast.tsx:10-62`); `$EDITOR` for long prompts; image paste becomes an attachment (`prompt/index.tsx:372-380`); mouse on by default (`app.tsx:202`); frecency-ranked file picker (`component/prompt/frecency.tsx`).

Why it feels faster (partly inference): a retained layout tree diffed by a real reconciler (Solid + opentui) instead of line-based redraws, streaming that touches one part at a time, and a UI that never waits on the network at startup. Frame timing under load: not verified.

## 4. Full slash-command table

Every built-in slash command in the TUI at HEAD. Source column is where the command object is defined.

| Command | Aliases | Meaning | Source |
|---|---|---|---|
| `/new` | `/clear` | Start a new session | `packages/tui/src/app.tsx:586` |
| `/sessions` | `/resume`, `/continue` | Fuzzy list and switch sessions | `app.tsx:575` |
| `/workspaces` | | Manage workspaces (worktree copies) | `app.tsx:615` |
| `/models` | `/mo` | Model picker (fuzzy, favourites, provider list) | `app.tsx:634` |
| `/agents` | | Switch agent (build/plan/custom) | `app.tsx:681` |
| `/mcps` | | Toggle MCP servers | `app.tsx:690` |
| `/variants` | | Pick model variant (e.g. reasoning effort) | `app.tsx:717` |
| `/connect` | | Connect a provider (API key / OAuth) | `app.tsx:742` |
| `/org` | | Switch console org (hosted service) | `app.tsx:754` |
| `/status` | | View status panel | `app.tsx:766` |
| `/debug` | | Debug info | `app.tsx:775` |
| `/themes` | | Switch theme | `app.tsx:784` |
| `/help` | | Help dialog | `app.tsx:812` |
| `/exit` | `/quit`, `/q` | Exit the app | `app.tsx:830` |
| `/share` | | Share session (copy link if already shared) | `packages/tui/src/routes/session/index.tsx:472` |
| `/unshare` | | Stop sharing | `index.tsx:592` |
| `/rename` | | Rename session | `index.tsx:510` |
| `/timeline` | | Jump to a message | `index.tsx:521` |
| `/fork` | | Fork session at a point | `index.tsx:543` |
| `/compact` | `/summarize` | Compact context via summary | `index.tsx:565` |
| `/undo` | | Undo last message and revert files | `index.tsx:614` |
| `/redo` | | Redo an undone message | `index.tsx:651` |
| `/timestamps` | `/toggle-timestamps` | Show/hide message times | `index.tsx:698` |
| `/thinking` | `/toggle-thinking` | Show/hide reasoning blocks | `index.tsx:715` |
| `/copy` | | Copy session transcript | `index.tsx:920` |
| `/export` | | Export transcript to `$EDITOR` | `index.tsx:950` |
| `/editor` | | Compose prompt in external editor | `packages/tui/src/component/prompt/index.tsx:427` |
| `/skills` | | Pick a skill to attach | `prompt/index.tsx:519` |
| `/warp` | | Warp terminal integration | `prompt/index.tsx:541` |
| `/move` | | Move session to another project copy | `prompt/index.tsx:551` |
| `/diff` | | Open diff viewer | `packages/tui/src/feature-plugins/system/diff-viewer.tsx:1058` |

Server-provided commands appear in the same `/` autocomplete (`component/prompt/autocomplete.tsx:448-458`):

| Command | Meaning | Source |
|---|---|---|
| `/init` | Guided AGENTS.md setup | `packages/opencode/src/command/index.ts:70-78` |
| `/review [commit\|branch\|pr]` | Review changes, runs as a subtask | `command/index.ts:79-88` |
| `/<name>` | User commands from config `command.<name>` or `{command,commands}/**/*.md` | `command/index.ts:90-103`; `packages/opencode/src/config/command.ts:15` |
| `/<name>:mcp` | MCP server prompts | `command/index.ts:105-120`; label at `autocomplete.tsx:452` |

Docs also list `/details` (`packages/web/src/content/docs/tui.mdx:100`); in code it is the `tool_details` keybind with no slash name (`keybind.ts:150`). Not verified whether the docs are stale.

Default keybinds worth copying (`packages/tui/src/config/keybind.ts`): `ctrl+x` leader (41); `ctrl+c`/`ctrl+d`/`<leader>q` exit (48); `ctrl+p` palette (57); `return` submit, `shift+return`/`ctrl+j` newline (163-164); `tab`/`shift+tab` agent cycle (130-131); `f2` recent model (122); `ctrl+t` variant (132); `<leader>n/l/m/a/t/s/c/e/x/b/g` new/sessions/models/agents/themes/status/compact/editor/export/sidebar/timeline (77-99, 121, 129); `<leader>u`/`<leader>r` undo/redo (147-148); `pageup`/`pagedown` scroll (135-136); `ctrl+z` suspend (223); emacs-style editing `ctrl+a/e/k/u/w` (173-197). Overrides via config `keybinds` (`keybind.ts:449-455`).

## 5. Tools and permission model

### Tools the model sees

Registered in `packages/opencode/src/tool/registry.ts:231-249`. Descriptions are `.txt` files next to each tool (e.g. `tool/read.txt`). Schemas are Effect `Schema` (`tool/task.ts:44-60`); plugin tools may use Zod (`registry.ts:125-137`). Every result is `{title, output, metadata}` (`tool/tool.ts:48-52`). Output is capped at 2,000 lines / 50 KB, with the full text written to a truncation dir kept 7 days (`tool/truncate.ts:12-15, 36-41`).

| Tool id | Purpose | File | When enabled |
|---|---|---|---|
| `bash` | Run a shell command; default timeout 2 min; kills on timeout with a retry hint | `tool/shell.ts:338, 347, 552-564`; id kept as `bash` for compatibility `tool/shell/id.ts:14-16` | always |
| `read` | Read file with offset/limit | `tool/read.ts` | always |
| `glob` | Find files by pattern | `tool/glob.ts:17` | always |
| `grep` | ripgrep search | `tool/grep.ts:20` | always |
| `edit` | Exact string replace | `tool/edit.ts:58` | not for GPT-5 family (`registry.ts:297-300`) |
| `write` | Write whole file | `tool/write.ts:27` | same as edit |
| `apply_patch` | Codex-style patch | `tool/apply_patch.ts:22` | only GPT-5 family (`registry.ts:297-299`) |
| `task` | Spawn a subagent, optional `background: true`, notified on finish | `tool/task.ts:24, 44-60, 97` | always; subagent list injected into description (`registry.ts:265-278`) |
| `webfetch` | Fetch URL, HTML to markdown | `tool/webfetch.ts:24` | always |
| `websearch` | Web search | `tool/websearch.ts:99` | only OpenCode-hosted providers or Exa/Parallel flags (`registry.ts:58-64, 293-296`) |
| `todowrite` | Maintain a todo list | `tool/todo.ts` | always |
| `skill` | Load a SKILL.md into context | `tool/skill.ts:12`; `tool/skill.txt:1-5` | always |
| `question` | Ask the user structured questions (options, multiple, custom answer) | `tool/question.ts:7`; UI `packages/tui/src/routes/session/question.tsx:22-42` | clients `app`, `cli`, `desktop` (`registry.ts:207`) |
| `invalid` | Catches malformed tool calls so the model gets an error instead of a crash | `tool/invalid.ts:9` | always |
| `lsp` | LSP queries | `tool/lsp.ts:37` | `experimentalLspTool` flag (`registry.ts:247`) |
| `plan_exit` | Leave plan mode | `tool/plan.ts:15` | experimental plan mode, CLI only (`registry.ts:248`) |
| `execute` | "Code mode" (run JS against MCP catalog) | `tool/code-mode.ts:188` | `experimentalCodeMode` flag (`registry.ts:118`) |

Count: 17 defined; 13-14 visible in a normal session. Extra tools come from `{tool,tools}/*.{js,ts}` in config dirs (`registry.ts:185-197`), from plugins (`registry.ts:199-204`), and from MCP servers (`packages/opencode/src/mcp/`, config `mcp` key at `packages/core/src/v1/config/config.ts:113`).

### Permission model

- **Rule format** in `opencode.json` (`packages/core/src/v1/config/permission.ts:5-34`): a key maps to `"ask" | "allow" | "deny"` or to an object of `pattern -> action`. A bare string at the top level means `{"*": action}` (`permission.ts:40-41`). Known keys: `read, edit, glob, grep, list, bash, task, external_directory, todowrite, question, webfetch, websearch, lsp, doom_loop, skill`, plus any string (`permission.ts:19-34`); code also uses `plan_enter`, `plan_exit` (`packages/opencode/src/agent/agent.ts:127-128`).

```json
{ "permission": { "bash": { "git *": "allow", "rm -rf *": "deny", "*": "ask" },
                  "edit": "allow", "webfetch": "ask", "external_directory": { "~/notes/*": "allow" } } }
```

- **Compilation.** Config becomes a flat list of `{permission, pattern, action}` rules; `~/` and `$HOME` expand (`packages/opencode/src/permission/index.ts:178-197`).
- **Evaluation.** `evaluate()` takes the **last** rule whose permission and pattern both wildcard-match; default is `ask` (`index.ts:28-38`). Rulesets merge in order: built-in agent defaults, then user config, then the agent's own `permission` block (`agent.ts:267-293`). So order in the JSON file is precedence.
- **Per-agent.** `build` allows most things; `plan` denies edits except plan files under `.opencode/plans/*.md` and the data dir (`agent.ts:141-181`); `explore` is read-only (`agent.ts:196-217`).
- **Ask flow.** (1) A tool calls `permission.ask({permission, patterns, always, metadata, ruleset})`. (2) If any pattern hits `deny`, the tool fails with `PermissionDeniedError` listing the rules (`index.ts:76-84`). (3) Otherwise a `Deferred` is stored and `permission.asked` is published with `{id, sessionID, permission, patterns, metadata, always, tool}` (`index.ts:89-105`). (4) The client answers `POST /session/:id/permissions/:permissionID {response: "once"|"always"|"reject"}` (`server/routes/instance/httpapi/handlers/session.ts:362-367`) or the v2 `/permission` reply route (`groups/permission.ts:11-40`). (5) `reject` fails the deferred; with a message it becomes `PermissionCorrectedError` and the text reaches the model; every other pending ask in that session is rejected too (`index.ts:121-141`). (6) `always` appends `allow` rules for the request's `always` patterns to an **in-memory** approved list (not written to config) and auto-resolves other pending asks that now match (`index.ts:146-172`). `once` resolves just this one (`index.ts:143`).
- **Doom loop.** If the last N tool calls have identical tool and input, a `doom_loop` ask fires (`packages/opencode/src/session/processor.ts:356-378`); default is `ask` (`agent.ts:121`).
- **Sandbox.** None. No seatbelt/bwrap/landlock references in `packages/opencode/src` or `packages/core/src`. Path control is the `external_directory` permission only (`tool/external-directory.ts`).
- **Plugins** can pre-decide any ask via the `permission.ask` hook (`packages/plugin/src/index.ts:261`).

## 6. Local-model support notes

- **SDK.** Vercel AI SDK `ai@6.0.168` with 19 `@ai-sdk/*` providers bundled (`packages/opencode/package.json`). A custom provider is any `npm` package name; `@ai-sdk/openai-compatible` is loaded lazily (`packages/opencode/src/provider/provider.ts:123`).
- **Config shape** (`packages/web/src/content/docs/providers.mdx:1453-1470` for LM Studio, `1652-1670` for Ollama, `1386-1412` for llama.cpp):

```json
{ "provider": { "ollama": { "npm": "@ai-sdk/openai-compatible", "name": "Ollama (local)",
    "options": { "baseURL": "http://localhost:11434/v1" },
    "models": { "qwen3:8b": { "name": "Qwen3 8B" } } } } }
```

  For LM Studio use `http://127.0.0.1:1234/v1`. Custom providers default `npm` to `@ai-sdk/openai-compatible` (`provider.ts:1270-1274`); a model's `tool_call` defaults to `true` and `reasoning` to `false` (`provider.ts:1287-1289, 1518-1520`).
- **Catalog.** models.dev is fetched from `https://models.opencode.ai/api.json`, cached at `~/.cache/opencode/models.json`, overridable with `OPENCODE_MODELS_URL` / `OPENCODE_MODELS_PATH` (`packages/core/src/models-dev.ts:160-184`). Local providers are not in the catalog; you declare their models by hand.
- **Special handling.** No Ollama- or LM Studio-specific code exists in `packages/opencode/src` or `packages/core/src` (rg found none). Generic OpenAI-compatible tweaks: `includeUsage` on (`provider.ts:1751`), verbosity param suppressed (`transform.ts:1340`), DashScope `enable_thinking` (`transform.ts:1281-1288`), DeepSeek `reasoning_content` field auto-set (`provider.ts:1541-1542`), schema `anyOf` rewrite (`transform.ts:1642`). The docs' only local-model advice is to raise Ollama `num_ctx` to 16-32k if tool calls fail (`providers.mdx:1681`).
- **Tools on non-tool models.** The `toolcall` capability flag is stored but I found no code path that removes tools when it is `false` (only tests read it: `packages/opencode/test/provider/provider.test.ts:1043`). Not verified further; assume a local model gets the full tool list and must cope.
- **Model choice.** `defaultModel()` order: config `model` → most recent entry in `~/.local/state/opencode/model.json` that still exists → first model of the first configured provider (`provider.ts:2004-2037`). `small_model` handles titles/summaries (`packages/core/src/v1/config/config.ts:77`).
- **Auth.** Keys in `auth.json` (`packages/opencode/src/auth/index.ts:10`); `opencode providers login [url]` (`cli/cmd/providers.ts:240, 300`); `/connect` in the TUI.

## 7. Agents, roles, skills, plugins, commands

- **Agents.** Built-ins: `build` (primary), `plan` (primary, edit-restricted), `general` and `explore` (subagents), `compaction`, `title`, `summary` (hidden) (`packages/opencode/src/agent/agent.ts:141-254`; prompts in `agent/prompt/*.txt`). Users define agents in `opencode.json` `agent.<name>` or as markdown in `{agent,agents}/**/*.md` under `~/.config/opencode` or any `.opencode` dir up the tree (`packages/opencode/src/config/agent.ts:11-26`; dirs from `config/paths.ts:26-39`). Frontmatter fields: `description, mode (primary|subagent|all), model, variant, temperature, top_p, tools (deprecated), permission, color, hidden, steps, disable`; the body is the prompt (`packages/core/src/v1/config/agent.ts:14-38`; merge at `agent.ts:267-293`). `default_agent` picks the starting one (`config.ts:80`).
- **How a "role" persists.** There is no memory subsystem (rg `memory` hits only `cli/heap.ts` and `plugin/xai.ts`). A role is: the agent `.md` file + instruction files + session history in SQLite. Instruction files loaded into the system prompt: global `~/.config/opencode/AGENTS.md`, `~/.claude/CLAUDE.md`, then the first project-level `AGENTS.md` / `CLAUDE.md` / `CONTEXT.md` found walking up (`packages/opencode/src/session/instruction.ts:61-67, 122`), plus config `instructions` globs and URLs (`instruction.ts:135-158`). System prompt = provider-specific base prompt (`session/system.ts:6-15`) + environment block with model id and cwd (`system.ts:67-75`) + skills list (`system.ts:105`) + instructions.
- **Skills.** Discovered from `.claude/skills/**/SKILL.md`, `.agents/skills/**/SKILL.md`, `.opencode/{skill,skills}/**/SKILL.md`, the same under `$HOME`, plus config `skills.paths` and remote `skills.urls` indexes cached in `~/.cache/opencode/skills` (`packages/opencode/src/skill/index.ts:21-25, 191-222`; `skill/discovery.ts:35-128`). Surfaced as a list in the system prompt; the model calls `skill` to load one (`tool/skill.txt:1-5`).
- **Plugins.** Config `plugin: ["npm-name" | "./file.ts"]` (`packages/core/src/v1/config/config.ts:56`); npm plugins are installed on demand into `~/.cache/opencode` (`plugin/loader.ts:94, 125`); local files from `{plugin,plugins}/*.{ts,js}` in config dirs (`packages/opencode/src/config/plugin.ts:21`). Hooks (`packages/plugin/src/index.ts:90-334`): `auth`, `models`, `tool`, `event`, `config`, `dispose`, `chat.message`, `chat.params`, `chat.headers`, `permission.ask`, `command.execute.before`, `tool.execute.before`, `tool.execute.after`, `tool.definition`, `shell.env`, plus experimental `chat.messages.transform`, `chat.system.transform`, `provider.small_model`, `session.compacting`, `compaction.autocontinue`, `text.complete`. The TUI has its own plugin host (`packages/tui/src/plugin/*`).
- **Custom commands.** `command.<name>: {template, description, agent, model, subtask}` or `{command,commands}/**/*.md` with the same fields as frontmatter (`packages/opencode/src/command/index.ts:22-32, 90-103`; `config/command.ts:13-28`). Template expansion (`session/prompt.ts:1383-1439`): `$1..$9`, `$ARGUMENTS` (appended if the placeholder is absent, `1390-1395`), `` !`cmd` `` runs a shell command and inlines its output (`config/markdown.ts:6`; `prompt.ts:1397-1408`), `@path` attaches a file (`markdown.ts:5`; `prompt.ts:1432`). If the target agent is a subagent the command runs as a subtask (`prompt.ts:1439`).
- **Non-interactive.** `opencode run` supports `--format json`, `--session`, `--continue`, `--fork`, `--agent`, `--model`, `--file`, `--attach <url>`, `--password`, `--dir`, `--variant`, `--thinking` (`cli/cmd/run.ts:143-216`). This is the hook a Signal bridge or cron would use if you wrapped OpenCode instead of rewriting it.

## 8. Weight: what would be heavy to replicate

Honest answer: **OpenCode is not light. It feels light because the client is thin and never waits.**

| Heavy piece | Size / version | Would you need it? |
|---|---|---|
| Effect 4.0 beta (`effect`, `@effect/platform-node`, `@effect/sql-sqlite-bun` at `4.0.0-beta.83`) | 227/357 core files; every service is a `Context.Service` + `Layer` (`tool/registry.ts:89-93`) | No. Plain async functions + one event emitter do the job for one user. |
| Vercel AI SDK 6 + 19 provider packages + openrouter/gitlab/venice adapters | ~25 deps | Partly. Anthropic + one OpenAI-compatible client covers cloud + local. |
| Web app embedded in the binary | 138,696 LOC in `packages/app`, 33k in `packages/ui`, 20k in `session-ui` | No. A ~2k-LOC web page over SSE is enough for a phone. |
| opentui 0.4.5 + Solid 1.9 + tree-sitter worker | 27k LOC TUI; native markdown/diff widgets | Maybe. This is what makes the TUI feel good. Alternatives: reuse opentui directly (it is a standalone library), or skip a TUI and use commands. |
| drizzle 1.0-rc + 30 migrations + legacy JSON store | `packages/core/src/database`, `packages/effect-drizzle-sqlite` 3.2k LOC | No. `bun:sqlite` with 4 tables (session, message, part, event) is enough. |
| Generated SDK + OpenAPI (1.06 MB), `packages/protocol`, `packages/client`, `httpapi-codegen` | 30k generated LOC | No. Hand-write ~15 routes. |
| Desktop (Electron), console, enterprise, ACP, control plane, workspaces, PTY over WebSocket, share service, LSP client, MCP client, GitHub/GitLab bots | tens of thousands of LOC | No for v1. MCP maybe later. |
| Bun `--compile` single binary with embedded assets (`script/build.ts:145-192`) | 137 MB, ~400 MB RSS idle | Not needed; a small `bun run` script starts faster and uses far less memory. Pi numbers not verified. |
| ripgrep downloaded from GitHub on first use (`packages/core/src/ripgrep/binary.ts:97-106`); LSP servers downloaded to `bin/` (109 MB here) | | Install `rg` with apt instead. |

Essential core to keep (LOC in OpenCode): session loop + streaming processor (`session/` 8.1k), tools (`tool/` 5.2k, ~1.8k of it prompt text), permissions (387), snapshot/undo (807), agents (480), commands (177), skills (494), config (2k). About 15-18k LOC of real logic sits inside an 81k-LOC package, wrapped by ~250k LOC of clients, codegen and platform.

## 9. Spec: what to copy into the new agent

1. Core = library with in-memory `fetch` + optional TCP listener; SSE `/event` with `message.part.delta` events; SQLite (session, message, part, event). Basic auth from an env password; bind loopback or the Tailscale IP only.
2. Commands defined once as `{id, title, slash, aliases, keybind, run}`; the same table serves the terminal, the web UI and Signal (`/new`, `/sessions`, `/models`, `/agents`, `/compact`, `/undo`, `/redo`, `/status`, `/help`, `/exit`, plus custom `command/*.md` with `$ARGUMENTS`, `` !`cmd` ``, `@file`).
3. Tools: `bash` (timeout, 2,000-line/50 KB cap, spill to file), `read`, `edit`, `write`, `glob`, `grep`, `webfetch`, `todowrite`, `task`, `question`, `skill`, `invalid`. That is 12.
4. Permissions: `{permission: {pattern: ask|allow|deny}}`, last match wins, per-agent overrides, `once/always/reject(+feedback)`, doom-loop check, `external_directory` fence. Add a real sandbox yourself; OpenCode has none.
5. Roles: `agent/<name>.md` with frontmatter + `AGENTS.md`; add the durable memory OpenCode lacks.
6. Local models: one OpenAI-compatible client with `baseURL`; hand-declared model list with `tool_call`/`reasoning` flags; actually honour `tool_call:false` (OpenCode does not).
7. UX thresholds: first frame immediately; spinner only after 500 ms, held 3 s; 16 ms event batching; snapshot before every turn so undo is real.
