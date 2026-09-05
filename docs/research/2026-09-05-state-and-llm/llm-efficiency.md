# Token-efficient, fast agent loops: what the last 18 months say, and what Nerd Genie should do about it

Written 2026-09-05. Every claim below has a dated source in the list at the end. Where I could not verify a number in the source's own text, I say so.

A few words used throughout. A **token** is a piece of text about the size of a short word. The **prompt** is everything sent to the model in one call. **Prefill** is the model reading the prompt; **generation** is the model writing its reply. The **KV cache** (key-value cache) is the model's scratch memory of a prompt it has already read; if the next prompt starts with the same tokens, that work can be reused, and that reusable start is the **prefix**. The **context window** is the most tokens a model can take in one call.

Two Nerd Genie numbers to keep in mind, from `QWEN_BENCHMARK.md`: the local Qwen 3.8 27B reads about 400 tokens a second and writes about 55 a second. Reading a 20,000-token prompt cold costs about 50 seconds; one output token costs as much time as about seven input tokens.

The most important thing I found: Qwen 3.8 27B is not a plain transformer. Of its 64 layers, 48 are Gated DeltaNet linear attention (a recurrent layer that keeps one running state instead of a per-token cache) and only 16 are full attention [Q1]. So llama.cpp cannot rewind its cache to an arbitrary earlier token. It can only resume from a saved **checkpoint** (a snapshot of the running state at one position). When no checkpoint sits at or before the point where the new prompt diverges, the daemon re-reads from token zero, logging "forcing full prompt re-processing due to lack of cache data (likely due to SWA or hybrid/recurrent memory)" [L3, L4, L6]. Several ideas below follow from that.

---

## 1. TOP IDEAS, ranked by likely impact

### Idea 1. Put the record's changing part where the daemon puts its checkpoint

**What.** llama.cpp now creates a context checkpoint just before the latest user message (PR merged 2026-05-25), plus one every `-cms` tokens (default 8,192) [L5, L6]. If Nerd Genie's every-turn rewrite (Work, Lessons, memory hint, budget line) sits *above* the last checkpoint, the hybrid model must re-read from an earlier checkpoint or from zero. If that rewrite is the final user message, only the rewrite plus the new results get re-read.
**Why it matters.** A full re-read of a 20-30K prompt on this card is 50-75 seconds per call. Issue #22746 shows exactly this on Qwen 3.6 27B: a 53,564-token prompt reprocessed from zero [L3]. Issue #24055 measured prompt eval falling from 12,680 ms to 2,581 ms when checkpoints worked [L4].
**How Nerd Genie uses it.** Stable prefix (rules, persona, tools, job summary, record goal and rules) as the system prompt; append-only recent messages next; the mutable tail as the last user message. Lower `-cms` toward the median turn size. Run the daemon at log level 4 in `make live` and fail the run if "forcing full prompt re-processing" appears more than once per task [L6].
**Risk.** This leans on a llama.cpp behaviour that changed twice in 2026 and regressed once [L4, L5]. Measure it. The wider rule holds everywhere: never rewrite anything above the point you want cached.
**Sources.** [L3] [L4] [L5] [L6] [Q1].

### Idea 2. Make the cost line exact, and gate `make live` on the cache-hit rate

**What.** Every backend now reports cache hits per call: llama.cpp returns `cache_n` (tokens reused) and `prompt_n` (tokens read afresh) in the response timings [L1]; Anthropic returns `cache_read_input_tokens` and `cache_creation_input_tokens` [A1]; OpenAI returns `cached_tokens` [O1]. The record header's "5.2k of them cached" can be a measured number, not an estimate.
**Why it matters.** A single changing line at the top of a prompt silently destroys the cache, and nobody notices until they read the logs. A June 2026 post traced months of full re-reads on llama.cpp to a small attribution header Claude Code put at the very start of its system prompt [L7]. Manus calls KV-cache hit rate "the single most important metric" for a production agent and names the killers: timestamps, non-deterministic JSON key order, and tool lists that change mid-task [M1].
**How Nerd Genie uses it.** Assert in the forty-step fixture that from turn three onward at least 80% of input tokens were cache reads, on all three models. Keep tool definitions byte-stable and sorted. Keep the token-cost line and the "budget left" line in the tail, never above the cache line (the design already says the budget line comes last; make the test prove it).
**Risk.** Almost none; it is measurement.
**Sources.** [L1] [A1] [O1] [L7] [M1].

