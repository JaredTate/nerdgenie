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
| `internal/context` | Build the working context from the layers, sized to the model | 2, built |
| `internal/loop` | Run one turn: orient, call, guard, permit, run, update, repeat; the done-check and the after-action review | 3 |
| `internal/tool` | The tool registry and the eighteen built-in tools, one folder each | 2, built |
| `internal/permission` | Decide allow, ask, or deny for a tool call | 2, built |
| `internal/sandbox` | Run a command inside bwrap and Landlock | 2, built |
| `internal/channel` | The queue, the router, the event stream, and the local socket | 3, built |
| `internal/command` | The command registry, the core slash commands, and the work behind `coeus init`, `doctor`, `install`, and `uninstall` | 3, built |
| `internal/tui` | The terminal screen | 3, built |
| `internal/signal` | The signal-cli client, linking, pairing, and the Signal channel | 3, built |
| `internal/vault` | The encrypted secret store, the resolver, TOTP, the sudo password, redaction | 2, built |
| `internal/reliability` | Leases, ledgers, sentinels, the breaker, the watchdog feed, backups | 4, built |
| `internal/memory` | The memory files, the search index, the hint, and zero-token capture | 4, built |
| `internal/skill` | The skill folder format, loading, learning, replay | 4 |
| `internal/browser` | The Go side of the browser: worker lifecycle, login, handoff | 5 |
| `internal/job` | Job records, the task list, and the scheduler | 4 |
| `internal/desktop` | The Go side of the desktop worker | 6, built |
| `internal/update` | Update, rollback, migrations | 6 |
| `internal/replay` | Re-run any logged task as a test | 6 |
| `cmd/coeus` | The binary; one file per subcommand; `main.go` and `serve.go` are the orchestrator's | 0 skeleton, built |
| `worker/browser` | The TypeScript browser worker | 5, built |
| `worker/desktop` | The TypeScript desktop worker | 6, built |

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

## The working context (built, wave 2)

`internal/context` turns one turn's state into one `contract.Request`. It depends on `internal/contract`, `internal/record`, and the standard library; the live test alone reaches for `internal/provider` and `internal/clock`. The package name is `context`, and the files inside it import the standard library's `context` under that same name, which Go allows because a package's own name is not in scope in its own files; every other package imports this one under an alias, as `coeuscontext`.

`context.New` takes an `Options` carrying the home folder, the memory caps, the output cap, and, for a test, the boundary identifier; the running program leaves that last one empty and gets a random one made once for the task. `Build` takes a `BuildInput` — the model's context length, the record, the job summary when the task belongs to a job, the pinned evidence, the recent messages and tool results, the tool specifications, and up to three lines of memory hint — and returns the request. The loop of wave 3 holds a `ContextBuilder` interface of exactly that one method and `serve.go` hands it one of these.

**The layers.** Above the cache line, in the design's order and each its own `contract.SystemBlock`: the instruction text of design section 5, which is a constant here copied from the design word for word and checked against it on every run; the three persona files, read from disk on every build so that a file the user edited by hand takes effect on the next turn, each cut at its limit with a note naming the file to shorten; then cache boundary A. A one-line block stands for the tool list and carries boundary B, because the provider renders the tools themselves and the Messages API reads them before the system prompt. Then the job summary, when there is one, and the record's goal and rules, ending boundary C. A layer with nothing in it is left out rather than sent as an empty block, which the wire refuses.

Below the cache line, in the messages, because anything put there leaves everything above it unmoved: the record's header, work, and lessons; the pinned evidence; the recent messages and results; and the memory hint last. The record is cut in two on the `## Work` heading its own printer always writes, and the header goes with the live half, because the budget line and the cost line are rewritten every turn.

**The one rule.** The window is the model's context length, less the output cap, less everything above the cache line, less the record's live part. The pins and the hint come out of what is left, and the rest is filled with the most recent messages, in full, newest first. When the window is full the oldest message's whole text leaves; its one line in the record and its copy in the event log stay, so `read r7` brings it back. Nothing is summarized and nothing is cut short. A message answering a tool call that has left goes with it, because a result with no call is refused on the wire. Pins never leave: a set of pins too large for the window is `ErrPinnedEvidenceTooLarge` naming the largest one to drop, and a model too small to hold even the rules and the record is `ErrNoRoomForTheRecord`.

**Two numbers and a marker.** The token estimate is three characters to a token plus a fixed cost for what each wire protocol wraps a message and a tool part in. It is not a tokenizer, and it was measured rather than guessed: the fixture's prompt at round forty is 19,961 characters over 82 messages, the local Qwen reported reading 8,013 tokens, and the estimate comes to 8,439, five per cent high. High is the safe side, since a low estimate builds a prompt the model refuses. Every tool result placed in the messages is wrapped in the data marker of rule 8 of design section 3, with a random sixteen-character boundary made once per task, so that nothing the agent reads can close the marker and have the words after it read as instructions. `context.WriteCostLine` writes the provider's own counts into the record header through `internal/record`; the estimate sizes the window before a call and never appears in the cost line.

**What the tests prove.** Two golden files hold the whole prompt for a 24k model and a 200k model at round twenty of the forty-step fixture: they share every byte above the cache line and differ only by a run of the oldest messages. Nothing above the cache line changes across rounds two to eleven, and the test then proves it is not vacuous by showing the user's correction at round twelve does change it. At a 3k window most of the fixture's results have left and every one of them still reads back by its id, while the user's correction is still in front of the model because it lives in the record's rules. The integration test runs the whole fixture against a real SQLite log and shows the prompt at round forty is about the size it was at round ten. The functional test plays all forty rounds against the scripted model, which refuses any request that has lost the correction or the user's reply to the stop. Under `live`, the same prompt goes to all three real models and the usage is printed for `docs/PROGRESS.md`.

**Two gaps to report.** `contract.MemoryCaps` has a limit for `MEMORY.md` and one for `USER.md` and none for `SOUL.md`, although the design says all three persona files have one; `context.SoulBytes` stands in at four thousand bytes until the contract carries it. And the instruction text of design section 5 is 550 words where the design's own sentence claims it is under five hundred; the constant is the design's text unchanged and `MaxInstructionWords` holds the line at what it actually is, so that nothing can be added without a decision.

