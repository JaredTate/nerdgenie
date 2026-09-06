#!/usr/bin/env python3
"""Behaviour analysis of Nerd Genie event logs. Read-only over copies."""
import sqlite3, json, re, statistics, sys
from datetime import datetime
from collections import Counter, defaultdict

HDR = re.compile(r'^# (task|job) (\d+)\s+(\S+)(.*)\n(?:this turn: ([\d.]+)k tokens in, ([\d.]+)k of them cached, ([\d.]+)k out)?', re.M)
TESTRUN = ["--test", "npm test", "pytest", "go test", "test/run", "vitest", "jest"]
PICTURE_TOOLS = {"browser_screenshot"}
BROWSER_TOOLS = {"browser_read", "browser_click", "browser_open", "browser_screenshot", "browser_act", "browser_resize", "browser_handoff", "computer"}
REFUSED = re.compile(r'^(\S+) was refused')
POLL_RUNNING = re.compile(r'(p\d+) is still running after (\d+)s')
PROGRESS = re.compile(r'rounds since progress: (\d+)')
DBS = ["run5", "run6", "live"]

def ts(s):
    return datetime.strptime(s[:26], "%Y-%m-%dT%H:%M:%S.%f")

def load(db):
    c = sqlite3.connect(f"file:{db}.db?mode=ro", uri=True)
    return [(s, o, t, k, json.loads(b)) for s, o, t, k, b in
            c.execute("select sequence, occurred, task_id, kind, body from events order by sequence")]

def sections(text):
    """Estimated tokens (chars/4) of the record's parts from a checkpoint text."""
    lines = text.split("\n")
    body = "\n".join(lines[2:])  # drop the two header lines
    def between(s, start, ends):
        i = s.find(start)
        if i < 0:
            return ""
        j = len(s)
        for e in ends:
            k = s.find(e, i + len(start))
            if k >= 0:
                j = min(j, k)
        return s[i:j]
    goal = between(body, "## Goal", ["## Rules", "## Work", "## Lessons"])
    rules = between(body, "## Rules", ["## Work", "## Lessons"])
    work = between(body, "## Work", ["## Lessons"])
    lessons = between(body, "## Lessons", [])
    situation = between(work, "Situation:", ["Plan:", "Results ("])
    plan = between(work, "Plan:", ["Results ("])
    results = between(work, "Results (", [])
    failures = between(lessons, "Failures:", [])
    est = lambda s: len(s) / 4
    return dict(total=est(body), goal=est(goal), rules=est(rules), work=est(work), situation=est(situation),
                plan=est(plan), results=est(results), lessons=est(lessons), failures=est(failures),
                result_lines=sum(1 for l in results.split("\n") if l.startswith("- r")),
                failure_lines=sum(1 for l in failures.split("\n") if l.startswith("- F")),
                plan_lines=sum(1 for l in plan.split("\n") if re.match(r'- (\[.\] )?', l) and l.startswith("- ")),
                stalled=[l for l in failures.split("\n") if "stalled:" in l])

def is_test_command(cmd):
    return any(r in cmd for r in TESTRUN)

class Round:
    def __init__(self):
        self.calls = []      # (name, input, seq, occurred)
        self.results = []    # (id, summary, text, seq)
        self.messages = []   # (occurred, sender, text)
        self.replies = []    # text
        self.file_changes = 0
        self.checkpoint = None
        self.occurred = None
        self.state = None
        self.tin = self.cached = self.out = 0.0
        self.number = None
        self.text = ""
        self.sec = None
        self.progress = 0
        self.wall = None
        self.start = None
        self.dup = False
        self.gap = False

    @property
    def uncached(self):
        return self.tin - self.cached
    def tools(self):
        return Counter(n for n, _, _, _ in self.calls)
    def only_task(self):
        return bool(self.calls) and all(n == "task" for n, _, _, _ in self.calls)
    def only_polls(self):
        return bool(self.calls) and all(n == "shell" and (i or {}).get("action") in ("poll", "tail") for n, i, _, _ in self.calls)
    def rn_reads(self):
        return [i.get("path") for n, i, _, _ in self.calls if n == "read" and re.fullmatch(r'r\d+', str((i or {}).get("path", "")))]
    def writes(self):
        return [n for n, _, _, _ in self.calls if n in ("write", "edit")]
    def shell_runs(self):
        return [(i or {}).get("command", "") for n, i, _, _ in self.calls if n == "shell" and (i or {}).get("action") in (None, "", "run")]
    def test_runs(self):
        return [c for c in self.shell_runs() if is_test_command(c)]
    def refusals(self):
        out = []
        for rid, summ, text, seq in self.results:
            m = REFUSED.match(summ or "")
            if m:
                out.append((m.group(1), (text or "")[:160]))
        return out
    def screenshots(self):
        return sum(1 for n, i, _, _ in self.calls if n in PICTURE_TOOLS or (n == "computer" and (i or {}).get("action") == "screenshot"))
    def result_chars(self):
        return sum(len(t or "") for _, _, t, _ in self.results)