### Idea 3. Cut output tokens before input tokens, and let speculation pay for the rest

**What.** Output is the slow side: 55 tokens a second versus 400 for input. Speculative decoding (a cheap guesser drafts several tokens, the big model checks them in one pass) is the one free speed-up left. llama.cpp's MTP (multi-token prediction: a draft head built into the model itself) was merged 2026-05-16 and gives 72-82% accepted drafts and about 1.85-1.9x faster generation on Qwen3.6-27B, at 2.5 GB of VRAM, a small prefill penalty, and `--parallel 1` [L8]. llama.cpp also ships model-free n-gram speculation (`--spec-type ngram-*`), which guesses from text already in the prompt, and the two can be listed together [L9].
**Why it matters.** Agents echo a lot: file contents into edits, a plan step into a done line, a command into a retry. An echoed token drafted from the prompt is nearly free.
**How Nerd Genie uses it.** Keep MTP on. Try `--spec-type mtp,ngram-map-k` and read acceptance from the timings. On the harness side: forbid re-quoting a file in a reply; prefer `edit` with short anchors over `write` of a whole file.
**Risk.** Acceptance falls as sampling temperature rises, and the daemon runs at 0.7 [L9]. MTP combined with n-gram drafting is unmeasured; treat it as an experiment.
**Sources.** [L8] [L9] and `QWEN_BENCHMARK.md`.

### Idea 4. Thinking is a dial the harness turns, at three moments only

**What.** The benchmark ran with thinking off, and Nerd Genie won. But the Qwen3 report shows thinking lifting Qwen3-32B from 63.0 to 70.3 on BFCL v3 (a tool-calling test) and from 31.3 to 65.7 on LiveCodeBench [Q2]. Against that, "The Danger of Overthinking" (4,018 agent trajectories) found that agents that reason more act less, and that picking the trajectory with the lowest overthinking score gave about 30% better results at 43% lower cost [T1]. HRBench (May 2026) found no universal switching rule; prompt-based switching gave the best token-for-accuracy trade [T3]. More budget is not the best use of compute past a point [T2].
**Why it matters.** At 55 tokens a second, a 1,500-token think is 27 seconds. Spent on "which file to read next," it is waste. Spent on the plan and the done list, it is the cheapest quality the small model can buy.
**How Nerd Genie uses it.** Thinking off by default. The harness turns it on, with a budget of a few hundred tokens, only when asking for the plan and done list, after a recorded failure, and at the done-check. Never re-send thinking text; Nerd Genie does not replay transcripts, so this is already true.
**Risk.** On llama.cpp the switch is rendered into the chat template, so it must not change anything above the cache line; on the Claude API changing thinking settings invalidates the messages cache [A1]. Put the switch in the tail.
**Sources.** [Q2] [T1] [T2] [T3] [A1].

### Idea 5. Every result gets a model-written one-line gist, and the harness stamps the time

**What.** "Context Length Alone Hurts" (EMNLP 2025) showed models lose 13.9-85% even when they can find the right text, and that the fix is to have the model recite the relevant evidence into a short space before solving [C3]. Manus rewrites a `todo.md` every step for the same reason [M1]. "The Sleeping Agent" (Aug 2026) found summaries keep names and events but throw away dates: 3.05% of time expressions survived, rising to 62.39% with one prompt change [C6].
**Why it matters.** Today the harness writes "r3 read memory/product.md, 2,100 characters." That says where a result is, not what it said. A gist turns the record into recited evidence, so a result can leave the table without its meaning leaving.
**How Nerd Genie uses it.** When a result is about to leave the table, the `task` tool asks for a `gist` of at most 25 words; the harness appends round number and clock time to every result, decision, and failure line for free.
**Risk.** About 30 output tokens per result; the small model may gist wrongly, so `read r7` stays the truth.
**Sources.** [C3] [M1] [C6] [C5].

### Idea 6. Treat big results as an environment, not as text: search and slice them by id

