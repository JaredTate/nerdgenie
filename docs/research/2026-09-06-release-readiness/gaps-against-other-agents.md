# What an ordinary user will miss in Nerd Genie, before it is open-sourced

Written 6 September 2026 from the repository at `/home/jared/Code/coeus` (read only), the four agents on disk under `/home/jared/Code` (hermes-agent, openclaw, opencode, zeroclaw, prime-agent), and the current Claude Code, Codex, Ollama and Telegram documentation on the web. Nothing was run; no test suite, no fuzzer.

**How to read the table.** *Have* means a person can do the thing today without reading code. *Partly* means the machinery is there but the door is missing, narrow, or undocumented. *None* means nothing. The last column says where Nerd Genie's version lives when it has one. Hermes and OpenClaw are the always-on assistants; Claude Code and Codex are the coding agents from the model labs. Where a claim about another agent comes from its docs rather than its code, that is said.

**The short version.** Nerd Genie already has the hard parts an ordinary user cares about: a real installer with a pinned runtime, an updater that rolls back, a doctor, a four-question setup, a permission gate with a preview, an optional kernel sandbox, scheduled jobs, skills, memory, a web tool, a browser, Signal, and a terminal screen with a live status line. What it lacks is mostly the *thin* layer other agents put on top: dates on the task list so "what did you do yesterday" has an answer, a project instruction file read from the work folder, a bell when it needs you, a working Ollama path, and documentation written for a stranger's machine rather than for irene and rosie. Those five are the essentials and come to about eight working days. Telegram is the one bigger thing worth doing soon after, at three to four days. MCP, hooks, voice, named sessions and per-project settings can wait.

---

## 1. What Nerd Genie has today

