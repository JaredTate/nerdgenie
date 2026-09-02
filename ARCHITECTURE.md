# Coeus Architecture

This document records how the code is put together and what each wave built. It is read by every worker before starting a brief and updated by any worker whose brief changes a package's job, its interface, or its dependencies. The orchestrator adds a wave section at the end of every wave. The design this code implements is `docs/COEUS_PLAN.md`; when the two disagree, the design is the intent and this document is the fact, and the orchestrator reconciles them at the wave gate.

Wave 0 is built: `internal/contract`, `internal/testkit`, `internal/lint`, the repository-map generator and its drift test, the skeleton of `cmd/coeus`, `worker/browser/PROTOCOL.md`, and the forty-step fixture. `internal/sandbox`, `internal/vault`, and the `coeus askpass` subcommand are built as well, ahead of the rest of wave 2, because each depends on nothing but `contract` and `testkit`. Everything else below describes what will exist once its wave is done, and is marked planned until then.

## Shape

One Go program owns everything: the message queue, the turn loop, the permission function, the task and job records, the memory, the jobs, and one SQLite database file. It starts two helper programs when it needs them, a browser worker and a desktop worker, both written in TypeScript, both speaking JSON-RPC over standard input and output. Two thin screens, the terminal and Signal, attach to the program over a local socket and own no state.

```
terminal ──┐                                   ┌── browser worker (TypeScript, Playwright, real Chrome)
           ├── local socket ──▶ coeus (Go) ────┤
signal ────┘   (JSON lines)        │           └── desktop worker (TypeScript, cua-driver)
                                   │
                              one SQLite file: event log, task and job records, memory index
                              plus markdown files: persona, memory, skills
```

## Packages and their one-sentence jobs

Packages are listed in build order, and a package may import only packages listed above it. Anything two packages both need is in `internal/contract`.

| Package | Job | Wave |
|---|---|---|
| `internal/contract` | Every interface, type, and constant that crosses a wave boundary, with no dependencies | 0, built |
| `internal/testkit` | Every fake, the golden-file helper, and the forty-step fixture data | 0, built |
| `internal/log` | The append-only event log in SQLite | 1, built |
| `internal/record` | The task record: parse, print, enforce its rules, checkpoint | 1 |
| `internal/config` | The configuration file and the home folder layout | 1, built |
| `internal/clock` | The real clock behind `contract.Clock`: the machine's time, a sleep that stops with its context, a ticker | 1, built |
| `internal/lint` | The plain-English style checker, used only by `make check` | 0, built |
| `internal/provider` | Turn a prompt into a streamed reply through the Anthropic API, the OpenAI-compatible API, or a vendor's command-line program on a subscription, with retries and the fallback chain | 1, built |
| `internal/repair` | Find the tool calls in a model reply, however the model wrote them | 1, built |
| `internal/context` | Build the working context from the layers, sized to the model | 2 |
| `internal/loop` | Run one turn: orient, call, guard, permit, run, update, repeat; the done-check and the after-action review | 3 |
| `internal/tool` | The tool registry and the built-in tools, one folder each | 2 |
| `internal/permission` | Decide allow, ask, or deny for a tool call | 2, built |
| `internal/sandbox` | Run a command inside bwrap and Landlock | 2, built |
| `internal/channel` | The queue, the event stream, and the local socket | 3 |
| `internal/command` | The command registry and the core slash commands | 3 |
| `internal/tui` | The terminal screen | 3 |
| `internal/signal` | The signal-cli client, linking, pairing, and the Signal channel | 3 |
| `internal/vault` | The encrypted secret store, the resolver, TOTP, the sudo password, redaction | 2, built |
| `internal/reliability` | Leases, ledgers, sentinels, the breaker, the watchdog feed, backups | 4 |
| `internal/memory` | The memory files, the search index, the hint, and zero-token capture | 4 |
| `internal/skill` | The skill folder format, loading, learning, replay | 4 |
| `internal/browser` | The Go side of the browser: worker lifecycle, login, handoff | 5 |
| `internal/job` | Job records, the task list, and the scheduler | 4 |
| `internal/desktop` | The Go side of the desktop worker | 6 |
| `internal/update` | Update, rollback, migrations | 6 |
| `internal/replay` | Re-run any logged task as a test | 6 |
| `cmd/coeus` | The binary; one file per subcommand; `main.go` and `serve.go` are the orchestrator's | 0 skeleton, 3 onward |
| `worker/browser` | The TypeScript browser worker | 5 |
| `worker/desktop` | The TypeScript desktop worker | 6 |