**What.** Recursive Language Models (Dec 2025) let the model treat a long input as a variable it can search, slice, and call itself over, instead of reading it whole; a post-trained Qwen3-8B beat its own base by 28.3% and approached GPT-5 on three tasks, and the method beat compaction by a median 26% [S1]. Anthropic's context-engineering guidance says the same in engineering terms: keep light identifiers (paths, ids) and fetch just in time [A2].
**Why it matters.** A 12,000-token web page in the window is 12,000 tokens in the worst-attended part of the prompt. `search r7 "login"` and `read r7 lines 40-90` cost a few hundred.
**How Nerd Genie uses it.** Extend `read` to take a line range and `search` to take a result id. Cap what a raw result puts on the table; keep the rest on the shelf.
**Risk.** More tool rounds per task; a 27B needs one worked example in the instructions.
**Sources.** [S1] [A2] [S3].

### Idea 7. Cap tool output at a fixed size, head and tail, and never rewrite it afterward

**What.** One team that stopped compacting reports keep-everything-with-caps beat their summarising setup on all counts: 92% memory recall versus 38%, $0.11 versus $0.24 per turn; capping outputs alone cut cost 38% without touching the cached prefix [C8]. Anthropic's context editing, which swaps old tool results for placeholders, cut tokens 84% on a 100-turn search task and raised scores 29% [A3]. TokenPilot (EMNLP 2026) makes the same point from the other side: every edit to old text breaks the prefix, so compact at ingestion and evict whole segments, never rewrite [C9].
**How Nerd Genie uses it.** The design already caps each tool's output. Make the cap deterministic (same input, same bytes), keep the first and last N lines, and point at the file for the rest.
**Risk.** A cut in the wrong place hides the one line that mattered; the pointer to the full file is the safety net.
**Sources.** [C8] [A3] [C9] [C10].

### Idea 8. Hold the working context near 32K on the 27B, whatever the 262K window says, and lay it out for the U-curve

**What.** Nobody has published a long-context study of Qwen 3.8 27B yet. The nearest evidence: Chroma's "Context Rot" (July 2025, 18 models including Qwen3-32B) found every model degrades as input grows and shuffled text scored *better* than coherent text [C1]. NoLiMa (ICML 2025) measured "effective length" (the longest input at which a model keeps 85% of its short-context score): Gemma 3 27B under 1K, Llama 3.3 70B 2K [C2]. LOCA-bench (Feb 2026) ran agents: open models went from 78.7% at 8K to 61.3% at 32K, 45.3% at 64K, and 10.7% at 128K [C4]. A 172-billion-token study of 35 open models found hallucination of 1-7% at 32K and above 10% for every model at 200K [C7]. Qwen3.8 27B at Q4 needs 20 GB of VRAM at a 32K context and 34 GB at 256K, and both prefill and generation slow as context grows [H1]. The "lost in the middle" U-curve, with the start and end attended best, is strongest when the input fills up to half the window [C11].
**How Nerd Genie uses it.** Start the daemon at 65,536 tokens, not 262,144, and give the freed VRAM to checkpoints (60-215 MiB each [L6]) and MTP. Keep the working context at or under about 32K. Rules and goal first; results in the middle, short and gisted; the open done lines and "next step" last.
**Risk.** A job with big results needs the shelf more often; that is what the shelf is for.
**Sources.** [C1] [C2] [C4] [C7] [Q2] [H1] [C11] [L6].

### Idea 9. Free text first, structure second, and never combine JSON-schema mode with tool calling on the local model

**What.** "The Format Tax" (Apr 2026, six open-weight and four API models) found JSON, XML, and Markdown requirements substantially degrade reasoning in open models, that the cause is interference in the prompt rather than the decoder, and that reasoning first and formatting second recovers most of it [F2]. "Capacity, Not Format" (Jun 2026) showed the cost tracks spare capacity: Sonnet lost nothing under JSON, Haiku lost 36.2 points, GPT-4o-mini 28.0; reason-then-format recovered 80-87% [F3]. "Constraint Tax" (Jun 2026) found that tool calling plus a JSON-schema response format together make several open models stop calling tools, because the grammar mask makes the tool-call tokens unreachable [F4]. JSONSchemaBench (Jan 2025) found constraining just the structured span can help by up to 4% [F1].
**How Nerd Genie uses it.** "First line says where the work stands, then the tool calls" is exactly reason-then-format; keep it. Never set `response_format` with tools. If a grammar is ever used, constrain only the tool-call span. Keep the loose parser and repair layer as the main path.
**Risk.** Loose parsing needs a good repair layer; it exists and is fuzzed.
**Sources.** [F1] [F2] [F3] [F4] [F5].

