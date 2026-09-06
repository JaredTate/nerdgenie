# Web research: state, tokens, records and reliability for the Nerd Genie harness

Written 2026-09-06. Target: Qwen 3.8 27B on one card through llama-server, about 400 tok/s prefill and 60-70 tok/s generation, 131k context, MTP on, hybrid Gated-DeltaNet + attention model, context checkpoints on. All arithmetic below uses those numbers.

How to read the labels. "Read in full" means I read the paper or page itself. "Read via summary" means I read a machine summary of the page or PDF and did not check every number against the original; treat those numbers as approximate.

---

## Part 1. Sources, two lines each

### Context engineering for coding agents

1. Manus, "Context Engineering for AI Agents: Lessons from Building Manus" (read in full). KV-cache hit rate is "the single most important metric for a production-stage AI agent"; input:output ratio about 100:1; cached tokens cost 10x less than uncached. Rules: stable prefix (no timestamps), append-only context, deterministic JSON key order, mask tools with logits instead of removing them, file system as unlimited memory, "restorable compression" (drop a page body but keep its URL/path), recite a todo.md at the end of context, keep failures in context, break few-shot ruts with small variation. Tasks average about 50 tool calls.
   https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus

2. Anthropic, "Effective context engineering for AI agents" (read in full). Compaction should keep decisions, unresolved bugs and implementation details and drop redundant tool output; "tool result clearing" is the safest lightest form; structured note-taking (a NOTES.md the agent writes and re-reads) carried an agent through thousands of Pokemon steps; sub-agents return 1,000-2,000-token summaries; just-in-time retrieval by lightweight identifiers (paths, queries) instead of pre-loading.
   https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents

3. Anthropic, "Effective harnesses for long-running agents" (read in full). Two-agent split: an initializer writes a JSON feature list (200+ items, all "passes": false), a progress file, init.sh and a first commit; each later session runs pwd, reads git log and the progress file, picks one unfinished feature, runs init.sh and a smoke test before touching code. JSON was chosen because "the model is less likely to inappropriately change or overwrite JSON files compared to Markdown files". Failure modes named: premature "done", undocumented progress, untested features, starting from scratch. No completion statistics.
   https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents

4. OpenAI, "Harness engineering: leveraging Codex in an agent-first world" (primary page returned 403; read via secondary summaries). AGENTS.md as a table of contents, not an encyclopedia, because a giant instruction file crowds out the task; docs directory as the knowledge base with structural linters enforcing it. Secondary sources report harness-level retained reasoning plus compaction took a model from 13.3% to 38.3% on ARC-AGI-3 while cutting tokens 6x. Unverified by me.
   https://openai.com/index/harness-engineering/ and https://kenhuangus.substack.com/p/from-software-engineering-to-harness

5. Cognition, "Don't Build Multi-Agents" (read in full). Two rules: share full traces, not summaries, and remember that actions carry implicit decisions that conflict across parallel agents. For long tasks they compress history with a dedicated model into "key details, events, and decisions" and say it is hard to get right; default to a single-threaded linear agent.
   https://cognition.com/blog/dont-build-multi-agents

6. OpenHands, "Context Condensation for More Efficient AI Agents" (read in full). LLM summarization every N turns keeping M recent: 54% solved with condensation versus 53% without on a SWE-bench Verified subset, per-turn cost "less than half" once it kicks in, linear rather than quadratic growth; slightly more turns per task.
   https://www.openhands.dev/blog/openhands-context-condensensation-for-more-efficient-ai-agents

7. Lindenbauer et al., "The Complexity Trap: Simple Observation Masking Is as Efficient as LLM Summarization" (NeurIPS 2025 workshop; read in full). SWE-agent on SWE-bench Verified, five model setups including Qwen3-32B. Observations are 84% of a turn's tokens. Replacing observations older than 10 turns with a placeholder halves cost and matches or beats LLM summaries. Qwen3-32B: raw 17.0% at $1.12, masking 15.0% at $0.55, summary 16.0% at $0.50 (differences inside the confidence interval). Qwen3-Coder-480B: raw 53.4%, masking 54.8% at -52.7% cost. LLM summaries lengthen trajectories 13-15% because they "mask failure signals" and cost 0.65-7.2% extra in summary calls. Hybrid (mask at 10, summarize only when 43 turns accumulate) is best; a naive hybrid was worse "due to compounding KV cache inefficiencies". The right window is scaffold-specific (OpenHands needed 58, not 10).
   https://arxiv.org/abs/2508.21433

8. "Evaluating Memory Condensation Strategies for Coding Agents in Data-Driven Scientific Discovery" (read via summary). Eight strategies, 60 tasks. Observation masking was the only one that saved tokens (+8.6%) with no quality loss; every LLM-based condenser raised total cost 24-94% because of the condensation calls and extra turns, with no measurable quality gain.
   https://arxiv.org/html/2605.18854

9. "Context Compaction Deep Dive: Codex CLI, Claude Code, OpenCode" (read in full). Codex compacts at window-13,000 tokens and then re-reads up to 5 recent files under a 50,000-token budget; OpenCode prunes tool outputs first and only when that frees over 20,000 tokens, protecting the newest 40,000 tokens; Claude Code layers microcompaction (tool results) under auto-compaction, and 85-90% was found a better trigger than 95%.
   https://codex.danielvaughan.com/2026/04/14/context-compaction-deep-dive-codex-cli-claude-code-opencode/

10. "Inside Claude Code's Compaction System" (read in full). Microcompaction is a cache policy: a "hot tail" of recent tool results stays verbatim, older ones become a path reference. Compaction triggers on headroom accounting (space for output plus the compaction call), not a fixed percent, and emits a structured working state (intent, decisions, files touched, errors, pending tasks, next steps), then re-reads recent files and restores the todo list.
    https://decodeclaude.com/compaction-deep-dive/

