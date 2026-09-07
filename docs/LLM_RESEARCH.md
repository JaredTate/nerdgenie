# What the research says, and what Nerd Genie should do

Written 2026-09-05. This is a plain-English reading of three reports in `docs/research/2026-09-05-state-and-llm/`: `llm-efficiency.md` on how models spend tokens and time, `harness-builders.md` on what the people who build agent harnesses have published, and `agent-harnesses.md` on what the research says about keeping an agent reliable. Every number and source comes from those reports, with dates.

A few words, used throughout. A **model** is a program that turns text into text. A **harness** is the program around the model that gives it memory and hands. A **token** is a piece of text about the size of a short word. The **prompt** is everything sent to the model in one call. A **tool call** is the model asking the harness to do something, such as read a file. The **record** is Nerd Genie's short task document, and the **log** is its history of everything that happened.

## 1. The summary, in five lines

1. Every call has two costs. **Prefill** is the model reading the prompt, at about 400 tokens a second on our card. **Generation** is the model writing its answer, at about 55 tokens a second. So one written token costs as much time as about seven read ones.
2. An agent loop makes many calls per task, about fifty for a typical job, and each call re-sends the prompt. Prompt size times call count is the token bill. Written tokens are most of the clock.
3. The one metric that matters is the **cache hit rate**. When the model reads a prompt it keeps a scratch copy of its work, called the **KV cache** (key-value cache). If the next prompt begins with exactly the same tokens, the server reuses that copy and reads only the new part. The hit rate is the share of the prompt it did not have to read again.
4. So, if you send the same prompt twice, is the second one cheaper? Yes. On our local server the second read costs only the part that changed, as long as the front of the prompt is identical to the byte and, for this particular model, a saved checkpoint sits where the two prompts part ways. On cloud models a cached token costs about a tenth of a fresh one.
5. Everything else follows from that. Keep the front of the prompt frozen. Keep the whole prompt small. Write as little as possible. Never let the model waste calls on loops or on work it cannot prove.

## 2. The ideas, ranked by impact

The evidence is in section 3.

### Idea 1. Freeze the front of the prompt, and put everything that changes at the very end

**What it is.** A prompt cache helps only up to the first byte that differs from the last call. So build the prompt in layers: what never changes first, what only grows next, what changes every turn last. The end is also where the model attends best, so the "next step" line belongs there.

**Where it comes from.** Manus (July 2025): keep the prefix stable, make the context append-only, recite the goal at the end. And a twist: Qwen 3.8 27B is a **hybrid model**: 48 of its 64 layers keep one running state instead of a per-token cache, so the server cannot rewind to any point. It resumes only from a **checkpoint**, a snapshot saved before the latest user message and every 8,192 tokens (llama.cpp, May 2026). A change above that checkpoint forces a read from token zero.

**What Nerd Genie would do.** The frozen parts already come first and the done list sits in the tail. Make the rest strict: recent messages appended and never touched; Work, Lessons, the memory hint, the cost line, and one recitation line ("step 3 of 5, next: confirm the compose box") last. Lower the checkpoint spacing toward a typical turn, and fail a `make live` run on more than one full re-read per task.

**The risk.** This leans on llama.cpp behaviour that changed twice in 2026 and broke once; measure it. The model may over-weight the tail; the fixture on Qwen decides.

### Idea 2. Measure the cache hit rate every turn, and fail the test when it drops

**What it is.** The server reports on every reply how many prompt tokens it reused and how many it read fresh (`cache_n` and `prompt_n` on llama.cpp; similar fields on Anthropic and OpenAI). The cost line can be a measured fact, and a test can assert it.

**Where it comes from.** Manus names the cache killers: a timestamp at the top, JSON keys in a different order (JSON is a way of writing data in text with braces and quotes), a tool list that changes mid-task. A June 2026 post traced months of full re-reads on llama.cpp to a small header at the start of Claude Code's system prompt.

**What Nerd Genie would do.** The header's "5.2k of them cached" becomes the server's own number. The forty-step fixture asserts that from turn three on, at least 80 percent of input tokens are cache reads. Tool definitions stay byte-identical and sorted. The budget line appears only when the user set one.