### Idea 10. Warm the prefix at startup and save the cache with the checkpoint

**What.** llama.cpp keeps its prompt cache in one slot and can save and restore a slot's state to disk (`--slot-save-path`, `/slots/{id}?action=save|restore`) [L1, L2]. Anthropic sells a 1-hour cache for 2x the write price and 0.1x reads; OpenAI keeps entries 5-10 minutes by default and offers a 24-hour retention option [A1, O1].
**How Nerd Genie uses it.** After the daemon starts, send the stable prefix once with `max_tokens: 1` so the first real call is warm. On a task's checkpoint, optionally save the slot; on `/tasks 17 back 3` or a three-day resume, restore it instead of re-reading 20K tokens. On cloud models, ask for the 1-hour cache when a task is waiting on the user.
**Risk.** The slots tutorial calls the flag "technically unsafe and not recommended" [L2]; hybrid state files are larger. Experimental; measure.
**Sources.** [L1] [L2] [A1] [O1].

---

## 2. FINDINGS

### 2.1 Prefix and KV-cache reuse

**How it works.** Reading a prompt produces a KV cache. If the next prompt starts with the same tokens, in the same order, the backend reuses that cache up to the first token that differs and reads only the rest. llama.cpp matches per slot, with `cache_prompt` on by default [L1]. vLLM hashes fixed-size blocks and evicts the least recently used; SGLang's RadixAttention keeps a tree of token sequences and branches at the first difference [V1, V2]. Anthropic caches in the order tools, then system, then messages: change a tool and everything after it is lost; change the system prompt and the messages are lost [A1]. OpenAI caches "the full rendered context," tools included, in 128-token steps [O1].

**Does sending the same prompt twice make it cheaper?** Yes, with conditions. On llama.cpp the second read costs only the differing suffix if it lands in the same slot with an identical token prefix; on the hybrid Qwen 3.8, also only if a checkpoint covers the divergence point, otherwise the whole prompt is re-read [L1, L3, L6]. On Anthropic the prompt must be at least 1,024 tokens (512 on the newest models), the first send costs 1.25x (5-minute) or 2x (1-hour), and each later read costs 0.1x; up to four breakpoints; parallel first requests miss each other's writes [A1]. On OpenAI caching is automatic above 1,024 tokens (2,048 on older models), reads are 0.1x on the newest models, and entries live about 5-10 minutes by default [O1].

**What breaks it.** Anything that changes bytes above the reuse point: a timestamp, a token count, a changed tool description, a reordered JSON key, an added image, a changed `tool_choice`, a changed thinking setting, or a summary that rewrites history [M1, A1, L7]. Manus's answer is to keep tool definitions fixed and mask the unavailable ones at decode time instead of removing them [M1]. Nerd Genie shows all eighteen tools always, which is the same effect.

**Hybrid models are different.** Recurrent layers keep one running state that cannot be cut back to an earlier token [L10]. llama.cpp answers with checkpoints: one before the latest user message and one every 8,192 tokens by default, up to 32 kept, each 60-215 MiB [L5, L6]. This machinery was added in May 2026 and regressed in June [L4, L5]. The practical guide's first fix is "stop mutating the prefix between turns" [L6].

**KV-cache memory.** Google's TurboQuant (ICLR 2026, blog Mar 2026) compresses the KV cache to 3 bits with no measured accuracy loss and at least 6x less memory [K1]; the daemon on this machine is a TurboQuant build, which is why 262K contexts fit at all.

### 2.2 Context compression: what is lost, what survives

The field tried summarising and is backing away from it. Anthropic's guidance (Sept 2025) describes Claude Code's compaction as keeping decisions and bugs and dropping tool outputs, and prefers just-in-time retrieval and structured notes [A2]; its context editing removes old tool results rather than rewriting them [A3]. Cognition (June 2025) argues for a single thread with a dedicated compression model rather than parallel agents [C10]. Manus keeps failures in context on purpose [M1].