- **Subcommands** (`cmd/nerdgenie/main.go`): `version`, `help`, `init`, `doctor`, `serve`, `run`, `tui`, `install`, `uninstall`, `signal link`, `backup`, `restore`, `replay`, `update`, `askpass`, and a hidden sandbox helper. Bare `nerdgenie` opens the screen. `run` takes `-yes`, `-wait`, `-timeout` (default 30 min) and exits 0 on a reply, 1 otherwise. `update` takes `--check`, `--rollback`, `--migrate`, `--to`, `--from`.
- **Slash commands** (registered in `cmd/nerdgenie/commands.go`, same on the screen and on Signal): `/help`, `/status`, `/model`, `/approve`, `/deny`, `/pause`, `/resume`, `/undo`, `/tasks` (list the newest 50, print one, wind one back), `/stop`, `/jobs`, `/cron` (list, show, run, off), `/skills` (show, run, rollback, remove), `/walk` (record, stop, replay, check a browser walk), `/screen`, `/vault` (terminal only), `/memory` (search, forget), `/clear`, `/think`, `/yolo`, `/readyz`, and `/pair` when Signal is configured. `/new` and `/sessions` exist in `internal/command/session.go` but are deliberately left out of the registry because there is no session store.
- **Configuration** (`internal/contract/config.go`, read by `internal/config`): one commented `config.toml` in `~/.nerdgenie` (or `$NERDGENIE_HOME`, the only environment variable read). Keys: `[[models]]` blocks (`name`, `provider` = `anthropic` | `openai` | `codex` | `cli`, `base_address`, `program`, `model_name`, `context_length`, `vision`, `key_reference`, `think`), `default_model`, `fallback_chain`, `signal_account`, `browser_profile_path`, `sandbox` (`off` | `fence`), `sandbox_roots`, `backup_path`, `search_server_address`, `handoff_timeout`, `ask_me_first`, `permission_rules`, `[caps]` (`rounds_per_task`, `time_per_task`, `time_per_tool`, `time_per_turn`, `queued_messages`, `tool_output_bytes`, `identical_call_window`, `output_tokens_per_call`, `buffered_browser_events`), `[memory_caps]`. Decoding is strict: an unknown key is refused with the line number.
- **Providers** (`internal/provider`): the Anthropic Messages API; any OpenAI-compatible `chat/completions` server (llama-server, LM Studio, OpenAI, and in principle Ollama; a loopback server is probed at `/props` and treated as llama-server when it answers); the ChatGPT backend through the `codex` program's sign-in file; and the `claude -p` / `codex exec` programs on a subscription. Retries, a fallback chain, six thinking levels, vision on both wires.
- **Channels**: the terminal (a Unix socket, `internal/channel`) and Signal (`internal/signal`: signal-cli daemon, pairing codes, attachments into `inbox/`, replies split at Signal's 2,000-unit limit, previews answered with `approve`/`always`/`deny`). `contract.Channel` has seven methods; `docs/EXTENDING.md` says Telegram is "one new package".
- **Memory and history**: an append-only event log in `nerdgenie.db` with a UTC time on every event; one checkpoint per round; `MEMORY.md`/`USER.md` with size caps; a full-text index over facts, notes **and every past message** (`/memory search`); zero-token capture of files, commands, sites and corrections; "where are we" answered from the records without a model call (`cmd/nerdgenie/statusanswer.go`); `scripts/runreport` for one task's numbers; `nerdgenie replay N` to re-run a recorded task as a test.
- **Safety**: the ask-me-first list (recursive deletes, sudo, spending money), user rules with last-match-wins, once/always/reject-with-reason, `/yolo` per session, standing approvals from skills, an unattended run that stops rather than waits, and an optional bwrap + Landlock fence with the vault, the browser profile and `~/.ssh` always outside.
- **Install and release**: `scripts/install.sh` (197 lines, `curl | sh`, Ubuntu/Debian apt for bubblewrap, ripgrep and signal-cli, an AppArmor profile, `~/.nerdgenie/releases/<v>/` with a `current` link, a launcher in `~/.local/bin`, then `init`); `make release` builds a tarball per architecture holding the binary, a pinned Node 24.18.0, and the two workers, with `SHA256SUMS` and `manifest.json`; `nerdgenie install` writes a systemd user service with a watchdog and a nightly backup timer; `nerdgenie update` swaps the link and rolls back if the new version is not up in sixty seconds.
- **Docs**: `README.md`, `INSTALL.md`, `SETUP.md`, `NERDGENIE.md`, `ARCHITECTURE.md` (1,870 lines), `docs/EXTENDING.md`, `docs/TRIAL.md`, `docs/HARNESS_V2.md`, a research folder, one example user tool (`examples/tools/wordcount`). MIT licence. Three GitHub workflows (check, install-in-a-container, release).

---

## 2. The table

| Feature | Nerd Genie | Hermes | OpenClaw | Claude Code | Codex | Where Nerd Genie's lives |
|---|---|---|---|---|---|---|
| Carry on the last conversation (`--continue`) | **partly**: the next message on a screen picks up that screen's stopped or waiting task, and this survives a restart; `nerdgenie run` has no flag for it | have (`-c`, `--resume`) | partly (`openclaw resume` re-attaches; no `--continue`) | have (`--continue`, `--resume`) | have (`codex resume --last`) | `cmd/nerdgenie/resuming.go` |
| Named sessions and a session list | **partly**: `/tasks` lists the newest 50 tasks by number, status and ask; no names, no dates; `/new` and `/sessions` are hidden stubs | have (`hermes sessions`, `/sessions`) | have (`openclaw sessions`) | have (`-n`, `/rename`, `/resume` picker) | have (`/rename`, archive) | `internal/loop/commands.go`, `internal/command/session.go` |
| Search past conversations ("what did you do two days ago") | **partly**: `/memory search <words>` finds any past message; "where are we" is answered from the records; nothing by date | have (FTS5 `session_search`) | have (`sessions_search` tool) | partly (picker search, no content search) | partly (picker) | `internal/memory/search.go`, `cmd/nerdgenie/statusanswer.go` |
| Cost and token report | **partly**: `/status` prints the session's tokens in, cached, out; the header shows the context meter and cache share; the record carries a cost line every turn; money only when the `claude` program reports it | have (`/usage`, `/insights`) | have (`/status`, `/usage`) | have (`/usage`, `/cost`, `/context`) | partly (`/status`, limits) | `internal/command/core.go`, `cmd/nerdgenie/watchedmodel.go` |
| Configuration file | **have**: one `config.toml`, a comment on every line, strict keys, `doctor` reads it | have (`config.yaml` + `.env`) | have (`config get/set`) | have (`settings.json` in four layers) | have (`config.toml`) | `internal/contract/config.go`, `internal/config` |
| Profiles, or several setups on one machine | **partly**: one home per profile through `NERDGENIE_HOME` | have (`hermes profile`) | have (`--profile`) | partly (`--settings`) | have (`[profiles.x]`, `--profile`) | `internal/config/home.go` |
| Switch model mid-session | **have**: `/model alias`, `/think level`; session only | have | have | have (`/model`, `/effort`) | have (`/model`) | `internal/command/core.go`, `cmd/nerdgenie/think.go` |
| Channels (Telegram, Discord, Slack, WhatsApp, Signal) | **partly**: terminal and Signal | have (~30) | have (~30) | partly (Slack; channel plugins in preview) | partly (Slack via cloud) | `internal/signal`, `internal/channel` |
| A notice when it needs you or has finished | **partly**: the report goes to the channel; Signal gets a "still working" note at five minutes; no bell, no desktop notice, no notify command | have | partly (through channels and hooks) | have (bell, desktop notice, `Notification` hook) | have (`notify` command, `tui.notifications`) | `internal/signal/channel.go`, the loop's reports |
| Scheduled or cron tasks | **have**: a job with a schedule, made through the `job` tool; `/cron` lists, runs, switches off; backoff and incidents | have (`hermes cron`) | have (automations, `/loop`) | have (`/loop`, `/schedule`) | partly (app only) | `internal/job` |
| Skills | **have**: a folder with `SKILL.md`, steps, a test and a changelog; replay with no model; `/walk` records a browser skill; not the `SKILL.md`-with-frontmatter format the others share | have | have | have | have | `internal/skill`, `internal/skill/browser` |
| Plugins, MCP, or user tools | **partly**: an executable in `~/.nerdgenie/tools/` that answers `--describe`; no MCP client; no marketplace | have (MCP + plugins) | have (MCP + ~180 plugins) | have (`claude mcp`, plugins) | have (`codex mcp`, plugins) | `internal/tool/usertool.go`, `examples/tools/wordcount` |
| Hooks | **none** | have (four kinds) | have | have (30+ events) | have | — |
| Permissions and allow-lists | **have**: ask-me-first list, `permission_rules` (last match wins), once/always/reject with a reason, `/yolo`, standing approvals on skills | have | have | have | have | `internal/permission` |
| Sandboxing | **have**: bwrap + Landlock fence, off by default; the doctor probes whether the machine can fence | partly (docker/ssh backends, no OS fence) | partly (docker, off) | have (bubblewrap + socat on Linux) | have (Landlock + seccomp by default) | `internal/sandbox` |
| Installer | **partly**: `curl \| sh` plus a release tarball with the pinned Node, for Ubuntu and Debian; other distributions are told what to install; no published release to point at yet; `go install` gives a binary without the workers | have (`curl \| bash`, docker, nix) | have (`curl \| bash`, npm, docker) | have (`curl \| bash`, brew, apt/dnf/apk, npm) | have (`curl \| sh`, npm, brew) | `scripts/install.sh`, `scripts/release` |
| Update | **have**: `nerdgenie update`, with `--check`, `--to`, `--rollback`, and an automatic rollback | have | have | have (auto-update) | have | `internal/update` |
| Doctor | **have** | have | have | have | have | `cmd/nerdgenie/doctor.go`, `internal/config/doctor.go` |
| First-run wizard | **have**: four questions (work folders, model, key, Signal); finds llama-server, LM Studio, `claude`, `codex`; does not look for Ollama; every answer is a flag | have (3,900-line wizard) | have (`onboard`) | partly (login, theme) | partly (login) | `internal/command/initialize.go`, `initmodels.go` |
| Pictures and files in chat | **partly**: photos and files arrive over Signal into `inbox/`; screenshots draw inline on kitty and iTerm; no paste and no `@file` on the screen or in `run` | have (`/image`, `/paste`) | have | have (paste, `@file`) | have (`-i`, paste) | `internal/signal/inbox.go`, `internal/tui/pictures.go` |
| Voice | **none** | have (STT, TTS, wake word) | have | have (dictation) | none in the CLI | — |
| Web search | **have**: SearXNG when configured, otherwise the DuckDuckGo results page; a fetch tool with an SSRF guard | have | have | have | have | `internal/tool/web` |
| Project instruction file (`AGENTS.md`, `CLAUDE.md`) | **partly**: `SOUL.md`, `USER.md`, `MEMORY.md` in the home's persona folder; nothing is read from the folder the work is in | have (`.hermes.md` → `AGENTS.md` → `CLAUDE.md`) | have (`AGENTS.md` in the workspace) | have (`CLAUDE.md`, rules, auto memory) | have (`AGENTS.md`) | `~/.nerdgenie/persona`, `internal/context` |
| Per-project settings | **none** (settings are per home) | partly (`hermes project`) | partly (project registry) | have (`.claude/settings.json`) | have (`.codex/config.toml`) | — |
| Status line | **have**: model and model file, context meter, cache share, tokens, prefill and output speed, health dot; not user-configurable | have (`/statusbar`) | none | have (`/statusline`, scriptable) | partly | `internal/tui` |
| Headless or quiet mode | **have**: `serve` + `run -wait -yes -timeout`; exit codes; no JSON output | have (`-z`) | have (gateway, `--json`) | have (`-p --output-format json`) | have (`exec --json`) | `cmd/nerdgenie/run.go` |
| Logs and debugging | **partly**: `serve.log` wherever you redirected it; the event log is the truth; `replay` and `runreport`; no `--verbose`, no `nerdgenie logs` | have (`hermes logs`, `-v`) | have (`openclaw logs --follow`) | have (`--debug`, `/doctor`) | have (`RUST_LOG`, `codex doctor`) | `SETUP.md` §7, `cmd/nerdgenie/replay.go`, `scripts/runreport` |
| Documentation and examples | **partly**: a lot of writing, but `INSTALL.md` is about irene, rosie and the TurboQuant fork; no five-minute quickstart for a stranger; one example; no CONTRIBUTING or CHANGELOG | have (docs site) | have (~600 pages) | have | have | `README.md`, `INSTALL.md`, `SETUP.md`, `docs/` |

---

## 3. The gaps, one at a time

Each paragraph says what a user expects, what Nerd Genie has instead, the smallest thing that would fill the gap, a rough size, and whether it is essential for the first open-source release. "Essential" here means: a stranger who follows the README on a clean Ubuntu machine gets a working agent and is not surprised by the first three things they try. Keep-it-simple and you-are-not-going-to-need-it are applied throughout, so most of the list is *not* essential.

### G1. "What did you do two days ago?" — **essential (small form), about 1 day**

A user expects to ask the agent about earlier days in plain words and to see a dated list of what it did. Nerd Genie already holds the answer: every event in `nerdgenie.db` carries a UTC time, every task's ask and every ending report are in the log, and `/memory search` already indexes every past message. What is missing is the door. `/tasks` prints number, status and ask with no date; `statusanswer.go` answers "where are we" but only about the current or newest task; a message that names a day starts a fresh task in which the model sees an empty record and says nothing happened. Smallest fill: put the start date and time on each `/tasks` line (the time of the task's first event is already read for `StatusFieldTaskStarted`); accept `/tasks today`, `/tasks yesterday` and `/tasks 2026-09-04` as filters; and extend the small phrase list in `resuming.go` so a status question that names a day is answered from those same records, with each task's ask and its ending line, and no model call. That is one file each in `internal/loop`, `cmd/nerdgenie` and a test. Not needed: a natural-language date parser; three words and an ISO date cover it.

