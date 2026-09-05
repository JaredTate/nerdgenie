# Agent harness design for reliability: what 2025-2026 research says, and what Nerd Genie should take from it

Written 2026-09-05 for the Nerd Genie team. Every source has a link and a date. Where a claim could not be verified, it says so.

A few words first. A **harness** is the program around the model that decides what the model reads and runs the tools it asks for. The **context** (or **context window**) is the text the model reads on one call. A **token** is a piece of text about the size of a short word. A **tool call** is the model asking the harness to run a tool. Papers often say **scaffold** for harness.

The live run that prompted this report: the local Qwen 3.8 27B built a Tetris game with 62 passing tests in 55 minutes, then a browser tool said a click had worked when it had not. With no true signal, the model re-read the same file four times and the repeat guard stopped the task. Three findings speak straight to that. Tools lie by omission, and only a check of the world catches it (idea 1). A model that can see its own failed attempts is more likely to fail again, and this does not go away with model size (idea 2). The cheapest stuck detectors measure progress, not repetition (idea 3).

## 1. Top ideas, ranked by likely impact

### 1. Never believe a tool. Check the state.

**What.** A tool that changes the world (click, type, write, shell) must return proof of the change, not a status word. The harness compares the world before and after (a page diff, a file hash, a test count) and writes "no change" as a failure whatever the tool said.

**Why.** "False success", an agent declaring done when the environment says otherwise, was 45-48% of failures on tau2-bench and 75.8% on AppWorld coding runs. Model judges could not spot it (best AUROC 0.65, barely better than a coin) because they read confident language, not state [S1]. Wrapping tool calls with post-condition checks (did the intended effect happen?), verify-before-retry, and idempotency keys (a tag that stops the same action running twice) cut duplicate actions with no loss of success [S2]. Read-only "deterministic gates" that inspect a proposed call against the current state before any write raised success from 29.6% to 42.0% [S3].

**How Nerd Genie uses it.** The design already has act-and-assert for the browser (§9). Make it a rule for every W, X, and N tool: the result line the harness writes carries a harness-computed delta ("page unchanged", "0 files changed", "tests 62/62 -> 62/62"). A click with no visible change returns a failure and a screenshot, never "ok". The done-check treats a line whose only proof is a tool's own claim as unproven.

**Risk.** More bytes per result; some effects are invisible (a network side effect). Cost is small: the browser worker already snapshots.

### 2. When stuck, rewind the context (and the files), and keep only the lesson.

**What.** Models "become more likely to make mistakes when the context contains their errors from prior turns", and this **self-conditioning** "is not mitigated by scaling model size" [S4]. So on a stuck signal the harness should not only stop. It should drop the looping results from the table, write one failure line with its cause into the record, and call once more with the clean context.

**Why.** Rolling back and re-running recovered 45% of failed runs, against 16% for plain re-sampling [S5]. In code agents, 60-69% of failures came after the agent had already reached the right code and then thrashed it; an edit-commit checkpoint recovered every case where the agent had once produced the correct patch [S6]. Vending-Bench meltdowns did not track a full context; they began with a misread of state and then a loop [S7].

**How.** The repeat guard becomes a ladder: first rewind (drop the last k results from the window, add a failure line, keep the record), then stop if the loop returns. Add file checkpoints: a git commit after every green test run, so thrash-after-success reverts to the last green state, as Anthropic's long-running harness does [S8].

**Risk.** A rewind cycle is itself a loop; allow one rewind per stuck event. Note the honest tension: Manus says "keep the wrong stuff in" so the model can adapt [S19]. The record's one-line failure keeps the evidence; the rewind removes the repetition.

### 3. A progress meter and a stuck ladder, not only a repeat guard.

**What.** Detect "no progress" as well as "same call". OpenHands' detector, on by default, catches four identical action-observation pairs, three identical action-error pairs, three model messages with no user input, and six alternating A-B-A-B pairs, comparing by content rather than by id [S9]. A static scan of 6,549 agent repositories confirmed 68 infinite loops in 47 projects, all from feedback paths with no effective bound [S10].

