# Live measurements from the 5070 Ti machine (2026-09-02 09:16 local), read-only over SSH

Host: the 5070 Ti machine, x86_64, Linux Mint 22.3, 32 cores, 59,917 MB RAM, disk 1.2T used of 1.8T. Uptime 2 days. Load avg 1.97 / 9.10 / 18.36.
Node: /usr/bin/node v24.14.1 (system) plus nvm v24.18.0 and v24.19.0 — three Node installs (matches the screenshot's "Current Node differs from managed gateway service Node").

## Installed versions
- OpenClaw 2026.8.2 (npm global at ~/.local/lib/node_modules/openclaw); systemd unit description still says "OpenClaw Gateway (v2026.7.1-2)" — stale unit text.
- Hermes 0.20.1 (repo at ~/.hermes/hermes-agent); only the dashboard service is running (gateway not running at probe time).
- OpenCode 1.18.18 (~/.opencode).
- Ollama with local models: hf.co/unsloth/Qwen3.8-27B-GGUF (18 GB), gemma4:31b (19 GB), qwen:latest (18 GB), glm:latest, cloud aliases glm-5.3:cloud, kimi-k2.7-code:cloud, custom "yolo"/"qwopus" (16–17 GB).

## Disk footprint (du -sh, file counts via find -type f)
| Thing | Size | Files | Notes |
|---|---|---|---|
| OpenClaw npm package (~/.local/lib/node_modules/openclaw) | 898 MB | 35,979 | node_modules 672 MB (290 top-level pkgs + 48 scoped), dist 213 MB |
| OpenClaw state (~/.openclaw) | 5.7 GB | 52,583 | agents/ 3.8 GB, hr-infra-chrome/ 618 MB (Chrome profile), browser/ 358 MB, npm/ 355 MB (plugin runtime deps), memory/ 172 MB (main.sqlite.migrated), plugins/ 116 MB, media/ 107 MB, workspace/ 83 MB, state/ 38 MB (openclaw.sqlite 34 MB), cache 28 MB, archive 25 MB, session-sqlite-migration-runs 9.9 MB, logs 6.4 MB, backups 2.6 MB |
| OpenClaw extras | — | — | ~/.openclaw-openclaw/plugin-runtime-deps/openclaw-2026.4.26-…, ~/.openclaw-cleanup-backup, ~/.config/openclaw/chrome-openclaw, ~/.config/google-chrome/openclaw + OpenClaw profiles, ~/.ollama/backup/openclaw — leftovers from prior versions |
| Hermes home (~/.hermes) | 1.8 GB | 61,027 | hermes-agent/ 1.4 GB (repo + venv 678 MB with 247 site-packages entries), node/ 204 MB (bundled Node for TUI), bin/ 79 MB, lsp/ 71 MB, state.db 63 MB, skills/ 6.6 MB (15 skills), memories 4, logs 4.1 MB |
| OpenCode (~/.opencode + ~/.local/share/opencode + ~/.config/opencode) | 238 MB + 130 MB + 63 MB | 3,652 + 266 + 3,652 | — |

## Resident memory (ps rss), gateway up 26 min
| Process | RSS |
|---|---|
| OpenClaw gateway (node dist/index.js gateway --port 18789) | 504 MB |
| OpenClaw codex helper (~/.openclaw/npm/projects/openclaw-codex-…/@openai/codex) | 147 MB + 48 MB |
| signal-cli (java 25, ~/tool…) — OpenClaw's Signal transport | 288 MB |
| **OpenClaw total incl. Signal transport** | **~990 MB** (702 MB in 4 node procs + 288 MB JVM) |
| Hermes dashboard (python … hermes dashboard --host 127.0.0.1) | 88 MB (2 days up) |
| ollama serve (idle) | 39 MB |
| Signal Desktop app (user's own, not the agent) | 1,515 MB across 11 procs |

## Config facts (keys only, no secrets read)
- OpenClaw channels enabled: signal only. Primary model: openai/gpt-5.6-sol, no fallbacks.
- Hermes: dashboard "Alison" on 127.0.0.1:9119 via systemd user unit hermes-dashboard.service.
- systemd user units: hermes-dashboard.service (active), openclaw-gateway.service (active).
- Docker: 10 Supabase containers for homerecon (unrelated to agents).

## Other hosts
- the development machine, shotrecon: SSH refused for users jared/jt/JaredTate/pi (publickey). Not measured.
- chateautater (Pi 5 "The 7900 XT machine"): offline at probe time (last seen ~1 h before). Not measured.