### G2. Carry on a *particular* earlier task — **not essential, about 1 day**

Claude Code and Codex let you pick any past session and continue it. Nerd Genie carries on the newest stopped or waiting task on each screen with any message, which is the common case, and `/tasks 17 back 3` already reloads an older record to rewind it. Smallest fill, if wanted later: `/tasks 17 continue` fills `loop.Task.ResumeID` the way the screen memory does, and `nerdgenie run --task 17 "..."` does the same from a script. Named sessions, `/new` and `/sessions` should stay out: a task number with a date on it is the session, and inventing a second object would fight the record.

### G3. Cost in money and per-task totals — **not essential, about half a day**

Users of cloud models expect `/cost` to show a dollar figure and a per-day total. Nerd Genie shows tokens (in, cached, out) for the session in `/status` and on the header, and the cost line is written into the record every turn, so the numbers exist; money is printed only when the `claude` program reports it, and the local model costs nothing. Smallest fill: register `/cost` as an alias of the cost part of `/status`, and sum the log's cost events per task and per day. A price table per alias (`price_per_million_in`, `price_per_million_out`) is the only way to show dollars for API providers and is a config field plus one multiplication; do it only when someone asks.

### G4. Profiles — **not essential, docs only**

`NERDGENIE_HOME=~/nerdgenie-b nerdgenie serve` already gives a second, fully separate setup, which is what Codex's `--profile` and Hermes' `hermes profile` are for. Say so in the README next to `init`; nothing to build.