def build_tasks(evs):
    """Group events per numeric task into rounds."""
    tasks = {}
    order = []
    current = {}
    first_event = {}
    for s, o, t, k, j in evs:
        if not t or t.startswith("j"):
            continue
        if t not in tasks:
            tasks[t] = []
            order.append(t)
            first_event[t] = o
        r = current.setdefault(t, Round())
        if k == "tool call":
            r.calls.append((j.get("Name"), j.get("Input") or {}, s, o))
        elif k == "tool result":
            r.results.append((j.get("id"), j.get("summary", ""), j.get("text", ""), s))
        elif k == "message":
            r.messages.append((o, j.get("Sender", ""), j.get("Text", "")))
        elif k == "reply":
            # a reply lands after the checkpoint of the round that answered
            if tasks[t]:
                tasks[t][-1].replies.append(j.get("text", ""))
            else:
                r.replies.append(j.get("text", ""))
        elif k == "file change":
            r.file_changes += 1
        elif k == "checkpoint":
            m = HDR.match(j["text"])
            r.checkpoint = s
            r.occurred = o
            r.number = j.get("number")
            r.text = j["text"]
            r.state = m.group(3) if m else "?"
            if m and m.group(5):
                r.tin, r.cached, r.out = float(m.group(5)) * 1000, float(m.group(6)) * 1000, float(m.group(7)) * 1000
            r.sec = sections(j["text"])
            pm = PROGRESS.search(j["text"])
            r.progress = int(pm.group(1)) if pm else 0
            tasks[t].append(r)
            current[t] = Round()
    # trailing events (messages/replies after the last checkpoint)
    for t, r in current.items():
        if r.messages or r.replies or r.calls:
            tasks[t].append(r)  # trailing pseudo-round marked by checkpoint None
    # duplicate checkpoints: the closing checkpoint of a task and the checkpoint written when a task is
    # resumed repeat the last cost header with no calls; they are not model rounds
    for t in order:
        prev = None
        for r in tasks[t]:
            if r.checkpoint is None:
                continue
            r.dup = prev is not None and not r.calls and (r.tin, r.cached, r.out) == (prev.tin, prev.cached, prev.out)
            prev = r
    # wall times; a wait of more than 15 minutes is idle time (a stop, a restart, the person away), not a round
    for t in order:
        prev = None
        for i, r in enumerate(tasks[t]):
            if r.checkpoint is None:
                continue
            start = ts(prev.occurred) if prev else ts(first_event[t])
            for o, _, _ in r.messages:
                start = max(start, ts(o))
            r.start = start
            r.wall = (ts(r.occurred) - start).total_seconds()
            r.gap = r.wall > 900
            if r.gap:
                r.wall = None
            prev = r
    return tasks, order

def real_rounds(rounds):
    """Model rounds: checkpoints that report a model call. Zero-token checkpoints are the harness opening, stopping or resuming a task."""
    return [r for r in rounds if r.checkpoint is not None and r.tin > 0 and not r.dup]

def bookkeeping_checkpoints(rounds):
    return [r for r in rounds if r.checkpoint is not None and r.tin <= 0]

def pct(a, b):
    return 100.0 * a / b if b else 0.0

def quantiles(xs):
    if not xs:
        return (0, 0, 0, 0, 0)
    xs = sorted(xs)
    q = lambda p: xs[min(len(xs) - 1, int(p * len(xs)))]
    return (xs[0], q(0.5), statistics.mean(xs), q(0.9), xs[-1])

def md_table(headers, rows):
    out = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    for r in rows:
        out.append("| " + " | ".join(str(x) for x in r) + " |")
    return "\n".join(out)

def fmt(x, d=1):
    if isinstance(x, float):
        return f"{x:.{d}f}"
    return str(x)

def first_line(s):
    return (s or "").split("\n")[0]

