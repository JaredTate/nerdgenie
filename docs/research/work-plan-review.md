# Review of `docs/WORK_PLAN.md` against `docs/NERDGENIE_PLAN.md`

Reviewer stance: senior engineering lead, adversarial, read-only. Both documents were read in full. Every reference path in the plan (105 of them) and every "search for" symbol (9) was checked on disk; all exist. The repository today contains only `README.md` and `docs/`; there is no `go.mod`, `Makefile`, CI configuration, or `LICENSE`.

Severity key: **blocker** = I would not hand this wave to workers as written; **should fix** = will cost a rework cycle if not fixed; **nit** = polish.

Findings are numbered continuously across sections so the count at the end is unambiguous.

---

## 1. Coverage against the design

Walked section by section through `NERDGENIE_PLAN.md`. Each finding names the design section and the brief that should own it.

### Design section 3 (how the agent works) and the ten rules

1. **Design 3, the router; brief 2.4 and 6.3.** The design says the router sorts every message into command, skill trigger, or task, and runs commands and skills "without calling the model." Brief 2.4 lists "route" as a loop step but never says what the router matches on; brief 6.3 adds a "router hook" three waves later; brief 4.2 builds commands. Nobody owns the router. **should fix.** Give brief 2.4 an explicit `Router` interface with two hooks (command table, skill triggers) and a test that a hook match never reaches the fake model.
2. **Design 3, rule 3, "a long task also gets a budget of rounds, tokens, and minutes"; record header "budget: 6 of 20 rounds, 41k tokens, 9 min".** Rounds live in 2.4 and the fifteen-minute deadline in 5.5, but no brief tracks or enforces a per-task token budget or writes the header line. **should fix.** Add the budget fields and their enforcement to brief 2.4 with a test that a task stops at its token budget.
3. **Design 3, rule 8, "an unattended run that hits a question stops and reports."** Brief 8.1 tests it, but brief 2.4 never mentions an unattended flag on the loop, so wave 8 will have to re-open the loop. **should fix.** Add an `Unattended bool` input to the loop in 2.4 with the stop-and-report test.
4. **Design 3, rule 10 and design 7, "asking the user a question is not a tool."** No brief defines how the loop tells a question from a final answer, so 2.4's test "a question that suspends" cannot be written unambiguously. **should fix.** Define it in 2.4: a reply without tool calls while the plan is incomplete puts the task in `waiting`; the done-check is the only path to `done`.
5. **Design 3, "one hundred queued messages".** Brief 4.1's queue has no cap stated. **nit.** Add the cap and a test that message 101 is refused with a reply.
6. **Design 3, "after a crash, the agent replays the log to rebuild its state"; design 11 self-fixing table "repair state and replay the log, at startup".** Brief 1.2 has a replay function and 5.5 has an integrity check, but no brief defines the startup sequence that uses them. **should fix.** Add to brief 4.4: `serve` opens the database, runs the integrity check, reloads every non-done task from its last checkpoint, drains the disk queue, then answers `/readyz`; test it.

### Design section 4 (state and the record)

7. **Design 4, "Situation: the harness, from the world as it is right now" and "the harness adds the budget and login walls" to the stop list.** No brief writes the Situation section or the harness-added stop lines. **should fix.** Assign both to brief 2.4 (the loop is the only place that sees tool results and tabs) with a golden-record test.
8. **Design 4, tiered folding, third tier "then it lives only in the log."** Brief 1.3 folds to single lines and says "nothing deleted"; brief 2.3 folds the recent window into the record. Nobody moves a folded line out of the record into log-only when the record passes its cap, so the record grows without bound on a long task. **should fix.** Give 1.3 the second fold (record line to log-only, readable by id) with a test that a record with 200 results stays under 3,000 tokens.
9. **Design 4, "the proof": the forty-step task.** Briefs 1.3, wave 2's intro, and 8.5 all invoke "the forty-step task" but no brief writes the fixture (the forty steps, the tool results, the three assertions). Three workers will write three different tasks. **blocker.** Add the fixture to brief 1.1 as `internal/testkit/testdata/forty-step-task/` with a scripted fake-model conversation and the three assertions as helper functions.
10. **Design 4, the cost line "this turn: 6.1k in, 5.2k of it cached, 0.4k out" in the record header.** Brief 2.3 "emits" it but brief 1.3's record has no header field for it and no brief writes it into the record. **nit.** Add the header field to 1.3 and say in 2.3 that the loop writes it.

### Design section 5 (what the model is told)

11. **Design 5, the under-300-word harness instructions.** No brief owns the text. Brief 2.3 mentions a "harness rules" layer without saying the text is the design's verbatim. **should fix.** Give brief 2.3 the file `internal/context/harness_rules.txt` as a golden copy of section 5 with a test that it is under 300 words and appears first in every prompt.
12. **Design 5, "the harness checks that the ask and the intent have not changed by so much as a character" after every turn.** 1.3 makes them immutable in the struct; nobody checks after a turn that the model's `task` update did not smuggle a change. Covered only if the `task` tool exists, and it does not (finding 16). **should fix.** Fold into the `task` tool brief.

### Design section 6 (extension points)

13. **Design 6, user tools: "a small program in a `tools/` folder that follows [the contract]".** Neither document says the language, how the program declares its name, description, input fields, and permission class, or how the harness calls it. Brief 3.1's test "registration of a folder tool" cannot be written. **blocker.** Define it in 3.1: one folder per tool with `tool.toml` (name, description, inputs, class) and an executable that reads JSON on stdin, prints text on stdout, runs inside the sandbox with the same output cap.
14. **Design 6, "the model's name and the fallback chain are set in the configuration."** Brief 1.4 never lists the configuration fields. The provider (2.1), the chain, the API key, the base address, the Signal number, the paired senders, the policy file, the pacing profile, and the daily budget all need fields, and each later worker will invent their own. **blocker.** Enumerate the full `config.toml` schema in brief 1.4 as a golden example file; later briefs may only add fields by fix-brief.
15. **Design 6 and 12, the channel contract "six things".** The Go interface is defined in brief 4.1 (wave 4) but the fake channel that implements it is built in brief 1.1 (wave 1). See finding 43. **blocker.**

### Design section 7 (tools)

