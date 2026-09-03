// Print one markdown table per phase from every benchmark run on this machine.
//
// Usage: node tater_table.mjs [ROOT]
//   ROOT defaults to ~/work/bench/tater2, the folder tater_run.sh writes into.
//
// It reads every ROOT/<phase>-<harness>-<label>/result.json, groups them by
// phase, and prints the columns a person comparing harnesses actually wants:
// did it finish, how long it took, how many model calls it made, what those
// calls cost in tokens and dollars, how much of the wall clock was the model
// and how much was the harness, and the four quality numbers.
//
// A run that is still going has no result.json yet and is simply not listed.
// A number the phase does not measure is printed as a dash rather than a zero,
// because a zero would read as a measurement.

import { readdirSync, readFileSync, existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const root = process.argv[2] ?? join(homedir(), "work", "bench", "tater2");

if (!existsSync(root)) {
  console.error(`There is no benchmark folder at ${root}, so there is nothing to tabulate.`);
  process.exit(1);
}

// ---- gather ----------------------------------------------------------------

const runs = [];
for (const entry of readdirSync(root, { withFileTypes: true })) {
  if (!entry.isDirectory()) continue;
  const resultPath = join(root, entry.name, "result.json");
  if (!existsSync(resultPath)) continue;
  try {
    runs.push(JSON.parse(readFileSync(resultPath, "utf8")));
  } catch (trouble) {
    console.error(`Skipping ${resultPath}, which is not readable JSON: ${trouble.message}`);
  }
}

if (runs.length === 0) {
  console.error(`No run under ${root} has finished and written a result.json yet.`);
  process.exit(1);
}

// ---- how each number is written --------------------------------------------

const DASH = "—";

function whole(value) {
  if (value === null || value === undefined) return DASH;
  const number = Number(value);
  if (!Number.isFinite(number)) return DASH;
  return number.toLocaleString("en-US");
}

function money(value) {
  if (value === null || value === undefined) return DASH;
  const number = Number(value);
  if (!Number.isFinite(number)) return DASH;
  return "$" + number.toFixed(2);
}

function clock(seconds) {
  const total = Number(seconds);
  if (!Number.isFinite(total)) return DASH;
  const minutes = Math.floor(total / 60);
  const rest = Math.round(total % 60);
  return `${minutes}m ${String(rest).padStart(2, "0")}s`;
}

function fromMilliseconds(value) {
  if (value === null || value === undefined) return DASH;
  const number = Number(value);
  if (!Number.isFinite(number)) return DASH;
  return clock(number / 1000);
}

function finished(run) {
  if (run.finished) return "yes";
  if (run.cap_hit) return "no (capped)";
  return "no";
}

// ---- one table per phase ---------------------------------------------------

const COLUMNS = [
  ["harness", (run) => `${run.harness} (${run.label})`],
  ["finished", finished],
  ["wall", (run) => clock(run.wall_seconds)],
  ["calls", (run) => whole(run.calls)],
  ["tokens in", (run) => whole(run.tokens_in)],
  ["cached", (run) => whole(run.cache_read)],
  ["out", (run) => whole(run.tokens_out)],
  ["cost", (run) => money(run.cost_usd)],
  ["model time", (run) => fromMilliseconds(run.model_ms)],
  ["harness time", (run) => fromMilliseconds(run.harness_ms)],
  ["logic 6", (run) => whole(run.logic_passed)],
  ["plays 4", (run) => whole(run.plays_passed)],
  ["tests written", (run) => whole(run.tests_written)],
  ["tests passing", (run) => whole(run.tests_passing)],
];

const HARNESS_ORDER = ["coeus", "opencode", "hermes", "openclaw"];

function sortRuns(left, right) {
  const byHarness = HARNESS_ORDER.indexOf(left.harness) - HARNESS_ORDER.indexOf(right.harness);
  if (byHarness !== 0) return byHarness;
  return String(left.label).localeCompare(String(right.label));
}

const phases = [...new Set(runs.map((run) => run.phase))].sort();
const out = [];
for (const phase of phases) {
  const mine = runs.filter((run) => run.phase === phase).sort(sortRuns);
  out.push(`## ${phase} phase`, "");
  out.push("| " + COLUMNS.map(([name]) => name).join(" | ") + " |");
  out.push("| " + COLUMNS.map(() => "---").join(" | ") + " |");
  for (const run of mine) {
    out.push("| " + COLUMNS.map(([, cell]) => cell(run)).join(" | ") + " |");
  }
  out.push("");
}
console.log(out.join("\n"));
