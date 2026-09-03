# The Tater Benchmark — four coding harnesses, one local model

This is an honest, apples-to-apples comparison of four coding harnesses building
the same small program with the same local model on the same graphics card,
judged by the same independent checker. It measures both how fast and cheap each
harness is, and the quality of what it actually builds.

## What was held identical

- **The model.** One llama-server daemon on card A, port 19091: the Qwen 3.8 27B
  uncensored GGUF (`hauhau-Q4_K_P`), turbo3 KV cache, embedded MTP, sampling baked
  into the daemon (temperature 0.7, top-p 0.8, top-k 20, presence-penalty 1.5,
  thinking off). Every harness spoke to this one daemon, so the model, the quant,
  and the sampling were the same for all four. Only one daemon and one harness ran
  at a time, so no run competed with another for the card.
- **The task.** The byte-identical canonical task at
  `~/work/bench/canonical/task.txt` (sha256 begins `c8ed58f2`): build a
  browser tic-tac-toe game with a pure-function `game.js` ES module, an
  `index.html` that plays, and a Node test file, told to be quick and to stop the
  moment the tests pass. The same file was handed to every harness, verified by
  its sha before each run.
- **The starting point.** Every run began in a new, empty, `git init`-ed folder
  `~/work/bench/tater/<harness>-<run>/`. For Coeus, a fresh home was made with
  `coeus init --yes`, its model left as the local one, its sandbox left off, and
  its work folder set as the only sandbox root. To keep it apples-to-apples with
  the others — which only know the local daemon — Coeus's cloud fallback chain
  was emptied, so a stumble on the local model could not silently borrow a cloud
  subscription.
- **The judge.** One checker, `scripts/bench/check-tater.mjs`, written for this
  benchmark and never trusting a harness's own tests. It runs the six logic
  checks against the folder's `game.js`, then serves the folder and *plays* the
  page in headless Chrome through the DevTools protocol (real clicks, reading the
  real status line), then counts and runs the harness's own tests and scores a
  small quality rubric.

## What was measured, and how

Each harness did its own thing — its own planning, its own number of steps, its
own way of checking its work. The steps it took, the model calls it made, the
tokens it read and generated, and the wall-clock time are the *measured result*
of letting it work, not anything forced on it.

**Speed and cost** come from the card-A daemon's own log, sliced from the line
count noted just before each run (`scripts/bench/countcalls.sh`): wall time from
launch to exit, the number of model calls, the prompt tokens the model had to
read (total, per-call average, and the largest single prefill — how much context
each harness makes the model re-read), and the tokens generated. The daemons run
without `--metrics`, so these come from the log, the same way for all four.

**Quality** is judged by `check-tater.mjs` on three axes:

1. **Correctness (of 6)** — a win in a row, a column, and a diagonal; a draw; an
   illegal move on a taken cell; and no moves after a win, all run against the
   folder's own `game.js`.
2. **Actually plays (of 4)** — the grid has nine cells; clicking a real X-winning
   sequence makes the status line announce X; a New game button clears the board
   and resets the status; and clicking an already-taken cell changes nothing.
3. **Thoroughness** — how many test cases the harness wrote, how many pass under
   `node --test`, the line count of `game.js`, and a quality score (of 6): shows
   whose turn it is, announces a draw, announces a win, has a New game control,
   uses one accent colour, and keeps the game functions pure.

A screenshot of each finished game is saved under `docs/bench/`.

Any harness that finished under ten minutes was run twice; the tables report both
runs and their median. A harness that took longer was run once.

## The exact commands

```sh
# The one daemon, on card A (already running for every harness):
setsid ~/llm/igo.sh Vulkan1 19091 262144 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &

# Each harness, on a fresh ~/work/bench/tater/<harness>-<run>/ folder, launched
# with setsid under a hard 30-minute cap (see scripts/bench/tater_run.sh):

# Coeus — fresh home, sandbox off, work folder as the sole sandbox root, driven
# over its socket by scripts/bench/drive.py (which stands in for a person):
COEUS_HOME=<home> coeus init --yes
#   config.toml: sandbox_roots = ["<work>"], fallback_chain = []
scripts/bench/run-coeus.sh coeus <home> <work>/task.txt 28

# opencode:
cd <work> && OPENCODE_CONFIG=~/.config/opencode/opencode.json \
  ~/.opencode/bin/opencode run "$(cat task.txt)"

# Hermes:
hermes chat -q "$(cat <work>/task.txt)" --provider turbo-a -m local-coder \
  --reasoning low --yolo --max-turns 250 --in <work>

# OpenClaw (its vllm provider already points at 127.0.0.1:19091/v1, local-coder):
openclaw agent exec --message-file <work>/task.txt --model vllm/local-coder \
  --cwd <work> --timeout 1740

# Then, for every harness, the same measurement:
scripts/bench/countcalls.sh ~/llm/logs/19091.log <lines-before-the-run>
node scripts/bench/check-tater.mjs <work> --harness <name> --run <n> --shot docs/bench/<name>-<n>.png
```

## Speed and cost

Numbers are per run; where a harness was run twice, the median is shown with
both runs noted below the table. "Prompt tokens read" is what the model could
*not* take from its cache and had to read again — the cost of the context a
harness re-sends. "Largest prefill" is the single biggest such read.

| harness | runs | wall (s) | model calls | prompt tokens read | avg/call | largest prefill | generated tokens |
|---|---|---|---|---|---|---|---|
| Coeus | 1 | 830 | 62 | 239,040 | 3,855 | 20,804 | 9,568 |
| opencode | | | | | | | |
| Hermes | | | | | | | |
| OpenClaw | | | | | | | |

## Quality

| harness | correct (/6) | plays (/4) | tests written | tests passing | game.js lines | quality (/6) |
|---|---|---|---|---|---|---|
| Coeus | 6 | 4 | 6 | 6 | 38 | 6 |
| opencode | | | | | | |
| Hermes | | | | | | |
| OpenClaw | | | | | | |

## The screenshots

<!-- FILLED AS RUNS COMPLETE -->

## Verdict

<!-- FILLED AT THE END -->
