# 14. OpenCode's "doom loop", loop guards elsewhere, and token discipline

Date: 2026-09-02. Read-only code study of five repos at HEAD:
OpenCode `69c172e8a7`, OpenClaw `752a983480e`, Hermes `95f62ca3bf`, Prime `0ba0423c5`, ZeroClaw `73ff732e6`.
Paths are relative to each repo. "Estimate" and "not verified" are used where I did not run code.

## 0. The short answer

The claim was: "the doom loop is what lets OpenCode work well with small local models."
The code does not support that. The doom loop is a small safety net that asks the user a question. By my reading it does not even catch the most common small-model loop (one identical call per step). What actually keeps OpenCode alive with weak models is a set of other, quieter mechanisms: an `invalid` tool that swallows malformed calls, "continue on unknown finish reason", output truncation, compaction with a reserve, and a cheap default prompt. Details and citations below.

## 1. OpenCode's doom loop, precisely

### 1.1 The condition

- Threshold constant: `const DOOM_LOOP_THRESHOLD = 3` (`packages/opencode/src/session/processor.ts:29`).
- Check runs on every `tool-call` stream event, after the call is recorded as a `running` tool part (`processor.ts:331-351`).
- It loads all parts of the current assistant message and takes the last three: `parts.slice(-DOOM_LOOP_THRESHOLD)` (`processor.ts:352-356`).
- All three must be tool parts, with the same tool name, not `pending`, and equal input by `JSON.stringify(part.state.input) === JSON.stringify(input)` (`processor.ts:358-368`). So comparison is tool name plus exact serialized input. Key order matters; no canonicalisation, no hashing.
- The current call counts as one of the three. So it fires on the third identical call.

### 1.2 Why it rarely fires (important)

`MessageV2.parts()` returns every part type, ordered by id (`packages/opencode/src/session/message-v2.ts:492-503`). The processor writes a `step-start` part at the start of every LLM step (`processor.ts:426-432`) and a `step-finish` part at the end (`processor.ts:459-468`). OpenCode runs one model call per step in its own loop (`session/prompt.ts:1178-1179` tracks `step`).

So for a model that makes one identical call per step, the last three parts when the third call arrives are `[step-finish, step-start, tool]`. The `every(part.type === "tool")` test fails and the check returns early. The detector only passes when the three identical calls sit next to each other inside one response, i.e. three identical parallel tool calls. The original implementation had the same shape (`git show a1214fff2e`, "Refactor agent loop (#4412)", 2025-11-17: `parts.slice(-DOOM_LOOP_THRESHOLD)` then `every(p.type === "tool" ...)`), and there is no test that exercises a multi-step loop (`packages/opencode/test/agent/agent.test.ts:469-472` only checks the default permission). Not verified at runtime; a 10-minute test with a stub model would settle it. Either way, this is not the mechanism that makes small models usable.

### 1.3 What fires

When the check passes, the processor calls `permission.ask({ permission: "doom_loop", patterns: [toolName], metadata: { tool, input }, always: [toolName], ruleset: agent.permission })` (`processor.ts:371-379`). It goes through the normal permission engine:

- Rules are evaluated per pattern with the agent ruleset plus session approvals (`packages/opencode/src/permission/index.ts:72-82`). `deny` throws `DeniedError` at once; `allow` skips the ask; otherwise an `Asked` event is published and the processor waits on a deferred (`permission/index.ts:84-107`).
- The user sees, in CLI run mode, icon `⟳`, title "Continue after repeated failures", line "This keeps the session running despite repeated failures." (`packages/opencode/src/cli/cmd/run/permission.shared.ts:111-117`) with options "Allow once", "Allow always", "Reject" (`permission.shared.ts:135-139`). The TUI home tip says "Permission doom_loop prevents infinite tool call loops" (`packages/tui/src/feature-plugins/home/tips-view.tsx`).
- "Allow always" adds a session rule `{permission: "doom_loop", pattern: <toolName>, action: "allow"}` (`permission/index.ts:145-150`), which lasts until restart (`permission.shared.ts:130-133`).

