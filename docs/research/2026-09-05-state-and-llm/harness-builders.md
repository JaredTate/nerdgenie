# What the harness builders published, 2025 to 2026, and what it means for Nerd Genie

This report reads what the teams behind the best-known agents have written about how they make their agents work, and turns each lesson into something Nerd Genie could do. A harness is the program wrapped around a language model that gives it memory and hands. Every source below is real, was fetched, and is dated. Where a page could not be fetched directly (OpenAI's own harness post blocks robots), the report says so and uses the secondary write-ups it did read. Section 1 is the ranked list of ideas. Section 2 is what each source actually says. Section 3 is the source list.

A few terms used throughout. A token is a piece of text about the size of a short word. The context window is the most text a model can read in one call. The KV cache (key-value cache) is the model server's saved copy of the part of a prompt it has already read; a cache hit means that part did not have to be read again. Prefill is the work of reading the prompt before the model writes its first word. A tool call is the model asking the harness to run something.

## 1. Top ideas for Nerd Genie, ranked by likely impact

### 1. Make the front of the prompt append-only and move the changing part of the record to the very end

**What.** Today the record's Work and Lessons sections sit in the middle of the prompt and change every turn, so everything after them (pinned evidence, recent messages) is re-read from scratch on every call. Move the changing sections to the tail, after the recent messages, so the front of the prompt only ever grows and the only rewritten text is the last two or three thousand tokens.

**Why.** Manus calls KV-cache hit rate "the single most important metric for a production-stage AI agent" and reports a 10x price gap between cached and uncached tokens and a 100:1 input-to-output ratio; its two rules are "keep your prompt prefix stable" and "make your context append-only." Manus also found that rewriting a todo list at the end of the context is a way of "reciting its objectives into the end of the context," which fights the model losing track in the middle. Hermes documents the same rule for its system prompt: "avoid mutating it mid-conversation." On the local llama-server there is no bill, but the server reuses the KV cache by longest matching prefix, so a changed middle means re-prefilling every token after it on a 27B model, which is seconds per turn.

**Nerd Genie.** Keep the order rules, persona, tools, job summary, Goal, Rules, then messages appended in order, then the Work and Lessons sections as the tail recitation, then the memory hint. The cost line already planned should print the cache hit rate every turn so this can be measured, not assumed.

**Risk.** A model may over-weight the tail and under-weight the last tool result; the forty-step fixture on Qwen decides. Dropping the oldest half of the messages still breaks the cache once, which is fine.

**Source.** Manus (July 2025); Hermes context-compression docs; Cursor's note that a model switch "means a cache miss."

### 2. Give the small model an edit tool that forgives it

**What.** Aider's evidence is that weaker models fail at exact-match diff formats and do far better when the format is familiar, simple, high level, and applied flexibly. Make Nerd Genie's `edit` tolerant: match ignoring leading whitespace and trailing spaces, accept a hunk that is off by a line, apply relative indentation, and fall back to a whole-file `write` for small files.

**Why.** Aider found that "experiments where flexible patching is disabled show a 9X increase in editing errors," and that its polyglot leaderboard shows Qwen2.5-Coder-32B producing malformed edits in 148 of 225 cases in diff format (71.6% correct format) but 99.6% correct format in whole-file mode. Cursor tunes the edit format per model because "OpenAI's models are trained to edit files using a patch-based format, while Anthropic's models are trained on string replacement." SWE-agent's linter on every edit was worth 3 points on its own.

**Nerd Genie.** `edit` "replaces one exact span" today. Add fuzzy matching with a confidence threshold, run a syntax check after every edit (gofmt, `python -m py_compile`, `node --check`) and refuse the edit if the file no longer parses, and make the refusal say what to do. Measure the malformed-edit rate per model in the fixture.

**Risk.** Fuzzy matching can edit the wrong place; keep the threshold high and show the applied diff in the result.

**Source.** Aider unified diffs (Dec 2023), Aider benchmarks (July 2023), Aider leaderboards; Cursor (Apr 2026); SWE-agent paper.

### 3. Use the local server's grammar to make a bad tool call impossible, and native tool calling instead of text parsing

**What.** llama-server can constrain the model's output to a grammar or JSON schema, and it has native tool-call handlers for the Qwen family. Send the tool schemas natively (with `--jinja` on the daemon side, which is already how the OpenAI-compatible endpoint works) and, on the local provider, pass a schema for the tool-call shape so the model cannot emit a malformed call or a tool name that does not exist.

**Why.** Manus masks tool choices at decode time rather than editing the tool list, because changing the tool list breaks the cache. Krafton's Terminus-KIRA replaced in-context JSON parsing with native tool calling and reports "a significantly shorter prompt and more reliable outputs." The llama.cpp docs warn that the generic fallback "may consume more tokens and be less efficient than a model's native format." The local-model guide's first two failure modes are hallucinated tool names and wrong argument types.

**Nerd Genie.** Keep the repair layer as the fallback for cloud models and text-only providers. On the local provider, prefer schema-constrained decoding and log how often the repair layer still fires; the number should go to zero.

**Risk.** Constrained decoding interacts with the model's thinking mode; test that reasoning quality does not drop. It is a per-request parameter, so the daemon is not touched.

**Source.** llama.cpp function-calling docs; Terminus-KIRA repository; Manus; InsiderLLM local function-calling guide (Feb 2026, updated July 2026).

### 4. Take a snapshot of the environment before the first model call

**What.** At task start, the harness runs one compound command (working directory, file listing, git status, languages and package managers present, memory and disk) and writes the answer into the record's Situation, with no model call.

**Why.** Meta-Harness, a system that searched over harness designs, found this one change beat the best hand-built Terminal-Bench harness (76.4% versus 74.7% on Opus 4.6, and +2.1 points on the much weaker Haiku 4.5): "before the agent loop begins, the harness runs a compound shell command to gather a snapshot of the sandbox environment and injects it into the initial prompt," with the largest gains "when the environment is non-obvious." Anthropic's long-running harness makes every session start with a fixed startup sequence: read the progress file, run the dev server, "verify basic functionality works," then pick the next feature.

**Nerd Genie.** The Situation section is already harness-written; add this snapshot at task creation and, for a task inside a job, re-run the job's last passing check so the task never builds on a broken base.

**Risk.** A few hundred tokens per task. Cap the listing depth.

**Source.** Meta-Harness (Mar 2026); Anthropic, Effective harnesses for long-running agents (Nov 2025).

### 5. Let the harness run the check, freeze the done list, and demand evidence

**What.** When a done line names a command, test, file, or URL, the harness runs or checks it itself at done-check time. A done line, once written, cannot be deleted or reworded by the model. The final report must carry the evidence (test output, command and result, page text) rather than the model's claim.

**Why.** Anthropic's long-running harness keeps a feature list that starts "failing" and tells the model "it is unacceptable to remove or edit tests," because the observed failure was "declaring victory prematurely." Claude Code's docs say "Claude stops when the work looks done. Without a check it can run, 'looks done' is the only signal," and add a Stop hook that blocks the turn from ending until a script passes. Google's Antigravity docs call a local verification mechanism "the single most effective way to ensure reliable, correct modifications." Krafton adds a last "double-confirmation checklist" from the view of a test engineer, a QA engineer, and a user before the agent may say done.

**Nerd Genie.** The done-check exists; add the immutability rule to the `task` tool, the harness-run checks, and one extra tools-off call at the end that reviews the done list from the tester's and user's point of view.

**Risk.** One more model call per task; skip it for tasks under five rounds.

**Source.** Anthropic long-running harnesses; Claude Code best practices; Antigravity CLI best practices; Terminus-KIRA.

### 6. Keep the local window small on purpose, and never tell the model the window is nearly full

**What.** The llama-server holds 262,144 tokens, but Nerd Genie should size the results table to roughly 16k to 32k tokens on the 27B model and measure quality against size with the fixture, rather than filling what the daemon allows. The budget-left header should only appear when a budget is set, and never as a fraction of the window.

**Why.** Chroma tested 18 models including Qwen3 and found "significantly higher performance on focused prompts compared to full prompts" (about 300 tokens versus 113k) and that "even a single distractor reduces performance." SWE-agent got 18.0% keeping the last five observations in full and collapsing the rest, versus 15.0% with full history. Cognition found "context anxiety": Sonnet 4.5 took "shortcuts or leaving tasks incomplete when it believed it was near the end of its window, even when it had plenty of room left," and fixed it by enabling a 1M window but capping use at 200k. The local guide's sweet spot for a 27B Q4 model on 24GB is "~16K-32K."

**Nerd Genie.** The one-rule window already fits the model; this makes the small-model number a measured choice. The header's "budget left" line should stay out of the prompt unless the user set a budget.

**Risk.** More results leave the table sooner; `read r7` covers that by design.

**Source.** Chroma context rot (July 2025); SWE-agent; Cognition, Rebuilding Devin for Sonnet 4.5 (Sept 2025); InsiderLLM guide.

### 7. Copy the measured interface numbers: 100-line reads, capped search that says how to narrow, and truncation that points somewhere

**What.** Default `read` to a 100-line window with the line range shown; make `search` cap its hits and reply "too many results, narrow the pattern" instead of listing them; and make every truncated result end with the file that holds the rest and a hint on how to ask for less.

**Why.** SWE-agent measured these: 100-line windows beat both 30 lines and whole files (18.0% versus 14.3% and 12.7%); a summarizing search beat a page-through search by six points because agents "look through every match exhaustively." Anthropic's tool-writing post says Claude Code caps tool results at 25,000 tokens and that truncation should carry "helpful guidance" toward a narrower call. Krafton caps terminal output at 30KB.

**Nerd Genie.** The output-to-file rule exists; add the read window, the search cap, and the wording of the truncation line. Cheap and testable.

**Risk.** None worth naming.

**Source.** SWE-agent paper (2024); Anthropic, Writing tools for agents (Sept 2025); Terminus-KIRA.

### 8. Keep errors in the record word for word, and count them per tool per model

**What.** When a tool fails, the one-line result in the record should carry the first line of the error verbatim, and errors should stay on the table longer than successes. Separately, the harness should classify every tool failure (bad arguments, unexpected environment, provider error, user abort, timeout) and count them per tool and per model.

**Why.** Manus: "one of the most effective ways to improve agent behavior is deceptively simple: leave the wrong turns in the context." Cursor built exactly this error taxonomy, alerts on spikes "per-tool and per-model," and "drove all tool calls to at least 2 or often 3 9s of reliability." Refact.ai found the model "often skipped some tools as their names were unclear" and only saw it by looking.

**Nerd Genie.** Failures with causes already exist; make the cause text the real error line, not a paraphrase, and add the counter to `/status`.

**Risk.** Low.

**Source.** Manus; Cursor, Continually improving our agent harness (Apr 2026); Refact.ai (May 2025).

### 9. Treat a job as a Ralph loop: fresh window per task, lessons inherited, one thing at a time

**What.** Geoffrey Huntley's Ralph technique is a bash loop that reruns the agent with the same prompt, a fresh context each time, state in files and git, and "only one thing" per run. Nerd Genie's jobs already do this; the missing piece is that a job's Failures and Decisions should be copied into each new task's record automatically, and each task should start clean.

**Why.** Anthropic's long-running harness and Cursor's long-running agents converged on the same shape: a plan file as shared state, workers that "focus entirely on execution without coordinating," one feature at a time, and a clean state at the end of each session. Cognition's 18-month review found the agent "usually performs worse when you keep telling it more after it starts," which argues for finishing a task and starting the next one with a clean window rather than steering one long session.

**Nerd Genie.** Add lesson inheritance from job to task and make it a fixture assertion.

**Risk.** Inherited lessons can grow; cap them at the record's size rule.

**Source.** Huntley, Ralph (July 2025); Anthropic long-running harnesses; Cursor long-running agents (Feb 2026); Cognition performance review (Nov 2025).

### 10. A read-only summarizing sub-call for big results, never a parallel worker

**What.** When the model needs a result too big for the table, the harness can run one separate call with a fresh window that reads the full result and returns a one- to two-thousand-token digest, saved as its own result id. The full text stays in the log.

**Why.** Anthropic's context-engineering post describes sub-agents that "explore extensively" but return "only a condensed, distilled summary (often 1,000-2,000 tokens)." Cognition's rule is to keep one thread of decisions and, when compression is needed, use "a new LLM model whose key purpose is to compress a history of actions." Claude Code's docs recommend sub-agents so exploration "doesn't consume your main context." Nothing here needs parallel workers, which Cognition warns produce conflicting decisions.

**Nerd Genie.** Expose it as `read r7 digest`. The digest is a result, so it is cited like any other, and the done-check can still demand the original.

**Risk.** A digest can drop the one line that mattered; the model can always read the full result.

**Source.** Anthropic context engineering (Sept 2025); Cognition (June 2025); Claude Code docs.

### 11. A freeze mode, and results the model cannot invent

**What.** Add `/freeze`: every write, shell, browser action, and job creation goes to ask-me-first until `/unfreeze`. Keep the rule that every result is built by the harness.

**Why.** Replit's agent deleted a production database during a declared code freeze, produced fake records, and said a rollback was impossible when it was not. The CEO called it "unacceptable and should never be possible" and shipped dev/prod separation, a planning-only mode, and one-click restore. The lesson is that a freeze stated in words is not a freeze; only the harness can hold one.

**Nerd Genie.** One command and one permission rule; the sandbox and the vault already do the rest.

**Risk.** None; it only adds prompts.

**Source.** Fortune (July 23, 2025); Amjad Masad on X (July 2025).

### 12. A twenty-task evaluation set and a keep rate

**What.** Besides the forty-step fixture, keep about twenty real tasks with a pass/fail check each, run them on every model, and track how much of the agent's output the user keeps.

**Why.** Anthropic's research-system team started with about 20 queries and found that was enough to see the biggest problems. Cursor measures "what fraction of those remain in the user's codebase after fixed intervals of time." The harness-evolution paper improved Terminal-Bench from 69.7% to 77.0% in ten iterations purely by reading traces and changing tools and middleware, "rather than the system prompt."

**Nerd Genie.** The failed-task-becomes-a-test rule is the seed; grow it into the set.

**Risk.** Local runs are slow; run nightly.

**Source.** Anthropic multi-agent research (June 2025); Cursor (Apr 2026); Agentic Harness Engineering (Apr 2026).

## 2. Findings by source

### Manus, "Context Engineering for AI Agents" (July 18, 2025)
Rules: KV-cache hit rate is the key metric (10x price gap, 100:1 input to output, about 50 tool calls per task); keep the prefix stable and the context append-only with deterministic serialization; mask tools rather than remove them; use the file system as "unlimited in size, persistent by nature" context with compression that is "always designed to be restorable"; recite goals by rewriting a todo file at the end of context; keep failed actions in context; add "structured variation" so the model does not fall into a rut. For Nerd Genie: ideas 1, 3, and 8; the record's fixed shape is a rut risk on a small model, so the repeated-call detector matters.

### Anthropic, "Effective context engineering for AI agents" (Sept 29, 2025)
Rules: context is "a finite resource with diminishing marginal returns" (context rot); write the system prompt at the "right altitude," neither brittle rules nor vague advice; keep tools few and unambiguous ("if a human engineer can't definitively say which tool should be used ... an AI agent can't be expected to do better"); prefer just-in-time retrieval by identifiers over pre-loading; for long tasks use compaction, structured notes (NOTES.md, the Pokémon tallies), or sub-agents that return 1,000 to 2,000 token summaries. For Nerd Genie: the record is the structured note; `read r7` is just-in-time retrieval; idea 10.

### Anthropic, "Effective harnesses for long-running agents" (Nov 26, 2025)
Rules: an initializer agent sets up `init.sh`, a progress file, and a first commit; a feature list starts "failing" and may not be edited; one feature per session; leave the tree mergeable; a fixed startup sequence; the agent must test "end-to-end like human users would." Failure modes named: declaring victory early, undocumented progress, half-done features. For Nerd Genie: ideas 4, 5, 9.

### Anthropic, "How we built our multi-agent research system" (June 13, 2025)
Rules: token use explained about 80% of the score difference; a lead with parallel sub-agents beat a single agent by 90.2% on research but used about 15x the tokens; give sub-agents an objective, an output format, tool guidance, and boundaries; "bad tool descriptions can send agents down completely wrong paths"; resume from where an agent failed rather than restarting; start evaluation with about 20 queries. For Nerd Genie: the 15x cost is why single-thread is right for a local model; idea 12.

### Anthropic, "Writing tools for agents" (Sept 11, 2025) and "Building agents with the Claude Agent SDK" (Sept 29, 2025)
Rules: consolidate tools rather than one per API; namespace them; return natural-language names over cryptic ids; offer concise versus detailed responses (the concise form used about a third of the tokens); paginate and truncate at sensible defaults (25,000 tokens in Claude Code) with guidance in the truncation; refine descriptions by evaluation. The SDK post frames the loop as "gather context, take action, verify work," prefers grep-style "agentic search" over embeddings, and lists three verification kinds: rules-based (linters), visual (screenshots), and LLM-as-judge, which is "generally not a very robust method." For Nerd Genie: idea 7; the text-only local model cannot do visual checks, so browser verification must stay in the page-tree text.

### Claude Code docs: best practices, how it works, dynamic workflows (2026)
Rules: "the context window is the most important resource to manage"; give a check that returns pass or fail, then gate the stop on it (`/goal`, a Stop hook that blocks until a script passes, a fresh-context reviewer); "show evidence rather than asserting success"; after two failed corrections, clear and rewrite the prompt; CLAUDE.md must be short because "bloated CLAUDE.md files cause Claude to ignore your actual instructions"; on a full window it "clears older tool outputs first, then summarizes"; dynamic workflows move the loop into a script so "Claude's context holds only the final answer." For Nerd Genie: ideas 5 and 10; the persona files should be pruned as ruthlessly as CLAUDE.md.

### Cognition: "Don't Build Multi-Agents" (June 12, 2025), "Rebuilding Devin for Sonnet 4.5" (Sept 29, 2025), "Devin's 2025 Performance Review" (Nov 14, 2025)
Rules: "share context, and share full agent traces"; "actions carry implicit decisions, and conflicting decisions carry bad results"; a single-threaded agent with a dedicated compressor model when needed; context anxiety and the 200k cap; the model's own notes "lacked comprehensiveness," so keep a harness memory system; Devin does best with "clear, upfront requirements and verifiable outcomes" and worse "when you keep telling it more after it starts." For Nerd Genie: ideas 6 and 9; corrections during a task are supported but the design should push big changes into a new task.

### OpenAI: "Harness engineering" (Feb 2026, read through InfoQ and AlphaSignal because the original returns 403), "Unrolling the Codex agent loop" (Jan 23, 2026), "Codex as a platform" (Aug 19, 2026)
Rules reported: about a million lines in five months with no hand-written code; AGENTS.md files per module pointing into a docs directory; architecture enforced by "custom linters, themselves generated by Codex" and structural tests over a fixed dependency order; background agents doing "garbage collection" of drift; agents given logs, metrics, and traces; the summary quote "the agent doesn't need more instructions. It needs a world where the right thing to do is obvious and the wrong thing is hard." The loop post describes replaying the whole history each turn and compacting past a limit into an opaque item. For Nerd Genie: the guard and permission function are the "wrong thing is hard" layer; a scheduled job that prunes stale memory and skills is the garbage collector.

### Cursor: "Best practices" (Jan 9, 2026), "Long-running agents" (Feb 12, 2026), "Continually improving our agent harness" (Apr 30, 2026)
Rules: a harness is instructions, tools, and model, tuned per model; start a new chat when the agent "shows confusion or repeating errors"; rules files should reference files, not copy them; "agents can't fix what they don't know about"; long-running agents plan first, wait for approval, and use workers that grind on one task; later they removed 2024 guardrails as models improved, kept per-model edit formats, built the error taxonomy, and measured keep rate. For Nerd Genie: ideas 2, 8, 12.

### Aider: repo map, edit formats, unified diffs (Dec 2023), code-editing benchmarks (July 2023), leaderboards
Rules: a ~1,000-token map of the most-referenced symbols, ranked by a graph of file dependencies; the whole-file format is "the most reliable" for weak models, which in diff mode put "the entire original source file in the ORIGINAL block"; a format should be familiar, simple, high level, and flexibly applied, and disabling flexibility caused "a 9X increase in editing errors." For Nerd Genie: idea 2; a repo map in the Situation for coding tasks is a cheap follow-on.

### SWE-agent (2024), mini-swe-agent (2025), "Coherence Collapse" (Mar 2026)
Rules: four interface principles (simple actions, compact actions, informative and concise feedback, guardrails); the measured window, search, linter, and history numbers in idea 7; a custom interface beat a bare shell by 64% relative with the same model. mini-swe-agent then showed that with strong models a bash-only, linear-history, subprocess-per-command agent scores over 74% on SWE-bench Verified. Coherence Collapse found 60 to 69% of failures "reach and edit the correct functions yet still produce incorrect patches," with agents destroying a correct patch later, and proposes an edit-commit checkpoint. For Nerd Genie: ideas 2 and 7; the checkpoints plus a failure that names a working state help the model stop thrashing.

### OpenHands paper (ICLR 2025) and condenser blog (Apr 9, 2025)
Rules: an event stream of actions and observations, a sandboxed runtime, and a condenser that keeps the first four events and a recent tail, summarizing the middle when history passes 120 events; on SWE-bench Verified it solved 54% versus 53% without, at "less than half the cost" per turn. For Nerd Genie: confirmation that a bounded working set does not lose accuracy.

### SWE-bench leaderboard write-ups: TRAE (May 19, 2025), Refact.ai (May 15, 2025), Warp (2025)
Rules: TRAE's single run scored 60.6 to 62.6% and only ensembling five or six candidates with a regression-test filter reached 70.6%; Refact renamed tools "as their names were unclear" and made tools tolerant of uncertainty; Warp added a best-of-k wrapper "to smooth over non-determinism." For Nerd Genie: ensembling is out of reach on one GPU, so the single-run rules (clear tool names, tolerant tools, run the project's tests) are the part that transfers.

