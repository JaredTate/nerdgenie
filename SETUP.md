# Setting up and using Nerd Genie

`INSTALL.md` gets the program and the local model onto the machine. This document is what comes after: the home folder, the configuration, the screen, the browser, giving it work, and what to do when something looks wrong. It is written in plain words for a person and for an AI.

## 1. The home folder

Nerd Genie keeps everything in one folder, the **home**. `nerdgenie init` makes it (`~/nerdgenie` by default), and `NERDGENIE_HOME` points a serve at a different one. Inside:

| what | where | notes |
|---|---|---|
| the configuration | `config.toml` | one commented line per setting |
| who it is and what it knows | `persona/SOUL.md`, `persona/USER.md`, `persona/MEMORY.md` | your words; the review adds lessons to MEMORY.md |
| skills | `skills/<name>/SKILL.md` | the browser skill ships with it |
| the event log | `nerdgenie.db` | every message, call and result, never rewritten |
| secrets | `vault.key` and the vault | passwords never reach the model |
| the socket, spill files, screenshots | `run/` | screens attach to `run/agent.sock` |

The home must sit outside every folder Nerd Genie may work in. That is checked, and refused, on purpose.

## 2. The configuration that matters

Open `config.toml`. Every line has a comment, and these are the ones to get right:

```toml
default_model = "local"

sandbox = "off"                                   # "fence" boxes commands into the roots below
sandbox_roots = ["/home/you/work"]                # the folders it may read and write

[[models]]
name = "local"
provider = "openai"
base_address = "http://127.0.0.1:19091/v1"
model_name = "local-coder"
context_length = 131072                           # the same number the server was started with
vision = true                                     # the server loaded the projector, so pictures reach the model
```

`context_length` must match the server's `--ctx-size`; the harness sizes every prompt to it. `vision = true` only when the server reports `"vision": true` on `/props`; with it off, a screenshot is described in words and the model is told the picture is not shown.

For a second card, a second home with `base_address = "http://127.0.0.1:19093/v1"` and its own `sandbox_roots`.

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
- `^B` hides the panel, `tab` moves focus, `esc` stops the task, `/help` lists the commands.

If you restart the serve, turn yolo back on (`/yolo`) if you had it on. A restart always brings the asking back.

## 4. Give it work

Type in the screen, or from a terminal:

```sh
NERDGENIE_HOME=~/nerdgenie bin/nerdgenie run "In ~/work/demo, write hello.py that prints hello and run it."
NERDGENIE_HOME=~/nerdgenie bin/nerdgenie run -wait -timeout 3h "$(cat ~/asks/tetris.md)"
```

Put a long ask in a file and pass it with `$(cat …)`; one stray apostrophe on a command line has killed a whole run silently.

Small asks are one **task**. A long ask, many features or a long list, becomes a **job**: the model writes the task list first and the harness runs one task at a time, reporting after each. An ask over six hundred words must be a job of at least three tasks, and the harness refuses anything less.

Slash commands work in the screen and over Signal: `/tasks`, `/jobs`, `/status`, `/memory`, `/skills`, `/undo`, `/stop`, `/clear`, `/yolo`, `/model`, `/think`.

**Yolo** means "run every call that would have stopped to ask me". It is what an unattended run needs. Anything on the ask-me-first list still asks; a rule that says never is still never.

## 5. The browser and the pictures

Nerd Genie drives its own Chrome window, with its own profile, on your desktop. The model opens a page and reads it as an outline of elements, acts on them by reference, and is told after every action whether what it expected happened. With `vision = true` it can also take a `browser_screenshot`, read a `.png` or `.jpg` with `read`, or take a desktop screenshot, and the picture reaches it as an image. A 1024 by 768 screenshot costs about 800 tokens; only the newest two pictures stay in the window.

A page it is building that hangs the browser is reported with the loop that never yields, by file and line, and the browser is started again on the next call. It fixes its own game this way.

## 6. Measure it

`scripts/nightly/run.sh <home> --with-tetris` runs the fixed set of asks on a fresh copy of a home and writes a table to `docs/nightly/<date>.md`: rounds, minutes, seconds a round, cache share, uncached tokens a round, rounds cut off at the output cap, refusals by tool. `scripts/runreport --log <db> --task N` prints one task's numbers. Every change to the harness this week came from one of these tables.

## 7. When something looks wrong

| what you see | what it is | what to do |
|---|---|---|
| the screen says disconnected | the serve stopped | start it again; the screen reconnects |
| everything takes twenty times longer than yesterday | the model fell off the card into system memory | `ps -o rss= -p <server pid>`; if it holds 20 GB, restart the server with a smaller context or without vision |
| the model keeps asking yes or no in an unattended run | yolo went off with a restart | `/yolo` |
| a task ends "waiting" on a small ask | the model ended on a question | say what you want, or `/clear` |
| a job stopped at "n of m tasks" | a task stopped, or the run was interrupted | any message picks the put-down task up |
| "the record refused this change" | the model broke one of the record's rules | nothing; the refusal tells the model what to write instead |
| the nightly table's cut-off column is not zero | replies hit the 8,192-token output cap | the harness already tells the model to write files in parts; watch the next run |

Logs: the serve writes to wherever you pointed it; the local model server writes to `~/llm/logs/<port>.log`; the event log is the truth of what happened, and `scripts/runreport` reads it.

## 8. Running two agents on one machine

Card A and card B are two servers on two ports (`INSTALL.md` section 7). Give each its own home, its own work folder, its own ports for anything it serves, and its own screen window. Do not share `localhost:8090`, a Chrome profile, or a git checkout between them. Start the cards one at a time. Never kill either by name pattern.
