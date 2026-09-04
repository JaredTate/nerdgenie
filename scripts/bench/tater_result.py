#!/usr/bin/env python3
"""Turn one benchmark run's raw output into result.json and result.txt.

Usage: tater_result.py RUN_FOLDER

The runner has already written RUN_FOLDER/facts.json with what it knows about
the run itself: the harness, when it started and stopped, and how it ended. This
script adds the three kinds of measurement that come from files, and writes them
out as one flat JSON object and the same numbers in plain words.

  1. The model's side of the run, from countcalls.sh's reading of the slice of
     the daemon log this run wrote. That is the only place the token counts for
     a local model exist, and every harness is counted the same way from it.
  2. The harness's own report of what it used, read from wherever that harness
     keeps it: Nerd Genie's driver log, opencode's step_finish lines, Hermes's
     sessions row, OpenClaw's JSON envelope. Each of those is kept beside the
     daemon's numbers as a cross-check, never in place of them.
  3. The checker's verdict on the work folder, which is the quality half.
"""
import json
import os
import sqlite3
import sys

RUN = os.path.abspath(sys.argv[1])
FACTS = json.load(open(os.path.join(RUN, "facts.json")))

WORK = os.path.join(RUN, "work")
HOME = os.path.join(RUN, "home")


def read(path):
    """read a whole text file, or return the empty string if it is not there."""
    try:
        with open(path, encoding="utf-8", errors="replace") as handle:
            return handle.read()
    except OSError:
        return ""


# ---- 1. the model's side: countcalls.sh on the daemon log ------------------

COUNTCALLS_NAMES = {
    "model calls": "calls",
    "prompt tokens read": "prompt_tokens_read",
    "prompt tokens read per call on average": "average_prefill",
    "largest single prefill": "largest_prefill",
    "generated tokens": "generated_tokens",
    "total input tokens": "tokens_in",
    "total output tokens": "tokens_out",
    "largest context": "largest_context",
}


def read_countcalls(path):
    """read countcalls.sh's "name: number" lines into the shared field names."""
    numbers = {}
    for line in read(path).splitlines():
        if ":" not in line:
            continue
        name, value = line.split(":", 1)
        key = COUNTCALLS_NAMES.get(name.strip())
        if not key:
            continue
        try:
            numbers[key] = int(value.strip())
        except ValueError:
            continue
    if not numbers:
        return {}, False
    # The daemon's "prompt tokens read" is the part it could not take from its
    # cache, so the rest of the input was a cache hit.
    numbers["cache_read"] = max(0, numbers.get("tokens_in", 0) - numbers.get("prompt_tokens_read", 0))
    numbers["cache_creation"] = 0
    return numbers, True


# ---- 2. what the harness itself says it used -------------------------------

def nerdgenie_own_report():
    """count what the driver had to say to Nerd Genie, from its log of envelopes."""
    text = read(os.path.join(HOME, "drive.log"))
    done = [line for line in text.splitlines() if "== done in" in line]
    report = {"own_source": "the driver's log of every envelope, drive.log"}
    if not done:
        report["own_note"] = "the driver wrote no summary line, so it did not finish cleanly"
        return report
    for pair in done[-1].split(":", 2)[-1].split():
        if "=" not in pair:
            continue
        name, value = pair.split("=", 1)
        try:
            report["own_" + name] = int(value)
        except ValueError:
            continue
    report.setdefault("own_continues", 0)
    # Nerd Genie's own running totals come over the socket as status fields, so the
    # last one the driver saw is what Nerd Genie itself thinks it spent.
    for field in ("tokensIn", "tokensOut", "cost", "contextTokens"):
        seen = [line for line in text.splitlines() if ("%s=" % field) in line]
        if not seen:
            continue
        piece = seen[-1].split("%s=" % field, 1)[1].split(" |")[0].strip()
        report["own_" + field] = piece
    return report


def opencode_own_report():
    """add up the step_finish parts opencode printed with --format json."""
    report = {
        "own_source": "opencode's own step_finish lines from --format json",
        "own_steps": 0, "own_input_tokens": 0, "own_output_tokens": 0,
        "own_cache_read_tokens": 0, "own_cache_write_tokens": 0,
        "own_reasoning_tokens": 0, "own_cost_usd": 0.0,
    }
    for line in read(os.path.join(RUN, "harness.out")).splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if not isinstance(event, dict) or event.get("type") != "step_finish":
            continue
        part = event.get("part") or {}
        tokens = part.get("tokens") or {}
        cache = tokens.get("cache") or {}
        report["own_steps"] += 1
        report["own_input_tokens"] += int(tokens.get("input") or 0)
        report["own_output_tokens"] += int(tokens.get("output") or 0)
        report["own_reasoning_tokens"] += int(tokens.get("reasoning") or 0)
        report["own_cache_read_tokens"] += int(cache.get("read") or 0)
        report["own_cache_write_tokens"] += int(cache.get("write") or 0)
        report["own_cost_usd"] += float(part.get("cost") or 0.0)
    report["own_cost_usd"] = round(report["own_cost_usd"], 6)
    return report