## The contracts (built, wave 0)

`internal/contract` holds the interfaces every fake and every real implementation share. Their full definitions live in the code with doc comments; this is the list.

- `Model`: name, context length, and one call that sends a `Request` and streams the reply back. A `Request` carries the system prompt as ordered blocks with the three cache boundaries on them, the messages, the tool specifications, whether tools are off, and the output cap. There are three provider kinds: `anthropic`, `openai` (the OpenAI-compatible API at a base address), and `cli`, which runs the vendor's own command-line program (`claude` or `codex`) on the user's subscription and gets text back.
- `Tool` and `ToolRegistry`: name, description under forty words, typed input fields, permission classes, run function returning text; and the set of tools one turn can see. The eighteen tool names are constants, and so is the one text form of a tool call, `<tool_call>{"name": ..., "arguments": ...}</tool_call>`, which a model reached through a command-line program has to write and `internal/repair` reads.
- `Channel`: receive, send, send a file, preview and collect a decision, masked prompt, health.
- `Permission`: decide allow, ask, or deny; the answers once, always for the session, reject with a reason.
- `Memory`: search, get, save, hint.
- `Command`: name, help line, run function; each package exports its slash commands as values and `serve.go` registers them.
- `Skill`: list, load, run, save.
- `Job`: create, add a task, list, run now, pause, switch off; and for the loop, the next due task, a finished task's report, and the job's record.
- `Sandbox`: run a command inside the fence.
- `Secrets`: resolve a reference, get the sudo password, redact text.
- `BrowserWorker`: the methods in `worker/browser/PROTOCOL.md`.
- `Desktop`: launch, screenshot, click, type, key, drag, clipboard.
- `Clock`: now, sleep, ticker.
- `Store`: the event log's write and read shapes.
- The record types: one set for both kinds, with a kind field saying task or job. A task's header carries the budget line and the cost line and its work carries plan steps and results (`r7`); a job's header carries the progress line and the next due task and its work carries the task list (`t31`) and the reports of finished tasks (`j4.2`).
- The configuration struct with every field and its default, the home folder layout as path helpers with the file modes, and the exit codes (75 restart me, 78 bad configuration).
- The user-tool protocol: an executable in `~/.coeus/tools/` answers `--describe` with JSON and takes a JSON object on standard input.

## The permission function (built, wave 2)

`internal/permission` implements `contract.Permission`. It depends on `internal/contract` and the Go standard library and nothing else.

Every call is first **reduced to a readable form**, by `permission.Reduce`. A shell command is cut down to the program and the words that matter and nothing that varies, so `git commit -m "..."` becomes `git commit` and `rm -rf /tmp/x` becomes `rm -rf`. A quote-aware splitter first cuts the line into the separate commands a shell would run, at the three shell operators and at the openers and closers of a command substitution, so a command hidden behind a pipe or inside `$(...)` is still read as a command; the parts are joined back with a pipe. A shape table, which is OpenCode's arity table written fresh for the programs Coeus rules on, says how many words define each program and whether its flags belong in the readable form, which is what keeps `git reset --hard` apart from `git reset`. Environment assignments and `env` are dropped, and a `sudo` prefix and its own flags are handled so that the program sudo will run is the one that gets reduced. Every other tool reduces to its name and the one field that says what it will do, which is the path, the address, the search words, or the intent; and a `write` or `edit` that would empty a file over four thousand bytes says so in the readable form itself, so that one mechanism, pattern matching, covers that case too. The readable form always fits on one line and is capped at two hundred runes, because it is matched on, logged, and shown.

