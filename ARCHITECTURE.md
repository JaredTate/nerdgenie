# Coeus Architecture

This document records how the code is put together and what each wave built. It is read by every worker before starting a brief and updated by any worker whose brief changes a package's job, its interface, or its dependencies. The orchestrator adds a wave section at the end of every wave. The design this code implements is `docs/COEUS_PLAN.md`; when the two disagree, the design is the intent and this document is the fact, and the orchestrator reconciles them at the wave gate.

Wave 0 is built: `internal/contract`, `internal/testkit`, `internal/lint`, the repository-map generator and its drift test, the skeleton of `cmd/coeus`, `worker/browser/PROTOCOL.md`, and the forty-step fixture. `internal/sandbox`, `internal/vault`, and the `coeus askpass` subcommand are built as well, ahead of the rest of wave 2, because each depends on nothing but `contract` and `testkit`. Everything else below describes what will exist once its wave is done, and is marked planned until then.

Wave 0 is built: `internal/contract`, `internal/testkit`, `internal/lint`, the repository-map generator and its drift test, the skeleton of `cmd/coeus`, `worker/browser/PROTOCOL.md`, and the forty-step fixture. `worker/browser` itself is built too, ahead of its wave, because it depends on nothing but that document. Everything else below describes what will exist once its wave is done, and is marked planned until then.

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
| `internal/record` | The task record and the job record: parse, print, enforce their rules, checkpoint | 1, built |
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
| `internal/channel` | The queue, the router, the event stream, and the local socket | 3, built |
| `internal/command` | The command registry and the core slash commands | 3 |
| `internal/tui` | The terminal screen | 3 |
| `internal/signal` | The signal-cli client, linking, pairing, and the Signal channel | 3 |
| `internal/vault` | The encrypted secret store, the resolver, TOTP, the sudo password, redaction | 2, built |

| `internal/signal` | The signal-cli client, linking, pairing, and the Signal channel | 3, built |
| `internal/vault` | The encrypted secret store, the resolver, TOTP, the sudo password, redaction | 2 |
| `internal/reliability` | Leases, ledgers, sentinels, the breaker, the watchdog feed, backups | 4 |
| `internal/memory` | The memory files, the search index, the hint, and zero-token capture | 4 |
| `internal/skill` | The skill folder format, loading, learning, replay | 4 |
| `internal/browser` | The Go side of the browser: worker lifecycle, login, handoff | 5 |
| `internal/job` | Job records, the task list, and the scheduler | 4 |
| `internal/desktop` | The Go side of the desktop worker | 6 |
| `internal/update` | Update, rollback, migrations | 6 |
| `internal/replay` | Re-run any logged task as a test | 6 |
| `cmd/coeus` | The binary; one file per subcommand; `main.go` and `serve.go` are the orchestrator's | 0 skeleton, 3 onward |
| `worker/browser` | The TypeScript browser worker | 5, built early |
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

## The record (built, wave 1)

`internal/record` owns the one text form of a task record and a job record, which is the pair of examples in section 4 of the design, and it owns the rules that make the record worth trusting. `Print` is the only thing in Coeus that writes that text and `Parse` is the only thing that reads it; the two round-trip byte for byte on both examples, which are the golden files in the package's `testdata`. Anything that must sit on one line is folded when it is printed and unfolded when it is read, so a message with more than one line in it comes back exactly as the user wrote it.

The two are held together by a check rather than by an argument: before any change is saved, the record is printed, read back, and compared with itself, and a change that would read back as something else is refused. That is what makes the promise true rather than merely intended, and it has one cost worth knowing about. A few pieces of text cannot be written as they stand, because a line of a record uses them as its own marks: an arrow (` -> `) inside a plan step, a comma and a space inside a task on a job's list, where the date it waits for goes, and the words `. Reason: ` or `. Cause: ` inside a decision or a failure. The writer is told to rephrase. If a later brief needs those texts accepted, the fix is to fold them the way this package already folds a line break, and the check stays as the thing that would have caught the mistake.

Every change goes through a `Keeper`, which holds one record and the event log behind it. The model's half of the record goes through one door, `Apply`, which takes an `Update` carrying only what the model may write: the why, the done list, the stop list, the plan or the task list, one decision, one failure. There is no field there for the ask, for a correction, for the header, or for the situation, so rules seven and eight of the brief are kept by the shape of the type rather than by a check. The harness's half is the rest of the methods: the budget, the cost line, the progress line, the situation, corrections, results, reports, and the check marks on the plan and the task list. Every rule has a named error, so a caller can tell one refusal from another with `errors.Is` and hand the model back a line it can act on, and an update is all or nothing: nothing changes unless every rule holds.

A result gets the next label, one line of at most seventy characters in the record, and its whole text in the log as a tool-result event, which is what `Read` brings back long after the text left the model's window. The label comes from the log rather than from the record in hand, so a record wound back three steps and then worked on again never hands out a label the abandoned path already used, and a checkpoint on either path still reads back its own evidence. Every change also saves the next numbered checkpoint into the log, holding the printed record, so `Load`, `LoadCheckpoint`, and `Back` need nothing but the log: a task can be put down for days and picked up on a different model, or wound back three steps to try another path. The done-check reads the done list and returns the lines with nothing behind them, and a record cannot be set to done while any of them is waiting. The one estimate of size lives here too: a record filled to a hundred-round budget measures about 2,800 tokens by this package's ratio, under the three thousand the design promises.