## The permission function (built, wave 2)

`internal/permission` implements `contract.Permission`. It depends on `internal/contract` and the Go standard library and nothing else.

Every call is first **reduced to a readable form**, by `permission.Reduce`. A shell command is cut down to the program and the words that matter and nothing that varies, so `git commit -m "..."` becomes `git commit` and `rm -rf /tmp/x` becomes `rm -rf`. A quote-aware splitter first cuts the line into the separate commands a shell would run, at the three shell operators and at the openers and closers of a command substitution, so a command hidden behind a pipe or inside `$(...)` is still read as a command; the parts are joined back with a pipe. A shape table, which is OpenCode's arity table written fresh for the programs Coeus rules on, says how many words define each program and whether its flags belong in the readable form, which is what keeps `git reset --hard` apart from `git reset`. Environment assignments and `env` are dropped, and a `sudo` prefix and its own flags are handled so that the program sudo will run is the one that gets reduced. A program is named by its base name wherever the reducer matches one, so `/usr/bin/sudo -u root ...` is sudo and `/usr/bin/git commit` is a git commit. A shell handed a script is not a program with an argument but a command line in its own right, so `sh -c 'rm -rf ~'` and `bash -lc 'rm -rf ~'` have their script reduced recursively, three nests deep. Every other tool reduces to its name and the one field that says what it will do, which is the path, the address, the search words, or the intent; and a `write` or `edit` that would empty a file over four thousand bytes says so in the readable form itself, so that one mechanism, pattern matching, covers that case too. The readable form always fits on one line and is capped at two hundred runes, because it is matched on, logged, and shown.

A readable form is either **the whole story or it says so**. Every bound the reducer keeps used to drop text quietly, and the dropped text is where a delete hides: the four caps in the word reader, on the length of the line, the number of separate commands, the number of words in one command, and the length of one word, and the cap on the readable form itself. Each of them now ends the form with `(cut short before the end)`. A command that works out part of what it will run while it runs, which is a `$(...)`, `<(...)` or `>(...)` substitution or a pair of backticks anywhere outside single quotes, ends the form with `(builds part of itself at run time)`, because such a command is not written down anywhere the reducer can read. A call whose form carries either note is **put to the user rather than run** when no rule covers it, and the reason says which of the two it was. A rule the user wrote still wins, so an explicit allow or deny is obeyed as before, and an emptied ask-me-first list still lets an ordinary call run on its own; what changed is that a call the harness cannot read is no longer counted as a call the harness has cleared.

The **rulebook** is a list of rules of tool, pattern, and action, compiled once into matchers that ignore capital letters; the last rule that matches wins, and a call no rule matches is allowed, because the agent runs on its own by default. The tool of a rule is a glob too, so `browser_*` is one rule. `permission.New` takes a `contract.Config` and builds the book from it: the shipped entries the user kept in `ask_me_first` first, then the user's own rules from `permission_rules`, so a rule the user wrote can change what an entry would have said. An entry named in `ask_me_first` that nobody shipped is a configuration error naming it, so a typo in config.toml is reported rather than silently dropping a safety rule. A rule written in a file carries no reason of its own, so when it wins it is described by itself: the pattern, the tool, and the action. The **three shipped entries** are data in one file: deleting many files at once is eleven patterns over the shell, write, edit, browser and desktop tools; running a command with sudo is two patterns, which catch both the word and the shell tool's `escalate` field, because the reducer writes `sudo` into the readable form when that field is set; and spending money is the six words buy, pay, purchase, checkout, subscribe and order paired with the four tools that can spend it. An empty ask-me-first list stands for no rules, and then everything the user's own rules do not cover simply runs, except a call whose readable form is not the whole story, which is still put to the user.

A ruling of ask carries a **preview** of exactly what is about to happen: the whole shell command, and the reason the model gave when it asked for administrator powers; the file and how many bytes it holds now; the intent and the element on the page. The user's answer is remembered **in memory for the session only**: once records nothing, always allows every later call with the same readable form, and reject refuses them with the words the user gave. A remembered answer is read on every call, not only on the calls a rule would have asked about, so a reject sticks to a call nothing on the list covers; the one thing that outranks it is a rule that refuses, which is read first because nothing outranks that. A **standing approval** registered by a skill names the skill, the readable form it covers, a limit on uses, and an expiry read against `contract.Clock`; it is checked after the answers the user gave, so a rejection still wins. When a run is unattended, a call that would have asked comes back ruled **`contract.RulingStop`** instead, carrying the same preview, so the loop stops the task and reports what it needed rather than waiting for somebody who is not there.

The **tests hold the shape of the ask-me-first list rather than the shape of the reducer**. `internal/permission/evasion_test.go` keeps the five probes the wave 1 gate review used, each written the way the reviewer ran it, and `FuzzADisguisedCommandStillNeedsTheSameYes` holds the property that catches a whole class of them at once: a command the shipped list needs a yes for still needs one when it is wrapped in `sh -c`, put behind a pipe, put in a subshell, or given a long prefix. Wrapping a command may never make it easier to run than the command on its own.

The tool input field names this package reduces by are the ones fixed in `docs/briefs/wave-2/2.5-tools.md`: `command`, `escalate` and `reason` for the shell; `path` and `content` for a write; `path`, `old` and `new` for an edit; `pattern` for a search; `url` and `query` for the web; and `intent`, `element` and `text` for the browser and desktop tools. They live in one table in `reduce.go`, and a name that is not there falls back to the tool's bare name rather than failing.

## The tools (built, wave 2, brief 2.5)

`internal/tool` is the tool registry, and under it there is one folder per tool: `read`, `write`, `edit`, `search`, `shell`, `web`, `memory`, `task`, `skill`, `job`, `browseropen`, `browserread`, `browserclick`, `browsertype`, `browseract`, `browserlogin`, `browserhandoff`, and `computer`. Every one of them implements `contract.Tool` and codes against `internal/contract` alone, except the three noted below; none of them imports a wave-2 neighbour.