The **rulebook** is a list of rules of tool, pattern, and action, compiled once into matchers that ignore capital letters; the last rule that matches wins, and a call no rule matches is allowed, because the agent runs on its own by default. The tool of a rule is a glob too, so `browser_*` is one rule. `permission.New` takes a `contract.Config` and builds the book from it: the shipped entries the user kept in `ask_me_first` first, then the user's own rules from `permission_rules`, so a rule the user wrote can change what an entry would have said. An entry named in `ask_me_first` that nobody shipped is a configuration error naming it, so a typo in config.toml is reported rather than silently dropping a safety rule. A rule written in a file carries no reason of its own, so when it wins it is described by itself: the pattern, the tool, and the action. The **three shipped entries** are data in one file: deleting many files at once is eleven patterns over the shell, write, edit, browser and desktop tools; running a command with sudo is two patterns, which catch both the word and the shell tool's `escalate` field, because the reducer writes `sudo` into the readable form when that field is set; and spending money is the six words buy, pay, purchase, checkout, subscribe and order paired with the four tools that can spend it. An empty ask-me-first list stands for no rules, and then everything the user's own rules do not cover simply runs.

A ruling of ask carries a **preview** of exactly what is about to happen: the whole shell command, and the reason the model gave when it asked for administrator powers; the file and how many bytes it holds now; the intent and the element on the page. The user's answer is remembered **in memory for the session only**: once records nothing, always allows every later call with the same readable form, and reject refuses them with the words the user gave. A **standing approval** registered by a skill names the skill, the readable form it covers, a limit on uses, and an expiry read against `contract.Clock`; it is checked after the answers the user gave, so a rejection still wins. When a run is unattended, a call that would have asked comes back ruled **`contract.RulingStop`** instead, carrying the same preview, so the loop stops the task and reports what it needed rather than waiting for somebody who is not there.

The tool input field names this package reduces by are the ones fixed in `docs/briefs/wave-2/2.5-tools.md`: `command`, `escalate` and `reason` for the shell; `path` and `content` for a write; `path`, `old` and `new` for an edit; `pattern` for a search; `url` and `query` for the web; and `intent`, `element` and `text` for the browser and desktop tools. They live in one table in `reduce.go`, and a name that is not there falls back to the tool's bare name rather than failing.

## The local socket (planned, wave 3)

The terminal and any future screen attach to the running program over a Unix socket at `~/.coeus/run/coeus.sock`, speaking newline-delimited JSON. Message types from the screen: `message`, `command`, `approve`, `deny`, `secret` (for the masked prompt), `attach`, `detach`. Message types from the program: `delta`, `reply`, `preview`, `ask`, `handoff`, `status`, `error`. The socket is itself a channel. The Signal channel does not use it; it runs inside the program and feeds the same queue.

## The browser worker protocol (document built, wave 0; code in wave 5)

`worker/browser/PROTOCOL.md` defines the JSON-RPC methods the Go side calls: `open`, `read`, `click`, `type`, `press`, `scroll`, `act`, `tabs`, `loginFill`, `screenshot`, `health`, and `dialog`, and the snapshot and diff shapes every method returns. The fake worker in `testkit` and the real worker implement the same document.

## The sandbox (built, wave 2, brief 2.3)

`internal/sandbox` implements `contract.Sandbox`. It has two halves, and both have to hold for the fence to mean anything.

**Outside.** `New` takes the sandbox roots from the configuration, the user's home directory, the tool output cap, and the program to start inside the fence. `contract.CheckSandboxRoot` refuses a root that is not a full path, or that is, sits inside, or *holds* `~/.coeus`, the vault, the browser profile, or `~/.ssh`; this package calls it and adds the one rule only something about to run a command needs, that the root is a folder that is really there, plus a cap of sixteen roots. The configuration package calls the same check, and this package calls it again, because it is the last thing between a bad root and a command that can read the vault.