Everything a record writes to the log goes under `contract.RecordLogKey`, which is a task's own number and `j` and the number for a job, because task 17 and job 17 are different records and their events must never mix. `Load`, `LoadCheckpoint`, and `Back` take the kind alongside the number for the same reason, and a checkpoint that turns out to hold the other kind is refused rather than handed back. Anything in a later wave that logs an event about a record, such as a tool call or a record change, uses the same key, which a keeper hands out as `LogKey`.

One note for later waves. The integration test in this package writes to a file of JSON lines rather than to `internal/log`, because the two packages were written in the same wave; when wave 3's functional suite joins them, the same test should run against the real log.

## The permission function (built, wave 2)

`internal/permission` implements `contract.Permission`. It depends on `internal/contract` and the Go standard library and nothing else.

Every call is first **reduced to a readable form**, by `permission.Reduce`. A shell command is cut down to the program and the words that matter and nothing that varies, so `git commit -m "..."` becomes `git commit` and `rm -rf /tmp/x` becomes `rm -rf`. A quote-aware splitter first cuts the line into the separate commands a shell would run, at the three shell operators and at the openers and closers of a command substitution, so a command hidden behind a pipe or inside `$(...)` is still read as a command; the parts are joined back with a pipe. A shape table, which is OpenCode's arity table written fresh for the programs Coeus rules on, says how many words define each program and whether its flags belong in the readable form, which is what keeps `git reset --hard` apart from `git reset`. Environment assignments and `env` are dropped, and a `sudo` prefix and its own flags are handled so that the program sudo will run is the one that gets reduced. Every other tool reduces to its name and the one field that says what it will do, which is the path, the address, the search words, or the intent; and a `write` or `edit` that would empty a file over four thousand bytes says so in the readable form itself, so that one mechanism, pattern matching, covers that case too. The readable form always fits on one line and is capped at two hundred runes, because it is matched on, logged, and shown.

The **rulebook** is a list of rules of tool, pattern, and action, compiled once into matchers that ignore capital letters; the last rule that matches wins, and a call no rule matches is allowed, because the agent runs on its own by default. The tool of a rule is a glob too, so `browser_*` is one rule. `permission.New` takes a `contract.Config` and builds the book from it: the shipped entries the user kept in `ask_me_first` first, then the user's own rules from `permission_rules`, so a rule the user wrote can change what an entry would have said. An entry named in `ask_me_first` that nobody shipped is a configuration error naming it, so a typo in config.toml is reported rather than silently dropping a safety rule. A rule written in a file carries no reason of its own, so when it wins it is described by itself: the pattern, the tool, and the action. The **three shipped entries** are data in one file: deleting many files at once is eleven patterns over the shell, write, edit, browser and desktop tools; running a command with sudo is two patterns, which catch both the word and the shell tool's `escalate` field, because the reducer writes `sudo` into the readable form when that field is set; and spending money is the six words buy, pay, purchase, checkout, subscribe and order paired with the four tools that can spend it. An empty ask-me-first list stands for no rules, and then everything the user's own rules do not cover simply runs.

A ruling of ask carries a **preview** of exactly what is about to happen: the whole shell command, and the reason the model gave when it asked for administrator powers; the file and how many bytes it holds now; the intent and the element on the page. The user's answer is remembered **in memory for the session only**: once records nothing, always allows every later call with the same readable form, and reject refuses them with the words the user gave. A **standing approval** registered by a skill names the skill, the readable form it covers, a limit on uses, and an expiry read against `contract.Clock`; it is checked after the answers the user gave, so a rejection still wins. When a run is unattended, a call that would have asked comes back ruled **`contract.RulingStop`** instead, carrying the same preview, so the loop stops the task and reports what it needed rather than waiting for somebody who is not there.

The tool input field names this package reduces by are the ones fixed in `docs/briefs/wave-2/2.5-tools.md`: `command`, `escalate` and `reason` for the shell; `path` and `content` for a write; `path`, `old` and `new` for an edit; `pattern` for a search; `url` and `query` for the web; and `intent`, `element` and `text` for the browser and desktop tools. They live in one table in `reduce.go`, and a name that is not there falls back to the tool's bare name rather than failing.

## Signal (built, wave 3)

`internal/signal` is the second channel. It depends on `internal/contract`, the
standard library, and one outside library, `github.com/mdp/qrterminal/v3`, which
draws the linking code. It imports no other package of Coeus.

**The daemon.** Signal is reached through **signal-cli**, a separate program
linked to the user's Signal account as a secondary device. `signal.Daemon` starts
it as `signal-cli -a <account> daemon --http <host>:<port> --no-receive-stdout`,
in a **process group of its own** (`Setpgid`), and stops it by signalling that
group by the child's **exact process identifier**, never by anything that looks
like its name. `Start` polls the health endpoint every 250 milliseconds and gives
up after `DaemonStartupTimeout`, thirty seconds, killing what it started. A
daemon that dies is started again after a wait that grows the same way the
stream's does, and after `MaxDaemonStarts`, twenty starts, the supervisor stops
trying. Coeus starts no daemon at all when `ChannelOptions.Program` is empty,
which is how a machine running signal-cli in a container is served, and is what
the tests use.

**The three paths.** signal-cli 0.13.23 serves exactly three: `/api/v1/check`
answers the health check, `/api/v1/events` streams inbound messages as
server-sent events with the account as a query parameter, and `/api/v1/rpc` takes
every JSON-RPC call. There is no fourth, so an attachment is fetched by calling
`getAttachment` with the attachment's opaque identifier and the sender, and the
daemon answers with the file as base64 under a `data` field. `signal.Client`
makes those calls, treats a `201` answer as a success with nothing to say, caps
what it will read, and sends every outbound message through
`contract.Secrets.Redact` first, so a secret cannot leave this way.
Attachments land in a cache under the home's Signal folder capped at
`AttachmentCacheLimit`, two hundred megabytes, from which the files that have
been there longest are removed first.