**The risk.** Almost none. It is measurement.

### Idea 3. Never believe a tool. Check the world.

**What it is.** A tool that changes something (a click, a file write, a command) should return proof of the change, not the word "ok." The harness compares the world before and after: the page tree, a fingerprint of the file, the test count.

**Where it comes from.** Tonight's Tetris failure: the browser said a click worked, it had not, and the model had no true signal. A June 2026 study found such "false success" was 45 to 76 percent of failures across two benchmarks, and model judges spotted it barely better than a coin flip.

**What Nerd Genie would do.** `browser_act` already does one action and checks it. Make that the rule for every tool that changes the world: the harness writes a computed difference on the result line ("page unchanged," "0 files changed," "tests 62/62 to 62/62"), a click with no visible change returns a failure and a screenshot, and a done line whose only proof is a tool's own claim stays unproven.

**The risk.** More bytes per result; the browser worker already snapshots the page.

### Idea 4. Measure progress, not only repeats, and climb a ladder: nudge, rewind, stop

**What it is.** The repeat guard catches the same call twice, but misses reads at different offsets or a read-test-read-test pattern. Progress is something the harness can count for free: a done line proven, a plan step marked, a file changed, a test count changed.

**Where it comes from.** Models "become more likely to make mistakes when the context contains their errors from prior turns," and this **self-conditioning** "is not mitigated by scaling model size" (ICLR 2026). Rolling back and re-running recovered 45 percent of failed runs against 16 percent for plain retries (Aug 2026).

**What Nerd Genie would do.** A harness-written "rounds since last progress" line in the Situation. At N rounds with none, one nudge, worded differently each time. At 2N, rewind: drop the looping results from the table, write one failure line with its cause, and call again with the clean context. At 3N, stop and ask. A git commit (a saved snapshot of the files) after every green test run, to go back to after a thrash.

**The risk.** A rewind is itself a loop; allow one per stuck event. A polled long command looks like no progress; exempt those.

### Idea 5. Done means proof a user would accept

**What it is.** A **unit test** checks one small piece of code; green unit tests prove their own lines, not that the game plays. A done line about something the user sees needs an **end-to-end check**: the harness drives the real page and asserts a difference.

**Where it comes from.** Tetris had 62 passing tests and was not playable. Strengthening the tests on SWE-bench (a set of real bug fixes from GitHub) dropped the top agent from 79 to 62 percent (Feb 2026). Anthropic's long-running harness (Nov 2025) forbids editing its feature list and requires browser-driven checks.

**What Nerd Genie would do.** The done-check with proof ids exists. Add three things: the `task` tool refuses to delete or reword a done line; a line about user-visible behaviour needs a `browser_act` result with an asserted difference, never a unit-test id; and one last call, tools off, reviews the done list as a tester and a user would.

**The risk.** End-to-end checks are slow and flaky; cap retries and fall back to a hand-off screenshot.

### Idea 6. Keep the working context small on purpose, about 32,000 tokens

**What it is.** The **context window** is the most text a model can take in one call. The daemon holds 262,144 tokens, but a 27-billion-parameter model reasons well over far less. Size what Nerd Genie sends by what the model handles, not by what the server allows.

**Where it comes from.** Chroma's "Context Rot" (July 2025): 18 models, every one worse as input grows. LOCA-bench (Feb 2026): open models fell from 79 percent at 8,000 tokens to 61 at 32,000, 45 at 64,000, and 11 at 128,000. The benchmark agrees: Nerd Genie's biggest prompt was 34,520 tokens and it finished green; the others reached 105,000 to 230,000 and looped for hours.

**What Nerd Genie would do.** Start the daemon at 65,536 tokens, not 262,144, and give the freed video memory to checkpoints and the drafting head (idea 9). Hold the working context at or under about 32,000 on the 27B. Never tell the model how full the window is.

**The risk.** Results leave the table sooner. That is what `read r7` is for.

### Idea 7. Short, honest tool results: fixed caps, head and tail, a pointer to the rest, and the real error text

