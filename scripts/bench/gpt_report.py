#!/usr/bin/env python3
"""Turn the GPT-5.6 Sol runs under ~/work/bench/gpt into the tables for
GPT_BENCHMARK.md, reading every number from each harness's own primary source:

- Coeus: its driver log (task in, answer out, and the cumulative counters its
  provider took from each `codex exec` result, differenced per call) and its
  record's per-turn cached counts (rounded to the hundred).
- opencode: the `step_finish` lines of its JSON output, one per model call.
- Hermes: the `sessions` row for the work folder in its fresh state.db.
- OpenClaw: the usage block of its JSON envelope (a run total, not per call).

Quality for all four: check-tater.mjs's verdict. Prices, when given on the
command line as --price IN CACHED OUT (dollars per million), are OpenAI's list
prices for the model; without them the cost columns are left blank.
"""
import glob
import json
import os
import re
import sqlite3
import sys

BASE = os.path.expanduser("~/work/bench/gpt")
HARNESSES = ["nerdgenie", "opencode", "hermes", "openclaw"]


def seconds(stamp):
    hours, minutes, secs = stamp.split(":")
    return int(hours) * 3600 + int(minutes) * 60 + int(secs)


def wall(folder):
    found = re.search(r"exit (\d+) launch_to_exit (\d+)", open(os.path.join(folder, "wall.txt")).read())
    return int(found.group(1)), int(found.group(2))


def checker(folder):
    try:
        verdict = json.load(open(os.path.join(folder, "check.json")))
    except (OSError, ValueError):
        return None
    thorough = verdict["thoroughness"]
    return {"logic": verdict["correctness"]["passed"], "plays": verdict["plays"]["passed"],
            "tests_passing": thorough["testsPassing"], "tests_total": thorough["testsTotal"],
            "lines": thorough["gameJsLines"], "quality": thorough["quality"]["passed"]}


def nerdgenie(folder):
    log = open(os.path.join(folder, "home", "drive.log")).read().splitlines()
    rows, previous = [], (0, 0)
    for line in log:
        found = re.search(r"tokensIn=(\d+) \| tokensOut=(\d+)", line)
        if not found:
            continue
        now = (int(found.group(1)), int(found.group(2)))
        if now == previous or now == (0, 0):
            continue
        rows.append({"in": now[0] - previous[0], "out": now[1] - previous[1]})
        previous = now
    cached = [0]
    db = sqlite3.connect(os.path.join(folder, "home", "nerdgenie.db"))
    for (body,) in db.execute("select body from events where kind='checkpoint' order by sequence"):
        text = body.decode() if isinstance(body, (bytes, bytearray)) else body
        found = re.search(r"this turn: ([\d.]+)k tokens in, ([\d.]+)k of them cached", text)
        if found and float(found.group(1)) > 0:
            cached.append(int(float(found.group(2)) * 1000))
    calls = []
    for index, row in enumerate(rows):
        read = min(cached[index] if index < len(cached) else 0, row["in"])
        calls.append({"in": row["in"], "cached": read, "out": row["out"], "reasoning": None})
    sent = next(seconds(l[:8]) for l in log if '>> {"type": "message"' in l)
    replied = next((seconds(l[:8]) for l in log if "<< REPLY #1" in l), None)
    tools = len([l for l in log if re.search(r"toolLine=▸.* · r\d+", l)])
    done = next((l for l in log if "== done in" in l), "")
    previews = re.search(r"previews=(\d+)", done)
    notes = [f"driver approved {previews.group(1)} preview(s)"] if previews and previews.group(1) != "0" else []
    notes.append("cache split from the record's per-turn line, rounded to the hundred")
    return calls, (replied - sent) if replied is not None else None, tools, notes


def opencode(folder):
    calls, tools = [], 0
    for line in open(os.path.join(folder, "out.jsonl"), errors="replace"):
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if event.get("type") == "tool_use":
            tools += 1
        if event.get("type") == "step_finish":
            tokens = event["part"]["tokens"]
            calls.append({"in": tokens["input"] + tokens["cache"]["read"] + tokens["cache"]["write"],
                          "cached": tokens["cache"]["read"], "out": tokens["output"], "reasoning": tokens.get("reasoning", 0)})
    return calls, None, tools, ["per call from its step_finish lines"]


def hermes(folder):
    db = sqlite3.connect(os.path.join(folder, "home", "state.db"))
    work = os.path.join(folder, "work")
    row = db.execute("select api_call_count, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens from sessions where cwd = ? order by started_at desc limit 1", (work,)).fetchone()
    if row is None:
        return [], None, 0, ["no sessions row for the work folder"]
    count, fresh, out, read, write, reasoning = [x or 0 for x in row]
    text = open(os.path.join(folder, "out.log"), errors="replace").read()
    found_tools = re.search(r"Messages:\s+\d+ \(\d+ user, (\d+) tool calls\)", text)
    tools = int(found_tools.group(1)) if found_tools else 0
    return [{"in": fresh + read + write, "cached": read, "out": out, "reasoning": reasoning, "count": count}], None, tools, ["a run total from its sessions row; Hermes counts fresh input and cached input apart"]