**The stream.** `signal.Stream` reads the server-sent frames, joins the `data:`
lines of one event, ignores the daemon's keepalive comments, and hands each
payload to the decoder in `event.go`. That decoder drops everything a person did
not send: a receipt, a typing notification, an envelope with no sender or no
words, and anything carrying a `syncMessage` key at all, because signal-cli
writes that key with a null under it and a message the agent already sent must
never come back as a new one. A stream that ends is opened again after a wait
that starts at two seconds and doubles to sixty; a stream that says nothing for
two minutes is dropped and opened again at once. Every wait is measured on
`contract.Clock`, so no test waits on a real one.

**Pairing.** A sender the agent does not know gets an eight-character code drawn
without bias from thirty-two characters that cannot be mistaken for each other,
and nothing else. Codes live one hour, three may be waiting at once, one sender
may ask once every ten minutes, and five wrong tries shut the door for an hour.
They are kept salted and hashed in `~/.coeus/signal/pairing.json`, mode 0600,
written to a file beside it and moved into place, and compared with
`subtle.ConstantTimeCompare` against every waiting entry rather than stopping at
the first match. Approved senders are kept the same way in
`~/.coeus/signal/approved.json`. The `/pair` command, exported as
`signal.PairCommand`, approves the sender whose code matches; it refuses anywhere
but the terminal until the first sender is paired, after which a paired sender
may pair the next one from their phone.

**The channel.** `signal.Channel` implements `contract.Channel` under the name
`signal`. Inbound messages come only from paired senders and only from direct
conversations, because a reply to a group message would go to the wrong place. A
reply goes to whoever last wrote, and to the account itself when nobody has
written yet; it is split at paragraph breaks into as few messages as fit Signal's
two-thousand-unit length, never in the middle of a sentence while a sentence
break is available, capped at ten messages with a note on the last one, and sent
with a typing indicator around it. A preview arrives as the actual text with the
three answers explained once, and waits `PreviewTimeout`, thirty minutes, for
`approve`, `always`, or `deny`. A handoff arrives through `SendFile` with its
screenshot. `AskSecret` always returns `contract.ErrNoMaskedPrompt`, because
Signal cannot hide what is typed. One "still working" note is sent five minutes
into a task that has said nothing, and only one. Inbound photos and files are
downloaded, kept in the cache, copied into `~/.coeus/inbox/`, and their paths
ride on `contract.Inbound.Attachments`.

**What the orchestrator wires.** `cmd/coeus/signal.go` holds `signalSubcommand`,
whose one action is `link`: it finds signal-cli on the PATH, draws the `sgnl://`
address it prints as a code, waits five minutes for the phone, and writes
`signal_account` into `config.toml` in place of the line that was there.
`signal.NewChannel` builds the channel, `channel.Pairing()` hands back the store,
and `signal.PairCommand(store)` is the command to register, so that both work on
the same codes.

## Channels: the queue, the router, the event stream, and the local socket (built, wave 3)

`internal/channel` owns the four parts every channel sits on, and `internal/channel/doc.go` is the fuller description. It imports `contract`, `testkit` in its tests, and the SQLite driver `internal/log` already brought in, and it knows nothing about the agent loop, the command registry, or the skill folder: everything it needs from them is a function `cmd/coeus/serve.go` supplies.

**The queue** is one table, `queued_messages`, in the same `coeus.db` file the event log owns. Every inbound message from every channel is written there before anything looks at it, so a message cannot be lost, and the loop's caller drains it in order with `Take`. A row is written **waiting**, marked **taken** when it is handed out, and marked **done** when the task it started ends. Opening the queue puts every row the last run left taken back into the line and clears the rows already done, so a crash between the taking and the finishing hands the message out again with `Queued.Duplicate` set, which is how the loop learns the work may already have happened. `Add` refuses once the queue holds `Caps.QueuedMessages` unfinished messages, and the refusal wraps `channel.ErrQueueFull`, whose text is written to be sent straight back to whoever wrote the message. `Arrived` carries one nudge per message so the drainer waits rather than asking again and again. **The event log must be opened before the queue**, because `log.Open` refuses a file whose tables are not its own, and a file holding only the queue's table is such a file; an integration test pins both halves of that.

**The router** takes one message and decides what it is. A text that starts with `/` is a command: the name is read up to the first space of any kind, lower-cased, looked up, and run with the channel the message came from, and an unknown one is answered with a single line naming `/help`. A command marked `TerminalOnly` is refused anywhere but the terminal, which is how the vault stays out of Signal. Otherwise `contract.Skill.Match` is asked, and a match runs the skill without the model. Anything else is work for the loop. A skill store that cannot answer is trouble rather than a verdict: the message goes to the loop as a task and the error comes back beside the kind, because a broken shortcut must never lose the user's words.

**The event stream** is one feed of `contract.SocketEnvelope` events from the loop that every attached channel subscribes to. `Publish` refuses anything but the seven types the program sends, and it refuses a `status` whose state field is not one of the five `contract.State` words, so that the program and every screen read one spelling; a status's fields are the `contract.StatusField` names, its command list is separated by `contract.StatusCommandSeparator`, and any other field goes through untouched, because a screen ignores a field it does not know. A subscriber that falls `SubscriberBacklog` events behind is dropped, its channel closed with `Dropped` set, and the rest go on untouched, because a screen nobody is watching must never stall the loop. A stream carries at most `MaxSubscribers` readers.