**The registry.** `tool.New` takes one `tool.Settings`, builds the eighteen built-in tools in the order design section 7 lists them, and then adds every executable in `~/.coeus/tools/` through the user-tool protocol in `contract`: run it with `--describe` under a ten-second bound in its own process group, read a `contract.ToolSpec` back as JSON, and register a tool that runs the same program again with the model's arguments on its standard input under the tool timeout. A user tool that cannot be asked, or whose answer will not parse, or whose description is over the forty-word cap, is skipped with one logged line naming the file and the problem, because one bad script in a folder must not stop the agent from starting. Registration refuses a tool with no name, no description, a description over the cap, an unknown permission class, a name already taken, or one tool past `tool.MaxTools`. `Specs` is what goes into the prompt and `Lookup` is what the loop calls; `Lookup` hands back the tool wrapped so that the output cap is already applied, so nothing in the loop has to remember to apply it.

**Nothing is interpreted.** The registry never reads the text a tool returns. Text past `Caps.ToolOutputBytes` is written whole to `~/.coeus/run/spill/<task>-<n>.txt`, named by the task and by a number one above the highest that task already has there, and the result the model sees is cut on a character boundary with one line naming that file, which the read tool opens. One result is capped at `tool.MaxResultBytes` before anything is written, and the spill folder is capped at `tool.MaxSpillFolderBytes` with its oldest files removed to make room.

**A field left empty in the settings does not hide a tool.** The design shows every model all eighteen names on every call, so a tool whose dependency is missing is still registered and still described; it refuses when it is called, with a line saying what is missing. The one exception in spirit is the shell tool, which turns itself off with the sandbox's own reason when `Available` fails.

**Three rules live in one place and are handed to the tools that need them.** `tool.NewPathCheck` builds the check the four file tools are given: a path is allowed only when it is a whole path that sits inside one of `Config.SandboxRoots` once its links are followed, and outside everything `contract.ExcludedFromSandbox` names, and every refusal names the roots. `browserread.PageText` and `browserread.ChangeText` are what a page and a change look like to the model, and the other six browser tools describe the page they left behind by calling them, so a page reads the same however the model arrived at it. `write.Change` writes the file-change event, and the edit tool records its change through the same door, so there is one shape of `contract.FileChangeBody` and one place that writes it: what the file held, the mode it had, and whether it was there at all go into the log **before** a byte is written, so a change that was never recorded is never made.

**The tools worth a line each.** `read` reads a file with a line number in front of every line, a folder as a sorted listing, or a past result by its label, which is `r7` through the task's record and `j4.2` through the job's, both injected as `read.Stored`; a file that is not text is refused by name. `edit` tries the exact text and then four fallbacks that each have a job the others do not — lines that match once trailing spaces are off, one line that matches once every run of spaces is one space, a block that matches once the shared indentation is off, and the text with its ends trimmed as a substring — and every candidate span must be in the file exactly once or it is refused with the count. `search` reads one pattern as a regular expression when it is one and as a name pattern such as `*.md` when it is not, matches both the names of files and the lines inside them, runs through ripgrep when it is on the machine and through its own walk when it is not, and is capped at fifty rows; every test in that package runs both ways so the two cannot drift. `shell` runs every ordinary command through `contract.Sandbox`, waits `shell.YieldAfter`, ten seconds counted on `contract.Clock`, and then hands back an id such as `p1` from a bounded table that answers `poll`, `tail`, and `kill`; `escalate` with a written reason puts the whole command to `contract.Permission`, and only an allowed one runs outside the fence through `sudo -A`, with `SUDO_ASKPASS` pointing at a little program the tool writes under the run folder that runs the coeus binary's `askpass` subcommand, because sudo takes one program there and `coeus askpass` is two words. `web` resolves a name once and connects to that exact number, refuses every private, loopback, link-local and cloud-credential address and any redirect that leads to one unless the settings name that host, caps the page, turns it into text keeping the headings, links and lists, and wraps everything from outside in a boundary with a random id and a sentence saying the text is data; search goes to the configured SearXNG address as JSON, or, with none configured, reads the DuckDuckGo results page, unwrapping the redirect that page wraps its links in. `task` is the model's only door into the record: seven operations, each turned into one `record.Update` and put through the record's own rules, with the rule handed back word for word when a change is refused, and no door at all to the ask, the corrections, the header, the situation, or a result. `browserlogin` never sees a credential in its own right: the harness supplies one through `browserlogin.Credentials` and a code through `browserlogin.TwoFactorCode`, both wired to the vault in wave 5, and the tests refuse any result that carries one. `browserhandoff` puts the reason to the user through `browserhandoff.AskUser`, which wave 3's channel fills in, and hands their reply back.

**The three imports outside `contract`.** `internal/tool/read` and `internal/tool/task` import `internal/record`, for a past result and for `record.Update`. `internal/tool/edit` imports `internal/tool/write` for the file-change door, and the browser tools import `internal/tool/browserread` for the words a page reads in. Inside `internal/tool`, the order is: `browserread`, then the other browser tools; `write`, then `edit`; then the registry itself, which imports all eighteen.

**What the tests prove.** A golden file per tool holds the exact text the model reads back, and one golden file holds all eighteen descriptions with their word counts. Fuzz tests cover the edit matcher, the shell call reader, the turn from a web page into text, and the address check. The shell tool's `integration` test drives it against a sandbox that really starts a process, in its own process group, with the fake clock advanced past the yield, because `internal/sandbox` is a wave-2 neighbour this package may not import. No test in the web package reaches the real internet: the fake search server, its fixture pages, and local servers a test starts itself are the only addresses used, and one test proves that a loopback address the settings do not name is refused.

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

## The terminal screen (built, wave 3; reworked, brief 3.6)

`internal/tui` is the terminal screen, and `cmd/coeus/tui.go` is the `tui`
subcommand that opens it, which is also what the bare `coeus` command runs. The
screen is a thin client: it holds only what is on the frame, it draws what the
running program sends over the local socket, and it sends back what the person
types. It is built on Bubble Tea, with lipgloss for the border glyphs and the
colour values, and it imports nothing of Coeus but `internal/contract`,
`internal/clock`, `internal/config` (in the subcommand, to find the home folder),
and `internal/testkit` in its tests.