The measurements: TRACE (Aug 2026), which tests OpenClaw's and Hermes's own summary templates on AppWorld, found summaries make agents lose track of where they are: at a 2K budget, summary-conditioned agents ended the task correctly 44.6% of the time against 77.2% with plain truncation, and "retaining the raw update yields the smallest divergence" [C5]. "The Sleeping Agent" found summaries keep entities and events but lose dates [C6]. Active Context Compression (Jan 2026) let the agent decide when to consolidate and cut tokens 22.7% with no accuracy change [C12]. SWE-Pruner (Jan 2026) uses a 0.6B skimmer to drop irrelevant code lines, saving 23-54% of tokens while improving success [C13]. Mem0 (Apr 2025) saves over 90% of tokens versus full context on long conversations [S4].

All of this supports Nerd Genie's shape: a fixed-format record beside an untouched log, results shelved not rewritten, recent results kept verbatim.

### 2.3 Long-context degradation, especially for 20-30B open models

See Idea 8 for the numbers. Three mechanisms recur: attention spread thin as tokens grow [A2]; the U-shaped position bias, strongest below half the window [C11], which a 2025 paper argues emerges from how retrieval is demanded in training [C14]; and distractors, which hurt more at length [C1]. Retrieval tests such as RULER overstate ability (Qwen3-32B scores 94.4 at 32K and 85.6 at 128K [Q2]); NoLiMa, which removes word overlap between question and answer, shows effective lengths of 1-2K for 27-70B open models [C2]. For agents, LOCA-bench shows open models at a third of frontier accuracy by 64K and names the failures: missed constraints, impatience, and values retrieved correctly then distorted later [C4]. No paper has yet measured Qwen 3.8 27B; a search summary claimed the Qwen3.5 27B "retains 0.77 of its accuracy at the longest context versus 0.65 at 9B," but I could not find that line in the paper's abstract, so treat it as unverified.

### 2.4 Structured and JSON output on small models

The 2024 "Let Me Speak Freely" paper first showed format restrictions cut reasoning and that parse failures were not the cause [F5]. 2025-2026 work refined it: constrained decoding of the structured part alone can help [F1]; the damage in open models comes from the format demand competing with reasoning, and reasoning first then formatting recovers most of it [F2, F3]; and combining schema mode with tool calling can silence tool calls entirely in open models [F4]. XGrammar-2 (Jan 2026) targets agents by switching grammars mid-reply and caching sub-grammars [F6]; a vendor post claims it lets a 3B model beat an unconstrained 70B on BFCL, which I could not verify in the paper's abstract.

### 2.5 Speculative decoding and multi-token prediction

Speculative decoding drafts several tokens cheaply and verifies them in one pass; output is identical to normal decoding. llama.cpp offers draft models, EAGLE-3 style heads, MTP, and prompt-lookup n-grams, and reports an acceptance rate per call [L9]. MTP on Qwen3.6-27B: 72-82% acceptance, 1.85-1.9x decode, 2.5 GB VRAM, single sequence only, slight prefill penalty [L8]. Acceptance is highest for code and repeated text and falls with temperature [L9, L11].

### 2.6 Reasoning budget

Qwen3 introduced the thinking budget and shows smooth gains with more budget on math and code [Q2]; Qwen 3.8 ships with reasoning on by default at adjustable effort [Q1]. But agentic tasks punish overthinking [T1], extra budget is not the best use of compute [T2], and the right switching policy depends on model and task [T3]. The harness, which knows what kind of step it is asking for, is the right place to set the dial.

### 2.7 Agents that keep state outside the window

MemGPT's idea (a small in-context core plus an out-of-context archive the model pages from) became Letta; Letta's sleep-time compute (Apr 2025) uses idle time to pre-think about stored context, cutting test-time compute about 5x when the next question is predictable [S2]. Anthropic's memory tool (Sept 2025) is a file directory the model reads and writes across sessions [A3]. "Everything is Context" (Dec 2025) mounts all context artefacts as a governed file system [S3]. Recursive Language Models treat the prompt itself as an external variable [S1]. Claude Code 2.0 (Sept 2025) added checkpoints you can rewind to [S5]. Nerd Genie's record is the MemGPT core block with a fixed schema; its log is the archive; its checkpoints are the save file. What is still new is that the harness, not the model, writes most of the record and enforces its shape.