**The local socket** is a Unix socket at the home folder's `run/coeus.sock`, mode `0600`, speaking one JSON envelope per line both ways as `contract` defines. A screen sends `attach` and is then sent everything on the event stream, `detach` stops that and leaves it connected, `message` and `command` are written into the queue, and `approve`, `deny`, and `secret` answer the preview or masked prompt with that id. Several screens may attach at once, up to `DefaultMaxClients`. A line past `DefaultMaxLineBytes`, a line that is not one JSON object, or a type only the program sends earns one `error` envelope and a disconnect; a blank line is passed over. Listening clears a socket file an earlier run left behind and refuses one another copy of the agent is answering on.

The socket is itself a `contract.Channel` named `terminal`, so the terminal is a channel exactly as Signal is. It sends a reply and a file to every attached screen, shows a preview and waits for the first approve or deny, and asks for a secret with `contract.SocketEnvelope.MaskInput` set, which is what tells a screen to hide what is typed and to answer with a `secret`. An approve whose text is `contract.ApproveAlwaysText` means always for the session, and any other approve means this once. A question nobody is attached for, and one nobody answers within `Options.AnswerDeadline`, both come back as `reject`, because nothing on the ask-me-first list happens without a yes; the deadline defaults to the shipped `Caps.TimePerTurn`, since a question asked inside a turn cannot usefully outlive it. Every piece of text on its way to a screen goes through `contract.Secrets.Redact` first, and the field a secret rides in is always emptied. `Receive` is the live copy of what the socket took in, for anything that wants to watch; the queue, not that stream, is the path work travels on.

`serve.go` supplies the router's four things — `FindCommand`, `FindChannel`, `StartTask`, and the skill store — and the socket's five: the path, the stream, the queue, the vault, and the clock. Nothing crossing the socket is named in this package any more: the masked-prompt flag, the word for "always", the status field names, the state words, and the command separator are all `contract`'s, so the terminal screen reads them without importing anything of this package's.

## The browser worker protocol (document built, wave 0; code in wave 5)

`worker/browser/PROTOCOL.md` defines the JSON-RPC methods the Go side calls: `open`, `read`, `click`, `type`, `press`, `scroll`, `act`, `tabs`, `loginFill`, `screenshot`, `dialog`, and `health`, and the snapshot and diff shapes every method returns. The fake worker in `testkit` and the real worker implement the same document.

## The browser worker (built, ahead of wave 5)

`worker/browser` is a TypeScript program on Node that drives a real Google Chrome
through `playwright-core` and speaks `worker/browser/PROTOCOL.md` over its
standard input and output. It was built early because it depends on nothing but
that document. Run it as
`node worker/browser/dist/main.js --profile <folder> [--chrome <path>] [--pacing human|fast]`.
`--pacing fast` exists only for its own tests; the Go side never passes it.

It launches the Chrome binary with its own profile folder, never the user's daily
one, on a loopback DevTools port the operating system picks, reads the address
Chrome prints, and attaches with `connectOverCDP`. Nothing but JSON-RPC responses
goes to standard output; its logging goes to standard error, one line per event.

Inside, one file has one job. `wire` and `params` turn a line into a request or
into the exact error the protocol names. `page-script` is the JavaScript that runs
inside the page, kept as text because it runs in Chrome and not in Node;
`page-bridge` calls it with a deadline on every call. `snapshot` builds the compact
tree the model sees, with a ref written onto each element so that a ref names the
same element for as long as it exists, and carries the wall so that `open` and
`read` can report a login page before the agent has done anything. `diff` compares
two snapshots, `expectation` judges the result against what the model said it
expected, and `walls` reports a login form, a prompt for a second code, or a
captcha. `refs` finds an element again when its ref has gone stale, by role and
name and then by visible text. `actions` and `pacing` do the thing at the speed a
person would. `pdf` saves a PDF page through the browser's own session, because
Chrome's viewer exposes no text to a program. `redact` takes the vault's secrets
back out of everything `loginFill` would otherwise hand back. `settle`, `session`,
`tabs`, `chrome`, `lines`, and `main` hold the waiting, the state, the tabs, the
browser, the input framing, and the process.

Three rulings shape how it behaves. An expectation is met when a word from it
turns up in a new element, in the new address, in the new title, in a dialog's
message, or in the element the action was aimed at; that last place is what lets
typing meet an expectation at all, since typing changes no element. Settling is
measured from the action rather than from whatever the page last did on its own,
changes to attributes alone do not count, and a page that never comes to rest is
read as it stands and returned with `settled: false`, so a chat or a clock stays
usable; `-32001` is kept for the page that cannot be read at all. And `dialog` is
the twelfth method, because Chrome stops a whole tab until a dialog is answered
and nothing else can free it.

Its dependencies are in `docs/DEPENDENCIES.md`. `npm test` builds and then runs
unit tests, property tests with `fast-check`, and tests that drive a real Chrome
against recorded fixture pages served on a loopback port, and fails under seventy
percent coverage.

## The sandbox (built, wave 2, brief 2.3)

`internal/sandbox` implements `contract.Sandbox`. It has two halves, and both have to hold for the fence to mean anything.

**Outside.** `New` takes the sandbox roots from the configuration, the user's home directory, the agent's own home folder, the tool output cap, and the program to start inside the fence. `contract.CheckSandboxRoot` refuses a root that is not a full path, or that is, sits inside, or *holds* any of the paths `contract.ExcludedFromSandbox` names: the agent's home, the vault, the browser profile, and `~/.ssh`. Both sides of that comparison are followed through their links, so a root that is a link to the home directory is judged as the home directory. This package calls it and adds the two rules only something about to run a command needs, that the root is a folder that is really there and that what the fence keeps is the folder its links lead to, plus a cap of sixteen roots. The configuration package calls the same check, and this package calls it again, because it is the last thing between a bad root and a command that can read the vault.