def hermes_own_report():
    """read the one session row Hermes wrote for this run's work folder."""
    database = os.path.join(HOME, "hermes", "state.db")
    report = {"own_source": "the sessions row in Hermes's own state.db"}
    if not os.path.exists(database):
        report["own_note"] = "Hermes wrote no state.db, so it never opened a session"
        return report
    columns = [
        "api_call_count", "input_tokens", "output_tokens",
        "cache_read_tokens", "cache_write_tokens",
    ]
    try:
        connection = sqlite3.connect("file:%s?mode=ro" % database, uri=True, timeout=10)
        try:
            row = connection.execute(
                "SELECT %s FROM sessions WHERE cwd = ? ORDER BY started_at DESC LIMIT 1"
                % ", ".join(columns),
                (WORK,),
            ).fetchone()
        finally:
            connection.close()
    except sqlite3.Error as trouble:
        report["own_note"] = "could not read state.db: %s" % trouble
        return report
    if row is None:
        report["own_note"] = "no session row has cwd %s" % WORK
        return report
    for name, value in zip(columns, row):
        report["own_" + name] = int(value or 0)
    return report


def openclaw_envelope():
    """find the JSON envelope OpenClaw printed among its own chatter.

    OpenClaw prints progress lines before the envelope and an error line after
    it, and the envelope itself is pretty-printed over many lines, so neither
    "the whole output" nor "a line that is JSON" finds it. Every opening brace
    is tried as the start of an object instead, and the last one that decodes
    into something shaped like the envelope wins.
    """
    text = read(os.path.join(RUN, "harness.out"))
    decoder = json.JSONDecoder()
    found = None
    at = text.find("{")
    # Bounded at ten thousand opening braces, which is far more than any run
    # prints, so a pathological output cannot spin here.
    for _ in range(10000):
        if at < 0:
            break
        try:
            value, _end = decoder.raw_decode(text, at)
        except ValueError:
            value = None
        if isinstance(value, dict) and ("ok" in value or "status" in value):
            found = value
        at = text.find("{", at + 1)
    return found


def openclaw_own_report():
    """take the usage, the turn count and the tool summary from the envelope.

    Which keys the usage carries depends on the route OpenClaw took, so every
    number it reports is copied through under its own name rather than being
    forced into a fixed set.
    """
    report = {"own_source": "OpenClaw's own JSON envelope from agent exec --json"}
    envelope = openclaw_envelope()
    if envelope is None:
        report["own_note"] = (
            "OpenClaw printed no JSON envelope, which is what a run stopped by a "
            "signal looks like"
        )
        return report
    report["own_status"] = str(envelope.get("status", ""))
    report["own_ok"] = bool(envelope.get("ok"))
    if envelope.get("assistantTurns") is not None:
        report["own_assistant_turns"] = int(envelope["assistantTurns"])
    usage = envelope.get("usage") or {}
    for name, value in usage.items():
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            continue
        report["own_usage_" + name] = value
    summary = envelope.get("toolSummary")
    if summary is not None:
        report["own_tool_summary"] = json.dumps(summary, sort_keys=True)[:2000]
    return report


OWN_REPORTS = {
    "nerdgenie": nerdgenie_own_report,
    "opencode": opencode_own_report,
    "hermes": hermes_own_report,
    "openclaw": openclaw_own_report,
}


# ---- 3. the checker's verdict ----------------------------------------------

def checker_verdict():
    """read check-tater.mjs's JSON, or say why there is none."""
    text = read(os.path.join(RUN, "check.json"))
    verdict = {
        "logic_passed": 0, "logic_of": 6, "plays_passed": 0, "plays_of": 4,
        "tests_written": 0, "tests_passing": 0, "tests_total": 0,
        "game_js_lines": 0, "quality_passed": 0, "checker_ran": False,
        "checker_note": "",
    }
    try:
        report = json.loads(text)
    except ValueError:
        verdict["checker_note"] = (
            (read(os.path.join(RUN, "check.err")).strip() or "the checker wrote no JSON")[:500]
        )
        return verdict
    correctness = report.get("correctness") or {}
    plays = report.get("plays") or {}
    thoroughness = report.get("thoroughness") or {}
    quality = thoroughness.get("quality") or {}
    verdict.update({
        "checker_ran": True,
        "logic_passed": int(correctness.get("passed") or 0),
        "logic_of": int(correctness.get("of") or 6),
        "plays_passed": int(plays.get("passed") or 0),
        "plays_of": 4,
        "tests_written": int(thoroughness.get("testsWritten") or 0),
        "tests_passing": int(thoroughness.get("testsPassing") or 0),
        "tests_total": int(thoroughness.get("testsTotal") or 0),
        "game_js_lines": int(thoroughness.get("gameJsLines") or 0),
        "quality_passed": sum(1 for value in quality.values() if value is True),
    })
    return verdict