The drawing is `docs/TUI_DESIGN.md`, line for line: a header, a rule, the
transcript with its four block kinds, a rule, the input box, and the status
strip. `View` builds the frame as rows of plain text and adds the colour codes at
the very last step, which is why `NO_COLOR` renders exactly the same structure
and why a golden file is readable. Nothing is drawn wider than the terminal: the
bubbles wrap, and the header and the status strip drop their right-hand piece
when it will not fit with a gap in front of it. The transcript is drawn from the
newest block backwards and stops as soon as it has the rows that fit, so a long
session costs no more to draw than a short one. `Run` asks the terminal how big
it is with the `TIOCGWINSZ` request before Bubble Tea paints anything, and falls
back to eighty by twenty-four, so the very first frame is the right size.

Every piece of text that goes onto a row has its control characters turned
into blanks first, so nothing the person types and nothing the program sends
can move the cursor, repaint the frame, or make a row wider than it measures;
the only escape codes in a frame are the screen's own colours and the picture
protocols on a screenshot.

**The look.** `style.go` holds the DigiByte palette as lipgloss colour values —
a light blue ground behind every row, white letters, a pale blue for the quiet
parts, DigiByte's own blue for the filled shapes, gold for the card that must be
answered, red for a failure — and turns each of them into escape codes at
whatever colour depth the terminal reports, stepping from twenty-four bit through
the two hundred and fifty-six colour cube to the sixteen ordinary colours and
down to none. The codes are written here rather than by `lipgloss.Style.Render`
because a lipgloss renderer reports no colour at all when its writer is not a
terminal, which every test process is. `View` paints every row out to the
right-hand edge so the ground has no gaps. `banner.go` draws the `COEUS AGENT`
wordmark in a five-row block font while the transcript is empty; `bubble.go`
draws the person's filled bubble leaning right, the agent's outlined bubble
leaning left, and a tool call as a small filled pill.

Time comes from `contract.Clock` and reaches the screen as one heartbeat every
thirty milliseconds. That heartbeat does three things: it moves the screen's idea
of the time on, it flushes the deltas that have arrived since the last one into
the reply block, and it lets the spinner decide whether it is still due. The
spinner appears only after five hundred milliseconds of waiting and stays at
least three seconds once it has appeared. Those numbers, the hundred-column wrap,
the five-row input box and the ten-second health window are pinned as literals in
a test rather than measured against themselves.

`Client` is the link. It dials through a `Dialer`, sends `attach`, reads
envelopes into Bubble Tea messages, and dials again on a wait that doubles from a
quarter of a second to ten seconds, saying so in the status strip the whole time.
`UnixDialer` is the real one, on `contract.Home.SocketFile`. A socket line past
one megabyte is thrown away and shown as an error card, and so is a line that is
not a message the two sides agree on; neither closes the link. A link that goes
away leaves the model, the task and the cost in the header and changes only the
word for the link itself.

**What the screen reads on the socket.** Every name on the wire comes from
`internal/contract`, so that the screen and `internal/channel` cannot disagree
about a spelling. A masked prompt is an `ask` with `MaskInput` set, the title of
the request in `Title`; there is no message type for asking a secret, because
`secret` travels only from the screen to the program. A `status` message fills the
header, the status strip, the tool lines, the health dot, the budget bar and the
command palette from the fields `contract.StatusFieldModel`, `StatusFieldTask`,
`StatusFieldTaskState`, `StatusFieldTokensIn`, `StatusFieldTokensOut`,
`StatusFieldCost`, `StatusFieldBudget`, `StatusFieldState`, `StatusFieldTool`,
`StatusFieldToolLine`, `StatusFieldHealthy`, and `StatusFieldCommands`, the last
holding one command per line with `contract.StatusCommandSeparator` between its
name and its help. The state field carries one of `contract.StateIdle`,
`StateThinking`, `StateUsingTool`, `StateWaitingForYou`, and `StatePaused`; the
words the strip draws are the design's, so `StateUsingTool` reads as "using read"
and `StateWaitingForYou` as "waiting for you". A field or a state word the screen
does not know changes nothing on the frame, a field that is not sent leaves what
is on the screen alone, and a status message with no `StatusFieldHealthy` counts
as the program answering for itself. The budget line is read for its first
number, and the fullest report seen since this task started is what the bar in
the status strip is measured against. A `reply` that carries `Attachments` names
each file as a pill.

Going the other way, an approve carrying `contract.ApproveAlwaysText` means every
call like this one for the rest of the session and an approve carrying no text
means this one call; a deny carries the person's reason in `Reason`; and
`contract.SocketCancel` with an id is what Escape at a card or at a masked prompt
sends, because the person refused nothing and the program must stop waiting at
once rather than sitting out its whole deadline. The card that holds the single
keys is remembered by the number it was given when it was shown, so that an error
card arriving above a preview cannot take the preview's three answers with it.

## The commands and the four subcommands (built, wave 3, brief 3.3)

`internal/command` holds the one table of slash commands and the work behind the four things a person types before there is anything to talk to. It depends on `contract`, `config`, `clock`, `vault`, and the standard library, and on nothing from its own wave: what it needs from the loop, the channels, or the jobs reaches it through a struct of functions that `cmd/coeus/serve.go` fills in.

**The registry.** `command.NewRegistry` holds every `contract.Command` in the order it was registered, which is the order `/help` prints. `Register` refuses a command with no name, a name holding a slash or a space, no help line, no function, or a name already taken, because a second command with the same name would silently hide the first. `Run` reads a typed line apart with `command.SplitLine`, refuses a terminal-only command on any other channel with one plain line rather than an error, and hands everything else to the command itself; a line naming nothing the registry holds comes back as `command.ErrNoSuchCommand`, so the router can send it to the model instead. `SplitLine` takes off every leading space and slash, not just the first, and `Lookup` reads a name through the same splitter, so the two can never disagree about which command a piece of text names and `//status` is not the status command. A fuzz test throws any text at the splitter and asserts that the name never holds a space or a slash, that both halves are really in the line, and that splitting the answer again gives the same answer.