---

## 3. SOURCES

**llama.cpp**
- [L1] llama-server README, `tools/server/README.md` (master, read 2026-09-05). https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md
- [L2] Tutorial: KV cache reuse with llama-server, Discussion #13606, 2025-05-17. https://github.com/ggml-org/llama.cpp/discussions/13606
- [L3] Issue #22746, Qwen 3.6 27B forcing full prompt re-processing, 2026-05-06. https://github.com/ggml-org/llama.cpp/issues/22746
- [L4] Issue #24055, context checkpoints always invalidated on hybrid/recurrent models, 2026-06-03. https://github.com/ggml-org/llama.cpp/issues/24055
- [L5] PR #22929, checkpoint before the latest user message, merged 2026-05-25. https://github.com/ggml-org/llama.cpp/pull/22929
- [L6] Particula, "Full prompt re-processing: llama.cpp cache fixes that work," July 2026. https://particula.tech/blog/prompt-reprocessing-swa-hybrid-models-kv-cache
- [L7] M. Aleksandrov, "Claude Code, llama.cpp, and the hidden prompt cache killer," 2026-06-21. https://www.mykolaaleksandrov.dev/posts/2026/06/claude-code-llamacpp-prompt-cache-fix/
- [L8] PR #22673, MTP speculative decoding, merged 2026-05-16. https://github.com/ggml-org/llama.cpp/pull/22673
- [L9] `docs/speculative.md` (master). https://github.com/ggml-org/llama.cpp/blob/master/docs/speculative.md
- [L10] Discussion #19264, partial cache reuse for recurrent models, 2026-02-02. https://github.com/ggml-org/llama.cpp/discussions/19264
- [L11] InventiveHQ, llama.cpp speculative decoding on consumer GPUs, 2025. https://inventivehq.com/blog/llama-cpp-speculative-decoding-consumer-gpu

**Vendor caching and context management**
- [A1] Anthropic, Prompt caching docs (read 2026-09-05). https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- [A2] Anthropic, "Effective context engineering for AI agents," 2025-09-29. https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- [A3] Anthropic, "Managing context on the Claude Developer Platform," 2025-09-29. https://claude.com/blog/context-management
- [O1] OpenAI, Prompt caching guide (read 2026-09-05). https://developers.openai.com/api/docs/guides/prompt-caching
- [V1] vLLM, Automatic Prefix Caching design doc. https://docs.vllm.ai/en/v0.9.2/design/automatic_prefix_caching.html
- [V2] D. Moon, "Prefix caching: SGLang vs vLLM," 2025. https://medium.com/byte-sized-ai/prefix-caching-sglang-vs-vllm-token-level-radix-tree-vs-block-level-hashing-b99ece9977a1
- [M1] Y. Ji (Manus), "Context engineering for AI agents: lessons from building Manus," 2025-07-18. https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- [K1] Google Research, "TurboQuant," 2026-03-24 (ICLR 2026). https://research.google/blog/turboquant-redefining-ai-efficiency-with-extreme-compression/