The excluded paths are worked out from two directories, not one. `$COEUS_HOME` can put the agent's home, and with it the vault and the browser profile, anywhere on the machine, so `Settings.AgentHome` carries where it really is. Whoever builds the fence passes the `Root` of the `contract.Home` it is already holding, which is what `cmd/coeus/serve.go` must do when the orchestrator wires the fence at the wave-3 gate; nothing builds a fence yet. An empty value means the default `~/.coeus`, and it is right only when `$COEUS_HOME` has not moved it. A root is kept as the folder its links lead to because bwrap binds the folder a link leads to and Landlock hangs its rule on the same folder, so a fence built from the link itself would bind one path and then be asked to write under another. The working directory is measured the same way, against the roots as they are held.

`Available` has three things to be sure of, and the third can only be found out by trying: `bwrap` on the PATH, `landlock` in the kernel's list at `/sys/kernel/security/lsm`, and a `bwrap` that is actually allowed to make a user namespace. Ubuntu ships with AppArmor refusing that last one to an unconfined program, so an installed bwrap is not the same as a working one. The probe is one `bwrap --unshare-user --ro-bind / / /bin/true` under a ten-second timeout, asked once per fence and remembered, and when it fails the error names the fix in a sentence: write an AppArmor profile for `/usr/bin/bwrap` that allows `userns`, or set `kernel.apparmor_restrict_unprivileged_userns` to 0. The shell tool of brief 2.5 turns itself off whenever `Available` returns an error.

`Run` builds a `bwrap` command line: new user, process, message-queue, and hostname namespaces, never a new network namespace, `--die-with-parent`, `--new-session`, `--clearenv`, a fresh `/proc`, `/dev`, and `/tmp`, `/usr`, `/bin`, `/lib`, `/lib64`, and `/etc` bound read-only, each root bound read-write at its own path, and the helper program bound read-only so the fence can start it. That last bind is the one deliberate exception to "nothing else is bound": the helper is the coeus binary, which normally lives under `~/.coeus/releases/`, and one read-only file is what it costs to have Landlock applied at all. The environment is `PATH`, `HOME` pointing at a scratch folder inside the first root, `LANG`, `TERM`, whatever the caller passed, and one marker saying the fence started the helper. The command runs in its own process group, its two output streams are capped at the tool output cap with a note saying how much was dropped, and on a timeout the whole group gets `SIGTERM`, then `SIGKILL` two seconds later.

**Inside.** bwrap cannot apply Landlock, so the fence starts `<the coeus binary> sandbox-entry --read <folder> ... --write <folder> ... -- <program> <arguments>`. The helper is `sandbox.Entry`, exported as the subcommand value `sandboxEntrySubcommand` in `cmd/coeus/sandbox_entry.go` for the orchestrator to add to the table in `main.go`. It refuses to run without the fence's marker, locks its operating-system thread, sets the no-new-privileges flag, builds a Landlock ruleset through the three raw system calls (444, 445, and 446 on both `amd64` and `arm64`) with the structures laid out by hand and the rights chosen from the ABI version the kernel reports, installs a seccomp filter written instruction by instruction that answers fifteen system calls no tool needs with "operation not permitted" and refuses an `unshare` that asks for a new user namespace, looks the program up on the fence's own `PATH`, and becomes the command. That lookup is there because `syscall.Exec` searches no path of its own while the fence sets `PATH=/usr/local/bin:/usr/bin:/bin`, so a tool that names its program `sh`, as the shell tool of brief 2.5 does, would otherwise fail with a message that blamed the wrong thing; it runs inside the fence and after Landlock, so it finds only what the fence allows, and a program that is nowhere is refused with "a full path or a program on the fence's PATH". Both restrictions survive the change of program. It says nothing at all on its way through, so that a shell result is the command's own output and nothing else; the one line saying how far it got is written only when it could not become the command.

**What the tests prove.** The bwrap command line for a fixture configuration is a golden file, and so is the seccomp filter's byte-for-byte encoding on each architecture. A fuzz test throws any root and any command at the fence and asserts that nothing forbidden is ever bound read-write.

Every integration test runs the real bwrap and the real Landlock, and stops with the reason when the machine will not allow a fence. A command inside one reports itself as process one or two, sees a mapped `uid_map` rather than the machine's own, and finds neither `.ssh` nor `.coeus` in the home directory. A sandboxed read of `~/.ssh` and a sandboxed write to `~/.coeus` both fail; a write inside the root and a network call to a loopback server both work; a command that outruns its timeout is killed along with the grandchild it started; output past the cap is dropped with a note; `Available` returns no error at all on the development machine, and a `bwrap` that cannot make a namespace makes it name the AppArmor fix instead. A root that is a link is fenced where the link leads, a root that is a link to the home directory, to `~/.ssh`, or to the agent's own home is refused, a link inside a root still cannot reach `~/.ssh`, and a command named `sh` rather than `/bin/sh` is found on the fence's PATH and runs.

**Two pieces beyond the brief, kept on purpose.** Brief 2.3 asked for neither the user-namespace probe nor `Settings.HelperProgram`, and both stay. Ubuntu ships with AppArmor refusing an unconfined program a new user namespace, so an installed bwrap is not the same as a working one, and a shell tool that turned itself off on the strength of "bwrap is on the PATH" would be wrong on most fresh machines; the probe is the only way to know, and it is asked once per fence and remembered, through a field a test can replace so that the remembering itself is proved. `Settings.HelperProgram` exists because the fence has to start the coeus binary from inside itself, and a test needs to point that at a program that is really on disk, which the test binary is; it defaults to this program, which is what production uses.

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

