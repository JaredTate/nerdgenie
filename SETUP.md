# Setting up and using Nerd Genie

`INSTALL.md` gets the program and the local model onto the machine. This document is what comes after: the home folder, the configuration, the screen, the browser, giving it work, and what to do when something looks wrong. It is written in plain words for a person and for an AI.

## 1. The home folder

Nerd Genie keeps everything in one folder, the **home**. `nerdgenie init` makes it (`~/nerdgenie` by default), and `NERDGENIE_HOME` points a serve at a different one. Inside:

| what | where | notes |
|---|---|---|
| the configuration | `config.toml` | one commented line per setting |
| who it is and what it knows | `persona/SOUL.md`, `persona/USER.md`, `persona/MEMORY.md` | your words; the review adds lessons to MEMORY.md, and facts moved out when it fills go to `memory/` |
| skills | `skills/<name>/SKILL.md` | the browser skill ships with it |
| the event log | `nerdgenie.db` | every message, call and result, never rewritten |
| secrets | `vault.key` and the vault | passwords never reach the model |
| the socket, spill files, screenshots | `run/` | screens attach to `run/agent.sock`; pictures land in `run/screenshots/` |
| your own tools | `tools/` | one executable per tool; read at startup |
| the browser profile, backups, Signal's state, files sent to it | `browser/`, `backups/`, `signal/`, `inbox/` | nothing here is read by the model |

The home must sit outside every folder Nerd Genie may work in. That is checked, and refused, on purpose.

## 2. The configuration that matters

Open `config.toml`. Every line has a comment, and these are the ones to get right:

```toml
default_model = "local"

yolo = true                                       # start running every call without asking; false to be asked. "/yolo" toggles it for a session
sandbox = "off"                                   # "fence" boxes commands into the roots below
sandbox_roots = ["/home/you/work"]                # the folders it may read and write

[[models]]
name = "local"
provider = "openai"
base_address = "http://127.0.0.1:19091/v1"
model_name = "local-coder"
context_length = 131072                           # the same number the server was started with
vision = true                                     # the server loaded the projector, so pictures reach the model
think = ""                                        # empty = thinking off for the local model, which qwen wants; set low/medium/high/xhigh/max to turn it on
```

`context_length` must match the server's `--ctx-size`; the harness sizes every prompt to it. `vision = true` only when the server reports `"vision": true` on `/props`; with it off, a screenshot is described in words and the model is told the picture is not shown.

**Thinking is off by default for the local model, and you can change it.** Qwen 3.8 works better without its hidden reasoning, so the harness tells the local server not to think unless `think` names a level (`low` through `max`), or you type `/think` for one session. Because the local server is on this machine, the harness sends the thinking-off instruction whether or not it had answered its startup probe, so a harness that started before the daemon does not silently fall back to full-effort thinking.

**Yolo is on by default**, because the agent is built to run unattended and cannot sit waiting for a yes nobody is there to give. Every call that would ask first runs and is logged as allowed by yolo. Set `yolo = false` to be asked, or type `/yolo off` for one session; a refusing rule on your ask-me-first list still refuses.

For a second card, a second home with `base_address = "http://127.0.0.1:19093/v1"` and its own `sandbox_roots`.

Other models sit beside it as more `[[models]]` blocks, and `fallback_chain = ["claude", "codex"]` names the ones to try when the first fails:

```toml
[[models]]
name = "claude"
provider = "cli"                                  # drives the vendor's own program on your subscription
program = "claude"
model_name = "claude-opus-4-8"
context_length = 200000

[[models]]
name = "ollama"
provider = "openai"                               # any OpenAI-compatible server, local or a cloud gateway
base_address = "http://127.0.0.1:11434/v1"
model_name = "glm-5.3:cloud"
context_length = 131072
```

`/model claude` switches a running serve to another block. Ollama's servers report no token counts while streaming, so the cost line reads zero on them.

## 3. Start it and open the screen

```sh
NERDGENIE_HOME=~/nerdgenie setsid bin/nerdgenie serve >> ~/nerdgenie/serve.log 2>&1 < /dev/null &
NERDGENIE_HOME=~/nerdgenie bin/nerdgenie tui
```