**The ten core commands.** `command.New(registry, deps)` returns a value whose `All` gives the ten in help order: `/help`, `/status`, `/model`, `/new`, `/sessions`, `/approve`, `/deny`, `/pause`, `/resume`, and `/undo`. `/status` prints the model in use, what the session has cost, the jobs with their progress, what is waiting for an answer, and the health of each channel, and says plainly what it could not reach rather than failing. `/model` shows the alias in use with the aliases `config.toml` names and sets one of them, refusing a name the file does not define. `/pause` pauses every running job through `contract.Job` and writes down which ones it stopped, so that `/resume` starts exactly those again and never a job the user paused for a reason of their own. `/undo` walks the event log with `Replay`, keeps only the file-change events since the newest message, which is the last turn, and puts them back newest first, so a file written twice in one turn ends holding what it held before the turn began and a file the turn made is removed rather than restored; it refuses when there is no turn, says so when the turn changed no files, and refuses a turn that changed more than `command.MaxUndoFiles` files rather than undoing part of it.

**`coeus init`.** `command.Init` makes the whole home layout with the modes from `contract`, writes the three persona files with one explaining line each unless the user already has them, and asks at most four questions with a two-minute wait on each: the folders Coeus may work in, the model, the API key when the model needs one, and Signal when signal-cli is installed. Every answer is also a flag (`--model`, `--work-folder`, `--api-key-from-env`, `--signal`, `--yes`, `--reset-config`), so a container can run it with no keyboard, and the flags are read before anything on disk is touched. Each folder answered is checked with `contract.CheckSandboxRoot`, the same rule the configuration checks it against, so the whole home directory is refused here with the reason; the default is the work folder `contract.DefaultSandboxRoots` names, which init creates. The model menu is built from what it finds, in one order: the llama-server daemon when its health check answers, an LM Studio server when `/v1/models` answers, from which it reads the loaded model's name, the Claude subscription when `claude` is on the PATH, the ChatGPT subscription when `codex` is on the PATH, and then Anthropic and OpenAI, which need a key and so cannot be found by looking. A key is read from the environment variable `--api-key-from-env` names or through the vault's masked prompt, never from a command line, and goes into the vault as `secret://anthropic` or `secret://openai`; `config.toml` holds only the reference. The file is written from a template this package owns, with a comment above every line, the chosen model as `default_model`, every other model that was found as the fallback chain, and a `[[models]]` block for each. Then the doctor's report is printed and the commands a new user needs, with `coeus signal link` among them only when signal-cli is there and Signal was not switched off. A home folder that already has a `config.toml` is left exactly as it is, byte for byte, and told how to write it again with `--reset-config`.

**`coeus doctor`.** `command.Doctor` prints `config.Doctor`'s report and returns false only for a problem, never for a warning: a warning means something Coeus can work without is switched off, and a machine like that is set up correctly.

**`coeus install` and `coeus uninstall`.** `command.UnitText` renders the systemd user unit for one home folder, and it is a golden file. It is `Type=notify` with `WatchdogSec=60`, `Restart=on-failure`, `RestartSec=5`, `SuccessExitStatus=75` with `RestartForceExitStatus=75` for the restart-me code, and `RestartPreventExitStatus=78` for a bad configuration, and its `ExecStart` points at `releases/current`, a link that never moves, so the updater can switch versions without rewriting the unit. `Service.Install` writes it to `~/.config/systemd/user/coeus.service`, makes that link point at the running binary when there is no installed release yet and leaves a link that is already there alone, and then runs `systemctl --user daemon-reload`, `enable`, and `start`, each under a thirty-second deadline. `command.Units` is the whole list of what is written, and there are three: the service, and `coeus-backup.service` and `coeus-backup.timer`, which run `coeus backup` at three in the morning with a fifteen-minute spread and `Persistent=true`, so a machine that was asleep backs up when it comes back. All three are golden files. The timer is enabled and started with the service; the backup service is not, because the timer is what starts it. `Service.Uninstall` stops and disables the service and the timer, removes all three units, reloads, and keeps the home folder; `--purge` removes it only after the word `delete` has been typed on a line of its own and compared exactly.

**The borrowed designs**, named at the top of each file with their paths: the one table of commands from OpenCode's registry, the question shapes from Hermes' setup program, the order of the model menu from ZeroClaw's quickstart, the unit and the way it is installed from OpenClaw's unit and ZeroClaw's service install, the exit codes from Hermes' restart module, and the order of the checks from OpenClaw's boot check.

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

## The desktop: `worker/desktop` and `internal/desktop` (built, wave 6, brief 6.1)

The desktop is the last resort, and design section 10 says so: if the browser can do the job, the browser does it. It is two halves written against one document, `worker/desktop/PROTOCOL.md`, which is JSON-RPC 2.0 over standard input and output, one object per line, in the same shape as the browser's.

**The protocol.** Nine methods: `launch`, `screenshot`, `click`, `type`, `press`, `drag`, `clipboardGet`, `clipboardSet`, and `health`. A **mark** is one numbered control, `{number, role, name}`, which is `contract.DesktopMark` exactly. A **screenshot** is the granted window's picture as base64 text with its marks, the application, the title, and how many controls the cap left out. Every action returns a **diff**: what the title did, which controls are new, how many went away, the controls now, whether what the model expected actually happened, one sentence saying what happened instead, and whether the window settled. The error table is the browser's, with the desktop's own five: no such mark, an unreadable window, no application open, a driver that is not there, and a launch that failed; the first four codes decide whether the Go side restarts the worker or simply tells the model.

**The worker** is TypeScript on Node over `@trycua/cua-driver`, and it acts only inside the one application `launch` granted. It never photographs the whole desktop and never reads another window's accessibility tree, so no window the user opened can reach the model's context. It brings the granted window to the front before every action, because on this display server a key combination cannot be delivered to a window in the background. It moves at human pacing: text goes in short runs with small varying gaps, a click is followed by a human pause, a drag runs in steps, and there is a pause between actions; `--pacing fast` shortens every one of those waits, including the settle waits, and only the tests pass it. After each action it settles, which is the browser's rule with the page's parts swapped for the window's: the accessibility tree must hold still for three hundred milliseconds, with a limit of three seconds, after which the window is read as it stands and reported with `settled: false`. Then it compares the controls before and after and judges the expectation by the browser's fixed word rule. The pieces are one file each: the wire, the marks, the expectation rule, the key chords, the pacing, the session, the nine methods, the process, the bounds, and the one file that touches the driver, so that everything above it is tested against a driver nobody can see.