`internal/vault` is the one place a secret lives. It implements `contract.Secrets` and depends on nothing but `internal/contract`, the standard library, `filippo.io/age`, and `github.com/pquerna/otp`. `vault.Open` takes the home folder and a `contract.Clock`, which is the only thing it needs the time for: saying how long a two-factor code has left. The running program passes `clock.System()`, a test passes the clock it controls, and a vault opened with no clock at all is refused.

The file is `~/.coeus/vault.age`, encrypted with age to an X25519 identity whose private key is the one line in `~/.coeus/vault.key`, mode `0600`, made on first use and published with a hard link so that two copies of the program starting at once cannot overwrite each other's key. Opening refuses a key file that is not a plain file, that anyone but the owner can read, or that belongs to another account, and the refusal says the mode to set. Inside the encrypted file is a small JSON document, a version and the whole list of entries, rewritten in full and published atomically on every change: write beside it, wait for the disk, rename, sync the folder. Every parse error names the file, so that the user knows what to restore. An entry is a name, a site, the hostnames the login may be typed into, a username, a password, and a two-factor secret; the entry named `sudo` holds only the machine password. `vault.Entry` prints as `contract.SecretMarker` and turns into the JSON string `"[secret]"`, which is the same marker `contract.Credential` prints as, so no value can leak by being printed or serialized into a tool result whichever of the two is holding it.

Four ways out, and no others. `Resolve("secret://name")` hands the harness a `contract.Credential`, for the browser's login tool and the provider's key lookup; the model writes the reference and never sees the value. `Code(name)` makes the six-digit, thirty-second, SHA-1 code and says how many seconds are left, so the login tool can wait for a fresh one when fewer than five remain. `SudoPassword()` serves `coeus askpass`, which prints the machine password on standard output for `sudo -A` through `SUDO_ASKPASS` and says on the error output when there is none. `Redact(text)` blacks out every stored value of six characters or more and everything that looks like a secret whether the vault holds it or not — key shapes, bearer tokens, `password=` and `token=` values, AWS key ids, private key blocks, GitHub and Slack tokens, and a six-digit code on a line that names it — in one scan of one expression compiled when the program starts. A value enters through `vault.AskSecret`, which reads the terminal with its echo off through `TCGETS` and `TCSETS`, shows an asterisk for each character, refuses anything that is not a terminal, and puts the terminal back on every path out, the interrupt key included. `vault.NewCommand(v)` is the `/vault` slash command, terminal only, with `list`, `add`, `remove`, and `test`. Its `add` takes the fields that are not secret on the line itself, `add <name> <site> <domain,domain> <username>`, so that the user can see what they are typing, and asks through the masked prompt only for the password and the two-factor secret; the machine password is `add sudo`, which asks for one value and nothing else.

## The fakes and the fixture (built, wave 0)

`internal/testkit` holds one fake for every interface above, so that a package written in a later wave can be tested before its neighbours exist. Beside each fake is a `Check` function that takes the **interface** rather than the fake and asserts the properties the contract promises: a unit test calls it on the fake and a `live` test calls it on the real thing, which is what keeps the two from drifting. There are also tests that hand each check a deliberately broken implementation and confirm the check catches it.

The set is: a scripted model whose steps can require that a piece of text is still somewhere in the request, which is how a test catches a harness that dropped a correction, and which gives up when its context is done; a provider server on a loopback port that speaks both wire protocols from the same script, checks the same expectations the fake model checks against the request body it was actually sent, records every request so a test can find the cache markers, and can stall, rate limit with a `Retry-After`, fail, overflow the context in each API's own shape, drop the stream part way through, or go wrong mid-stream in the protocol's own error shape; a channel that hands each attach its own stream and closes it when the context is cancelled or the channel shuts down; a clock that moves only when a test moves it; a temporary home built on the real filesystem with the right modes; a permission decider that acts on what the user answered, so an always allows the same request for the session and a reject refuses it with the user's reason; a tool registry and a scripted tool; memory, skill, job, sandbox, and secrets stores; a signal-cli daemon with a health check, an event stream a test can drop or make go quiet, an attachment named on the event by an opaque id whose bytes come back through the `getAttachment` call, and a record of what was sent, plus a `signal-cli` program a test can put on its PATH for the linking flow; a search server serving SearXNG-shaped JSON, a DuckDuckGo-shaped results page whose links are wrapped in the redirect the real one uses, a record of every query, a misbehave switch, and fixture pages including one that tries to give the agent orders; a browser worker with five fixture pages and the failure modes the protocol names, offered both in process and over a local socket speaking `worker/browser/PROTOCOL.md`; a desktop; and the golden-file helper, whose rewrite switch is the environment variable `COEUS_UPDATE_GOLDEN` rather than a flag, because a flag registered from a file that is not a test collides with the next package that registers one.

Every fake is bounded and every one of them reports rather than blocks: a queue that is full says so instead of hanging the test, the provider server caps the request body it will read, and the browser's protocol server has a read deadline, a cap on one request line answered with -32700, a limit on how many callers it serves at once, and a context with a timeout around one method call. Its error table is the whole of the one in `worker/browser/PROTOCOL.md`, so that no page open is -32002 and a browser that has gone is -32003. The fake browser judges an expectation by the rule the protocol writes out rather than by whether anything went wrong, reports a wall on the snapshot that `open` and `read` return, and hands back a page that never came to rest as it stood with `settled` false, keeping -32001 for a page it cannot read at all.