The serve is the agent. The `tui` is a screen that attaches to it; open it in a terminal window on your desktop and leave it there. It reconnects by itself if the serve restarts. Keep one window per home: a second screen on the same home is fine to look at, but two people typing into one task is not.

What the screen shows:

- **The top line:** the model alias and the model file the server loaded (`local hauhau-Q4_K_P`), the context meter, the task and its round, the cache share, and the session's tokens. The dot on the right is the health check.
- **The strip at the bottom:** what it is doing now, how long the call has run, how many tokens it has written, and the last call's speeds: `prefill 418 tok/s · output 58 tok/s`. Prefill is how fast the server read the prompt; output is how fast it wrote.
- **The side panel:** MODEL, NOW, the job's task list with its marks, the STATE of files and commands, FAILURES, and the ROUND.
- `^B` hides the panel, `tab` moves focus, `/help` lists the commands. `esc` lets go of one thing at a time: a focus on the panel, then a record you opened by clicking a task or a job, then any opened pills; with nothing left to let go of, `esc` stops the task. So after clicking a task or a job, one `esc` brings the live view back, and a third `esc` out of habit is a stop. A click on the panel's NOW rows brings the live view back too.

If you restart the serve, turn yolo back on (`/yolo`) if you had it on. A restart always brings the asking back.

## 4. Give it work

Type in the screen, or from a terminal:

```sh
NERDGENIE_HOME=~/nerdgenie bin/nerdgenie run "In ~/work/demo, write hello.py that prints hello and run it."
NERDGENIE_HOME=~/nerdgenie bin/nerdgenie run -wait -timeout 3h "$(cat ~/asks/tetris.md)"
```

Put a long ask in a file and pass it with `$(cat …)`; one stray apostrophe on a command line has killed a whole run silently. The seven `EX_PROMPT_*` files in the repository root are complete asks you can run as they are, with `<WORK>` replaced by the folder the project should go in:

```sh
NERDGENIE_HOME=~/nerdgenie bin/nerdgenie run -wait -timeout 5h "$(sed 's|<WORK>|/home/you/Desktop|g' EX_PROMPT_3_TETRIS.md)"
```

`PROMPT_TEMPLATE_GUIDE.md` says how to write one: a goal, where, what done looks like, the rules, the tasks, and the details under their own headings. An ask in that shape is turned into the job before the model is called: your done lines, with their bracketed checks, become the job's done list; your rules ride in front of every task, with tests first as the first; your tasks become the job's tasks in order, and each task sees only the Details sections its line names. A plain ask works too; the model then writes the job itself.

Small asks are one **task**. A long ask, many features or a long list, becomes a **job**: the model writes the task list first and the harness runs one task at a time, reporting after each. A done list over five lines or a plan over ten steps is refused with the words "this ask is a job", so the model makes one.

Slash commands work in the screen and over Signal: `/tasks`, `/jobs`, `/cron`, `/status`, `/memory`, `/skills`, `/undo`, `/stop`, `/clear`, `/yolo`, `/model`, `/think`, and `/help` lists them all.

**Yolo** means "run every call that would have stopped to ask me". It is what an unattended run needs. Anything on the ask-me-first list still asks; a rule that says never is still never.

## 5. The browser and the pictures

Nerd Genie drives its own Chrome window, with its own profile, on your desktop. The model opens a page and reads it as an outline of elements, acts on them by reference, and is told after every action whether what it expected happened. With `vision = true` it can also take a `browser_screenshot`, read a `.png` or `.jpg` with `read`, or take a desktop screenshot, and the picture reaches it as an image. A 1024 by 768 screenshot costs about 800 tokens; only the newest two pictures stay in the window.

A page it is building that hangs the browser is reported with the loop that never yields, by file and line, and the browser is started again on the next call. It fixes its own game this way.

## 6. The project's own documents

A work folder may carry three files the harness reads for the model. `NERDGENIE.md`, the project's rules and how to run and test it, rides under the job summary on every call of a task in that folder, cut at sixty lines. `ARCHITECTURE.md` and `REPO_MAP.md` are never read whole: at every task start and fresh window the orientation block names the architecture page's sections and the map's root folders, and the model reads one section with `read ARCHITECTURE.md <heading>`. Write them for any project you keep, and keep `NERDGENIE.md` short; it is in front of the model on every call.