Two things about this machine are written into the worker rather than assumed. The desktop is Wayland with XWayland, and the driver reaches windows through X11 and the AT-SPI accessibility bus, so applications the worker starts are started with `GDK_BACKEND=x11` and `QT_QPA_PLATFORM=xcb`, which changes nothing about the windows the user opened. And a control's handle belongs to the reading it came from: a handle kept from an earlier reading is refused by the driver, so the worker looks a control up again by its number before every action.

**The Go side**, `internal/desktop`, implements `contract.Desktop`. It starts the worker as a child process on first use, keeps that one alive, and when it dies or answers with one of the three codes the protocol's table says to restart on, stops it by its **exact process identifier** and starts a new one on the next call, telling the model the desktop was interrupted; the application then has to be opened again. One request goes at a time, each with a deadline of its own, and a response line longer than eight megabytes is refused rather than held. The worker is handed an environment with the display in it and nothing secret, which is protocol rule seven.

Two rules of the design live here rather than in the worker. An application is **granted once per session** through a preview on `contract.Channel`, and nothing at all can be done until one is granted. Every action inside it that **cannot be undone**, which is typing into a field, a drag, and a paste, goes through `contract.Permission` as the tool `computer` with an `intent`, an `element`, and the `text`, and a ruling of ask becomes a preview of exactly what is about to happen. A click and a key press do not, because they can be undone.

`contract.Desktop` has no place for the expectation the model states, so this package also exports `LaunchExpecting`, `ClickExpecting`, `TypeExpecting`, `PressExpecting`, and `DragExpecting`, and the interface's own methods are those with an empty expectation. An expectation that was not met comes back as an **error** carrying what the worker saw instead, because the interface returns only an error and the model has to be told rather than left to guess.

**What the tests prove.** On the TypeScript side, the wire, the marks, the expectation rule, the key chords, the pacing, the session, the nine methods, and the process all have unit tests against a fake driver; property tests throw any expectation, any key combination, any accessibility tree, and any bytes on standard input at it and get a well-formed answer or nothing, never a crash; and a fixture-window suite drives a real `zenity` entry box on this machine's display, one test per action, including an expectation that is not met, and ends by clicking OK and reading back exactly what was typed. On the Go side, the client, the grant, the previews, the restart, and the contract check all run against a scripted worker on a pair of pipes; two fuzz targets throw any bytes at the reading of the worker's answers; and under `integration` the real worker drives the same fixture window end to end. `internal/desktop` needs no new Go dependency; the worker's six pinned TypeScript libraries are in `docs/DEPENDENCIES.md`.

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

## Reliability and backups (built, wave 4, brief 4.1)

`internal/reliability` holds the eight mechanisms that keep Coeus serving through a crash, a restart, a wedged turn, and a broken database, and the encrypted backup behind them. It depends on `contract`, the standard library, `filippo.io/age` for the archive, the same pure-Go SQLite driver `internal/log` opens the file with, and `github.com/coreos/go-systemd/v22/daemon` for the watchdog. It reads the event log only through `contract.Store` and the time only through `contract.Clock`, so every mechanism is tested by moving a fake clock rather than by waiting.

**The guard is the one thing `serve.go` wires.** `reliability.New(reliability.Settings{Home, Clock, Store, Caps, BackupFolder, Send})` returns a `*Guard`. `Send(ctx, channel, text)` is how a message reaches the user, with an empty channel meaning the one the user is normally talked to on. `Guard.Start(ctx)` does the whole startup sequence in order and returns a `Startup` saying what it found: whether the last exit was unclean, where a damaged database was moved to, which archive it was put back from, whether the breaker has tripped, and how many replies were sent again. `Guard.Stop()` is the clean exit. The loop uses `MayStartTask`, `AcquireTurn`, `TurnDeadline`, `ToolDeadline`, `Ledger`, and `Drain`; `serve.go` uses `Ready` when the readiness check first answers, `FeedWatchdog` for the life of the program, and `Backup` for the nightly timer.

**The crash-loop breaker.** `crash-loop.json` under the run folder counts the starts that followed an unclean exit. Three inside five minutes trip it; a tripped breaker keeps serving the user and starts no task, tells the user once, and clears itself after thirty minutes of quiet, so a machine nobody is watching heals. A state file that cannot be read leaves the breaker open and says why, because a broken breaker that wedges a healthy agent is worse than the loop it guards against.

**The turn lease.** One turn per session. A second turn on the same session waits five seconds, looking again every twenty-five milliseconds, and is then refused with `ErrTurnInProgress` rather than run beside the first, because two turns writing one record is the damage the lease exists to prevent. A release is checked against the exact lease that was handed out, so a late unwind cannot free a newer turn's, and the registry refuses past `MaxLeases`.

**The delivery ledger.** Every reply is written into the event log as a `contract.EventReply` before it is sent and marked delivered after, so the sequence number of the first event is the reply's identifier and the log is the ledger. `Undelivered` replays the log and returns what was written but never confirmed, capped at `MaxUndeliveredReplies`. `Resend` sends each of them again with `DuplicateMarker` in front, which says plainly that the user may have it already, and gives up after three attempts or a day with a line in the log saying so.

**The lifecycle sentinel and the database check.** `lifecycle.json` under the run folder exists only while the program is running, so finding it at startup means no exit path ran. That is the moment `CheckDatabase` runs SQLite's own `PRAGMA quick_check` before anything else opens the file; a database that fails is moved aside as `coeus.db.broken-<time>` with its write-ahead and shared-memory files, and the newest archive is put back, database only, so that a vault and a browser profile newer than the backup are left alone. A recovery with no backup to put back tells the user and comes up on an empty database rather than refusing to start.

**The drain marker.** `drain.json` under the run folder tells the loop to finish the task it has and take no new one, which is how the updater of wave 6 stops the agent without cutting a task in half. It carries the boot identifier and expires after thirty minutes, so a marker left behind by a machine that has since restarted, or by a writer that crashed, cannot park the agent forever; a marker that cannot be read still means stop.

