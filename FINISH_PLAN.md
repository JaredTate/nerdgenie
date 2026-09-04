# Finish Plan: complete the platform, and rename Coeus to Nerd Genie

This plan takes the platform from "built and benchmarked" to "finished, hardened,
renamed, and ready to hand to a person." It runs as waves of at most five Opus 4.8
subagents at a time, every worker test-first, one area each, with a full `make check`
gate between waves. **Nothing here runs until you have read it and said go.**

Two things are true and shape the whole plan:

1. **The platform already works.** Every wave 0 to 6 brief is merged on `main`:
   61 internal packages, the real turn loop wired into `coeus serve`, tools,
   skills, jobs, memory, the record, the browser and desktop workers, the
   installer and updater. It passed a security review (23 findings fixed), a
   fresh-eyes review (89 findings), a human trial, and three benchmark suites
   this session. So this is a **finish-and-polish** plan, not a build plan.
2. **The rename is large and mechanical.** The name `coeus` appears in 1,094
   tracked files and 4,134 lines: the Go module path, the `cmd/coeus` binary,
   the `COEUS_HOME` env var, `coeus.db`, `coeus.sock`, the `~/.coeus` home, the
   systemd units, dozens of golden files, and every document. It must be one
   careful wave with its own gate, not sprinkled through the feature work.

---

## Part 1: Decisions I need from you before any wave runs

These change what the workers do, so answer them on this document:

1. **The module path.** `github.com/JaredTate/coeus` → `github.com/JaredTate/nerdgenie`?
   That implies renaming the GitHub repo too. Say the exact path you want.
2. **The on-disk names.** Change `COEUS_HOME`→`NERDGENIE_HOME`, `~/.coeus`→`~/.nerdgenie`,
   `coeus.db`→`nerdgenie.db`, `coeus.sock`→`nerdgenie.sock`, the binary to `nerdgenie`,
   the systemd units to `nerdgenie.service` / `nerdgenie-backup.*`? Recommended yes,
   since this is pre-release and there are no installs to migrate but your own.
   If you want the old `COEUS_HOME` to keep working for one release as a shim, say so.
3. **The command name.** The binary and the slash-command prompt become `nerdgenie`.
   A shorter alias people type (`ng`, `genie`)? Or just `nerdgenie`?
4. **The marketing surface.** Product name "Nerd Genie", agent persona already
   ships as "Nerd Genie" in the init template. Do you want a tagline, and should
   `README.md` be rewritten as a marketing-facing front page or stay a builder's
   readme with a marketing line at the top?
5. **The benchmark name.** Keep "Tater" for the benchmark, or rename it too?
6. **Order.** I recommend: finish-and-harden waves **first** (1 to 3), then the
   rename as wave 4, then the release/trial as wave 5. Reason: renaming touches
   every golden file, so doing it last means it is done once, over finished code,
   not re-done as features land. The cost is that waves 1 to 3 ship under the old
   name. Say if you would rather rename first.
7. **The live session.** Your Tetris run on card B and the benchmark scripts use
   `bin/coeus` and `COEUS_HOME`. The rename wave will not start until that session
   is done, or you tell me to stop it, so nothing in flight breaks.

---

## Part 2: Ground truth — what is done, and what is left

**Done and on `main`:** the loop, record, context builder, permission function,
sandbox, vault, all eighteen tools, channels, terminal screen, Signal, memory,
skills, jobs, scheduler, browser and desktop workers, reliability, updater,
replay, installer, release. Plus this session: `/think`, `/yolo`, the `codex`
provider, the five-line job rule with the job side panel, no task budget unless
set, paste in the terminal, and the three benchmark reports.

**Genuinely left, and the source that says so:**

- **`cmd/coeus` is under-tested.** 58 to 66 percent, the one package the coverage
  gate prints but does not enforce. It holds the wiring, and it is where bugs like
  the job double-claim and the cut-off-task loss lived. (`docs/PROGRESS.md`, gate notes.)