### G5. Telegram (and the other channels) — **not essential for the first release, the first thing after; 3 to 4 days**

Every assistant-style agent has Telegram, and for most people it is far easier to set up than Signal (a bot token from BotFather, no phone linking, no signal-cli). Nerd Genie has terminal and Signal. The channel contract is small and Signal is a worked example that already solves pairing, splitting, previews and attachments, so the cost is mostly the Bot API client and tests. Section 4 has the findings and the sizing. Discord, Slack and WhatsApp are the same shape but each needs its own client; none of them should be in the first release.

### G6. A bell when it needs you — **essential, about half a day**

The single most common complaint with terminal agents is "I did not notice it was waiting for me". A task that hits the ask-me-first list, asks a question, finishes, or fails is announced on the screen and, on Signal, as a message; on the terminal nothing rings. Claude Code rings the terminal bell and sends a desktop notice on iTerm2, Ghostty and Kitty; Codex runs a `notify` command with a JSON payload. Smallest fill: the screen writes `\a` and an OSC 9 (or OSC 777) notification when it draws an ask, a done, a stopped or a failed event, gated by one config key `notify = "bell" | "off"`. An optional `notify_command` that receives one JSON line on stdin costs another half day and covers every desktop; leave it until asked.

### G7. Project instruction file read from the work folder — **essential, about 1 day**

Anyone who has used Claude Code, Codex, OpenCode or Hermes expects a file in the repository (`AGENTS.md`, or `CLAUDE.md`) to be read on every task: build commands, conventions, what not to touch. Nerd Genie reads only the home's persona files, so a user's repository notes are invisible unless they paste them into every ask. Smallest fill: when a task's working folder (the folder named in the ask or the first sandbox root) holds `AGENTS.md`, put its first 16 KB into the prompt as one block above cache boundary A, read once per task the way the skill list is read in `cmd/nerdgenie/taskcontext.go`, and say in the situation line that it was loaded. Fall back to `CLAUDE.md` when there is no `AGENTS.md`, as Hermes and OpenCode do. Not needed: the directory walk, path-scoped rules, imports, or a per-project settings file.

### G8. Ollama as a first-class local server — **essential, 1 to 2 days**

Most people who run a local model run Ollama, and the README promises "any language model, big or small". Today an Ollama entry works only by accident: `provider = "openai"` at `http://127.0.0.1:11434/v1` connects, but the output cap is silently dropped, thinking cannot be turned off, the context length is whatever Ollama decided (4,096 by default) rather than what `config.toml` says, and `nerdgenie init` does not look for it. Section 5 has the findings and the exact changes; they are the same shape as the llama-server probe that already exists.

### G9. Skills in the shared `SKILL.md` format — **not essential, about 1 day**

Claude Code, Codex, OpenCode and Hermes all read a skill as a folder with a `SKILL.md` whose YAML frontmatter carries `name` and `description`, and people trade them. Nerd Genie's `SKILL.md` is a heading, a line, `## Triggers` and `## Permissions`, with `steps.md` beside it. A skill that is only words (no steps) is already allowed. Smallest fill: let the loader accept a frontmatter `SKILL.md` as a words-only skill, mapping `name` and `description`, ignoring the rest, and refusing nothing else. Do it when someone drops one in and it fails.

### G10. MCP and plugins — **not essential; do not build for the release**

MCP is on every other list. Nerd Genie's answer is an executable in `~/.nerdgenie/tools/` that describes itself, which is simpler and goes through the same permission function. An MCP client over stdio is three to five days and drags in a JSON-RPC schema, tool listing, and prompt injection questions the data-boundary wrapping already guards against for tool results. Document the user-tool protocol with the `wordcount` example on the README, and wait to see whether anyone actually asks for MCP.

### G11. Hooks — **not essential**