### 1.4 What the model sees

- Allow: nothing. The ask returns, the processor returns from the event (`processor.ts:379-380`), the tool executes, and the model gets an ordinary tool result. No nudge text is injected.
- Reject: `PermissionRejectedError` with message "The user rejected permission to use this specific tool call." (`packages/core/src/v1/permission.ts:7-11`) is thrown inside the stream handler. The retry policy only retries API errors (`session/retry.ts:85-97`), so `halt()` stores the error on the assistant message and sets the session idle (`processor.ts:613-640`), and `process()` returns `"stop"` (`processor.ts:694`). The turn ends with a visible error. On the next user turn, assistant messages that carry an error are dropped from the model's view entirely (`message-v2.ts:248-256`), so the model never reads "you were looping"; the looping message simply disappears from history. Whether the third call's side effects run before the stream is torn down: not verified.
- Deny in config: same path as reject, without asking.

### 1.5 Defaults, config, and hooks

- Built-in default ruleset: `"*": "allow"`, `doom_loop: "ask"`, `external_directory: ask`, `question: deny`, `.env` reads ask (`packages/opencode/src/agent/agent.ts:119-134`). The `build` agent merges `question: allow`, `plan_enter: allow` (`agent.ts:145-152`).
- Config knob: `permission.doom_loop: "ask" | "allow" | "deny"` (`packages/core/src/v1/config/permission.ts:32`; docs `packages/web/src/content/docs/permissions.mdx:164,173`).
- `experimental.continue_loop_on_deny: true` makes a denied tool call a non-blocking tool error instead of ending the turn (`processor.ts:647`, `200-201`; schema `packages/core/src/v1/config/config.ts:179`).
- Plugin hook: `"permission.ask"?: (input, output: {status: "ask"|"deny"|"allow"})` (`packages/plugin/src/index.ts:261`). I did not find the trigger site in `packages/opencode/src`; not verified whether it wraps `Permission.ask` today.

### 1.6 Verdict

The doom loop is one of several mechanisms and the weakest of them. It is off-path for sequential loops, it asks a human rather than steering the model, and on reject it ends the turn instead of recovering. The real work is done by the items in section 2.

## 2. What actually helps OpenCode with small models

