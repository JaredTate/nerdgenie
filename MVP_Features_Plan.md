# MVP Features Plan: what Nerd Genie needs before it is open-sourced

Written 6 September 2026 from three measurements made the same evening: every test suite run with coverage, a survey of what Hermes, OpenClaw, Claude Code and Codex give a user that we do not, and a read of our own installer, updater, providers and channels. The reports are under `docs/research/2026-09-06-release-readiness/`. Nothing in this plan is built yet. It applies two rules throughout, KISS and YAGNI: the simplest thing that works, and nothing until somebody needs it.

## The goal in one paragraph

A stranger with a Linux machine types one command, answers a handful of questions, and is talking to Nerd Genie in ten minutes, on a model they already have. Three ways in, in this order of ease: a model in Ollama (local or Ollama's cloud, such as GLM 5.3), a subscription they already pay for (Claude Code for Opus, Codex for GPT 5.6 Sol), or our own way, the uncensored Qwen 3.8 on llama-server with TurboQuant and MTP, which stays the primary path and the fastest one, but is the most work to set up. They talk to it in the terminal, on Signal, or on Telegram. They can ask it what it did two days ago and get a dated answer.

## What exists today

More than it looks from the outside. Every item below is built and tested:

- A release build: one static binary for linux/amd64 and linux/arm64, packed with the two worker bundles and a pinned Node runtime, so a user needs neither Go nor Node. Beside it a checksum file and a manifest.
- An installer script, `scripts/install.sh`, for `curl | sh`: fetches the checksum before the archive, keeps nothing that does not match, installs signal-cli where the package manager lacks it, tells the user to install Chrome, and runs `nerdgenie init`.
- `nerdgenie init`: at most six questions, answerable on the command line for a machine with no keyboard. `nerdgenie doctor` checks everything. `nerdgenie install` writes a systemd user service and a nightly backup. `nerdgenie update` installs the newest release and rolls back by itself when the new one does not come up.
- Providers: any OpenAI-compatible server (llama-server, LM Studio, Ollama, cloud gateways), Anthropic, Claude Code's command line, Codex's command line.
- A Signal channel as a linked device, with pairing, and a channel router with a documented shape for adding a service (`docs/EXTENDING.md`).
- The record, the event log with a UTC time on every event, jobs, scheduled jobs, skills, memory with full-text search over every past message, a browser on screen, a desktop driver, a permission gate with previews and `/yolo`, an optional sandbox fence, a live screen with the model's name and speeds.

## The plain answer to "do they need Go?"

No. A user installs a binary. The release archive carries the program, the two workers and their own Node, so nothing else is installed for them. The only things a user installs themselves are Google Chrome, if they want the browser tools, and signal-cli, which the installer fetches for them. A developer who wants to change the code needs Go 1.27 and Node 22; that is what `CONTRIBUTING.md` says. This is simpler than a TypeScript agent, which asks every user to install Node and npm first.

## The plan, in order

Sizes are working days for one person, test first. Total: about fifteen days of essentials, then Telegram.

### 1. A published release, so the one-line install works (1 day)

The installer and the updater both point at `releases/latest` on GitHub, and there is no release yet, so `curl | sh` and `nerdgenie update` fail at their first step. Run `make release`, publish the archives, the checksum file and the manifest as a tagged release, and make the container test in `scripts/release` (which today skips when Docker is absent) run in CI on a clean Ubuntu 24.04 image: install from the published release, run `nerdgenie doctor`, run one task against the fake model. That test is the proof the install works on a machine that is not ours.

### 2. Onboarding as one command and one wizard (2 days)

Keep the wizard inside `nerdgenie init`; do not add a second one. Make its model question a menu:

1. "A model in Ollama on this machine" (probe `localhost:11434`, list what `ollama list` has, pick one).
2. "A model in Ollama's cloud, such as GLM 5.3" (an Ollama key, kept in the vault).
3. "Claude Code, on my subscription" (checks the program is installed and signed in).
4. "Codex, on my subscription" (the same).
5. "A local llama-server" (probe the port, read its `/props`, as today).
6. "An OpenAI-compatible or Anthropic API key" (kept in the vault).

Then: which folders it may work in, Signal yes or no, Telegram yes or no. Six questions, all with defaults, all answerable on the command line. `nerdgenie doctor` says what is missing in one line each.

### 3. Ollama as a real, tested path (2 days)

Ollama's OpenAI-compatible endpoint is not llama-server's, and three things differ: it ignores `max_completion_tokens` and wants `max_tokens`, so the output cap never applies today; thinking is switched off with `reasoning_effort: "none"`; and its context window is Ollama's setting, four thousand by default, not the number in `config.toml`, with `/api/show` and `/api/ps` as the way to read it, since there is no `/props`. The provider gains an Ollama probe beside the llama-server one and those three tweaks. Then two Ollama models join `make live` as the fourth and fifth real models: one local, and GLM 5.3 in Ollama's cloud.

The model matrix to prove green before release, on the live suite and the nightly set: Opus through Claude Code, GPT 5.6 Sol through Codex, Qwen 3.8 on llama-server (our primary), GLM 5.3 through Ollama's cloud, and one local Ollama model. `docs/PROGRESS.md` gets the table with cost per task.

### 4. Two ways to run local, said plainly (part of the docs day)

The documents must say it in one breath: Ollama is the easy way, one command, any model in its library, slower and with its own context settings; llama-server is our way, faster, with the checkpoints and the vision projector, full control of the context, and the uncensored Qwen 3.8 that everything here was tuned on. Nerd Genie treats both as the same provider kind; only the address and the probe differ.

### 5. Signal documented, Telegram added (Signal 0.5 day, Telegram 3 to 4 days)

Signal works today and is undocumented for a stranger: `nerdgenie signal link` links the machine as a device, `/pair` admits a sender, the installer fetches signal-cli. That goes into the setup page.

Telegram is the one new channel, built the way Signal is built and the way Hermes, OpenClaw and ZeroClaw all build it: raw HTTP against the Bot API, long polling (no webhook, no public address), a BotFather token kept in the vault, direct messages only, senders admitted by the same pairing codes Signal uses, replies split at four thousand characters, pictures fetched through `getFile` so the model can see them. One `init` question, one config key, one package mirroring `internal/signal`, through the channel router that exists. Nothing else: no groups, no inline keyboards, no webhooks, until someone needs them.

### 6. "What did you do two days ago" (1 day)

The answer is already in the log; the person just cannot ask for it. Three small things: `/tasks` shows the date of each task; `/tasks yesterday`, `/tasks today` and `/tasks 2026-09-04` filter by day and print each task's closing line; and the orientation block carries today's date so that a dated question in chat is answered from the memory index, which already holds every past message, instead of starting a blind task. No sessions, no resume command: the record is the session, and any message picks a put-down task up, which is more than the other agents offer.

### 7. Small things a user expects (2 days in all)

- An `AGENTS.md` or `CLAUDE.md` in the work folder read into the prompt, because every Claude Code and Codex user has one (1 day).
- A terminal bell and a desktop notice when the agent asks a question, finishes, stops or fails (half a day).
- `nerdgenie logs`, and a section saying where every log is (half a day).

### 8. The house in order (2 days)

- Twelve phantom tests removed or written: five permanently skipped stubs in `test/functional/browserflows_test.go` whose reason is stale, five assertion-free smoke tests in `cmd/nerdgenie`, one generator wearing a test's name, one fuzz target that checks nothing about its answer. `TESTING.md` says reviewers reject these; the project must not ship any.
- One stale constant: the live functional test still pins the local window at 262,144 where the daemon runs 131,072.
- `SECURITY.md` and `CODE_OF_CONDUCT.md`, one page each, and a `CHANGELOG.md` started at the first release.
- Coverage from 92.8 to 95 percent: 608 more statements, most of them in `cmd/nerdgenie` (83 percent, the Signal wiring and the streaming pieces at zero) and the fifteen functions at zero named in the report.

### 9. Documents for a stranger (3 days)

`INSTALL.md` today is a runbook for our own machines and the TurboQuant fork. It stays, renamed for what it is, and three short pages go in front of it: a ten-minute quickstart (install, init with Ollama or a subscription, first task, first Signal message), a configuration reference with every key in `config.toml` and one line each, and a page on the two local paths. `README.md` leads with the quickstart. Every page in plain English, tested by giving it to someone who has never seen the project.

## What we are not doing, on purpose

MCP servers, hooks, a plugin system, voice, named sessions and a resume command, per-project settings files, profiles beyond `NERDGENIE_HOME`, a `.deb` package (it would give the package manager and `nerdgenie update` competing ownership of the same files), Discord, Slack and WhatsApp, a web interface, macOS and Windows. Each is a real feature somewhere else; none has a user here yet. The channel router and the provider shape mean any of them is one package when someone needs it.

## Bold, and still simple

- **`nerdgenie bench`.** Ship the nightly set as a subcommand: any user runs it against their model for an hour and gets the same table we read every morning (rounds, minutes, cache share, cut-offs, refusals, checks). Users post their tables; the project gets a public model matrix for free, and every claim about a model is a number somebody else can reproduce.
- **The record as the selling point.** Every other agent sells sessions and resume. We sell the opposite: there is nothing to resume, because nothing was forgotten. `/tasks yesterday` and a put-down task picked up by any message are the demonstration; the quickstart should end on exactly that.
- **One binary, one folder.** The whole of a user's Nerd Genie is one binary and one home folder with one SQLite file. Backup is one encrypted archive. Uninstall is one command. That is worth a line on the front page.
- **The on-screen browser as the feature, not the limitation.** Nobody else shows the user the browser the agent drives. Say so.

## The order of work

Weeks one and two: items 1, 2, 3, 6 and 8, because the release, the wizard, Ollama, the dated history and a clean test suite are what a stranger meets first. Week three: items 7 and 9, the small expectations and the documents, then the release tag. Week four: Telegram, as the first thing after the release rather than a blocker for it.

## The numbers behind this plan, 6 September 2026

| What | Where it stands |
|---|---|
| Go tests | 3,809 test functions and 83 fuzz targets in 749 files; 90 functional tests; all green in the plain and the integration-tagged runs |
| Worker tests | browser 331 passed, 88.7 percent of statements; desktop 163 passed, 78.0 percent |
| Coverage, whole repository | 92.8 percent of statements; every gated package above its floor; 608 statements to 95 |
| Production Go | 74,669 lines, of which the harness alone (`internal/` and `cmd/`) is 73,848 |
| Test Go | 117,825 lines, plus 7,301 lines of fakes in `internal/testkit` |
| Production and test TypeScript | 6,691 and 5,107 lines |
| Documents | 21,101 lines of Markdown |
| Test lines per production line | Go 1.68 with the fakes counted as test code, 1.44 with them counted as production; TypeScript 0.76; overall 1.60 |
| Phantom tests | 12 in Go (5 skipped stubs, 5 without an assertion, 1 generator, 1 crash-only fuzz), none in TypeScript |