`Available` has three things to be sure of, and the third can only be found out by trying: `bwrap` on the PATH, `landlock` in the kernel's list at `/sys/kernel/security/lsm`, and a `bwrap` that is actually allowed to make a user namespace. Ubuntu ships with AppArmor refusing that last one to an unconfined program, so an installed bwrap is not the same as a working one. The probe is one `bwrap --unshare-user --ro-bind / / /bin/true` under a ten-second timeout, asked once per fence and remembered, and when it fails the error names the fix in a sentence: write an AppArmor profile for `/usr/bin/bwrap` that allows `userns`, or set `kernel.apparmor_restrict_unprivileged_userns` to 0. The shell tool of brief 2.5 turns itself off whenever `Available` returns an error.

`Run` builds a `bwrap` command line: new user, process, message-queue, and hostname namespaces, never a new network namespace, `--die-with-parent`, `--new-session`, `--clearenv`, a fresh `/proc`, `/dev`, and `/tmp`, `/usr`, `/bin`, `/lib`, `/lib64`, and `/etc` bound read-only, each root bound read-write at its own path, and the helper program bound read-only so the fence can start it. That last bind is the one deliberate exception to "nothing else is bound": the helper is the coeus binary, which normally lives under `~/.coeus/releases/`, and one read-only file is what it costs to have Landlock applied at all. The environment is `PATH`, `HOME` pointing at a scratch folder inside the first root, `LANG`, `TERM`, whatever the caller passed, and one marker saying the fence started the helper. The command runs in its own process group, its two output streams are capped at the tool output cap with a note saying how much was dropped, and on a timeout the whole group gets `SIGTERM`, then `SIGKILL` two seconds later.

**Inside.** bwrap cannot apply Landlock, so the fence starts `<the coeus binary> sandbox-entry --read <folder> ... --write <folder> ... -- <program> <arguments>`. The helper is `sandbox.Entry`, exported as the subcommand value `sandboxEntrySubcommand` in `cmd/coeus/sandbox_entry.go` for the orchestrator to add to the table in `main.go`. It refuses to run without the fence's marker, locks its operating-system thread, sets the no-new-privileges flag, builds a Landlock ruleset through the three raw system calls (444, 445, and 446 on both `amd64` and `arm64`) with the structures laid out by hand and the rights chosen from the ABI version the kernel reports, installs a seccomp filter written instruction by instruction that answers fifteen system calls no tool needs with "operation not permitted" and refuses an `unshare` that asks for a new user namespace, and becomes the command. Both restrictions survive the change of program. It says nothing at all on its way through, so that a shell result is the command's own output and nothing else; the one line saying how far it got is written only when it could not become the command.

**What the tests prove.** The bwrap command line for a fixture configuration is a golden file, and so is the seccomp filter's byte-for-byte encoding on each architecture. A fuzz test throws any root and any command at the fence and asserts that nothing forbidden is ever bound read-write.

Every integration test runs the real bwrap and the real Landlock, and stops with the reason when the machine will not allow a fence. A command inside one reports itself as process one or two, sees a mapped `uid_map` rather than the machine's own, and finds neither `.ssh` nor `.coeus` in the home directory. A sandboxed read of `~/.ssh` and a sandboxed write to `~/.coeus` both fail; a write inside the root and a network call to a loopback server both work; a command that outruns its timeout is killed along with the grandchild it started; output past the cap is dropped with a note; and a `bwrap` that cannot make a namespace makes `Available` name the AppArmor fix.

The three calls that can never be undone are applied for real on an operating-system thread that is then thrown away, which is what lets a test watch Landlock deny a read outside the roots, watch a write inside one succeed, and watch a denied system call turn from "bad file number" into "operation not permitted", without crippling the rest of the run.

## Data on disk (built, wave 1)

`internal/config` owns this layout and the one file at the top of it. The root is the folder `$COEUS_HOME` names when that variable is set and `~/.coeus` otherwise; `config.Root` and `config.HomeFolder` are the only places that decide, and every path below comes from the helpers on `contract.Home`. `$COEUS_HOME` is the only environment variable Coeus reads.