**What it is.** Every tool result is cut to a fixed size: the first and last N lines, a line saying where the full text is, and a hint on how to ask for less. A failure line in the record carries the first line of the real error, word for word.

**Where it comes from.** SWE-agent (2024) measured it: 100-line file windows beat 30 lines and whole files, and a search that says "too many results, narrow the pattern" beat one that lists them. Cursor counted tool errors per tool and per model and drove calls to "two or often three nines" of reliability.

**What Nerd Genie would do.** The output-to-file rule exists. Make the cap deterministic, default `read` to a 100-line window that shows its range, cap `search` hits with the narrowing hint, end every cut with the file that holds the rest, make a failure's cause the real error line, and count failures per tool and per model in `/status`.

**The risk.** A cut in the wrong place hides the one line that mattered; the pointer to the full text is the safety net.

### Idea 8. Shelve a result with its meaning: a gist, a time stamp, and search and slice by id

**What it is.** A result line today says where a thing is, not what it said. When a result leaves the table, the model writes a gist of at most 25 words and the harness stamps the round and clock time. And a 12,000-token page should never sit whole in the window: the model searches and slices it by id, or asks for a digest.

**Where it comes from.** "Context Length Alone Hurts" (EMNLP 2025): models lose 14 to 85 percent even when they can find the right text, and reciting the relevant evidence into a short space is the fix. Recursive Language Models (Dec 2025) let the model search and slice a long input as a variable.

**What Nerd Genie would do.** The `task` tool asks for a `gist` as a result goes to the shelf; the harness appends round and time to every line. Extend `read` to take a line range and `search` to take a result id. Add `read r7 digest`: one read-only call whose digest becomes a new result id.

**The risk.** About 30 output tokens per gist, more tool rounds, and a small model may gist wrongly; `read r7` stays the truth.

### Idea 9. Spend the slow side wisely: thinking at three moments, no echoes, and the drafting trick

**What it is.** Output is the slow side: every token written costs as much time as seven read. **Thinking mode** is a private reasoning pass before the answer; Qwen 3.x switches it per request, and a 1,500-token think is 27 seconds. Echoes (re-quoting a file, writing a whole file to change one line) are pure waste. **Speculative decoding** has a cheap guesser draft several tokens for the big model to check in one pass; the output is identical, only faster.

**Where it comes from.** The benchmark ran with thinking off and Nerd Genie won, but the self-conditioning paper found "thinking mitigates self-conditioning," while "The Danger of Overthinking" (Feb 2025) found agents that reason more act less. llama.cpp's **MTP** (multi-token prediction, a draft head built into the model, May 2026) writes about 1.9 times faster on Qwen3.6-27B, and a model-free drafter guesses from words already in the prompt.

**What Nerd Genie would do.** Thinking off by default, and on with a budget of a few hundred tokens at three moments: the plan and done list, the first call after a recorded failure, and the done-check. Forbid re-quoting files and prefer small edits. MTP is already on; try the prompt-lookup drafter beside it and read the acceptance rate.

**The risk.** The thinking switch is rendered into the **chat template** (the fixed wrapper around a prompt), so it must sit in the tail, below the cache line. Drafting acceptance falls as the randomness setting rises; measure it.

### Idea 10. Forgiving tools for a small model: reason first, constrain only the call, a fuzzy edit with a syntax check, fewer tools by phase

**What it is.** Asking a small model to write strict JSON hurts its reasoning; letting it think in plain text first, then write the call, gets most of that back. **Constrained decoding** is a grammar that makes the model unable to emit a token that breaks the format; it should cover only the tool-call block. Small models also get exact-match edits wrong, so `edit` should match loosely and check that the file still parses. And fewer tools in view means better picks.

**Where it comes from.** "The Format Tax" and "Capacity, Not Format" (2026) found format demands cost small models up to 36 points, and "Constraint Tax" found a whole-reply JSON schema makes open models stop calling tools. Aider found disabling flexible patching gave "a 9X increase in editing errors."

**What Nerd Genie would do.** Keep "first line says where the work stands, then the tool calls," and never set a whole-reply response format with tools. Try a grammar on the tool-call span only, with the repair layer as fallback. Give `edit` fuzzy matching with a high threshold and run `gofmt`, `node --check`, or `python -m py_compile` after it. Keep all eighteen definitions in the cached front, but mask by phase (no browser tools until the plan names a page), always with an "ask for tool X" path.

