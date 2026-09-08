# Installing Nerd Genie

This is how to get Nerd Genie onto a Linux machine with a local model behind it. It is written for a person at a keyboard and for an AI reading it before it sets a machine up. Every command is one you type in a terminal. Nothing here needs an API key or an internet account; the model runs on your own graphics card.

There are three parts: the program, the local model server, and the model files. Do them in that order. `SETUP.md` is the next document: it says how to configure and use Nerd Genie once it is installed.

The local model is the fastest path and the one everything here was tuned on, but it is not the only one. Any server that speaks the OpenAI-compatible API works, Ollama and LM Studio among them, and so does a Claude Code or Codex subscription through the command-line provider; those need only step 2 and `SETUP.md` section 2.

## 1. What you need

- Linux. Ubuntu 24.04 and Linux Mint 22 are what we run.
- Go 1.27 or newer, to build the program.
- Node 22 or newer, to build the two workers (browser and desktop). Node 24 is what we run.
- Google Chrome, if you want the browser tools. Nerd Genie starts its own Chrome with its own profile.
- A graphics card with at least 16 GB of memory for the local model. 24 GB is comfortable.
- About 20 GB of disk for the model file and the vision projector.

## 2. Build the program

```sh
git clone https://github.com/JaredTate/nerdgenie ~/Code/nerdgenie
cd ~/Code/nerdgenie
make build
```

That puts the program at `bin/nerdgenie` and the two worker bundles under `bin/workers/`. `make check` runs every test and every checker; it takes a few minutes and should be clean. Run it before the model server is up, not beside it: its fuzzers once took the server off the card.

## 3. Install the model server

Nerd Genie talks to a local model over the OpenAI-compatible API. We use the TurboQuant fork of llama.cpp, because it compresses the model's memory about five times, which is what lets a 27B model hold a long context on one card.

1. Download the release from `https://github.com/AtomicBot-ai/atomic-llama-cpp-turboquant/releases`. Take the **`.tar.gz`** for your card: `llama-turboquant-linux-x64-vulkan.tar.gz` for AMD cards, the `cuda` one for NVIDIA. Do not take the zip; it breaks the shared library links and the server silently falls back to the CPU.
2. Unpack it somewhere stable, such as `~/llm/turboquant/llama-server`.
3. Check it runs: `<path>/llama-server --help | head`.

## 4. Get the model files

Make a folder `~/llm/models/` and put two files in it.

- **The model.** Qwen 3.8 27B in a GGUF that keeps its MTP head, from `https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF`. Pick the quant that fits your card: `hauhau-Q4_K_P.gguf` (18 GB) for a 24 GB card, `hauhau-IQ4_XS.gguf` (16 GB) for a 20 GB card, `hauhau-IQ3_M.gguf` (13 GB) for a 16 GB card. The MTP head is what makes generation about twice as fast; standard GGUF conversions strip it, so a model from somewhere else will work but slowly.
- **The vision projector**, so the model can read pictures: `mmproj-Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-BF16.gguf` from the same repository, or `mmproj-F16.gguf` from `https://huggingface.co/unsloth/Qwen3.8-27B-GGUF`. Either is about 930 MB. Save it as `~/llm/models/mmproj-Qwen3.8-27B-F16.gguf`.

Check the model is what you think: the file name is not proof. Read `general.name` out of the GGUF header, or ask the server once it is up.

## 5. Start the server

Write a start script, `~/llm/igo.sh`, that takes the card, the port, the context length and whether to use MTP, and start it detached:

```sh
setsid ~/llm/igo.sh Vulkan1 19091 131072 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &
```

What the script sets, and why each setting is what it is:

| setting | value | why |
|---|---|---|
| card | `--device Vulkan1` | Vulkan0 is usually the built-in graphics; the discrete card is 1 (and 2 for a second card) |
| memory compression | `--cache-type-k turbo3 --cache-type-v turbo3 --flash-attn on -kvu` | holds five times the context in the same memory |
| `TURBO_AUTO_ASYMMETRIC=0` | in the environment | stops the library quietly upgrading part of the memory to a bigger format; worth 2 GB |
| prediction | `--spec-type draft-mtp --spec-draft-n-max 3` | the model's own next-token head, about twice as fast; depth 3 on 24 GB cards, 2 on 16 GB cards |
| sampling | `--temp 0.7 --top-p 0.8 --top-k 20 --presence-penalty 1.5` | 0.7 is the number; 0.3 makes the agent repeat a command forever |
| thinking | `--reasoning-budget 0` | off; the harness keeps the state, not the model's scratchpad |
| context | `--ctx-size 131072` | the most that fits beside the vision projector on a 24 GB card |
| checkpoints | `--checkpoint-min-step 1024 --ctx-checkpoints 16` | this model rewinds only to checkpoints; every 1,024 tokens instead of 8,192 cuts a round's re-reading from about 4,900 tokens to about 1,500 |
| vision | `--mmproj <projector>` | the model reads screenshots and picture files; `MMPROJ=none` turns it off |
| alias | `-a local-coder` | the name Nerd Genie asks for; it must not contain "qwen" |
| one at a time | `--parallel 1` | two conversations would split the context |

