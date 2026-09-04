#!/usr/bin/env python3
"""Turn the nine clean Opus 4.8 runs under ~/work/bench/opus3 into the tables
for OPUS_BENCHMARK.md, reading every number from its primary source.

Coeus: its driver log (task in, answer out, and the cumulative counters that
Coeus's own provider took from each `claude -p` result line, differenced per
call, with the cache split taken from Claude Code's own bill per call). OpenClaw and Hermes: the Claude
Code session file each run left behind, one row per unique model call. Quality
for all three: check-tater.mjs's verdict. Prices are Anthropic's list prices
per million tokens: fresh input 5, one-hour cache write 10, cache read 0.50,
output 25.
"""
import glob
import json
import os
import re
import sys

BASE = os.path.expanduser("~/work/bench/opus3")
PROJECTS = os.path.expanduser("~/.claude/projects")
PRICE_FRESH, PRICE_WRITE, PRICE_READ, PRICE_OUT = 5.0, 10.0, 0.5, 25.0
HARNESSES = ["nerdgenie", "openclaw", "hermes"]


def cost(fresh, write, read, out):
    return (fresh * PRICE_FRESH + write * PRICE_WRITE + read * PRICE_READ + out * PRICE_OUT) / 1e6


def seconds(stamp):
    hours, minutes, secs = stamp.split(":")
    return int(hours) * 3600 + int(minutes) * 60 + int(secs)


def session_calls(work_folder):
    """Every unique model call in the Claude Code session left in work_folder."""
    key = "-" + work_folder.strip("/").replace("/", "-")
    folder = os.path.join(PROJECTS, key)
    seen, order = {}, []
    for path in sorted(glob.glob(os.path.join(folder, "*.jsonl"))):
        for line in open(path):
            try:
                event = json.loads(line)
            except ValueError:
                continue
            message = event.get("message") or {}
            if event.get("type") != "assistant" or not isinstance(message, dict) or not message.get("usage"):
                continue
            ident = message.get("id")
            if ident not in seen:
                seen[ident] = {"usage": message["usage"], "tools": [], "model": message.get("model")}
                order.append(ident)
            seen[ident]["tools"] += [c.get("name") for c in message.get("content", []) if isinstance(c, dict) and c.get("type") == "tool_use"]
    calls = []
    for ident in order:
        usage = seen[ident]["usage"]
        creation = usage.get("cache_creation") or {}
        write = creation.get("ephemeral_1h_input_tokens", 0) + creation.get("ephemeral_5m_input_tokens", 0) or usage.get("cache_creation_input_tokens", 0)
        calls.append({
            "fresh": usage.get("input_tokens", 0), "write": write,
            "read": usage.get("cache_read_input_tokens", 0), "out": usage.get("output_tokens", 0),
            "thinking": (usage.get("output_tokens_details") or {}).get("thinking_tokens", 0),
            "tools": seen[ident]["tools"], "model": seen[ident]["model"],
        })
    return calls


def nerdgenie_calls(run_folder):
    """Coeus's per-call counters, differenced from its driver log, with the
    cache split taken from Claude Code's own bill for each call."""
    log = open(os.path.join(run_folder, "home", "drive.log")).read().splitlines()
    rows, previous = [], (0.0, 0, 0)
    for line in log:
        found = re.search(r"cost=([\d.]+) \| tokensIn=(\d+) \| tokensOut=(\d+)", line)
        if not found:
            continue
        now = (float(found.group(1)), int(found.group(2)), int(found.group(3)))
        if now == previous:
            continue
        rows.append({"cost_program": now[0] - previous[0], "in": now[1] - previous[1], "out": now[2] - previous[2]})
        previous = now
    # Coeus's counters merge fresh, cache write and cache read into one "in"
    # figure, so the split is taken from Claude Code's own bill for the call:
    # a read costs 0.50 where a write costs 10, so the gap between the all-write
    # price and the bill says how many tokens were read. A tiny Haiku side call
    # the program sometimes makes on its own adds about a tenth of a cent, which
    # is under 150 tokens' worth and is ignored.
    calls = []
    for row in rows:
        all_write = (row["in"] * PRICE_WRITE + row["out"] * PRICE_OUT) / 1e6
        read = int(round((all_write - row["cost_program"]) / ((PRICE_WRITE - PRICE_READ) / 1e6)))
        read = max(0, min(read, row["in"]))
        calls.append({"fresh": 0, "write": row["in"] - read, "read": read, "out": row["out"], "thinking": None,
                      "tools": [], "model": "claude-opus-4-8", "cost_program": row["cost_program"]})
    sent = next(seconds(l[:8]) for l in log if '>> {"type": "message"' in l)
    replied = next((seconds(l[:8]) for l in log if "<< REPLY #1" in l), None)
    tool_calls = len([l for l in log if re.search(r"toolLine=▸.* · r\d+", l)])
    done = next((l for l in log if "== done in" in l), "")
    asks = re.search(r"asks=(\d+)", done)
    previews = re.search(r"previews=(\d+)", done)
    return calls, (replied - sent) if replied is not None else None, tool_calls, int(asks.group(1)) if asks else 0, int(previews.group(1)) if previews else 0