```
~/.coeus/
  config.toml            read once at startup
  coeus.db               the one SQLite file
  persona/               SOUL.md, USER.md, MEMORY.md
  memory/                markdown files, indexed
  skills/<name>/         SKILL.md, steps or script, test, changelog
  tools/                 the user's own tools, one executable each
  vault.age, vault.key   the secret store and its key, mode 0600
  browser/<profile>/     Chrome user data, mode 0700, outside the sandbox
  inbox/                 files and photos received over Signal
  releases/<version>/    installed binaries; `current` is a symlink
  run/                   the socket and the lock
  backups/               nightly encrypted archives
```

`config.Load` reads `config.toml` once at startup. It starts from
`contract.DefaultConfig`, decodes the file over it, fills in the three fields
whose default is a place rather than a value (the browser profile, the backup
folder, and the sandbox roots), and then checks every field, so a missing file
and an empty file are both valid configurations. Decoding is strict: a key the
configuration does not have is refused rather than ignored. The keys are the
`toml` tags on `contract.Config`, which write each field name in lower case with
underscores between the words, so `DefaultModel` is `default_model` and
`BaseAddress` is `base_address`; a key run together without the underscores, such
as `defaultmodel`, is refused as unknown. The model aliases are a table array of
`[[models]]` blocks, the caps live in `[caps]` and `[memory_caps]`, and a length
of time may be written either as a string such as `"30m"` or as a whole number of
nanoseconds. Every refusal is a
`config.Problem`, which prints as `path:line: key: what to do`; the line comes
from the TOML library when it reports one and from this package's own scan of
the file otherwise.

The checks are the rules the rest of the program may then assume: every alias has
a provider kind from `contract.ProviderKinds`, an address or a vendor program to
match it, a model name, and a context length above zero; a key is a
`secret://name` reference and never a key; the default model and every fallback
name an alias that exists; every cap and every length of time is above zero; a
Signal account is a phone number in international form; every entry kept on the
ask-me-first list is one of the three `contract.DefaultAskMeFirst` ships, and an
emptied list is allowed; every permission rule the user writes names a tool, has
a pattern, and says `allow`, `ask`, or `deny`, never `stop`, which is what the
permission function decides on its own for an unattended run; and no sandbox root
is, sits inside, or holds the paths `contract.ExcludedFromSandbox` names, nor sits
above the user's home directory. The user's home directory itself is refused,
because it holds the daily browser profile, cloud credentials, and keys, and the
fence can only grant, never subtract; the shipped default is the work folder
`~/coeus`, which `coeus init` creates.

`config.Doctor` reads a home folder and returns a `config.Report`: one `Finding`
per check, and a `Verdict` that is the worst of them. The line about
`config.toml` says which model the file reaches for, how much of the
ask-me-first list the user kept, and how many rules of their own they wrote. A warning means something
Coeus can work without is switched off, such as a missing browser or a Signal
account that has not been linked; a problem means something is broken, such as a
vault key other accounts can read or a configuration that will not load. The
doctor changes nothing at all: it stats the layout, loads the configuration,
looks for `signal-cli`, `bwrap`, `rg`, `google-chrome` or `chromium`, and `node`
on the PATH, and makes one read-only request to the local model daemon's health
endpoint, which it works out from the base address of the `local` alias. `coeus
init` and `coeus doctor` in wave 3 both print `Report.String()`.

### The event log inside `coeus.db` (built, wave 1)

`internal/log` owns the first two tables in the one SQLite file, and `internal/log/doc.go` is the fuller description. `events` holds one row per thing that happened: a sequence number that is the primary key, that only ever grows, and that is never reused; the time it happened, written as text in UTC so that any moment comes back exactly as it went in; the task or job it belongs to, which may be empty; its kind, one of the eight in `contract.EventKind`; and its own fields as JSON. There is one index on the task and one on the kind, which are two of the read shapes, and nothing cleverer than that. `schema_version` holds one number, `1`, which is where wave 6's migrations start. Nothing in either table is ever changed or removed.

