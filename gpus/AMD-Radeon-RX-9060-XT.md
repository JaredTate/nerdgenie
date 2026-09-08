# AMD Radeon RX 9060 XT — Nerd Genie GPU profile

A tested, copy-it-and-go configuration for running Nerd Genie's local model on the
**AMD Radeon RX 9060 XT** (gfx1200, RDNA4, 16 GB). This is the measured answer to
the "RDNA4 16 GB — to be measured" row in `INSTALL.md` section 7. Read `INSTALL.md`
and `SETUP.md` first for the general install; this page only records what is
different or exact for this card. Every number here was read off a running machine,
not from memory.

**One-line summary:** use the **ROCm/HIP** TurboQuant build (not Vulkan — it crashes
on this card), the **Q3_K_P** quant with the vision projector, **thinking off at the
server**, MTP draft depth 3, and a **65,536-token** context. Measured **~38 tokens a
second** on code with vision on, fitting in ~14.6 GiB of the card's 15.9 GiB.

---

## 1. The machine this was measured on

| part | value |
|---|---|
| Host | `jared-sassy` |
| GPUs | **2× AMD Radeon RX 9060 XT**, gfx1200 (RDNA4), SKU 44TC6SHB1 |
| VRAM per card | 17,095,983,104 bytes = **16,304 MiB (15.92 GiB)** |
| Card roles | GPU0/ROCm0 drives the display; **GPU1/ROCm1 is free** (use it for the model) |
| CPU | Intel Core i7-8700K |
| System RAM | 62 GiB |
| OS | Ubuntu 24.04.4 LTS, kernel 6.17.0-35-generic |
| Compute stack | **ROCm 7.2.1** (`/opt/rocm`), which supports gfx1200 natively |
| Go | 1.27.1 · Node | 24.18.0 · aria2 | 1.37.0 · Chrome | `/opt/google/chrome/chrome` |

The two cards are ~320 GB/s each. That memory bandwidth is the speed ceiling: a 27B
model at ~3-4 bits reads ~13 GB per token, so a single card tops out near ~25 tok/s
of raw decode. MTP speculative decoding is what beats that ceiling (see section 7).

---

## 2. The one big difference: use the ROCm build, NOT Vulkan

`INSTALL.md` tells AMD users to take the **Vulkan** TurboQuant build. **Do not do that
on gfx1200/RDNA4.** The Vulkan build device-loses and crashes the server:

```
terminate called after throwing an instance of 'vk::DeviceLostError'
  what():  vk::Queue::submit: ErrorDeviceLost
```

The crash happens in `ggml_backend_vk_buffer_get_tensor` while reading a **turbo3 KV
checkpoint** back to the host (Mesa RADV is fragile for this on a brand-new card).
The **ROCm/HIP build is stable** here and has the same turbo3 + MTP features. Take the
ROCm asset:

```sh
mkdir -p ~/llm/turboquant
cd ~/llm/turboquant
curl -L -o tq-rocm.tar.gz \
  https://github.com/AtomicBot-ai/atomic-llama-cpp-turboquant/releases/download/b10269-1.5.1/llama-turboquant-linux-x64-rocm.tar.gz
mkdir -p rocm && tar xzf tq-rocm.tar.gz -C rocm
# binary lands at ~/llm/turboquant/rocm/build/bin/llama-server (with libggml-hip.so)
```

Build in use: TurboQuant `b10269-1.5.1` (build 10696, commit cd5609390). It enumerates
the cards as `ROCm0` and `ROCm1` (`llama-server --list-devices`), so the launcher uses
`--device ROCm1`, not `--device Vulkan1`.

---

## 3. The model files

Put these in `~/llm/models/`. On a 16 GB card the 18 GB `Q4_K_P` will **not** fit with
vision — use **Q3_K_P** (a HauhauCS "K_P Perfect" quant that is both higher quality and
faster than `IQ3_M`, for +0.65 GB):

| file | size | sha256 (first bytes) | note |
|---|---|---|---|
| `hauhau-Q3_K_P.gguf` | 13.44 GB | `582b8081…886ba7a2b` | **the model in use** |
| `mmproj-Qwen3.8-27B-F16.gguf` | 0.93 GB | — | vision projector |
| `hauhau-IQ3_M.gguf` | 12.79 GB | — | slightly lower quality/speed; good fallback |
| `hauhau-IQ3_XS.gguf` | 12.18 GB | — | a touch smaller/faster, lower quality |

