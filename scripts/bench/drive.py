#!/usr/bin/env python3
"""Drive the Coeus serve over its socket with one prompt and log every envelope.

Usage: drive.py SOCKET PROMPT_FILE LOG [TIMEOUT_MINUTES]

The driver stands in for the person at the terminal during a benchmark run. It
attaches, sends the prompt, and answers what the program asks for: a preview is
approved unless it looks destructive or spends money, a plain question gets a
"use your judgment" answer, a masked prompt is cancelled, and a handoff is
declined. When the program says the task's budget is used up, the driver says
"continue", as a person would. It leaves as soon as the program has replied and
gone idle, so the wall time measured around it is the program's own.
"""
import datetime
import json
import re
import select
import socket
import sys
import time

SOCK, PROMPT_FILE, LOG = sys.argv[1], sys.argv[2], sys.argv[3]
# Zero minutes, which is what a missing argument means, is no timeout at all:
# the driver waits for as long as the program takes.
TIMEOUT_MIN = float(sys.argv[4]) if len(sys.argv) > 4 else 0.0
# How long the program must stay idle after its reply before the driver leaves.
QUIET_SECONDS = 5.0

DANGER = re.compile(
    r"rm\s+(-\w*r|--recursive)|\bsudo\b|\bdoas\b|mkfs|\bdd\s+if=|shutdown|reboot"
    r"|>\s*/dev/sd|chmod\s+-R\s+777|curl[^|]*\|\s*(ba)?sh|\bpay\b|purchase|checkout",
    re.I,
)

log = open(LOG, "a", buffering=1)


def now():
    return datetime.datetime.now().strftime("%H:%M:%S")


def out(line):
    log.write(f"{now()} {line}\n")


sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect(SOCK)
sock.setblocking(False)


def send(obj):
    data = (json.dumps(obj) + "\n").encode()
    sock.setblocking(True)
    sock.sendall(data)
    sock.setblocking(False)
    out(">> " + json.dumps(obj)[:200])


send({"type": "attach"})
send({"type": "message", "text": open(PROMPT_FILE).read()})

start = time.time()
last = time.time()
buffer = b""
deltas = []
laststatus = {}
counts = {"replies": 0, "previews": 0, "asks": 0, "errors": 0, "toolLines": 0, "recordLines": 0}


def flush_deltas():
    global deltas
    if deltas:
        joined = "".join(deltas)
        out(f"   [streamed {len(joined)} chars] " + joined[-700:].replace("\n", " ⏎ "))
        deltas = []


def handle(envelope):
    kind = envelope.get("type")
    ident = envelope.get("id")
    text = envelope.get("text", "") or ""
    if kind == "delta":
        deltas.append(text)
        return
    flush_deltas()
    if kind == "reply":
        counts["replies"] += 1
        out(f"<< REPLY #{counts['replies']} ({len(text)} chars) task={envelope.get('taskId')}: {text[:2000]}")
        if envelope.get("attachments"):
            out(f"   attachments: {envelope.get('attachments')}")
        lowered = text.lower()
        if "budget" in lowered and ("used up" in lowered or "ran out" in lowered or "spent" in lowered) and counts.get("continues", 0) < 8:
            counts["continues"] = counts.get("continues", 0) + 1
            out(f"   the budget ran out; saying continue ({counts['continues']} of 8)")
            send({"type": "message", "text": "continue"})
    elif kind == "preview":
        counts["previews"] += 1
        out(f"<< PREVIEW {ident} [{envelope.get('title')}]: {text[:400].replace(chr(10), ' ⏎ ')}")
        if DANGER.search(envelope.get("title", "") + "\n" + text):
            send({"type": "deny", "id": ident, "reason": "Not while unattended: that looks destructive or spends money. Find another way."})
        else:
            send({"type": "approve", "id": ident})
    elif kind == "ask":
        counts["asks"] += 1
        out(f"<< ASK {ident} masked={envelope.get('maskInput')} [{envelope.get('title')}]: {text[:400]}")
        if envelope.get("maskInput"):
            send({"type": "cancel", "id": ident})
        else:
            send({"type": "message", "id": ident, "text": "I am not at the terminal. Use your best judgment, pick the simplest option, and continue."})
    elif kind == "handoff":
        out(f"<< HANDOFF {ident}: {text[:300]} {envelope.get('attachments')}")
        send({"type": "message", "id": ident, "text": "I cannot take over the browser right now. Try another way and continue."})
    elif kind == "status":
        fields = envelope.get("fields", {}) or {}
        changed = {k: v for k, v in fields.items() if laststatus.get(k) != v and k not in ("commands", "healthy", "callStarted", "streamed")}
        if changed:
            if "toolLine" in changed and changed["toolLine"]:
                counts["toolLines"] += 1
            if "recordLine" in changed and changed["recordLine"]:
                counts["recordLines"] += 1
            out("<< status " + " | ".join(f"{k}={str(v)[:120]}" for k, v in changed.items()))
        laststatus.update(fields)
    elif kind == "error":
        counts["errors"] += 1
        out(f"<< ERROR: {text[:500]}")
    else:
        out(f"<< {kind}: {json.dumps(envelope)[:300]}")


while True:
    if TIMEOUT_MIN > 0 and time.time() - start > TIMEOUT_MIN * 60:
        out("!! overall timeout reached")
        break
    if counts["replies"] and time.time() - last > QUIET_SECONDS and laststatus.get("state") == "idle":
        out("== the program replied and is idle; done")
        break
    ready, _, _ = select.select([sock], [], [], 1.0)
    if not ready:
        continue
    chunk = sock.recv(65536)
    if not chunk:
        out("!! socket closed by the program")
        break
    last = time.time()
    buffer += chunk
    while b"\n" in buffer:
        line, buffer = buffer.split(b"\n", 1)
        if not line.strip():
            continue
        try:
            handle(json.loads(line))
        except Exception as trouble:  # noqa: BLE001
            out(f"?? could not handle line ({trouble}): {line[:200]!r}")

flush_deltas()
out(f"== done in {(time.time() - start) / 60:.1f} min: " + " ".join(f"{k}={v}" for k, v in counts.items()))