def openclaw(folder):
    """OpenClaw reports a run total in its JSON envelope only when its own
    embedded loop ran. When it handed the turn to the codex program (its
    default on a ChatGPT subscription), the exact per-call usage is in the
    codex rollout file inside a kept state folder, and the envelope holds
    only the last turn."""
    envelope = json.load(open(os.path.join(folder, "out.json")))
    summary = envelope.get("toolSummary") or {}
    rollouts = glob.glob(os.path.join(folder, "state", "**", "rollout-*.jsonl"), recursive=True)
    if rollouts:
        calls, tools = [], []
        for line in open(rollouts[0], errors="replace"):
            try:
                event = json.loads(line)
            except ValueError:
                continue
            payload = event.get("payload") or {}
            if not isinstance(payload, dict):
                continue
            if payload.get("type") in ("function_call", "custom_tool_call", "local_shell_call"):
                tools.append(payload.get("name") or payload.get("type"))
            usage = (payload.get("info") or {}).get("last_token_usage") if payload.get("type") == "token_count" else None
            if usage:
                calls.append({"in": usage.get("input_tokens", 0), "cached": usage.get("cached_input_tokens", 0),
                              "out": usage.get("output_tokens", 0), "reasoning": usage.get("reasoning_output_tokens", 0)})
        return calls, None, len(tools), ["ran through the codex program (OpenClaw's codex runtime); per call from the codex rollout it left in the kept state folder; tools were codex's: " + ", ".join(sorted(set(tools)))]
    usage = envelope.get("usage") or {}
    turns = envelope.get("assistantTurns")
    note = "OpenClaw's own loop; a run total from its JSON envelope" if turns else "envelope holds the last turn only (no assistantTurns), so tokens are not a run total"
    return ([{"in": usage.get("input", 0) + usage.get("cacheRead", 0) + usage.get("cacheWrite", 0),
              "cached": usage.get("cacheRead", 0), "out": usage.get("output", 0), "reasoning": usage.get("reasoningTokens"), "count": turns}],
            None, summary.get("calls", 0), [note + f"; status {envelope.get('status')}; tools " + ", ".join(summary.get("tools") or [])])


def load(harness, number, price):
    folder = os.path.join(BASE, f"{harness}-{number}")
    if not os.path.exists(os.path.join(folder, "wall.txt")):
        return None
    exit_status, launch_to_exit = wall(folder)
    calls, task_to_answer, tools, notes = globals()[harness](folder)
    total_in = sum(c["in"] for c in calls)
    cached = sum(c["cached"] for c in calls)
    out = sum(c["out"] for c in calls)
    reasoning = None if any(c["reasoning"] is None for c in calls) else sum(c["reasoning"] for c in calls)
    count = calls[0]["count"] if calls and "count" in calls[0] else len(calls)
    cost = None
    if price:
        fresh_price, cached_price, out_price = price
        cost = ((total_in - cached) * fresh_price + cached * cached_price + out * out_price) / 1e6
    return {"harness": harness, "number": number, "exit": exit_status, "launch_to_exit": launch_to_exit,
            "task_to_answer": task_to_answer if task_to_answer is not None else launch_to_exit,
            "calls": count, "tools": tools, "in": total_in, "cached": cached, "out": out, "reasoning": reasoning,
            "cost": cost, "check": checker(folder), "notes": notes}


def fmt_time(value):
    return f"{value // 60} min {value % 60} s" if value >= 60 else f"{value} s"


def green(run):
    c = run["check"]
    return bool(c) and c["logic"] == 6 and c["plays"] == 4 and c["tests_passing"] == c["tests_total"] and c["tests_total"] >= 6


def main():
    price = None
    if "--price" in sys.argv:
        at = sys.argv.index("--price")
        price = tuple(float(x) for x in sys.argv[at + 1:at + 4])
    runs = [r for n in (1, 2, 3, "own-1", "own-2", "own-3") for h in HARNESSES if (r := load(h, n, price))]
    print("## Every run\n")
    print("| Harness | Run | Green | Task in to answer out | Model calls | Tool calls | Tokens in | of which cached | Tokens out | of which reasoning | Cost | Logic | Plays | Tests | game.js |")
    print("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    for r in runs:
        c = r["check"] or {}
        tests = f"{c.get('tests_passing', 0)}/{c.get('tests_total', 0)}" if c else "no verdict"
        reasoning = "not counted" if r["reasoning"] is None else f"{r['reasoning']:,}"
        cost = "" if r["cost"] is None else f"${r['cost']:.2f}"
        print(f"| {r['harness']} | {r['number']} | {'yes' if green(r) else 'NO'} | {fmt_time(r['task_to_answer'])} | {r['calls']} | {r['tools']} | {r['in']:,} | {r['cached']:,} | {r['out']:,} | {reasoning} | {cost} | {c.get('logic', '-')}/6 | {c.get('plays', '-')}/4 | {tests} | {c.get('lines', '-')} |")
    print("\n## Notes per run\n")
    for r in runs:
        print(f"- {r['harness']} {r['number']}: exit {r['exit']}, launch to exit {fmt_time(r['launch_to_exit'])}; " + "; ".join(r["notes"]))


if __name__ == "__main__":
    sys.exit(main())