**The forty-step fixture** is in `test/fixtures/forty-step/task.json` and is loaded by `testkit.LoadFortyStepTask`. It is the tweet example from design section 4: forty rounds, a correction at round twelve in the user's exact words, a login page at round thirty that fires the stop condition the model wrote in round one, and a final report where every done line points at a result. The loader turns it into a script the fake model plays and into the scripted tool results, and it exposes the three assertions as functions, so wave 1, wave 3, and the live suite all run the same code rather than each writing its own idea of what "works on any model" means.

The script makes the model write the record as well as read it: a round that writes to the record asks for the `task` tool as a second call in the same reply, so a harness that never writes the plan or the decision fails the script rather than passing it. A record write is a tool call like any other and gets a result label, which is why the two done lines point at `r42` and `r43`. Every round after the correction expects the correction to still be in the request, and every round after the stop expects the user's reply as well, so a harness that runs straight through the stop condition fails at round thirty-one. `FortyStepUpdate` is the contract-shaped reading of one round's record write, and `RecordAtTheEnd`, `PlanAtTheEnd`, `DecisionsAtTheEnd`, `FailuresAtTheEnd`, and `DoneLinesAtTheEnd` build the record the fixture describes out of it, so that every package driving the fixture reads it the same way. The loader refuses a fixture that writes something no reader knows.

## Tool-call repair (built, wave 1)

`internal/repair` has one exported function, `Find`. It takes a `contract.Reply`, the specifications of the tools that really exist, and the number of parses that have already failed in a row, and it returns the tool calls, the text that is left once the tool-call envelopes are taken out, any names it repaired, a note when it had to stay inside a bound, and a problem written for the model when something looked like a call and could not be read. It never panics, and either the calls or the problem come back, never both. It depends on `internal/contract` and on the standard library, and nothing else.

Before any shape is read, a reasoning model's thinking is taken out: everything between a `<think>` tag and its closing tag, in any case, one block or many, and everything after a block that never closes. Nothing inside the thinking is ever read as a call and nothing inside it is ever handed back as the answer, because a model reasoning aloud writes calls to itself that it has not decided to make. What was written outside the blocks is kept, joined so that a block taken out of the middle leaves no gap. Thinking that is still open where the searching cap falls swallows the rest of the reply, so half a thought never leaks past the cap.

It then reads five envelope shapes, most trusted first, and the first shape that finds anything is the only one read: the calls the provider already parsed; a `<tool_call>` block, with or without its closing tag, one block or many, holding one call or a list of them; JSON in a code fence with the `json` tag or no tag; a bare JSON object in the text carrying both a name and an arguments object; and a line of the form `name({...})` or `name: {...}`. A written name is read as a real tool by four rules in order — the same name, the same name in another case, the same name with the underscores and hyphens taken out, and a name within two edits when it is at least five characters — and a name equally close to two real tools is refused rather than guessed at. Arguments must be a JSON object; a string that itself holds one object is read once more; nothing where the arguments go is read as no arguments. Everything is bound: a reply is searched only to 128 KB, on a character boundary; a reply may ask for at most as many calls as `Caps.IdenticalCallWindow`, and the rest are dropped with a note; a name over 120 characters is refused; and the regular expressions are compiled once. After `MaxFailedParses` failed parses in a row, text that still looks like a call comes back as the answer with no problem, so that the loop of wave 3 stops fighting the model. The loop keeps that count; this package only applies the rule.

The designs are borrowed and named at the top of each file: the envelope shapes and their order of trust from ZeroClaw's parser, the stripping of the thinking before anything else reads the reply and the dropping of a block that never closes from ZeroClaw's parser and its channel orchestrator, the repair of a near-miss name and the cap on a name's length from OpenClaw's grammar, and the rule that a bad call becomes an error the model can fix from OpenCode's invalid-call path.
## The model providers (built, wave 1)

`internal/provider` turns one `contract.Request` into a streamed `contract.Reply`, three ways, on the standard library alone. `provider.New` takes one `contract.ModelAlias` from the configuration and an `Options` value carrying the clock, the resolved key, the home folder, and a one-line log, and returns a `contract.Model`.

- **The Anthropic provider** posts to `{base}/v1/messages` with `x-api-key` and `anthropic-version: 2023-06-01`, always streaming. The system prompt goes as ordered text blocks, and a block that ends a cache boundary carries `cache_control`. Boundary B is expressed on the last tool instead, because the Messages API reads the tools before the system prompt; at most four markers are ever sent. No sampling field and no thinking field is ever sent. The stream reader answers `message_start`, `content_block_start`, `content_block_delta` (text and partial tool-call JSON accumulated per block index), `content_block_stop`, `message_delta`, `message_stop`, `ping`, and `error`.
- **The OpenAI-compatible provider** posts to `{base}/chat/completions` with a bearer token when there is one, `stream: true`, and `stream_options.include_usage`. The system blocks are joined with blank lines into one system message, because this wire has no form for a cache boundary and both OpenAI and llama-server reuse a prefix on their own. Tool results go back as `tool` role messages. The output cap is `max_completion_tokens`. Tool-call arguments are accumulated by their index, and the final chunk carries the usage with no choices at all.
- **The command-line provider** runs `claude -p` or `codex exec` once per call on the user's own subscription, which is how the two cloud models are reached on a machine with no API keys. It renders the system blocks and the tools into text, writes every tool call in the one text form from `contract`, feeds the conversation on standard input, and runs the program in a folder of its own under `~/.coeus/run/cli/` holding nothing but that call's system prompt, so that no CLAUDE.md, no project settings, and no hooks can reach the prompt. Neither program is given a word of the conversation on its command line: `claude` is handed the path of its system prompt on `--system-prompt-file` and streams, and `codex` is handed its own through the `model_instructions_file` setting and returns one delta. Both files are written mode 0600 and go when the folder is removed at the end of the call. A command line is public — every process on the machine can read another's — and the kernel refuses a single argument over 128 KiB, which a working context passes easily, so the file is the only way to pass a real system prompt. The reply carries no structured tool calls: `internal/repair` reads them back out of the text. A hung run is killed by its exact process group identifier, never by anything matching a name.

