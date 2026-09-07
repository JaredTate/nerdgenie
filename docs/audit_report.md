**Nerd Genie: the ten improvements I would pursue first**

**The biggest opportunity is to finish useful work with fewer model calls, then make the remaining calls reuse more of what the model has already read.** Those two improvements can help coding, browser work, files and media. The next priority is making the record more trustworthy: keep every requirement, keep proof current, and distinguish attempted actions from completed ones.

**Keep the record design. Improve what enters it and how its evidence is delivered.** A compact record is a strong foundation. It does not eliminate the cost of recent results, incorrect summaries, repeated checks or recovery. The code already handles several of these problems; this report recommends extending that work.

The ranking below reflects expected impact across different kinds of work. It is a judgment, not a measured result. Reusable procedures move near the top for recurring work; a broken model connection must be fixed before optimizing that model. Implementation order also differs from impact: reliable outcomes and checks are prerequisites for safely automating more work.

**The old benchmarks cannot establish today's savings.** Every number below is an illustrative estimate with stated assumptions, not a fresh measurement or a promised improvement. An unaffected task saves nothing. Extra checks, retrieval or recovery can make a change cost more. Do not add these estimates together: several ideas remove the same wasted work.

**1. Make one model call accomplish more work.**

**What and why:** Let Go finish the routine parts of a requested step before asking the model what to do next. Eliminating an unnecessary round avoids another wait and another prompt.

**How it fits the code:** Automatic tests after edits, syntax checks, first-line marks and browser batches already exist. Their savings belong to today's baseline. Extend them narrowly: let an explicit group of related edits share one expensive final test, and let a server-start result report verified readiness instead of requiring repeated model polling. Keep immediate checks that catch a bad individual edit. Every helper must use the normal permission and event paths; the current automatic-check path needs attention before it is expanded. [Automatic tests](internal/loop/autotest.go#L26), [call handling](internal/loop/calls.go#L71).

**Thought experiment and regression test:** The first of five edits breaks an interface. Deferring every check lets four more edits build on the mistake. Check cheap local conditions as work proceeds, stop on failure, and run the shared suite at the end. Also test a changed working directory, a denied command, a user correction between steps and a test that is still running. Preserve its process handle and each action's result. “Still running” must never mean “passed.”

**Estimated benefit:** Removing six unnecessary rounds from a 40-round task means **15% fewer model calls**. It does not automatically mean 15% fewer tokens or shorter runtime, because calls differ in size. Separately, five edits each triggering the same eight-second suite cost 40 seconds; one shared run costs eight, saving **32 seconds of tool time** before extra checks or recovery. Measure whether those repeated rounds and suites still exist.

**2. Make the remaining calls reuse more cached work.**

**What and why:** Keep unchanged prompt text reusable by the model server. This reduces the work of reading the prompt before generating an answer, called *prefill*. It can lower local latency and some providers' input charges.

**How it fits the code:** The context builder already puts changing record text after recent messages and removes duplicate result summaries. Do not sell that existing fix again. Investigate the remaining changes to pictures, memory, tools and task boundaries. Keep the stable instruction and tool prefix intact. Test image handling in each provider's valid message format: moving an image away from its tool result can break meaning or protocol. A new correction must take effect immediately even when it invalidates cached text. [Context order](internal/context/window.go#L43), [picture replacement](internal/loop/picture.go#L50).

**Thought experiment and regression test:** A third screenshot causes removal of the first picture from an older message. That can invalidate reuse from that point onward. Preserving every image instead can exhaust memory. Compare bounded, correctly labeled image representations on the installed server; verify which page and observation the model sees. Include corrections, changed tools, restarts, cold caches and warm caches. A successful cache-save operation alone is not proof of restored reuse.

**Estimated benefit:** If prefill takes 40% of task time and this change removes a quarter of it, the task takes **10% less time**, before other effects. The same request can still present exactly the same number of input tokens: **zero reduction in transmitted tokens**. Extra memory pressure or worse answers can erase the gain. Keep daemon sampling unchanged; additional server slots are a separate hardware experiment.

**3. Keep every user requirement through replanning and restart.**

**What and why:** Give each requested outcome a stable ID tied to the user's words. A rewritten plan must not quietly erase unfinished work.

