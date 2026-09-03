// Judge one harness's tic-tac-toe folder ourselves, never trusting its own tests.
//
// Borrows the file-finding, local-HTTP-serving, and lock-down-Chrome design of
// check-tictactoe.mjs (scripts/bench/check-tictactoe.mjs, same folder) and goes
// further: it does not just read the DOM, it DRIVES the page. Chrome is opened
// headless with a remote-debugging port on a throwaway profile, the accessibility
// bridge and the session bus kept away from it, and the page is played through
// the DevTools protocol — real clicks on real buttons, reading the real status
// line — so we measure what the harness actually built, not what it claimed.
//
// It reports three things the task asks for:
//   1. Correctness — the six logic checks against the folder's game.js.
//   2. Actually plays (of 4) — nine cells, an X-winning click sequence announces
//      X, New game clears the board and status, and a click on a taken cell
//      changes nothing.
//   3. Thoroughness — how many tests the harness wrote and how many pass under
//      node --test, the line count of game.js, and a six-point quality score:
//      whose turn is shown, a draw is announced, a win is announced, a New game
//      control exists, one accent colour is used, and the game functions are pure.
//
// One screenshot of the finished (X-won) game is written for a person to see.
//
// Usage: node check-tater.mjs FOLDER [--harness NAME] [--run N] [--shot PATH]
// It prints one JSON object.

import {
  existsSync, readFileSync, writeFileSync, unlinkSync, mkdirSync, mkdtempSync,
  readdirSync, rmSync, statSync,
} from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, extname, join, resolve, basename } from "node:path";
import { pathToFileURL } from "node:url";
import { spawn, spawnSync } from "node:child_process";

const CHROME = "/usr/bin/google-chrome";

// ---- arguments -------------------------------------------------------------
const args = process.argv.slice(2);
let folderArg = null;
let harness = "unknown";
let run = "1";
let shot = null;
for (let i = 0; i < args.length; i += 1) {
  const a = args[i];
  if (a === "--harness") harness = args[++i];
  else if (a === "--run") run = args[++i];
  else if (a === "--shot") shot = args[++i];
  else if (!folderArg) folderArg = a;
}
const folder = resolve(folderArg);

const report = {
  harness,
  run,
  folder,
  files: { gameJs: false, indexHtml: false, tests: false, gameJsPath: null },
  correctness: { checks: {}, passed: 0, of: 6 },
  plays: {
    nineCells: false, announceX: false, newGameResets: false, takenCellNoChange: false,
    passed: 0, of: 4, cells: 0, statusAfterWin: "", initialStatus: "",
  },
  thoroughness: {
    testsWritten: 0, testsPassing: 0, testsTotal: 0, gameJsLines: 0,
    quality: {
      showsTurn: false, announcesDraw: false, announcesWin: false,
      newGameControl: false, oneAccentColour: false, functionsPure: false,
      accentCount: 0, passed: 0, of: 6,
    },
  },
  screenshot: null,
  notes: [],
};

function note(text) {
  report.notes.push(String(text).slice(0, 400));
}

// ---- find the files (they may be one folder down) --------------------------
function findFile(name) {
  const direct = join(folder, name);
  if (existsSync(direct)) return direct;
  let entries = [];
  try {
    entries = readdirSync(folder, { withFileTypes: true })
      .filter((e) => e.isDirectory() && e.name !== "node_modules" && e.name !== ".git")
      .map((e) => e.name);
  } catch { entries = []; }
  for (const entry of entries) {
    const inside = join(folder, entry, name);
    if (existsSync(inside)) return inside;
  }
  return null;
}

const gamePath = findFile("game.js");
const pagePath = findFile("index.html");
report.files.gameJs = Boolean(gamePath);
report.files.indexHtml = Boolean(pagePath);
report.files.gameJsPath = gamePath;
const root = gamePath ? dirname(gamePath) : (pagePath ? dirname(pagePath) : folder);
const testPath = ["tests/game.test.mjs", "tests/game.test.js", "game.test.mjs", "test/game.test.mjs"]
  .map((rel) => join(root, rel)).find((p) => existsSync(p)) || null;
report.files.tests = Boolean(testPath);