**The deadline.** One type behind both limits: `TurnDeadline` is the fifteen minutes `contract.Caps` gives a turn and `ToolDeadline` the seven minutes it gives a tool call. `Watch` returns a context cancelled when the time is up, with `ErrDeadlineExpired` as its cause, so a caller can tell our limit from the provider's.

**The watchdog feed.** `Ready` sends `READY=1`, which is what a `Type=notify` unit waits for, and `Feed` sends `WATCHDOG=1` at half the interval systemd announced. The feed stops when the breaker has tripped: nothing inside the program is going to put a crash loop right, so systemd is left to start it again. Outside systemd there is no interval and no socket, and all of it does nothing, which is what lets the same code run from a terminal.

**Backups.** `coeus backup` writes one age-encrypted tar to the configured `backup_path` holding the database, the vault, and the browser profile, named `coeus-backup-<date>-<time>.tar.age`, and keeps the newest seven. The database goes in as a copy made by SQLite's own `VACUUM INTO`, because a plain read of a file the agent is writing catches it mid-write. The archive is locked with the vault's own key, so `~/.coeus/vault.key` is what opens it again and `coeus backup` says so every time. `coeus restore <archive>` puts the three back, refusing a home that already holds any of them unless `--force`, and taking `--key` for a restore onto a machine whose vault key lives elsewhere. The reader is bounded and refuses any name that is not one of the three things a backup holds, so nothing an archive says can write outside the home folder; a fuzz test throws arbitrary bytes at it, both raw and inside a properly locked archive, and asserts no panic, an error naming the archive, and nothing written outside the home.

**The borrowed designs**, named at the top of each file with their paths: the restart-loop guard, the per-session turn lease, the delivery ledger, the lifecycle ledger, the drain control, the deadline layer, and the database recovery, one Hermes file each under `~/Code/hermes-agent/`.

## Memory (built, wave 4)

`internal/memory` implements `contract.Memory`. It depends on `internal/contract`, the standard library, and the same pure-Go SQLite driver `internal/log` uses, and on nothing else; it reads the event log only through `contract.Store`, never by touching its table.

**Two files, a folder, and one index.** `MEMORY.md` holds facts about the world and `USER.md` holds facts about the user, both in the persona folder, both capped by `contract.MemoryCaps`. Every fact is one line: `- m7 [2026-09-02T14:00:00Z | task 17 | supersedes m3] the fact itself`, with the date in universal time to the second, the source, and the optional id of the fact this one replaces inside the brackets, and the fact itself after them. A line that is not a fact line, such as a heading somebody typed in by hand, is kept where it is at the top of the file and indexed as a note of its own, so nothing a person writes into these files is ever thrown away. Which file a fact goes in is decided in two steps: a fact that supersedes another goes wherever that one went, and otherwise an id beginning with the letter `u` means `USER.md` and anything else means `MEMORY.md`. A fact saved with no id is given the next free number in its family, `m7` or `u3`, and a fact saved with no date is dated from `contract.Clock`.

**The cap moves facts, it never refuses them.** This is where Coeus parts company with the Hermes design it borrows the size limit from: Hermes refuses a save that would pass the limit and asks the model to consolidate, which costs a model call and can fail. Coeus takes facts off the front of the file, which are the oldest ones in it, until what is left fits, and appends them to a dated note in the memory folder, `MEMORY-2026-09-02.md` or `USER-2026-09-02.md`, starting `-2` and onward when one reaches sixty-four kilobytes. A moved fact is still in the index, still comes back from a search, and is still readable by its id; a note read back by its id is cut to that same sixty-four kilobytes, counted in bytes. One save carries at most five hundred facts, because a save reads a row of the index for every fact in the batch before it writes anything. Every memory file the package writes is written through a temporary name and a rename, and is then recorded in the event log as a `contract.EventFileChange` carrying what the file held before, which is what an undo puts back.

**The index.** Four tables in the one SQLite file, created idempotently: `memory_facts`, `memory_state` for the counters and the last event read, `memory_indexed`, and `memory_search`, which is a full-text search table the pure-Go driver supports, ranked by its own relevance measure. `internal/log` owns the `events` and `schema_version` tables and this package touches neither; it does insist that the events table is already there, so that the file's identity is always decided by the log, and says so if it is not. The index holds three kinds of thing, all of which come back from a search as a `contract.Fact`: a fact, whose id is its own; a note, whose id is `note:` and its path inside the home folder; and a past message, whose id is `msg:` and its number in the event log. A search matches on every distinct word of the query, each quoted so that a word the search table would read as one of its own is read as a word, and orders by relevance first and by date second, so a query with no words in it returns the newest facts. Results are capped at fifty. A fact something later replaced comes back with `(superseded by m12)` at the **front** of its own text, because nothing is deleted, a reader has to be told, and a mark on the end is cut off with the rest when the line is shortened to fit.

**The hint is not the search.** The hint is `contract.MemoryHintLines` lines of at most a hundred and twenty runes each, and it is empty far more often than a search is, because it rides in the model's context on every turn whether it is wanted or not. It is built from the words of the step that are at least four runes long and are not on this package's list of stop words; a step that offers no such word gets no hint at all. Its query leaves out every fact something later replaced, so a withdrawn fact can never ride along. It reads the best twenty results and then scores them itself, on how many of the step's own words each line holds, and keeps only the lines holding at least two of them, or the only one the step had. The search table's own relevance measure cannot do this job: it scores a match against the size of the index rather than against the step, so in a small memory it scores everything the same and in any memory it ranks a match on "the" alongside a match on the subject.

**The indexer.** It runs when the memory is opened, on every save, and on each new message event the loop hands to `IndexEvent`. Every run is capped at two thousand pieces of work, and every read the run makes is capped with it. The run at opening reads the two memory files and the dated notes back off disk and puts into the index any fact that is written in a file but missing from the table, which is how a fact somebody typed in by hand, or one written just before a crash, is found; it walks the memory folder and indexes every markdown file in it, stopping the walk before it reads another file once the budget is gone; it forgets the notes whose files are gone, looking at no more of them than the budget has left and paying a piece of the budget for each one; and it reads the event log **from where the last run stopped, in spans of sequence numbers**, never by replaying it, because a replay hands out every event ever written and a run only wants the ones written since the run before it. A log longer than one read can hold is caught up a page at a time, at most a hundred pages to a run, over several runs.