def main():
    all_tasks = {}
    for db in DBS:
        evs = load(db)
        tasks, order = build_tasks(evs)
        all_tasks[db] = (tasks, order, evs)
    report = []
    P = report.append

    # ---------- 1. rounds, wall time, tokens per task ----------
    P("## T1. Rounds, wall time and tokens per task\n")
    rows = []
    totals = Counter()
    all_walls = []
    all_unc = []
    all_out = []
    per_db_walls = defaultdict(list)
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            if not rs:
                continue
            walls = [r.wall for r in rs if r.wall is not None]
            gaps = sum(1 for r in rs if r.gap)
            tin = sum(r.tin for r in rs); cached = sum(r.cached for r in rs); out = sum(r.out for r in rs)
            unc = [r.uncached for r in rs]
            minutes = sum(walls) / 60
            span = (ts(rs[-1].occurred) - rs[0].start).total_seconds() / 60 if rs[0].start else 0
            q = quantiles(walls)
            final_state = [r for r in tasks[t] if r.checkpoint is not None][-1].state
            rows.append([db, t, final_state, len(rs), fmt(minutes), fmt(span, 0) + (f" ({gaps} idle gap)" if gaps else ""), fmt(q[1], 0), fmt(q[2], 0), fmt(q[3], 0), fmt(q[4], 0),
                         fmt(tin / 1000, 0), fmt(pct(cached, tin), 0), fmt(statistics.median(unc) / 1000, 1), fmt(statistics.mean(unc) / 1000, 1),
                         fmt(out / 1000, 1), fmt(out / len(rs) / 1000, 2)])
            totals["rounds"] += len(rs); totals["minutes"] += minutes; totals["tin"] += tin; totals["cached"] += cached; totals["out"] += out
            all_walls += walls; all_unc += unc; all_out += [r.out for r in rs]
            per_db_walls[db] += walls
    q = quantiles(all_walls)
    rows.append(["all", "", "", totals["rounds"], fmt(totals["minutes"]), "", fmt(q[1], 0), fmt(q[2], 0), fmt(q[3], 0), fmt(q[4], 0),
                 fmt(totals["tin"] / 1000, 0), fmt(pct(totals["cached"], totals["tin"]), 0), fmt(statistics.median(all_unc) / 1000, 1),
                 fmt(statistics.mean(all_unc) / 1000, 1), fmt(totals["out"] / 1000, 1), fmt(totals["out"] / totals["rounds"] / 1000, 2)])
    P(md_table(["log", "task", "state", "rounds", "min (sum of rounds)", "min (span)", "s/round med", "mean", "p90", "max",
                "tokens in (k)", "cached %", "uncached/round med (k)", "mean (k)", "out (k)", "out/round (k)"], rows))
    P("")
    # wall time distribution
    P("\n### T1b. Wall time per round, distribution (all task rounds)\n")
    buckets = [(0, 5), (5, 10), (10, 20), (20, 30), (30, 60), (60, 120), (120, 300), (300, 10 ** 9)]
    rows = []
    for lo, hi in buckets:
        n = sum(1 for w in all_walls if lo <= w < hi)
        rows.append([f"{lo}-{hi if hi < 10**9 else 'inf'} s", n, fmt(pct(n, len(all_walls)), 0), fmt(sum(w for w in all_walls if lo <= w < hi) / 60, 0)])
    P(md_table(["bucket", "rounds", "% of rounds", "minutes in bucket"], rows))
    P("")
    for db in DBS:
        q = quantiles(per_db_walls[db])
        P(f"- {db}: {len(per_db_walls[db])} rounds, s/round min {q[0]:.0f}, median {q[1]:.0f}, mean {q[2]:.0f}, p90 {q[3]:.0f}, max {q[4]:.0f}")
    # fit wall = a + b*uncached + c*out (least squares) over rounds with wall < 600 s
    pts = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            for r in real_rounds(tasks[t]):
                if r.wall is not None and r.wall < 600 and r.tin > 0:
                    pts.append((r.uncached / 1000, r.out / 1000, r.wall))
    try:
        import numpy as np
        A = np.array([[1, u, o] for u, o, _ in pts]); y = np.array([w for _, _, w in pts])
        coef, *_ = np.linalg.lstsq(A, y, rcond=None)
        pred = A @ coef
        resid = y - pred
        P(f"\nLeast-squares fit over {len(pts)} rounds under 600 s: wall s = {coef[0]:.1f} + {coef[1]:.2f} s per 1k uncached tokens in + {coef[2]:.2f} s per 1k tokens out (residual sd {resid.std():.0f} s)."
          f" That is about {1000/coef[1]:.0f} tokens/s prompt processing and {1000/coef[2]:.0f} tokens/s generation, if the fit is trusted.")
    except Exception as e:
        P(f"(no numpy fit: {e})")

    # ---------- 1c. uncached share evolution and cache breaks ----------
    P("\n### T1c. Cache events: fresh windows and cache losses, with their causes\n")
    rows = []
    cause_count = Counter()
    cause_tokens = Counter()
    catchup_rounds = 0
    catchup_tokens = 0
    step_rows = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            if not rs:
                continue
            prev_stalled = 0
            prefix = min((r.cached for r in rs if r.cached > 0), default=0)
            # cache step: median positive difference between consecutive distinct cached values while in grows
            steps = [rs[i].cached - rs[i - 1].cached for i in range(1, len(rs)) if rs[i].cached > rs[i - 1].cached and rs[i].tin >= rs[i - 1].tin]
            same = sum(1 for i in range(1, len(rs)) if rs[i].cached == rs[i - 1].cached and rs[i].tin > rs[i - 1].tin)
            # excess uncached: what a perfect prefix cache would not have re-read. The legitimate tail of a round's prompt is the
            # record below the line (everything but goal and rules), last round's reply and last round's result text.
            excess = 0.0
            for i in range(1, len(rs)):
                r, p = rs[i], rs[i - 1]
                tail = (r.sec["total"] - r.sec["goal"] - r.sec["rules"] if r.sec else 0) + p.out + p.result_chars() / 4 + 300
                excess += max(0.0, r.uncached - tail)
            task_catchup = 0
            in_catchup = False
            cu_before = catchup_rounds
            step_rows.append([db, t, len(rs), fmt(prefix / 1000, 1), fmt(statistics.median(steps) / 1000, 1) if steps else "-", len(steps), same,
                              fmt(statistics.median(r.uncached for r in rs) / 1000, 1), fmt(sum(r.uncached for r in rs) / 1000, 0), fmt(excess / 1000, 0), None])
            for i, r in enumerate(rs):
                stalled_now = len(r.sec["stalled"]) if r.sec else 0
                new_stall = stalled_now > prev_stalled
                prev_stalled = stalled_now
                prev = rs[i - 1] if i else None
                fresh = i == 0 or (prev and r.tin < 0.6 * prev.tin)
                cause = None
                if i == 0:
                    cause = "first round of the task"
                elif new_stall:
                    cause = "rewind (" + ("same-call guard" if "asked for" in r.sec["stalled"][-1] else "progress meter") + ")"
                elif fresh and r.messages:
                    cause = "fresh window: task resumed on the person's message"
                elif fresh and r.gap:
                    cause = "fresh window after an idle gap or restart"
                elif fresh:
                    cause = "window trim (oldest half of the messages left)"
                elif r.cached == 0 and prev.cached > 0:
                    cause = "cache lost, prompt unchanged (another model call took the daemon's slot)"
                elif prev and r.cached < prev.cached - 3000 and r.tin >= prev.tin:
                    cause = "cache fell back within the window (prefix changed mid-conversation)"
                if cause:
                    cause_count[cause] += 1
                    cause_tokens[cause] += r.uncached
                    rows.append([db, t, i + 1, fmt(r.tin / 1000, 1), fmt(r.cached / 1000, 1), fmt(prev.tin / 1000, 1) if prev else "-", cause])
                    in_catchup = True
                    continue
                # catch-up rounds: the cache still sits at the fixed prefix while the window grows
                if in_catchup and prefix and r.cached <= prefix + 500 and r.tin > prefix + 2000:
                    catchup_rounds += 1
                    catchup_tokens += r.uncached
                else:
                    in_catchup = False
            step_rows[-1][-1] = catchup_rounds - cu_before
    P(md_table(["log", "task", "round", "in (k)", "cached (k)", "prev in (k)", "cause"], rows))
    P("\nBy cause:\n")
    P(md_table(["cause", "rounds", "uncached tokens in those rounds (k)"], [[c, n, fmt(cause_tokens[c] / 1000, 0)] for c, n in cause_count.most_common()]))
    P(f"\nCatch-up rounds after a cache event, where the cache still sat at the fixed prefix while the window had grown 2k+ beyond it: {catchup_rounds} rounds, {catchup_tokens/1000:.0f}k uncached tokens.")
    P("\n### T1c2. Cache step per task: the fixed prefix the cache always keeps, and the size of the steps by which the cached count advances\n")
    step_rows.append(["all", "", sum(r[2] for r in step_rows), "", "", "", sum(r[6] for r in step_rows), "", fmt(sum(float(r[8]) for r in step_rows), 0), fmt(sum(float(r[9]) for r in step_rows), 0), sum(r[10] for r in step_rows)])
    P(md_table(["log", "task", "rounds", "fixed prefix (k, smallest nonzero cached)", "median cache step (k)", "steps", "rounds where in grew but cached did not", "uncached/round median (k)", "uncached total (k)", "excess uncached (k)", "catch-up rounds"], step_rows))
    P("\n(excess uncached = uncached tokens beyond what the prompt tail legitimately adds each round: the record below the cache line, last round's reply, last round's result text, and 300 for the harness's lines; it is what a perfect prefix cache would have saved)")

    # uncached share by round index within task (deciles of task life)
    P("\n### T1d. Uncached share by position in the task (tasks with 20+ rounds)\n")
    bins = defaultdict(list)
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = [r for r in real_rounds(tasks[t]) if r.tin > 0]
            if len(rs) < 20:
                continue
            for i, r in enumerate(rs):
                d = min(9, int(10 * i / len(rs)))
                bins[d].append((r.uncached, r.tin, r.sec["total"] if r.sec else 0))
    rows = []
    for d in range(10):
        xs = bins[d]
        if not xs:
            continue
        rows.append([f"{d*10}-{d*10+10}%", len(xs), fmt(statistics.median(u for u, _, _ in xs) / 1000, 1), fmt(statistics.mean(u for u, _, _ in xs) / 1000, 1),
                     fmt(statistics.median(t for _, t, _ in xs) / 1000, 1), fmt(statistics.median(rec for _, _, rec in xs) / 1000, 2)])
    P(md_table(["task life", "rounds", "uncached med (k)", "uncached mean (k)", "tokens in med (k)", "record est med (k)"], rows))

    # ---------- 2. tool mix ----------
    P("\n## T2. Tool mix per task\n")
    rows = []
    tool_totals = Counter()
    kinds_total = Counter()
    kinds_tokens = Counter()
    per_task_kinds = {}
    rn_when = Counter()
    rn_repeat = 0
    rn_total = 0
    dup_reads_total = 0
    dup_shell_total = 0
    test_after_write_total = 0
    tests_by_model_total = 0
    poll_rounds_total = 0
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            if not rs:
                continue
            tools = Counter()
            for r in rs:
                tools.update(r.tools())
            tool_totals.update(tools)
            book = [r for r in rs if r.only_task()]
            polls = [r for r in rs if r.only_polls()]
            rn = [p for r in rs for p in r.rn_reads()]
            rn_total += len(rn)
            rn_repeat += len(rn) - len(set(rn))
            # when do rN reads happen: rounds after a cached==0 round
            last_fresh = -99
            for i, r in enumerate(rs):
                if i == 0 or r.tin < 0.6 * rs[i - 1].tin:
                    last_fresh = i
                for p in r.rn_reads():
                    rn_when["within 3 rounds of a fresh window" if i - last_fresh <= 3 else "later"] += 1
            # repeated reads of same file region: identical read inputs
            reads = [json.dumps(i, sort_keys=True) for r in rs for n, i, _, _ in r.calls if n == "read" and not re.fullmatch(r'r\d+', str(i.get("path", "")))]
            dup_reads = len(reads) - len(set(reads))
            dup_reads_total += dup_reads
            # same shell command repeats
            shells = [c for r in rs for c in r.shell_runs()]
            dup_shell = len(shells) - len(set(shells))
            dup_shell_total += dup_shell
            # test runs by model, and those right after a write with 'tests after this change'
            tests = 0; after_write = 0
            for i, r in enumerate(rs):
                tr = r.test_runs()
                tests += len(tr)
                if tr:
                    prev_text = " ".join(t2 or "" for _, _, t2, _ in (rs[i - 1].results if i else [])) + " ".join(t2 or "" for _, _, t2, _ in r.results)
                    if "tests after this change" in prev_text:
                        after_write += len(tr)
            tests_by_model_total += tests
            test_after_write_total += after_write
            replies = sum(len(r.replies) for r in tasks[t])
            msgs = sum(len(r.messages) for r in tasks[t]) - 1
            no_call = [r for r in rs if not r.calls]
            kinds = dict(book=len(book), polls=len(polls), nocall=len(no_call))
            per_task_kinds[(db, t)] = kinds
            kinds_total["bookkeeping-only rounds"] += len(book)
            kinds_total["poll-only rounds"] += len(polls)
            kinds_total["rounds with no tool call (reply or cut off)"] += len(no_call)
            kinds_tokens["bookkeeping-only rounds"] += sum(r.uncached + r.out for r in book)
            kinds_tokens["poll-only rounds"] += sum(r.uncached + r.out for r in polls)
            kinds_tokens["rounds with no tool call (reply or cut off)"] += sum(r.uncached + r.out for r in no_call)
            poll_rounds_total += len(polls)
            top = ", ".join(f"{n} {c}" for n, c in tools.most_common())
            rows.append([db, t, len(rs), sum(tools.values()), top, len(book), len(polls), len(no_call), len(rn), len(rn) - len(set(rn)), dup_reads, tests, after_write, dup_shell, replies, max(msgs, 0)])
    P(md_table(["log", "task", "rounds", "calls", "calls by tool", "task-only rounds", "poll-only rounds", "no-call rounds", "rN reads", "rN re-reads", "dup file reads", "model test runs", "of which right after a write", "dup shell cmds", "replies", "mid-task msgs"], rows))
    P("\nTotals by tool: " + ", ".join(f"{n} {c}" for n, c in tool_totals.most_common()))
    P(f"\nrN reads: {rn_total} ({rn_repeat} of them a repeat of the same id); when: {dict(rn_when)}")
    P(f"Duplicate file reads (identical read input within a task): {dup_reads_total}; duplicate shell commands (identical command within a task): {dup_shell_total}")
    P(f"Test runs by the model: {tests_by_model_total}, of which {test_after_write_total} right after a write/edit whose result already said 'tests after this change'")
    P("\nRound kinds, all logs:\n")
    P(md_table(["kind", "rounds", "uncached+out tokens (k)"], [[k, n, fmt(kinds_tokens[k] / 1000, 0)] for k, n in kinds_total.items()]))

    # poll durations
    P("\n### T2b. Polled commands\n")
    rows = []
    poll_summary = Counter()
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            per_pid = {}
            for r in real_rounds(tasks[t]):
                for rid, summ, text, seq in r.results:
                    m = POLL_RUNNING.search(text or "")
                    if m:
                        pid = m.group(1); secs = int(m.group(2))
                        d = per_pid.setdefault(pid, dict(polls=0, maxs=0, cmd=first_line(text)[:70]))
                        d["polls"] += 1; d["maxs"] = max(d["maxs"], secs)
            for pid, d in per_pid.items():
                rows.append([db, t, pid, d["polls"], d["maxs"], d["cmd"].replace("|", "/")])
                poll_summary["commands polled"] += 1
                poll_summary["polls"] += d["polls"]
                poll_summary["max seconds sum"] += d["maxs"]
    P(md_table(["log", "task", "process", "polls", "longest 'running after' (s)", "command"], rows))
    P(f"\n{dict(poll_summary)}")

    # repeated shell commands top list
    P("\n### T2c. Most repeated shell commands (identical text, per task)\n")
    rows = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            c = Counter(cmd for r in real_rounds(tasks[t]) for cmd in r.shell_runs())
            for cmd, n in c.most_common(3):
                if n >= 3:
                    rows.append([db, t, n, cmd[:90].replace("|", "/").replace("\n", " ")])
    P(md_table(["log", "task", "times", "command"], rows))

    # repeated reads of files
    P("\n### T2d. Files read most often (read tool, same path, any range), per task\n")
    rows = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            c = Counter(str(i.get("path")) for r in real_rounds(tasks[t]) for n, i, _, _ in r.calls if n == "read" and not re.fullmatch(r'r\d+', str(i.get("path", ""))))
            for p, n in c.most_common(2):
                if n >= 5:
                    rows.append([db, t, n, p.replace("/home/jared/Desktop/Tater Tots Tetrisv1/", "…/")])
    P(md_table(["log", "task", "reads", "path"], rows))

    # ---------- 3. loops and interventions ----------
    P("\n## T3. Stalls, nudges, rewinds, refusals\n")
    rows = []
    tot = Counter()
    refusal_kinds = Counter()
    refusal_by_tool = Counter()
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            if not rs:
                continue
            # stall streaks from the progress line
            streaks = []
            cur = []
            for i, r in enumerate(rs):
                if r.progress > 0:
                    cur.append((i, r))
                else:
                    if cur:
                        streaks.append(cur)
                    cur = []
            if cur:
                streaks.append(cur)
            nudged = [s for s in streaks if max(r.progress for _, r in s) >= 10]
            rewound = [s for s in streaks if max(r.progress for _, r in s) >= 20]
            stall_rounds = sum(max(r.progress for _, r in s) for s in streaks)
            stall_tokens = sum(r.uncached + r.out for s in streaks for _, r in s)
            recovery = [len(s) - 10 for s in nudged if max(r.progress for _, r in s) < 20]
            # rewinds from failures
            prev = 0
            rewinds = []
            for i, r in enumerate(rs):
                n = len(r.sec["stalled"]) if r.sec else 0
                if n > prev:
                    rewinds.append((i + 1, "same-call" if "asked for" in r.sec["stalled"][-1] else "meter"))
                prev = n
            # refusals
            refs = [(tool, txt) for r in rs for tool, txt in r.refusals()]
            def kind(txt):
                if txt.startswith("You have already called"): return "same-call guard (3rd identical call)"
                if "times in a row, so the turn ends" in txt: return "same-call hard cap (turn ended)"
                if txt.startswith("The conversation is being cleared"): return "call skipped: rewind pending"
                if txt.startswith("the record refused"): return "record refused the change"
                if "names no" in txt or "there is no done line" in txt or "say which" in txt: return "call missing an argument"
                if "ask-me-first" in txt or "not allowed" in txt or "permission" in txt: return "permission denied"
                if txt.startswith("cannot"): return "tool failed (browser/desktop/read)"
                return "other"
            kc = Counter(kind(txt) for _, txt in refs)
            refusal_kinds.update(kc)
            refusal_by_tool.update(tool for tool, _ in refs)
            refused_rounds = sum(1 for r in rs if r.refusals())
            all_refused_rounds = sum(1 for r in rs if r.refusals() and all(REFUSED.match(s or "") for _, s, _, _ in r.results))
            cutoffs = sum(1 for r in rs if r.out >= 8100)
            rows.append([db, t, len(rs), len(streaks), stall_rounds, fmt(stall_tokens / 1000, 0), len(nudged), len(rewound),
                         ", ".join(f"r{i} {k}" for i, k in rewinds) or "-", ", ".join(str(x) for x in recovery) or "-",
                         len(refs), refused_rounds, all_refused_rounds, ", ".join(f"{k} {n}" for k, n in kc.most_common()), cutoffs])
            tot["streaks"] += len(streaks); tot["stall_rounds"] += stall_rounds; tot["stall_tokens"] += stall_tokens; tot["nudged"] += len(nudged)
            tot["rewound"] += len(rewound); tot["rewinds"] += len(rewinds); tot["refs"] += len(refs); tot["refused_rounds"] += refused_rounds
            tot["all_refused_rounds"] += all_refused_rounds; tot["cutoffs"] += cutoffs
    P(md_table(["log", "task", "rounds", "stall streaks", "rounds without progress", "tokens in them (k)", "streaks reaching the nudge (10)", "reaching 20", "rewinds written (round, cause)", "rounds from nudge to recovery", "refused calls", "rounds with a refusal", "rounds where every call was refused", "refusal kinds", "cut off at cap"], rows))
    P(f"\nTotals: {dict(tot)}")
    P("\nRefusal kinds, all logs: " + ", ".join(f"{k} {n}" for k, n in refusal_kinds.most_common()))
    P("Refusals by tool: " + ", ".join(f"{k} {n}" for k, n in refusal_by_tool.most_common()))

    # nudge text detail: what did the model do in the rounds after a nudge
    P("\n### T3b. Stall streaks of 10+ rounds: what the rounds were doing\n")
    rows = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            cur = []
            streaks = []
            for i, r in enumerate(rs):
                if r.progress > 0:
                    cur.append((i, r))
                else:
                    if cur: streaks.append(cur)
                    cur = []
            if cur: streaks.append(cur)
            for s in streaks:
                mx = max(r.progress for _, r in s)
                if mx < 10:
                    continue
                tools = Counter()
                for _, r in s:
                    tools.update(r.tools())
                first_i, last_i = s[0][0] + 1, s[-1][0] + 1
                ended = "reset" if (s[-1][0] + 1 < len(rs)) else "task end"
                rows.append([db, t, f"{first_i}-{last_i}", len(s), mx, fmt(sum(r.uncached + r.out for _, r in s) / 1000, 0), fmt(sum(r.wall or 0 for _, r in s) / 60, 0), ", ".join(f"{n} {c}" for n, c in tools.most_common(6)), ended])
    P(md_table(["log", "task", "rounds", "length", "max count", "tokens (k)", "minutes", "tools in the streak", "ended by"], rows))

    # ---------- 4. record size ----------
    P("\n## T4. Record size over a task's life (chars/4)\n")
    rows = []
    share_all = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = [r for r in real_rounds(tasks[t]) if r.sec]
            if not rs:
                continue
            first, last = rs[0].sec, rs[-1].sec
            mx = max(r.sec["total"] for r in rs)
            shares = [r.sec["total"] / r.tin for r in rs if r.tin > 0]
            share_all += shares
            tail = [r.tin - r.sec["total"] for r in rs if r.tin > 0]
            rows.append([db, t, len(rs), fmt(first["total"] / 1000, 2), fmt(last["total"] / 1000, 2), fmt(mx / 1000, 2),
                         fmt(first["goal"] / 1000, 2), fmt(last["goal"] / 1000, 2),
                         fmt(first["situation"] / 1000, 2), fmt(last["situation"] / 1000, 2),
                         fmt(first["plan"] / 1000, 2), fmt(last["plan"] / 1000, 2), last["plan_lines"],
                         fmt(first["results"] / 1000, 2), fmt(last["results"] / 1000, 2), max(r.sec["result_lines"] for r in rs),
                         fmt(first["failures"] / 1000, 2), fmt(last["failures"] / 1000, 2), last["failure_lines"],
                         fmt(100 * statistics.mean(shares), 1) if shares else "-", fmt(statistics.median(tail) / 1000, 1) if tail else "-"])
    P(md_table(["log", "task", "rounds", "record first (k)", "last (k)", "max (k)", "goal first", "last", "situation first", "last", "plan first", "last", "plan lines", "results first", "last", "max result lines", "failures first", "last", "failure lines", "record % of tokens in (mean)", "tokens in minus record, median (k)"], rows))
    P(f"\nAcross all task rounds: record is {100*statistics.mean(share_all):.1f}% of tokens in on average (median {100*statistics.median(share_all):.1f}%).")
    # the fixed prefix estimate: minimum tokens in per log
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        ins = [r.tin for t in order for r in real_rounds(tasks[t]) if r.tin > 0]
        firsts = [real_rounds(tasks[t])[0].tin for t in order if real_rounds(tasks[t]) and real_rounds(tasks[t])[0].tin > 0]
        P(f"- {db}: smallest tokens in of any round {min(ins)/1000:.1f}k; first rounds of tasks: median {statistics.median(firsts)/1000:.1f}k, min {min(firsts)/1000:.1f}k, max {max(firsts)/1000:.1f}k; largest {max(ins)/1000:.1f}k")

    # ---------- 5. replies ----------
    P("\n## T5. Output per round\n")
    q = quantiles(all_out)
    P(f"All task rounds: out tokens min {q[0]:.0f}, median {q[1]:.0f}, mean {q[2]:.0f}, p90 {q[3]:.0f}, max {q[4]:.0f}.")
    buckets = [(0, 100), (100, 200), (200, 400), (400, 800), (800, 1600), (1600, 3200), (3200, 8100), (8100, 10 ** 9)]
    rows = []
    for lo, hi in buckets:
        n = sum(1 for o in all_out if lo <= o < hi)
        rows.append([f"{lo}-{hi if hi < 10**9 else 'cap'}", n, fmt(pct(n, len(all_out)), 0), fmt(sum(o for o in all_out if lo <= o < hi) / 1000, 0)])
    P(md_table(["out tokens", "rounds", "%", "out tokens in bucket (k)"], rows))
    # by round kind
    groups = defaultdict(list)
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            for r in real_rounds(tasks[t]):
                w = r.writes()
                k = ("write" if "write" in w else "edit" if "edit" in w else "task only" if r.only_task() else "poll only" if r.only_polls() else "no call" if not r.calls else "other tool")
                groups[k].append((r.out, r.wall or 0))
    rows = []
    for k, xs in sorted(groups.items(), key=lambda kv: -len(kv[1])):
        q = quantiles([o for o, _ in xs])
        rows.append([k, len(xs), fmt(q[1], 0), fmt(q[2], 0), fmt(q[3], 0), fmt(q[4], 0), fmt(statistics.median(w for _, w in xs), 0)])
    P("")
    P(md_table(["round kind", "rounds", "out med", "mean", "p90", "max", "wall s med"], rows))
    # cutoffs
    rows = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            for i, r in enumerate(real_rounds(tasks[t])):
                if r.out >= 8100:
                    rows.append([db, t, i + 1, fmt(r.out / 1000, 1), fmt(r.wall or 0, 0), ", ".join(f"{n} {c}" for n, c in r.tools().most_common()) or "no call"])
    P("\nRounds cut off at the output cap:\n")
    P(md_table(["log", "task", "round", "out (k)", "wall s", "calls"], rows) if rows else "none")

    # ---------- 6. browser ----------
    P("\n## T6. Browser and desktop use\n")
    rows = []
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            c = Counter()
            for r in rs:
                for n, i, _, _ in r.calls:
                    if n in BROWSER_TOOLS:
                        c[n + ("/" + i.get("action", "") if n == "computer" else "")] += 1
            if not c:
                continue
            shots = sum(r.screenshots() for r in rs)
            refused = sum(1 for r in rs for tool, _ in r.refusals() if tool in BROWSER_TOOLS)
            # delta in after a screenshot round vs other rounds
            d_shot, d_other = [], []
            for i in range(1, len(rs)):
                p, r = rs[i - 1], rs[i]
                if p.tin <= 0 or r.tin <= 0 or r.cached == 0:
                    continue
                delta = r.tin - p.tin - p.out - p.result_chars() / 4
                (d_shot if p.screenshots() else d_other).append(delta)
            rows.append([db, t, sum(c.values()), ", ".join(f"{n} {k}" for n, k in c.most_common()), shots, refused,
                         fmt(statistics.median(d_shot), 0) if d_shot else "-", len(d_shot), fmt(statistics.median(d_other), 0) if d_other else "-", len(d_other)])
    P(md_table(["log", "task", "browser/desktop calls", "by tool", "screenshot calls", "refused", "Δin after a screenshot round, residual med", "n", "Δin after other rounds, residual med", "n"], rows))
    P("\n(residual = tokens in this round − tokens in last round − last round's out − last round's result text/4; what is left is the picture and the record's own growth)")

    # ---------- waste table ----------
    P("\n## T9. What each behaviour pattern cost (rounds, marginal tokens = uncached in + out, minutes of wall time)\n")
    patterns = defaultdict(lambda: [0, 0.0, 0.0])
    def add(name, r):
        patterns[name][0] += 1; patterns[name][1] += r.uncached + r.out; patterns[name][2] += (r.wall or 0) / 60
    for db in DBS:
        tasks, order, _ = all_tasks[db]
        for t in order:
            rs = real_rounds(tasks[t])
            seen_reads, seen_shell = set(), set()
            last_fresh_msg = -99
            for i, r in enumerate(rs):
                add("all rounds", r)
                if r.only_task(): add("bookkeeping-only rounds (only `task` calls)", r)
                if r.only_polls(): add("poll-only rounds (shell poll/tail)", r)
                if r.calls and all(n == "read" and re.fullmatch(r'r\d+', str(inp.get("path", ""))) for n, inp, _, _ in r.calls): add("rounds that only read result ids (rN)", r)
                dup_read = False; dup_shell = False
                for n, inp, _, _ in r.calls:
                    if n == "read" and not re.fullmatch(r'r\d+', str(inp.get("path", ""))):
                        key = json.dumps(inp, sort_keys=True)
                        if key in seen_reads: dup_read = True
                        seen_reads.add(key)
                for c in r.shell_runs():
                    if c in seen_shell: dup_shell = True
                    seen_shell.add(c)
                if dup_read: add("rounds with a repeated file read (identical read input seen before in the task)", r)
                if dup_shell: add("rounds with a repeated shell command (identical command seen before in the task)", r)
                if r.test_runs():
                    add("rounds where the model ran the tests itself", r)
                    prev_text = " ".join(t2 or "" for _, _, t2, _ in (rs[i - 1].results if i else [])) + " ".join(t2 or "" for _, _, t2, _ in r.results)
                    if "tests after this change" in prev_text: add("of which right after the harness had already rerun them", r)
                refs = r.refusals()
                if refs and all(REFUSED.match(s2 or "") for _, s2, _, _ in r.results): add("rounds where every call was refused", r)
                if r.progress > 0: add("rounds inside a stall streak (rounds since progress > 0)", r)
                if r.progress >= 10: add("of which past the nudge (count 10 or more)", r)
                if r.out >= 8100: add("rounds cut off at the 8,192 output cap", r)
                if i > 0 and r.tin < 0.6 * rs[i - 1].tin and r.messages: last_fresh_msg = i
                if 0 <= i - last_fresh_msg <= 2: add("first 3 rounds after a resume on the person's message (re-orientation)", r)
                if r.screenshots(): add("rounds with a screenshot call", r)
                if not r.calls: add("rounds with no tool call", r)
    rows = [[k, v[0], fmt(v[1] / 1000, 0), fmt(v[2], 0)] for k, v in patterns.items()]
    P(md_table(["pattern", "rounds", "uncached in + out (k tokens)", "minutes"], rows))

    # ---------- per job ----------
    P("\n## T7. Per job\n")
    rows = []
    for db in DBS:
        tasks, order, evs = all_tasks[db]
        # job -> tasks via checkpoints "# job N ... " text listing? use askFrom and job checkpoints count; simpler: tasks with a t-number message ID "2.t1"
        job_of = {}
        for s, o, t, k, j in evs:
            if k == "message" and t and not t.startswith("j"):
                mid = j.get("ID", "")
                m = re.match(r'(\d+)\.t\d+', mid)
                if m:
                    job_of[t] = "j" + m.group(1)
        jobs = defaultdict(list)
        for t in order:
            jobs[job_of.get(t, "ask (no job)")].append(t)
        jrounds = Counter(1 for s, o, t, k, j in evs if k == "checkpoint" and t.startswith("j"))
        for jname, ts_ in jobs.items():
            rs = [r for t in ts_ for r in real_rounds(tasks[t])]
            if not rs:
                continue
            walls = [r.wall for r in rs if r.wall is not None]
            tin = sum(r.tin for r in rs); cached = sum(r.cached for r in rs); out = sum(r.out for r in rs)
            jck = sum(1 for s, o, t, k, j in evs if k == "checkpoint" and t == jname)
            rows.append([db, jname, ",".join(ts_), len(ts_), len(rs), jck, fmt(sum(walls) / 60, 0), fmt(tin / 1000, 0), fmt(pct(cached, tin), 0), fmt((tin - cached) / 1000, 0), fmt(out / 1000, 0)])
    P(md_table(["log", "job", "tasks", "n tasks", "task rounds", "job checkpoints (no model call)", "minutes", "tokens in (k)", "cached %", "uncached (k)", "out (k)"], rows))

    # ---------- messages / replies / continue ----------
    P("\n## T8. Turn ends and the person's messages\n")
    rows = []
    for db in DBS:
        tasks, order, evs = all_tasks[db]
        for t in order:
            msgs = [(o, snd, txt) for r in tasks[t] for o, snd, txt in r.messages]
            replies = [x for r in tasks[t] for x in r.replies]
            conts = sum(1 for _, _, txt in msgs if txt.strip().lower().startswith("continue"))
            if len(msgs) > 1 or replies:
                rows.append([db, t, len(replies), len(msgs) - 1, conts, "; ".join(txt[:50].replace("\n", " ").replace("|", "/") for _, _, txt in msgs[1:])[:200]])
    P(md_table(["log", "task", "replies (turn ends)", "mid-task messages", "of which 'continue'", "messages"], rows))

    # ---------- per-round dump for a few tasks (for the appendix) ----------
    open("rounds.tsv", "w").write("\n".join(
        "\t".join(str(x) for x in [db, t, i + 1, r.occurred, fmt(r.wall or 0, 0), fmt(r.tin, 0), fmt(r.cached, 0), fmt(r.out, 0), r.progress, fmt(r.sec["total"], 0) if r.sec else 0, r.sec["result_lines"] if r.sec else 0, ",".join(f"{n}:{c}" for n, c in r.tools().most_common()), len(r.refusals()), len(r.messages), len(r.replies)])
        for db in DBS for t in all_tasks[db][1] for i, r in enumerate(real_rounds(all_tasks[db][0][t]))))
    sys.stdout.write("\n".join(report) + "\n")

if __name__ == "__main__":
    main()