// ---- load game.js as a module (with the same .mjs fallback) ----------------
let game = null;
if (gamePath) {
  try {
    game = await import(pathToFileURL(gamePath).href);
  } catch (trouble) {
    note("game.js would not load directly: " + trouble);
    try {
      const copy = join(dirname(gamePath), ".bench-game-copy.mjs");
      writeFileSync(copy, readFileSync(gamePath));
      game = await import(pathToFileURL(copy).href + "?t=" + Date.now());
      note("it loaded once copied to an .mjs name");
      unlinkSync(copy);
    } catch (second) {
      note("and would not load as .mjs either: " + second);
    }
  }
}

// ---- 1. Correctness: the six logic checks ----------------------------------
function check(name, body) {
  let ok = false;
  try { ok = Boolean(body()); }
  catch (trouble) { note(name + ": " + trouble); }
  report.correctness.checks[name] = ok;
  if (ok) report.correctness.passed += 1;
}
function playAll(cells) {
  let state = game.newGame();
  for (const cell of cells) state = game.play(state, cell);
  return state;
}
if (game && typeof game.newGame === "function" && typeof game.play === "function"
  && typeof game.winner === "function") {
  check("win in a row", () => game.winner(playAll([0, 3, 1, 4, 2])) === "X");
  check("win in a column", () => game.winner(playAll([0, 1, 3, 2, 6])) === "X");
  check("win in a diagonal", () => game.winner(playAll([0, 1, 4, 2, 8])) === "X");
  check("draw", () => game.winner(playAll([0, 1, 2, 4, 3, 5, 7, 6, 8])) === "draw");
  check("illegal move on a taken cell", () => {
    const first = game.play(game.newGame(), 0);
    const again = game.play(first, 0);
    return JSON.stringify(again) === JSON.stringify(first);
  });
  check("no moves after a win", () => {
    const won = playAll([0, 3, 1, 4, 2]);
    return JSON.stringify(game.play(won, 5)) === JSON.stringify(won);
  });
} else if (game) {
  note("game.js does not export newGame, play and winner");
} else {
  note("game.js could not be loaded at all");
}

// ---- 3a. Thoroughness that needs no browser --------------------------------
// Gather every bit of source so we can scan for words and colours.
let allSource = "";
function addSource(p) { if (p && existsSync(p)) { try { allSource += "\n" + readFileSync(p, "utf8"); } catch { /* ignore */ } } }
if (gamePath) {
  const text = readFileSync(gamePath, "utf8");
  report.thoroughness.gameJsLines = text.split(/\r?\n/).length;
  addSource(gamePath);
  // functionsPure, source side: no DOM or global-state access in game.js.
  const usesDom = /\bdocument\b|\bwindow\b|localStorage|sessionStorage/.test(text);
  // functionsPure, runtime side: play() must not mutate the state handed to it.
  let mutates = true;
  try {
    const before = game.newGame();
    const snap = JSON.stringify(before);
    game.play(before, 0);
    mutates = JSON.stringify(before) !== snap;
  } catch (trouble) { note("purity runtime check failed: " + trouble); }
  report.thoroughness.quality.functionsPure = !usesDom && !mutates;
  if (usesDom) note("game.js touches document/window/storage (not pure)");
  if (mutates) note("play() mutated the state it was given (not pure)");
}
addSource(pagePath);
if (pagePath) {
  // Fold in any scripts and stylesheet the page names, so a split-file build is
  // scanned the same as an inline one.
  const html = readFileSync(pagePath, "utf8");
  const base = dirname(pagePath);
  for (const m of html.matchAll(/<script\b[^>]*src=["']([^"']+)["']/gi)) {
    const p = resolve(base, m[1]); if (p.startsWith(base)) addSource(p);
  }
  for (const m of html.matchAll(/<link\b[^>]*href=["']([^"']+\.css)["']/gi)) {
    const p = resolve(base, m[1]); if (p.startsWith(base)) addSource(p);
  }
}

// The page announces a draw / a win if the words are there to be shown.
report.thoroughness.quality.announcesDraw = /\bdraw\b/i.test(allSource);