**The risk.** A grammar can hide the model's confusion, fuzzy matching can edit the wrong place, and a wrong mask blocks a needed tool.

### Idea 11. Start each task oriented: a snapshot of the world, the job's lessons, a fresh window, and narrow workers

**What it is.** Before the first model call of a task, the harness runs one compound command (files, git status, languages, test runner) and writes the answer into the Situation. A task inside a job inherits the job's decisions and failures and starts with a clean window. A well-defined step can run as a worker: one call holding only the goal, that step, and the result it needs.

**Where it comes from.** Meta-Harness (Mar 2026) found the snapshot alone beat the best hand-built Terminal-Bench harness (a set of terminal tasks). Huntley's Ralph loop restarts the agent with a fresh context on one task at a time. Cognition's 2026 review: "one main loop carries state, with subagents as stateless workers with narrow scope."

**What Nerd Genie would do.** Jobs already give each task its own record. Add the snapshot at task creation, re-run the job's last passing check so a task never builds on a broken base, copy the job's Failures and Decisions into the new task's record, and add worker calls with no record access whose text becomes a result id.

**The risk.** A few hundred tokens per task. Planner-worker splits hurt on easy tasks, and a worker may lack context it needed.

### Idea 12. Learn across tasks without losing the original: verbatim-backed memory, a traps list, and a twenty-task test set

**What it is.** Summaries in memory lose; each fact should point at the raw log event it came from. A "trap" is a lesson from a failure, stored by tool and site and dropped if a later task proves it wrong, such as "a click on a canvas game reports success with no page change; check by screenshot." And keep about twenty real tasks with a pass or fail check each.

**Where it comes from.** "Verbatim Chunks Beat Extracted Artifacts" (Dec 2025): raw chunks beat model-extracted facts by 16 to 22 points. ReasoningBank (ICLR 2026) stored lessons from failures and gained 8 percent on WebArena. Anthropic's research team found about 20 test queries enough to see the biggest problems; Cursor tracks how much of the agent's output the user keeps.

**What Nerd Genie would do.** The never-summarize rule and the after-action review exist. Give every `MEMORY.md` fact a pointer to its log event. Add a trap type to the review. Grow the failed-task-becomes-a-test rule into a nightly twenty-task set on every model, and track the keep rate.

**The risk.** A 2026 reliability study found a free-form scratchpad injected into every prompt "never helps" on long tasks. Keep traps few, dated, and measured.

## 3. The findings, topic by topic

### Prefix caching

When a model reads a prompt it builds a KV cache, its working notes on every token. If the next prompt starts with the same tokens in the same order, the server reuses the notes up to the first token that differs and reads only the rest. Anthropic caches tools, then system, then messages, so a changed tool loses everything after it; a cached read costs a tenth. What breaks it: a timestamp, a token count, a changed tool description, a reordered JSON key, a changed thinking setting, a summary that rewrites history. Our hybrid model adds one rule: reuse only works from a saved checkpoint. **Lesson:** measure the hit rate every turn, and never change a byte above the point you want reused.

### Context rot and long-context decay

Three things happen as a prompt grows. Attention spreads thin. The model reads the start and end best and the middle worst, the "lost in the middle" U-curve. And distractors hurt more at length. NoLiMa (ICML 2025), which removes word overlap between question and answer, puts the "effective length" of 27B to 70B open models at one to two thousand tokens. LOCA-bench shows open models at a third of frontier accuracy by 64,000 tokens, and a study of 35 open models found made-up answers above 10 percent for every model at 200,000. No paper has measured Qwen 3.8 27B yet. **Lesson:** small prompt, rules and goal first, results short in the middle, the open done lines and the next step last.

### Compression