### Terminal-Bench harnesses: Terminus-2, Terminus-KIRA (2026), Meta-Harness (Mar 2026), Agentic Harness Engineering (Apr 2026), Tmax (June 2026), harness survey (June 2026)
Rules: Terminus-2 is one tmux tool with a three-way fallback when context overflows, down to "system prompt + task + current state only"; KIRA added native tool calling, a 30KB output cap, marker-based polling, and a completion checklist; Meta-Harness found the environment snapshot; harness evolution gained 7.3 points and 12% fewer tokens from tools, middleware, and memory; Tmax trained a 9B model to 27% on Terminal-Bench 2.0, evidence that small models can drive terminals; the survey names six harness jobs: observation, context, control, action, state, verification. For Nerd Genie: ideas 3, 4, 5, 7.

### Replit incident (July 2025)
Rules learned the hard way: separate what the agent can touch from what matters; a freeze must be enforced by the system; keep backups the agent cannot reach; the agent "didn't have access to the proper internal docs" and invented data and a false "rollback impossible." For Nerd Genie: idea 11 and the existing rule that the harness builds every result.

### Google: Gemini CLI hooks (Jan 28, 2026), Antigravity CLI best practices (2026)
Rules: hooks before tools and after the agent to inject context and block dangerous commands, "keep hooks fast"; explore, plan, then execute; local verification is the single most effective practice; GEMINI.md or AGENTS.md read at startup. For Nerd Genie: the guard already is the hook; nothing new beyond idea 5.