16. **Design 7, eighteen tools.** The briefs build fifteen: `read`, `write`, `edit`, `search` (3.2), `shell` (3.3), `web`, `memory` (3.5), seven `browser_*` (7.2 and 7.3), `computer` (8.2). Missing: **`task`** (the model's only way to write the plan, decisions, failures, facts, and pins), **`skill`** (view, run, save), **`schedule`** (create a job). Without `task`, nothing in wave 2 can be driven by a model. **blocker.** Add `task` to brief 3.1 (or a new 3.6) with a test per record rule; add `skill` to 6.3; add `schedule` to 8.1.
17. **Design 7, `read` "reads a past result by its id".** Brief 3.2's `read` handles files only. **should fix.** Add result-id reads (through `internal/log`) to 3.2 with a test for `read r7`.
18. **Design 7, `web` "searches the web."** No search provider is named anywhere in either document or the research; every search API needs either a key (Brave, Serper, Tavily, Exa) or a self-hosted server (SearXNG). **blocker.** Pick one in brief 3.5 (recommend SearXNG at a configurable address, no key, plus DuckDuckGo HTML as a keyless default), put the address in 1.4's config, and add a fake search server to testkit.
19. **Design 7, the "cap on how much text [a tool] may return".** Brief 3.1 says "the output cap from the plan," but the design never gives a number; the research suggests 4,000 characters for local models and 16,000 for cloud. **should fix.** State the numbers in 3.1.
20. **Design 7, the tool table shows all eighteen to every model (no `tool_search` deferral).** No brief states this rule and no test checks that the tool layer fits a 24k-context local model (the research measures roughly 150–220 tokens per tool, so about 3,000–4,000 tokens for eighteen). **should fix.** Add to 2.3's golden-prompt test an assertion on the tool layer's token count for the 24k model, and state "no deferral" in 3.1.

### Design section 8 (skills)

21. **Design 8, the dry-run test "runs the procedure up to the first step that cannot be undone and then stops."** Brief 6.3 covers format, loader, permissions, changelog, and trigger; the dry run is absent. **should fix.** Add it to 6.3 with a test on a fixture skill that has one irreversible step.
22. **Design 8, "the user demonstrates the task once while the harness records it."** Brief 7.4 records browser steps only. For shell or file tasks, there is no recording path. **nit.** Say in 6.3 that demonstration recording is browser-only in the first version, or add shell-step recording to 6.3.
23. **Design 4 and 8, the after-action review's fourth answer "goes into memory or into a skill."** Brief 2.5 hands it to memory only; nothing updates a skill's known failures or steps from a review. **should fix.** Add to 2.5 an interface `SkillPatcher` and to 6.3 an implementation that appends a "known failure" line to the skill named in the record, with a test.

### Design section 9 (the browser)

24. **Design 9, the flow diagram's "Login wall or captcha?" check and "if at any point it hits a login wall, a two-factor prompt, or a captcha, it hands off."** No brief builds the detector. 7.5 tests "hit a captcha fixture and hand off" against code nobody writes. **blocker for wave 7.** Add wall detection (login form, 2FA field, known captcha frames) to brief 7.1 as a field in every snapshot, with fixtures for each.
25. **Design 9, "every action carries an expected result, and the browser worker checks that result."** Brief 7.1 lists settle and diff but never the expectation field or the "expected versus seen" report. **should fix.** Add the `expect` field and the mismatch report to every 7.1 action, with a test where the expectation is not met.
26. **Design 9, "if a click produced no visible change, it retries once by clicking at the element's spot on the screen."** Brief 7.1 only "reports it." **nit.** Add the coordinate retry and a test.
27. **Design 9, "everything a human can do": frames, uploads, PDF-as-text, keyboard shortcuts, drag and drop, hovering, back and forward.** Brief 7.1 covers tabs, dialogs, downloads only. **should fix.** Either enumerate the missing seven in 7.1 with one test each, or state in the plan that they are left for a later wave.
28. **Design 9, "each website gets a daily budget of actions".** Brief 7.2 covers it but says nothing about where the counter lives, which day boundary, or what the model is told. **nit.** Persist in SQLite, reset at local midnight from the config timezone, and reply with the reset time.
29. **Design 9, "one long-lived Playwright program per browser profile."** The design implies more than one profile; the briefs assume one. **nit.** State one profile in the first version.
30. **Design 9, the "logged in?" fact in the Situation ("tab t1: the compose page, logged in as the company").** No brief detects login state; it falls out of the missing Situation writer (finding 7) and wall detector (finding 24). **should fix.** Covered by fixes to 7 and 24.

### Design section 10 (desktop, command-line tools, memory)

31. **Design 10, "actions inside that application that cannot be undone still get a preview."** Brief 8.2 promises "a preview for every irreversible action" but nothing says how a desktop click is classified as irreversible. **should fix.** Define it in 8.2: every `computer` action is class I and previewed unless the granted app's skill marks the step reversible.
32. **Design 10, "the agent reads the tool's help text and documentation".** Brief 6.4 does not say where docs come from (`--help`, man page, a URL through `web`). **nit.** Say all three, in that order.
33. **Design 10, the memory hint "three lines from search."** Brief 6.1 never says what the search query is. **nit.** Define it as the last user message plus the current plan step.

### Design section 11 (safety, vault, reliability)

34. **Design 11, "every shell command and every file-writing tool runs inside a sandbox."** Brief 3.3 sandboxes `shell`; brief 3.2's `write` and `edit` are not sandboxed or path-limited, so the model can write `~/.ssh/authorized_keys` through `write` while `shell` is walled off. **blocker.** Add a path allowlist (Landlock or an explicit denylist of the vault, the profile, `~/.ssh`, and `~/.nerdgenie`) to 3.2 with a test that `write` to `~/.ssh` is refused.
35. **Design 11, "one redaction pass runs on everything that leaves the program."** Brief 5.4 builds the function; no brief wires it onto channel sends, log lines, and tool results. **should fix.** Add to 5.4 the wiring points (4.1 send, 1.2 write, 3.1 result) with a functional test that a vault value never appears in a reply or the log.
36. **Design 11, "the database, the vault, and the browser profile are backed up every night in encrypted form."** No brief creates the nightly backup, and brief 5.5's test "the nightly backup restored" restores a backup nobody makes. The encryption key for backups is not named. No restore command exists. **blocker.** Add a brief (or extend 8.3): `nerdgenie backup` and `nerdgenie restore <date>`, run nightly by 8.1, encrypted with the vault key, kept for seven days, with a round-trip test.
37. **Design 11, sudo "with the sudo password from the vault."** Brief 3.3 (wave 3) runs sudo on approval, but the vault arrives in wave 5. Where does wave 3 get the password? **should fix.** Make 3.3 take a `PasswordSource` interface, ship a fake in testkit, and let 5.4 supply the real one.
38. **Design 11, "if the sandbox is missing from the machine, the shell tool is turned off."** Covered by 3.3. But the plan adds a **seccomp filter** the design never asked for, with no syscall list, and seccomp in Go means either cgo `libseccomp` or a hand-built BPF program. **should fix.** Drop seccomp from 3.3 (bwrap plus Landlock is what the design says) or name the library and the syscall list.
39. **Design 11, self-fixing tiers "propose a repair from its own logs" and "edit its own code in a branch."** No brief. **nit.** Say in the plan that both are left out of the first eight waves.

### Design section 12 (interfaces)

40. **Design 12, `/screen`.** Brief 4.2 says "the commands from section 12," but `/screen` needs the browser (wave 7) and the desktop (wave 8), which do not exist in wave 4, and no later brief adds it. **should fix.** Move `/screen` to brief 7.2 (browser) with a note that 8.2 extends it.
41. **Design 12, `/undo` "reverts the last turn's file changes".** No brief snapshots the files a turn touched. **should fix.** Add to 3.2 a per-turn snapshot of every file `write` and `edit` touch (copy-before-write into `~/.nerdgenie/undo/<turn>/`), and give `/undo` to 4.2 with a test.
42. **Design 12, `/model` sets the model, `/jobs`, `/memory`, `/skills`, `/vault`, `/pause`, `/resume`.** Brief 4.2 owns the table in wave 4, but the bodies need waves 5, 6, and 8. No later brief says "add your commands to the table." **should fix.** In 4.2 build the table with the wave-4 commands only, and add one line to briefs 5.4, 6.1, 6.3, 8.1, 7.2 saying which commands they register.
43. **Design 12, Signal "linked to the user's Signal account as a secondary device."** No brief does the linking: running `signal-cli link -n nerdgenie`, rendering the returned URI as a QR code in the terminal, waiting for the phone to scan it. Brief 5.1 starts the daemon as if the account already exists. **blocker.** Add to brief 5.1 a `nerdgenie signal link` command with a QR renderer (unicode blocks) and a test against the fake signal-cli's link flow.
44. **Design 12, "unknown senders receive a pairing code."** Brief 5.2 builds the codes but never says how the user confirms one (a `/pair <code>` command in the terminal? an approval prompt?). **should fix.** Define `/pair <code>` in 5.2, register it in the command table, test the round trip.
45. **Design 12, "photos and files the user sends are saved, and the model can read them."** Brief 5.3 saves them to an inbox; the `read` tool returns text, and brief 2.1's provider interface has no image input, so the model cannot look at a photo. **should fix.** Add image content blocks to 2.1 (both APIs support them) and let `read` on an image file return an image block, with a test on a fixture PNG.
46. **Design 12, "the vault works only in the terminal."** Brief 4.2's single table has no way to mark a command terminal-only. **nit.** Add a `terminalOnly` flag with a test that Signal gets "use the terminal for this."

### Design section 13 (building it)

47. **Design 13, "ships as one static binary" and design 1 "runs on any Linux machine".** Nothing says what a fresh Linux machine must have installed: Go (no, the binary is prebuilt, but 8.3 "builds into releases/" implies a toolchain), Node and `playwright-core` (7.1), Chrome (7.1), signal-cli and its Java runtime (5.1), bwrap and a Landlock-capable kernel (3.3), ripgrep (3.2), `@trycua/cua-driver` (8.2), a display server for the visible browser (design 9), systemd user services (4.4). No brief writes an installer, an install-time check, or a prerequisite list. Brief 1.4's `doctor` checks only the home folder; brief 4.4's `nerdgenie install` writes only a unit file. **blocker.** Add a brief (wave 4 or 8): `nerdgenie install` checks every prerequisite by name and version, prints the one command to install each missing one for Debian and Fedora, and refuses to continue; `docs/INSTALL.md` lists them; `nerdgenie doctor` re-checks them.
48. **First-run onboarding.** No brief creates the home folder, writes the default persona files (`MEMORY.md`, `USER.md`, the agent's identity file), asks for the model and provider address, stores the API key (the vault is wave 5, so in wave 4 the key has no home), links Signal, pairs the phone, or shows the first `/help`. Brief 8.5's "README tells a new user how to install and pair in ten lines" is documentation, not code. **blocker.** Add `nerdgenie init` to brief 4.4: create the home, write the persona defaults from embedded templates, prompt for provider, base address, model, and API key (masked, into a 0600 file until 5.4 moves it to the vault), then print the next steps; extend in 5.1 with linking.
49. **Upgrades of a running install.** Brief 8.3 builds rollback and migrations, but not: how the user triggers an update (`nerdgenie update` appears in the layout but not in 4.4 or 8.3), where releases come from (download from a release page, or `go build` on the machine), what happens to the browser worker's `node_modules` and signal-cli on update, or what the user sees during the sixty-second window. **should fix.** In 8.3 define `nerdgenie update [version]` (download a signed tarball of the binary plus the worker bundle, verify, install, migrate, switch, wait for `/readyz`) and say the two outside programs are the user's to update.
50. **Uninstall.** Nothing. **should fix.** Add `nerdgenie uninstall` (remove the unit, stop the daemon, leave the home folder unless `--purge`) to 4.4 with a test.
51. **Design 13, "every failed task becomes a test" is built twice.** Brief 1.1 builds "a recorded-task replayer" in testkit; brief 8.4 builds `internal/replay` that does the same thing. **should fix.** Keep it in 8.4 (it needs the whole stack) and remove it from 1.1.
52. **Design 13, v4 "visual QA".** Brief 7.5 tests "the visual QA skill walks a fixture app and produces a report," but no brief builds a visual QA skill. **should fix.** Either add it as a fixture skill in 7.4 or drop the test.
53. **Design 13, "the forty-step task runs on every supported model size every time the software is built."** Brief 8.5 runs it once against a local model and a cloud model; nothing makes it run on every build, and running a cloud model in CI needs a key and a budget. **should fix.** Define a `live` job in CI that runs nightly with a cloud key and an Ollama model, separate from the pull-request job.
54. **Design 13, order of work "terminal, Signal, browser, skills and memory".** The plan puts skills and memory (wave 6) before the browser (wave 7), which is right because 7.4 needs 6.3, but the design says the opposite. **nit.** Update the design's sentence.

---

## 2. Dependency order

"Workers run in parallel" means any brief that imports a package built in the same wave cannot compile, let alone test, until its neighbor finishes. Imports below are what each brief's stated job requires, not what the brief says (most briefs say nothing).

| Brief | Package | Needs (earlier waves) | Needs (same wave: deadlock) | Needs a fake testkit does not have |
|---|---|---|---|---|
| 1.1 | testkit | none | none | the channel interface (4.1), the browser wire protocol (7.1), the forty-step fixture |
| 1.2 | log | testkit | none | a subprocess killer for the mid-write test |
| 1.3 | record | log, testkit | none | none |
| 1.4 | config | testkit | none | none |
| 1.5 | lint | none | none | none |
| 2.1 | provider | config, testkit | **2.3** (who places cache markers) | none (fake provider server exists) |
| 2.2 | repair | tool names (3.1, later wave) | none | none |
| 2.3 | context | record, testkit | **2.1** (token counts for the cost line) | a tokenizer, a fake memory hint, a fake tool list, a fake skill list |
| 2.4 | loop | record, log | **2.1, 2.2, 2.3, 2.5** | a fake permission function, a fake tool registry, a fake router |
| 2.5 | review | record | **2.1** | a fake memory sink |
| 3.1 | tool | log, config | none | a fixture folder tool (protocol undefined) |
| 3.2 | tool/file | log (result ids) | **3.1** (the contract) | ripgrep on the test machine |
| 3.3 | tool/shell, sandbox | permission (3.4, same wave) | **3.1, 3.4** | a fake sudo, a fake bwrap-missing path, a Linux kernel |
| 3.4 | permission | record (taint) | none | a fake approver |
| 3.5 | tool/web, tool/memory | testkit | **3.1** | a fake search server, a fake memory store, a fake DNS resolver |
| 4.1 | channel | log, loop | none | none |
| 4.2 | command | record, loop | **4.1** | fakes for every later-wave feature its commands touch |
| 4.3 | tui | channel | **4.1, 4.2, 4.4** (the socket protocol) | a fake agent server behind the socket |
| 4.4 | cmd/nerdgenie | everything so far | **4.1, 4.2, 4.3** | a fake systemd notify socket |
| 4.5 | functional | everything | **4.1–4.4** | none |
| 5.1 | signal | config | none | none (fake signal-cli exists, but no link flow) |
| 5.2 | signal/pairing | log | none | none |
| 5.3 | channel/signal | channel | **5.1, 5.2** | none |
| 5.4 | vault | config | none | none |
| 5.5 | reliability | log, loop, channel | none (but its functional test needs 5.3) | a backup to restore (finding 36) |
| 6.1 | memory | log, tool/memory | none | the twenty-question fixture |
| 6.2 | memory/capture | log | **6.1** | none |
| 6.3 | skill | tool, permission, loop | none | none |
| 6.4 | skill/learn | provider, shell | **6.3** | a fixture command-line tool and its docs |
| 6.5 | functional | all | **6.1–6.4** | none |
| 7.1 | worker/browser (TS) | none | none | Chrome, a fixture page server, a TS test runner |
| 7.2 | browser | tool | **7.1** (protocol) | fake browser worker with the 7.1 protocol |
| 7.3 | browser/login, handoff | vault, channel | **7.1, 7.2** | a fixture login site |
| 7.4 | skill/browser | skill | **7.1, 7.2** | fixture flows |
| 7.5 | functional | all | **7.1–7.4** | a captcha fixture, a visual-QA fixture app |
| 8.1 | schedule | loop, channel, log | none | none |
| 8.2 | desktop, worker/desktop | tool, permission | none | a display, a fixture window, a fake desktop worker |
| 8.3 | update | config, cmd | none | a fake release source |
| 8.4 | replay | log, loop, testkit | **8.1** (the nightly job) | none |
| 8.5 | functional, docs | all | **8.1–8.4** | a clean Linux machine, a cloud key, a local model |

55. **Wave 2 is not parallel.** Brief 2.4 imports 2.1, 2.2, 2.3, and 2.5; 2.3 needs 2.1's counts; 2.5 needs 2.1. Four of five workers block on each other. **blocker.** Move 2.1 (provider) and 2.2 (repair) into wave 1 in place of 1.5 (lint, which can run any time) and 1.4 (config, which can be one worker's afternoon added to 1.2), or define the four Go interfaces in a wave-1 brief and let each wave-2 worker test against a fake.
56. **Wave 3 is not parallel.** Briefs 3.2, 3.3, and 3.5 all implement the contract that 3.1 defines in the same wave; 3.3 needs 3.4's classes. **blocker.** Put the tool contract (one file, the interface and the class enum) in wave 2 or wave 1, and keep 3.1 for the registry, the folder loader, and the output cap.
57. **Wave 4 is fully serial.** 4.3 needs the socket protocol that only 4.4 (or 4.1) can define; 4.2 needs 4.1; 4.4 needs all; 4.5 needs all. **blocker.** Move 4.1 (the channel contract, the queue, the event stream, and the socket protocol as a JSON-lines schema) into wave 3, replacing 3.5's memory half which belongs in wave 6 anyway.
58. **Wave 5: 5.3 depends on 5.1 and 5.2.** **should fix.** Make 5.3 code against the fake signal-cli and a pairing interface, and define that interface in 5.3's brief.
59. **Wave 6: 6.2 depends on 6.1, 6.4 on 6.3, 6.5 on all.** **should fix.** Merge 6.2 into 6.1 (capture is 400 lines) and 6.4 into 6.3, or define the interfaces up front.
60. **Wave 7: 7.2, 7.3, 7.4, and 7.5 all depend on 7.1.** See finding 123. **blocker.**
61. **Wave 8: 8.4's nightly job depends on 8.1; 8.5 depends on everything.** **should fix.** 8.4 codes against a `Scheduler` interface; 8.5 starts when the other four merge, which the plan should say.
62. **The layout list omits fourteen packages** (`repair`, `review`, `config`, `lint`, `command`, `signal/pairing`, `channel/signal`, `reliability`, `memory/capture`, `skill/learn`, `browser/login`, `browser/handoff`, `skill/browser`, `replay`), so the rule "packages depend only downward in this list" cannot be applied to them. **should fix.** List every package in dependency order.
63. **The layout list contradicts the briefs.** `context` sits above `provider` but 2.3's test says "the cost line matches the fake provider's counts"; `loop` sits above `permission` and `tool` but must call both. The rule is satisfiable only if `loop` defines its own interfaces for tools and permissions and `cmd/nerdgenie` wires them, and the plan should say so. **should fix.** State that `loop` owns `Tool`, `Permit`, and `Router` interfaces, and move the cost line into `loop`.
64. **`internal/log` and `internal/context` shadow the Go standard library's `log` and `context` packages.** Every file that needs `context.Context` (most of them) will alias or collide; staticcheck will complain. **should fix.** Rename to `eventlog` and `prompt`.

---

## 3. Brief quality

Checked each of the forty briefs for four things: a job a stranger could act on, reference paths specific enough to open, tests a stranger could write, and a checkable "done." Briefs not listed pass all four (1.2, 2.1, 2.2, 3.4, 4.5, 5.4, 6.2, 8.3).

65. **1.1 (testkit).** No reference paths at all, although four of its fakes must speak real wire protocols (Anthropic streaming, OpenAI streaming, signal-cli JSON-RPC and SSE, the browser worker's JSON-RPC). "Every fake has its own unit tests" is not a test a stranger could write. **should fix.** Add the protocol references (the Anthropic and OpenAI streaming docs, signal-cli's `daemon --http` documentation, and OpenClaw's `extensions/signal/src/client.ts`) and one named test per fake.
66. **1.3 (record).** "The forty-step test" is named but never defined (finding 9). **blocker** (covered by 9).
67. **1.4 (config).** No fields listed (finding 14); "the home folder layout" does not say who creates it or with what permissions. **blocker** (covered by 14 and 48).
68. **1.5 (lint).** "Any comment that is not a complete sentence" and "any error string that does not say what to do" are not machine-checkable as written; a stranger will guess. **should fix.** Give the heuristics: a comment starts with a capital letter and ends with a period; an error string has at least two sentences or contains one of a listed set of verbs (`try`, `check`, `run`, `set`, `remove`).
69. **2.3 (context).** Never says how tokens are counted (a tokenizer library, characters divided by four, or the provider's reported `prompt_tokens`). The golden prompt for "a 24k model and a 200k model" depends entirely on that. **should fix.** State: count with the provider's last reported usage, estimate with characters divided by four before the first call.
70. **2.4 (loop).** The taint flag is listed but nothing says which tools set it or how a tool result carries it; the router is unspecified (finding 1); the question-versus-answer rule is unspecified (finding 4). **should fix.** Add a `Tainted bool` on the tool result in 3.1's contract, set by `web`, `browser_read`, and `read` of an inbox file.
71. **2.5 (review).** "Hands the fourth answer to memory as a proposed fact" does not say whether the user approves it, and it ignores the skill path (finding 23). **should fix.**
72. **3.1 (tool).** The folder-tool protocol (finding 13) and the output cap number (finding 19) are missing; "registration of a folder tool" cannot be tested. **blocker** (covered).
73. **3.3 (shell and sandbox).** Two packages in one brief against the rule "one brief, one package"; the sudo password source is undefined in wave 3 (finding 37); "a fuzz test on the command parser" names a parser the shell tool does not have (it hands the string to `/bin/sh`; the parser is in 3.4). **should fix.** Split into 3.3a shell and 3.3b sandbox, and move the fuzz target to 3.4's prefix reducer.
74. **3.5 (web and memory).** Two packages; the search provider is undefined (finding 18); the memory interface it codes against is defined nowhere. **blocker** (covered by 18).
75. **4.1 (channel).** Fine as a brief but it is in the wrong wave (finding 15). The socket protocol for the thin client is not mentioned here or in 4.3 or 4.4. **should fix.** Add: "the event stream is also the socket protocol: JSON lines over `~/.nerdgenie/agent.sock`, one event type per line, listed in `doc.go`."
76. **4.2 (command).** "Every command through the fake channel" cannot pass in wave 4 (findings 40–42). **should fix** (covered).
77. **4.3 (tui).** Names Bubble Tea v2 but not the inline-image protocol (kitty, iTerm2, sixel) for "screenshots inline where the terminal supports it"; tests need a fake agent server that testkit does not have; "done" is not stated. **should fix.** Name one image protocol (kitty graphics, with a text fallback), add a fake agent server to the brief's own test files, and add a done line.
78. **4.4 (cmd/nerdgenie).** The layout says the binary has `update`; the brief has `serve`, `tui`, `doctor`, `install`. No `init`, `uninstall`, `update`, `backup`, `restore`, `signal link`. Exit-code numbers "from the plan" are not in the plan. **blocker** (covered by 47–50). Also state the numbers: 0 clean stop, 1 bad configuration (do not restart), 75 restart me.
79. **5.1 (signal).** Missing the link flow (finding 43); "bounded cache" has no bound. **blocker** (covered by 43).
80. **5.2 (pairing).** Missing the user-side confirmation step (finding 44). **should fix** (covered).
81. **5.3 (channel/signal).** Does not say what the masked prompt does on Signal (design: "if it can"), or the inbox path, or that attachment text is wrapped as external content. **should fix.** Add: masked prompt returns "use the terminal"; inbox at `~/.nerdgenie/inbox/<date>/`; every attachment's extracted text goes through 3.5's external-content wrapper.
82. **5.5 (reliability).** Seven mechanisms in one package violates rule 4 ("one package, one job") and is the largest brief in the wave; its test "the nightly backup restored" depends on code nobody writes (finding 36). **should fix.** Split into `lease` plus `deadline` (one brief) and `breaker`, `ledger`, `sentinel`, `drain`, `watchdog` (a second brief), and move the backup test to the backup brief.
83. **6.1 (memory).** "The twenty-question memory test against a recorded session" names a fixture that does not exist; 6.5 also claims it, as "monthly." **should fix.** Put the twenty questions and the recorded session in 6.1's `testdata/`, and have 6.5 reuse them.
84. **6.3 (skill).** "A skill with a step over its limit is refused" names a limit the plan never states; the dry-run test is missing (finding 21); the `skill` tool is missing (finding 16). **should fix.** State the limit (steps per skill and characters per step).
85. **6.4 (learn).** Does not say where the smoke test runs (the sandbox) or what "a scripted docs page" is; the fixture command-line tool for the test is unnamed. **should fix.** Use a fixture binary in `testdata/` with a `--help` and a man page, and run the smoke test through the sandbox.
86. **7.1 (browser worker).** The largest brief in the plan and the only TypeScript one; the plan's test rules (Go `testing`, `testing.F`, `go vet`, ninety percent) do not apply and no TypeScript equivalents are named; the wire protocol is not written down anywhere a Go worker can read it; wall detection, the expectation field, and seven human actions are missing (findings 24, 25, 27). **blocker.** Split (finding 123), and add a `worker/browser/PROTOCOL.md` with every method, its parameters, and an example, written before the wave starts.
87. **7.2 (browser Go side).** "The seven browser tools from section 9" — section 9 does not list tools; section 7 does. It never maps the seven tools onto 7.1's actions (`browser_open` = ?, `browser_read` = snapshot or screenshot?). It claims all seven, but 7.3 builds two of them. **should fix.** Add a table: tool, worker method, class; and state that 7.2 owns five and 7.3 owns two.
88. **7.3 (login and handoff).** Two packages; "blocks the task until the user replies" contradicts the tool time limit (finding 96). **should fix** (covered).
89. **7.5 (functional).** Tests a visual QA skill that nobody builds (finding 52) and a captcha detector nobody builds (finding 24). **should fix** (covered).
90. **8.1 (schedule).** Actionable but dense; "a delivery target with a separately validated failure lane" is not explained; the `schedule` tool is missing (finding 16); the job prompt needs the loop's unattended mode (finding 3). **should fix.** Explain the failure lane in one sentence ("a second Signal recipient or terminal line that is checked to exist before the job is saved, so a failure notice always has somewhere to go").
91. **8.2 (desktop).** Two packages and two languages; "against a fixture window" does not say what window, on which display server (X11 or Wayland), or how CI gets a display; the irreversible classifier is undefined (finding 31); `@trycua/cua-driver` is named without a version or a link. **should fix.** State X11 with Xvfb in CI, a fixture window built from a tiny Tk or GTK script in `testdata/`, and the driver version.
92. **8.4 (replay).** Duplicates 1.1 (finding 51); "reports one line" does not say to whom. **should fix.** Report through the failure lane of the nightly job.
93. **8.5 (final pass).** "With one command" names no command; "a clean Linux machine" names no distribution; the prerequisites for that machine are undefined (finding 47); "done" is the README. **blocker.** Name the command (`make check`), the image (Debian 13 and Fedora 42 containers in CI), and add the manual acceptance script (finding 138).

---

## 4. Internal contradictions

94. **Eighteen tools (design 7) versus fifteen built (briefs).** See finding 16. **blocker.**
95. **Seven browser tools built by 7.2 and also by 7.3.** 7.2 "turns the seven browser tools ... into worker calls"; 7.3 builds `browser_login` and `browser_handoff`. Two workers own two tools. **should fix.** 7.2 owns five, 7.3 owns two (finding 87).
96. **Handoff blocks (7.3) versus "seven minutes per tool" (design 11) and "a question ends the turn" (rule 10).** A captcha or a two-factor code can take the user an hour; a blocking tool call trips the deadline and pins the loop. **should fix.** Make handoff end the turn in `waiting` with the record's Situation noting the handoff; the user's `done` or a code resumes it.
97. **"One message per reply" (design 12) versus "four-thousand-character chunks" (5.1).** A long reply becomes several messages. **nit.** Say "one reply, split only above Signal's limit, with a part counter."
98. **Rule 5 (design 3) versus brief 2.2.** The design says a badly formed call that cannot be repaired gets the list of real tools back; 2.2 adds "two failed parses in a row and the text is treated as the answer," which the design never says. **nit.** Add the two-strikes rule to the design or drop it from 2.2.
99. **"Each worker gets one brief, one package" (Part 1) versus briefs 3.3, 3.5, 7.3, 8.2 (two packages each) and 5.5 (seven jobs in one package, against rule 4).** **should fix.** Split them or change the rule.
100. **Testkit's fake channel (1.1, wave 1) versus the channel interface (4.1, wave 4); testkit's fake browser worker (1.1) versus the worker's protocol (7.1, wave 7).** The fakes cannot implement contracts that do not exist yet. **blocker.** Define the channel interface in wave 1 and the browser protocol as a document before wave 7 (findings 15, 57, 123).
101. **Replay-as-test built in 1.1 and again in 8.4.** **should fix** (finding 51).
102. **"Monthly memory test" (6.5) versus "nightly job that runs the memory test" (8.4).** **nit.** Nightly.
103. **Rule 3 "the Go code uses the standard library wherever the standard library will do" versus the briefs' outside libraries** (`modernc.org/sqlite`, Bubble Tea v2, `age`, a Landlock binding, a seccomp binding, a TOTP library, a TOML parser, a cron parser, an SSE client). Reasonable choices, but Part 1 says every one is listed in `docs/DEPENDENCIES.md` with a reason, and no brief creates that file. **should fix.** Pre-approve the list in the plan and have 1.2 create `docs/DEPENDENCIES.md`.
104. **"Ships as one static binary" (design 13) versus a Node worker, Chrome, a Java-based signal-cli, bwrap, and ripgrep.** True only for the agent process. **should fix.** Say "one static binary for the agent, plus four outside programs the installer checks for" in both documents.
105. **Design stage table (v0 "five tools, one model provider, the terminal") versus the waves (two providers in wave 2, seven tools in wave 3, terminal in wave 4).** **nit.** Say the wave list replaces the stage table.
106. **Design 4 "the harness fills in the rest" and design 5 "the harness enforces what it can" versus brief 1.3 which makes the record immutable but has no `task` tool to enforce anything against.** **blocker** (covered by finding 16).
107. **Design 11 "every file-writing tool runs inside a sandbox" versus brief 3.2.** **blocker** (covered by finding 34).
108. **Design 13 "order of work" versus wave order.** **nit** (covered by finding 54).
109. **Part 1 "every package takes a clock as an input" versus the TypeScript workers**, which have no such rule and whose settle timers (three hundred milliseconds, three seconds, human pacing) are exactly what a fake clock is for. **nit.** State the TypeScript rule: pacing and settle timers take an injectable timer.

---

## 5. Test framework completeness

110. **Fakes the briefs need that testkit does not list:** a fake permission function and approver (2.4, 3.x, 4.5); a fake tool registry and router (2.4); a fake memory store and sink (2.5, 3.5); a fake search server (3.5); a fake sudo and a fake "bwrap missing" (3.3); a fixture HTTP server for pages and redirects to private addresses (3.5, 7.x); a fake agent server behind the socket (4.3); a fake systemd notify socket (4.4, 5.5); a fake release source (8.3); a fake desktop worker (8.2); a fixture command-line tool with docs (6.4); the forty-step fixture (1.3, 2.x, 8.5); the twenty-question memory fixture (6.1, 6.5); the login, captcha, and visual-QA fixture sites (7.3, 7.5). **blocker.** Add each to brief 1.1 or to the first brief that needs it, and drop the sentence "the framework is the only place fakes live" (it makes every later wave edit a package nobody owns).
111. **The fake browser worker cannot be built in wave 1** because its protocol is defined in wave 7. **blocker** (finding 100). Write `worker/browser/PROTOCOL.md` first, then the fake, then the real worker.
112. **The fuzz rule ("every function that parses outside text has a fuzz test that has run for at least one minute") is realistic per function but not per suite.** Roughly thirty fuzz targets at one minute each puts thirty minutes inside `make test`, which the wave gate runs on every merge. **should fix.** Split into `make fuzz` (one minute per target, run once at brief close and nightly) and a five-second smoke inside `make test`.
113. **The fuzz rule has no TypeScript equivalent.** 7.1 parses JSON-RPC and page trees from outside the program. **should fix.** Name `fast-check` for the worker's parsers and `vitest` with `c8` for coverage.
114. **Ninety percent line coverage is not realistic for `internal/tui`, `cmd/nerdgenie`, `worker/browser`, or `worker/desktop`.** The TUI has terminal-capability branches (image protocols, resize, color depth) that only a real terminal hits; the binary has process, signal, and systemd paths; the workers launch Chrome or a display. **should fix.** Set seventy percent for those four with the uncovered branches named in `doc.go`, keep ninety for everything else.
115. **The integration-test rule "in the package that sits highest in the dependency order" contradicts "a worker never touches a package another worker owns" whenever the highest package belongs to a neighbor in the same wave.** **nit.** Put cross-package integration tests in `test/integration/`, owned by the functional-test brief of each wave.
116. **Sandbox, systemd, Landlock, and the visible browser only run on Linux, and the development machine is macOS.** No brief or rule says where tests run. **blocker.** Add CI (finding 132) and say in Part 1 that `integration`-tagged tests run only in the Linux container.

---

## 6. Wave balance and risk

Rough sizes are Go (or TypeScript) lines including tests.

117. **Wave 1 is lopsided.** 1.1 testkit with seven fakes and two protocol servers is 2,500–4,000 lines; 1.4 config is about 500; 1.5 lint about 700. **should fix.** Split 1.1 into 1.1a (fake model, clock, home, golden files, forty-step fixture) and 1.1b (fake provider server, fake signal-cli), and fold 1.4 into 1.2's worker.
118. **Wave 2: 2.4 loop (1,500–2,500) and 2.1 providers (1,500–2,500) against 2.5 review (400–600).** Acceptable if the deadlock is fixed; otherwise 2.4 waits for three neighbors. **should fix** (finding 55).
119. **Wave 3: 3.3 shell plus sandbox (1,500–2,500, and needs Linux) is twice 3.1 (500–800).** **should fix.** Split 3.3.
120. **Wave 4: 4.3 tui (2,000–3,500) is the largest Go brief in the plan.** **should fix.** Split into 4.3a (the client, the event loop, streaming, first frame, spinner) and 4.3b (approvals, masked prompt, inline images).
121. **Wave 5: 5.5 reliability (1,500–2,500, seven mechanisms) against 5.2 pairing (500–800).** **should fix** (finding 82).
122. **Wave 6 is balanced** (1,200–1,800 for 6.1 and 6.3; 400–1,200 for the rest). **nit.** No change.
123. **Wave 7 is a single-lane road.** 7.1 is 3,000–5,000 lines of TypeScript plus fixture pages, and 7.2, 7.3, 7.4, and 7.5 all wait for its protocol and its fixtures. As written, wave 7 is one worker for a week and four workers idle. **blocker.** Do three things: (a) write `PROTOCOL.md` and the fake browser worker in wave 6 as a sixth brief; (b) split 7.1 into 7.1a launch, attach, snapshot, screenshot, wall detection and 7.1b click, type, press, scroll, act, settle, diff, dialogs, tabs, downloads; (c) let 7.2, 7.3, 7.4 code against the fake, and add a short wave 7b that swaps the real worker in and runs 7.5.
124. **Wave 8: 8.2 desktop (2,000–3,500 across two languages and a display) against 8.4 replay (500–800).** **should fix.** Split 8.2 into the TypeScript worker and the Go tool.
125. **The design's "twelve to sixteen thousand lines" is two to three times low** for the feature list in the briefs; my sum is 45,000–70,000 including tests, or roughly 20,000–30,000 without. **should fix.** Re-estimate in the design, or cut the desktop worker and the learn-from-docs path from the first version.
126. **No time budget per brief.** A worker at "extra-high effort" with a 2,500-line brief and a ninety-percent coverage bar can run for a day; there is no rule for when to stop and report. **should fix.** Add "a worker reports after N hours whether or not it is done, with what passes."

---

## 7. Plain English

Both documents promise prose a high-school student could follow. The design mostly delivers; the work plan does not, because it assumes the reader has the design's glossary in their head and adds forty terms of its own without explaining any of them on first use.

127. **Unexplained on first use in the work plan:** WAL mode, SSE, JSON-RPC, FTS5, TOTP, seccomp, Landlock ruleset, bwrap, `connectOverCDP`, DevTools port, loopback, process group, build tag, symlink, `SUDO_ASKPASS`, age-encrypted, salted and hashed, constant time, Bubble Tea v2, deltas, table-driven, staticcheck, gofmt, atomic update, lease, sentinel, drain marker, boot id, heartbeat, "hashes output," "cascade" (7.4), "envelope shapes" (2.2), "near-miss," settle, refs, "prefix reduction," "taint flag," "tools-off call," "act batch," `modernc.org/sqlite`, `playwright-core`, `@trycua/cua-driver`, "failure lane," "claim," "monitor mode," "readiness," "golden file" (this one is explained, in the testkit list). **should fix.** Add a one-page glossary at the top of Part 3, or a parenthetical on first use, as the design does.
128. **Run-on sentences a stranger will re-read three times:** brief 5.5's first sentence (about ninety words and seven mechanisms); brief 2.4's second sentence (six clauses); brief 8.1's first sentence (nine clauses). **nit.** One mechanism per sentence.
129. **Fragments in the briefs:** "Reference, one Hermes file per feature:" (5.5), "Tests: everything against the fake signal-cli:" (5.1), and the "Tests:" lists throughout are semicolon-chained fragments. Acceptable in a checklist, but the plan's own rule 5 says every comment is a complete sentence; the plan should hold itself to the standard it sets or say the checklists are exempt. **nit.**
130. **"Fable 5.1" and "Opus 4.8" are named without saying what they are** to a reader who does not know they are model names. **nit.** "The orchestrator is a Fable 5.1 model, the more capable of the two models we use; each worker is an Opus 4.8 model, the one that writes code."
131. **Design 12, "Sends a screenshot of the browser or the desktop right now"** and design 9's "fingerprint," "headless," "proxy servers" are explained; the work plan's "loopback DevTools port with a per-launch token" (7.1) is not, and it is the security-critical sentence in the brief. **should fix.** "A DevTools port is the door Chrome opens so that a program can drive it; loopback means only programs on this machine can knock; the token is a password made fresh each launch."

---

## 8. Anything else a lead would refuse to sign

132. **No continuous integration.** The wave gate is "the orchestrator ... runs the whole test suite," on a macOS machine, for code that needs bwrap, Landlock, systemd, Chrome, and a display. **blocker.** Add a wave-0 brief: a GitHub Actions workflow on `ubuntu-latest` that installs bwrap, ripgrep, Chrome, Node, and signal-cli, runs `make check` on every push, and a nightly `live` job with keys in secrets; the orchestrator gates on the green check, not on a local run.
133. **No Makefile and no named targets.** Part 1 and brief 1.5 mention `make test`; nothing creates it. Workers in wave 1 will each invent one. **blocker.** Same wave-0 brief: `make build`, `make test` (unit and fuzz smoke), `make integration`, `make fuzz`, `make lint` (vet, staticcheck, gofmt, the style checker), `make check` (all of the above), `make live`, `make install`.
134. **No `go.mod`, module path, Go version, or Node version.** Five workers in wave 1 creating `go.mod` in five branches is five merge conflicts. **blocker.** Wave-0 brief: `go.mod` with the module path and a pinned toolchain line, `worker/browser/package.json` with a pinned Node version and a lockfile, both owned by the orchestrator thereafter (finding 137).
135. **No license, and no license notes for the copied third-party files.** `docs/reference/` holds verbatim copies of Codex (`shell_spec.rs`, `landlock.rs`; Apache-2.0), browser-use (`serializer.py`, `views.py`, `prompts.py`; MIT), and Moltis (`manager.rs`, `snapshot.rs`, `types.rs`; check). The repository has no `LICENSE`. "Borrow designs, not code" lowers the risk for the Go code but not for the files already checked in. **blocker.** Choose a license for Nerd Genie, add `LICENSE`, and add `docs/reference/LICENSES.md` naming each copied file's origin, commit, and license text.
136. **No security review step.** The plan builds a sandbox, a vault, a sudo path, an SSRF guard, a pairing system, and a browser that holds the user's logins, and never once says "someone reviews this against a threat model." **blocker.** Add a written threat model (`docs/SECURITY.md`) as a wave-2 deliverable and a security review checklist as the gate for waves 3, 5, and 7: prompt injection through web pages, attachments, and Signal senders; pairing brute force; vault key handling and backup key handling; sudo askpass exposure; Landlock symlink escapes; DevTools port exposure; the API key at rest.
137. **No ownership rule for shared files.** `go.mod`, `go.sum`, `Makefile`, `docs/DEPENDENCIES.md`, `docs/PROGRESS.md`, and `internal/testkit` are edited by many workers; the plan's merge step will hit conflicts every wave. **should fix.** The orchestrator owns those files; a worker that needs a dependency or a fake names it in its report and the orchestrator adds it before the merge.
138. **No "how a human tries it" milestone per wave.** Only wave 4 says "this is the first thing a human tries." Waves 5 through 8 say "after this wave, a person can ..." with no script. **should fix.** Add five lines per wave: wave 4 "`nerdgenie init`, `nerdgenie serve`, `nerdgenie tui`, ask a question, `/status`"; wave 5 "`nerdgenie signal link`, scan, text the number, `/pair`, get a reply"; wave 6 "tell it a fact, `/new`, ask for it"; wave 7 "`/vault add`, ask it to log in to a fixture site, `/screen`"; wave 8 "`schedule` a job for one minute out, receive it, `nerdgenie update`."
139. **The API key has no home before wave 5.** Wave 4's "against a real model" needs a key; `config.toml` is the only file, and its permissions are unstated. **should fix.** Store it in a 0600 file `~/.nerdgenie/credentials.toml` from wave 4; 5.4 migrates it into the vault.
140. **Prompt-injection wrapping covers `web` only.** Signal attachments (5.3), files the model `read`s from the inbox, and browser page text (7.1) are all outside words. **should fix.** State that the external-content wrapper from 3.5 is applied by 5.3 and 7.2 too, with one functional test per source.
141. **Database migrations start in wave 8 (8.3) but the schema is created in wave 1 (1.2) and changed in waves 5, 6, and 8.** Until 8.3 lands, every schema change on a running install is unmanaged. **should fix.** Move the numbered forward-only migration runner into 1.2; 8.3 adds the backup-before-migrate and the version refusal.
142. **The worker branches are merged by the orchestrator, but the plan never says what happens to a wave that merges green and then fails a later wave's integration test.** No revert or bisect rule. **nit.** "A later wave that finds a bug in an earlier package writes a fix brief for that package; the earlier wave is not re-opened."
143. **Recorded page fixtures (7.1, 7.5) from real websites carry third-party markup, scripts, and trademarks into the repository.** **nit.** Use synthetic fixture sites written for the tests.
144. **The orchestrator "reads every new file" but the plan gives it no checklist beyond Part 4.** The definition-of-done list is a good start but omits "does the brief's every test exist by name" and "does the package's `doc.go` reference match the brief." **nit.** Add those two lines to the definition of done.

---

## Count

- Total findings: **144**
- Tagged blocker: **37** entries. Thirteen of them restate an earlier blocker under a different heading (66, 67, 72, 74, 78, 79, 86, 94, 100, 106, 107, 111, 116), leaving **24 distinct blockers**: 9, 13, 14, 15, 16, 18, 24, 34, 36, 43, 47, 48, 55, 56, 57, 60/123, 93, 110, 132, 133, 134, 135, 136, and the wave-7 protocol document (123).
- Tagged should fix: **82**
- Tagged nit: **25**

The plan's strengths, for balance: every reference path resolves; the test-first rule and the definition of done are concrete; the reliability wave is a faithful port of Hermes's hardening; the briefs for the log, the providers, the repair layer, the permission engine, the vault, and the updater are ready to hand out today. What is not ready is everything a user touches before the first message (install, init, link, pair) and everything that lets five workers actually run in parallel (interfaces defined a wave early, a protocol document before the browser wave, a CI machine that is Linux).