# ---- put it all together ---------------------------------------------------

result = dict(FACTS)
result["cap"] = FACTS.get("cap") or "none"
result["minutes_cap"] = FACTS.get("minutes_cap") or "none"
result["cap_hit"] = os.path.exists(os.path.join(RUN, "capped"))

numbers, read_it = read_countcalls(os.path.join(RUN, "countcalls.txt"))
result.update(numbers)
result["model_source"] = "countcalls.sh on %s" % FACTS.get("daemon_log", "the daemon log")
if not read_it:
    result["model_source"] = "countcalls.sh wrote nothing readable"
# A local model on this machine costs no dollars and the daemon log records no
# per-call wall time, so those are left unmeasured rather than called zero.
result["cost_usd"] = None
result["model_ms"] = None
result["harness_ms"] = None
for name in ("calls", "tokens_in", "tokens_out", "cache_read", "cache_creation"):
    result.setdefault(name, 0)

result.update(OWN_REPORTS[FACTS["harness"]]())
result.update(checker_verdict())
result["finished"] = (
    result["checker_ran"]
    and result["logic_passed"] == result["logic_of"]
    and result["plays_passed"] == result["plays_of"]
)

with open(os.path.join(RUN, "result.json"), "w") as handle:
    json.dump(result, handle, indent=2, sort_keys=True)
    handle.write("\n")


def say(value):
    """write one value the way a person reads it."""
    if value is None:
        return "not measured for a local model"
    if isinstance(value, bool):
        return "yes" if value else "no"
    return str(value)


minutes, seconds = divmod(int(result["wall_seconds"]), 60)
lines = [
    "%s phase, %s, run %s, card %s" % (
        result["phase"], result["harness"], result["label"], result["card"]),
    "",
    "How it went",
    "  finished the task: %s" % say(result["finished"]),
    "  how it ended: %s (exit status %s)" % (result["finished_by"], result["exit_status"]),
    "  wall clock: %d minutes %d seconds" % (minutes, seconds),
    "  call cap: %s, and it was hit: %s" % (result["cap"], say(result["cap_hit"])),
    "  wall-clock cap: %s" % result["minutes_cap"],
    "  Nerd Genie's own task budget raised out of the way: %s" % say(result.get("budget_raised", False)),
    "",
    "What the model did (from %s)" % result["model_source"],
    "  model calls: %s" % say(result.get("calls")),
    "  input tokens, cached and uncached both: %s" % say(result.get("tokens_in")),
    "  of those, taken from the cache: %s" % say(result.get("cache_read")),
    "  read afresh: %s" % say(result.get("prompt_tokens_read")),
    "  read afresh per call on average: %s" % say(result.get("average_prefill")),
    "  largest single prefill: %s" % say(result.get("largest_prefill")),
    "  largest context held: %s" % say(result.get("largest_context")),
    "  output tokens: %s" % say(result.get("tokens_out")),
    "  dollars: %s" % say(result.get("cost_usd")),
    "",
    "What the harness itself reported (%s)" % result.get("own_source", "nothing"),
]
for name in sorted(key for key in result if key.startswith("own_") and key != "own_source"):
    lines.append("  %s: %s" % (name[4:].replace("_", " "), say(result[name])))
lines += [
    "",
    "What the checker found in the work folder",
    "  game logic, of six: %s" % result["logic_passed"],
    "  actually plays, of four: %s" % result["plays_passed"],
    "  tests the harness wrote: %s" % result["tests_written"],
    "  of those, passing: %s of %s" % (result["tests_passing"], result["tests_total"]),
    "  lines of game.js: %s" % result["game_js_lines"],
    "  quality points, of six: %s" % result["quality_passed"],
]
if result["checker_note"]:
    lines.append("  the checker could not judge it: %s" % result["checker_note"])
lines += [
    "",
    "Where the raw files are",
    "  %s" % RUN,
    "",
]

with open(os.path.join(RUN, "result.txt"), "w") as handle:
    handle.write("\n".join(lines))