11. mem0, "How Hermes and Claude handle context compression" (read in full). Hermes fires its compressor at 50% and a safety net at 85%; before any model call it replaces old tool outputs longer than 200 characters with a placeholder by string replacement; summaries use a fixed five-section template and are updated, not regenerated. Both systems silently lose exact numbers, one-time constraints, and the reasons behind decisions.
    https://mem0.ai/blog/how-hermes-and-claude-handle-context-compression-in-real-production-agents-(and-what-you-should-extract)

12. Chroma, "Context Rot" (read in full). 18 models including Qwen3-32B and Qwen3-8B: accuracy falls monotonically with input length even on trivial tasks; a single distractor hurts; a coherent haystack is worse than a shuffled one; focused ~300-token prompts beat the same question inside 113k tokens.
    https://www.trychroma.com/research/context-rot

13. SWE-agent (NeurIPS 2024; read via summary of the paper). A 100-line file viewer window beat both 30 lines and whole files; an edit-time linter that rejects syntax errors added 3.0 points; all guardrails were built to be 100%-precision.
    https://arxiv.org/abs/2405.15793

### Small and local models in agent loops

14. McClendon et al., "Three Roles, One Model" (read in full). Qwen3-8B on AppWorld, one 24 GB card. The same frozen model in three roles, summarizer, actor, and a corrector that is denied the history, took task completion from 5.4% to 8.9% (FP16) and 3.0% to 5.9% (AWQ), passing DeepSeek-Coder-33B's 7.1%. Failure analysis: authentication loss 28%, planning 26%, API schema mismatch 18%; a "credential-loss loop" once context pushed early facts out. Cost: at most three forward passes per step.
    https://arxiv.org/abs/2604.11465

15. "Where Does Agent Reliability Come From?" (read via summary). On SpreadsheetBench most of an +11.0-point uplift came from structure (+9.5) and the verification loop added +1.5, but the verification step was the decisive one at the top of the leaderboard. Swapping the small independent verifier for the same frontier model that wrote the artifact cut rescues from 6 to 2: the observer must be independent. Verifiers were 0.5-4B Qwen3 models.
    https://arxiv.org/html/2607.17044

