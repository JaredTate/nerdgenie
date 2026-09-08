# NVIDIA GeForce RTX 5070 Ti — Nerd Genie GPU profile

A tested, copy-it-and-go configuration for running Nerd Genie's local model on the
**NVIDIA GeForce RTX 5070 Ti** (Blackwell, 16 GB, CUDA). This is the measured answer
to the "RTX 5070 Ti 16 GB (CUDA)" row in `INSTALL.md` section 7, and it upgrades that
row's model from `IQ3_M` to **`Q3_K_P`**. Read `INSTALL.md` and `SETUP.md` first for the
general install; this page records only what is different or exact for this card.
Every number here was read off the running machine, not from memory.

**One-line summary:** use the **CUDA** TurboQuant build (not Vulkan — turbo3 is broken on
NVIDIA's Vulkan), the **Q3_K_P** quant with the vision projector, **thinking off** (both
the chat-template switch and the harness default), MTP draft depth **2**, turbo3 KV, and
a **65,536-token** context. Measured **~98 tokens a second** on a shallow code prompt
with vision on, the model fitting in **15.79 GiB of the card's 15.92 GiB**.

---

## 1. The machine this was measured on

| part | value |
|---|---|
| Host | `jared-rosie` |
| GPUs | **2× NVIDIA GeForce RTX 5070 Ti** (Blackwell), 16 GB each |
| VRAM per card | 16,303 MiB (15.92 GiB) |
| Card roles | **GPU0/CUDA0 runs the model** (busy); GPU1/CUDA1 is free (second agent, or headroom) |
| CPU | AMD Ryzen 9 9950X (16-core) |
| System RAM | 58 GiB |
| OS | Linux Mint 22.3, kernel 6.17.0-42-generic |
| Compute stack | **CUDA 13.2**, driver 595.84 |
| Go 1.27.1 · Node 24.18.0 · aria2 1.37.0 |

The 5070 Ti's memory bandwidth (~900 GB/s) is far higher than the RDNA4 cards' ~320 GB/s,
which is why raw decode is fast here; **MTP speculative decoding** then pushes a shallow
code prompt to ~98 tok/s (see section 7).

---

## 2. The one big difference: use the CUDA build, NOT Vulkan

`INSTALL.md` tells AMD users to take the Vulkan TurboQuant build. On NVIDIA, **take the
CUDA build** — `INSTALL.md` section 7 notes turbo3 is broken on NVIDIA's Vulkan. The CUDA
build is stable here and keeps the turbo3 + MTP features. It lives at
`~/llm/turboquant-cuda/` and enumerates the cards as `CUDA0` and `CUDA1`, so the launcher
uses `--device CUDA0`.

Build in use: TurboQuant **`b10269-1.5.1`** (build 10696, commit cd5609390), CUDA
(`libcublas`/`libcudart` 13.x).

---

## 3. The model files

Put these in `~/llm/models/`. On a 16 GB card the 18 GB `Q4_K_P` will **not** fit with
vision — use **Q3_K_P**, a HauhauCS "K_P Perfect" quant that is both higher quality and
faster than `IQ3_M`, for +0.65 GB. It fits on this card with vision on, ~0.13 GiB to spare.

| file | size | sha256 | note |
|---|---|---|---|
| `hauhau-Q3_K_P.gguf` | 13.44 GB | `582b80812df428af61cd77891c703b4f0ac388fc22759a159a6c408886ba7a2b` | **the model in use** |
| `mmproj-Qwen3.8-27B-F16.gguf` | 0.93 GB | — | vision projector |
| `hauhau-IQ3_M.gguf` | 12.79 GB | — | the previous model here; slightly lower quality; good fallback |

Both come from `https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF`.

**Download with `aria2c`, not `curl`.** Hugging Face throttles a single connection; use 16:

```sh
sudo apt-get install -y aria2
cd ~/llm/models
aria2c -c -x16 -s16 -k1M -o hauhau-Q3_K_P.gguf \
  "https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF/resolve/main/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-Q3_K_P.gguf"
sha256sum hauhau-Q3_K_P.gguf   # expect 582b80812df428af61cd77891c703b4f0ac388fc22759a159a6c408886ba7a2b
```

The throttle varies by network and time of day: we measured ~10 MiB/s on one run
(~20 min) and the 9060 XT profile measured ~86 MB/s (~2 min). Do not kill and restart
aria2c to "unstick" a slow run — `-c` resumes fine, but a mid-flight restart can reset
progress. Let it finish.

---

## 4. The server launcher (`~/llm/ngo.sh`)

The exact working script for this card. It wraps the machine's `go2.sh` settings (the CUDA
build, turbo3, MTP depth 2, baked sampling, thinking off, `TURBO_AUTO_ASYMMETRIC=0`) and
adds the vision projector and the context checkpoints Nerd Genie needs. Start on CUDA0:

```sh
setsid ~/llm/ngo.sh 0 19091 65536 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &
```

The effective `llama-server` invocation it runs (model swapped to Q3_K_P):

```sh
export LD_LIBRARY_PATH=$HOME/llm/turboquant-cuda TURBO_AUTO_ASYMMETRIC=0
$HOME/llm/turboquant-cuda/llama-server --no-webui --jinja \
  -m $HOME/llm/models/hauhau-Q3_K_P.gguf \
  --spec-type draft-mtp --spec-draft-n-max 2 --spec-draft-ngl 99 --spec-draft-p-min 0 \
  --cache-type-k-draft turbo3 --cache-type-v-draft turbo3 \
  --mmproj $HOME/llm/models/mmproj-Qwen3.8-27B-F16.gguf --image-min-tokens 560 --image-max-tokens 560 \
  --port 19091 --host 127.0.0.1 -ngl 99 -fa on --split-mode none \
  --cache-type-k turbo3 --cache-type-v turbo3 -kvu --parallel 1 \
  --batch-size 2048 --ubatch-size 256 \
  --checkpoint-min-step 1024 --ctx-checkpoints 16 \
  --chat-template-kwargs '{"enable_thinking": false}' \
  --temp 0.7 --top-p 0.8 --top-k 20 --presence-penalty 1.5 \
  -a local-coder --ctx-size 65536 --device CUDA0 --reasoning-budget 0
```