The field tried summarizing and is backing away. TRACE (Aug 2026) tested the summary templates OpenClaw and Hermes use: summary-conditioned agents finished correctly 45 percent of the time against 77 percent with plain truncation. "The Sleeping Agent" found summaries throw away dates. One team that stopped compacting reports 92 percent memory recall against 38 percent at half the cost per turn. Anthropic's context editing, which swaps old tool results for placeholders, cut tokens 84 percent on a 100-turn task and raised scores 29 percent. **Lesson:** Nerd Genie's shape is right: a fixed record beside an untouched log, results shelved not rewritten, recent results kept word for word.

### Small-model tool use

Qwen3-Coder-30B reaches 50 percent on SWE-bench Verified under OpenHands, and Qwen3.5-27B is within 10 percent of the leader on the Berkeley function-calling leaderboard, so a model this size can drive tools. The levers with evidence: fewer tools in view (RAG-MCP tripled selection accuracy by showing only relevant tools), masks instead of removals, distinct names and plain descriptions, a syntax check on every edit, and a way to say "no tool fits." Aider's leaderboard shows Qwen2.5-Coder-32B malformed 148 of 225 diff edits but 99.6 percent of whole-file ones were well-formed. **Lesson:** eighteen tools is at the edge for a 27B; measure the selection error rate, mask by phase, and let the model reason in plain text before it writes the call.

### Why agents loop

Four mechanisms have evidence. Self-conditioning: errors in the context breed more errors, not fixed by size, eased by thinking. Context rot: more text, worse reasoning. State misreads: Vending-Bench agents believed an order had arrived when it had not, then looped or quit "regardless of context window size." And feedback paths with no bound: a scan of 6,549 agent repositories found 68 infinite loops. What stops loops: rollback and re-run, a checkpoint after each edit, read-only gates before writes (success up from 30 to 42 percent), OpenHands' pattern detector, and hard bounds. The benchmark agrees: Hermes alternated two commands twenty times because they succeeded and its guards only count failures. **Lesson:** a true signal from the world (idea 3), a progress meter with a rewind (idea 4), and never showing the model its own failed attempts.

### Planning shapes

ReAct (think, act, look, on every step) generally beats plan-then-execute because it adapts, at about 35 percent more input tokens. Planner and executor splits pay off on hard tasks, hurt on easy ones, and models under 10B could not run them at all. NVIDIA argues most agent steps are narrow, repeated, format-bound work suited to 3 to 10B models. Anthropic and Cognition converge on one stateful loop plus stateless workers. Checklists extracted from the instruction and scored item by item beat reward models on every benchmark tested. **Lesson:** keep the one loop that owns the record, plan first with a short think, give a well-defined step a narrow worker, and prove each done line.

### Verification

Models cannot reliably self-correct without outside feedback, and weak models critique worse than they generate. Model judges of code show position, length, and self-preference bias and are barely better than a coin at spotting false success. Execution is the honest signal, but weak tests over-credit: strengthening tests dropped the top SWE-bench agent 17 points. The builders converge: Anthropic's feature list that may not be edited, Claude Code's stop hook that blocks the turn until a script passes, Krafton's final checklist. tau-bench's pass^k (the chance all k runs succeed) shows a 90 percent agent passes eight runs in a row only 43 percent of the time. **Lesson:** run tests, strengthen tests, check the world, and use a model judge only for lines no program can check, made to name the result.

### What Manus says

Manus (July 2025) is the post the others quote. KV-cache hit rate is "the single most important metric for a production-stage AI agent," with a 10x price gap between cached and fresh tokens and a 100-to-1 ratio of input to output. Keep the prefix stable and the context append-only, with the same JSON key order every time. Mask tools rather than remove them. Use the file system as context, with any compression "designed to be restorable." Recite the goal at the end of the context. Keep failed actions in so the model adapts. Add "structured variation" so it does not fall into a rut. **Lesson:** ideas 1, 2, 7, and 10 are Manus applied; the record's fixed shape is a rut risk on a small model, so vary the nudge wording.

### What Anthropic says