The file is opened in write-ahead mode with `synchronous=NORMAL` and a five-second busy timeout, through one connection for writing held behind a mutex, so there is only ever one writer, and four connections for reading. `Append` returns the sequence number it gave the row, which is what lets a caller write down what it is about to do before doing it. The three list reads, by task, by kind, and by a span of sequence numbers, stop at `log.MaxEventsPerRead`, ten thousand rows, so nothing can pull the whole log into memory by accident. A read that fills that cap is never truncated quietly: it hands back the events it read **together with** an error naming the last of them and saying to read the rest in pages with `ByRange` starting after that number, so a caller cannot mistake part of an answer for the whole one. `Replay` streams a row at a time instead and so has no such limit. Opening a file that holds another program's tables, or one written by a newer Coeus, is refused with an error that names the file. A row the running version cannot read is an error from every read rather than a silently skipped line.
## The vault (built, wave 2)

`internal/vault` is the one place a secret lives. It implements `contract.Secrets` and depends on nothing but `internal/contract`, the standard library, `filippo.io/age`, and `github.com/pquerna/otp`.

The file is `~/.coeus/vault.age`, encrypted with age to an X25519 identity whose private key is the one line in `~/.coeus/vault.key`, mode `0600`, made on first use and published with a hard link so that two copies of the program starting at once cannot overwrite each other's key. Opening refuses a key file that is not a plain file, that anyone but the owner can read, or that belongs to another account, and the refusal says the mode to set. Inside the encrypted file is a small JSON document, a version and the whole list of entries, rewritten in full and published atomically on every change: write beside it, wait for the disk, rename, sync the folder. Every parse error names the file, so that the user knows what to restore. An entry is a name, a site, the hostnames the login may be typed into, a username, a password, and a two-factor secret; the entry named `sudo` holds only the machine password. `vault.Entry` prints as `[secret]` and turns into the JSON string `"[secret]"`, so no value can leak by being printed or serialized into a tool result.

Four ways out, and no others. `Resolve("secret://name")` hands the harness a `contract.Credential`, for the browser's login tool and the provider's key lookup; the model writes the reference and never sees the value. `Code(name)` makes the six-digit, thirty-second, SHA-1 code and says how many seconds are left, so the login tool can wait for a fresh one when fewer than five remain. `SudoPassword()` serves `coeus askpass`, which prints the machine password on standard output for `sudo -A` through `SUDO_ASKPASS` and says on the error output when there is none. `Redact(text)` blacks out every stored value of six characters or more and everything that looks like a secret whether the vault holds it or not — key shapes, bearer tokens, `password=` and `token=` values, AWS key ids, private key blocks, GitHub and Slack tokens, and a six-digit code on a line that names it — in one scan of one expression compiled when the program starts. A value enters through `vault.AskSecret`, which reads the terminal with its echo off through `TCGETS` and `TCSETS`, shows an asterisk for each character, refuses anything that is not a terminal, and puts the terminal back on every path out, the interrupt key included. `vault.NewCommand(v)` is the `/vault` slash command, terminal only, with `list`, `add`, `remove`, and `test`.

## The fakes and the fixture (built, wave 0)

`internal/testkit` holds one fake for every interface above, so that a package written in a later wave can be tested before its neighbours exist. Beside each fake is a `Check` function that takes the **interface** rather than the fake and asserts the properties the contract promises: a unit test calls it on the fake and a `live` test calls it on the real thing, which is what keeps the two from drifting. There are also tests that hand each check a deliberately broken implementation and confirm the check catches it.