Note the differences from the 9060 XT profile: **MTP draft depth 2** (not 3), `ubatch 256`
(not 512), and the CUDA build. `--split-mode none` keeps the whole model on one card.

Verify, every start (a server that fell into system RAM looks alive but is 20x slower):

```sh
curl -s http://127.0.0.1:19091/health          # {"status":"ok"}
curl -s http://127.0.0.1:19091/props | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["modalities"],d["default_generation_settings"]["n_ctx"])'
nvidia-smi --query-gpu=index,memory.used --format=csv,noheader   # CUDA0 ~15.8 GiB, under 15.92
```

---

## 5. Thinking off

Qwen 3.8 runs better as an agent with its hidden reasoning off. On this card it is off
three ways, and they agree:

- **`--chat-template-kwargs '{"enable_thinking": false}'`** at the server — the switch that
  actually holds (this is what the 9060 XT profile found is the real lever).
- **`--reasoning-budget 0`** at the server — kept as a second gate; harmless here.
- **The harness default** (commit `52d0ab8d`): Nerd Genie sends `enable_thinking:false` to a
  loopback daemon from the base address alone, and `think = "off"` in `config.toml`.

Verify: a trivial ask returns with an empty `reasoning_content`. We confirmed this on the
running server (see section 7).

---

## 6. Nerd Genie config (`~/.nerdgenie/config.toml`)

```toml
default_model = "local"
# yolo defaults on at serve start as of commit 52d0ab8d; set yolo = false to turn asking back on.
sandbox = "off"

[[models]]
name = "local"
provider = "openai"
base_address = "http://127.0.0.1:19091/v1"
model_name = "local-coder"
context_length = 65536      # must match the server's --ctx-size
vision = true               # the projector is loaded, so pictures reach the model
think = "off"               # off for this model
```

The model file is chosen in `~/llm/ngo.sh` (the `-m` path), not in `config.toml`. To swap
the model, edit that one line and restart the server; `config.toml` is unchanged as long as
the context length and alias stay the same.

---

## 7. Measured performance (Q3_K_P, vision on, ctx 65,536)

| metric | value |
|---|---|
| Output (generation) speed, shallow code prompt | **~97.9 tok/s** |
| Prefill (prompt read) speed, short prompt | ~133 tok/s |
| On-card VRAM used | 15,792 MiB = **15.42 GiB** of 15.92 GiB (~0.13 GiB free) |
| Server process RAM (RSS) | ~1.25 GB (confirms the model is on the card) |
| Thinking | off — `reasoning_content` came back empty |

For comparison, the 9060 XT profile measured Q3_K_P at ~38.7 tok/s (bandwidth-bound at
~320 GB/s); the 5070 Ti's ~900 GB/s plus MTP is why it is much faster here. **Caveat:** the
~98 tok/s figure is a shallow prompt; like every card, output slows at deep context (the
9060 XT saw ~30 tok/s at 85K), and VRAM headroom here is thin (~0.13 GiB), so keep the
context at 65,536 with vision — `INSTALL.md` notes 128k loads and then dies mid-job on this
card. Turn vision off to run a longer context (~114,688).

---

## 8. Two agents, one per card (A and B)

Each card runs one server on its own port; nothing is shared. CUDA0 runs the model above;
start a second on the free CUDA1 for a second agent:

```sh
setsid ~/llm/ngo.sh 0 19091 65536 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &   # card A
# wait for /health = ok, then:
setsid ~/llm/ngo.sh 1 19093 65536 mtp > ~/llm/logs/19093.log 2>&1 < /dev/null &   # card B
```

Give each its own Nerd Genie home (`NERDGENIE_HOME=~/nerdgenie-b nerdgenie serve`), its own
`base_address` (`:19093`), its own work folder and Chrome profile.

---

## 9. Rules that are not optional (this card)

- **CUDA build, not Vulkan** — turbo3 is broken on NVIDIA's Vulkan.
- **Keep the context at 65,536 with vision** — VRAM headroom is only ~0.13 GiB; 128k dies mid-job.
- **`aria2c -x16`** for Hugging Face downloads, and do not kill/restart it to unstick a slow run.
- Never kill a server by name pattern (`pkill -f`, `killall`, `pgrep -af`); kill exact pids by port.
- Never start a second server on a card that already has one; never `ollama pull`.
- Keep port 8090 free (reserved on this machine).

---

## 10. Verify checklist

1. `~/llm/turboquant-cuda/llama-server --list-devices` shows `CUDA0` and `CUDA1`.
2. `curl -s 127.0.0.1:19091/health` → `{"status":"ok"}`.
3. `/props` shows `vision: true`, `n_ctx: 65536`, model `hauhau-Q3_K_P.gguf`.
4. `nvidia-smi` shows ~15.8 GiB used on CUDA0, under the 16,303 MiB total; server RSS under ~1.5 GB.
5. A trivial ask returns with empty `reasoning_content` (thinking is off).
6. A code ask shows high MTP draft acceptance and ~90+ tok/s shallow in `~/llm/logs/19091.log`.