**Zero-token capture.** `Capture(ctx, taskID)` reads the events of a finished task through `contract.Store.ByTask` and writes down, with no model call, the files it changed, the commands it ran, the sites it visited, the jobs it created, and every message from the user whose first word is "no", "actually", "always", "never", or "don't", kept word for word as a fact about the user with the task as its source. Each captured fact's id is built from the task and the number of the event it came from, so capturing the same task twice writes nothing new. A task with more events than one read of the log returns comes back cut short, and the capture reads the rest of it in pages with `ByRange`, stopping at the end of the log, at two hundred captured facts, or at a hundred pages; a read that really failed is a failure of the capture, never a task that did nothing. The two are told apart by the word `ByRange` in the error, which is the only thing `contract.Store` gives a caller to tell them apart with. It reads the event bodies tolerantly, by the field names `internal/permission` already fixes for the tools, because `internal/contract` defines a body type only for a file change; when the loop of wave 3 defines the message and tool-call bodies, they belong in `contract` and this package should read them.

**`/memory`.** `(*Memory).Command()` returns the `contract.Command` the orchestrator registers in `serve.go`. On its own it prints how full the two files are and the last ten facts; `search <words>` looks something up; `forget <id>` supersedes a fact with one saying the user withdrew it, which leaves the old fact searchable and marked rather than deleting it. The new fact names the withdrawn one by its id and never copies its words, because a copy would be a live, unmarked fact holding exactly what the user asked the agent to stop repeating.

**This package exports no constants.** Nothing outside it referenced any of the nine it used to export, and a bound nothing reads is a bound that can drift; the numbers are written out in `internal/memory` and pinned as literals by its own tests. For the record, most of this package was written code first and tests after, which is not how the rest of the repository was built and is why the gate review found six unpinned bounds in it.

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

**The fixture site** is in `test/fixtures/site/` and is what the browser flows run against. Four ordinary HTML pages: a login page with a username, a password, and a code field that accepts exactly one credential; a compose page that keeps what it is posted and shows it; a captcha page; and a form for the quality skill. The behaviour that makes them a site rather than four files is in `test/functional/fixturesite_test.go`, a little server on a loopback port the operating system picks, holding everything in memory and serving no file it has no route for. Every page is proved through plain HTTP first, so that a browser test which fails is never the site's fault. Under the `integration` tag the browser flows drive the real worker from `worker/browser` as a child process, JSON-RPC over its pipes one request at a time, building it first when the build is missing or older than its source, stopping it by closing its standard input and ending it only ever by the exact pid it was started with. Every answer is decoded into the `contract` shapes, so a worker whose JSON stopped fitting `internal/contract` fails there rather than in a later wave. The flows that need `internal/browser` — the vault-driven login, the post with a preview, the handoff at the captcha, the replayed skill, and the quality report — are named and skipped for one stated reason until brief 5.2 merges.

## Repository map

`REPO_MAP.md` is generated from the tree by `make repo-map`. Inside a git work tree the list is what git tracks, which is exactly what a fresh clone holds, so a file must be staged before the map will list it; outside one the generator walks the tree. Which of the two it does comes from whether the folder holds a `.git` entry, not from whether git happened to work, because a git that fails inside a work tree would otherwise be papered over with a walk that lists every untracked file. The git child runs with every `GIT_` setting taken out of its environment, because `GIT_DIR` outranks the folder it was given; it runs under a timeout and its output is read through a cap; and the sorted list has its repeats taken out, because git prints one line per stage of a file left in conflict. The generator omits dependency folders, build outputs, test artifacts, and version-control internals. A drift test in `make check` fails when the map is stale.

## The gate

`make check` is `scripts/gofmt.sh`, `go vet`, `staticcheck`, the style checker, the repository-map drift test, `scripts/coverage.sh`, and `make test`. The three scripts are driven by tests of their own in `scripts/gate`, which build a small module in a temporary folder and run the real script against it, because a shell script nothing tests is a gate nothing guards.

The format check reads the file list from `git ls-files` rather than from `find`, so it covers what this branch tracks and nothing in another worker's worktree, quotes every path, and checks gofmt's exit status as well as its output. The coverage gate is driven by `go list`, so a package the report never mentions is a failure rather than a silence, and a package with no test file is named for what it is. The fuzz script lists with the integration tag on and fails on a package whose test binary will not build, rather than skipping it. `internal/lint` reports a file it cannot parse as a violation naming the file, resolves the callee's package through the file's own imports so an alias or a dot import cannot hide an error message, counts an error message's words with the format verbs taken out, reads a field's trailing comment as well as the one above it, and allows the conventional one-letter names only in a test file, in a helper that takes a testing handle, and in a request handler.

## Wave log

Each wave gate adds a section here: what was built, what changed in the interfaces, what the next wave depends on. Three paragraphs at most.

### Wave 0 (planned)

The development machine readied, the skeleton, the contracts, the browser protocol document, the repo-map generator, and the first versions of the three living documents.

### Wave 3, brief 3.4: the terminal screen

`internal/tui` and `cmd/coeus/tui.go` are built, as described above. Nothing in
`internal/contract` or `internal/testkit` changed for them. The screen depends
only on the socket envelope, the home's socket path, `contract.Command`,
`contract.Clock`, and `internal/clock` for the real one, so it was written and
tested before `internal/channel` existed, against a fake dialer of its own and a
real Unix socket in a temporary home.

The screen was first written against three shapes the contract did not yet name,
and those are now named in it: `SocketEnvelope.MaskInput`, the twelve
`StatusField` names with `StatusCommandSeparator` and the five state words, and
`ApproveAlwaysText`. The screen reads and writes those names and no strings of
its own. The orchestrator has one thing left to wire: add `tuiSubcommand` to the
table in `cmd/coeus/main.go` and map the bare `coeus` command to it.