The set is: a scripted model whose steps can require that a piece of text is still somewhere in the request, which is how a test catches a harness that dropped a correction; a provider server on a loopback port that speaks both wire protocols from the same script, records every request so a test can find the cache markers, and can stall, rate limit with a `Retry-After`, fail, overflow the context in each API's own shape, or drop the stream part way through; a channel; a clock that moves only when a test moves it; a temporary home built on the real filesystem with the right modes; a permission decider; a tool registry and a scripted tool; memory, skill, job, sandbox, and secrets stores; a signal-cli daemon with a health check, an event stream, and a record of what was sent, plus a `signal-cli` program a test can put on its PATH for the linking flow; a search server serving SearXNG-shaped JSON, a DuckDuckGo-shaped results page, and fixture pages including one that tries to give the agent orders; a browser worker with five fixture pages and six failure modes, offered both in process and over a local socket speaking `worker/browser/PROTOCOL.md`; a desktop; and the golden-file helper.

**The forty-step fixture** is in `test/fixtures/forty-step/task.json` and is loaded by `testkit.LoadFortyStepTask`. It is the tweet example from design section 4: forty rounds, a correction at round twelve in the user's exact words, a login page at round thirty that fires the stop condition the model wrote in round one, and a final report where every done line points at a result. The loader turns it into a script the fake model plays and into the scripted tool results, and it exposes the three assertions as functions, so wave 1, wave 3, and the live suite all run the same code rather than each writing its own idea of what "works on any model" means.

## Tool-call repair (built, wave 1)

`internal/repair` has one exported function, `Find`. It takes a `contract.Reply`, the specifications of the tools that really exist, and the number of parses that have already failed in a row, and it returns the tool calls, the text that is left once the tool-call envelopes are taken out, any names it repaired, a note when it had to stay inside a bound, and a problem written for the model when something looked like a call and could not be read. It never panics, and either the calls or the problem come back, never both. It depends on `internal/contract` and on the standard library, and nothing else.

Before any shape is read, a reasoning model's thinking is taken out: everything between a `<think>` tag and its closing tag, in any case, one block or many, and everything after a block that never closes. Nothing inside the thinking is ever read as a call and nothing inside it is ever handed back as the answer, because a model reasoning aloud writes calls to itself that it has not decided to make. What was written outside the blocks is kept, joined so that a block taken out of the middle leaves no gap. Thinking that is still open where the searching cap falls swallows the rest of the reply, so half a thought never leaks past the cap.

It then reads five envelope shapes, most trusted first, and the first shape that finds anything is the only one read: the calls the provider already parsed; a `<tool_call>` block, with or without its closing tag, one block or many, holding one call or a list of them; JSON in a code fence with the `json` tag or no tag; a bare JSON object in the text carrying both a name and an arguments object; and a line of the form `name({...})` or `name: {...}`. A written name is read as a real tool by four rules in order — the same name, the same name in another case, the same name with the underscores and hyphens taken out, and a name within two edits when it is at least five characters — and a name equally close to two real tools is refused rather than guessed at. Arguments must be a JSON object; a string that itself holds one object is read once more; nothing where the arguments go is read as no arguments. Everything is bound: a reply is searched only to 128 KB, on a character boundary; a reply may ask for at most as many calls as `Caps.IdenticalCallWindow`, and the rest are dropped with a note; a name over 120 characters is refused; and the regular expressions are compiled once. After `MaxFailedParses` failed parses in a row, text that still looks like a call comes back as the answer with no problem, so that the loop of wave 3 stops fighting the model. The loop keeps that count; this package only applies the rule.

The designs are borrowed and named at the top of each file: the envelope shapes and their order of trust from ZeroClaw's parser, the stripping of the thinking before anything else reads the reply and the dropping of a block that never closes from ZeroClaw's parser and its channel orchestrator, the repair of a near-miss name and the cap on a name's length from OpenClaw's grammar, and the rule that a bad call becomes an error the model can fix from OpenCode's invalid-call path.
## The model providers (built, wave 1)

`internal/provider` turns one `contract.Request` into a streamed `contract.Reply`, three ways, on the standard library alone. `provider.New` takes one `contract.ModelAlias` from the configuration and an `Options` value carrying the clock, the resolved key, the home folder, and a one-line log, and returns a `contract.Model`.