Both come from `https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF`.
(The GGUF's `general.name` is `Qwen3.8-27B-Uncensored-HauhauCS-Aggressive`; its arch tag
is `qwen35`, a hybrid of 48 Gated-DeltaNet + 16 attention layers — which is why context
is cheap on VRAM here.)

**Download with `aria2c`, not `curl`.** Hugging Face throttles a single connection to
~1 MB/s on this network (a ~2.6-hour download); 16 connections hit ~86 MB/s (~2 minutes):

```sh
sudo apt-get install -y aria2
cd ~/llm/models
aria2c -c -x16 -s16 -k1M -o hauhau-Q3_K_P.gguf \
  "https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF/resolve/main/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-Q3_K_P.gguf"
aria2c -c -x16 -s16 -k1M -o mmproj-Qwen3.8-27B-F16.gguf \
  "https://huggingface.co/HauhauCS/Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-MTP-GGUF/resolve/main/mmproj-Qwen3.8-27B-Uncensored-HauhauCS-Aggressive-BF16.gguf"
sha256sum hauhau-Q3_K_P.gguf   # expect 582b80812df428af61cd77891c703b4f0ac388fc22759a159a6c408886ba7a2b
```

---

## 4. The server launcher (`~/llm/igo.sh`)

The exact working script for this card. It picks the backend from the device name, so
`ROCm1` uses the HIP build. Start agent A on the free card:

```sh
setsid ~/llm/igo.sh ROCm1 19091 65536 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &
```

```bash
#!/usr/bin/env bash
# Start the TurboQuant llama-server for Nerd Genie on one AMD card.
# Usage: igo.sh <device> <port> <ctx> <mtp|nomtp>   e.g. igo.sh ROCm1 19091 65536 mtp
set -uo pipefail
DEV="${1:-ROCm1}"; PORT="${2:-19091}"; CTX="${3:-65536}"; MTP="${4:-mtp}"
case "$DEV" in
  ROCm*)   TQ="$HOME/llm/turboquant/rocm/build/bin";   export LD_LIBRARY_PATH="$TQ:/opt/rocm/lib:${LD_LIBRARY_PATH:-}" ;;
  Vulkan*) TQ="$HOME/llm/turboquant/vulkan/build/bin"; export LD_LIBRARY_PATH="$TQ:${LD_LIBRARY_PATH:-}" ;;
  *) echo "unknown device '$DEV' (use ROCmN or VulkanN)"; exit 2 ;;
esac
MODEL="${MODEL:-$HOME/llm/models/hauhau-Q3_K_P.gguf}"
MMPROJ="${MMPROJ:-$HOME/llm/models/mmproj-Qwen3.8-27B-F16.gguf}"
export TURBO_AUTO_ASYMMETRIC=0                       # stop a silent KV upgrade; worth ~2 GB
args=(
  --model "$MODEL" --alias local-coder
  --host 127.0.0.1 --port "$PORT"
  --device "$DEV" --n-gpu-layers 999 --ctx-size "$CTX"
  --cache-type-k turbo3 --cache-type-v turbo3        # ~5x KV compression
  --flash-attn on -kvu                               # turbo3 needs a unified flash-attn KV cache
  --checkpoint-min-step 1024 --ctx-checkpoints 16
  --temp 0.7 --top-p 0.8 --top-k 20 --presence-penalty 1.5   # qwen non-thinking sampler
  --reasoning-budget 0
  --chat-template-kwargs '{"enable_thinking": false}'   # THIS is what turns thinking off (see section 5)
  --batch-size 2048 --ubatch-size 512 --no-mmap
  --parallel 1
)
[ "$MTP" = "mtp" ] && args+=( --spec-type draft-mtp --spec-draft-n-max 3 --spec-draft-p-min 0 )  # depth 3
[ "$MMPROJ" != "none" ] && args+=( --mmproj "$MMPROJ" )
exec "$TQ/llama-server" "${args[@]}"
```

Then verify, every start (a server that fell into system RAM looks alive but is 20x slower):