**Long context and compression**
- [C1] Hong, Troynikov, Huber (Chroma), "Context Rot," 2025-07-14. https://www.trychroma.com/research/context-rot
- [C2] Modarressi et al., "NoLiMa," arXiv 2502.05167, Feb 2025 (ICML 2025); results table at https://github.com/adobe-research/NoLiMa
- [C3] Du et al., "Context Length Alone Hurts LLM Performance Despite Perfect Retrieval," arXiv 2510.05381, 2025-10-06 (EMNLP 2025 Findings). https://arxiv.org/abs/2510.05381
- [C4] Zeng, Huang, He, "LOCA-bench," arXiv 2602.07962, Feb 2026. https://arxiv.org/html/2602.07962v1
- [C5] Min et al., "Toward Reliable Context Compression for Long-Horizon Agents" (TRACE), arXiv 2608.06503, 2026-08-06. https://arxiv.org/html/2608.06503v1
- [C6] Kyrkewood, "The Sleeping Agent: What Gist-Based Context Compression Loses and Why," arXiv 2608.11775, 2026-08-12. https://arxiv.org/abs/2608.11775
- [C7] Roig, "How Much Do LLMs Hallucinate in Document Q&A?" arXiv 2603.08274, 2026-03-09. https://arxiv.org/abs/2603.08274
- [C8] L. Bouchard, "Context Engineering in 2026: Why We Stopped Compacting Our Agent's Context," 2026-08-18. https://www.louisbouchard.ai/context-engineering-2026/
- [C9] Xu et al., "TokenPilot: Cache-Efficient Context Management for LLM Agents," arXiv 2606.17016, June 2026 (EMNLP 2026 Findings). https://arxiv.org/abs/2606.17016
- [C10] W. Yan (Cognition), "Don't Build Multi-Agents," June 2025. https://cognition.com/blog/dont-build-multi-agents
- [C11] Veseli et al., "Positional Biases Shift as Inputs Approach Context Window Limits," arXiv 2508.07479, 2025-08-10. https://arxiv.org/abs/2508.07479
- [C12] Verma, "Active Context Compression," arXiv 2601.07190, 2026-01-12. https://arxiv.org/abs/2601.07190
- [C13] Wang et al., "SWE-Pruner," arXiv 2601.16746, 2026-01-23. https://arxiv.org/abs/2601.16746
- [C14] "Lost in the Middle: An Emergent Property from Information Retrieval Demands in LLMs," arXiv 2510.10276, Oct 2025. https://arxiv.org/abs/2510.10276

**Structured output**
- [F1] Geng et al., "Generating Structured Outputs from Language Models: Benchmark and Studies" (JSONSchemaBench), arXiv 2501.10868, 2025-01-18. https://arxiv.org/html/2501.10868v1
- [F2] Lee, D'Antoni, Berg-Kirkpatrick, "The Format Tax," arXiv 2604.03616, 2026-04-04. https://arxiv.org/abs/2604.03616
- [F3] Fan, "Capacity, Not Format," arXiv 2606.09410, 2026-06-08. https://arxiv.org/abs/2606.09410
- [F4] Li, Zhang, Lv, "Constraint Tax in Open-Weight LLMs," arXiv 2606.25605, 2026-06-24. https://arxiv.org/abs/2606.25605
- [F5] Tam et al., "Let Me Speak Freely?" arXiv 2408.02442, Aug 2024 (EMNLP 2024). https://arxiv.org/abs/2408.02442
- [F6] Li et al., "XGrammar-2," arXiv 2601.04426, 2026-01-07. https://arxiv.org/abs/2601.04426

**Thinking budget**
- [T1] Cuadron et al., "The Danger of Overthinking," arXiv 2502.08235, Feb 2025. https://arxiv.org/abs/2502.08235
- [T2] Iacobacci et al., "Increasing the Thinking Budget is Not All You Need," arXiv 2512.19585, 2025-12-22. https://arxiv.org/abs/2512.19585
- [T3] Ning et al., "HRBench," arXiv 2605.28398, 2026-05-27. https://arxiv.org/abs/2605.28398

**State outside the window**
- [S1] Zhang, Kraska, Khattab, "Recursive Language Models," arXiv 2512.24601, 2025-12-31 (v. 2026-05-11). https://arxiv.org/abs/2512.24601
- [S2] Lin et al. (Letta), "Sleep-time Compute," arXiv 2504.13171, 2025-04-17. https://arxiv.org/abs/2504.13171 and https://www.letta.com/blog/sleep-time-compute/
- [S3] "Everything is Context: Agentic File System Abstraction," arXiv 2512.05470, 2025-12-05. https://arxiv.org/abs/2512.05470
- [S4] Chhikara et al., "Mem0," arXiv 2504.19413, Apr 2025. https://arxiv.org/abs/2504.19413
- [S5] Claude Code checkpointing docs (feature shipped 2025-09-29). https://code.claude.com/docs/en/checkpointing

**Qwen**
- [Q1] MindStudio, "Qwen3.8-27B explained: hybrid attention, 262K context," 2026-08-15. https://www.mindstudio.ai/blog/qwen3-8-27b-architecture-benchmarks
- [Q2] Qwen Team, "Qwen3 Technical Report," arXiv 2505.09388, May 2025. https://arxiv.org/html/2505.09388v1
- [H1] Hardware Corner, "We tested Qwen3.8 27B," 2026-08-17. https://www.hardware-corner.net/qwen3-8-27b-hardware-tests/