def checker(run_folder):
    try:
        verdict = json.load(open(os.path.join(run_folder, "check.json")))
    except (OSError, ValueError):
        return None
    thorough = verdict["thoroughness"]
    return {
        "logic": verdict["correctness"]["passed"], "plays": verdict["plays"]["passed"],
        "tests_written": thorough["testsWritten"], "tests_passing": thorough["testsPassing"], "tests_total": thorough["testsTotal"],
        "lines": thorough["gameJsLines"], "quality": thorough["quality"]["passed"],
    }


def wall(run_folder):
    text = open(os.path.join(run_folder, "wall.txt")).read()
    found = re.search(r"exit (\d+) launch_to_exit (\d+)", text)
    return int(found.group(1)), int(found.group(2))


def load_run(harness, number):
    folder = os.path.join(BASE, f"{harness}-{number}")
    if not os.path.isdir(folder) or not os.path.exists(os.path.join(folder, "wall.txt")):
        return None
    exit_status, launch_to_exit = wall(folder)
    run = {"harness": harness, "number": number, "exit": exit_status, "launch_to_exit": launch_to_exit, "check": checker(folder), "notes": []}
    if harness == "nerdgenie":
        calls, task_to_answer, tool_calls, asks, previews = nerdgenie_calls(folder)
        run.update(calls=calls, task_to_answer=task_to_answer, tool_calls=tool_calls)
        if asks or previews:
            run["notes"].append(f"driver answered {asks} questions and {previews} previews")
    else:
        calls = session_calls(os.path.join(folder, "work"))
        run.update(calls=calls, task_to_answer=launch_to_exit, tool_calls=sum(len(c["tools"]) for c in calls))
        if harness == "hermes":
            qwen = open(os.path.join(folder, "qwen-calls.txt")).read()
            found = re.search(r"model calls: (\d+)", qwen)
            run["qwen_calls"] = int(found.group(1)) if found else None
            err = open(os.path.join(folder, "out.log"), errors="replace").read()
            run["notes"].append("claude -p typed by Hermes" if "claude -p" in err else "no claude -p seen in Hermes's output")
        if harness == "openclaw":
            err = open(os.path.join(folder, "err.log"), errors="replace").read()
            turns = re.findall(r"cli turn: .*?durationMs=(\d+)", err)
            run["claude_code_ms"] = sum(int(t) for t in turns)
            run["notes"].append(f"OpenClaw turns: {len(turns)}")
    models = {c["model"] for c in run["calls"]}
    if models and models != {"claude-opus-4-8"}:
        run["notes"].append(f"MODEL WAS {models}")
    return run


def totals(run):
    calls = run["calls"]
    fresh = sum(c["fresh"] for c in calls)
    write = sum(c["write"] for c in calls)
    read = sum(c["read"] for c in calls)
    out = sum(c["out"] for c in calls)
    thinking = sum(c["thinking"] or 0 for c in calls) if calls and calls[0]["thinking"] is not None else None
    as_run = cost(fresh, write, read, out)
    first_read = calls[0]["read"] if calls else 0
    cold = as_run + first_read * (PRICE_WRITE - PRICE_READ) / 1e6
    program = sum(c.get("cost_program", 0) for c in calls) if run["harness"] == "nerdgenie" else None
    return dict(fresh=fresh, write=write, read=read, out=out, thinking=thinking, tokens_in=fresh + write + read,
                as_run=as_run, cold=cold, program=program, first_read=first_read)