The cap on one reply is `Request.MaxOutputTokens`, or `Caps.OutputTokensPerCall` from the configuration's defaults when the request names none, and a cap that is not a positive number is refused by name before anything is sent, because on the wire it comes back as a bare four hundred that is neither of the two sentinels.

Every reply names the model that answered in `Reply.Model`, and carries what the call cost in `Usage.CostUSD` when the provider reports money at all, which today is only the `claude` program. `Usage.InputTokens` is everything the model read: on the Messages API that is the plain input tokens plus what was written into the cache plus what was read back out of it, and `Usage.CachedInputTokens` is that last part on its own, so the cached count is always inside the input count. The Chat Completions API already reports the whole prompt in one field, so nothing is added there.

Around any of them, `provider.WithRetries` tries three times in all, waiting one second and then two with up to a quarter added, honouring `Retry-After` in both its forms, and never retrying a request that was simply too long. `provider.NewChain` tries the models in order and moves on when one is out of tries, setting `Reply.Model` to the alias that actually answered, but hands `contract.ErrContextOverflow` straight back, because only the context builder can fix a prompt that does not fit. Hermes' two local-server rules are kept: a base address on this machine is probed once at `/props`, and a server that answers the way llama-server does is sent `chat_template_kwargs: {"enable_thinking": false}` and has a smaller reported window preferred over the configured one.

Both of those make more than one attempt at the same request, so both stream through a delta gate (`deltas.go`): an attempt's pieces of text are held until that attempt succeeds, and only then handed to the caller. Without it a stream that broke after saying "Hello " and a retry that said "Hello world" left the caller with "Hello Hello world" while the reply said "Hello world". The cost is that the words reach the caller when the attempt finishes rather than as they are written, which is the price of never showing anyone text that was thrown away; the day a screen wants both, the reset a caller would need belongs in `contract`, not here.

Every wait in the package is measured on `contract.Clock`, including the sixty-second watch that gives up on a stream which has gone quiet, so no test ever waits on the real clock. `provider.New` refuses an `Options` with no clock in it; the running program passes `clock.System()` and the tests pass the fake.

Two things in this package went in out of order, and the history is not being rewritten: `props.go`, the local-server probe, was committed before its tests, and `parts_test.go` was written after the code it covers rather than before it.

## Test architecture

Four kinds of tests, described in `docs/WORK_PLAN.md`: unit, integration, functional, fuzz. Two tiers of model: the scripted fake on every commit, and three real models (the local Qwen 3.8 through the llama-server daemon on the development machine, Opus 4.8 through the Claude Code program on the user's subscription, and GPT-5.5 through the Codex program on the user's subscription, both driven by the `cli` provider) at every wave gate under the `live` tag. Every fake has a contract test against the real thing. The forty-step fixture is the proof of the record and the context builder.

## Repository map

`REPO_MAP.md` is generated from the tree by `make repo-map`. Inside a git work tree the list is what git tracks, which is exactly what a fresh clone holds, so a file must be staged before the map will list it; outside one the generator walks the tree. Which of the two it does comes from whether the folder holds a `.git` entry, not from whether git happened to work, because a git that fails inside a work tree would otherwise be papered over with a walk that lists every untracked file. The git child runs with every `GIT_` setting taken out of its environment, because `GIT_DIR` outranks the folder it was given; it runs under a timeout and its output is read through a cap; and the sorted list has its repeats taken out, because git prints one line per stage of a file left in conflict. The generator omits dependency folders, build outputs, test artifacts, and version-control internals. A drift test in `make check` fails when the map is stale.

## The gate

`make check` is `scripts/gofmt.sh`, `go vet`, `staticcheck`, the style checker, the repository-map drift test, `scripts/coverage.sh`, and `make test`. The three scripts are driven by tests of their own in `scripts/gate`, which build a small module in a temporary folder and run the real script against it, because a shell script nothing tests is a gate nothing guards.

The format check reads the file list from `git ls-files` rather than from `find`, so it covers what this branch tracks and nothing in another worker's worktree, quotes every path, and checks gofmt's exit status as well as its output. The coverage gate is driven by `go list`, so a package the report never mentions is a failure rather than a silence, and a package with no test file is named for what it is. The fuzz script lists with the integration tag on and fails on a package whose test binary will not build, rather than skipping it. `internal/lint` reports a file it cannot parse as a violation naming the file, resolves the callee's package through the file's own imports so an alias or a dot import cannot hide an error message, counts an error message's words with the format verbs taken out, reads a field's trailing comment as well as the one above it, and allows the conventional one-letter names only in a test file, in a helper that takes a testing handle, and in a request handler.

## Wave log

Each wave gate adds a section here: what was built, what changed in the interfaces, what the next wave depends on. Three paragraphs at most.

### Wave 0 (planned)

The development machine readied, the skeleton, the contracts, the browser protocol document, the repo-map generator, and the first versions of the three living documents.
