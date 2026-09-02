# 15 — Cross-agent tool inventory (for HARNESS_V2 §8.5)

Date: 2026-09-02. Read-only survey of eight agents. Repos: OpenClaw `/Users/jt/Code/openclaw`, Hermes `/Users/jt/Code/hermes-agent` (HEAD 95f62ca3bf), Prime `/Users/jt/Code/prime-agent`, OpenCode `/Users/jt/Code/opencode`, ZeroClaw `/Users/jt/Code/zeroclaw` (73ff732e), Codex clone (5971d42, 2026-09-02) and Atomic Agent clone (0ed39a7, v0.5.4) under `scratchpad/ext/`. Claude Code is compiled; facts come from `https://code.claude.com/docs/en/tools-reference.md`, `.../scheduled-tasks.md`, `.../hooks.md`, `.../sub-agents.md`, `.../agent-sdk/tool-search.md`. Anything I could not confirm is marked **not verified**.

Browser/desktop/email tools were fixed by study 11 (`browser_open/snapshot/act/screenshot/eval/file/tabs/handoff`, `computer_screenshot/act`, `skill_run`, `email_send`). This file covers everything else and then folds all of it into one final table.

Permission classes used below: **R** read, **W** write files, **X** run code, **N** network, **I** irreversible or user-visible (send, delete, schedule, spend). Verdict column: **keep**, **merge → X**, **skip**.

---

## 1. OpenClaw (57 catalog tools)

Registry: `CORE_TOOL_DEFINITIONS` in `src/agents/tool-catalog.ts:69-457`. Each entry has `id`, `description`, `sectionId` (fs/runtime/web/memory/sessions/ui/messaging/automation/nodes/agents/media, `:55-67`) and `profiles` (`minimal|coding|messaging|full`, `:29`). The "full" profile allows `*` (`:478`). Descriptions in the catalog are UI summaries; the model-facing text is assembled elsewhere (`tool-description-presets.ts:3-24`).