**Why.** The Tetris loop, four reads of one file, should trip an identical-arguments guard, but reads at slightly different offsets, or alternating read/test pairs, slip past. Progress is something the harness can measure for free: a done line proven, a plan step marked, a file changed, a test count changed.

**How.** Keep a harness-computed "rounds since last progress" in the situation block. Ladder: at N rounds, one line back ("no progress in 6 rounds; the last 3 results were the same file"); at 2N, rewind (idea 2); at 3N, stop and ask, which is what agents do too late on their own [S11]. Vary the wording of the nudge each time, because a uniform context invites more of the same [S19].

**Risk.** OpenHands users hit false alarms when an agent legitimately polls a long command [S12]; exempt `shell` polls of a running process id.

### 4. Turn on thinking for the local model at the orient step.

**What.** The self-conditioning paper found "thinking mitigates self-conditioning" and that recent thinking models "do not self-condition and can execute much longer tasks in a single turn" [S4]. Thinking mode is a private reasoning pass before the answer; Qwen 3.x switches it per request.

**Why.** Nerd Genie already asks the model to orient before acting. The evidence says a short reasoning step before the tool call is the one lever that breaks the loop-feeds-loop effect in small models. HAL warns the effect is not monotonic: "higher reasoning effort reduc[ed] accuracy in the majority of runs" across 21,730 rollouts [S13], so use a modest budget.