Hooks are how Claude Code and Codex users run a formatter after every edit or block a command. Nerd Genie runs the language's own checker and the tests after every write on its own, and its permission rules deny by pattern, which covers the two most common hook uses. If a third one appears, a `hooks/` folder of executables run on `task_done`, `task_stopped`, `ask` and `tool_done` with a JSON line on stdin is one to two days and would also serve as the notify command of G6.

### G12. Pictures and files from the terminal — **not essential, about 1 day**

Over Signal a photo arrives in `inbox/` and rides on the message. On the screen there is no paste and no attach, though the `read` tool already reads a `.png` or `.jpg` by path and shows it to a model with eyes. Smallest fill: expand `@path` in a typed message to an attachment so it lands on `Inbound.Attachments` exactly as a Signal photo does, and add `nerdgenie run --file path`. Clipboard paste through the terminal's image protocols is a second day and can wait.

### G13. Voice — **not essential; do not build**

Hermes and OpenClaw have STT, TTS and wake words; Claude Code has dictation. Nothing in Nerd Genie's design needs it and it would add a runtime dependency the release does not carry. Signal voice notes already arrive as files in `inbox/`; the model is told a file is there.

### G14. Per-project settings — **not essential**

Claude Code and Codex read `.claude/settings.json` or `.codex/config.toml` from the repository. Nerd Genie's settings are per home, and the sandbox roots list is the closest thing. A second home is the answer for now (G4). G7 covers the instruction half of what people put in those files.

### G15. Headless output as JSON — **not essential, about half a day**

`nerdgenie run` prints the reply on stdout and everything else on stderr, which is enough for a shell script. Claude Code's `-p --output-format json` and Codex's `exec --json` are what people wire into other programs. The socket already speaks one JSON envelope per line, so `run --json` is a flag that stops translating and passes the envelopes through.

### G16. Finding the logs — **essential (docs and one shortcut), about half a day**

