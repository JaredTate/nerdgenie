# Final review of `docs/WORK_PLAN.md` (version 2) before hand-off

Reviewer stance: last reviewer, adversarial, read-only. Read in full: `docs/WORK_PLAN.md` at commit `2097ee2`, `docs/NERDGENIE_PLAN.md`, `CLAUDE.md`, `ARCHITECTURE.md`, and the previous review `docs/research/work-plan-review.md`.

Line references: **WP** = `WORK_PLAN.md` line, **CL** = `CLAUDE.md` line, **AR** = `ARCHITECTURE.md` line, **CP** = `NERDGENIE_PLAN.md` section.

Checked on disk (Mac clones under `/Users/jt/Code`, the same repos the dev machine holds under `/home/jared/Code`):

- Every reference path named in every brief resolves (about 100 paths, none missing).
- The five pinned commits are each the local HEAD: openclaw `752a983480e4`, hermes-agent `95f62ca3bfcf`, prime-agent `0ba0423c5c18`, opencode `69c172e8a7c0`, zeroclaw `73ff732e66c7`. HomeRecon is at `96b481db6361` (the plan pins no commit for it).
- `docs/reference/{codex,browser-use,moltis}/` and `docs/research/16-browser-agent-spec.md` exist.
- The repository already holds `REPO_MAP.md`, `THIRD_PARTY.md`, `README.md`, and `scripts/repo-map.sh`; `docs/briefs/` does not exist; no `go.mod`, `Makefile`, `LICENSE`, or CI file exists.
- The design's section-5 instruction text is **361 words** without its bold headings (382 with them). Brief 2.1 (WP L212) says "a test asserts it is under three hundred words." That test fails on the verbatim text.

## Verdict

**DO NOT SIGN.** The plan is far better than version 1 and most of the 24 blockers are addressed in substance, but four things would stop a team on day one: (a) wave 1 is not parallel as written, and waves 3, 4, 5, 6, and 7 each still have a same-wave import; (b) the "import only downward" rule is stated backwards, so an orchestrator following it literally rejects every import; (c) the live tier has two models, not three, and never names OpenAI; (d) several pieces the design calls part of the MVP (`read r7`, the Situation section, the token budget, pairing confirmation, `/undo`) have no owning brief and no "left for later" line. The shortest list of edits that would earn a signature is at the end.

---

## Part A: the owner's checklist

### 1. A finished, working MVP; nothing deferred without being named — **FAIL**

The goal line (WP L7) and wave 7 describe a working agent, and no brief says "later." But these design features have no owning brief, and the plan has no "left for after the MVP" list, so each is deferred silently:

| Missing piece | Design says | Where the plan should own it |
|---|---|---|
| `read r7`: read a past result by id | CP §4 "read any of them with `read r7`", CP §7 `read` row | Brief 2.3's `read` is "offsets, line numbers, a continue hint": files only |
| The Situation section and the harness-added stop lines (budget, login walls) | CP §4 table: "Situation: the harness, from the world as it is right now"; "the harness adds the budget and login walls" | No brief writes either; 2.2 is the only package that sees tool results |
| Per-task token budget and the header line `budget: 6 of 20 rounds, 41k tokens, 9 min` | CP §3 rule 3, CP §4 example | 2.2 has the round cap, 4.5 the minutes; nobody tracks tokens or writes the header |
| Third fold tier (record line to log-only) | CP §4 "then it lives only in the log" | 1.3 folds to single lines; nothing ever leaves the record, so it grows past its cap on a long task |
| The user's side of pairing (confirming a code) | CP §12 "unknown senders receive a pairing code" | 4.3 generates and stores codes; no command or prompt lets the user approve one |
| `/undo` | CP §12 "reverts the last turn's file changes" | 3.4 builds "the commands from section 12" but no brief snapshots the files a turn touched |
| `/screen`, `/jobs`, `/memory`, `/skills`, `/vault`, `/pause`, `/resume` | CP §12 | 3.4 owns the table in wave 3; their bodies live in waves 4 to 7 and no later brief says "register your command" |
| A search server on a fresh install | CP §7 `web` "searches the web" | 2.5 picks self-hosted SearXNG; the installer (3.5) neither installs it nor names it as a prerequisite, so `web` search is dead on every fresh install |
| The desktop worker | CP §10, v4 | Built by 7.2, but absent from the goal line (WP L7): "use a real Chrome web browser, remember, run jobs, update itself." Either it is MVP and belongs in the goal, or it is not and should be cut |

**Fix:** add a "Left for after the MVP" paragraph to Part 3 naming anything deliberately out, and give the rest an owner: `read r7` to 2.3; Situation, harness stop lines, and the token budget to 2.2; the third fold tier to 1.3; `/pair <code>` to 4.3; the `/undo` snapshot to 2.3; one "registers these commands" line in 4.4, 5.1, 5.3, 6.2, 7.1; SearXNG (or a keyless fallback) to 3.5.

### 2. Tested against three real models, including OpenAI/Codex — **FAIL**

WP L65: "the same functional tests run against **two** real models. One is Opus 4.8 through the Anthropic API ... The other is the local Qwen 3.8 model served by Ollama." CL L36 and AR L99 say the same two. The words "OpenAI," "GPT," and "Codex" appear in the plan only as (a) the OpenAI-compatible *provider implementation* tested against the fake server (WP L206) and (b) the reference project `docs/reference/codex/` (WP L324). No GPT model through the real OpenAI API is in the live tier anywhere. The Ollama path exercises the OpenAI-compatible client against a local server, but nothing ever exercises it against a cloud endpoint with real rate limits, real streaming, and real cache accounting.