Context is "a finite resource with diminishing marginal returns"; keep tools few and unambiguous; fetch by identifier just in time; for long tasks use structured notes or sub-agents that return short digests (Sept 2025). The long-running harness (Nov 2025): a progress file, a feature list that starts "failing" and may not be edited, one feature per session, end-to-end checks; the named failure was "declaring victory prematurely." The research system (June 2025): parallel workers cost 15 times the tokens. The Agent SDK post calls a model judge "generally not a very robust method"; Claude Code's docs warn that "bloated CLAUDE.md files cause Claude to ignore your actual instructions." **Lesson:** ideas 5, 7, 8, and 11; the 15x cost is why one thread is right for a local model; prune the persona files as hard as a CLAUDE.md.

### What Cognition says

"Don't Build Multi-Agents" (June 2025): share context and full traces, because "actions carry implicit decisions, and conflicting decisions carry bad results"; one thread, with a dedicated compressor model when needed. "Rebuilding Devin for Sonnet 4.5" (Sept 2025): context anxiety, a model cutting corners when it believed its window was nearly full, fixed by a bigger window capped well below its limit. The 2025 review: the agent does best with "clear, upfront requirements and verifiable outcomes" and worse "when you keep telling it more after it starts." **Lesson:** ideas 6, 8, and 11; keep mid-task corrections but push a big change into a new task.

### What OpenAI says

The harness-engineering post (Feb 2026; the original blocks robots, read through two write-ups): an `AGENTS.md` per module as a table of contents; architecture enforced by "custom linters, themselves generated by Codex"; background agents doing "garbage collection" of drift; and the line "the agent doesn't need more instructions. It needs a world where the right thing to do is obvious and the wrong thing is hard." The Codex loop post (Jan 2026) replays the whole history each turn and compacts past a limit. **Lesson:** the guard and the permission function are the "wrong thing is hard" layer; a scheduled job that prunes stale memory and skills is the garbage collector.

### What Aider says

A repo map of about 1,000 tokens lists the most-referenced symbols, ranked by a graph of file dependencies. The whole-file edit format is "the most reliable" for weak models, which in diff mode put "the entire original source file in the ORIGINAL block." A format should be familiar, simple, high level, and applied flexibly, and disabling flexibility caused "a 9X increase in editing errors." **Lesson:** idea 10; a small repo map in the Situation for coding tasks is a cheap follow-on.

### What SWE-agent says

Four interface principles: simple actions, compact actions, informative and concise feedback, guardrails. The numbers: 100-line file windows beat 30 lines and whole files; a summarizing search beat a page-through search by six points; a linter on every edit was worth three points; the interface beat a bare shell by 64 percent relative. mini-swe-agent then showed a bash-only agent of about 100 lines scoring over 74 percent on SWE-bench Verified. "Coherence Collapse" (Mar 2026): 60 to 69 percent of failures reach the right code and still produce a wrong patch, because the agent later destroys a correct one; a checkpoint after each edit recovered every case. **Lesson:** ideas 4, 7, and 10; the interface moves results as much as the model.

### What OpenHands says

An event stream of actions and observations, a sandboxed runtime, and a condenser that keeps the first four events and a recent tail, summarizing the middle past 120 events; on SWE-bench Verified it scored 54 percent against 53 without, at "less than half the cost" per turn. Its stuck detector, on by default, compares by content rather than by id and catches four identical action-observation pairs, three identical action-error pairs, and six alternating A-B-A-B pairs. Users hit false alarms when an agent legitimately polled a long command. **Lesson:** idea 4, with an exemption for polls; a bounded working set does not lose accuracy.

## 4. Sources

All from the three reports.

**llama.cpp and the local model**

- llama-server README, read 2026-09-05. https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md
- Issue #24055, checkpoints invalidated on hybrid models, 2026-06-03. https://github.com/ggml-org/llama.cpp/issues/24055
- PR #22929, checkpoint before the latest user message, 2026-05-25. https://github.com/ggml-org/llama.cpp/pull/22929
- PR #22673, MTP speculative decoding, 2026-05-16. https://github.com/ggml-org/llama.cpp/pull/22673
- llama.cpp speculative decoding docs. https://github.com/ggml-org/llama.cpp/blob/master/docs/speculative.md
- Particula, llama.cpp cache fixes for hybrid models, July 2026. https://particula.tech/blog/prompt-reprocessing-swa-hybrid-models-kv-cache
- M. Aleksandrov, the hidden prompt cache killer, 2026-06-21. https://www.mykolaaleksandrov.dev/posts/2026/06/claude-code-llamacpp-prompt-cache-fix/
- MindStudio, Qwen3.8-27B explained, 2026-08-15. https://www.mindstudio.ai/blog/qwen3-8-27b-architecture-benchmarks