| tool | what it does | input shape | output cap | perm | design detail worth copying | verdict |
|---|---|---|---|---|---|---|
| read | Read file contents (`tool-catalog.ts:71`) | path, offset/limit | adaptive page 32 KB, up to 128 KB, max 4 pages (`agent-tools.read.ts:77-81`) | R | byte-budget pages, not just line counts | keep |
| write | Create or overwrite (`:77`) | path, content | — | W | — | keep |
| edit | "Make precise edits" (`:83`) | path, old, new | — | W | — | keep |
| apply_patch | Patch files (`:89`, `core-coding-tools.ts:17`) | patch text | — | W | second edit grammar for GPT-family | skip |
| exec | "Run shell now." (`tool-description-presets.ts:5`) | command, workdir, env, yieldMs (default 10 000 ms then auto-background), background, timeoutSeconds, pty, elevated, host, ask, node (`bash-tools.schemas.ts:25-73`) | truncation in result-limits (not read) | X | **yieldMs**: run in foreground, auto-detach after N ms, hand back a session id; code lazy-loaded (`lazy-exec-tool.ts:29-31`) | keep (merge process into it) |
| process | Inspect/control exec sessions | action list/poll/log/write/send-keys/submit/paste/kill/clear/remove, sessionId (`bash-tools.schemas.ts:90-100`) | — | X | one verb enum for job control | merge → shell |
| code_execution | Sandboxed remote analysis (`:107`) | code | — | X N | — | skip |
| secrets | Write-only credential requests (`:113`) | — | — | I | never returns secret to model | skip (Coeus uses env + ledger) |
| web_search / web_fetch / x_search | Search, fetch, X posts (`:121-141`) | query / url | cache 100 entries (`web-shared.ts:21`) | N | — | keep (merge → web) |
| memory_search | "Mandatory recall step: semantically search … before answering questions about prior work, decisions, dates, people, preferences, or todos" (`extensions/memory-core/src/memory-tool-contract.ts:87-93`) | query, corpus | — | R | description tells the model *when* it is mandatory | keep (merge → memory) |
| memory_get | "Safe exact excerpt read … bounded excerpt when lines are omitted" (`:96-101`) | file, lines | bounded excerpt | R | search-then-get pair | merge → memory |
| sessions, sessions_list/history/search/send/spawn, sessions_yield, subagents, agents_wait, agents_list, session_status | 11 tools for session bookkeeping and sub-agents (`:157-235`) | spawn: task, taskName, label, runtime, agentId, model, runTimeoutSeconds, thinking, cwd, thread, cleanup, sandbox, lightContext (`sessions-spawn-tool.ts:163-221`) | — | X | "Spawn hidden subagent (ephemeral) or visible work session (durable)" (`presets.ts:14-15`) | merge → delegate (one tool) |
| conversations_list/send/turn | External conversation addressing (`:187-205`) | channel target | — | N I | `conversations_turn` = send and wait for correlated reply | merge → message |
| message | Send messages (`:325`) | channel, target/targets, accountId, dryRun, buttons (`message-tool-schema.ts:22-70`) | — | N I | `dryRun`; idempotency module (`message-tool-idempotency.ts`) | keep |
| ask_user | "Ask the user and wait for an answer." (`presets.ts:19`) | questions[{id, header ≤12 chars, question, options[{label, description}], multiSelect}], timeoutSeconds (`ask-user-tool.ts:28-75`) | — | I | header chip + options; timeout | keep |
| cron (AUTOMATIONS_TOOL_NAME) | "Schedule reminders, automations, wake events." (`presets.ts:7`) | action status/list/get/add/update/remove/run/runs/next_check/wake; schedule kind at/every/cron/stream; payload systemEvent/agentTurn/script; delivery none/announce/webhook; job fields at, everyMs, expr, tz, staggerMs, model, timeoutSeconds, toolBudget, toolsAllow… (`cron-tool-schema.ts:24-46, 60-200`) | list ≤200 (`:18`) | I | **one job object for add and update** so the shape ships once per prompt (`:19-22`, #121606); triggers-disabled variant hides fields the scheduler would reject (`:47-56`) | keep (as schedule) |
| browser | "Control web browser" (`:275`) | action enum: batch click type press hover drag select fill resize wait evaluate close doctor status start stop profiles importprofile tabs open focus snapshot screenshot navigate console requests errors text emulate pdf download waitfordownload upload dialog act (`extensions/browser/src/browser-tool.schema.ts:10-60`) | — | N X | one tool, many actions; `batch` | given |
| computer, mobile_ui, nodes, canvas, screen, dashboard, terminal, portal, show_widget, gateway | Node/desktop/UI surfaces (`:281-364`) | — | — | X | — | given (computer) / skip |
| get_goal/create_goal/update_goal, progress_card, suggest_task, dismiss_task | Thread goals and progress cards (`:377-405, 261-273`) | — | — | W | — | skip |
| skill_workshop | Author reusable skills (`presets.ts:22-23`) | — | — | W | — | merge → skill |
| github_identity_status, github_publish, heartbeat_respond | GitHub + heartbeat plumbing (`:219-231, 331`) | — | — | N I | — | skip |
| view_image, image_generate, music_generate, video_generate, tts | Media (`:421-452`) | — | — | N | — | skip |

Deferral: I found no schema-level deferral in this checkout; only lazy *code* loading for exec (`lazy-exec-tool.ts:29`) and profile allowlists (`tool-catalog.ts:466-480`). "OpenClaw deferred schemas" — **not verified**.

## 2. Hermes (59 toolsets, ~83 tools, 19 behind the bridge)

Registry: `registry.register(name=…, toolset=…, schema=…, handler=…, check_fn=…, max_result_size_chars=…)` (`tools/registry.py:215`); 93 `register(` calls across `tools/*.py` (some conditional). Toolsets table `toolsets.py:103`. Always-on set `_HERMES_CORE_TOOLS` (`toolsets.py:31-60`): web_search, web_extract, terminal, process_manage, read_file, write_file, patch, search_files, vision_analyze, image_generate, skills_list, skill_view, skill_manage, 12 browser_* + browser_exec, text_to_speech, todo_list, memory. Commit e16ad33a9d (2026-08-29) put a "curated 19-tool set behind the bridge by default … 13.4K -> 6.9K desktop schemas, -49%". Bridge tools: `tool_search`, `tool_describe`, `tool_call` (`tools/tool_search.py:67-69`). Default deferred set (`:273-280`): computer_use, session_search, image_generate, todo_list, process_manage, cronjob_manage + 12 desktop GUI tools. **clarify was pulled back to eager after an A/B: 18/18 structured asks when visible vs 7/18 when deferred** (`:265-272`).

| tool | what it does | input shape | output cap | perm | design detail | verdict |
|---|---|---|---|---|---|---|
| read_file | "Read a text file with line numbers and pagination. Use this instead of cat/head/tail" (`file_tools.py:2681`) | path, offset (1-based), limit (default/max 2000) (`:2685-2687`) | ~100K chars, line boundary, `next_offset` (`:2903`) | R | `LINE_NUM\|CONTENT`; suggests similar filenames | keep |
| write_file | Whole-file replace; "use 'patch' for targeted edits" (`:2695`) | path, content | 100K | W | auto syntax check on .py/.json/.yaml/.toml | keep |
| patch | Exact old_string/new_string (`:2721-2740`) | path, old_string (unique unless replace_all), new_string ('' deletes) | 100K | W | — | keep (as edit) |
| search_files | "Search file contents or find files by name. Use this instead of grep/rg/find/ls" (`:2810`) | pattern, target content/files, path, file_glob, limit 50, offset, output_mode content/files_only/count, context (`:2814-2821`) | 100K | R | **grep and glob in one tool** | keep |
| terminal | Shell; "Do NOT use cat/head/tail (use read_file), grep/rg/find/ls (use search_files)…" (`terminal_tool.py:1133-1139`) | command, background, timeout (default 180 s, foreground max env `FOREGROUND_MAX_TIMEOUT` `:120`), workdir, pty, notify true/[patterns] (`:4149-4188`) | 100K (`:4259`) | X | "Returns INSTANTLY when command finishes — set high"; `notify=true` fires one message on exit | keep |
| process_manage | Job control, "dieted (#95681): the action enum names the verbs" (`process_registry.py:3369-3372`) | action, session_id… | — | X | write vs submit trap documented | merge → shell |
| memory | Batched durable facts; "make ALL your changes in ONE call via an 'operations' array … applies atomically and the char limit is checked only on the FINAL result" (`memory_tool.py:1264-1284`) | action add/replace/remove, target user/memory, content, old_text, operations[] | char-limited store | W | atomic batch; "SKIP: … task progress … Reusable procedures belong in a skill, not memory" | keep (merge → memory) |
| session_search | "Recall past conversations … (FTS5) … Results are actual DB messages, no LLM" (`session_search_tool.py:1148-1160`) | query / session_id / around_message_id, limit 3, window 5 (`:1256-1270`) | — | R | four call shapes in one tool | merge → memory |
| skills_list / skill_view / skill_manage | List; load SKILL.md or a linked file (`skills_tool.py:2004-2014`); create/patch/write_file/remove_file/delete ops, atomic rollback, "first 57 chars a self-contained trigger" (`skill_manager_tool.py:2138-2150`) | name, file_path / operations[] | — | R W | index in prompt, body on demand | merge → skill |
| delegate_task | "Spawn one or more subagents in isolated contexts" (`delegate_tool.py:5217-5221`) | goal, context, tasks[], max_iterations, role, background, output_schema, action, subagent_id, message (`:5358-5375`) | — | X | description rebuilt per call with the user's delegation limits | keep (as delegate) |
| cronjob_manage | create/list/update/pause/resume/remove/run; "'run' fires … in the BACKGROUND (returns a handle at once … do not wait or poll)" (`cronjob_tools.py:2018`) | action, job_id, schedule, prompt | — | I | "always list first — never guess job IDs" | keep (as schedule) |
| clarify | Structured questions; "put your recommended option FIRST … auto-appends an 'Other' free-text row" (`clarify_tool.py:443-452`) | questions[], choices, multi_select | — | I | eager, never deferred (A/B above) | keep (as ask_user) |
| todo_list | Session checklist (`todo_tool.py:443`) | todos[], merge | — | W | deferred by default | skip |
| web_search / web_extract | 5 results default (`web_tools.py:1657`); markdown extract, 15 000-char budget, PDFs (`:1679`) | query / urls | 100K | N | "no LLM summarization — fast" | keep (merge → web) |
| execute_code | Persistent code kernel (`code_execution_tool.py:2424`, `:2496`) | code | 100K | X | — | skip |
| browser_* (12) + browser_exec, browser_cdp, browser_dialog | Browser (`browser_tool.py:6503-6622`, `browser_cdp_tool.py:744`, `browser_dialog_tool.py:137`) | — | — | N X | `@eN` refs | given |
| computer_use, vision_analyze, text_to_speech, image_generate, video_* | Desktop/vision/media (`computer_use_tool.py:21`, `vision_tools.py:1917`, `tts_tool.py:4816`) | — | — | X N | — | given (computer) / skip |
| tool_search / tool_describe / tool_call | Bridge for deferred tools (`tool_search.py:67-69`) | query / name / name+args | — | R | core working set never defers | keep |
| discord, kanban_*, ha_*, feishu_*, yb_*, desktop_ui set, setup_mcp | Platform-specific | — | — | — | — | skip |

## 3. Prime Agent (one tool: `ipython`)

Registry: `createAllToolDefinitions` returns only `ipython` (`packages/coding-agent/src/core/tools/index.ts:46-50`). Schema is one field, `code` (`ipython.ts:144-149`); `executionMode: "sequential"` (`:621`). Output truncation 2 000 lines / 50 KB (`truncate.ts:11-12`). Everything else is a Python name inside the kernel, taught by the prompt (`prompts/rlm.ts`).

| kernel name | what it does | input shape | output cap | perm | design detail | verdict |
|---|---|---|---|---|---|---|
| `bash(cmd)` | Starts a shell command in the background, returns a handle at once (`rlm.ts` REPL_CONTROL_PROMPT; `rlm/bash.py:108`) | command string | head 512 KB + rolling tail 1.5 MB, middle dropped (`bash.py:33-34, 60-61`) | X | handle API `h.pid/h.running/h.tail(n)/h.output()/h.poll()/h.kill()` (`bash.py:240-263`); "each bash() is its own process; use os.chdir" | steal the head+tail buffer |
| Python file ops | "Use Python for reading, searching, and editing files … Always assign read/search results to named variables" (`rlm.ts`) | — | — | R W | state persists across turns | skip (Coeus uses explicit tools) |
| `edit` skill | `await edit(path, old_str, new_str)` (`rlm.ts`, skills block) | exact strings | — | W | — | same as edit |
| `rlm('task', name=)` | Spawn child, returns at admission (`rlm/__init__.py:34-41`); `rlm.find_models` (`:117`), `list_subagents` (`:161`), `delete_subagent` (`:170`) | prompt, name, model | — | X | "end your turn instead of awaiting completion"; fan-in by files | merge → delegate |
| `rlm.harness.*` | CRUD on memories, prompt notes, skills, subagents: `create_memory` (`harness.py:531`), `update_memory` (`:544`), `create_prompt_note` (`:560`), `create_skill` (`:589`), `create_subagent` (`:648`), `record_refinement` (`:677`), `overview` (`:722`) | id, content, global_ | — | W | one state file, local vs global scope (`:78-95`) | merge → memory / skill |
| `mcp.list_tools / call_tool / reload / close` | MCP from Python (`rlm/mcp.py:472-489`) | server, tool, args | — | N | — | skip |
| skills as modules / CLIs | `await <skill>.<fn>()` or `<skill> --help` (`rlm.ts`; `rlm/skill.py:23`) | — | — | X | — | merge → skill |

Prime's lesson: one tool is the cheapest schema (~150 tokens) but pushes all guardrails into prose, and local models write bad Python. Not for Coeus's default path.

## 4. OpenCode (17 tools)

Registry: `tool/registry.ts:101-119` — invalid, task, read, question, todowrite, lsp, plan_exit, webfetch, websearch, bash, glob, write, edit, grep, apply_patch, skill, optional code-mode. Descriptions are `.txt` files next to each tool.

| tool | what it does | input shape | output cap | perm | design detail | verdict |
|---|---|---|---|---|---|---|
| read | File or directory; "By default … up to 2000 lines"; "Avoid tiny repeated slices (30 line chunks)" (`read.txt`) | filePath (absolute), offset (1-based), limit | 2 000 lines, 2 000 chars/line, 50 KB, "Use offset=N to continue" (`read.ts:13-17, 345`) | R | line prefix `N: `; images/PDF as attachments | keep |
| edit | Exact replace; fails on not-found or multiple matches; `replaceAll` (`edit.txt`) | filePath, oldString, newString, replaceAll | — | W | **nine fallback replacers** (Simple, LineTrimmed, BlockAnchor, WhitespaceNormalized, IndentationFlexible, EscapeNormalized, TrimmedBoundary, ContextAware, MultiOccurrence) and a "disproportionate match" guard (`edit.ts:694-710`) | keep |
| write | Overwrite; must Read first; "NEVER proactively create documentation files" (`write.txt`) | filePath, content | — | W | — | keep |
| glob / grep | Ripgrep-backed; "If you need to … count … use the Bash tool with rg" (`grep.txt`) | pattern, path, include | 100 rows then "(Results truncated…)" (`grep.ts:80-101`, `glob.ts:49`) | R | points to Task for open-ended search | merge → search |
| bash | "DO NOT use it for file operations … use the specialized tools" (`shell/shell.txt`) | command, timeout ms, description | default 2 min (`shell.ts:347`); kill + 3 s force (`:550-554`); 2 000 lines / 50 KB then "Full output saved to: file" (`truncate.ts:14-15`, `shell.ts:579`); metadata 30 000 chars (`:27`) | X | timeout message tells the model to retry with larger timeout (`:564`) | keep |
| task | Sub-agent; "When NOT to use …" list; `task_id` resumes; background=true "You will be notified … DO NOT sleep, poll" (`task.txt`, `task.ts:26-40, 44-60`) | description, prompt, subagent_type, task_id, background | — | X | depth limit `subagent_depth` (`:114`) | keep (as delegate) |
| question | Structured questions; custom answer row added by default; "(Recommended)" first (`question.txt`, `question.ts:6-7`) | questions[] | — | I | — | keep (as ask_user) |
| todowrite | 44-line description with when/when-not/states/rules (`todowrite.txt`) | todos[] | — | W | best written "when NOT to use" text in the sample | skip for Coeus |
| skill | "Load a specialized skill when the task … matches one … listed in the system prompt" (`skill.txt`, `skill.ts:8-9`) | name | — | R | — | merge → skill |
| webfetch / websearch | Fetch to markdown/text/html, 5 MB, 120 s max (`webfetch.ts:9-11`); search with `{{year}}` injected (`websearch.txt`) | url, format, timeout / query | 5 MB | N | year hint in description | merge → web |
| lsp | goToDefinition, findReferences, hover, … (`lsp.txt`) | filePath, line, character | — | R | — | skip |
| plan_enter / plan_exit | Suggest switching to plan agent / exit (`plan-enter.txt`, `plan-exit.txt`) | — | — | I | user confirms mode switch | skip |
| apply_patch | `*** Begin Patch` grammar (`apply_patch.txt`) | patch | — | W | GPT-family only | skip |
| invalid | "Do not use"; harness calls it with `{tool, error}` when args fail validation so the model gets a readable repair message (`invalid.ts:4-19`) | tool, error | — | — | **argument-repair channel** | keep the mechanism, not the tool |
| code-mode | "Script body executed by the confined interpreter" (`code-mode.ts:16-19`) | code | — | X | optional | skip |

## 5. Codex CLI (Rust, `codex-rs/core/src/tools`)

Specs are `ToolSpec::Function` (JSON) or `ToolSpec::Freeform` (grammar) or `Namespace` (`spec_plan.rs:436, 824, 1464`).

| tool | what it does | input shape | output cap | perm | design detail | verdict |
|---|---|---|---|---|---|---|
| exec_command | "Runs a command in a PTY, returning output or a session ID for ongoing interaction." (`handlers/shell_spec.rs:95-104`) | cmd, workdir, tty, yield_time_ms, max_output_tokens, shell, login, environment_id + sandbox_permissions `use_default\|with_additional_permissions\|require_escalated`, justification, prefix_rule, additional_permissions{network, file_system{enabled, read, write}} (`:232-262`) | default 10 000 tokens (`unified_exec/mod.rs:79`); output schema wall_time_seconds, output (`shell_spec.rs:225-228`) | X | **escalation is a field on the call**: model asks for more sandbox with a user-facing `justification`; `yield_time_ms` = foreground wait before session id | keep the escalation field |
| write_stdin | Continue a session (`:145-146`) | session_id, chars, yield_time_ms, max_output_tokens | 10 000 tokens | X | — | merge → shell |
| request_permissions | Ask for permissions ahead of time (`:179-180`) | permissions, reason | — | I | — | skip |
| apply_patch | Freeform; "do not wrap the patch in JSON"; Lark grammar (`handlers/apply_patch_spec.rs:9-27`) | raw patch | — | W | grammar-constrained edits | skip |
| update_plan | Plan steps (`plan_spec.rs:42-43`) | explanation, plan[{step, status}] | — | W | — | skip |
| view_image | Show an image (`view_image_spec.rs:42-43`) | path, detail | — | R | — | merge → read |
| request_user_input | Structured questions (`request_user_input_spec.rs:77-78`) | questions[{id, header, question, options[{label, description}]}] | — | I | same shape as OpenClaw ask_user | keep (as ask_user) |
| web_search | Hosted tool (`hosted_spec.rs:14`) | — | — | N | — | merge → web |
| spawn_agent, send_input/send_message, followup_task, resume_agent, wait_agent, list_agents, close_agent, interrupt_agent | Multi-agent v1/v2 (`multi_agents_spec.rs:123, 170-199, 232-344`) | agent_type, message, model, reasoning_effort, name, fork_context, fork_turns, timeout_ms… | — | X | fork with N turns of context | merge → delegate |
| tool_search | "Search query for deferred tools", limit (`tool_search_spec.rs:16-33`); MCP/dynamic tools carry `defer_loading: true` (`handlers/tool_search.rs:318, 375, 418-448`; `dynamic.rs:76`) | query, limit | — | R | — | keep |
| get_context_remaining, new_context_window | Context budget tools (`get_context_remaining_spec.rs:11`, `new_context_window_spec.rs:9`) | — | — | — | model can ask how much room is left | skip (Coeus injects budget in prompt) |
| list_mcp_resources, list_mcp_resource_templates, read_mcp_resource | MCP resources (`mcp_resource_spec.rs:23-80`) | — | — | N | — | skip |
| request_plugin_install, list_available_plugins_to_install, current_time, sleep, wait_for_environment, test_sync | Plumbing (`handlers/*.rs`) | — | — | — | — | skip |

Legacy `shell` function tool: selection runs through `ConfigShellToolType` (`spec_plan.rs:75, 995`); I did not find a `"shell"` spec by name in this checkout — **not verified**.

## 6. Claude Code (docs; hint list checked against tools-reference)

Verified from `https://code.claude.com/docs/en/tools-reference.md` unless noted.

| tool | what it does | input shape | output cap | perm | design detail | verdict |
|---|---|---|---|---|---|---|
| Read | Contents with line numbers; PARTIAL view notice when over token limit; `pages` for PDFs | file_path, offset, limit, pages | token-limited page | R | images resized; PDFs ≤20 pages per call | keep |
| Edit | Read-before-edit, exact match, uniqueness or `replace_all`; a Bash `cat/sed -n/rg` on one file counts as a read | file_path, old_string, new_string, replace_all | — | W | `Edit(...)` allow rule also grants Read | keep |
| Write | Create/overwrite | file_path, content | — | W | — | keep |
| Bash | Shell; `BASH_DEFAULT_TIMEOUT_MS` 2 min, `BASH_MAX_TIMEOUT_MS` 10 min; auto-moves to background on timeout except `sleep`, `git`, unparsable compounds; `run_in_background` | command, timeout, run_in_background, description | ~30 000 chars inline (`BASH_MAX_OUTPUT_LENGTH`, ceiling 150 000); files past 64 MiB; kill at 5 GB | X | **auto-background on timeout**; `cd` carry-over reset outside project | keep |
| Glob / Grep | File and content search; Grep modes files_with_matches/content/count, `head_limit`, `offset`, `multiline`, respects .gitignore | pattern, path, glob, type… | head_limit (default 250 — from hint, **not verified**) | R | — | merge → search |
| WebFetch / WebSearch | Fetch to markdown + prompt; search | url, prompt / query | 15-min cache | N | fetch takes a *prompt* and answers with a small model — big token saver | merge → web |
| Agent | Sub-agent in its own context; background by default; `fork` inherits the whole conversation; `isolation: worktree`; depth limit 3; resume by name via SendMessage (`sub-agents.md`) | description, prompt, subagent_type, name, model, isolation | — | X | background agents get a reduced tool set | keep (as delegate) |
| SendMessage / ListAgents | Message a subagent, teammate, or other session; optional `summary` ≤200 chars | to, message, summary | — | I | — | merge → delegate |
| AskUserQuestion | Multiple-choice with Other row; `askUserQuestionTimeout` auto-continue | questions[] | — | I | idle timeout with countdown | keep (as ask_user) |
| TodoWrite / TaskCreate/Get/List/Update | Checklist; TodoWrite disabled by default in favor of Task* | — | — | W | — | skip |
| Skill | Runs a skill in the main conversation | name, args | — | X | — | merge → skill |
| ToolSearch | Loads deferred tool schemas; on by default; "50 tools can use 10-20K tokens"; accuracy degrades past 30–50 tools; ≤5 results; `auto:N` threshold 10% of context (`agent-sdk/tool-search.md`) | query `select:a,b` or keywords | 5 | R | core tools never deferred | keep |
| CronCreate / CronList / CronDelete | Session-scoped 5-field cron, ≤50 tasks, 7-day expiry, 1-min granularity, jitter ≤30 min (`scheduled-tasks.md`) | cron, prompt, recurring | — | I | jitter derived from task id | keep (as schedule) |
| ScheduleWakeup | Self-paced loop: next run 1 min–1 h, `stop: true` ends | delay, stop | — | I | fallback wakeup 20 min if neither | merge → schedule |
| Monitor | Background command; each output line comes back as an event; WebSocket source | command / url, timeout_ms, persistent | 1 MiB per message | X N | event push instead of polling | skip (Coeus cron + shell background) |
| Workflow | Script orchestrating many subagents | script | — | X | — | skip |
| EnterWorktree / ExitWorktree, EnterPlanMode / ExitPlanMode | Isolation and mode switches | path | — | W | — | skip |
| TaskOutput / TaskStop | Background task output (deprecated → Read the output file) / stop | id | — | X | output-as-file pattern | merge → shell |
| SendUserFile / PushNotification / Artifact | Deliver a file (`display: render\|attach`), push to phone, publish page | path, caption | — | N I | — | merge → message |
| NotebookEdit, LSP, PowerShell, RemoteTrigger, ReportFindings, SendFeedback, EndConversation, ListMcpResourcesTool, ReadMcpResourceTool, WaitForMcpServers, ShareOnboardingGuide | Specialised | — | — | — | — | skip |
| MCP tools as `mcp__server__tool` | from hint, **not verified** on the page | — | — | — | — | — |
| Hooks | PreToolUse: `permissionDecision`, `updatedInput`; PostToolUse: `additionalContext`, `updatedMCPToolOutput` (MCP only) (`hooks.md`) | — | — | — | args can be rewritten before execution | keep the idea (policy layer) |

## 7. Atomic Agent (TypeScript, local-model first)

Tools are registered by subsystem builders (`src/tools/index.ts:12-18`; finish/reply there) and named with dots. The model never emits JSON tool calls freely: llama-server output is constrained by `grammars/tool-call.gbnf` — "Array-only root … Up to 16 calls per completion" (`:1-7`); tool names are enumerated in the grammar (`:8-15`) and the MCP branch is rebuilt by `buildGrammar` at runtime (`:20-25`). Transport default `"grammar"` (`src/agent/agent-loop.ts:630`); cloud providers get JSON Schemas with `additionalProperties: false` so "models that pad with stray keys … get a clean 400 … and learn to stop" (`src/prompt/default-tool-args-schemas.ts:1-30`).

| tool | what it does | input shape | output cap | perm | design detail | verdict |
|---|---|---|---|---|---|---|
| os.shell.run | "Run an OS command … Prefer the structured form `{cmd, args:[…]}`" (`src/tools/os/shell.ts:187-189`) | cmd, args[], timeoutMs | 4 MiB (`:20`); no default timeout (`:220-224`) | X | argv form avoids quoting bugs; command guard rules (`os/shell-command-guard/rules-*.ts`) | steal argv form as an option |
| os.fs.read / write / edit / patch / list / grep / glob / diff / hash / trash / watch / locate_project / read_document / archive.* | File family (`fs-read.ts:38`, `fs-write.ts:24`, `fs-edit.ts:26`, `fs-patch.ts:39`, `fs-grep.ts:61`, `fs-glob.ts:65`, `fs-trash.ts:118`…) | — | — | R W | `fs.trash` instead of delete; approval scope (`fs-approval-scope.ts`) | keep read/write/edit; merge grep+glob → search |
| os.http.request, os.web.search, os.web.fetch | Network (`http-request.ts:56`, `web-search/`) | — | — | N | — | merge → web |
| os.git.status/log/diff/show/blame/branch, os.proc.list/kill, os.clipboard.*, os.window.*, os.notify | OS helpers | — | — | R X | — | skip (shell covers) |
| browser.navigate/click/type/read_aria/search/tabs/scroll | Browser (`src/tools/browser/*.ts`) | — | — | N X | ARIA compressor | given |
| memory.profile.set/remove/list/history, memory.notes.store/recall/forget, memory.lessons.recall, memory.procedures.recall | Layered memory (`src/tools/memory/*.ts`) | — | — | R W | profile vs notes vs lessons vs procedures | merge → memory |
| tasks.schedule / cron / list / cancel / show | Scheduler (`src/tools/tasks/*.ts`) | — | — | I | — | merge → schedule |
| skill.view / skill.run_script, tool.view | Skill body; run a script; **fetch a tool's full arg schema on demand** (`skill-view.ts:7`, `skill-run-script.ts:20`, `tool-view/tool-view.ts:11`) | name | — | R X | tool.view = Atomic's tool_search | merge → skill / tool_search |
| vision.describe | Image → text (`vision/describe.ts:57`) | — | — | N | — | skip |
| reply / finish | Reply to user; end turn (`conversation/reply.ts:15`, `finish.ts:9`) | text | — | I | explicit end-of-turn tool keeps small models from rambling | keep idea (Coeus: plain text ends turn) |

## 8. ZeroClaw (Rust)

Tool trait (`crates/zeroclaw-api/src/tool.rs:371-418`): `name`, `description`, `parameters_schema`, optional `output_schema`, `param_domains`, trigger phrases, `execute`. 153 `fn name(&self)` impls across `zeroclaw-tools` and `zeroclaw-runtime/src/tools` (includes tests/mocks; ~120 real). Every tool result is stamped with an **HMAC-SHA256 receipt** `zc-receipt-{timestamp}-{hash}` from a per-session key "never exposed to the LLM" (`crates/zeroclaw-runtime/src/agent/tool_receipts.rs:1-13, 43`) so the loop can tell real tool output from hallucinated output; receipts flow into sub-agents (`tools/delegate.rs:1881-1947`).

| tool | what it does | input | cap | perm | detail | verdict |
|---|---|---|---|---|---|---|
| shell | "Execute a shell command in the workspace directory" (`runtime/src/tools/shell.rs:252-256`) | command | 1 MiB (`:13`); timeout `security.shell_timeout_secs` (`:104`); SIGKILL process group on cancel (`:16`) | X | allowed env var list (`:218`) | keep |
| file_read | Line numbers, offset/limit, base64 for binaries (`file_read.rs:51-55`) | path, offset, limit, encoding | — | R | — | keep |
| file_write / file_edit | Write (base64 option) (`tools/src/file_write.rs:38-42`); "replacing an exact string match" (`file_edit.rs:34-38`) | path, content / old, new | — | W | — | keep |
| content_search / glob_search | rg-backed regex; sorted glob (`content_search.rs:55-60`, `glob_search.rs:23-28`) | pattern, path | — | R | — | merge → search |
| memory_store / recall / forget / purge / export | Categories core/daily/conversation/custom (`memory_store.rs:24-28`); scored recall, `*` or since/until (`memory_recall.rs:22-26`) | content, category / query | — | R W | time-window recall | merge → memory |
| ask_user / escalate_to_human / poll / send_via / pushover / send_message_to_peer / deliver_file | Ask and block on channel (`ask_user.rs:52-57`); urgency routing (`escalate.rs:128-133`); native polls (`poll.rs:144-149`); "Control where and how this turn's reply is delivered" (`send_via.rs:204-209`); file to client (`runtime/tools/deliver_file.rs:176-181`) | — | — | N I | escalate with urgency; send_via for cross-channel | merge → ask_user / message |
| cron_add / list / update / remove / run / runs, schedule | Agent or shell jobs, cron/at/after/every (`runtime/tools/cron_add.rs:177-182`, `cron_list.rs:27-31`); shell-only `schedule` "output is only logged, NOT delivered" (`schedule.rs:53-58`) | — | — | I | receipts for cron mutations (`cron/scheduler.rs:27-30`) | merge → schedule |
| skills_list / skill_view / skill_manage / read_skill | "Read-only. Use before skill_view or skill_manage" (`skill_manage.rs:71-76, 171, 299`; `read_skill.rs:26`) | name | — | R W | — | merge → skill |
| delegate / spawn_subagent | "Delegate a subtask to a specialized agent … different model" (`delegate.rs:245, 1041`); ephemeral sub-agent inheriting policy (`spawn_subagent.rs:34, 66`) | task, model | — | X | — | merge → delegate |
| tool_search | "Fetch full schema definitions for deferred MCP tools … `select:name1,name2` … or keywords" (`tools/src/tool_search.rs:121-138`); 5 results (`:12`); deny-list wins (`:56-60`) | query, max_results | 5 | R | same query form as Claude Code | keep |
| web_fetch / web_search_tool / http_request / text_browser / browser / browser_open / browser_delegate / screenshot | Web and browser (`web_fetch.rs:414`, `web_search_tool.rs:1373`, `http_request.rs:466`, `browser.rs:1306`) | — | — | N | — | merge → web; browser given |
| llm_task | "Run a prompt through an LLM with no tool access … validates … JSON Schema" (`llm_task.rs:53-58`) | prompt, schema | — | N | cheap side-call | merge → delegate (mode: no tools) |
| TodoWrite, sop_*, security_ops, model_switch, vi_verify, sessions_*, git_*, jira/notion/linkedin/composio/google_workspace/microsoft365, coding-CLI runners, hardware_*, canvas, backup, image_gen, calculator, weather… | Everything else | — | — | — | — | skip |

---

## 9. Cross-agent comparison

**(a) File reading.** Everyone but Prime has a dedicated read tool with `offset/limit` and line numbers. Caps differ: OpenCode 2 000 lines / 2 000 chars per line / 50 KB (`read.ts:13-17`), Hermes 2 000 lines and ~100K chars with `next_offset` (`file_tools.py:2687`), OpenClaw byte pages 32–128 KB (`agent-tools.read.ts:77-81`), Claude Code a token-limited page with a PARTIAL notice. Prime tells the model to read with Python and keep results in variables. Coeus: OpenCode's numbers, Hermes's `next_offset` hint.

**(b) Editing.** Three grammars exist. Exact-match `old/new` (OpenClaw edit, Hermes patch, OpenCode edit, Claude Edit, ZeroClaw file_edit, Atomic os.fs.edit). Patch envelopes (`*** Begin Patch`) for GPT-family: OpenClaw apply_patch, OpenCode apply_patch, Codex freeform Lark grammar (`apply_patch_spec.rs:9-27`). Whole-file write everywhere. Only OpenCode softens exact-match with nine fallback replacers plus a size guard (`edit.ts:694-710`). Claude Code and OpenCode both enforce read-before-edit; Claude Code accepts `cat/sed -n/rg` on one file as the read. Coeus: exact-match with OpenCode's replacers; no patch grammar.

**(c) Shell.** Defaults: OpenCode 2 min (`shell.ts:347`), Claude Code 2 min / 10 min ceiling with auto-background on timeout, Hermes 180 s foreground with a max and `background+notify` (`terminal_tool.py:4163-4178`), OpenClaw `yieldMs` 10 s then auto-detach (`bash-tools.schemas.ts:37-40`), Codex `yield_time_ms` and PTY session ids (`shell_spec.rs:95-116`), Prime always-background handle (`bash.py:108`), Atomic no default timeout and argv form (`shell.ts:187-224`), ZeroClaw fixed policy timeout and 1 MiB cap (`shell.rs:13,104`). Output caps: 30K chars (Claude), 50 KB (OpenCode, Prime), 100K chars (Hermes), 10K tokens (Codex), 1 MiB (ZeroClaw), 4 MiB (Atomic). Sandboxing is a call-level field only in Codex (`sandbox_permissions`, `justification`) and OpenClaw (`elevated`, `ask`). Coeus: OpenClaw's yield-then-background, OpenCode's overflow-to-file, Codex's justification field for escalation.

**(d) Search.** Two camps. Tools: Claude Glob/Grep, OpenCode glob/grep (100-row cap), Atomic fs.grep/fs.glob, ZeroClaw content_search/glob_search, Hermes one `search_files` with `target` and `output_mode` (`file_tools.py:2814-2821`). Shell: Prime (`rg` via `bash()`), OpenClaw (exec). OpenCode's grep description explicitly punts counting to `rg` in Bash. Coeus: Hermes's single `search` tool; rg under the hood.

**(e) Web.** All have fetch + search except Prime (Python). Claude's WebFetch takes a `prompt` and returns an answer from a small model; Hermes web_extract returns raw markdown with a 15 000-char budget (`web_tools.py:1679`); OpenCode allows 5 MB / 120 s (`webfetch.ts:9-11`); OpenClaw caches 100 entries. Coeus: one `web` tool with `mode: search|fetch`, a char budget, optional `question` for local-model summarization.

**(f) Planning/todo.** OpenCode todowrite (44-line rules), Claude Task* tools, Codex update_plan, ZeroClaw TodoWrite, Hermes todo_list (deferred by default), OpenClaw goals/progress_card, Atomic none, Prime none. For a Signal assistant this is UI, not capability. Coeus: skip; plans live in the reply text or a file.

**(g) Asking the user.** Structured question objects with header/options/recommended-first: OpenClaw (`ask-user-tool.ts:28-75`), Codex request_user_input, Claude AskUserQuestion (idle timeout), OpenCode question, Hermes clarify (A/B says keep it eager, `tool_search.py:265-272`), ZeroClaw ask_user blocks on the channel. Coeus: keep eager; same shape; Signal renders as numbered list.

**(h) Delegation.** OpenClaw sessions_spawn + agents_wait + sessions_yield (three tools), Codex nine multi-agent tools, Claude Agent + SendMessage (fork, worktree, depth 3), OpenCode task (background, task_id resume, depth), Hermes delegate_task (one tool, output_schema), Prime `rlm()` (admission-return, fan-in by files), ZeroClaw delegate/spawn_subagent/llm_task, Atomic none. Coeus: one `delegate` with `background` and `wait`, like OpenCode/Hermes.

**(i) Memory recall.** OpenClaw memory_search+memory_get with "mandatory recall step" wording; Hermes memory (atomic batch, char limit) + session_search (FTS5); ZeroClaw five memory_* tools with categories and time windows; Atomic nine layered tools; Prime harness CRUD; Claude/Codex/OpenCode none (files only). Coeus: one `memory` tool with `search|get|save`, Hermes's batch-and-limit rules.

**(j) Skills.** Index in prompt, body on demand: Hermes skill_view/skill_manage (57-char trigger rule), OpenCode skill, Claude Skill, ZeroClaw skills_list/skill_view/skill_manage, Atomic skill.view/skill.run_script, OpenClaw skill_workshop, Prime skills as Python modules. Coeus: `skill` with `view|run|save`.

**(k) Scheduling.** OpenClaw cron (10 actions, 4 schedule kinds, delivery modes, one shared job schema), Hermes cronjob_manage (7 actions, background run), ZeroClaw six cron_* + schedule, Atomic tasks.*, Claude CronCreate/List/Delete (session-scoped, 7-day expiry, jitter) + ScheduleWakeup, Codex/OpenCode/Prime none. Coeus: one `schedule` tool, OpenClaw's single job object, Hermes's "list first, never guess ids".

**(l) Messaging the user.** OpenClaw message (dryRun, idempotency), ZeroClaw send_via/escalate/poll/deliver_file, Claude SendUserFile/PushNotification, Atomic reply/finish, Hermes send_message_tool (platform), Codex/OpenCode/Prime none (reply text is the message). Coeus: one `message` for out-of-turn sends and file delivery; the normal reply stays plain text.

**(m) Tool discovery/deferral.** Hermes: `tool_search/tool_describe/tool_call`, curated 19 deferred, working set never deferred, -49% schema tokens. Claude Code ToolSearch: on by default, ≤5 results, `auto:N` threshold, core tools exempt. Codex: `defer_loading` on MCP/dynamic tools plus tool_search. ZeroClaw tool_search: MCP only, `select:` form. Atomic `tool.view`: arg schema on demand, names still in the grammar. OpenCode: none (17 tools, all eager). OpenClaw: profiles, no deferral found. Coeus: a `tool_search` bridge for local models only; cloud models get the full set.

---

## 10. The Coeus tool set (20 tools)

To stay at 20 I fold four given tools into siblings: `browser_screenshot` becomes `browser_snapshot(mode: image)`, `browser_tabs` becomes `browser_open(action: list|close)`, `browser_file` becomes `browser_act` actions `upload|download`, and `computer_screenshot`+`computer_act` become one `computer` (OpenClaw does the same, `tool-catalog.ts:289`). `skill_run` becomes the `run` action of `skill`. If the harness author wants them separate, drop `write` (edit with `create: true`) and `tool_search` on cloud models.

| # | name | description as the model sees it (≤40 words) | input fields | output cap | perm | crib from |
|---|---|---|---|---|---|---|
| 1 | read | Read a file or list a directory with line numbers. Use offset/limit for big files; the result says how to continue. Not for images of screens (use browser_snapshot) or for searching (use search). | path, offset?, limit? | 2 000 lines, 2 000 chars/line, 50 KB, "use offset=N" | R | `opencode/.../tool/read.ts:13-17, 345`; `hermes tools/file_tools.py:2681-2687` |
| 2 | write | Create or fully overwrite a file. Read it first if it exists. Prefer edit for changes; do not create docs or notes unless asked. | path, content | — | W | `opencode/.../write.txt`; Hermes syntax check `file_tools.py:2695` |
| 3 | edit | Replace one exact text span in a file. old must match exactly and once, or set replace_all. Not for new files (use write). | path, old, new, replace_all? | — | W | `opencode/.../edit.ts:694-710` (replacers + guard); `edit.txt` |
| 4 | search | Find files by glob or lines by regex under a path. Ripgrep-backed. Not for reading whole files (use read). | pattern, target: content\|files, path?, glob?, mode: content\|files\|count, limit? (50), offset?, context? | 50 rows + "more with offset" | R | `hermes tools/file_tools.py:2810-2821`; `opencode grep.ts:80-101` |
| 5 | shell | Run a shell command. Returns output, or a job id if it runs longer than yield_s. Use action poll/log/kill with job_id. Not for reading, editing, or searching files. | command?, cwd?, timeout_s? (120, max 600), yield_s? (10), job_id?, action?: poll\|log\|kill, justification? | 30 000 chars inline, rest to a file path | X (I with justification) | `openclaw src/agents/bash-tools.schemas.ts:25-100`; `opencode shell.ts:347, 550-579`; Codex `shell_spec.rs:232-262`; Prime head+tail buffer `bash.py:33-34,60` |
| 6 | web | mode search: web results for a query. mode fetch: a page as text, optionally answering `question` instead of returning the page. Not for sites needing login (use browser_open). | mode: search\|fetch, query?, url?, question?, max_chars? (15 000) | 15 000 chars | N | `hermes tools/web_tools.py:1657-1679`; Claude WebFetch prompt idea (`tools-reference.md`) |
| 7 | memory | Recall or save durable facts. action search: ranked snippets; get: exact lines from a memory file; save: add/replace/remove entries in one atomic batch. Not for task progress or scratch notes. | action: search\|get\|save, query?, file?, lines?, operations?[{op, content, old}] | search 10 hits; get bounded excerpt | R W | `openclaw extensions/memory-core/src/memory-tool-contract.ts:87-101`; `hermes tools/memory_tool.py:1264-1284`; `zeroclaw memory_recall.rs:22-26` |
| 8 | ask_user | Ask JT one or more short questions and wait. Put the recommended option first; an "Other" row is added. Use only when a wrong guess would cost more than the round-trip. | questions[{id, header, question, options[{label, description?}], multi?}], timeout_s? | — | I | `openclaw src/agents/tools/ask-user-tool.ts:28-75`; `hermes clarify_tool.py:443-452` |
| 9 | message | Send a message or file to JT (Signal/web) outside the normal reply, or to another channel. The normal reply is plain text; do not use this to answer the current turn. | text, channel?, target?, file?, dry_run? | — | N I | `openclaw src/agents/tools/message-tool-schema.ts:22-70`; `zeroclaw send_via.rs:204-209`, `deliver_file.rs:176-181` |
| 10 | schedule | Manage scheduled jobs: add, list, get, update, remove, run. One job object: when (at\|every\|cron, tz), prompt or command, delivery. Always list before update/remove; never guess ids. | action, id?, job?{kind, at?, every_s?, cron?, tz?, prompt?, command?, deliver: none\|announce}, once? | list ≤50 | I | `openclaw src/agents/tools/cron-tool-schema.ts:19-46, 60-200`; `hermes cronjob_tools.py:2018`; Claude 7-day expiry (`scheduled-tasks.md`) |
| 11 | delegate | Run a sub-task in a fresh context and return its final text. background=true returns a job id and notifies you; do not poll. tools=none for a plain LLM call. Not for one quick lookup. | task, role?, model?, tools?, background?, output_schema?, wait_ids? | result ≤ 8 000 chars | X | `opencode tool/task.ts:26-60, 114`; `hermes delegate_tool.py:5358-5375`; `zeroclaw llm_task.rs:53-58` |
| 12 | skill | Skills are recorded procedures. action view: load SKILL.md or a linked file; run: execute its script with its declared permissions; save: create or patch a skill (atomic). | action: view\|run\|save, name, file?, args?, operations? | 20 000 chars | R X W | `hermes tools/skills_tool.py:2004-2014`, `skill_manager_tool.py:2138-2150`; study 11 `skill_run` |
| 13 | tool_search | Load the full schema of a deferred tool by name (select:a,b) or keyword. Only for tools listed as deferred in the system prompt. | query, max_results? (5) | 5 schemas | R | `zeroclaw crates/zeroclaw-tools/src/tool_search.rs:121-138`; `hermes tools/tool_search.py:67-69, 273-300` |
| 14 | browser_open | (given) Open or reuse a labeled tab in a persistent profile; action list/close for hygiene. | url?, label, profile?, action? | — | N | study 11; `openclaw extensions/browser/src/browser-tool.schema.ts:10-60` |
| 15 | browser_snapshot | (given) See the page as accessibility text with @refs (default) or as an image (mode: image). | label, mode?, query?, interactive? | text ≤ 20 000 chars | R | study 11; Hermes `browser_tool.py:6516` |
| 16 | browser_act | (given) Batch of click/type/press/select/hover/scroll/wait/upload/download by ref; stops on first error. | label, actions[] | — | N X | study 11; OpenClaw `batch` action |
| 17 | browser_eval | (given) Run JS in the page. Off unless the active skill allows it. | label, js | 10 000 chars | X | study 11 |
| 18 | browser_handoff | (given) Pause and hand the window to JT for login/2FA/captcha; resume when he says done. Never type passwords. | label, reason | — | I | study 11 |
| 19 | computer | (given) Desktop last resort: action screenshot or act (click/type/key). Every act needs approval. Not while a browser tool can do it. | action, x?, y?, text?, key? | image | X I | study 11; `openclaw tool-catalog.ts:289-295` |
| 20 | email_send | (given) Send an email draft that is already marked approved in the ledger. Cannot send unapproved drafts. | draft_id | — | N I | study 11 |

**Token cost.** Claude's docs put 50 tools at 10–20K tokens (200–400 each); Hermes measured its desktop set at 13.4K → 6.9K tokens (`git log e16ad33a9d`). With 40-word descriptions and 3–7 fields, Coeus's 20 tools land near 150–220 tokens each: **about 3.5–4.5K tokens for the full set**, roughly 2K for the 13 core tools and 1.5–2.5K for the browser/computer/email group. Prompt caching hides this on cloud models; on a local 8–32B model at 8–32K context it is 10–30% of the window, so defer.

**Deferral for local models (behind `tool_search`).** Eager (13): read, write, edit, search, shell, web, memory, ask_user (Hermes A/B: must be eager), message, skill, browser_open, browser_snapshot, browser_act. Deferred (7): schedule, delegate, browser_eval, browser_handoff, computer, email_send, and `tool_search` itself lists them in one line each in the system prompt. This trims ~1.5K tokens per turn and keeps the local model at 13 choices, under the 30–50 accuracy cliff Claude's docs cite. Cloud models get all 20 eager.

**Mechanisms to copy that are not tools.** OpenCode's `invalid` repair channel (`invalid.ts:4-19`), ZeroClaw's HMAC receipts on every result (`tool_receipts.rs:1-13`), Claude Code's PreToolUse `updatedInput` as the policy layer (`hooks.md`), and Atomic's GBNF grammar with an array-only root when Coeus runs on llama-server (`grammars/tool-call.gbnf:1-13`).