| Mechanism | What it does | Where |
|---|---|---|
| `invalid` tool | AI SDK `experimental_repairToolCall`: if the model used a wrong-case name that exists in lowercase, fix the name; otherwise rewrite the call into tool `invalid` with `{tool, error}` | `session/llm.ts:303-315` |
| | `invalid` returns text "The arguments provided to the tool are invalid: <error>" as a normal tool result, so the loop continues and the model can retry | `tool/invalid.ts:9-21` |
| | `invalid` is in `tools` but removed from `activeTools`, so the model never sees it in the list | `llm.ts:317` |
| Continue on unknown finish | Finish reasons `tool-calls` and `unknown` both keep the loop going; only other reasons end it. Added 2026-08-21 (#43892, v1.18.21) | `session/prompt.ts:1113`, `1295`; commit `57fa34f235` |
| Content filter surfaced | `content-filter` finish becomes an explicit error, not silent idle | `prompt.ts:1301-1309` |
| Retry/backoff | Retries 5xx, SDK-retryable, and "resource exhausted"-style messages; honours `retry-after`; never retries context overflow | `retry.ts:39`, `51-67`, `85-97` |
| Empty response | No empty-response guard found in `session/prompt.ts` or `processor.ts`. Not verified further |  |
| Provider transforms | Deepseek gets a `reasoning` part on every assistant message; `reasoning_content` echoed back where required; tool-call ids scrubbed | `provider/transform.ts:303-315`, `329-345`, `232-280` |
| Tool schema rewrite | `schema()` sanitises only for `@ai-sdk/openai`/`azure`; type arrays become `anyOf`; `required` filtered to real properties. LM Studio/Ollama use `@ai-sdk/openai-compatible`; whether they get this is not verified | `transform.ts:1546-1567`, `1642-1660` |
| Per-model tool set | GPT-5-class models get `apply_patch` and lose `edit`/`write`; everyone else keeps `edit`/`write` | `tool/registry.ts:297-300` |
| Output caps | `MAX_LINES = 2000`, `MAX_BYTES = 50 KB`; overflow written to a file kept 7 days; overridable via `tool_output` config | `tool/truncate.ts:12-16`, `41`, `69` |
| Compaction | Overflow when used tokens >= `usable` = input limit minus reserve, reserve = min(20 000, max output) | `session/overflow.ts:8-33` |
| | Prune old tool outputs before summarising: protect the newest 40 000 tokens of tool output, prune only if > 20 000 would be freed; opt-in `compaction.prune` | `session/compaction.ts:28-31`, `273-315` |
| | Keep the most recent 25% of usable context verbatim after a summary | `compaction.ts:115-118` |
| `small_model` | Titles and summaries run with `small: true`, no tools, `retries: 2`, on `small_model` if set | `session/prompt.ts:218-230`; `provider/provider.ts:1938-1950` |
| Per-model prompt | Claude, GPT, Gemini, Kimi get their own prompt; everything else (LM Studio, Ollama) gets `default.txt`, 8 528 bytes. Env block of ~10 lines follows | `session/system.ts:28-48`, `73-82` |
| Step cap | `agent.steps` (optional, default unbounded); at the cap the model gets `MAX_STEPS_PROMPT` and must answer in text. No other per-turn tool cap found | `packages/core/src/v1/config/agent.ts:34-37`; `prompt.ts:1178-1179`; `packages/core/src/session/runner/max-steps.ts` |
| Prompt cache | `applyCaching` marks the first two system messages and the last two non-system messages; message-level for anthropic/bedrock, else on the last content part | `transform.ts:358-405` |

Estimated fixed prefix for the `build` agent on a local model: `default.txt` ~2 100 tokens + env ~100 + tool descriptions (~11 of the 15 `.txt` files, ~3 500) + JSON schemas (~800) ≈ 6 500 tokens before AGENTS.md. Estimate; the bash description is not a `.txt` file and was not counted.

## 3. Equivalents in the other four agents

### OpenClaw

- Hash key: `${toolName}:${sha256Hex(stableStringify(params))}` (`src/agents/tool-loop-detection.ts:70-72`). Canonical, order-independent.
- History window 30 (`:49`); warning threshold 10 (`src/agents/tool-loop-thresholds.ts:1`); critical 20 (`:51`); global circuit breaker 30 (`:52`); unknown-tool repeat 10 (`:50`).
- Six detectors: generic repeat, argument churn, unknown-tool repeat, known-poll no-progress, global circuit breaker, ping-pong (`:28-34`, ladder `:544-660`).
- Warnings go out once per bucket of 10 calls per key (`agent-tools.before-tool-call.diagnostics.ts:63`, `551-563`); critical results block the call (`tool-loop-admission.ts:58-65`).
- One-shot recovery: the blocked call gets an error result "<reason>\n\nDo not repeat this exact tool action. Reassess the task. You may answer the user, ask for clarification, or continue with a different tool or different arguments." (`packages/agent-core/src/agent-loop.ts:1578`).
- Terminate on second critical: another critical loop during recovery ends the run with "This run is stopping now." (`agent-loop.ts:75-76`, `1575-1576`).
- `packages/tool-call-repair` promotes plain-text tool calls (Harmony `<|channel|>`/`<|message|>`/`<|call|>`, `[END_TOOL_REQUEST]`) into native blocks with a name resolver (`grammar.ts:2-8`, `promote.ts:36-50`).
- Tool result cap: 16 000 chars default, 32 000 at >= 100k context, 64 000 at >= 200k, never more than 30% of the window (`src/agents/tool-result-limits.ts:4-9`, `26-32`).
- Cache boundary `"\n<!-- OPENCLAW_CACHE_BOUNDARY -->\n"` (`packages/ai/src/utils/system-prompt-cache-boundary.ts:8`) closes the stable section (`src/agents/system-prompt.ts:1432`); date/timezone lead the volatile suffix (`:1437-1439`); runner joins `stable + boundary + dynamic` (`embedded-agent-runner/run/attempt-system-prompt.ts:79`).

### Hermes

- No identical-tool-call detector found. `repetition_guard.py` is about repeated text: 60-char windows, >= 5 repeats, 50% dominance, fragments >= 400 chars (`agent/repetition_guard.py:28-40`). Searched `conversation_loop.py` and `agent_runtime_helpers.py` for repeat/identical/consecutive; nothing tool-level. Not verified exhaustively.
- Empty responses: retry budget 3, dropped to 1 when a streak costs over $0.25; "deterministic empty" needs >= 2 consecutive zero-output attempts with usage present (`agent/empty_response_guard.py:28-30`, `56-57`, `234-237`).
- Liveness watchdog: 600 s idle timeout, 15 s poll (`agent/turn_liveness.py:67-68`).
- Wrong tool names: `repair_tool_call` lowercases, maps hyphens/spaces to underscores, CamelCase to snake_case, strips `_tool` suffixes twice, then difflib fuzzy match at 0.7 (`agent/agent_runtime_helpers.py:3748-3766`). Still unknown: tool result "Tool '<name>' does not exist. Available tools: ..." (`agent/conversation_loop.py:1547`). Empty name: a terse "that was data, not a call; reply in plain text" message, on purpose without the catalogue (`:1524-1545`).
- Models that cannot emit native tool calls: I found a text parser only in the ACP bridge (`agent/acp_openai_bridge.py:211 extract_tool_calls_from_text`), not in the main loop. Not verified that the main loop has an XML fallback.
- Tool deferral: 33 core tools always exposed (`toolsets.py:31-`), 18 names deferred by default behind `tool_search`/`tool_describe` bridges (`tools/tool_search.py:4`, `273-280`). The 19 in the brief is close; I count 18 in the frozenset.
- Caching: four `cache_control` breakpoints, static system prefix, end of system prompt, last two non-system messages (`agent/prompt_caching.py:3-8`); the prefix registry lets the planner place a breakpoint at the stable boundary (`agent/prompt_cache_boundary.py:8-9`, `56`, `69`).
- Compression: trigger at 50% of context (`agent/context_compressor.py:3422`), 75% for small contexts (`:1360`), summary budget 20% (`:817`). Caps: memory context 6 000 chars (`agent/context_engine.py:34`), context files 20 000 (`agent/prompt_builder.py:1441`), web extract 15 000 (`tools/web_tools.py:635`).
- Style prompt worth copying: "Be direct: match the length of your reply to the weight of the ask ... No filler ... no narrating tool calls the user can see." (`agent/prompt_builder.py:160-170`).

### Prime

- No repeated-call detector found in `packages/coding-agent/src/core` (grep for consecutive/identical/stuck). Loop control is budget-based: autonomous limits `maxContinuations 3, maxTurns 12, maxTokens 80 000, timeoutMs 30 min` (`core/autonomous.ts:48-55`).
- Truncation: 2 000 lines / 50 KB, grep lines cut at 500 chars (`core/tools/truncate.ts:11-13`). Same numbers as OpenCode.
- Caching: Anthropic markers on the system prompt blocks, the last tool definition, and the last user message (`packages/ai/src/providers/anthropic.ts:966-982`, `1188-1197`; `openai-completions.ts:744`, `781`, `795`).

### ZeroClaw

- `LoopDetectorConfig` default `window_size 20, max_repeats 3` (`crates/zeroclaw-runtime/src/agent/loop_detector.rs:22-27`). Args hashed from canonical JSON (`:76`). Only successful calls are recorded (`turn/results_collect.rs:67-71`).
- Ladder on consecutive identical calls: 3 → `Warning` "Try a different approach."; 4 → `Block`; 5 → `Break` circuit breaker (`loop_detector.rs:204-232`). Ping-pong needs 4 full cycles (`:242`); no-progress needs 5 calls with different args and identical output (`:307`).
- Handling: Warning appends a system message "[Loop Detection] ..."; Block replaces the tool output with "[Loop Detection — BLOCKED] ..." and continues; Break aborts the turn (`results_collect.rs:74-121`). 3 identical tool outputs in a row also abort (`:190-191`).
- Malformed text tool calls: `MAX_MALFORMED_TOOL_PROTOCOL_RETRIES = 2` (`turn/mod.rs:93`). The model gets a user-role "[Tool call parse error] ... Use the supported tool-call schema, or answer in natural language if no tool is needed." (`:1045-1056`); after two retries a fixed fallback ends the turn (`:1058-1067`).
- Iteration cap: `DEFAULT_MAX_TOOL_ITERATIONS = 10` when config is 0 (`:97`, `:509-513`). Presets: `tight` 10 iterations / 8k context / 8 000 result chars; `local_small` 4 / 8k / 4 000, keep 1 tool turn; `unbounded` 100 / 128k / 64 000 (`crates/zeroclaw-config/src/presets.rs:225-297`).
- Parser crate: `parse_tool_calls(text) -> (clean_text, calls)` (`crates/zeroclaw-tool-call-parser/src/lib.rs:2094`), unwraps JSON-in-string args (`:99`), classifies malformed envelopes (`:726-790`), rebuilds native history from parsed calls (`:2681`).
- `tool_search` loads deferred MCP schemas on demand, 5 results max (`crates/zeroclaw-tools/src/tool_search.rs:1-12`). Text-protocol instructions enter the system prompt only when exposed (`agent/agent.rs:2060-2065`). OpenRouter gets `cache_control: ephemeral` on text parts (`crates/zeroclaw-providers/src/openrouter.rs:401-403`).

### Side-by-side

| | OpenCode | OpenClaw | Hermes | Prime | ZeroClaw |
|---|---|---|---|---|---|
| Identical-call key | name + `JSON.stringify(input)` | name + sha256(stable JSON) | none found | none found | name + hash(canonical JSON) |
| First action | ask user at 3 (same-step only) | warn at 10 (feedback) | n/a | n/a | nudge at 3 |
| Hard stop | reject ends turn | block at 20, stop on 2nd critical, breaker 30 | n/a | budgets (12 turns) | block at 4, abort at 5 |
| Malformed call | `invalid` tool result | text-call promotion | name repair + catalogue | n/a | parser + 2 retries |
| Per-turn cap | `steps` (unset) | n/a here | n/a here | maxTurns 12 (autonomous) | 10 (4 on local_small) |
| Tool result cap | 2 000 lines / 50 KB | 16k chars, <= 30% ctx | per-tool (web 15k) | 2 000 lines / 50 KB | 8k/4k/64k by preset |
| Compaction trigger | ctx - 20k reserve | not read | 50% (75% small) | threshold (not read) | `compact_context` per preset |
| Cache points | 2 system + last 2 | boundary in system | 4 breakpoints | system + last tool + last user | OpenRouter ephemeral |

## 4. Token-minimisation and caching checklist

| Rule | Evidence |
|---|---|
| Keep a byte-stable prefix; put date/time after a boundary | OpenClaw `system-prompt.ts:1432-1439`; Hermes `prompt_cache_boundary.py:8-9` |
| Mark cache points at end of stable system, end of full system, last two messages | Hermes `prompt_caching.py:3-8`; OpenCode `transform.ts:358-405` |
| Defer rarely used tools behind a search tool | Hermes `tool_search.py:273-280`; ZeroClaw `tool_search.rs:1-12` |
| Cap tool results hard; spill to a file with a path | OpenCode `truncate.ts:14-15`, `69`; ZeroClaw `presets.rs:266` (4 000 chars local) |
| Compact early on small contexts; keep a reserve | Hermes 50%/75% (`context_compressor.py:3422`, `1360`); OpenCode 20k reserve (`overflow.ts:8`) |
| Prune old tool outputs before summarising | OpenCode `compaction.ts:28-31`, `273-315` |
| Use a small model for titles and summaries | OpenCode `prompt.ts:218-230` |
| Pick a shorter prompt for unknown/local models | OpenCode `system.ts:48` → `default.txt` |
| Keep style rules short and behavioural | Hermes `prompt_builder.py:160-170` |

Estimated fixed tokens per turn, default agent, no project instructions (all estimates, none measured):

| Agent | System prompt | Tools | Total fixed | Basis |
|---|---|---|---|---|
| OpenCode build, local model | ~2 200 | ~4 300 (11 tools) | ~6 500 | `default.txt` 8.5 KB, `tool/*.txt` 15 KB |
| OpenClaw default | ~6 000-12 000 | ~3 000+ | ~10 000-15 000 | builder is 67 KB source with many sections; not measured |
| Hermes default | ~1 500-6 000 (memory 6k chars, context files 20k chars caps) | ~6 000-9 000 (33 core tools) | ~8 000-15 000 | `toolsets.py:31`, `context_engine.py:34` |
| Prime default | ~1 500 | ~2 000 | ~3 500 | `system-prompt.ts` 7.7 KB source |
| ZeroClaw local_small | must fit in 8k total | text protocol only if exposed | ~1 500-3 000 | `presets.rs:249-270` |

## 5. Recommendation for Coeus

### 5.1 Loop guard (five parts)

1. Identical-call detector. Key = tool name + canonical JSON of args (sorted keys, like OpenClaw/ZeroClaw). Count consecutive identical calls across steps, ignoring step markers (fix OpenCode's flaw). Window 20. Ladder: 3rd identical call → do not execute; return a tool result "Same call as the last two. The result would be identical. Do something different or answer." 4th → same text plus end the turn with a one-line report. Also stop after 3 identical tool outputs in a row (ZeroClaw `results_collect.rs:190-191`). Never ask the user; the user is on Signal and may be away.
2. Malformed-call catcher. Unknown name → lowercase, `-`/space → `_`, CamelCase → snake, strip `_tool`, fuzzy 0.7 (Hermes). Still unknown → tool result "Tool 'x' does not exist. Available: a, b, c." Empty name → "That was data, not a call." Bad JSON → tool result with the validator message (OpenCode `invalid`). Never abort the turn on these.
3. Text tool-call repair. Parse `<tool_call>{json}</tool_call>`, ```json fences, and Harmony markers from plain text (ZeroClaw parser, OpenClaw promote). After 2 unparseable attempts, tell the model once, then accept the text as the answer (ZeroClaw `MAX_MALFORMED_TOOL_PROTOCOL_RETRIES = 2`).
4. Per-turn tool cap. 8 tool iterations for local models, 20 for cloud, configurable per agent. At the cap send one short user-role message: "Tool budget used. Answer in text: what you did, what is left." (OpenCode `MAX_STEPS_PROMPT`, cut to two lines.)
5. Repair feedback with admissible alternatives. Every guard message ends with the same three options: answer the user, ask one question, or use a different tool/arguments (OpenClaw `agent-loop.ts:1578`). Also: treat finish reason `unknown` like `tool-calls` (OpenCode), retry empty output twice then stop (Hermes budget 3 → 1), 10-minute idle watchdog (Hermes 600 s).

### 5.2 Token budget rules

- Fixed prefix <= 2 000 tokens for local models. Contents in order: identity + style (<= 110 tokens), environment (<= 60), 6-8 core tools with <= 40-word descriptions and flat schemas (<= 1 200), a `tool_search` bridge listing deferred tool names only (<= 150), memory summary (<= 300), skills as names only. Everything else is loaded on demand.
- Dynamic tail after the cache boundary: date/time, active job list, last approvals.
- Tool result cap: 4 000 chars on local, 16 000 on cloud; overflow to a file and return "[truncated: N of M lines; full output: <path>]".
- Compaction at 50% of context on local (75% if context <= 16k), reserve 4k output; prune old tool outputs first; run summaries on a small model.

### 5.3 Cache layout

```
[system: identity+style | env | core tools | tool_search manifest]   <- stable, cache point A
<!-- NERDGENIE_CACHE_BOUNDARY -->
[system: date/time | memory delta | jobs]                              <- cache point B
[messages ... last two messages]                                       <- cache points C, D
```
Anthropic: four `cache_control` markers at A, B, and the last two messages (Hermes layout). OpenAI-compatible cloud: same order, markers only where the provider accepts them. LM Studio and Ollama: prefix caching is byte-exact, so never change anything above the boundary within a session; keep `num_ctx` fixed.

### 5.4 Output-style snippet (74 words)

"You are Coeus, JT's home assistant. Write plain, short English. Match length to the ask: one-line question, one-line answer. No filler, no restating the request, no narrating tool calls. State facts; say 'not sure' when unsure. Use a tool only when needed, and never repeat a call with the same arguments. When work is done, report three things: what changed, what you checked, what is left. Ask at most one question at a time."

Tool-result formatting rule: every tool result is plain text. Line 1 is a status: `ok`, `ok (truncated)`, or `error: <one sentence>`. Then the payload. Truncated results end with the file path line above. Error results end with "Options: answer the user, ask one question, or try different arguments." No JSON wrappers, no markdown tables inside results.

### 5.5 Minimal command set (12) for Signal and terminal

OpenCode's TUI has 12 session-scoped slash commands (`share, rename, timeline, fork, compact, unshare, undo, redo, timestamps, thinking, copy, export`; `packages/tui/src/routes/session/index.tsx`) and about 14 app-scoped ones (`session.new, session.list, model.list, agent.list, mcp.list, provider.connect, theme.switch, help.show, opencode.status, app.exit`, and others; `packages/tui/src/app.tsx`). Plugin commands join through one shim exposing `slashName`/`slashAliases` (`packages/tui/src/plugin/command-shim.ts:59-60`). I count 26; the 31 in the brief is not verified.

| Command | One-line semantics |
|---|---|
| `/new` | Start a fresh session; old one stays listed. |
| `/sessions` | List recent sessions with id, title, age; `/sessions <id>` switches. |
| `/model [name]` | Show or set the model for this session (local or cloud). |
| `/compact` | Summarise history now; keep the last few messages verbatim. |
| `/undo` | Drop the last assistant turn and its file changes. |
| `/status` | One screen: model, tokens used, cache hit, active jobs, pending approvals. |
| `/help` | The command list, one line each. |
| `/stop` | Cancel the running turn or job. |
| `/pause` / `/resume` | Pause or resume all scheduled jobs (one command with a flag). |
| `/jobs` | List cron/scheduled jobs with next run; `/jobs <id> run` runs one now. |
| `/approve <id>` | Approve a pending tool or job action; `/approve <id> always` remembers it for the session. |
| `/deny <id>` | Reject a pending action; the model gets a one-line error result. |

One registry serves both surfaces: a table of `{name, aliases, argsSchema, run(ctx) -> string}`. The handler returns plain text; the terminal prints it, Signal sends it. Approvals follow OpenCode's model: the core emits a `permission.asked` event (`packages/opencode/src/cli/cmd/run.ts:801`) with an id, and any client answers with once/always/reject (`permission/index.ts:109-170`). In the terminal that is a dialog; on Signal it is a message "Approve #3 (bash: rm -rf build)? /approve 3 or /deny 3". `/exit` is terminal-only and is not in the shared set.

## 6. Open items (not verified)

- Runtime confirmation that OpenCode's doom loop never fires for one-call-per-step loops (section 1.2).
- Whether `@ai-sdk/openai-compatible` requests get OpenCode's schema sanitiser.
- Where OpenCode triggers the `permission.ask` plugin hook.
- Measured token counts for OpenClaw and Hermes system prompts.
- Whether Hermes's main loop has a text tool-call fallback for models without native tool calling.