**Fix:** in WP L65, L7, L278, CL L36, and AR L99 make the live tier three models: Opus 4.8 (Anthropic), a named GPT model through the OpenAI API (the OpenAI-compatible provider against a real cloud server), and the local Qwen through Ollama; add the OpenAI key to `make live` and `docs/PROGRESS.md`. Also pin the exact Ollama tag for "Qwen 3.8" (for example `qwen3:8b`) so `make live` is reproducible.

### 3. Easy install: one command, prerequisites named, clean machine, tested — **FAIL**

What is there (WP L234): a shell script under 200 lines, checks the distribution, installs bwrap, ripgrep, signal-cli, tells the user about Chrome, downloads the release binary, verifies the checksum, links `current`, runs `nerdgenie init`; tested on clean Ubuntu and Debian containers in CI ending with a passing `doctor`. Good shape. What stops it:

- **No release exists when the test runs.** The installer "downloads the release binary for the architecture" in wave 3; no brief in any wave produces a release artifact or names where it is published (7.3's "release manifest" is also unsourced; 7.5 "the release" never says publish). The CI test cannot download anything. Fix: add `make release` (binary plus checksum plus manifest, to GitHub Releases) to 0.1 or 3.5, and give the installer a `--from <path>` flag the CI test uses.
- **`init` is interactive; the container test is not.** 3.4 says `init` "asks at most six questions." The CI test needs a non-interactive path (flags or environment) that no brief provides.
- **Prerequisites missing from the list:** Node and the browser worker bundle (6.1 needs `node` and `playwright-core`; neither the installer nor the updater ships or installs them, so wave 6 has no install path), a Java runtime or the native signal-cli build (the brief must say which), SearXNG (item 1), a display server for the visible Chrome window, and a systemd user session with lingering enabled so the service survives logout.
- **"Ends with a passing `doctor`" is undefined** when Chrome is absent by design in the container. Say whether `doctor` warns or fails on Chrome.

### 4. Onboarding: first-run flow creates everything, few questions, links Signal, pairs a phone, tested — **FAIL**

`nerdgenie init` (WP L232) creates the home, persona files, model menu, masked key, runs `doctor`, prints next steps, six questions, two minutes, and is tested on an empty home and an existing home. That half passes. The other half does not: `init` never links Signal or pairs a phone. Linking is a separate command in 4.2 (`nerdgenie signal link`), and 4.4 updates `init` only to move the key into the vault. Pairing (4.3) has no user-side confirmation at all: the brief stores approved ids but never says how an id becomes approved (a `/pair <code>` command? a prompt in the terminal?), and its test list ("every pairing rule") does not include the round trip. The wave-4 human trial (WP L250) is the only place the three steps are joined, by a person.

**Fix:** 4.2 adds "`init` offers Signal linking as its last step when signal-cli is present"; 4.3 adds a terminal-only `/pair <code>` command with a test that a code sent from the fake signal-cli, then typed in the terminal, produces an approved id and a first reply.

### 5. Updating: one command, data survives, rollback, tested — **FAIL** (one ordering hole)

WP L290 is the strongest brief in the plan: manifest, checksum, `releases/<version>/`, keep three, switch the symlink, restart, `/readyz` within sixty seconds, switch back on failure; forward-only numbered migrations in a transaction after a backup; three tests including "an update over a live install preserves every task, memory, and skill." Two holes:

- **Rollback after a migration leaves a broken install.** The brief runs migrations, restarts, and switches back on readiness failure. It also says "an older binary refuses a newer schema." So a failed update that already migrated rolls the symlink back to a binary that refuses to start. Fix: order the update as download, switch, readiness check, then migrate; or restore the pre-migration backup as part of rollback. Add the test "rollback after a migration leaves a working install."
- **The browser worker is not updated.** `nerdgenie update` downloads "the binary." The TypeScript worker and its `node_modules` are part of the product from wave 6 and have no update path (same gap as item 3).

### 6. Every wave: all tests run and pass, ARCHITECTURE.md and REPO_MAP.md updated, orchestrator checks — **FAIL** on "all tests"

The lines that say it:

- WP L9: "When the five report back, merge, run `make check`, read every new file, run `make live` for the wave's features, and only then move on."
- WP L89 to L90: "The orchestrator merges the five branches, runs `make check`, reads every new file, and runs the tier-two live tests for the wave's features. If everything passes, the orchestrator adds the wave section to `ARCHITECTURE.md`, writes three lines in `docs/PROGRESS.md`, and starts the next wave."
- WP L37: "A worker's brief is not done until `ARCHITECTURE.md` is true for that package and `REPO_MAP.md` is regenerated."
- WP L307: "`REPO_MAP.md` drift fails `make check`. `ARCHITECTURE.md` is read by the orchestrator at every wave gate against the code."
- CL L35: "`make check` ... A wave does not pass until this is clean."

The living-document half passes. The "all tests" half does not: **no named target runs the integration-tagged tests or the fake-model functional tests at the gate.** WP L98 defines `make check` as "`go vet`, `staticcheck`, `gofmt`, the style checker, and the repo-map drift test" (no tests at all). CL L34 to L35 says `make check` also runs `make test`, which is "unit tests plus one minute of fuzzing per fuzz target." Neither mentions `//go:build integration` or `test/functional/`. `make live` runs functional tests only against real models. So the tier-one functional suite (WP L64 "every commit ... runs in CI") has no command that runs it. The two documents also disagree on what `make check` contains.

**Fix:** define `make test` in WP L98 and CL L34 as unit + integration + functional (fake model) + a five-second fuzz smoke; add `make fuzz` for the one-minute runs; state that `make check` includes `make test`.

### 7. Visual QA by a human in a visible window, with when and how — **FAIL**

Human trials exist after waves 4, 6, and 7 (WP L9, L91, L250, L280, L308), and WP L102 admits "screen code is tested by looking at it." But the plan never says *what* the person looks at or *how* they record it beyond "what was confusing." Nothing names a human looking at: the terminal's first frame, spinner timing, streaming, inline approval, masked prompt, or inline screenshot (4.1); the visible Chrome window during the wave-6 live runs, human pacing, or the handoff bringing the window forward (6.1, 6.3); the desktop fixture window (7.2). The wave-6 "visual QA skill" (6.5) is the agent looking, not a person. And the plan contradicts itself on which waves get a person: WP L91 "Every wave from wave 4 onward" (includes wave 5) versus WP L9 and L308 "waves 4, 6, and 7."

**Fix:** add a five-line visual checklist to each human trial (terminal: first frame, spinner, streaming, approval, masked prompt, inline image; browser: window visible and in front, pacing looks human, handoff raises the window; desktop: fixture app on screen and marks aligned), a line "the person's notes go in `docs/PROGRESS.md` under the wave," and make L91 agree with L9 and L308.

### 8. Five Opus 4.8 workers at extra-high effort per wave, Fable 5.1 orchestrating and running tests — **FAIL** (small, wording)

Stated: WP L9 "launch five Opus 4.8 workers at extra-high effort at once"; L25 the orchestrator is Fable 5.1 and "runs the full test suite after every wave"; L27 workers are Opus 4.8 at extra-high effort; L89 the orchestrator runs `make check` and the live tests itself. Wave 0 (WP L180) says plainly "One worker, not five, because everything else depends on it," which is fine, but the standing instruction at L9 ("For each wave, write the five briefs ... launch five") and L86 ("The orchestrator writes five briefs") carry no exception, so an orchestrator following L9 literally writes five wave-0 briefs. Wave 6 is self-contradictory: 6.1 "runs alone for the first half of the wave" and in the same sentence "briefs 6.2 through 6.5 start against the fake worker and switch to the real one when 6.1 lands." Either they start together or they do not. Briefs 5.5, 6.5, and 7.5 (the functional passes) cannot start until their four neighbors merge, and the plan does not say so.

**Fix:** L9 and L86: "five briefs, except wave 0, which has one"; wave 6: "all five launch at once; 6.2 to 6.5 code against the fake worker until 6.1 merges"; 5.5, 6.5, 7.5: "starts when the other four merge."

### 9. World-class test suite: four kinds per package, coverage gates, fuzz duration, contract tests, forty-step fixture, replay, CI — **FAIL**

Present and good: four kinds (WP L53 to L58), coverage numbers (L102), fuzz one minute per parser (L96), contract tests for every fake under `live` (L305, CL L43), the forty-step fixture (L82), replay (7.4), CI on a Linux runner (0.1). What breaks it:

- **Coverage is a rule, not a gate.** No target measures it; `make check` (L98) has no coverage step. Add a coverage step to `make check` with the per-package threshold.
- **One minute of fuzz per target inside `make check` on every commit.** By wave 4 that is twenty-plus targets and twenty-plus minutes per merge. The previous review's finding 112 stands. Split into a smoke in `make test` and `make fuzz`.
- **The forty-step fixture "runs in both tiers" (L82) but is "a scripted task" with scripted replies.** A real model does not follow a script. The live variant (real model, forty scripted tool results, the same three assertions, a forty-round cap) is never defined, so the tier-two run cannot be written as described. Define it in 1.1.
- **The fixture cannot run in wave 1.** Its assertions need the loop (2.2), the context builder (2.1), and the done-check (2.2). Brief 1.3's test "the forty-step fixture from testkit passes" has nothing to drive forty rounds in wave 1. Say: wave 1 ships the data and the three assertion helpers; the first end-to-end run is 2.2's, and it joins the gate from wave 2.
- **Replay is built twice:** testkit's "recorded-task replayer" (L81) and `internal/replay` (7.4, L292). Previous finding 51. Delete it from 1.1.
- **Brief 7.2 has no test tooling.** Part 6 (L353) claims "Vitest, fast-check, seventy percent, in briefs 6.1 and 7.2"; 7.2 says only "Coverage seventy percent." Add the tooling line to 7.2.
- **The fake list is still short** for what briefs test: a fake sandbox and a bwrap-missing path (3.1, 3.2), a fake password source for sudo (3.2), a fake systemd notify socket (3.5, 4.5), a fake release source (7.3), a fake desktop worker (7.2), a fixture command-line tool with docs (5.4), the twenty-question memory fixture (5.1, 5.5), the login, captcha, and QA fixture sites (6.5), fake skill and schedule stores (2.5, see item 17).

### 10. KISS and YAGNI stated and enforced — **PASS**

Stated as rule 1 (WP L41): "The simplest thing that passes the test is the right thing. No abstraction is added until the second use of it exists. No configuration option is added until a real user needs it." Repeated in CL L16. Enforced by: the style checker's function and file limits in `make check` (1.4, L100), the dependency gate (L43, L300), "a package grows past its one sentence: split it" (L303), and the orchestrator reading every new file (L89) holding workers "to the rules in Part 1" (L9). One note: the names KISS and YAGNI never appear, and the plan should say that `internal/contract` (interfaces written before any implementation) and the clock parameter are the sanctioned exceptions to "no abstraction before the second use," or a worker will cite rule 1 against them.

### 11. Plain English for code, comments, errors, logs, and documents; enforced — **FAIL** for documents

Code, comments, errors, and logs: rule 5 (WP L45), CL L20, enforced by the style checker in `make check` (1.4, L306). Documents: nothing. No rule covers briefs, `ARCHITECTURE.md`, `docs/PROGRESS.md`, the README, or the `doc.go` paragraph beyond "a paragraph a new reader could understand" (L99), and the style checker "reads a Go tree" only. The plan itself still uses SSE, JSON-RPC, TOTP, seccomp, Landlock ruleset, bwrap, `connectOverCDP`, DevTools port, `SUDO_ASKPASS`, `age`, salted and hashed, constant time, symlink, process group, build tag, and more without explaining them on first use (previous finding 127, unchanged), which is the standard the design holds itself to.

**Fix:** extend rule 5 with "and every document a worker writes: briefs, `ARCHITECTURE.md` sections, `PROGRESS.md`, `doc.go`, the README; a technical term is explained the first time it appears"; add a short glossary at the top of Part 3; have the orchestrator's gate read include the wave's `ARCHITECTURE.md` section for this.

### 12. Layout in Part 2 matches ARCHITECTURE.md and CLAUDE.md — **FAIL**

Mismatches found:

1. **The import rule is stated backwards.** WP L154 and AR L22: "Packages import only downward in this list." In both lists `internal/contract` (no dependencies) is at the top and `internal/replay` (imports nearly everything) is near the bottom, so "downward" would let `contract` import everything and `replay` import nothing. The list is in build order; the rule must read "a package imports only packages listed **above** it." And `cmd/nerdgenie`, which imports everything, sits at the **top** of Part 2 (WP L117), above `internal/`, contradicting either reading. Move it to the bottom.
2. `internal/lint`: Part 2 lists it last (WP L145, after `replay`) though it is wave 1; AR L31 lists it after `config`. AR L31 also says "used only in tests" while WP L204 says "wired into `make check`" (a command). Pick one.
3. **The Signal channel's package.** AR L43 says `internal/signal` holds "the signal-cli client, linking, pairing, and the Signal channel." WP L244 puts the channel at `internal/channel/signal`. AR L40's `internal/channel` row does not mention it.
4. Part 2 omits every sub-package the briefs name (`tool/file`, `tool/shell`, the five `tool/*` folders, `signal/pairing`, `channel/signal`, `memory/capture`, `skill/learn`, `skill/browser`, `browser/login`, `browser/handoff`), so the "import only ..." rule cannot be applied to them. AR folds them into parent rows but AR L47 puts `internal/skill` in wave 5 while `skill/browser` is wave 6.
5. `skills/qa/` (WP L278) is a new top-level repository folder that appears in neither Part 2 nor AR nor `REPO_MAP.md`'s roots.
6. The browser protocol method list differs: WP L192 names nine methods "open, read, click, type, press, scroll, act, tabs, login-fill" plus shapes; AR L77 names eleven, adding `screenshot` and `health` and spelling `loginFill`; brief 6.1 (L270) also names "the labeled screenshot." The wave-0 worker will write whichever list it reads.
7. `LICENSE`, `.github/workflows/`, `docs/DEPENDENCIES.md`, `docs/PROGRESS.md`, `docs/briefs/`, and `worker/browser/package.json` are created by briefs but absent from the Part 2 tree. Minor.

CLAUDE.md names no package that is missing elsewhere.

### 13. Built to expand: new channel, tool, skill, provider is a new file against a contract; stated and tested — **FAIL** (one line)

Tested in substance: two real providers against `contract.Model` (1.5), a user tool loaded through the protocol (2.3 "a user tool in a folder registers and runs"), a skill folder loaded (5.3), and a second channel, because 4.3 repeats "the wave-3 functional tests ... through the Signal channel" after the fake channel ran them in wave 3. What is missing is the sentence: the plan never states the design's rule (CP §6) that a new channel, tool, skill, or provider is one new file against a `contract` interface and touches nothing else, and the terminal is not a `contract.Channel` at all (AR L73: the socket is separate; Signal "runs inside the program"), so the only real `Channel` is Signal.

**Cheapest fix:** in 3.3 make the local socket the first `contract.Channel` implementation, and in 4.3 say "the functional suite is a table over every `Channel` implementation and runs unchanged for the socket, Signal, and the fake." Add one sentence to Part 2: "A new channel, tool, skill, or provider is one new file against its `contract` interface; the functional suite passing unchanged through it is the test of the extension point."

### 14. MVP scope is terminal and Signal only — **PASS**

No third channel appears anywhere; LM Studio (3.4) is a model server, not a channel; `/readyz` is a health probe. Note for item 1: the desktop worker (7.2) is built but not in the goal line.

### 15. Design features, brief, test, live test — **FAIL** (see table)

| Design feature | Design section | Brief | Tier-one test named in the brief | Live (tier-two) test | Verdict |
|---|---|---|---|---|---|
| History (event log) | CP §1, §4 "History is the log" | 1.2 `internal/log` | every read shape; 10,000 events replayed; kill mid-write; fuzz on the decoder | none named; covered only indirectly by any live functional test | PASS tier one |
| State (task record) | CP §4 | 1.3 `internal/record` | round-trip golden; each rule rejects a bad edit; forty-step; rewind; fuzz | forty-step "in both tiers" (WP L82), live variant undefined | PARTIAL |
| Working context | CP §4 layers table | 2.1 `internal/context` | golden prompt for 24k and 200k; nothing above the cache line changes over ten turns; folding readable by id; cost line | forty-step on Qwen and Opus (same caveat) | PARTIAL |
| Record shaped like an operations order, who writes what | CP §4 sections table | 1.3 types; 2.5 `task` tool; **Situation and harness stop lines: no brief** | 1.3 and 2.5 tests | none | FAIL |
| Done-check | CP §4, §5 | 2.2 `internal/review` | "a done-check that fails on one line" | forty-step assertion two, both tiers | PASS |
| After-action review | CP §4 | 2.2 review; 5.4 skill offer; 5.5 functional | "the review hands off the fourth answer"; 5.5 "the review writing to memory" | wave-5 gate `make live` (WP L65 covers every feature a wave adds) | PASS |
| Checkpoints and rewind | CP §4 | 1.3 | "rewind" | none; `/tasks 17 back 3` exists only inside 3.4's "every command" test | FAIL live |
| Tiered folding | CP §4 | 2.1 (window to record line); 1.3 (record fold at 3,000 tokens); **tier three: no brief** | 2.1 "folding keeps every exchange readable by id"; 1.3 names no fold test at all | forty-step assertion three; forty steps may never reach the cap, so folding may never fire | FAIL |
| Cost line | CP §4 | 2.1 | "the cost line matches the fake provider's counts" | token cost recorded in `PROGRESS.md` (WP L65); no assertion that the line matches real usage and cached counts | PARTIAL |
| Any-model context rule | CP §4 "one rule, sized to the model" | 2.1 | golden prompts at 24k and 200k | Qwen (small) and Opus (large) in tier two | PASS (where a model's context length comes from is unstated) |
| Task budget of rounds, tokens, minutes; header line | CP §3 rule 3, §4 header | rounds 2.2; minutes 4.5; **tokens and the header: no brief** | none | none | FAIL |
| Read a past result by id | CP §4, §7 | **no brief** (2.3 is file-only) | none | forty-step assertion three assumes it | FAIL |

### 16. Every worker told to read ARCHITECTURE.md, REPO_MAP.md, NERDGENIE_PLAN.md — **FAIL**

Only the orchestrator is told: WP L9 "Read this whole plan, then `CLAUDE.md`, `ARCHITECTURE.md`, and `docs/NERDGENIE_PLAN.md`." For workers there are only claims: WP L31 "Three files ... are context for every worker," AR L3 "It is read by every worker before starting a brief," CL L5 to L10 the reading order. The brief template (WP L86) lists package, job, interface, references, tests, and done, and no reading line. A sub-agent launched with a brief as its prompt gets `CLAUDE.md` only if the harness injects it, which the plan cannot rely on and never checks.

**Fix:** WP L86: "Every brief begins with the same first line: *Before anything else, read `CLAUDE.md`, `ARCHITECTURE.md`, `REPO_MAP.md`, and `docs/NERDGENIE_PLAN.md`, in that order, then this brief.*" and WP L88: the worker's report states it did.

### 17. Dependency order: no brief imports a same-wave or later package — **FAIL**

| Wave | Brief | Imports | Problem |
|---|---|---|---|
| 1 | 1.2, 1.3, 1.4, 1.5 | `internal/testkit` (1.1): fake clock, temporary home, golden files, the fake provider server ("both APIs against the fake provider server," L206), the forty-step fixture ("the forty-step fixture from testkit passes," L202) | Four of five wave-1 briefs need the fifth. WP L69 says the framework is "built in wave 1, before any feature," and then puts the features in the same wave |
| 1 | 1.1 | the loop, context builder, review (wave 2) for the replayer ("re-runs it against the current code," L81) and for the fixture's forty rounds and done-check | Later-wave dependency |
| 2 | 2.5 `skill`, `schedule` thin tools | "over the `contract` interfaces" (L220) | `contract` (L158 to L170) has no `Skill` or `Schedule` interface, and testkit has no fake for either |
| 3 | 3.2 shell | `internal/sandbox` (3.1, same wave); the vault's sudo entry (4.4, later wave) | Same-wave and later-wave |
| 3 | 3.4 and 3.5 | both write into `cmd/nerdgenie` (3.4: "the `init` subcommand in `cmd/nerdgenie`"; 3.5: "Package `cmd/nerdgenie`") | Two workers own one package, against WP L27 |
| 3 | 3.5 `serve` | the queue and socket (3.3) and the command table (3.4) | Same-wave |
| 4 | 4.3 Signal channel | the signal-cli client (4.2, same wave) | Same-wave (previous finding 58, unchanged) |
| 4 | 4.2, 4.4, 4.5 | all add subcommands to `cmd/nerdgenie` (`signal link`, the `init` change, `backup`, `restore`) | Three workers in one package |
| 4 | 4.4 redaction "on every reply, log line, and tool result" | must be called from `log` (1), `tool` (2), `channel` (3) | Upward import; `contract` has no `Redactor` |
| 4 | 4.5 "nightly" backup | a scheduler (7.1) or a systemd timer nobody names | Later-wave or unowned |
| 5 | 5.4 learn | the folder format and saver in 5.3 (same wave) | Same-wave (previous finding 59) |
| 5 | 5.5 | 5.1 to 5.4 | Cannot start with them; not stated |
| 6 | 6.3 login, handoff | the worker lifecycle in 6.2 (same wave) to reach `login-fill` | Same-wave |
| 6 | 6.4 browser skills | 6.2 for replay "through the cascade" | Same-wave |
| 7 | 7.4 nightly job | `internal/schedule` (7.1, same wave) | Same-wave (previous finding 61) |
| 7 | 7.1 "the `schedule` tool from wave 2 now talks to this package"; 7.3 `update` in `cmd/nerdgenie` | a wave-2 tool importing a wave-7 package unless a `contract.Schedule` interface exists; `cmd/nerdgenie` touched again | Upward |

Part 6 (L335) says this blocker is fixed; it is fixed for wave 2 and reopened in wave 1.

**Fix:** move `internal/testkit` (or at least clock, home, golden, fake provider server, the fixture data) into wave 0; add `Skill`, `Schedule`, `Sandbox`, `PasswordSource`, `Redactor`, and `BrowserWorker` to `contract` with fakes in testkit; make `cmd/nerdgenie` orchestrator-owned with a subcommand registration table so each brief adds one file; state that 3.5, 5.5, 6.5, 7.5 start after their neighbors merge; 4.3, 5.4, 6.3, 6.4 code against the `contract` interface or the fake over the wire.

### 18. Reference paths — **PASS**

Spot-checked against the Part 5 table (`/home/jared/Code/<repo>` at commit) and the local clones at the same commits; all ten and, in fact, every path in every brief resolve:

1. `~/Code/hermes-agent/gateway/delivery_ledger.py` (1.2) yes
2. `~/Code/prime-agent/packages/coding-agent/src/core/refinement/refinement.ts` (1.3) yes
3. `~/Code/zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs` (1.5) yes
4. `~/Code/openclaw/packages/tool-call-repair/src/grammar.ts` (1.5) yes
5. `~/Code/opencode/packages/opencode/src/permission/arity.ts` (2.4) yes
6. `~/Code/openclaw/src/infra/net/ssrf.ts` (2.5) yes
7. `~/Code/zeroclaw/crates/zeroclaw-runtime/src/security/landlock.rs` (3.1) yes
8. `~/Code/openclaw/extensions/signal/src/sse-reconnect.ts` (4.2) yes
9. `~/Code/openclaw/extensions/browser/src/browser/pw-session-cdp-transport.ts` (6.1) yes
10. `~/Code/hermes-agent/cron/lifecycle_guard.py` (7.1) yes

Paths outside the table: `docs/reference/codex/`, `docs/reference/browser-use/`, `docs/reference/moltis/` (all present, all in Part 5), `docs/research/16-browser-agent-spec.md` (present). No brief names a path outside the repos or `docs/reference`. Nits: WP L11 writes `NERDGENIE_PLAN.md` and `HARNESS_V2.md` without the `docs/` prefix; the HomeRecon row pins no commit; `~/Code/openclaw/extensions/cua-computer/` (7.2) is not among the purposes listed in the OpenClaw row.

### 19. Anything else that stops a signature

Numbers and names that conflict:

1. **The harness instruction text is 361 words** (382 with headings); brief 2.1 asserts "under three hundred words" (WP L212) and CP §5 claims "under three hundred words." The test fails on the golden text. Trim the design text or change the number.
2. **6.2 "the seven browser tools from section 9"** (WP L272): section 9 lists no tools, section 7 does, and 6.3 builds two of the seven, so 6.2 owns five. Previous findings 87 and 95, unchanged.
3. **Browser protocol methods:** nine in WP L192, eleven in AR L77, `login-fill` versus `loginFill`.
4. **"One message per reply" (4.3, L244) versus "four-thousand-character chunks" (4.2, L242).**
5. **Handoff "blocks the task until the user replies" (6.3, L274)** versus the seven-minute tool deadline (4.5, L248) and CP §3 rule 10 "a question ends the turn." A captcha can take an hour. Make handoff end the turn in `waiting`.
6. **"The monthly memory test harness" (5.5, L264)** versus "a nightly job runs the memory test" (7.4, L292).
7. **`make live` in a clean container (7.5, L294)** versus CL L36 "development machine only" and the visible Chrome window. A container has no Ollama, no display, no Chrome window. Containers run `make check`; `make live` runs on the dev machine.
8. **Human trials:** L91 versus L9 and L308 (item 7).
9. **`make check` contents:** WP L98 versus CL L35 (item 6).

Tests that cannot be written as described:

10. 2.2 "a question that ends the turn waiting": nothing says how the loop tells a question from a final answer (previous finding 4). Define: a reply with no tool calls while any plan step is open puts the task in `waiting`; only the done-check reaches `done`.
11. 2.1 golden prompts "for a 24k model and a 200k model" and 1.3's "cap of three thousand tokens": how tokens are counted is never stated (tokenizer, characters divided by four, or the provider's last usage report). Previous finding 69.
12. 3.5 "the installer ... ends with a passing `doctor`" (item 3).
13. 1.3 "the forty-step fixture from testkit passes" in wave 1 (item 9).

Vague "done":

14. 3.4 "prints the five commands a new user needs": not named.
15. 3.5 "`serve` answers `/readyz` within one second": over what? The socket speaks JSON lines; `/readyz` implies HTTP on an unnamed port.
16. 5.1 "the twenty-question memory test against a recorded session": neither fixture is owned.
17. 0.1 "a `LICENSE` file": which license? Previous blocker 135 asked for the choice.

Rules the plan breaks itself:

18. WP L27 "one brief, one package" versus 1.4, 1.5, 2.2, 3.4, 4.3, 6.3, 7.2 (two packages each) and 4.5 (seven mechanisms in one package, against rule 4). Previous findings 82 and 99.
19. `internal/log` and `internal/context` shadow the standard library's `log` and `context`; every file in `internal/context` that takes a `context.Context` must alias the import. Previous finding 64. Rename to `eventlog` and `prompt`.
20. The dependency starting list (WP L192) omits a TOML parser (needed by 1.4; the standard library has none), a Landlock binding and a seccomp library (3.1; or drop seccomp, which the design never asked for), and a QR encoder (4.2). Each will trigger "ask the orchestrator first" in wave 1 and wave 3.
21. Brief 0.1 is one worker writing the whole of `contract` (every interface, the record types, the full configuration struct with defaults, the user-tool protocol), the browser protocol document, the Makefile, CI, the repo-map generator, and the three living documents. It is the largest brief in the plan and the one every other brief depends on, with a single test ("every interface compiles and has a doc comment"). At minimum the orchestrator should review `contract` line by line against CP §4 to §7 before wave 1 starts, and the plan should say so.
22. The configuration struct is frozen in wave 0 (`contract`, "changed rarely") but 1.4's field list (L204) omits fields later briefs need: the SearXNG address (2.5), the policy file path (2.4), the pacing profile and daily action budget (6.2), the timezone (7.1), the release manifest address (7.3), and each model alias's context length (2.1). Each becomes a fix brief against `contract`.
23. Shared files (`go.mod`, `go.sum`, `Makefile`, `docs/DEPENDENCIES.md`, `docs/PROGRESS.md`, `internal/testkit`) have no owner; five branches per wave will conflict on them. Previous finding 137. Name the orchestrator.
24. No time or effort budget per brief (previous finding 126): a worker with a ninety-percent bar and no stop rule can run indefinitely.

Security (the plan builds a sandbox, then leaves a door):

25. 2.3: `write` and `edit` roots "never include the vault, the browser profile, or `~/.ssh`" but say nothing about `~/.nerdgenie` itself. The model can `write` an executable into `~/.nerdgenie/tools/`, and 2.3's registry loads "any executable in `~/.nerdgenie/tools/`" with whatever permission class the file declares. Exclude `~/.nerdgenie` from the roots and require user tools to be installed by the user, not the model.
26. The security review (7.5) is a single pass at the end; there is no threat-model document and no gate at waves 3 and 4, so sandbox, vault, sudo, and pairing bugs ship through two human trials before anyone looks. Previous blocker 136 asked for `docs/SECURITY.md` and per-wave checks.

---

## Part B: the 24 blockers from the previous review

"Fixed" means the brief now contains what the finding asked for; "gap" means the plan claims it in Part 6 but the brief falls short of the finding.

| # | Blocker (previous review) | Part 6 claims | Actually | Evidence |
|---|---|---|---|---|
| 9 | Forty-step fixture never written | Brief 1.1 | **Gap** | 1.1 builds it (L198) but it cannot run in wave 1 (needs 2.1, 2.2) and the live variant is undefined (item 9) |
| 13 | User-tool protocol undefined | `contract`, wave 0 | Fixed | L170 defines `--describe` and stdin JSON |
| 14 | Config fields not listed | Brief 1.4 | **Gap (minor)** | L204 lists fields; six fields later briefs need are missing (item 19 #22) |
| 15 | Fake channel before its interface | Interfaces in wave 0 | Fixed | L158 to L163 |
| 16 | `task`, `skill`, `schedule` tools never built | Brief 2.5 | **Gap** | Tools exist (L220); the `contract` interfaces they are "thin tools over" do not (item 17) |
| 18 | No search provider | SearXNG in 2.5 | **Gap** | Chosen (L220); not installed or listed as a prerequisite (item 3), so a fresh install cannot search |
| 24 | No login-wall or captcha detector | Brief 6.1 | Fixed | L270 wall detector with fixtures |
| 34 | `write` and `edit` not path-limited | Brief 2.3 | Fixed | L216; but the roots must also exclude `~/.nerdgenie` (item 19 #25) |
| 36 | No nightly backup or restore | Brief 4.5 | **Gap (minor)** | L248 has `backup`, `restore`, round-trip test; "nightly" has no trigger before wave 7 and the backup key is unnamed |
| 43 | No Signal QR linking | Brief 4.2 | Fixed | L242 `nerdgenie signal link` with QR and a test against the fake |
| 47 | No installer, no prerequisites | Brief 3.5 | **Gap** | L234 installer exists; release source, Node and the worker bundle, SearXNG, display, Java-or-native signal-cli, non-interactive `init` missing (item 3) |
| 48 | No first-run onboarding | Brief 3.4 | **Gap (minor)** | L232 `init` is good; does not link or pair (item 4) |
| 55 | Wave 2 not parallel | Provider and repair to wave 1 | **Gap** | Wave 2 is fixed; wave 1 now has the same defect (item 17) |
| 56 | Wave 3 not parallel | Tool contract to wave 2 | **Gap** | 3.2 imports 3.1 and the wave-4 vault; 3.4 and 3.5 share `cmd/nerdgenie`; 3.5 imports 3.3 and 3.4 |
| 57 | Wave 4 fully serial | Channel core to wave 3 | **Gap** | 4.3 imports 4.2; 4.2, 4.4, 4.5 all edit `cmd/nerdgenie`; redaction imports upward |
| 60 / 123 | Browser wave a single lane | Split into two halves | Fixed (wording) | L270; the launch timing sentence contradicts itself (item 8) |
| 93 | Final pass names no command, image, or prerequisites | Brief 7.5 | **Gap** | `make check` and `make live` named; Ubuntu and Debian named without versions; `make live` cannot run in a container (item 19 #7) |
| 110 | Fakes the briefs need that testkit lacks | Four fakes added | **Gap** | Permission decider, tool registry, memory store, search server added (L76 to L79); ten more still missing (item 9) |
| 132 | No CI | Brief 0.1 | Fixed | L192 GitHub Actions on Linux with bwrap; no nightly live job, acceptable given "development machine only" |
| 133 | No Makefile | Brief 0.1 | **Gap (minor)** | Targets named (L192); `make test` defined only in CL L34 and runs no integration or functional tests (item 6) |
| 134 | No `go.mod`, Node version | Brief 0.1 | **Gap (minor)** | `go.mod` yes; `worker/browser/package.json` and a Node pin nowhere |
| 135 | No license, no third-party notes | Brief 0.1 | **Gap (minor)** | `THIRD_PARTY.md` exists in the repo already; the Nerd Genie license is still "a `LICENSE` file" with no name |
| 136 | No security review | Brief 7.5 | **Gap** | One review at the end; no threat-model document, no gate at waves 3 or 4 |
| 123 (doc) | Browser protocol document before the fake | `PROTOCOL.md` in wave 0 | Fixed | L192; method list differs between WP L192 and AR L77 (item 12 #6) |

Fully fixed: 8 of 24. Fixed with a minor gap: 7. Not fixed enough to hand out: 9 (9, 16, 18, 47, 55, 56, 57, 110, 136).

---

## Sign or do not sign

**Do not sign.** The shortest list of edits that would get a signature, in the order I would make them:

1. **Wave parallelism (item 17).** Move `internal/testkit` to wave 0 (wave 0 becomes two workers, or one worker in two steps). Add `Skill`, `Schedule`, `Sandbox`, `PasswordSource`, `Redactor`, and `BrowserWorker` to `contract` with fakes. Make `cmd/nerdgenie` orchestrator-owned with a one-file-per-subcommand registration table. State that 3.5, 5.5, 6.5, and 7.5 start after their neighbors merge. Have 4.3, 5.4, 6.3, and 6.4 code against `contract` or the fake over the wire.
2. **The import rule (item 12 #1).** Change "downward" to "a package imports only packages listed above it" in WP L154 and AR L22, and move `cmd/nerdgenie` to the bottom of the Part 2 list.
3. **Three live models (item 2).** Add a named GPT model through the real OpenAI API to WP L7, L65, L278, CL L36, AR L99, and `make live`; pin the Ollama tag for Qwen.
4. **Own the missing MVP pieces (items 1, 15).** `read r7` to 2.3; Situation, harness stop lines, the token budget, and the question-versus-answer rule to 2.2; the third fold tier plus a fold test to 1.3; `/pair <code>` and `init` offering linking to 4.3 and 4.2; the `/undo` snapshot to 2.3; a "registers these commands" line in 4.4, 5.1, 5.3, 6.2, 7.1; SearXNG or a keyless fallback to 3.5. Add a "left for after the MVP" paragraph and decide whether the desktop worker is in the goal.
5. **The gate (items 6, 9).** `make test` = unit + integration + functional (fake model) + five-second fuzz smoke; `make fuzz` = one minute per target; a coverage threshold in `make check`; define the live variant of the forty-step fixture and say it joins the gate from wave 2; delete the replayer from 1.1; add Vitest and fast-check to 7.2.
6. **Every brief's first line (item 16):** read `CLAUDE.md`, `ARCHITECTURE.md`, `REPO_MAP.md`, `docs/NERDGENIE_PLAN.md`, then the brief.
7. **Install and update (items 3, 5).** Name the release source and add `make release`; a `--from <path>` flag and a non-interactive `init` for the container test; Node and the worker bundle in both the installer and `nerdgenie update`; migrate after readiness or restore the backup on rollback, with a test.
8. **The nine number conflicts (item 19 #1 to #9),** including the 361-word instruction text, "seven tools from section 9," the protocol method list, one-message versus chunks, handoff blocking, monthly versus nightly, `make live` in a container, the wave-5 human trial, and the contents of `make check`.
9. **Visual QA (item 7):** a five-line checklist per human trial and where the notes go.
10. **Plain English for documents (item 11):** extend rule 5, add a glossary to Part 3.
11. **Security:** exclude `~/.nerdgenie` from the `write` and `edit` roots (item 19 #25); name the license (0.1).

Everything else in this review is a should-fix that would cost a rework cycle but would not stop the first wave.