- **The Anthropic provider** posts to `{base}/v1/messages` with `x-api-key` and `anthropic-version: 2023-06-01`, always streaming. The system prompt goes as ordered text blocks, and a block that ends a cache boundary carries `cache_control`. Boundary B is expressed on the last tool instead, because the Messages API reads the tools before the system prompt; at most four markers are ever sent. No sampling field and no thinking field is ever sent. The stream reader answers `message_start`, `content_block_start`, `content_block_delta` (text and partial tool-call JSON accumulated per block index), `content_block_stop`, `message_delta`, `message_stop`, `ping`, and `error`.
- **The OpenAI-compatible provider** posts to `{base}/chat/completions` with a bearer token when there is one, `stream: true`, and `stream_options.include_usage`. The system blocks are joined with blank lines into one system message, because this wire has no form for a cache boundary and both OpenAI and llama-server reuse a prefix on their own. Tool results go back as `tool` role messages. The output cap is `max_completion_tokens`. Tool-call arguments are accumulated by their index, and the final chunk carries the usage with no choices at all.
- **The command-line provider** runs `claude -p` or `codex exec` once per call on the user's own subscription, which is how the two cloud models are reached on a machine with no API keys. It renders the system blocks and the tools into text, writes every tool call in the one text form from `contract`, feeds the conversation on standard input, and runs the program in an empty folder under `~/.coeus/run/cli/` so that no CLAUDE.md, no project settings, and no hooks can reach the prompt. `claude` takes its system prompt on `--system-prompt` and streams; `codex` takes its own through the `model_instructions_file` setting and returns one delta. The reply carries no structured tool calls: `internal/repair` reads them back out of the text. A hung run is killed by its exact process group identifier, never by anything matching a name.

Every reply names the model that answered in `Reply.Model`, and carries what the call cost in `Usage.CostUSD` when the provider reports money at all, which today is only the `claude` program. `Usage.InputTokens` is everything the model read: on the Messages API that is the plain input tokens plus what was written into the cache plus what was read back out of it, and `Usage.CachedInputTokens` is that last part on its own, so the cached count is always inside the input count. The Chat Completions API already reports the whole prompt in one field, so nothing is added there.

Around any of them, `provider.WithRetries` tries three times in all, waiting one second and then two with up to a quarter added, honouring `Retry-After` in both its forms, and never retrying a request that was simply too long. `provider.NewChain` tries the models in order and moves on when one is out of tries, setting `Reply.Model` to the alias that actually answered, but hands `contract.ErrContextOverflow` straight back, because only the context builder can fix a prompt that does not fit. Hermes' two local-server rules are kept: a base address on this machine is probed once at `/props`, and a server that answers the way llama-server does is sent `chat_template_kwargs: {"enable_thinking": false}` and has a smaller reported window preferred over the configured one.

Every wait in the package is measured on `contract.Clock`, including the sixty-second watch that gives up on a stream which has gone quiet, so no test ever waits on the real clock. `provider.New` refuses an `Options` with no clock in it; the running program passes `clock.System()` and the tests pass the fake.

## Test architecture

Four kinds of tests, described in `docs/WORK_PLAN.md`: unit, integration, functional, fuzz. Two tiers of model: the scripted fake on every commit, and three real models (the local Qwen 3.8 through the llama-server daemon on the development machine, Opus 4.8 through the Claude Code program on the user's subscription, and GPT-5.5 through the Codex program on the user's subscription, both driven by the `cli` provider) at every wave gate under the `live` tag. Every fake has a contract test against the real thing. The forty-step fixture is the proof of the record and the context builder.

## Repository map

`REPO_MAP.md` is generated from the tree by `make repo-map`. The generator omits dependency folders, build outputs, test artifacts, and version-control internals. A drift test in `make check` fails when the map is stale.

## Wave log

Each wave gate adds a section here: what was built, what changed in the interfaces, what the next wave depends on. Three paragraphs at most.

### Wave 0 (planned)

The development machine readied, the skeleton, the contracts, the browser protocol document, the repo-map generator, and the first versions of the three living documents.