A user who runs `nerdgenie install` has the serve under systemd and will not know that `journalctl --user -u nerdgenie -f` is where its output went; a user who started `serve` by hand was told to redirect it. There is no `--verbose`. The event log, `nerdgenie replay` and `scripts/runreport` are excellent for the developer and invisible to the user. Smallest fill: `nerdgenie logs [-f]` that runs the journalctl line when the service is installed and otherwise prints the path it would read, plus a "when something looks wrong" section in the README that names the three places (the journal or `serve.log`, `nerdgenie.db`, and the model server's own log).

### G17. Documentation for a stranger — **essential, 2 to 3 days**

This is the largest gap and it is all writing. `INSTALL.md` is a runbook for irene, rosie, jarvis and sassy: the TurboQuant fork, a specific uncensored GGUF, `igo.sh`, Vulkan device numbers, port 8090, `digibyte-qt`. A person with a laptop and Ollama, or a Claude subscription, or an OpenAI key, has no page that takes them from `curl | sh` to a first reply in five minutes. `SETUP.md` is good and mostly general. `ARCHITECTURE.md` and `NERDGENIE.md` are for readers who want to understand the design, which is right, but they are the first thing a visitor sees after the README. Smallest fill: a `QUICKSTART.md` (or the top of the README) with three paths, Ollama, a subscription program, an API key, each ending at `nerdgenie doctor` and a first ask; move the machine-specific material of `INSTALL.md` into `docs/machines/` and keep a generic "install a local server" section that names llama-server and Ollama as equals; a `docs/CONFIG.md` generated from the doc comments in `internal/contract/config.go` so it cannot drift; a `CONTRIBUTING.md` that points at `make check` and the style checker; a `CHANGELOG.md` that the release workflow expects. No new features, only the words.

### G18. A published release and a clean-machine proof — **essential, about 1 day plus CI time**

`internal/update/source.go` points at `https://github.com/JaredTate/nerdgenie/releases/latest/download`, and `scripts/install.sh` fetches `SHA256SUMS` from the same place before it changes anything. Until a release is actually published there, the README's `curl | sh` line and `nerdgenie update` both fail at the first step. The container test in `scripts/release/container_test.go` and the `install.yml` workflow already prove an install on clean Ubuntu and Debian; run them against the first tagged release and keep the tag.

---

## 4. Telegram: how the others do it, and what one for Nerd Genie would take

**Hermes** (`/home/jared/Code/hermes-agent/plugins/platforms/telegram/`, about 12,000 lines including a 11,358-line adapter): `python-telegram-bot` 22.8. Long polling by default (`start_polling`); webhooks are opt-in through `TELEGRAM_WEBHOOK_URL`, `_PORT` (8443) and `_SECRET`. The token is `TELEGRAM_BOT_TOKEN` in `~/.hermes/.env` or `platforms.telegram.token`. Users are allow-listed by numeric id in `TELEGRAM_ALLOWED_USERS` (unset means nobody), layered with `dm_policy` = `open` | `allowlist` | `pairing` | `disabled` and a pairing-code store; the setup wizard can create a managed bot from a QR code or walks you through BotFather and tells you to message `@userinfobot` for your id, then asks for a home chat where cron results are delivered. Inbound photos, voice (transcribed), audio, video, documents and stickers come through `get_file` with per-type size caps; outbound `send_photo`, `send_document`, `send_video`, `send_voice`, `send_media_group`. Replies are split at 4,000 measured in UTF-16 units with `(n/m)` markers; formatting is MarkdownV2 with a plain-text retry when Telegram rejects the parse; typing is `send_chat_action` on a loop with a per-chat cooldown; approvals arrive as inline keyboards.

**OpenClaw** (`/home/jared/Code/openclaw/extensions/telegram/`, about 57,000 lines without tests): grammY with its runner. Long polling by default with one poller per token, a 409 guard, a 120-second liveness restart and a persisted offset; webhook mode when `webhookUrl` is set (own `node:http` listener on 127.0.0.1:8787, a required secret, a spool). Token in `channels.telegram.botToken`, a `tokenFile`, or `TELEGRAM_BOT_TOKEN`; `openclaw channels add telegram --token`. `dmPolicy` defaults to `pairing` (codes live one hour; `openclaw pairing approve telegram CODE`), else `allowlist` of numeric ids, `open`, or `disabled`; groups are allow-listed per chat with `requireMention` on by default. Attachments in and out including albums, `mediaMaxMb` 100, photo falling back to document on size errors. Split at 4,000 over a Markdown intermediate form so tags are never cut; captions at 1,024; HTML parse mode, never MarkdownV2; typing refreshed every four seconds; streaming by editing a draft message in modes `off` | `partial` | `block` | `progress`. Its docs cover BotFather's `/newbot`, privacy mode (`/setprivacy` off or make the bot an admin, then remove and re-add it to each group) and finding your numeric id.

**ZeroClaw** (`/home/jared/Code/zeroclaw/crates/zeroclaw-channels/src/telegram.rs`, about 477 KB): raw `reqwest` against `https://api.telegram.org/bot<token>/<method>`, no bot library, `getUpdates` long polling with a carefully monotonic offset, `channels.telegram.<alias>.bot_token` and `allowed_users` (`"*"` for everyone, empty for nobody), pairing on an unknown sender, `getFile` downloads, multipart `sendDocument`/`sendPhoto`/`sendVideo`/`sendAudio`/`sendVoice`, streaming through message edits, inline-keyboard approvals. This is the closest model for a Go port.

**The Bot API minimum** (core.telegram.org/bots/api): `getUpdates` with `offset` = last `update_id` + 1, `limit` up to 100, `timeout` in seconds (set 30 for long polling; the call is refused while a webhook is set); `sendMessage` with `text` up to 4,096 characters and an optional `parse_mode` of `HTML` or `MarkdownV2`; `sendChatAction` `typing`, which lasts about five seconds so it is refreshed; `getFile` then `https://api.telegram.org/file/bot<token>/<file_path>`, 20 MB at most; `sendPhoto` (10 MB) and `sendDocument` (50 MB) with captions of 1,024; `message.chat.id` is where to reply and `message.from.id` is who wrote; about one message a second per chat.

**Sizing for Nerd Genie.** One new package `internal/telegram` implementing `contract.Channel`, importing `contract` and the standard library only, exactly as `docs/EXTENDING.md` prescribes, and mirroring `internal/signal` file for file:

- *Client and stream*: `net/http` against the Bot API, no library; `getUpdates` long polling with `timeout: 30`, the offset advanced only after a message is handed to the queue, a growing wait on errors as the Signal stream has, and the same "drop what a person did not send" decoder. No webhooks: they need a public HTTPS address and Nerd Genie runs on a desktop. About 300 lines and a fake Bot API server in the tests.
- *Who may talk*: `telegram_allowed_users` in `config.toml` as numeric ids, and the pairing-code flow copied from `internal/signal/pairing.go` for an unknown sender (`/pair CODE` already exists; the store needs a channel-neutral home, either a small move into `internal/channel` or a second copy). Direct messages only, as Signal is today; groups later if asked.
- *Sending*: plain text with no `parse_mode` in the first version, so nothing has to be escaped and nothing is rejected; split at 4,000 with the paragraph-and-sentence rule already in `internal/signal/split.go`, after the redactor; `sendChatAction typing` every four seconds while a task runs; `SendFile` through `sendDocument`, with `sendPhoto` for a `.png`/`.jpg`; a preview as text answered with the same `approve` / `always` / `deny` words Signal uses. Inline keyboards are a nice second step.
- *Receiving files*: `getFile` into the attachment cache and `inbox/`, the same cap and eviction as Signal's cache, riding on `Inbound.Attachments`.
- *Setup*: `nerdgenie init` asks "Do you want to talk to it on Telegram? Paste the token BotFather gave you" through the masked prompt into the vault as `secret://telegram`, writes `telegram_bot_token = "secret://telegram"`, and prints "message your bot once and type /pair with the code it shows you" — which also finds the user's id without `@userinfobot`. `doctor` calls `getMe`.
- *Wiring*: one `openTheTelegramChannel` in `cmd/nerdgenie` beside `signalchannel.go`, the `/pair` registration widened to "when any paired channel is configured", and a doc page.

Roughly: client, stream and channel 1.5 days; pairing move, config, init question and doctor 1 day; tests with the fake server and the live test under the `live` tag 1 day; docs half a day. **Three to four days** for direct messages, attachments, typing and previews. Left out on purpose: webhooks, groups, HTML formatting, streaming edits, inline keyboards, voice transcription; each is half a day to a day later if anyone wants it.

---

## 5. Ollama: how its OpenAI-compatible endpoint differs from llama-server, and what a provider entry needs

Sources: docs.ollama.com (openai-compatibility, faq, api/chat, api/ps, cloud), the `openai/openai.go` and `server/sched.go` files in ollama/ollama, and ollama.com/search?c=cloud, all read on 6 September 2026.

**What is the same.** `POST /v1/chat/completions` with `model`, `messages`, `stream: true`, `stream_options.include_usage`, `tools`, `temperature`, `top_p`, `stop`, `seed`, `response_format`, and base64 images as `data:` URLs in `image_url` (plain http image URLs are refused). Tools stream. `/v1/models` lists what is pulled. A bearer token is accepted and ignored locally. Nerd Genie's existing `openai` provider connects and gets replies.

**What silently differs, and why it matters here.**

1. **The output cap is dropped.** Ollama's request struct has `max_tokens` and no `max_completion_tokens`; Go's decoder throws unknown keys away, so the 8,192-token cap Nerd Genie sends (`openaiwire.go` line 96, chosen because OpenAI's reasoning models refuse the old name) never reaches Ollama, and a reply runs until the model stops or the window is full. Support for `max_completion_tokens` is still an open pull request (ollama/ollama #14464).
2. **`chat_template_kwargs` and `keep_alive` and `options` are dropped too.** The thinking-off hint Nerd Genie sends to llama-server does nothing; on Ollama the same thing is `reasoning_effort: "none"`, which Ollama maps to `think: false` (`low`/`medium`/`high` pass through, higher levels clamp to `max`). `keep_alive` is honoured only on the native `/api/chat`; on `/v1` the server-wide `OLLAMA_KEEP_ALIVE` (default five minutes, negative for forever) is the only control. `num_ctx` cannot be passed on `/v1` at all.
3. **The context window is Ollama's, not `config.toml`'s.** Ollama's default is 4,096 tokens (newer builds tier it by VRAM: under 24 GiB 4k, 24 to 48 GiB 32k, above 256k), set by `OLLAMA_CONTEXT_LENGTH` on the server or `PARAMETER num_ctx` in a Modelfile. Nerd Genie sizes every prompt to `context_length` and llama-server's `/props` probe corrects a too-large number; Ollama has no `/props`, so a `context_length = 131072` alias on a 4,096-token Ollama gets its prompt truncated from the front with only a line in Ollama's own log. The real numbers are on `GET /api/ps` (`context_length` of the loaded model, `expires_at`) and `POST /api/show {"model": ...}` (`model_info["<arch>.context_length"]`, the model's maximum, and `capabilities`: `tools`, `vision`, `thinking`). A request whose `num_ctx` differs from the loaded runner's forces a reload.
4. **Names carry tags.** `qwen3.5:9b`, `gpt-oss:120b`, `glm-5.3:cloud`; the tag list is at `ollama.com/library/<model>/tags`. Nothing in Nerd Genie's config check refuses a colon, so this works as it stands.
5. **Usage is reported, cache included.** `prompt_tokens`, `completion_tokens`, and `prompt_tokens_details.cached_tokens` when the runner reports prompt tokens read from its cache; on a stream the usage rides on the final `finish_reason` chunk rather than on an extra empty chunk, and a tool call arrives whole in one delta rather than in argument fragments. Nerd Genie's stream reader accumulates by index and reads usage from any chunk, so both shapes are fine; the cache share on the header would come from `cached_tokens` instead of llama-server's `timings.cache_n`.
6. **Cloud models.** After `ollama signin`, `ollama pull glm-5.3:cloud` makes the same local `/v1` endpoint serve a hosted model under that name; the catalogue today lists `glm-5.3:cloud`, `glm-5.3-flash:cloud`, `glm-5.2:cloud`, `glm-5.1:cloud`, `deepseek-v4-pro:cloud`, `deepseek-v4-flash:cloud`, `kimi-k2.6:cloud`, `kimi-k3:cloud`, `minimax-m3:cloud`, `qwen3.5:cloud`, `gemma4:cloud`, `gpt-oss:20b-cloud` and `gpt-oss:120b-cloud`, tagged with tools, thinking and vision. The hosted API can also be called directly at `https://ollama.com/v1` with `Authorization: Bearer <OLLAMA_API_KEY>`, which is the ordinary OpenAI provider with a key. Pricing is metered per million tokens with free starter credits, Pro at 20 dollars a month and Max at 100.

**What a provider entry needs.** Keep `provider = "openai"`; do not add a fifth kind. Add one probe beside the llama-server one in `internal/provider/props.go`: on a loopback base address, `GET <base without /v1>/api/version`; when it answers with `{"version": ...}` the server is Ollama and the model gets three tweaks, exactly as `takesTheThinkingHint` is set today: send `max_tokens` (instead of `max_completion_tokens`); send `reasoning_effort: "none"` for the thinking level `off` and the default, and the level otherwise; and read `POST /api/show` for `context_length` and `capabilities`, preferring the smaller window over the configured one, setting a note when `vision = true` is written for a model without the `vision` capability, and telling the user to set `OLLAMA_CONTEXT_LENGTH` when the loaded window is smaller than the task needs. `ModelFileOf` can read the name back from `/api/ps` for the screen's header. Then `nerdgenie init` looks at `http://127.0.0.1:11434/api/tags` beside its llama-server and LM Studio checks and offers each pulled model by name, writing this block:

```toml
[[models]]
name = "ollama"
provider = "openai"
base_address = "http://127.0.0.1:11434/v1"
model_name = "qwen3.5:9b"         # any pulled model, cloud ones included after `ollama signin`
context_length = 32768            # what Ollama loaded, read from /api/ps; raise OLLAMA_CONTEXT_LENGTH to change it
vision = false                    # true only when /api/show lists the vision capability
```

and for the hosted API, the same block with `base_address = "https://ollama.com/v1"`, `key_reference = "secret://ollama"`, and the key in the vault. Size: probe and three tweaks half a day; `/api/show` and the window rule half a day; `init` detection half a day; a live test under the `live` tag and a page in the docs half a day. **One to two days**, and it turns "any local model" from a claim into a tested path.

---

## 6. Installer options for a Go program on Linux, and what each asks of the user

Nerd Genie is one static Go binary plus two Node workers (browser and desktop) that need a Node runtime, plus optional system packages: Google Chrome for the browser, bubblewrap for the fence, ripgrep for search, signal-cli for Signal. That second half is what makes this harder than a plain Go tool.

| Option | What the user does | What they get | What it asks of them | State today |
|---|---|---|---|---|
| **Release tarball with the pinned Node** (`make release`) | Download `nerdgenie-<v>-amd64.tar.gz`, check `SHA256SUMS`, untar into `~/.nerdgenie/releases/<v>/`, link `current`, run `init` | The binary, `node/bin/node` 24.18.0, both workers; `nerdgenie update` works from then on | Nothing installed system-wide; about 60 to 80 MB to download; Chrome, bwrap, ripgrep and signal-cli still by hand | Built and tested; nothing published yet |
| **`curl \| sh` installer** (`scripts/install.sh`) | One line, answer four questions (or pass them as flags after `--`) | Everything above, plus apt installs of bwrap, ripgrep and signal-cli, the AppArmor profile, a launcher in `~/.local/bin`, and `init` | Trust in a script from the web (it verifies the archive's checksum before it changes anything, and never runs `sudo bash` on something fetched); sudo for apt; Ubuntu or Debian for the packages, any Linux for the rest | Built, tested against a fake machine and in Ubuntu and Debian containers; waits on a published release |
| **`go install github.com/JaredTate/nerdgenie/cmd/nerdgenie@latest`** | One line, with Go 1.27 on the machine | The binary only: terminal, Signal, files, shell, web, memory, jobs, skills; the browser and desktop tools refuse until `make build` produces the worker bundles | Go installed; no browser; the user must know the workers are separate | Works in principle (the binary already comes up without workers and says so); not documented as a path |
| **A `.deb`** | `sudo apt install ./nerdgenie_<v>_amd64.deb`, or an apt repository later | Binary and workers under `/usr/lib/nerdgenie`, `Depends:` on bubblewrap and ripgrep, the AppArmor profile in `/etc/apparmor.d`, a man page | sudo; a system-wide install that dpkg owns, which conflicts with `~/.nerdgenie/releases/<v>/current` and with `nerdgenie update` swapping files under it (dpkg would then own files the updater rewrites); updates come from apt, so the rollback-on-failed-health logic would have to be dropped or moved | Not built; about one to two days with nfpm or fpm, plus a repository if updates are to be automatic |

**Recommendation for the first release.** Publish the tarball and keep `curl | sh` as the one documented path; it is what Claude Code, Codex, OpenCode, Hermes, ZeroClaw and Prime all lead with, it needs nothing but curl, and it is the only path on which `nerdgenie update` and its rollback were designed to work. Document `go install` as the developer's path in `CONTRIBUTING.md` with a plain sentence that the browser needs `make build`. Leave the `.deb` until there is a reason: it would give a second update mechanism that fights the first, which is exactly the OpenClaw failure `docs/HARNESS_V2.md` Part 4 describes. Two small things would make the installer friendlier: print the download URL and checksum line before running so a cautious user can fetch and read the script first, and make the non-Ubuntu message name the three packages with the commands for dnf and pacman.

---

## 7. The essential list

| Gap | Days |
|---|---|
| G17 documentation for a stranger (quickstart with three paths, generic install, config reference, contributing, changelog) | 2 to 3 |
| G18 a published release and the clean-machine workflow run against it | 1 |
| G8 Ollama probe, three tweaks, `init` detection | 1 to 2 |
| G1 dates on `/tasks`, a day filter, and a dated status answer | 1 |
| G7 `AGENTS.md` (or `CLAUDE.md`) read from the work folder | 1 |
| G6 a bell and desktop notice when it needs you or has finished | 0.5 |
| G16 `nerdgenie logs` and a where-the-logs-are section | 0.5 |
| **Total** | **7 to 9 days** |

Right after the release, in this order: Telegram (G5, 3 to 4 days), `run --json` (G15, 0.5), `@file` attachments on the screen (G12, 1), `/cost` with per-task totals (G3, 0.5). Not planned unless someone asks: MCP, hooks, voice, named sessions, per-project settings, a `.deb`, the shared `SKILL.md` format, profiles beyond `NERDGENIE_HOME`.

Report: `/tmp/claude-1000/-home-jared/09f06b9f-a0be-401c-831d-adb99bc858bb/scratchpad/testreport/gaps.md`