### Huntley's Ralph loop (July 14, 2025)
Rules: `while :; do cat PROMPT.md | claude-code ; done`; one thing per loop; specs, a priority-sorted plan file, an AGENT.md, and "signs" that stop repeated mistakes; git as the save file. For Nerd Genie: idea 9.

### Hermes (Nous Research) and OpenClaw compaction docs (2026), Claw-SWE-Bench (June 2026)
Rules: Hermes compresses at 50% with an 85% safety net, prunes old tool results to a placeholder for free, keeps the first three and last twenty messages, and summarizes into a fixed template (Goal, Constraints, Progress, Key Decisions, Relevant Files, Next Steps, Critical Context) that is updated, not rewritten; OpenClaw runs a silent "memory flush" turn before compacting and keeps 20,000 recent tokens. Claw-SWE-Bench showed the same model scoring 19.1% with a minimal OpenClaw adapter and 73.4% with a full one. For Nerd Genie: the template is the record's shape, which is a strong sign the design is right; the harness matters as much as the model even on the same weights.

### Running agents on small local models: llama.cpp docs, InsiderLLM guide (Feb to July 2026), Qwen-Agent, Unsloth Qwen3-Coder guide, Hermes function-calling format
Rules: use the model's native tool format (Qwen-Agent defaults to the Hermes-style `<tool_call>` tags for Qwen3); llama.cpp can constrain output with a grammar and warns that "extreme KV quantizations ... can substantially degrade the model's tool calling performance"; validate tool names against the registry, validate argument types, cap iterations, and keep tools few because "ten tools can consume 1,000+ tokens"; multi-step accuracy "degrades beyond 2-3 tool calls on smaller models"; the 27B sweet spot is 16K to 32K of context; Unsloth's tool-calling template fix shows chat templates are a common cause of broken calls. For Nerd Genie: ideas 3 and 6; the eighteen-tool list is at the edge for a 27B model, so measure the tool-selection error rate and consider hiding the desktop and job tools until a task needs them.