**The harness builders**

- Anthropic, prompt caching docs, read 2026-09-05. https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- Anthropic, Effective context engineering for AI agents, 2025-09-29. https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- Anthropic, Managing context on the Claude Developer Platform, 2025-09-29. https://claude.com/blog/context-management
- Anthropic, Effective harnesses for long-running agents, 2025-11-26. https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents
- Anthropic, How we built our multi-agent research system, 2025-06-13. https://www.anthropic.com/engineering/built-multi-agent-research-system
- Anthropic, Building agents with the Claude Agent SDK, 2025-09-29. https://claude.com/blog/building-agents-with-the-claude-agent-sdk
- Claude Code docs, 2025 to 2026. https://code.claude.com/docs/en/best-practices
- OpenAI, prompt caching guide, read 2026-09-05. https://developers.openai.com/api/docs/guides/prompt-caching
- OpenAI, Harness engineering, Feb 2026, blocked to robots; read via https://www.infoq.com/news/2026/02/openai-harness-engineering-codex/ and https://alphasignalai.substack.com/p/a-closer-look-at-harness-engineering
- OpenAI, Unrolling the Codex agent loop, 2026-01-23. https://openai.com/index/unrolling-the-codex-agent-loop/
- Y. Ji (Manus), Context engineering for AI agents, 2025-07-18. https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- Cognition, Don't Build Multi-Agents, 2025-06-12. https://cognition.com/blog/dont-build-multi-agents
- Cognition, Rebuilding Devin for Sonnet 4.5, 2025-09-29. https://cognition.com/blog/devin-sonnet-4-5-lessons-and-challenges
- Cognition, Devin's 2025 Performance Review, 2025-11-14. https://cognition.com/blog/devin-annual-performance-review-2025
- W. Yan (Cognition), X post, 2026. https://x.com/walden_yan/status/2047054554433462360
- Cursor, Continually improving our agent harness, 2026-04-30. https://cursor.com/blog/continually-improving-agent-harness
- Aider, repo map, edit formats, leaderboards. https://aider.chat/docs/repomap.html , https://aider.chat/docs/more/edit-formats.html , https://aider.chat/docs/leaderboards/
- Yang et al., SWE-agent: Agent-Computer Interfaces, May 2024. https://arxiv.org/abs/2405.15793
- SWE-agent team, mini-swe-agent, 2025. https://github.com/SWE-agent/mini-swe-agent
- OpenHands paper, ICLR 2025. https://arxiv.org/abs/2407.16741
- OpenHands condenser blog, 2025-04-09. https://www.openhands.dev/blog/openhands-context-condensensation-for-more-efficient-ai-agents
- OpenHands SDK docs, Stuck Detector. https://docs.openhands.dev/sdk/guides/agent-stuck-detector
- OpenHands issue 5355, false alarms. https://github.com/All-Hands-AI/OpenHands/issues/5355
- Krafton, Terminus-KIRA, 2026. https://github.com/krafton-ai/KIRA
- G. Huntley, Ralph Wiggum as a software engineer, 2025. https://ghuntley.com/ralph/
- L. Bouchard, Why we stopped compacting our agent's context, 2026-08-18. https://www.louisbouchard.ai/context-engineering-2026/

**Long context, compression, and memory**