```sh
curl -s http://127.0.0.1:19091/health          # {"status":"ok"}
curl -s http://127.0.0.1:19091/props | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d["modalities"],d["default_generation_settings"]["n_ctx"])'
rocm-smi --showmeminfo vram | grep -i used     # GPU1 should read ~14.6 GiB, well under 15.9
```

---

## 5. Thinking off — the gotcha

Qwen 3.8 runs better as an agent with its hidden reasoning **off**, and it is a must on
this card (low/high reasoning burns the token budget on `<think>` and can return empty).
Two things that do **not** work here:

- `--reasoning-budget 0` — **this model ignores it** (it still thinks).
- Nerd Genie's own thinking-off hint did not take effect against this exact build.

What works is telling the chat template directly, at the server, for every request:

```
--chat-template-kwargs '{"enable_thinking": false}'
```

Verify: a trivial ask should come back in ~3 tokens with an empty `reasoning_content`.
Keep `think = "off"` in `config.toml` too, so the two agree.

---

## 6. Nerd Genie config (`~/.nerdgenie/config.toml`)

```toml
default_model = "local"
yolo = true
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

Set it up with: `bin/nerdgenie init -yes -model local -work-folder ~/work -signal off`,
then edit `context_length` to `65536`, add `vision = true`, and set `think = "off"`.

---

## 7. Measured performance (Q3_K_P, vision on, ctx 65,536)

| metric | value |
|---|---|
| Output (generation) speed, code | **~38.7 tok/s** |
| Prefill (prompt read) speed | ~500+ tok/s (short: ~100; large cached prompt: 500+) |
| MTP draft acceptance | **93–95%** on code with thinking off |
| On-card VRAM used | 15,658,958,848 bytes = **14.58 GiB** of 15.92 GiB (~1.3 GiB free) |
| Server process RAM (RSS) | under 1 GB (confirms the model is on the card) |

For comparison on this card: `IQ3_M` measured ~35 tok/s. Thinking **off** is both a speed
lever (predictable output → high MTP acceptance) and a correctness one. Sustained 40+
tok/s would need a smaller quant or a dual-card tensor split (~2× bandwidth).

---

## 8. Two agents, one per card (A and B)

Each card runs one server on its own port; nothing is shared. Start them one at a time:

```sh
setsid ~/llm/igo.sh ROCm1 19091 65536 mtp > ~/llm/logs/19091.log 2>&1 < /dev/null &   # card A (free card)
# wait for /health = ok, then:
setsid ~/llm/igo.sh ROCm0 19093 65536 mtp > ~/llm/logs/19093.log 2>&1 < /dev/null &   # card B (display card)
```

Give each its own Nerd Genie home (`NERDGENIE_HOME=~/nerdgenie-b nerdgenie serve`), its
own `base_address` (`:19093`), its own work folder and Chrome profile. Card B shares the
display card, so leave it more headroom if the desktop is busy.

---

## 9. Rules that are not optional (this card)

- **ROCm build, not Vulkan** — Vulkan device-loses on gfx1200.
- **`--chat-template-kwargs '{"enable_thinking": false}'`** is the real thinking-off switch.
- **`aria2c -x16`** for Hugging Face downloads — single connections are throttled.
- **Q4_K_P (18 GB) does not fit** one 16 GB card with vision; use Q3_K_P or smaller.
- Never kill a server by name pattern (`pkill -f`, `killall`, `pgrep -af`); kill exact pids by port.
- Never start a second server on a card that already has one; never `ollama pull`.
- Keep port 8090 free (reserved on this machine).

---

## 10. Verify checklist

1. `~/llm/turboquant/rocm/build/bin/llama-server --list-devices` shows `ROCm0` and `ROCm1`.
2. `curl -s 127.0.0.1:19091/health` → `{"status":"ok"}`.
3. `/props` shows `vision: true` and `n_ctx: 65536`, model `hauhau-Q3_K_P.gguf`.
4. `rocm-smi` shows ~14.6 GiB used on the model's card, under the 15.9 GiB total.
5. A trivial ask returns ~3 tokens with empty `reasoning_content` (thinking is off).
6. A code ask logs `draft acceptance ~0.9+` and ~38 tok/s in `~/llm/logs/19091.log`.
