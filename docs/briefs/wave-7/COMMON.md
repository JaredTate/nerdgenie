# Wave 7: the rules every brief in this wave starts with

Before you write anything, read these five files in this order: `CLAUDE.md` for the rules and the commands, `NERDGENIE.md` for the plain-words explanation of the state design and the check table that ties each claim to a brief and a test, `ARCHITECTURE.md` for how the code is put together and what earlier waves built, `REPO_MAP.md` for where everything lives, and `docs/NERDGENIE_PLAN.md` for the design you are implementing, especially the section this brief names. Then read this brief and the reference files it points at. You own exactly one package. Write the tests first.

Why this wave exists. On the night of 6 September 2026 the local 3-bit model on the 5070 Ti machine ran the Tetris ask (`TETRIS_TEST_PROMPT.md`) and its browser-shell task looped for 71 minutes, 213 rounds, 228 calls, before the guard stopped it twice. The event log (`~/.nerdgenie/nerdgenie.db`, task 8) showed four causes, and this wave fixes them: the shell tool's "same as the last run" answer broke the syntax check after a write on 34 of 46 writes; the same-call guard counts only unbroken runs, so A, B, A, B never trips it; the test-state reader did not know the model's own runner, so the harness saw no test run all task; and a stall got a nudge and a cut, not a real rethink. Each brief names the numbers it stands on.

The rules, in short:

- Go is at `/usr/local/go/bin` and staticcheck at `~/go/bin`; put both on your `PATH` first: `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin`.
- Tests first. Write the failing test, run it and watch it fail for the right reason, commit it with a message beginning `Failing tests:`, then write the code and commit that. Never commit red. Never push.
- Keep it simple and build only what the brief asks for. No abstraction until its second use exists, no option nobody asked for, no feature for later.
- Plain English: identifiers say what they are, comments are complete sentences, errors say what went wrong and what to do. `go run ./scripts/stylecheck ./<your package>/...` must pass.
- Bound everything: every new number is a named constant near the top of its file with a sentence saying why that number.
- No function over sixty lines, no file over five hundred lines.
- Before you report: `gofmt -l` on your files prints nothing; `go vet ./<pkg>/...`; `staticcheck ./<pkg>/...`; `go test -tags integration -cover ./<pkg>/...` passes with coverage above ninety percent for the package. Do not run `make check`; the orchestrator runs it after the merge.
- Never edit `cmd/nerdgenie/main.go` or `cmd/nerdgenie/serve.go`, and never touch a package another brief in this wave owns.
- Keep the docs honest: update the matching section of `ARCHITECTURE.md` and the row of the check table in `NERDGENIE.md` this brief names, and add three lines under a new heading at the end of `docs/PROGRESS.md`. If you add or move a file, run `make repo-map`.
- You work in your own git worktree on your own branch. Commit there. In your final report say: the branch name, the worktree path, every test you wrote by name, the coverage number, and anything you left undone and why.