- Hong, Troynikov, Huber (Chroma), Context Rot, 2025-07-14. https://www.trychroma.com/research/context-rot
- Modarressi et al., NoLiMa, Feb 2025. https://github.com/adobe-research/NoLiMa
- Du et al., Context Length Alone Hurts LLM Performance Despite Perfect Retrieval, 2025-10-06. https://arxiv.org/abs/2510.05381
- Zeng, Huang, He, LOCA-bench, Feb 2026. https://arxiv.org/html/2602.07962v1
- Min et al., TRACE: Toward Reliable Context Compression for Long-Horizon Agents, 2026-08-06. https://arxiv.org/html/2608.06503v1
- Kyrkewood, The Sleeping Agent, 2026-08-12. https://arxiv.org/abs/2608.11775
- Roig, How Much Do LLMs Hallucinate in Document Q&A?, 2026-03-09. https://arxiv.org/abs/2603.08274
- Liu et al., Lost in the Middle, July 2023. https://arxiv.org/abs/2307.03172
- Zhang, Kraska, Khattab, Recursive Language Models, 2025-12-31. https://arxiv.org/abs/2512.24601
- An, Verbatim Chunks Beat Extracted Artifacts, Dec 2025. https://arxiv.org/abs/2601.00821
- Ouyang et al., ReasoningBank, Sept 2025. https://arxiv.org/abs/2509.25140
- Khanal, Tao, Zhou, Beyond pass@1: A Reliability Science Framework for Long-Horizon LLM Agents, March 2026. https://arxiv.org/abs/2603.29231

**Structured output and thinking**

- Lee, D'Antoni, Berg-Kirkpatrick, The Format Tax, 2026-04-04. https://arxiv.org/abs/2604.03616
- Fan, Capacity, Not Format, 2026-06-08. https://arxiv.org/abs/2606.09410
- Li, Zhang, Lv, Constraint Tax in Open-Weight LLMs, 2026-06-24. https://arxiv.org/abs/2606.25605
- Cuadron et al., The Danger of Overthinking, Feb 2025. https://arxiv.org/abs/2502.08235

**Reliability, loops, tools, and verification**

- Advani, Characterizing False Success in LLM Agents, June 2026. https://arxiv.org/abs/2606.09863
- Reddy, Challaram, Basu, Reason Less, Verify More, July 2026. https://arxiv.org/abs/2607.07405
- Sinha et al., The Illusion of Diminishing Returns: Measuring Long Horizon Execution in LLMs, Sept 2025. https://arxiv.org/abs/2509.09677
- Dubey, Real-Time Detection and Repair of LLM Agent Failures, Aug 2026. https://arxiv.org/abs/2608.02464
- Kim et al., Coherence Collapse, March 2026. https://arxiv.org/abs/2603.24631
- Andon Labs, Vending-Bench, Feb 2025. https://arxiv.org/abs/2502.15840
- Hou et al., When Agents Do Not Stop: Uncovering Infinite Agentic Loops, July 2026. https://arxiv.org/abs/2607.01641
- RAG-MCP: Mitigating Prompt Bloat in LLM Tool Selection, May 2025. https://arxiv.org/abs/2505.03275
- Yu et al., SWE-ABS: Adversarial Benchmark Strengthening Exposes Inflated Success Rates, Feb 2026. https://arxiv.org/abs/2603.00520
- Molinari, Ciravegna, Reason-Plan-ReAct, Dec 2025. https://arxiv.org/abs/2512.03560
- Belcak et al., Small Language Models are the Future of Agentic AI, June 2025. https://arxiv.org/abs/2506.02153
- Viswanathan et al., Checklists Are Better Than Reward Models, July 2025. https://arxiv.org/abs/2507.18624
- Yao et al., tau-bench, June 2024. https://arxiv.org/abs/2406.12045
- Huang et al., Large Language Models Cannot Self-Correct Reasoning Yet, Oct 2023. https://arxiv.org/abs/2310.01798
- Lin et al., CriticBench, Feb 2024. https://arxiv.org/abs/2402.14809
- Don't Judge Code by Its Cover: Biases in LLM Judges for Code, May 2025. https://arxiv.org/abs/2505.16222
- Lee et al., Meta-Harness, 2026-03-30. https://arxiv.org/abs/2603.28052
- Qwen3-Coder-30B-A3B SWE-bench Verified reproduction, Hugging Face. https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct/discussions/30
- Berkeley Function Calling Leaderboard V4. https://gorilla.cs.berkeley.edu/leaderboard.html

**Nerd Genie's own measurements**

- `QWEN_BENCHMARK.md` in this repository, runs of 2026-09-03.