def fmt_time(value):
    return "" if value is None else (f"{value // 60} min {value % 60} s" if value >= 60 else f"{value} s")


def green(run):
    c = run["check"]
    return bool(c) and c["logic"] == 6 and c["plays"] == 4 and c["tests_passing"] == c["tests_total"] and c["tests_total"] >= 6


def main():
    runs = [r for h in HARNESSES for n in (1, 2, 3) if (r := load_run(h, n))]
    print("## Every run\n")
    print("| Harness | Run | Green | Task in to answer out | Opus calls | Tool calls | Tokens in | Cache write | Cache read | Out | Thinking | Cost as run | Cost if cold | Logic | Plays | Tests | game.js |")
    print("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    for run in runs:
        t = totals(run); c = run["check"] or {}
        extra = f" (+{run['qwen_calls']} Qwen)" if run.get("qwen_calls") else ""
        think = "not counted" if t["thinking"] is None else f"{t['thinking']:,}"
        tests = f"{c.get('tests_passing', 0)}/{c.get('tests_total', 0)}" if c else "no verdict"
        print(f"| {run['harness']} | {run['number']} | {'yes' if green(run) else 'NO'} | {fmt_time(run['task_to_answer'])} | {len(run['calls'])}{extra} | {run['tool_calls']} | {t['tokens_in']:,} | {t['write']:,} | {t['read']:,} | {t['out']:,} | {think} | ${t['as_run']:.2f} | ${t['cold']:.2f} | {c.get('logic', '-')}/6 | {c.get('plays', '-')}/4 | {tests} | {c.get('lines', '-')} |")
    print("\n## Averages of the three runs\n")
    print("| Harness | Green | Task in to answer out | Opus calls | Tool calls | Tokens in | Cache read | Out | Cost as run | Cost if cold |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for harness in HARNESSES:
        group = [r for r in runs if r["harness"] == harness]
        if not group:
            continue
        ts = [totals(r) for r in group]
        n = len(group)
        mean = lambda key: sum(t[key] for t in ts) / n
        time_mean = sum(r["task_to_answer"] for r in group) / n
        print(f"| {harness} | {sum(green(r) for r in group)} of {n} | {fmt_time(int(round(time_mean)))} | {sum(len(r['calls']) for r in group) / n:.1f} | {sum(r['tool_calls'] for r in group) / n:.1f} | {mean('tokens_in'):,.0f} | {mean('read'):,.0f} | {mean('out'):,.0f} | ${mean('as_run'):.2f} | ${mean('cold'):.2f} |")
    print("\n## Every call\n")
    print("| Run | Call | Fresh | Cache write | Cache read | Out | Thinking | Tokens in | Cost | Tools |")
    print("|---|---|---|---|---|---|---|---|---|---|")
    for run in runs:
        for index, c in enumerate(run["calls"], 1):
            think = "" if c["thinking"] is None else str(c["thinking"])
            tools = ", ".join(c["tools"]) if c["tools"] else ("Coeus's own tools" if run["harness"] == "nerdgenie" else "none")
            print(f"| {run['harness']} {run['number']} | {index} | {c['fresh']:,} | {c['write']:,} | {c['read']:,} | {c['out']:,} | {think} | {c['fresh'] + c['write'] + c['read']:,} | ${cost(c['fresh'], c['write'], c['read'], c['out']):.3f} | {tools} |")
    print("\n## Notes per run\n")
    for run in runs:
        t = totals(run)
        line = f"- {run['harness']} {run['number']}: exit {run['exit']}, launch to exit {fmt_time(run['launch_to_exit'])}"
        if run["harness"] == "nerdgenie" and t["program"] is not None:
            line += f", Claude Code's own bills sum to ${t['program']:.2f}"
        if run.get("claude_code_ms"):
            line += f", Claude Code's own time {run['claude_code_ms'] / 1000:.1f} s"
        if run["notes"]:
            line += "; " + "; ".join(run["notes"])
        print(line)


if __name__ == "__main__":
    sys.exit(main())