- **"Never forgets where it is."** Five gaps the budget worker mapped from the
  16:19 failure this session: recent tasks do not ride in the prompt; a status
  question ("where are we") starts a blind task; a job with zero tasks lets a
  project run in a plain task; a task still "running" after a hard shutdown is
  not marked interrupted on restart; closing the terminal window must not end a
  task. (Worker report, this session.)
- **The permission function over-asks.** It stopped six times in one benchmark run
  on a `node -e` command it could not read to the end. A read-only command should
  not need a yes. (Qwen benchmark, this session.)
- **The full human trial is unrun.** `docs/TRIAL.md` is written as a checklist;
  a person has not walked it end to end since the wave 6 features landed.
- **The release path is unproven end to end** under the new name: `make release`,
  `coeus install`, the systemd unit, `coeus update` with rollback. The container
  install test exists but has not run against a renamed build.
- **`make live` is green** on the three real models for the suites that exist; the
  wave-6-gate full run should be repeated once everything below has merged.

---

## Part 3: The waves

Every wave: at most five Opus 4.8 subagents, each in its own git worktree on its
own branch, each owning one package or area, **tests written first and watched to
fail**, coverage held above ninety percent (seventy for the screen and the two
workers). The orchestrator (me, on Fable 5.1) makes any `internal/contract` change
itself, tests first, before the wave, reviews and merges each branch, runs one
full `make check` alone as the gate, regenerates the repo map, and updates
`ARCHITECTURE.md` and `docs/PROGRESS.md`. No worker pushes; no worker touches
`cmd/coeus/main.go` or `serve.go`.

### Wave 1 — the agent never loses its place (5 agents)

The theme is the one the Tetris failure exposed: a long job that survives budgets,
restarts, and status questions.

1. **Recent work in the prompt** (`internal/context`): a small layer above the
   record with the last three tasks' ask and standing line and the newest job's
   progress, rebuilt from checkpoints at start. Test: finish a task, ask "where
   are we," the request carries the last task's ask.
2. **Status questions answer without a blind task** (`cmd/coeus/resuming.go`, a new
   file): "update", "where are we", "status", "what happened" are answered from the
   records with no model call; "continue" carries on a failed task as well as a
   stopped one. Test: the 16:19 scenario ends with the real status, not task 5 empty.
3. **The job is the project** (`internal/tool/job`, `internal/context` rules text):
   `create` refuses an empty job and starts its first task under the job; the rules
   tell the model that work of many features or a date is a job written first. Test:
   a Tetris-shaped ask lands as a job with a task list.
4. **Interrupted tasks survive a shutdown** (`internal/loop`, `cmd/coeus/starting.go`):
   on start, a record still "running" is marked interrupted at its last checkpoint
   and named in the first status; "continue" resumes it. Test: kill serve mid-task,
   restart, continue.
5. **Closing the window does not end a task** (`internal/channel`, `internal/tui`):
   detaching a screen leaves the task running under the service; a functional test
   detaches mid-task and reattaches to the same record.

Contract change I make first: a `RecentWork` field on the status envelope if
agents 1 and 5 both need it.

### Wave 2 — fewer stops, more polish (4 agents)

1. **The permission function stops asking about read-only commands**
   (`internal/permission`): a command that only reads (its whole line is `pwd`,
   `ls`, `cat`, `node -e`/`node --test` with no writing redirection, a pipe of
   readers) is allowed without a preview even when it cannot be read to the end;
   anything that writes, deletes, or runs sudo still asks. Test: the `node -e`
   case runs with no preview; `rm` still previews.
2. **`cmd/coeus` coverage to ninety** (`cmd/coeus`, test files only): raise the
   wiring package's tests to the gate the other packages meet, so the wiring is
   guarded. No behaviour change.
3. **The side panel shows the plan and job count** (`internal/tui`,
   `cmd/coeus/status.go`): the two status fields the panel worker left unsent are
   filled, so a person sees the plan and how many jobs wait, not only the running job.