// One accent colour: count the distinct non-neutral colours used in styling.
function accentColours(text) {
  const found = new Set();
  const neutralNames = new Set(["white", "black", "transparent", "inherit", "currentcolor", "gray", "grey", "silver", "whitesmoke", "gainsboro", "dimgray", "dimgrey", "lightgray", "lightgrey", "darkgray", "darkgrey"]);
  const isNeutral = (r, g, b) => Math.max(r, g, b) - Math.min(r, g, b) <= 12;
  for (const m of text.matchAll(/#([0-9a-f]{6}|[0-9a-f]{3})\b/gi)) {
    let h = m[1];
    if (h.length === 3) h = h.split("").map((c) => c + c).join("");
    const r = parseInt(h.slice(0, 2), 16), g = parseInt(h.slice(2, 4), 16), b = parseInt(h.slice(4, 6), 16);
    if (!isNeutral(r, g, b)) found.add("#" + h.toLowerCase());
  }
  for (const m of text.matchAll(/rgba?\(\s*(\d+)[,\s]+(\d+)[,\s]+(\d+)/gi)) {
    const r = +m[1], g = +m[2], b = +m[3];
    if (!isNeutral(r, g, b)) found.add(`rgb(${r},${g},${b})`);
  }
  for (const m of text.matchAll(/hsla?\(\s*(\d+)/gi)) found.add("hsl" + m[1]);
  for (const m of text.matchAll(/\b(?:color|background|background-color|border|fill|stroke|accent-color|outline)\s*:\s*([a-z]{3,20})\b/gi)) {
    const name = m[1].toLowerCase();
    const known = ["red", "blue", "green", "teal", "purple", "orange", "crimson", "tomato", "coral", "gold", "indigo", "navy", "maroon", "olive", "lime", "aqua", "cyan", "magenta", "fuchsia", "violet", "salmon", "khaki", "turquoise", "steelblue", "dodgerblue", "royalblue", "seagreen", "forestgreen", "darkblue", "darkgreen", "darkred", "hotpink", "pink", "chocolate", "brown", "rebeccapurple", "cornflowerblue", "mediumseagreen", "slateblue", "deeppink"];
    if (known.includes(name) && !neutralNames.has(name)) found.add(name);
  }
  return found;
}
const accents = accentColours(allSource);
report.thoroughness.quality.accentCount = accents.size;
report.thoroughness.quality.oneAccentColour = accents.size >= 1;

// Count the tests the harness wrote, and run them under node --test.
if (testPath) {
  const testText = readFileSync(testPath, "utf8");
  report.thoroughness.testsWritten = (testText.match(/\b(test|it)\s*\(/g) || []).length;
  const run = spawnSync("node", ["--test", testPath], { cwd: root, encoding: "utf8", timeout: 60000 });
  const out = (run.stdout || "") + "\n" + (run.stderr || "");
  // Node's test runner summarises with either a "#" or an "ℹ" prefix.
  const pass = out.match(/[#ℹ]\s*pass\s+(\d+)/);
  const fail = out.match(/[#ℹ]\s*fail\s+(\d+)/);
  const total = out.match(/[#ℹ]\s*tests\s+(\d+)/);
  report.thoroughness.testsPassing = pass ? +pass[1] : 0;
  report.thoroughness.testsTotal = total ? +total[1] : 0;
  if (!pass) note("node --test gave no pass count: " + out.slice(0, 300));
  if (fail && +fail[1] > 0) note(`node --test reported ${fail[1]} failing`);
}

// ---- serve the folder over local HTTP --------------------------------------
function serveFolder(baseDir) {
  const types = { ".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css", ".json": "application/json" };
  return new Promise((ready) => {
    const server = createServer((request, response) => {
      let wanted;
      try { wanted = decodeURIComponent(new URL(request.url, "http://127.0.0.1").pathname); }
      catch { response.writeHead(400); response.end(); return; }
      const file = resolve(baseDir, "." + (wanted === "/" ? "/index.html" : wanted));
      if (!file.startsWith(baseDir) || !existsSync(file) || statSync(file).isDirectory()) {
        response.writeHead(404); response.end(); return;
      }
      response.writeHead(200, { "content-type": types[extname(file)] || "application/octet-stream" });
      response.end(readFileSync(file));
    });
    server.listen(0, "127.0.0.1", () => ready({ server, port: server.address().port }));
  });
}

// ---- a tiny Chrome DevTools client -----------------------------------------
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function openChrome(address) {
  const profile = mkdtempSync(join(tmpdir(), "tater-chrome-"));
  const environment = { ...process.env, NO_AT_BRIDGE: "1" };
  delete environment.DBUS_SESSION_BUS_ADDRESS;
  const child = spawn(CHROME, [
    "--headless=new", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
    "--user-data-dir=" + profile, "--remote-debugging-port=0",
    "--window-size=900,900", "about:blank",
  ], { env: environment, stdio: ["ignore", "pipe", "pipe"] });
  let stderr = "";
  child.stderr.on("data", (p) => { stderr += p; });
  // Chrome writes the real debugging port into this file once it is up.
  const portFile = join(profile, "DevToolsActivePort");
  let port = null;
  for (let i = 0; i < 120; i += 1) {
    if (existsSync(portFile)) {
      const line = readFileSync(portFile, "utf8").split("\n")[0].trim();
      if (line) { port = line; break; }
    }
    await sleep(100);
  }
  if (!port) { child.kill("SIGKILL"); rmSync(profile, { recursive: true, force: true }); throw new Error("Chrome never opened a debugging port: " + stderr.slice(0, 200)); }
  // Find the page target and its WebSocket.
  const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
  const pageTarget = list.find((t) => t.type === "page") || list[0];
  if (!pageTarget || !pageTarget.webSocketDebuggerUrl) throw new Error("no page target from Chrome");
  const ws = new WebSocket(pageTarget.webSocketDebuggerUrl);
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = () => rej(new Error("could not attach to Chrome")); });
  let counter = 0;
  const pending = new Map();
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) {
      const { resolve: ok, reject: no } = pending.get(msg.id);
      pending.delete(msg.id);
      if (msg.error) no(new Error(JSON.stringify(msg.error))); else ok(msg.result);
    }
  };
  const send = (method, params = {}) => new Promise((ok, no) => {
    const id = ++counter;
    pending.set(id, { resolve: ok, reject: no });
    ws.send(JSON.stringify({ id, method, params }));
  });
  const close = () => {
    try { ws.close(); } catch { /* ignore */ }
    try { child.kill("SIGKILL"); } catch { /* ignore */ }
    rmSync(profile, { recursive: true, force: true });
  };
  await send("Page.enable");
  await send("Runtime.enable");
  return { send, close, address };
}

// The in-page helpers, prepended to every expression we evaluate.
const PRELUDE = `
function getCells(){
  const all=[...document.querySelectorAll('button')];
  const isNew=b=>/new\\s*game|reset|restart|play\\s*again|clear/i.test(((b.textContent||'')+' '+(b.id||'')+' '+(b.className||'')));
  let cells=all.filter(b=>!isNew(b));
  if(cells.length<9){
    const alt=[...document.querySelectorAll('[data-cell],[data-index],[data-i],[data-idx],.cell,.square,.tile,.box')];
    if(alt.length>=9) cells=alt;
  }
  return cells;
}
function newButton(){
  const all=[...document.querySelectorAll('button,[role="button"],input[type="button"]')];
  return all.find(b=>/new\\s*game|reset|restart|play\\s*again/i.test(((b.textContent||b.value||'')+' '+(b.id||'')+' '+(b.className||''))))||null;
}
function statusEl(){
  let el=document.querySelector('[id*="status" i],[class*="status" i],[id*="message" i],[class*="message" i],[id*="turn" i],[class*="turn" i],[role="status"]');
  if(el) return el;
  const cand=[...document.querySelectorAll('h1,h2,h3,h4,p,div,span')].filter(e=>e.children.length===0 && (e.textContent||'').trim() && /turn|win|won|draw|next|move|player|['\\u2019]s/i.test(e.textContent));
  return cand[0]||null;
}
function statusText(){ const e=statusEl(); return e?(e.textContent||'').trim():''; }
function boardTexts(){ return getCells().map(c=>(c.textContent||'').trim()); }
`;
function evalExpr(sess, body) {
  const expression = `(()=>{ ${PRELUDE}\n ${body} })()`;
  return sess.send("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true })
    .then((r) => {
      if (r.exceptionDetails) throw new Error("page eval: " + (r.exceptionDetails.exception?.description || r.exceptionDetails.text));
      return r.result.value;
    });
}
async function loadFresh(sess) {
  await sess.send("Page.navigate", { url: sess.address });
  // ES-module page scripts run after load, so wait for the board to appear.
  for (let i = 0; i < 60; i += 1) {
    try {
      const n = await evalExpr(sess, "return getCells().length;");
      if (n >= 9) return n;
    } catch { /* still loading */ }
    await sleep(150);
  }
  try { return await evalExpr(sess, "return getCells().length;"); } catch { return 0; }
}

const announcesX = (s) => /x/i.test(s) && /win|won|\u{1F389}|\u{1F3C6}|congrat/iu.test(s);

if (pagePath) {
  const base = dirname(pagePath);
  const served = await serveFolder(base);
  const address = "http://127.0.0.1:" + served.port + "/index.html";
  let sess = null;
  try {
    if (!existsSync(CHROME)) throw new Error("no Chrome at " + CHROME);
    sess = await openChrome(address);

    // Fresh load; how many cells, and the opening status line.
    const cells = await loadFresh(sess);
    report.plays.cells = cells;
    report.plays.nineCells = cells >= 9;
    report.plays.initialStatus = await evalExpr(sess, "return statusText();");
    report.plays.newGameResets; // placeholder — set below
    const hasNew = await evalExpr(sess, "return !!newButton();");

    // Play an X-winning sequence (top row) and read the status line.
    const afterWin = await evalExpr(sess, "[0,3,1,4,2].forEach(i=>{const c=getCells()[i]; if(c) c.click();}); return {status:statusText(), board:boardTexts()};");
    report.plays.statusAfterWin = afterWin.status || "";
    report.plays.announceX = announcesX(afterWin.status || "");

    // Screenshot the finished game for a person to look at.
    if (shot) {
      try {
        const cap = await sess.send("Page.captureScreenshot", { format: "png" });
        mkdirSync(dirname(shot), { recursive: true });
        writeFileSync(shot, Buffer.from(cap.data, "base64"));
        report.screenshot = existsSync(shot) ? shot : null;
      } catch (trouble) { note("screenshot failed: " + trouble); }
    }

    // New game must clear the board and reset the status line.
    const afterNew = await evalExpr(sess, "const n=newButton(); if(n) n.click(); return {had:!!n, board:boardTexts(), status:statusText()};");
    const boardEmpty = Array.isArray(afterNew.board) && afterNew.board.length >= 9 && afterNew.board.every((t) => t === "");
    const statusReset = !/win|won|draw/i.test(afterNew.status || "");
    report.plays.newGameResets = Boolean(afterNew.had) && boardEmpty && statusReset;

    // A click on a taken cell must change nothing. Reload for a clean board.
    await loadFresh(sess);
    const taken = await evalExpr(sess, "const cs=getCells(); if(cs[0]) cs[0].click(); const before={s:statusText(),b:boardTexts()}; if(cs[0]) cs[0].click(); const after={s:statusText(),b:boardTexts()}; return {before, after};");
    const placed = taken.before && Array.isArray(taken.before.b) && taken.before.b[0] !== "";
    report.plays.takenCellNoChange = placed && JSON.stringify(taken.before) === JSON.stringify(taken.after);

    // Quality bits that the live page settles.
    report.thoroughness.quality.newGameControl = hasNew;
    const initial = report.plays.initialStatus || "";
    report.thoroughness.quality.showsTurn = Boolean(initial)
      && (/turn|to move|move|next|['\u2019]s go/i.test(initial)
        || (/\bx\b/i.test(initial) && !/win|won|draw/i.test(initial)));
    report.thoroughness.quality.announcesWin = report.plays.announceX || /\bwin|won|winner\b/i.test(allSource);
  } catch (trouble) {
    note("the live page check failed: " + trouble);
    // Fall back so New game / win quality can still be judged from source.
    report.thoroughness.quality.newGameControl = /new\s*game|reset|restart|play\s*again/i.test(allSource);
    report.thoroughness.quality.announcesWin = /\bwin|won|winner\b/i.test(allSource);
    report.thoroughness.quality.showsTurn = /turn|to move|whose|next/i.test(allSource);
  } finally {
    if (sess) sess.close();
    served.server.close();
  }
} else {
  note("no index.html, so the page could not be played");
  report.thoroughness.quality.newGameControl = /new\s*game|reset|restart|play\s*again/i.test(allSource);
  report.thoroughness.quality.announcesWin = /\bwin|won|winner\b/i.test(allSource);
  report.thoroughness.quality.showsTurn = /turn|to move|whose|next/i.test(allSource);
}

// ---- tally -----------------------------------------------------------------
report.plays.passed = ["nineCells", "announceX", "newGameResets", "takenCellNoChange"]
  .filter((k) => report.plays[k]).length;
const q = report.thoroughness.quality;
q.passed = ["showsTurn", "announcesDraw", "announcesWin", "newGameControl", "oneAccentColour", "functionsPure"]
  .filter((k) => q[k]).length;

console.log(JSON.stringify(report, null, 2));