16. smallcode (read in full). Agent tuned for 8-35B models: two-stage tool routing (category first, then only that category's schemas), forgiving tool-call parser, patch-first editing because small models "truncate, hallucinate, or drift" when rewriting files, tool results capped at 4k characters with mid-turn eviction, TODO-file planning with lint/compile before advancing, early-stop detection for repetition loops and "patch spirals", and retries at varied temperature. The "87%" claim is not backed by a published table.
    https://github.com/Doorman11991/smallcode

17. Kovacs, "Squeez: Task-Conditioned Tool-Output Pruning for Coding Agents" (read in full). A LoRA-tuned Qwen 3.5 2B that, given a one-line query and one raw tool output, returns only the lines the agent needs: 0.86 recall, 0.80 F1 while removing 92% of tokens; 11 recall points above zero-shot Qwen 3.5 35B-A3B; head/tail/BM25 heuristics reach 0.05-0.22 recall. Returns empty output correctly 80% of the time on negatives (Qwen 35B: 7%). Downstream task effect not measured.
    https://arxiv.org/abs/2604.04979

18. "A Workflow-Aware Serving Layer for Agentic Applications" (read via summary). A recovery ladder that re-runs the failed step with the error as feedback and escalates (more tokens, then a bigger model, then a reworded prompt) recovered 84% of forced failures versus 55% for flat retry and 0% without retry, at +37% mean latency; end-to-end correctness 14% to 20%.
    https://arxiv.org/abs/2607.02942

19. Pham et al., "AgentStop" (read in full). A gradient-boosted classifier over the 10 smallest logprobs per step, chain-of-thought length, and token overlap between consecutive steps, trained on local-agent traces, cut wasted energy 15-20% with under 5% utility drop for Qwen3-30B-A3B agents run through llama.cpp. Failed SWE-bench runs averaged 544 s and 24.5k input tokens versus 275 s and 15.1k for successes.
    https://arxiv.org/abs/2605.15206

20. prime-agent issue #1029 (read in full). A thinking block degenerated into "The the the" 2,366 times, 79k characters, 19.8k output tokens before a human aborted. Fix ported from a sibling harness: 250-character verbatim tail repeat, word-trigram Jaccard near-duplicate check, low-novelty entropy stall; calibrated on 13.5k non-loop blocks with zero false positives; on trigger, abort and re-sample instead of persisting the garbage.
    https://github.com/PrimeIntellect-ai/prime-agent/issues/1029

21. "Agent Cost Runaway Detection" (read in full). Rate-based breaker (10,000 tokens/min reference) caught a loop within 60 s; budget ceiling at twice the p95 of staging runs; blocking must happen before the next call, not in a dashboard.
    https://www.getreadyforagents.com/blog/agent-cost-runaway-detection-token-enforcement-production/

### Long-horizon memory and state

22. Wu et al. (Meta), "Remember When It Matters: Proactive Memory Agent" (read in full). Names the failure "behavioral state decay": facts still in the window stop steering behaviour. A separate memory agent every 8 steps updates a bank (status, knowledge, procedural: failed commands, fixes) through four tool calls, then either injects a short reminder or stays silent. Terminal-Bench 2.0: Sonnet 4.5 37.6% to 45.9% (+8.3), Opus 4.6 43.5% to 45.9%. Ablation: exposing the whole bank every step is worse than selective injection (macro 61.5 vs 64.3); always-inject roughly ties; Mem0 retrieval 62.1. Qwen3.5-27B trained with SFT+GRPO as the memory agent "partially" transfers.
    https://arxiv.org/abs/2607.08716

23. "Long-Horizon-Terminal-Bench" (read via summary). 46 tasks, average 231 episodes, 85 minutes, 9.9M tokens, $10 per task; best model 15.2% at the 0.95 threshold, average 4.3%. 79% of failures are the 90-minute budget expiring mid-work, 19% are the agent stopping early "despite not yet satisfying the hidden verifier"; 14 runs at 0.75+ reward declared victory with 15-20 minutes unused.
    https://arxiv.org/abs/2607.08964

24. Zhang, Kraska, Khattab, "Recursive Language Models" (read in full). Put the prompt in a REPL variable and let the model write code to slice it and call itself on pieces. Beats a compaction agent by a 26% median on GPT-5; on CodeQA, Claude Code jumps 12% to 62% simply by offloading context to a file; fine-tuning Qwen3-8B on 1,000 trajectories raised it 28% as an RLM. Compaction "presumes that some details that appear early in the prompt can safely be forgotten".
    https://arxiv.org/abs/2512.24601

25. ESAA-Conversational (read via summary). Event-sourced memory for coding agents: append-only activity.jsonl with conversation_turn, decision.recorded, task.* events; four read models (state.md, handoff.md, decisions.md, tasks.json) that "are not edited manually" and are rebuilt deterministically; agents pull paginated windows by topic or agent rather than replaying the log. 570 events, Codex/Claude/Grok sharing one log. No compaction or snapshots yet.
    https://arxiv.org/html/2606.23752

26. tianpan.co, "Agent State as Event Stream" (read in full). Events are truth, projections answer "what is the state now", the model sees the projection and emits new events; time-travel replay without spending tokens. Pitfalls: unbounded log growth (snapshot), ordering, schema versioning from day one.
    https://tianpan.co/blog/2026-04-10-agent-state-event-stream-immutable-event-sourcing

27. Ralph Wiggum loop (ghuntley; codecentric write-up) (read via summary). A bash loop restarts the agent with a fresh context each iteration; IMPLEMENTATION_PLAN.md on disk is the only shared state; each iteration picks one task, implements, tests, commits, exits. No controlled numbers.
    https://github.com/ghuntley/how-to-ralph-wiggum and https://www.codecentric.de/en/knowledge-hub/blog/the-ralph-wiggum-loop-autonomous-code-generation-with-a-fresh-context

28. Letta, "Context Repositories" (read via summary). Memory moves from database tools to git-backed files edited with ordinary bash; sub-agents get isolated worktrees and merge memory through git.
    https://www.letta.com/blog/context-repositories/

### llama.cpp and llama-server specifics

29. llama.cpp issue #22384, hybrid checkpoint restore fix (read in full). On Qwen3.6-27B Q4_K_M, RTX 3090: turn 2 at 12k context went from re-processing 12,146 tokens (~11 s) to 31 tokens (115 ms). Two bugs: pos_min check never matched for recurrent state, and checkpoints needed 64+ tokens (lowered to 4).
    https://github.com/ggml-org/llama.cpp/issues/22384

30. llama.cpp issue #24055, checkpoints always invalidated on hybrid models (read in full; open, unconfirmed). Regression at commit e98cb51: builds b9354 and later create one checkpoint and throw it away ("forcing full prompt re-processing due to lack of cache data"); b9309 with 512-token checkpoints worked. Reported on a Qwen 3.6 27B MTP build with --checkpoint-min-step 512 --ctx-checkpoints 64 --kv-unified.
    https://github.com/ggml-org/llama.cpp/issues/24055

31. llama.cpp discussion #19264, partial prompt-cache reuse for recurrent models (read in full). Recurrent state cannot be truncated, so only snapshots at boundaries can be restored; maintainer says it is implemented, reporter says cache still rebuilds. 2,400 tokens x 22 ms = 54 s per request without it.
    https://github.com/ggml-org/llama.cpp/discussions/19264

32. Particula, "Full Prompt Re-Processing: llama.cpp Cache Fixes That Work" (read in full). Diagnose with `-lv 4 | grep "forcing full prompt re-processing"`; dump prompts with `--log-prompts-dir` and diff consecutive turns; use `--reasoning-preserve` instead of stripping thinking; set `--checkpoint-min-step` near the median turn length; about 150 MiB per checkpoint per slot; `--checkpoint-every-n-tokens` removed May 2026; `--swa-full` does nothing for hybrids; `--parallel 1` or a higher `--slot-prompt-similarity` stops warm slots being stolen.
    https://particula.tech/blog/prompt-reprocessing-swa-hybrid-models-kv-cache

33. llama.cpp discussion #20572, persistent KV per session (read in full). `--slot-save-path` plus /slots/0/restore before and /slots/0/save after each turn: a 27B Qwen at 100k context on a 3090 went from over a minute of prefill to about 0.2 s; files 1-4.4 GB. A separate issue in search results says the on-disk format did not include hybrid checkpoints, so verify.
    https://github.com/ggml-org/llama.cpp/discussions/20572

34. llama.cpp discussion #13606, KV cache reuse tutorial (read in full). `cache_prompt` reuses the common prefix and only processes the suffix ("kv cache rm [33, end)"); automatic slot selection by similarity was "inconsistent"; pin `id_slot` for determinism.
    https://github.com/ggml-org/llama.cpp/discussions/13606

35. llama-server README (read in full). Response `timings` carries `cache_n` ("number of prompt tokens reused from cache"), `prompt_n`, `prompt_ms`, `predicted_n`, `predicted_ms`; /metrics has draft_n and draft_n_accepted. `--cache-reuse N` is KV shifting for chunk reuse (attention-only; not usable on recurrent state). `--ctx-checkpoints` default 32, `--checkpoint-min-step` default 8192. `--slot-prompt-similarity` default 0.10. `--image-min-tokens` / `--image-max-tokens` bound vision tokens.
    https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md

36. llama.cpp docs/speculative.md (read in full). `--spec-type` accepts draft-mtp, draft-eagle3, draft-dflash, draft-dspark and draft-free ngram-simple, ngram-map-k, ngram-map-k4v, ngram-mod (a ~16 MB shared hash pool), ngram-cache; defaults `--spec-draft-n-max 3`; n-gram modes are recommended for "iterating over a block of text/code" and reasoning models repeating their thinking. Acceptance in the examples 0.58-0.70.
    https://github.com/ggml-org/llama.cpp/blob/master/docs/speculative.md

37. sudoingX/qwen38-mtp (read in full). Qwen3.8-27B with `--spec-type draft-mtp --spec-draft-n-max 2 --parallel 1`: RTX 3090 31.0 to 41.3 tok/s (+33%, acceptance 0.78), 4090 47.7 to 76.3 (+60%), 7900 XTX 36.3 to 62.6 (+72%), 5090 76.9 to 155.5 (+102%). Code prompts accept 0.75-0.90, prose 0.40-0.60. 24 GB cards peak at n-max 2. "Speculative decode is a single-stream optimization... by --parallel 4 the advantage is gone." Their stack used q4_0 K and V.
    https://github.com/sudoingX/qwen38-mtp

38. "Why MTP doesn't speed up your llama.cpp inference" (read in full). Acceptance under ~0.6 makes MTP slower; temperature above 0.7 and heavy quantization of the head hurt; CUDA graph re-capture on variable acceptance (test with GGML_CUDA_DISABLE_GRAPHS=1); dropping draft length 4 to 2 pushed acceptance over 0.7 and gave 1.7x.
    https://dev.to/alanwest/why-mtp-doesnt-speed-up-your-llamacpp-inference-and-how-to-actually-fix-it-2m2m

39. llama.cpp issue #24507 (read in full). draft-mtp and an ngram mode can run together from CLI flags; in router mode (models.ini) only the last spec-type wins. Open.
    https://github.com/ggml-org/llama.cpp/issues/24507

40. KV-cache quantization evidence (read via summaries; weak). q8_0 K/V is near-lossless across reports; q4_0 adds 0.1-0.25 perplexity depending on architecture and one report measured 36.8% lower generation throughput at 110k context with q4_0 from dequantization. Nobody measured tool-call or JSON accuracy. For Qwen3.8 only 16 of 64 layers hold attention KV, so the VRAM saving is small anyway.
    https://medium.com/rigel-computer-com/optimize-your-gpu-kv-cache-for-llama-cpp-opencode-co-13b6bc74f5ec and https://dev.to/plasmon_imp/q4-kv-cache-fit-32k-context-into-8gb-vram-only-math-broke-209k

41. "Stateful Inference for Low-Latency Multi-Agent Tool Calling" and "SmoothAgent" (both read via summary; numbers unverified). Keep KV resident across tool calls and pin sessions; SmoothAgent prefills the predicted next context while the tool is still running. Both on vLLM-class servers; the idea of overlapping prefill with tool execution transfers.
    https://arxiv.org/abs/2605.26289 and https://arxiv.org/abs/2607.00151

### Token accounting and pictures

42. theinfinity.dev, "What a Screenshot Costs an AI Agent: 25,457 Tool Results Measured" (read in full). Six weeks of Claude Code transcripts: screenshot median 2,054 tokens (p90 3,588), file read median 2,085 (p90 11,401, p99 29,824), bash median 1,278 (p90 3,556), search 3,104. Bash is 64% of all tool results; screenshots were 6% of spend; 45% overhead from re-reading files just edited. Screenshots are the most predictable cost because pixels cap them.
    https://theinfinity.dev/articles/agent-tool-cost-measured

43. Qwen vision token formula (read via docs and an HF thread). Qwen3-VL-family: tokens = h x w / (32 x 32) + 2, so 1920x1080 = 2,027 tokens, 1280x720 = 902, 900x900 = 793. This matches the ~800 tokens per screenshot already measured on irene. llama.cpp caps with `--image-min-tokens` / `--image-max-tokens`.
    https://huggingface.co/Qwen/Qwen3.6-35B-A3B/discussions/36

44. Qwen3.8-27B model card (read in full). 64 layers as 16 x (3 DeltaNet + 1 attention), MTP head trained in, 262k native context. Recommended sampling: thinking temp 1.0 / top_p 0.95 / top_k 20; non-thinking temp 0.7 / top_p 0.8 / presence_penalty 1.5. Terminal-Bench 2.1 73.0, SWE-bench Pro 61.7, OSWorld 84.3.
    https://huggingface.co/Qwen/Qwen3.8-27B

45. OpenClaw token-use reference (read in full). Everything the model receives counts; tool results capped at 30% of the window with absolute tiers (16k chars under 100k context, 32k, 64k); `imageMaxDimensionPx` default 1200; keep caches warm with heartbeats just under the TTL. Useful as the comparison harness on the same box.
    https://docs.openclaw.ai/reference/token-use

---

## Part 2. The measurement harness every idea needs (do this first)

Every completion response from llama-server carries `timings.cache_n`, `prompt_n`, `prompt_ms`, `predicted_n`, `predicted_ms`; /metrics carries draft_n and draft_n_accepted. Log all five per round into the event log as a `llm.timings` event with the round number, the prompt byte length and a hash of the first 4 KB of the prompt. From that, one script gives per job:

- uncached tokens per round = prompt_n - cache_n (target: about the size of the last tool result, not the whole context)
- prefill seconds per job = sum(prompt_ms) / 1000
- generation seconds per job = sum(predicted_ms) / 1000
- acceptance = draft_n_accepted / draft_n
- rounds, tool errors, repeated identical tool calls, "declared done then verifier failed"

Run the server with `-lv 4` for one night and count lines matching "forcing full prompt re-processing" and "restored context checkpoint". If the first is common, nothing else in this document matters until it is fixed (issue #24055 is a live regression in builds b9354 and later).

Baseline reference at 400 tok/s prefill: every 10,000 uncached tokens costs 25 s. A 100-round job that re-prefills a 60k context each round spends 100 x 150 s = 4.2 hours in prefill alone; the same job with a stable prefix and ~2k new tokens per round spends 100 x 5 s = 8 minutes.

---

## Part 3. Eight candidate ideas

### Idea 1. Cache-shaped context: stable prefix, append-only, and a sawtooth rebuild instead of per-round summaries

What it is. Freeze the byte-exact prefix (system prompt, tool schemas in fixed order, the task order), never rewrite anything already sent, and manage size by (a) replacing observations older than M turns with a one-line placeholder that keeps the path or command, and (b) doing that replacement in one batch at a fixed cadence rather than one turn at a time. Do not run an LLM summarizer on the transcript except as a last resort when the batch-masked context still exceeds the budget.

Evidence. Manus (append-only, stable prefix, restorable compression). Complexity Trap: masking halves cost and matches summaries across five setups, and summaries lengthen runs 13-15% and cost up to 7.2% extra; the paper warns a naively scheduled hybrid was worse "due to compounding KV cache inefficiencies". Memory-condensation study: masking was the only strategy that saved tokens without hurting; LLM condensers cost 24-94% more. Claude Code's microcompaction (hot tail verbatim, cold results by path) and OpenCode (prune only when it frees 20k+ tokens) are the same shape.

Expected gain, with arithmetic. Observations are ~84% of turn tokens. With ~2.4k tokens per turn, an unmasked 100-round job is ~250k tokens and overflows 131k around round 50; masking all but the last 10 turns' observations holds it near 10k + 100 x 400 + 10 x 2k = 70k. The cadence matters on this model: masking one old turn each round edits the prefix each round, so each round re-prefills roughly the last 10 turns (~24k tokens = 60 s). Masking 10 turns at once every 10 rounds costs one 60 s round and nine ~5 s rounds, about 10.5 s average, six times cheaper. A full rebuild (system + projection + last K turns) every ~40 rounds is cheaper still, about 2 s per round amortised, and is what Codex and Claude Code effectively do.

What could go wrong with a 27B model. The Complexity Trap's Qwen3-32B result is the one setup where masking's solve rate dipped (17.0 to 15.0, within noise) and its thinking variant saved only 10% because its runs were short. Small models sometimes re-run a tool because the placeholder hides the answer; keep the placeholder informative ("read src/a.py lines 1-120; 4,812 chars; hash abc1") so the model can decide whether to re-read. The right window is scaffold-specific (10 for SWE-agent, 58 for OpenHands, which keeps retry turns); sweep 6, 10, 16.

How to measure in one night. Same 10 tasks, three configs: no masking, per-round masking M=10, batched masking every 10 rounds M=10. Compare uncached tokens per round, prefill seconds per job, rounds, solve count, and tool re-reads.

Status. Proven for the masking and stable-prefix parts (two independent studies plus three production harnesses). The batching arithmetic for a hybrid model is my inference from how checkpoints work; it needs the one-night test.

### Idea 2. Checkpoint-aware turns: make every context edit land on a checkpoint boundary, and verify it nightly

What it is. On a Gated-DeltaNet hybrid the recurrent state cannot be truncated, so llama-server can only roll back to a saved checkpoint; any edit earlier than the oldest retained checkpoint forces a full re-prefill. Tune `--checkpoint-min-step` to roughly the median turn length so there is a checkpoint at nearly every turn boundary, keep `--ctx-checkpoints` high enough to cover the masking window from Idea 1, pin `id_slot`, run `--parallel 1` unless Idea 4 is on, and assert in the harness that the prefix through the last checkpoint is byte-identical before each request. Also test whether /slots save+restore preserves checkpoints in the current build, so a job can be paused and resumed without a 100k re-prefill.

Evidence. Issue #22384: on Qwen3.6-27B, turn-2 prefill fell from 12,146 tokens (~11 s) to 31 tokens (115 ms) once checkpoints restored correctly. Issue #24055: a regression in b9354+ throws checkpoints away on hybrids. Particula: ~150 MiB per checkpoint per slot, `--checkpoint-every-n-tokens` removed, `--swa-full` irrelevant here, diff consecutive prompts with `--log-prompts-dir`. Discussion #20572: slot restore is ~0.2 s versus over a minute of prefill at 100k on a 3090.

Expected gain. At 400 tok/s a 100k re-prefill is 250 s; a checkpoint hit costs only the tail. If even 10 of 100 rounds silently fall back to full re-prefill at an average 60k context, that is 10 x 150 s = 25 minutes per job that shows up as "the model is slow". Checkpoint VRAM: the current igo.sh runs `--checkpoint-min-step 1024 --ctx-checkpoints 16`; at ~150 MiB each that is ~2.4 GB, which is fine, but 16 checkpoints at 1,024-token spacing only cover about 16k tokens of history, so a masking window of 10 turns x 2.4k = 24k tokens is not coverable; either raise checkpoints to 32 or lengthen the step to match the turn size.

What could go wrong. Build drift: the regression in #24055 is open and the fix in #22384 is recent; a `llama.cpp` update can silently return to full re-prefill. Thinking-token stripping between turns changes the prefix (use `--reasoning-preserve` or never strip). Two slots on a card double checkpoint VRAM.

How to measure in one night. `-lv 4` log; count "restored context checkpoint" versus "forcing full prompt re-processing" per round; plot prompt_ms against round; should be flat. Then save+restore a slot mid-job and confirm the next turn's prompt_n is small.

Status. Proven mechanism with measured numbers on the sibling model (Qwen3.6-27B); whether the current build on irene behaves is unknown until measured.

### Idea 3. The order is a projection: an event log that is the truth, a harness-owned task record the model cannot edit, and a recited tail

What it is. Keep the append-only event log as the only source of truth. Have the harness (not the model) deterministically rebuild three small read models from it: the order (mission, constraints, acceptance criteria, sub-tasks with pass/fail flags, in JSON), a progress note (what happened, last N decisions with reasons, open bugs), and a handoff block for a fresh context. Each round the model sees the stable prefix, the projection, the recent turns, and, as the last thing before it speaks, a ~200-token recitation of the current sub-task and its acceptance check. The model changes state only by emitting typed events (task.claimed, task.passed with evidence, decision.recorded with rationale), which the harness validates before appending.

Evidence. Manus recites a todo at the end of context to fight lost-in-the-middle across ~50 tool calls. Anthropic's long-running harness uses a JSON feature list because models tamper with JSON less than Markdown, plus a progress file, git log and a fixed session-start routine, and names premature "done" and starting-from-scratch as the failures it cured. ESAA and tianpan.co: read models are rebuilt, never hand-edited; replay is free. LHTB: 19% of failed long runs stop early despite not satisfying the hidden verifier; 14 near-complete runs quit with time unused. Proactive Memory names "behavioral state decay" and shows +8.3 points from re-injecting decision-relevant state.

Expected gain. No controlled number exists for the pattern itself; the failures it targets are measured (a fifth of long-run failures are premature exits). The recitation costs ~200 tokens of uncached prefill per round, about 0.5 s. A projection rebuilt from events also gives resume-after-crash for free: a new context is system + projection + last K turns, ~15k tokens, 40 s of prefill instead of replaying 100 rounds.

What could go wrong with a 27B model. The model may emit task.passed without evidence; require the event to carry a verifier reference (test output hash, screenshot path) and reject it otherwise (this is where Idea 4 plugs in). Recitation can become a few-shot rut (Manus); vary wording slightly. If the model is allowed to write the projection files directly it will overwrite them (Anthropic's exact failure); keep them harness-owned and read-only.

How to measure in one night. Count per job: task.passed events later contradicted by a verifier, repeated identical tool calls, rounds spent re-reading files already in the projection, and whether a killed job resumes and finishes. Compare against the current task-record format.

Status. Proven in practice at Anthropic, Manus and the Ralph-loop community, with no published effect sizes; the resume arithmetic is straightforward.

### Idea 4. An independent verifier that is denied the history, run on a second slot only at gates

What it is. A separate request (same 27B weights, own stable prefix, own `id_slot`) that receives only the artifacts (the diff, the test output, the screenshot, the acceptance criterion from the order) and never the transcript, and answers a fixed rubric: does the evidence satisfy the criterion, yes/no, with the one line of evidence. Run it at gates only: after each edit that touches a file named in the order, and whenever the model claims a sub-task passed. A failed verification appends a task.verify_failed event with the verifier's line; two consecutive failures escalate (Idea 6).

Evidence. Leni paper: the verification step's isolated uplift was small (+1.5 of +11 points) but decisive at the top, and independence is what mattered: letting the generating model verify its own work dropped rescues from 6 to 2. Three Roles: a corrector with no access to history lifted Qwen3-8B from 5.4% to 8.9% (FP16) on AppWorld by breaking "repetitive correction loops". SWE-agent: a syntax check at edit time added 3 points and was 100%-precision. Anthropic: end-to-end tests "as a human user would" are what stopped false completions.

Expected gain, with arithmetic. One verification is ~3k tokens of prefill (7.5 s) plus ~150 tokens of output (2.5 s), about 10 s; 10 gates per 100-round job cost under 2 minutes. One caught false "done" saves the whole job.

What could go wrong. Same weights agreeing with themselves; withholding the history and asking for the evidence line is the mitigation the papers used, and it only partly works. A second slot halves per-slot context unless `--kv-unified` is on, adds ~150 MiB per checkpoint for that slot, and, per qwen38-mtp, concurrent decode erodes the MTP gain (gone by `--parallel 4`; with 2 slots and the verifier idle most of the time the loss should be small but measure it). A too-strict verifier causes loops: cap at two corrections per gate then escalate.

How to measure in one night. Seed 20 known-bad states (a patch that does not compile, a screenshot of the wrong screen, a test that was skipped) plus 20 known-good ones; record catches, false rejections, seconds per verification, and MTP acceptance with `--parallel 2` versus 1.

Status. Proven as a pattern with small measured effect sizes; independence effect measured once. The slot-sharing cost on this card is conjecture until measured.

### Idea 5. Task-conditioned tool-output pruning with a tiny model, plus reference-and-restore

What it is. Before a tool result enters the transcript, pass it with a one-line "what am I looking for" query through a 2B pruner (Squeez's released LoRA on Qwen 3.5 2B, or a rules engine for the common cases: pytest failures, grep hits, git log, ls) and keep only the returned lines plus a header with the full result's path, size and hash. The full output stays on disk; a `restore <hash> [lines]` tool re-fetches it. Screenshots get the same treatment: keep the newest one verbatim, turn older ones into their path.

Evidence. Squeez: 0.86 recall at 92% compression, 11 recall points over zero-shot Qwen 3.5 35B, and heuristics (head, tail, BM25) reach only 0.05-0.22 recall because "relevant lines may occur at the beginning, middle, or end". theinfinity.dev: file reads have a median of 2,085 tokens but a p90 of 11,401; bash is 64% of tool results; 45% of read cost was re-reading files just edited. Manus: restorable compression. Anthropic: tool-result clearing is the safest compaction. SWE-agent: a 100-line window beat whole files. Screenshots: 2,054 tokens median in Claude Code; on Qwen3-VL-class encoders a 1920x1080 image is 2,027 tokens and a 900x900 one about 793, so the ~800 already measured on irene is a downscaled frame; a full-resolution one would cost 2.5x more.

Expected gain, with arithmetic. If the average observation is 2k tokens and pruning keeps 10%, uncached prefill per round drops from ~2,000 to ~200 tokens, saving 4.5 s per round or 7.5 minutes per 100 rounds. The bigger effect is growth: 100 rounds x 2k = 200k tokens (overflow) versus 100 x 200 = 20k, which keeps the whole job inside the window and may make Idea 1's masking rare.

What could go wrong with a 27B model. A second model on the card: ~2-3 GB VRAM and ~1 s per call at 2B; on a single card it also competes with the main model's decode. Pruning removes the line the model needed and the model re-reads (Squeez's residual errors are "adjacent over-selection", which is the safe direction). Structured outputs such as JSON must bypass the pruner. The 27B model must be taught the restore tool or it will re-run the original command.

How to measure in one night. Replay 200 logged tool outputs from past runs through the pruner: tokens before and after; then rerun 10 tasks live and count re-reads, restore calls, rounds, and solve count against the unpruned baseline.

Status. Recall and compression are proven; the downstream effect on task success is explicitly unmeasured in the paper (and in the memory-condensation study, observation masking of whole results was safe, which is the coarse version of this).

### Idea 6. Stop-and-reroute: cheap runaway detection feeding a retry ladder

What it is. Three guards that run on the token stream and the event log, and a ladder they feed. Guards: (a) verbatim tail repeat over the last 250 characters plus word-trigram near-duplicate check plus low-novelty stall, abort the request and re-sample; (b) per-round output budget (n_predict per request) sized by the expected action, with a sequence-aware DRY sampler only if (a) proves insufficient; (c) a doomed-run score from the ten smallest logprobs of the step, chain-of-thought length, and token overlap with the previous step. Ladder on failure: retry at a different temperature, then re-prompt with the error text and the verifier's line, then hand the sub-task to a bigger model over the existing tunnels, then stop the job and record why.

Evidence. prime-agent #1029: a 19.8k-token "The the the" loop with no guard; the three-signal detector had zero false positives on 13.5k non-loop blocks. AgentStop: on Qwen3-30B-A3B agents through llama.cpp, a logprob/length/overlap classifier cut wasted energy 15-20% at under 5% utility loss; failed runs used 24.5k tokens and 544 s versus 15.1k and 275 s for successes. Workflow-aware serving: the escalation ladder recovered 84% of forced failures versus 55% for flat retry at +37% latency. smallcode: temperature-varied retries and "patch spiral" detection. Qwen model card: non-thinking mode wants presence_penalty 1.5; a search result recorded QwQ looping worse with naive repetition penalty until sampler order was fixed.

Expected gain, with arithmetic. At 65 tok/s a 20k-token runaway costs 5 minutes; the tail-repeat guard fires within ~60 tokens, about 1 s. If AgentStop's numbers transfer, 15-20% of total generation time on failed runs is recovered, and the job ends with a recorded reason instead of a timeout (LHTB: 79% of long-run failures are timeouts mid-work).

What could go wrong with a 27B model. False stops on legitimately repetitive output (long diffs, tables, JSON); protect tool-call blocks from the repeat guard and set DRY sequence breakers if DRY is used. The logprob classifier needs labelled local runs; the event log already has the labels once the verifier exists. Escalating to a bigger model changes the prefix for that sub-task, so it must run in its own slot or after a rebuild.

How to measure in one night. Replay one night of logs: count runaway events and the tokens they burned; then run live with guard (a) and (b) on, then the ladder; report tokens wasted, jobs recovered, false stops.

Status. Guards (a) and (b) are proven in production harnesses; the classifier (c) is proven on a 30B local model in one paper; the ladder is proven once. The combination is conjecture.

### Idea 7. Speculation tuned for the agent's own workload: MTP plus n-gram lookup, judged by acceptance, not tokens per second

What it is. Keep draft-mtp, but log acceptance per turn and treat it as a first-class metric; add an n-gram mode (`--spec-type draft-mtp,ngram-mod` on the CLI, not in a router preset) so that when the model re-emits lines it just read (patch-first edits copy from the file in context), drafts come from the prompt for free; set `--spec-draft-n-max 2` on a 24 GB card; run the actor in non-thinking mode at the card's recommended temperature 0.7 or lower for actions, since acceptance falls above 0.7; check `GGML_CUDA_DISABLE_GRAPHS=1` once.

Evidence. qwen38-mtp: on Qwen3.8-27B, +33% on a 3090 to +102% on a 5090 with n-max 2; code prompts accept 0.75-0.90 versus 0.40-0.60 for prose; the gain is single-stream and gone by `--parallel 4`. dev.to: acceptance under 0.6 makes MTP slower; 4 to 2 draft tokens moved acceptance over 0.7 and gave 1.7x. llama.cpp docs: ngram-mod is 16 MB, made for "iterating over a block of text/code". Issue #24507: CLI flags combine the two; router mode keeps only the last.

Expected gain, with arithmetic. Current 60-70 tok/s is consistent with MTP already on for a 3090-class card (31 to 41) or off for a faster one. Every 1,000 generated tokens at 65 tok/s is 15 s; at the 3090-measured +33% it is 11.6 s; on diff-heavy turns where 60% of lines are copies and n-gram drafts land, more. Over a 100-round job generating ~40k tokens, that is 10 minutes saved at +33% and closer to 20 at +100%.

What could go wrong. Thinking mode at temperature 1.0 hurts acceptance; prose-heavy planning turns will accept ~0.5 and can go slower than no speculation. Two slots (Idea 4) share the gain. Draft VRAM competes with checkpoints. The MTP head in a Q4 GGUF may be quantized too hard (check the repo's caveat on the head tensors).

How to measure in one night. Replay the same 20-turn transcript under four configs (no spec, mtp n-max 2, mtp n-max 3, mtp+ngram-mod); record tokens/s, acceptance from /metrics, and total wall time.

Status. MTP numbers are proven on this exact model on five GPUs; the MTP+ngram combination for edit-heavy turns is conjecture.

### Idea 8. Selective memory intervention, and "the big input is a variable, not a message"

What it is. Two related moves. First, every N rounds (8 in the paper) or on a trigger (tool error, repeated command, verifier failure), a memory pass with its own fixed prefix and slot reads the last N turns plus a small bank and emits only bank edits (knowledge: facts and paths; procedural: what failed, what fixed it) and either nothing or a reminder of at most 150 tokens that the harness appends after the newest tool result. It does not summarise the transcript. Second, for large inputs (a 40k-token log, a repo listing, a long spec) do not paste them into the transcript; put them on disk and give the model slice/search/sub-call tools over them, and let it call itself on a slice with a fresh short context.

Evidence. Proactive Memory: +8.3 points on Terminal-Bench 2.0 for Sonnet 4.5, +2.4 for Opus 4.6; showing the whole bank every step was worse than selective injection (61.5 vs 64.3 macro); the bank is edited through four constrained tool calls, not free text; Qwen3.5-27B trained as the memory agent partially transfers. Three Roles: the credential-loss loop is exactly this decay on an 8B model. Recursive Language Models: context-as-variable beat a compaction agent by a 26% median and offloading context to a file lifted Claude Code from 12% to 62% on CodeQA; Qwen3-8B improved 28% with 1,000 fine-tuning trajectories. Chroma: a 300-token focused prompt beats the same question buried in 113k.

Expected gain, with arithmetic. A memory pass is ~16k tokens of prefill for the 8-turn window (40 s) plus ~500 tokens of output (8 s), about 50 s; 12 passes per 100-round job is 10 minutes, too much at a fixed cadence on this card, so use the trigger schedule (the paper lists tool errors, failed tests, repeated commands as valid triggers), which should fire ~3-5 times per job, about 3-4 minutes. The reminder itself costs under a second. For big inputs: a 40k-token log pasted in costs 100 s of prefill once and then rides in the context forever; slicing costs a 2-4k-token sub-call each time it is needed.

What could go wrong with a 27B model. The paper's memory agent was Opus 4.6; a prompted 27B model injecting noise every few rounds could hurt (their always-inject ablation was only neutral with a frontier model). Keep reminders rare, short, and grounded (quote the bank entry). The sub-call pattern needs a fresh short prefix per call, which is cheap but must not evict the main slot. The bank must be harness-validated (Idea 3's typed events).

How to measure in one night. Count repeated failed commands and re-reads of the same file per job (the procedural bank's purpose) with and without the trigger-scheduled memory pass; count reminders emitted and rounds; run the same job with one large input pasted versus offloaded and compare prefill seconds and solve.

Status. Proven with a frontier memory agent and partially with a trained 27B; a prompted 27B is conjecture. Context-offloading of big inputs is proven across two independent scaffolds.

---

## Part 4. Which five I would put money on, in order

1. Idea 2 (checkpoint verification) because it is a prerequisite: if the current build is re-prefilling, every other number is wrong, and the fix is a flag and a log grep.
2. Idea 1 (sawtooth cache-shaped context) because two independent studies say masking is at least as good as summaries at half the cost, and the batching detail is where a hybrid model wins or loses.
3. Idea 3 (event log plus harness-owned projection plus recitation) because it is the record-keeping spine the others hang on and it makes crash-resume a 40-second prefill.
4. Idea 6 (guards plus ladder) because runaway and timeout are the two dominant measured failure modes on long runs and the guards have zero measured false positives.
5. Idea 4 (independent verifier at gates) because false "done" is a fifth of long-run failures and the independence effect is measured.

Ideas 5, 7 and 8 are real but second-order on this card: 5 needs a second model resident, 7 is mostly a tuning pass, and 8 depends on a memory agent stronger than the actor unless it is trained.

## Part 5. Things I could not verify

- OpenAI's harness-engineering post itself (403). The ARC-AGI-3 and 6x numbers come from secondary write-ups.
- The three vLLM-side serving papers (stateful inference, SmoothAgent, on-device adaptive context) were read through machine summaries only; I did not trust their numbers enough to build an idea on them.
- Claw-SWE-Bench and Lita: the PDFs did not yield readable result tables through the fetch tool.
- Whether llama-server's /slots save format includes hybrid checkpoints in the build on irene; one search result says it did not at some point.
- Per-checkpoint VRAM for Qwen3.8-27B specifically; the 150 MiB figure is Particula's for a different hybrid.

Report path: /tmp/claude-1000/-home-jared/09f06b9f-a0be-401c-831d-adb99bc858bb/scratchpad/analysis/web-ideas.md