4. **`/help` and the command list** (`internal/command`): one screen that lists
   every slash command with a line each, including the ones added this session
   (`/think`, `/yolo`, `/clear`), so a new person can discover them.

### Wave 3 — prove it end to end (3 agents)

1. **The functional suite grows the trial's spine** (`test/functional/`): the
   TRIAL.md checklist's automatable steps become functional tests against the fake
   model, so a regression in the trial path fails `make check`, not a person.
2. **The release and updater, proven under a rename-safe build**
   (`scripts/release`, `internal/update`): the container install test runs against
   a fresh `make release`, installs, starts the unit, and rolls a bad update back;
   golden service files are read from the build, not hard-coded.
3. **`make live` full run, recorded** (`docs/PROGRESS.md`): the whole live tier on
   the three real models, numbers written down, as the wave-6 gate always meant to.

### Wave 4 — the rename to Nerd Genie (up to 5 agents, one careful pass)

This is mechanical but touches everything, so it is its own wave with a hard gate.
It only starts once your live session is done and you have answered Part 1.

1. **The Go module and identifiers** (module path, `cmd/coeus`→`cmd/nerdgenie`,
   every import, every `Coeus`/`coeus` identifier that is not a person-facing
   string). A single scripted rename, then `go build ./...` and `make check` until
   green. This one agent owns the whole tree for this step, because a module rename
   cannot be split.
2. **The on-disk names** (`COEUS_HOME`, `~/.coeus`, `coeus.db`, `coeus.sock`, the
   systemd units, the release archive names) with, if you asked for it, a shim that
   still reads `COEUS_HOME` for one release. Golden service files regenerated and
   re-reviewed.
3. **The documents** (`COEUS.md`→`NERDGENIE.md`, `ARCHITECTURE.md`, `README.md`,
   `docs/`, `CLAUDE.md`): the product is Nerd Genie, the agent is Nerd Genie, the
   builder docs updated, marketing line where you want it.
4. **The tests and golden files**: every golden that embedded the old name
   regenerated deliberately, with the diff read, not blindly accepted.
5. **The benchmark and script tree** (`scripts/`, the three benchmark reports):
   renamed if you chose to rename the benchmark, left as history if not.

Because this wave rewrites so many golden files, its gate is stricter: `make check`
green, `make build` green, `make release` green, one real `coeus`/`nerdgenie serve`
smoke on the local model, and a diff review that no person-facing string still says
the old name.

### Wave 5 — the human trial and the finish (you plus me)

Not agents: you walk `docs/TRIAL.md` (now `NERDGENIE`'s) on the local Qwen, note
anything rough, and a short fix wave clears the notes. Then the final `make live`,
a tagged release, and the platform is done.

---

## Part 4: The rules every wave follows

From `CLAUDE.md`, non-negotiable: tests first and watched to fail; plain-English
identifiers, comments, and error messages; standard library first, no dependency
without your yes; every loop, wait, and buffer bounded; one package per worker;
never touch a package another worker owns this wave; contract changes only by the
orchestrator; `make repo-map` before every commit that moves files;
`ARCHITECTURE.md` updated in the same branch as the package it describes; `make check`
green before a wave passes; the machine rules (never kill by name, exact pids,
never restart the model daemons or digibyte-qt, one heavy job at a time).

## Part 5: Rough size

Waves 1 to 3 are about twelve worker-days of Opus 4.8 time, run five at a time, so
three sittings with a gate each. Wave 4 is one to two sittings, most of it one
agent doing the module rename and the rest verifying golden files. Wave 5 is your
afternoon plus a short fix wave. If a session limit or an API overload cuts a
worker off, its branch holds its work and it resumes, as they did this session.

---

_Waiting for your review. Tell me the Part 1 answers and the order, and I start
Wave 1._

https://claude.ai/code/session_01LiBFUY2d6WUqDnzPfpmYZK