The harness keeps them too. At the end of every task the review asks which section of `ARCHITECTURE.md` the task changed and writes the answer under that heading, dated; a folder with no page gets one from the first job task. A job that finishes with every done line proved writes `NERDGENIE.md` when the folder has none, from the job's own record: what it is, how to run and test it, the rules. It never overwrites a file you wrote.

Two more things the harness does on every task, whatever the ask looked like. A write or edit to a code file, when no test has failed since the last green run, gets one line on its result naming the test to write: "tests first: no failing test covers this change; add a failing test to tests/game.test.js first" when the project has that test file, "write it first in tests/game.test.js" when it does not yet. And a done line that ends in a bracket the harness can run, `[tests pass: npm test]`, `[exit 0: node build.js]`, `[shows: "Tic Tac Toe" at http://127.0.0.1:8096]` or `[exists: dist/index.html]`, is run by the harness at the end of every task and at the finish; the job header counts "done lines proved: 1 of 2", and a job cannot close while a check fails.

## 7. Measure it

`scripts/nightly/run.sh <home> --with-tetris` runs the fixed set of asks on a fresh copy of a home and writes a table to `docs/nightly/<date>.md`: rounds, minutes, seconds a round, cache share, uncached tokens a round, rounds cut off at the output cap, refusals by tool. `go run ./scripts/runreport --log <db> --task N` prints one task's numbers. Every change to the harness this week came from one of these tables.

## 8. When something looks wrong

| what you see | what it is | what to do |
|---|---|---|
| the screen says disconnected | the serve stopped | start it again; the screen reconnects |
| everything takes twenty times longer than yesterday | the model fell off the card into system memory | `ps -o rss= -p <server pid>`; if it holds 20 GB, restart the server with a smaller context or without vision |
| the model keeps asking yes or no in an unattended run | yolo went off with a restart | `/yolo` |
| a task ends "waiting" on a small ask | the model ended on a question | say what you want, or `/clear` |
| a job stopped at "n of m tasks" | a task stopped, or the run was interrupted | any message picks the put-down task up |
| "the record refused this change" | the model broke one of the record's rules | nothing; the refusal tells the model what to write instead |
| the nightly table's cut-off column is not zero | replies hit the 8,192-token output cap | the harness already tells the model to write files in parts; watch the next run |
| a task ends "rounds without progress, four times over" or "asked for the same call over and over" | the progress meter or the same-call guard stopped a model that was going round in circles, after three cuts and a rethink | read the record's failures; a job picks such a task up once by itself, and any message picks it up again |
| the model talks about a build that is not there | it read the last run's lessons in `persona/MEMORY.md` or `memory/` | for a fresh test, wipe the memory (section 10) |

Logs: the serve writes to wherever you pointed it; the local model server writes to `~/llm/logs/<port>.log`; the event log is the truth of what happened, and `scripts/runreport` reads it.

## 9. Running two agents on one machine

Card A and card B are two servers on two ports (`INSTALL.md` section 7). Give each its own home, its own work folder, its own ports for anything it serves, and its own screen window. Do not share `localhost:8090`, a Chrome profile, or a git checkout between them. Start the cards one at a time. Never kill either by name pattern.

## 10. A fresh run for a test

A run meant to be measured starts from nothing, and archiving the event log alone is not enough: the learned facts live outside it. Stop the serve by its exact pid, then:

```sh
cd ~/nerdgenie
mv nerdgenie.db nerdgenie.db.run12                      # the log, kept by run number (and its -wal and -shm if present)
mkdir -p old-runs/memory.run12
mv memory/MEMORY-*.md persona/MEMORY.md old-runs/memory.run12/   # the learned facts
head -2 old-runs/memory.run12/MEMORY.md > persona/MEMORY.md     # the two template lines and nothing else
rm -f run/screenshots/*
```

Empty the work folder too, or the model will find the last build in it. Then start the serve, turn `/yolo` on, open the screen, and submit the ask. `scripts/nightly/run.sh` does all of this on a copy of the home.