### Chroma, "Context Rot" (July 14, 2025)
Rules: across 18 models, performance falls as input grows "even on simple tasks"; distractors compound; a focused prompt beats the full one for every model. For Nerd Genie: idea 6 and the reason the record exists.

## 3. Sources

- Manus, Context Engineering for AI Agents (Yichao Ji, July 18, 2025): https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- Anthropic, Effective context engineering (Sept 29, 2025): https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- Anthropic, Effective harnesses for long-running agents (Nov 26, 2025): https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents
- Anthropic, Multi-agent research system (June 13, 2025): https://www.anthropic.com/engineering/multi-agent-research-system
- Anthropic, Writing tools for agents (Sept 11, 2025): https://www.anthropic.com/engineering/writing-tools-for-agents
- Anthropic, Building agents with the Claude Agent SDK (Sept 29, 2025): https://claude.com/blog/building-agents-with-the-claude-agent-sdk
- Anthropic, Building effective agents (Dec 19, 2024): https://www.anthropic.com/engineering/building-effective-agents
- Claude Code docs: https://code.claude.com/docs/en/best-practices , https://code.claude.com/docs/en/how-claude-code-works , https://code.claude.com/docs/en/workflows
- Cognition, Don't Build Multi-Agents (June 12, 2025): https://cognition.com/blog/dont-build-multi-agents
- Cognition, Rebuilding Devin for Sonnet 4.5 (Sept 29, 2025): https://cognition.com/blog/devin-sonnet-4-5-lessons-and-challenges
- Cognition, Devin's 2025 Performance Review (Nov 14, 2025): https://cognition.com/blog/devin-annual-performance-review-2025
- OpenAI, Harness engineering (Feb 2026, blocked to robots): https://openai.com/index/harness-engineering/ ; read via InfoQ https://www.infoq.com/news/2026/02/openai-harness-engineering-codex/ and AlphaSignal https://alphasignalai.substack.com/p/a-closer-look-at-harness-engineering
- OpenAI, Unrolling the Codex agent loop (Michael Bolin, Jan 23, 2026): https://openai.com/index/unrolling-the-codex-agent-loop/
- OpenAI, Codex as a platform (Aug 19, 2026): https://developers.openai.com/blog/codex-as-a-platform
- Cursor, Best practices for coding with agents (Jan 9, 2026): https://cursor.com/blog/agent-best-practices
- Cursor, Long-running agents (Feb 12, 2026): https://cursor.com/blog/long-running-agents
- Cursor, Continually improving our agent harness (Apr 30, 2026): https://cursor.com/blog/continually-improving-agent-harness
- Aider: https://aider.chat/docs/repomap.html , https://aider.chat/docs/more/edit-formats.html , https://aider.chat/2023/12/21/unified-diffs.html , https://aider.chat/2023/07/02/benchmarks.html , https://aider.chat/docs/leaderboards/
- SWE-agent paper (Yang et al., 2024): https://arxiv.org/abs/2405.15793
- mini-swe-agent: https://github.com/SWE-agent/mini-swe-agent
- Coherence Collapse (Kim et al., Mar 2026): https://arxiv.org/abs/2603.24631
- OpenHands paper (ICLR 2025): https://arxiv.org/abs/2407.16741 ; condenser blog (Apr 9, 2025): https://www.openhands.dev/blog/openhands-context-condensensation-for-more-efficient-ai-agents ; https://docs.openhands.dev/sdk/arch/condenser
- TRAE on SWE-bench Verified (May 19, 2025): https://se-research.bytedance.com/blogs/trae-on-swe-bench-verified-71
- Refact.ai (May 15, 2025): https://refact.ai/blog/2025/open-source-sota-on-swe-bench-verified-refact-ai/
- Warp: https://www.warp.dev/blog/swe-bench-verified-update
- Terminus-2: https://www.harborframework.com/docs/agents/terminus-2 ; Terminus-KIRA: https://github.com/krafton-ai/KIRA
- Meta-Harness (Lee et al., Mar 30, 2026): https://arxiv.org/abs/2603.28052
- Agentic Harness Engineering (Lin et al., Apr 28, 2026): https://arxiv.org/abs/2604.25850
- Tmax (Ivison et al., June 22, 2026): https://arxiv.org/abs/2606.23321
- Survey on agent system and harness design (Guo et al., June 14, 2026): https://arxiv.org/abs/2606.20683
- Claw-SWE-Bench (Zheng et al., June 10, 2026): https://arxiv.org/abs/2606.12344
- Replit incident: Fortune (July 23, 2025) https://fortune.com/2025/07/23/ai-coding-tool-replit-wiped-database-called-it-a-catastrophic-failure/ ; Amjad Masad on X https://x.com/amasad/status/1946986468586721478
- Google, Gemini CLI hooks (Jan 28, 2026): https://developers.googleblog.com/tailor-gemini-cli-to-your-workflow-with-hooks/ ; Antigravity CLI best practices: https://antigravity.google/docs/cli/best-practices/
- Huntley, Ralph (July 14, 2025): https://ghuntley.com/ralph/
- Hermes Agent context compression and caching: https://hermes-agent.nousresearch.com/docs/developer-guide/context-compression-and-caching/ ; architecture: https://hermes-agent.nousresearch.com/docs/developer-guide/architecture
- OpenClaw compaction: https://docs.openclaw.ai/concepts/compaction
- llama.cpp function calling: https://github.com/ggml-org/llama.cpp/blob/master/docs/function-calling.md
- InsiderLLM, Function calling for local LLMs (Feb 11, 2026, updated July 15, 2026): https://insiderllm.com/guides/function-calling-local-llms/
- Qwen-Agent: https://github.com/QwenLM/Qwen-Agent ; Unsloth Qwen3-Coder guide: https://unsloth.ai/docs/models/tutorials/qwen3-coder-how-to-run-locally ; Hermes function-calling format: https://github.com/NousResearch/Hermes-Function-Calling
- Chroma, Context Rot (July 14, 2025): https://www.trychroma.com/research/context-rot