**How.** Enable thinking through the chat-template flag, not sampling (the daemon's temperature stays baked). Cap thinking tokens. A/B it in the forty-round fixture: thinking on the first call of a turn and after any failure line, off otherwise.

**Risk.** Latency on one GPU; thinking text must be stripped before logging the reply.

### 5. Constrained decoding for tool calls: prevent bad calls instead of repairing them.

**What.** **Constrained decoding** is a grammar that makes the model unable to emit a token that would break the format, so a tool call is always well-formed JSON in the right schema. llama.cpp accepts a grammar or JSON schema per request, so no daemon change is needed.

**Why.** Secondary write-ups of XGrammar-2 report a constrained Llama-3.2-3B beating an unconstrained Llama-3.1-70B on the BFCL function-calling benchmark, mostly by removing malformed calls; that number is not in the paper's abstract [S14], so treat it as plausible, not proven. The counter-finding is verified: open-weight models show **tool suppression**, ceasing to call tools when tool-calling and JSON-schema constraints are on at once, because the grammar mask makes tool-call tokens unreachable [S15].

**How.** Constrain only the tool-call block, never the whole reply, and keep the repair layer as fallback. Measure the bad-call rate on the fixture before and after.

**Risk.** The constraint tax above; a grammar can also hide the model's confusion instead of surfacing it.

### 6. Mask tools by phase; show the small model fewer tools.

**What.** Fewer tools in view means better selection. "Less is More" cut execution time up to 70% and raised success by limiting tools dynamically on edge hardware [S17]. RAG-MCP measured selection accuracy falling as the tool pool grows and more than tripled it (13.6% to 43.1%) by showing only relevant tools [S18]. Manus masks token probabilities instead of removing tools, to keep the prompt cache warm [S19].

**How.** Keep all eighteen definitions in the cached prefix, but mask by phase: no browser tools until the plan names a page; no `computer` unless enabled; no `job` inside a job's task. Masking can be a harness reject-with-hint or a decoding grammar (idea 5).

**Risk.** A wrong mask blocks a needed tool; every mask needs an "ask for tool X" path. Hammer found small models are misled by tool names and generalize better when forced to read descriptions [S20]; keep names distinct and descriptions plain [S21].

### 7. Done means an end-to-end proof, not green unit tests.

**What.** 62 passing tests did not make Tetris playable. Strengthening tests on SWE-bench dropped the top agent from 78.8% to 62.2%, and "one in five 'solved' patches from the top-30 agents are semantically incorrect" [S22]. Anthropic's long-running harness keeps a JSON feature list marked passing or failing and requires browser-driven end-to-end checks before a feature flips [S8].

**How.** Split proof classes on the done list. A line about behavior the user sees ("the piece moves left on ArrowLeft") needs an end-to-end result: a `browser_act` with an asserted diff or screenshot. A unit test proves only its own line. The `task` tool refuses to mark a user-facing line done with a unit-test result id.

**Risk.** End-to-end checks are slow and flaky; cap retries and fall back to a handoff screenshot.

### 8. Sub-step workers with a fresh, tiny context.

**What.** Give a well-defined step ("make the ArrowLeft test pass") its own call holding only the goal, that step, and the one result it needs; return one report line to the record. Cognition's 2026 update: the setups that work share one property, "one main loop carries state, with subagents as stateless workers with narrow scope" [S24]. Anthropic's research system uses an orchestrator with workers in separate contexts [S25]. Plan-and-Act ran its executor on an 8B model [S26].

**Why.** This is Breunig's **context quarantine** [S27]: the loop cannot poison what it cannot see. Small models gain most because their window is smallest.

**How.** The main loop stays the manager and owns the record; a worker call is a plain model call with no record access, and its text becomes a result id.

**Risk.** The worker may lack context it needed. Planner/executor splits helped on hard tasks and hurt on easy ones, and models under 10B "are still unable to solve tasks consistently regardless of the approach" [S28]. More calls, but each is short.

### 9. Recite the next step as the last line of the prompt.

**What.** Manus rewrites a todo.md at the end of context on every step to pull the goal into recent attention, on tasks averaging about 50 tool calls [S19]. "Lost in the middle" showed models read the start and end of a context best [S29].

**How.** Nerd Genie already puts work and lessons in the tail. Add a harness-written final line: "Step 3 of 5: post it. Last result r9: page unchanged. Next: confirm the compose box exists." Cheap and cache-safe.

**Risk.** Practitioner evidence only; no controlled study found. About twenty tokens.

### 10. Verbatim beats extraction: make memory an index over raw text.

**What.** A controlled ablation found raw conversation chunks beat model-extracted facts by 15.9 points on LoCoMo and 22.0 on LongMemEval; "lossy distillation, not structure per se" causes the gap, and structure should "augment verbatim text rather than replace it" [S30]. ACE found iterative rewriting causes **context collapse** and keeps itemized bullets with incremental updates instead [S31].

**How.** This supports the record's never-summarize rule. Extend it across tasks: every MEMORY.md fact keeps a pointer to its raw log event, and memory search returns the raw span, not only the fact.

**Risk.** Memory is not free help. A 2026 reliability study found a free-form scratchpad injected into every prompt "never helps" on long tasks across ten open models [S32]. That was notes, not a structured record, but it says: measure, do not assume.

### 11. Keep a "traps" memory from failures, gated by later outcomes.

**What.** ReasoningBank stores strategies from successes and failures as title, description, and content, retrieved by similarity; Google reports +8.3% success on WebArena, +4.6% on SWE-bench Verified, and about three fewer steps per task [S33]. Reflexion is the 2023 ancestor [S34].

**How.** The after-action review already saves one lesson. Add a trap type: "browser click on a canvas game reports success with no DOM change; verify by screenshot diff". Retrieve traps by tool and site. Drop a trap that a later task marks wrong, because retrieved experiences propagate their errors [S35].

**Risk.** Bad traps poison future tasks; keep them few and dated.

## 2. Findings

### Memory architectures

CoALA [S36] gives the vocabulary: **working** memory (this call), **episodic** (what happened), **semantic** (facts), and **procedural** (how to do things). Nerd Genie's log, record, MEMORY.md, and skills map onto those four. MemGPT [S37] treats the model as an operating system paging between a fixed main context and archival storage; Anthropic's memory tool and context editing productized that, with context editing cutting tokens 84% on a 100-turn search task [S38]. Generative Agents [S39] score memories by recency, relevance, and importance and periodically write higher-level reflections. Voyager [S40] stores skills as executable code found by description; Anthropic's Agent Skills [S41] are folders with a SKILL.md loaded on demand, about 80 tokens each at discovery. A-MEM [S42] links notes and revises old ones. Agent Workflow Memory and Dynamic Cheatsheet [S43] store reusable procedures found during use. Honest summary: structured, itemized, incrementally updated, verbatim-backed memory has the best evidence [S30, S31]; free-form notes have mixed-to-negative evidence at long horizons [S32]; stored experiences can propagate errors [S35].

### Why agents loop, and what stops it

Four mechanisms have evidence. Self-conditioning: errors in context breed errors, not fixed by size, eased by thinking [S4]. **Context rot**: 18 models including Qwen3-32B and 8B degraded with input length even on trivial copying; distractors hurt more as context grew; focused excerpts beat 113k-token full histories on LongMemEval [S44]. Breunig's four failure modes, poisoning, distraction, confusion, and clash, with quarantine, pruning, and offloading as fixes [S27]. State misreads: Vending-Bench agents believed an order had arrived when it had not, then looped or quit, "regardless of context window size" [S7]. Interventions with numbers: rollback-and-rerun [S5], edit-commit checkpoints [S6], deterministic gates [S3], OpenHands' pattern detector [S9], and static bounds on feedback paths [S10]. Semantic early stopping halts a loop when drafts stop changing in meaning and found the harder question was "which round was best", an argument for checkpoints [S45]. Agents also stop too late on their own [S11].

### Planning shapes and small models

ReAct (think, act, observe, each step) generally beats plan-and-execute on success because it adapts, at about 35% more input tokens; planner/executor splits pay off on hard tasks and when a cheap executor runs many steps [S28, S26]. NVIDIA's position paper argues most agent steps are narrow, repeated, format-bound work suited to 3-10B models, with a big model only for open-ended steps [S46]. Anthropic and Cognition converge on one stateful loop plus stateless workers [S25, S24]. Checklists: extracting a checklist from the instruction and scoring each item with judges and verifier programs beat reward models on every benchmark tested [S47], which supports the done list as the unit of proof. Tree of Thoughts and LATS search over branches [S48]; no controlled evidence was found that they help 7-30B models on agent tasks, though Fork/Explore/Commit shows the operating-system side (copy-on-write workspaces, branches under 350 microseconds) is ready [S49].

### Benchmarks and the "harness matters" argument

SWE-bench Verified: mini-swe-agent, about 100 lines with bash only, scores over 74% [S50]; Agentless beat the agents of its day with a fixed localize-repair-validate pipeline [S51]; SWE-agent showed the interface (a linter that blocks broken edits, short outputs) moves results as much as the model [S52]. Terminal-Bench 2.0 (Nov 2025, 89 tasks) lists agent-plus-model pairs, and the same model scores differently under different agents [S53]. Claw-SWE-Bench measured it: with the model fixed, harness choice moved pass@1 by 27.4 points, nearly as much as model choice at 29.4, and a minimally adapted OpenClaw scored 19.1% against 73.4% for the tuned version on the same models [S54]. HAL: "scaffolds dramatically impact both accuracy and cost", and more reasoning often hurt [S13]. tau-bench introduced **pass^k**, the chance all k runs succeed: a 90% per-run agent passes 8 in a row only 43% of the time, and GPT-4o's pass^8 in retail was under 25% [S55]; tau2-bench added telecom, where pass^1 fell to 34% [S56]. GAIA tests general tool use [S57]. A June 2026 survey splits the harness into observation, context, control, action, state, and verification [S58]; the record is the state leg and idea 1 is the verification leg. Practitioners now call harness code "a 2026 artifact" to be replaced per model release: "thin harness, fat skills" [S59].

### Tool-use reliability with 7B-30B open models

Qwen3-Coder-30B-A3B reaches 50.3% on SWE-bench Verified under OpenHands [S60]; a 32B model fine-tuned on 5,017 SWE-smith trajectories hit 40.2% [S61]. BFCL v4 ranks Qwen3.5-27B within 10% of the leader [S62]. Levers with evidence: fewer tools [S17, S18], masks not removals [S19], distinct names and plain descriptions [S21, S20], constrained decoding with its tax [S14, S15], and irrelevance detection so the model can say "no tool fits" [S20].

### Self-consistency and verification for code

Models cannot reliably self-correct reasoning without external feedback, and sometimes get worse [S63]. CriticBench: weak models critique worse, and the generate/critique/correct gap is largest in small models [S64]. Model judges of code show position, length, and self-preference bias [S65] and score AUROC 0.65 or less at spotting false success [S1]. Execution is the honest signal, but weak tests over-credit [S22]. So: run tests, strengthen tests, check state; use a model judge only for lines no program can check, and make it name the result.

### Running for hours without a person

METR's 50% time horizon doubles every 4-7 months; the best 2026 model is estimated near 16 hours, at the edge of what METR can measure [S66]. Anthropic's long-running harness: initializer agent, feature list JSON, progress file, git, one feature per session, end-to-end checks [S8]; that file-backed state is "externalized, path-addressable, compaction-stable" [S59]. OpenAI's harness-engineering post (early 2026; the original returned 403, read via a mirror): a short AGENTS.md as table of contents, linters as "enforced, not documented" guardrails, agent-readable observability, periodic "garbage collection" of the codebase [S67]. The Ralph loop: a bash loop that restarts the agent with a fresh context on one task at a time, avoiding compaction drift [S68]. Claude Code checkpoints rewind code and conversation [S69]. Reliability metrics for long runs (decay curves, meltdown onset) rank models differently than pass@1 [S32]. Nerd Genie's checkpointed record already matches the file-backed pattern; ideas 2, 3, and 7 add the rewind, the progress bound, and the end-to-end proof those harnesses rely on.

## 3. Sources

- [S1] Advani, "From Confident Closing to Silent Failure: Characterizing False Success in LLM Agents", June 2026. https://arxiv.org/abs/2606.09863
- [S2] Mansoor, Phadke, Rana, "Verified Tool Calls Improve LLM Agent Reliability Under Non-Atomic Failures", July 2026. https://arxiv.org/abs/2608.02645
- [S3] Reddy, Challaram, Basu, "Reason Less, Verify More", July 2026. https://arxiv.org/abs/2607.07405
- [S4] Sinha, Arun, Goel, Staab, Geiping, "The Illusion of Diminishing Returns: Measuring Long Horizon Execution in LLMs", Sept 2025, ICLR 2026. https://arxiv.org/abs/2509.09677
- [S5] Dubey, "Real-Time Detection and Repair of LLM Agent Failures", Aug 2026. https://arxiv.org/abs/2608.02464
- [S6] Kim et al., "Coherence Collapse: Diagnosing Why Code Agents Fail After Reaching the Right Code", March 2026. https://arxiv.org/abs/2603.24631
- [S7] Andon Labs, "Vending-Bench", Feb 2025, and Vending-Bench 2. https://arxiv.org/abs/2502.15840 and https://andonlabs.com/evals/vending-bench-2
- [S8] Anthropic, "Effective harnesses for long-running agents", Nov 26, 2025. https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents
- [S9] OpenHands SDK docs, "Stuck Detector". https://docs.openhands.dev/sdk/guides/agent-stuck-detector
- [S10] Hou, Wang, Zhao, Wang, "When Agents Do Not Stop: Uncovering Infinite Agentic Loops in LLM Agents", July 2026. https://arxiv.org/abs/2607.01641
- [S11] Luo, Wen, Wang, "Agentic Abstention: Do Agents Know When to Stop Instead of Act?", June 2026. https://arxiv.org/abs/2606.28733
- [S12] OpenHands issue 5355, loop detection false alarms. https://github.com/All-Hands-AI/OpenHands/issues/5355
- [S13] Kapoor et al., "Holistic Agent Leaderboard", Oct 2025. https://arxiv.org/abs/2510.11977
- [S14] Li et al., "XGrammar-2", Jan 2026. https://arxiv.org/abs/2601.04426 (the 3B-beats-70B BFCL claim appears only in secondary write-ups, not verified in the abstract)
- [S15] Li, Zhang, Lv, "Constraint Tax in Open-Weight LLMs: Tool Calling Suppression Under Structured Output Constraints", June 2026. https://arxiv.org/abs/2606.25605
- [S17] Paramanayakam et al., "Less is More: Optimizing Function Calling for LLM Execution on Edge Devices", Nov 2024, DATE 2025. https://arxiv.org/abs/2411.15399
- [S18] "RAG-MCP: Mitigating Prompt Bloat in LLM Tool Selection", May 2025. https://arxiv.org/abs/2505.03275
- [S19] Ji, "Context Engineering for AI Agents: Lessons from Building Manus", July 18, 2025. https://manus.im/blog/Context-Engineering-for-AI-Agents-Lessons-from-Building-Manus
- [S20] Lin et al., "Hammer: Robust Function-Calling for On-Device Language Models via Function Masking", Oct 2024. https://arxiv.org/abs/2410.04587
- [S21] Anthropic, "Writing effective tools for agents", Sept 2025. https://www.anthropic.com/engineering/writing-tools-for-agents
- [S22] Yu et al., "SWE-ABS: Adversarial Benchmark Strengthening Exposes Inflated Success Rates", Feb 2026. https://arxiv.org/abs/2603.00520
- [S24] Cognition, "Don't Build Multi-Agents", June 2025, https://cognition.com/blog/dont-build-multi-agents ; Walden Yan, X post, 2026, https://x.com/walden_yan/status/2047054554433462360
- [S25] Anthropic, "How we built our multi-agent research system", June 13, 2025. https://www.anthropic.com/engineering/built-multi-agent-research-system
- [S26] Erdogan et al., "Plan-and-Act", March 2025, ICML 2025. https://arxiv.org/abs/2503.09572
- [S27] Breunig, "How Long Contexts Fail", June 22, 2025, https://www.dbreunig.com/2025/06/22/how-contexts-fail-and-how-to-fix-them.html ; "How to Fix Your Context", June 26, 2025, https://www.dbreunig.com/2025/06/26/how-to-fix-your-context.html
- [S28] Molinari, Ciravegna, "Reason-Plan-ReAct", Dec 2025. https://arxiv.org/abs/2512.03560
- [S29] Liu et al., "Lost in the Middle", July 2023. https://arxiv.org/abs/2307.03172
- [S30] An, "Verbatim Chunks Beat Extracted Artifacts", Dec 2025. https://arxiv.org/abs/2601.00821
- [S31] Zhang et al., "Agentic Context Engineering", Oct 2025, ICLR 2026. https://arxiv.org/abs/2510.04618
- [S32] Khanal, Tao, Zhou, "Beyond pass@1: A Reliability Science Framework for Long-Horizon LLM Agents", March 2026. https://arxiv.org/abs/2603.29231
- [S33] Ouyang et al., "ReasoningBank", Sept 2025, ICLR 2026, https://arxiv.org/abs/2509.25140 ; Google Research blog, April 21, 2026, https://research.google/blog/reasoningbank-enabling-agents-to-learn-from-experience/
- [S34] Shinn et al., "Reflexion", March 2023. https://arxiv.org/abs/2303.11366
- [S35] Xiong et al., "How Memory Management Impacts LLM Agents", May 2025, revised Oct 2025. https://arxiv.org/abs/2505.16067
- [S36] Sumers et al., "Cognitive Architectures for Language Agents", Sept 2023. https://arxiv.org/abs/2309.02427
- [S37] Packer et al., "MemGPT", Oct 2023. https://arxiv.org/abs/2310.08560
- [S38] Anthropic, "Managing context on the Claude Developer Platform", Sept 29, 2025. https://claude.com/blog/context-management
- [S39] Park et al., "Generative Agents", April 2023. https://arxiv.org/abs/2304.03442
- [S40] Wang et al., "Voyager", May 2023. https://arxiv.org/abs/2305.16291
- [S41] Anthropic, "Equipping agents for the real world with Agent Skills", Oct 16, 2025. https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills
- [S42] Xu et al., "A-MEM", Feb 2025. https://arxiv.org/abs/2502.12110
- [S43] Wang et al., "Agent Workflow Memory", Sept 2024, https://arxiv.org/abs/2409.07429 ; Suzgun et al., "Dynamic Cheatsheet", April 2025, https://arxiv.org/abs/2504.07952
- [S44] Hong, Troynikov, Huber, "Context Rot", Chroma, July 14, 2025. https://www.trychroma.com/research/context-rot
- [S45] Shrivastava, "Semantic Early-Stopping for Iterative LLM Agent Loops", June 2026. https://arxiv.org/abs/2606.27009
- [S46] Belcak et al., "Small Language Models are the Future of Agentic AI", June 2025. https://arxiv.org/abs/2506.02153
- [S47] Viswanathan et al., "Checklists Are Better Than Reward Models For Aligning Language Models", July 2025, NeurIPS 2025. https://arxiv.org/abs/2507.18624
- [S48] Yao et al., "Tree of Thoughts", May 2023, https://arxiv.org/abs/2305.10601 ; Zhou et al., "LATS", Oct 2023, https://arxiv.org/abs/2310.04406
- [S49] Wang, Zheng, "Fork, Explore, Commit: OS Primitives for Agentic Exploration", Feb 2026. https://arxiv.org/abs/2602.08199
- [S50] SWE-agent team, mini-swe-agent, 2025. https://github.com/SWE-agent/mini-swe-agent
- [S51] Xia et al., "Agentless", July 2024, FSE 2025. https://arxiv.org/abs/2407.01489
- [S52] Yang et al., "SWE-agent: Agent-Computer Interfaces", May 2024, NeurIPS 2024. https://arxiv.org/abs/2405.15793
- [S53] Terminal-Bench 2.0 and Harbor, Nov 2025. https://www.tbench.ai/leaderboard/terminal-bench/2.0 and https://venturebeat.com/ai/terminal-bench-2-0-launches-alongside-harbor-a-new-framework-for-testing
- [S54] Zheng, Han et al., "Claw-SWE-Bench", June 2026. https://arxiv.org/abs/2606.12344
- [S55] Yao, Shinn, Razavi, Narasimhan, "tau-bench", June 2024. https://arxiv.org/abs/2406.12045
- [S56] Sierra, "tau2-bench", June 2025. https://arxiv.org/abs/2506.07982
- [S57] Mialon et al., "GAIA", Nov 2023. https://arxiv.org/abs/2311.12983
- [S58] Guo et al., "From Question Answering to Task Completion: A Survey on Agent System and Harness Design", June 2026. https://arxiv.org/abs/2606.20683
- [S59] Lee, "Hidden Technical Debt of AI Systems: Agent Harness", May 8, 2026. https://leehanchung.github.io/blogs/2026/05/08/hidden-technical-debt-agent-harness/
- [S60] Qwen3-Coder-30B-A3B SWE-bench Verified reproduction, Hugging Face discussion. https://huggingface.co/Qwen/Qwen3-Coder-30B-A3B-Instruct/discussions/30
- [S61] Yang et al., "SWE-smith", April 2025. https://arxiv.org/abs/2504.21798
- [S62] Berkeley Function Calling Leaderboard V4. https://gorilla.cs.berkeley.edu/leaderboard.html
- [S63] Huang et al., "Large Language Models Cannot Self-Correct Reasoning Yet", Oct 2023, ICLR 2024. https://arxiv.org/abs/2310.01798
- [S64] Lin et al., "CriticBench", Feb 2024. https://arxiv.org/abs/2402.14809
- [S65] "Don't Judge Code by Its Cover: Biases in LLM Judges for Code", May 2025, https://arxiv.org/abs/2505.16222 ; "Bias in the Loop: Auditing LLM-as-a-Judge for Software Engineering", April 2026, https://arxiv.org/abs/2604.16790
- [S66] METR, "Time Horizon 1.1", Jan 29, 2026, https://metr.org/blog/2026-1-29-time-horizon-1-1/ ; original paper March 2025, https://arxiv.org/abs/2503.14499
- [S67] OpenAI, "Harness engineering: leveraging Codex in an agent-first world", 2026. https://openai.com/index/harness-engineering/ (403 to this fetch; read via https://alexlavaee.me/blog/openai-agent-first-codebase-learnings/)
- [S68] Huntley, "Ralph Wiggum as a software engineer", May 2025. https://ghuntley.com/ralph/
- [S69] Claude Code checkpointing docs, Sept 2025. https://code.claude.com/docs/en/checkpointing

Dropped as unverifiable: a third-party page claiming GPT-5.2-Codex scored 57.5% under Terminus-2 versus 64.7% under Codex CLI on Terminal-Bench 2.0; the page rate-limited the fetch and the figure could not be confirmed elsewhere.