Then check it, every time you start it, because a server that fell off the card into system memory looks alive and runs twenty times slower:

```sh
curl -s http://127.0.0.1:19091/health          # {"status":"ok"}
curl -s http://127.0.0.1:19091/props | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["modalities"], d["default_generation_settings"]["n_ctx"])'
ps -o rss= -p $(ss -ltnp | grep :19091 | grep -o "pid=[0-9]*" | cut -d= -f2)   # under 1000000 means the model is on the card; 20 GB means it is not
```

If the process holds 20 GB of RAM, the model did not fit. Lower the context, drop to a smaller quant, or turn vision off.

## 6. Set Nerd Genie up

```sh
bin/nerdgenie init
```

It makes the home folder, asks which folders Nerd Genie may work in, finds the local server on its port, and writes `config.toml`. Every question has a flag, so a machine with no keyboard is set up in one line: `bin/nerdgenie init -yes -model local -work-folder ~/work -signal off`. The model choices are `local`, `lmstudio`, `claude`, `codex`, `anthropic` and `openai`. Then:

```sh
bin/nerdgenie doctor       # checks everything and says what to fix
bin/nerdgenie install      # optional: a systemd user service that starts with your session
```

`SETUP.md` takes it from here: the configuration, the screen, the browser, and running work.

## 7. More than one card, and more than one machine

Each graphics card runs one server on its own port, one process, nothing shared. On a machine with two cards that is **A and B**:

```sh
setsid ~/llm/igo.sh Vulkan1 19091 131072 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &
# wait for the first to answer /health before starting the second; two models loading at once can take the machine down
setsid ~/llm/igo.sh Vulkan2 19093 131072 mtp > ~/llm/logs/19093.log 2>&1 < /dev/null &
```

Each server gets its own Nerd Genie home, and each home points at its own port in `config.toml`. Start the second serve with `NERDGENIE_HOME=~/nerdgenie-b nerdgenie serve`. Two agents on one machine are proven clean as long as nothing is shared: separate work folders, separate ports for anything they serve, separate Chrome profiles.

The cards this was tested on, and the settings that fit each:

| card | model file | context | notes |
|---|---|---|---|
| RX 7900 XTX 24 GB (Vulkan) | `hauhau-Q4_K_P.gguf` | 131,072 with vision | the primary; MTP depth 3; two of them on one machine run as A and B on `:19091` and `:19093` |
| RX 7900 XT 20 GB (Vulkan) | `hauhau-IQ4_XS.gguf` | 262,144 without vision; 131,072 with | 20 GB holds the longer context or the projector, not both. At 131,072 with vision, measured 7 Sep 2026: 60 tok/s shallow, 30 tok/s at 85K depth, exact recall at 84K. 262,144 with the projector loads and answers a short ask, then runs out of memory deep in a job — do not run it |
| RTX 5070 Ti 16 GB (CUDA) | `hauhau-IQ3_M.gguf` | 65,536 with vision; 114,688 without | the CUDA build; turbo3 is broken on NVIDIA's Vulkan; 128k loads and then dies mid-job, so 114,688 is the ceiling without the projector |
| RDNA4 16 GB (Vulkan) | a 3-bit quant | 64k to 100k | to be measured |

Every machine sets `TURBO_AUTO_ASYMMETRIC=0`. On a machine with two cards, a `~/llm/start.sh` that starts them one at a time and answers `start.sh status` saves a lot of mistakes.

## 8. Rules that are not optional

- Never kill a server by name pattern (`pkill -f`, `killall`, `pgrep -af`). A shell's own command line matches the pattern and dies with it. Kill exact process ids, found by port.
- Never start a second server on a card that already has one.
- Never run `ollama pull` on these machines; the models are plain files, not Ollama blobs, and Ollama will grab a card.
- Keep one port free for whatever else the machine serves; ours is 8090, and every ask says so.
- Start cards one at a time.

## 9. Updating

`nerdgenie update` installs the newest release and keeps the one before it, so a bad update rolls back by itself when the new program does not come up; `nerdgenie update --rollback` goes back on purpose. To update a source build, `git pull && make build`, then restart the serve.
