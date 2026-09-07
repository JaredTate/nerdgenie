# Contributing to Nerd Genie

Nerd Genie is an open-source AI agent that runs on your own Linux machine: you talk to it in a terminal or over Signal, it works with any language model, local or cloud, and it does not forget what it is doing, because it keeps a task record shaped like an Army operations order instead of re-reading its whole conversation every turn. It drives a real Chrome window and the Linux desktop like a person, and it is written in Go with two small TypeScript workers.

Start with `README.md` for the idea, `INSTALL.md` to get it onto a machine, and `SETUP.md` to configure and use it. `NERDGENIE.md` is the plain-words explanation with a check table that ties every claim to a test. `ARCHITECTURE.md` says how the code is put together, and `TESTING.md` says how it is tested.

## The ground rules

These are the hard rules from `CLAUDE.md`, restated for anyone outside the project. They apply to every line.

- **Keep it simple, and build only what is needed now.** The simplest thing that passes the test is the right thing. No abstraction until its second use exists. No configuration option until a real user needs it. No feature because it might be useful later. A package has one job that fits in one sentence in its `doc.go`.
- **Tests first.** Write the test, watch it fail, write the code, watch it pass. The commit history shows the order. A test written after the code is rewritten. See `TESTING.md`.
- **Go for the agent, TypeScript for `worker/browser` and `worker/desktop`, nothing else.** Standard library first. Every outside library is listed in `docs/DEPENDENCIES.md` with a one-line reason why the standard library will not do, and a new one needs the maintainer's yes before it is added.
- **Borrow designs, not code.** Read the reference, understand it, write it fresh. Never copy lines. Name the borrowed design and its path in a comment at the top of the file, and make sure its license is in `THIRD_PARTY.md`.
- **Plain English.** Identifiers say what they are. Comments are complete sentences. Error messages say what went wrong and what to do. Documents follow the same rule, with technical terms explained on first use. The style checker in `make check` enforces the code half; reviewers enforce the document half.
- **Bound everything.** Every loop has a limit, every wait a timeout, every buffer a cap, every outside call a failure path.
- **Every cross-package interface lives in `internal/contract`.** Fakes in `internal/testkit` and real implementations are written against the same lines. Never define an interface two packages share anywhere else.
- **Keep the docs honest.** When a change alters a package's job, interface, or dependencies, update the matching section of `ARCHITECTURE.md` in the same change. After adding or moving files, run `make repo-map`. `make check` fails if either is stale.

## How to propose a change

A fix for a clear bug can go straight to a pull request with its test. Anything beyond a fix starts with an issue that states the problem and the measurement: what goes wrong, on which model, how often, and what number would show it fixed. The nightly set (`scripts/nightly`) and the run report (`go run ./scripts/runreport`) are how the project measures a change, and a proposal that names the number it will move is easy to say yes to.

New ideas for the harness go through `IDEAS.md`. Each entry there carries its evidence, its expected gain, its risk, how it will be measured, and its cost. The maintainer approves an idea before it is built; an approved idea is built test first and lands in `docs/PROGRESS.md` with its measured effect. Every idea has to hold on any task, a book as much as a build, and on the local model that runs it.

## Pull requests

These rules are hard. A pull request that breaks one is sent back, not argued about.

- Tests first, visible in the commit order: the test commit comes before the code commit.
- `make check` is green on your machine, and its output is pasted into the pull request description.
- No pull request is reviewed without the full suite run. A partial run is not a run.
- Coverage does not drop below the gate: ninety percent per package, seventy for `internal/tui` and the two workers.
- No phantom tests: no test without an assertion, no skipped test, no test that passes whatever the code does.
- Golden files are refreshed only with a reason, and the reason is in the commit message.
- When a package's job or interface changes, `ARCHITECTURE.md`, `docs/DEPENDENCIES.md` if a library came or went, and the check table in `NERDGENIE.md` change in the same pull request.
- One change per pull request. A fix and a refactor are two pull requests.

## Commit messages

One line that says what changed, in plain words, then a blank line, then a short paragraph saying what failed and what changed, in plain sentences. The summary may lead with the package or area and a colon. Two from the history:

```
Test: the same test failing for a dozen runs draws a stuck line
```

```
shell: a poll waits up to twenty seconds for the command before saying it is still running

The play-test task started its own test script, got the ticket after ten
seconds, and asked whether it was done every five seconds: fourteen rounds of
still running for one run, each round a full prompt to the model. A poll now
waits on the tool's clock for the command, through the same waitFor that the
ten-second yield uses, and answers with the result when the command finishes
inside the wait; a command still running after PollWaitsFor is reported with
how long it has run, as before.
```

A message that says only "fix" or "update" is sent back.

## What gets a pull request rejected

- The code came before the test, or there is no test.
- `make check` output is missing from the description, or it is not green.
- Coverage fell below the gate.
- A phantom test: no assertion, a skip, or a pass that means nothing.
- A golden file refreshed without a reason.
- A new library with no row in `docs/DEPENDENCIES.md` and no yes from the maintainer.
- Code copied from a reference project rather than written fresh from its design.
- A package that imports a neighbour instead of going through `internal/contract`.
- An abstraction, option, or flag nobody asked for.
- `ARCHITECTURE.md` or `REPO_MAP.md` left stale.
- Comments that are not sentences, names that are abbreviations, error messages that do not say what to do.
- More than one change in the pull request.

## Code review

A reviewer reads every new file, not just the diff. They check that the test came first and fails without the code; that the simplest thing was built; that every loop, wait, and buffer is bounded; that nothing on the user's ask-me-first list runs without a yes, nothing secret reaches the model's context, and everything is logged; that the prose is plain; and that the documents are true. They will change one thing your test should catch and confirm it fails.

Expect a first reply within a week. A pull request whose author has not answered in thirty days is closed and can be reopened. Reviews are direct; a request for change is about the code, not about you.

## Running it locally

`INSTALL.md` gets the program, the model server, and the model files onto a Linux machine, and `SETUP.md` says how to use it. For tests, the local model is optional: the fakes in `internal/testkit` cover the whole loop, and `make test` needs only Go 1.27, Node 22 or newer for the workers, and the tools `make check` names. A real model is needed only for `make live` and the nightly set, which run on the development machine.

## Reporting a bug

Open an issue with:

- What you asked for, and what happened instead.
- The event log excerpt. The log is `nerdgenie.db` in the home; copy it (with its `-wal` file) rather than reading the file the agent is writing, and run `go run ./scripts/runreport --log <copy> --task N` for the task's numbers. Paste the rounds around the failure and the serve log lines from the same minute.
- The nightly table line, if the bug showed in a nightly run: the row from `docs/nightly/<date>.md` with how the task ended, the check, and the report's numbers.
- The model used: the alias, the model file the screen's header names, and the context length.
- The commit you were on (`nerdgenie version`).

A bug with a failing test attached is the best kind. Every live task that fails is meant to become the next nightly ask.

## Reporting a security problem

Do not open a public issue. Send it privately to the maintainer, Jared Tate, through the contact on the maintainer's GitHub profile, with the steps to reproduce and what an attacker could do. You will get a reply before anything is published. The threat list the project holds itself to is design section 11 of `docs/NERDGENIE_PLAN.md`.

## License

Nerd Genie is released under the MIT License; see `LICENSE`. By contributing, you agree that your contribution is licensed the same way. The projects whose designs were borrowed, and their licenses, are in `THIRD_PARTY.md`.

## Conduct

Be direct about the work and kind to the person; anyone who is not will be asked to leave.