**How it fits the code:** The done list can currently be replaced, and marks refer to list positions. Add stable requirement IDs alongside the initial plan, without an extra planning call. Carry them through first-line marks, job tasks and completion checks. Keep the original source text and its authority. An ID prevents accidental identity changes; it does not prove that the model extracted every requirement. Preserve current corrections and deliberately pinned evidence when resuming. [List replacement](internal/record/model.go#L239), [position-based marks](internal/loop/firstline.go#L65), [job completion](internal/job/finish.go#L219).

**Thought experiment and regression test:** The user asks for captions, vertical output and no publication. A later plan drops captions and renumbers everything. Completion must still require captions, while the publication restriction remains in force. Then let the user explicitly withdraw captions: that change must work. Test reordered lists, split jobs, late corrections, examples that are not instructions, eviction, restart and switching models. Existing records need a compatible migration.

**Estimated benefit:** Suppose ten of 100 tasks currently omit a requirement and this prevents five omissions. That is **five fewer affected tasks per 100**, with at most a five-percentage-point improvement in completion if nothing else fails. It is not an estimated current failure rate. A 200-token requirement index repeated over 40 calls adds **8,000 input tokens**; it pays back only if it prevents more rework than that costs.

**4. Make “done” depend on evidence that is still valid.**

**What and why:** Remember which file or page was checked and what could make that check outdated. “Tests passed” must stop counting as current proof after a relevant change.

**How it fits the code:** The done check largely checks references and descriptions; it does not maintain dependencies between proof and changed inputs. Start with exact cases: the checked output file, its content hash, checker, settings and result ID. A hash identifies bytes, not correctness. Tests may depend on configuration, generated files and the environment too. Until those dependencies are known, use a conservative project-change boundary and a final rerun. This needs trustworthy outcomes from idea 5 and stable requirements from idea 3. [Current proof check](internal/record/proof.go#L31).

**Thought experiment and regression test:** Tests pass, then a configuration file changes while source files stay identical. A source-only hash falsely preserves success. Cover configuration changes, shell edits, external edits and restart. Avoid hashing an entire project after every keystroke or rerunning all tests for an unrelated note. For subjective media quality, preserve the distinction between an exact check and a judgment.

**Estimated benefit:** If stale proof causes ten failures per 100 tasks and this prevents half, it prevents **five such failures per 100**. Additional verification may increase tokens and time on tasks that already succeed. A 40-token proof-status addition across 20 calls costs **800 input tokens**; preventing one 10,000-input-token recovery call would save 9,200 net input tokens, before other overhead. An extra model reviewer is not required by this proposal.

**5. Record what actions actually did, including uncertainty.**

**What and why:** Distinguish requested, refused, failed, completed and unknown outcomes. This prevents false memories and unsafe duplicate actions.

**How it fits the code:** Tool calls are logged before permission checks, while memory capture describes some requests as accomplishments. Persisted full results contain an ID, summary and text, but no call ID for a reliable join. First stop describing requests as completed work. Then add versioned shared outcome metadata that links each result to its task and call. Never infer that link from adjacent log entries. Old ambiguous events stay unknown. File-change events can describe preparation for a write, so they are not proof that the write completed. [Capture](internal/memory/capture.go#L203), [stored results](internal/record/keeper.go#L123), [write order](internal/tool/write/write.go#L87).

**Thought experiment and regression test:** A website publishes a post, but the response is lost. Automatically retrying creates a duplicate. Inspect the visible page within a bounded wait; continue only when the outcome is established or repetition is safe. Test refusal, failed writes, delayed confirmation, a crash after the external action, legacy events and repeated call IDs in different tasks. A local action ID cannot force an arbitrary website to ignore duplicates.

**Estimated benefit:** If five of 100 tasks suffer outcome confusion and this prevents three, that is **three fewer incidents per 100**, not a 60% improvement in all browser work. Classifying known outcomes needs no new model call. Extra observations have a cost: 300 additional tokens shown on ten calls add 3,000 input tokens; avoiding two 10,000-input-token retries saves **17,000 net input tokens** in that example.

**6. Make each model connection carry the information it needs.**

**What and why:** Keep one semantic task record while correctly delivering tools, images and provider-required continuation data. A small prompt is no bargain if the model never received its screenshot.

**How it fits the code:** The shared message types omit some provider continuation fields. The Codex request asks for encrypted reasoning, while its ordinary input path carries text, calls and results; its tool-result rendering also omits pictures. Start with tested capabilities and complete usage accounting. Then experiment with a bounded provider-specific continuation envelope, separate from the readable record. Preserve required opaque or signed items intact through a valid tool cycle. Never pass them to another provider or reset them at an unsupported point. [Shared types](internal/contract/model.go#L132), [Codex transport](internal/provider/codexwire.go#L134).

**Thought experiment and regression test:** A vision task falls back to a text-only route and confidently judges an image it never received. Deliver the image or make its absence explicit. Test provider switches, partial streams, tool-call pairing, supported reasoning settings and continuation limits. Retaining old reasoning can also preserve a mistaken plan and crowd out useful evidence; a universal “keep all thinking” switch is not justified.

**Estimated benefit:** There is **no defensible general savings percentage**. Under illustrative prices where generated output costs five times ordinary input and cached input costs one tenth, retaining 2,000 cached tokens costs 200 input-equivalent units. Avoiding 100 output tokens saves 500, a **300-unit net benefit on that warm request**, before cache preparation charges. If those 2,000 tokens are uncached, it instead costs 1,500 extra units. The actual route's behavior and prices decide.

**7. Remove repeated browser output before attempting broad evidence selection.**

**What and why:** Return a short receipt for each browser action and one final page outline. Repeating the full page after every action hides the changes and fills the prompt.

**How it fits the code:** Browser batches already work; their rendering calls the page formatter for each step. Change the report, preserving the same visible clicks, typing, waits, permissions and order. Save original evidence before reducing the model-facing result. Keep important intermediate observations, navigation changes and failures. After that small experiment, test selecting evidence explicitly linked to the active requirement. Broad context selection remains a research proposal. [Batch rendering](internal/tool/browseract/browseract.go#L121), [page outline](internal/tool/browserread/page.go#L74).

**Thought experiment and regression test:** A confirmation number appears after step two and disappears after step five. Keeping only the final page loses proof. Preserve that receipt and stop at an uncertain or failed step. Test tab changes, stale element references, transient notices, screenshot delivery and retrieval of omitted evidence. A context selector must preserve current corrections and complete tool exchanges, not just recent-looking text.

**Estimated benefit:** Five steps each returning a 1,200-token outline and an 80-token receipt produce 6,400 tokens. One outline, five receipts and 80 tokens of labels produce 1,680: **4,720 fewer result tokens, about 74% of that result**. This is not a 74% task saving. Extra retrieval and cache rebuilding count against it. If preserved intermediate evidence is large, the reduction is smaller.

**8. Make repeated failure lead to a better diagnosis.**

**What and why:** Keep the observed symptom separate from the suspected cause, then choose a check that tells likely causes apart. Repeating “the button is broken” provides no new direction.

**How it fits the code:** The latest revision already keeps conversation rounds before the last progress and restores up to six recent results during orientation. Credit that improvement. The remaining proposal is to revise explanations for an existing symptom and make the existing stall message request a discriminating observation. Use a bounded set of short output fingerprints for progress tracking. Normalize only known irrelevant variation. [Recovery already built](internal/loop/guard.go#L79), [recent evidence](internal/loop/freshwindow.go), [progress tracking](internal/loop/progress.go#L97).

**Thought experiment and regression test:** A useful investigation reads twenty files without editing anything. A simplistic “no edits means no progress” rule stops it. Conversely, changing timestamps can make repeated useless output look new. Test both, including timing bugs where timestamps matter. Preserve useful research, new viewport observations and changes in the best known test result. Do not add a separate diagnostic model call by default.

**Estimated benefit:** If genuinely unproductive failure loops consume 20% of task time and better diagnosis removes a quarter, it saves 5% of task time. Subtract diagnostic work equal to 1% of the original time and the net is **4% less task time**. If the detector interrupts useful investigation, it can make the task slower. That distinction must be scored from outcomes, not assumed from repeated calls.

**9. Turn repeated successful work into checked procedures.**

**What and why:** Save a proven sequence with new inputs so the model does not rediscover it every week. This can have the largest gain for recurring image conversions, report generation or stable browser workflows.

**How it fits the code:** The generic learner saves completed plan prose; the runner already supports executable tool steps with permission decisions. Extend that runner with parameterized actions, starting conditions and observable checks. Learn from confirmed outcomes, not merely from a plan marked successful. Remove temporary paths, old process IDs and browser references. Stop at the first changed condition or uncertain effect. Browser procedures still use the visible browser and human pacing. [Learner](internal/skill/learn.go#L148), [runner](internal/skill/run.go#L98).

**Thought experiment and regression test:** A procedure learned on one website later finds a changed form. Blind replay enters data in the wrong field. Recheck starting conditions and the meaning of each consequential action. Test new inputs, revoked permission, missing dependencies, interruption, failed checks and duplicate publication. One success is not enough evidence to generalize every trace.

**Estimated benefit:** Suppose preparation and validation cost 6,000 tokens. A manual repeat costs 4,000; replay costs 500; 10% of replays need 4,000 additional recovery tokens. Expected repeat cost is 900, saving **3,100 tokens, or 77.5% per later use**. Preparation breaks even after **two later uses** under those assumptions. One-off work may never repay the learning cost; include validation and recovery in the real measurement.

**10. Apply memory only where it belongs.**

**What and why:** Separate a temporary instruction from a project rule or lasting user preference. “Make this banner blue” must not become a rule for every future banner.

**How it fits the code:** Memory facts already have IDs, sources, dates and supersession links. The missing part is explicit scope and retrieval that respects it. Add that through the shared contract, storage and query path. Use a project identity that survives renaming and deliberately handles worktrees. Include enough scope and source information in the short hint to retrieve the full fact. Keep ambiguous instructions narrow; keep clearly global preferences available. [Existing fact fields](internal/contract/memory.go#L15).

**Thought experiment and regression test:** Two projects require opposite colors, but the user prefers metric units everywhere. A query restricted too narrowly loses the global preference; one too broad mixes the colors. Test both, plus replacements, renamed projects and legacy unscoped facts. Metadata must not truncate a crucial word such as “not.” Do not mistake a website's text for the user's preference.

**Estimated benefit:** Adding 15 tokens of metadata to each of three hints across 40 calls costs **1,800 input tokens**. If that prevents one 10,000-input-token reread, it saves 8,200 net input tokens in that task. Otherwise it adds overhead. Keeping the existing hint budget avoids growth but leaves less space for fact text. The main expected benefit is fewer wrong instructions applied, whose current frequency must be measured.

**What survives the three checks.**

I checked the code path and existing behavior, tried a case where each proposal makes things worse, and checked the savings arithmetic including overhead. That supports the narrower proposals above as implementation candidates. It does not prove that unimplemented code works or that regressions are impossible.

The most direct first efficiency experiment is browser-output deduplication. Broader automatic work depends on fixing action accounting and check accuracy. General evidence selection and retained provider reasoning need controlled experiments. Procedures are strongest on genuinely repeated tasks. These distinctions matter more than a large unsupported percentage.

**Rebenchmark today's code before treating any estimate as a result.**

The existing [Qwen](QWEN_BENCHMARK.md), [GPT](GPT_BENCHMARK.md), and [Opus](OPUS_BENCHMARK.md) reports describe earlier versions and setups. They do not establish today's ranking. Later changes include automatic checks, first-line record marks, recovery behavior, context layout, result and lesson limits, and vision support. The [progress record](docs/PROGRESS.md) and [current ideas](IDEAS.md) document that continuing work. Fresh measurements should determine what waste remains.

I would run the new benchmark in this order:

1. **Freeze today's baseline.** Record the Nerd Genie revision, model file or version, tool versions, server build, and settings. Run the current harness before adding another improvement. Keep the binary unchanged throughout each scored run.
2. **Measure complete tasks.** Start with the six kinds of work below. Use three repeats per task for an initial picture, then expand the tasks and repeats where results vary. Include every failed run and human intervention.
3. **Find where time and tokens go.** Separate reading new prompt text, generating output, waiting for tools, polling, bookkeeping, and recovering from errors. Count real model calls and exact usage for every attempt, including retries.
4. **Compare current products fairly.** Pin each competitor's version too. Use the same task, starting files, model, and outcome checker where possible. Report comparisons with different subscription routes or tool sets separately. Run both normal defaults and clearly documented tuned settings.
5. **Try one Nerd Genie change at a time.** Compare it with the frozen baseline on the same tasks. Keep changes that improve completed-task time, cost, or reliability. Do not credit smaller prompts when they merely create more recovery work.

| Work to include | Example | How to judge the result |
|---|---|---|
| Short coding | Fix a parser or add a small feature | Independent tests the agent cannot weaken |
| Long coding | Change several files, receive a correction, restart | Final tests and every original requirement |
| Browser work | Prepare and publish an approved fixture post | Exact content, correct destination, one publication, visible UI use |
| CLI and files | Transform data or finish a long-running command | Output contents, expected paths, exit status and recovery |
| Media | Produce captioned clips and thumbnails | Duration, dimensions, captions, representative frames and quality review |
| Memory and jobs | Retrieve an old fact and continue recurring work | Correct scope, retained corrections, old evidence recovered, no repeated effects |

For each task, report **correct completion, false completion, total time, tool time, total model calls, fresh input tokens, cache reads, cache writes, output tokens, and human steers**. Add actual API cost where available. Keep subscription usage separate from hypothetical API pricing. For local runs, include memory use and the time spent processing prompts versus generating output. Show typical and slow runs, variation, and failures.

Run cold and warm cache cases. Include a correction, image replacement, context eviction, and restart. For smaller local models, repeat a useful subset across more than one size and quantization. One custom 27B model cannot tell us how every smaller model behaves.

**Some acceptance tests need strengthening before their green result can score the new benchmark.** The live forty-step test currently loads a completed scripted record and makes one real model call. Keep that persistence check, but add a task where the model actually chooses the steps, receives a correction, and retrieves evidence that has left its recent context. [Current fixture](test/functional/livefixture_test.go#L29).

Five browser functional tests remain placeholders, while separate worker tests cover useful pieces. Complete the path from the user socket through the loop, permissions, vault, and browser. The normal gate also omits the separate browser target and worker test commands. Nightly acceptance should inspect final files or fixture state, match each result to its own call and task, and fail when the outcome is wrong. [Browser placeholders](test/functional/browserflows_test.go#L19), [worker coverage](test/functional/browserflows_integration_test.go#L23), [gate](Makefile#L74), [nightly checks](scripts/nightly/checks/05-tetris.sh#L6), [nightly exit](scripts/nightly/run.sh#L145).

Borrow the useful testing methods: repeated-trial reliability and final-state checks from [τ-bench](https://arxiv.org/abs/2406.12045), realistic fixture websites from [WebArena](https://webarena.dev/og/), and desktop outcome checks from [OSWorld](https://os-world.github.io/). The browser agent should still act through the visible UI; a separate evaluator may inspect fixture data to score what happened.

**The regression gate must include whole tasks.**

Write failing tests first for each concrete bug, then unit, integration, functional and parsing fuzz tests where the change needs them. Preserve compatibility with old records and event logs. The scenarios under each idea are acceptance cases, not a substitute for the repository gate.

Compare a frozen baseline and candidate on paired tasks with the same starting state. Alternate run order, distinguish cold from warm caches, and keep separate results for each model and task family. Include unseen tasks so a procedure or prompt does not win by memorizing the benchmark. Three repeats are an initial screen, not proof of reliability: even zero failures in three independent trials is consistent with a very high true failure rate.

Any new lost correction, unauthorized action, duplicate ambiguous side effect or false completion blocks acceptance. Report correctness uncertainty; an inconclusive comparison stays experimental. Show added verification cost openly when a reliability fix spends more to prevent a bad outcome. Test the combined final changes too: two successive 10% reductions of the remaining cost yield 19%, not 20%, and overlapping changes can save much less.

**Count the cost honestly.** Capture every attempt, retry, review and recovery call, rather than inferring totals from repeated record checkpoints. Report cache reads and writes explicitly; cache reads are part of total input, and reasoning output must not be counted twice. Unknown usage stays unknown.

A smaller prompt can cost more: at an assumed cache-read price of 0.1 times ordinary input, 4,000 fresh tokens plus 36,000 cached tokens cost 7,600 input-equivalent units. Replacing that with 12,000 entirely fresh tokens reduces presented input by 70% but raises that request's input cost by about 58%. Output and cache-write charges are excluded from this example. Measure completed-task resources, not just prompt length.

**The code findings behind the recommendations.** These are findings from reading revision `5e7298e`, not fresh runtime reproductions. They do not depend on the old competitor benchmarks.

| Finding | Why it needs attention | Source |
|---|---|---|
| Pinned result entries can be trimmed; resume rebuilds pins from retained entries. | Evidence deliberately kept can disappear after restart. | [Trimming](internal/record/size.go#L107), [resume](internal/loop/taskcall.go#L194) |
| Restoring an older record checkpoint restores its correction list. | Continuing an old plan can lose newer instructions without undoing files or website actions. This differs from the loop recovery improved in the latest revision. | [Checkpoint restoration](internal/record/checkpoint.go#L57), [test](internal/record/checkpoint_test.go#L88) |
| The test reader recognizes a bare Go package failure without counting a failure. | A failed build can produce “tests: all passing.” | [Parser](internal/loop/teststate.go#L131), [summary](internal/loop/teststate.go#L44) |
| Browser expectation matching accepts any meaningful matching word. | A word match can be presented as proof that the expected action worked. | [Matcher](worker/browser/src/expectation.ts#L89), [wording](internal/tool/browserread/page.go#L48) |
| Automatic checks call the timeout helper directly. | They miss the normal loop's permission decision and complete per-action result trail. | [Automatic tests](internal/loop/autotest.go#L26), [syntax checks](internal/loop/syntax.go#L67), [normal path](internal/loop/calls.go#L71) |
| Memory capture describes tool-call requests as completed actions. | A refused command or failed visit can become a false memory. | [Capture](internal/memory/capture.go#L203), [logging before execution](internal/loop/calls.go#L72) |
| Done lists can be replaced, and completion largely checks references. | An omitted requirement or unrelated old result can escape a meaningful final check. | [Replacement](internal/record/model.go#L239), [proof check](internal/record/proof.go#L31), [job finish](internal/job/finish.go#L219) |
| Browser batches render a page for each step. | The model receives repeated page text. | [Batch](internal/tool/browseract/browseract.go#L121), [page rendering](internal/tool/browserread/page.go#L74) |
| Codex and CLI rendering omit pictures from tool results. | A fallback can answer without the image the loop intended to show. | [Codex](internal/provider/codexwire.go#L153), [CLI](internal/provider/cliprompt.go#L100) |
| The generic skill learner saves prose; direct replay needs tool steps. | A saved plan is not yet a reusable procedure that avoids inference. | [Learning](internal/skill/learn.go#L148), [execution](internal/skill/run.go#L98) |

Two separate maintenance proposals deserve focused tests. Add direct result lookup instead of repeatedly scanning the task's [whole event history](internal/record/harness.go#L152), after measuring lookup cost. Give [scheduled jobs](internal/job/next.go#L108) a bounded way to archive completed batches before their 200-entry list fills. Archiving is a separate job-lifecycle change, not a memory-scope feature: it needs stable task references, retained instructions, correct counters and crash-safe rollover. The current cap is deliberate; simply removing it would violate the bounded-state design.

**Keep the small record, and state exactly what its size means.** The current 3,000-token limit is a word-based estimate that excludes the ask. Long asks have a separate shortened prompt view, and the original remains stored. Arbitrarily many exact corrections cannot fit forever in one fixed-size view. Keep source text lossless, keep the active view bounded, and check actual token counts for each model. Test long specifications, code, and languages without spaces. The decisive test is whether the model still follows the request. [Record limits](internal/record/size.go#L16), [ask display](internal/record/print.go#L115), [prompt estimate](internal/context/estimate.go).

**What this means for the requested models.**

| Model | First practical priority |
|---|---|
| Qwen 3.8 locally | Preserve cached work, reduce unnecessary calls, and test exact tool and image delivery. Record the actual GGUF, quantization, projector, template and TurboQuant build. Keep existing sampling settings. The official card does not prove how the modified local artifact behaves. [Qwen card](https://huggingface.co/Qwen/Qwen3.8-27B) |
| GPT-5.6 | Name the exact variant and access route. Preserve supported continuation data and test explicit cache boundaries on transports that document them. Measure cache writes, reuse and generated output together. Do not assume public API fields work on the private Codex backend. [Model](https://developers.openai.com/api/docs/models/gpt-5.6-sol), [caching](https://developers.openai.com/api/docs/guides/prompt-caching) |
| Opus | Measure Claude CLI and Messages API separately. On the API route, preserve required thinking data and choose cache lifetimes based on actual reuse. Compare complete-task results before reducing reasoning effort. [Opus guidance](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/prompting-claude-opus-4-8), [caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching) |
| GLM | Use the exact model and endpoint's documented thinking and continuation controls. Test a complete tool sequence, rather than assuming OpenAI-compatible means identical behavior. [Thinking](https://docs.z.ai/guides/capabilities/thinking-mode), [cache](https://docs.z.ai/guides/capabilities/cache) |
| Llama | Test a named model with its native prompt and tool format. Keep the model identity separate from llama.cpp, the server that can run many model families. [Meta's format](https://github.com/meta-llama/llama-models/blob/main/models/llama4/prompt_format.md), [llama.cpp tool support](https://github.com/ggml-org/llama.cpp/blob/master/docs/function-calling.md) |

For Codex subscription access, separately investigate whether the documented [App Server](https://learn.chatgpt.com/docs/app-server) can meet Nerd Genie's needs. It is itself an agent interface, and its dynamic-tool support is experimental. It must be shown to preserve Nerd Genie's control over tools and permissions before replacing the current connection. This is not a proposal to add API keys to this machine.

**The computer science ideas are useful because they solve specific problems here.**

- **Track what a conclusion depends on.** Truth-maintenance systems record why a belief is held; build systems rerun affected work when inputs change. Apply that narrowly to fresh proof in idea 4. A file hash identifies the checked bytes; it does not prove correctness. [Doyle, 1979](https://www.sciencedirect.com/science/article/pii/0004370279900080), [Build Systems à la Carte](https://simon.peytonjones.org/assets/pdfs/build-systems-original.pdf).
- **Keep needed information close.** Operating systems retain a program's working set to avoid repeatedly fetching it. Apply that to the experimental evidence selection in idea 7, while measuring the cost of rearranging the prompt. [Denning, 1968](https://denninginstitute.com/pjd/PUBS/WSModel_1968.pdf).
- **Plan around what has actually been observed.** Work on partial observation explains why an uncertain result should stay uncertain. Apply that to diagnosis and browser recovery, without building a large probabilistic planner. [Kaelbling, Littman and Cassandra](https://cs.brown.edu/research/pubs/techreports/reports/CS-96-08.html).
- **Design retries around possible side effects.** Distributed systems distinguish safe repeats from ambiguous outcomes. Apply that to commands, posting and restart recovery in idea 5. [AWS on safe retries](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/).

These sources support the underlying methods. Whether the proposed adaptations save Nerd Genie tokens or improve its success rate requires the new experiments.

**A short optional prompt format is worth testing.**

```text
Goal: What should be finished?
Inputs: Which files, pages or examples should be used?
Constraints: What must be preserved or avoided?
Done when: What result would prove it worked?
Ask first: Which actions need approval for this task?
```

For example: “Make three 30-second vertical clips from this interview. Add accurate captions and save previews here. Check the duration and captions. Ask before publishing.” The harness should also handle the same request in ordinary conversation. Existing permissions still apply when the last line is absent. Compare equivalent requests so the formatted version does not win just because it received extra information.

**What I would do next:** establish the fresh baseline and fix incorrect success, lost instructions and incomplete action accounting with failing tests. Trial the small browser-output change, measure remaining cache losses, then expand automatic work where the evidence shows wasted rounds. Test reusable procedures on recurring work. Keep broader evidence selection and provider continuation as separate experiments. Shared types belong in `internal/contract`; each implementation should stay small and follow the repository's test-first rules.

Keep credit for what is already built. Automatic checks, first-line marks, context eviction, result IDs, pinned evidence, browser batches and skill replay are the foundation these ideas extend. The [current idea list](IDEAS.md) and [previous decisions](STATE_IMPROVE_INNOVATE_PLAN.md) also explain why blanket output replacement, more agents by default, broad state rewrites and daemon thinking changes should not be casually reintroduced.

**Scope:** revised 6 September 2026; source revision `5e7298e`. The original review covered the onboarding documents, state and LLM research, relevant implementation and tests, with four specialist reviewers and a fifth report reviewer. This revision rechecks the proposals against the pulled code, ranks broad impact, adds counterexamples and conditional estimates, and distinguishes existing features from new work. The recovery change in `5e7298e` is included in the baseline. No performance estimate here is a new benchmark result. The checkout is on `jared-the RDNA4 machine`, while builds and tests are required on the development machine. No code, services or model settings were changed, and no live benchmarks were run. Only this report was edited for the audit.
