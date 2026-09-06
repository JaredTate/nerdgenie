import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { isAPageOnThisMachine } from "../src/ask.js";
import { MAX_ANSWER_CHARS, MAX_ASK_CHARS } from "../src/limits.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { SnapshotElement } from "../src/types.js";

function refFor(snapshot: Record<string, unknown>, name: string): string {
  const found = (snapshot["elements"] as SnapshotElement[]).find(
    (element) => element.name === name,
  );
  if (found === undefined) {
    throw new Error(`no element named ${name} in ${JSON.stringify(snapshot["elements"])}`);
  }
  return found.ref;
}

// On the fifth game build the model wrote six play-test drivers through the
// shell to read window.__engine.state from its own game, because no browser
// tool would answer a question about the page. The read method takes one
// expression and answers it, on a page served from this machine only, because
// the browser holds the person's logins and a script on any other page is
// never run.
describe("asking a page a question", () => {
  let worker: TestWorker;
  let site: FixtureServer;

  beforeAll(async () => {
    site = await startFixtureServer();
    worker = await startTestWorker();
  });

  afterAll(async () => {
    await worker.stop();
    await site.stop();
  });

  it("answers with the value of the expression, written as JSON", async () => {
    await worker.result("open", { url: site.page("game-state.html") });
    const read = await worker.result("read", { ask: "window.game.state" });
    expect(read["answer"]).toBe('"MENU"');
    const object = await worker.result("read", { ask: "window.game.board" });
    expect(object["answer"]).toBe('{"rows":20,"cols":10}');
  });

  it("sees what an action changed", async () => {
    const page = await worker.result("open", { url: site.page("game-state.html") });
    await worker.result("click", { ref: refFor(page, "Start Game"), expectation: "the game starts" });
    const read = await worker.result("read", { ask: "window.game.state + ' ' + window.game.score" });
    expect(read["answer"]).toBe('"PLAYING 42"');
  });

  it("says what the page threw, rather than failing the read", async () => {
    await worker.result("open", { url: site.page("game-state.html") });
    const read = await worker.result("read", { ask: "window.nothing.here" });
    expect(String(read["answer"])).toMatch(/^the page threw: /);
    expect(Array.isArray(read["elements"])).toBe(true);
  });

  it("cuts a long answer and says so", async () => {
    await worker.result("open", { url: site.page("game-state.html") });
    const read = await worker.result("read", { ask: "'x'.repeat(10000)" });
    expect(String(read["answer"]).length).toBeLessThanOrEqual(MAX_ANSWER_CHARS + 40);
    expect(String(read["answer"])).toContain("more characters were cut");
  });

  it("takes an ask as long as a whole play session and refuses one past the cap", async () => {
    await worker.result("open", { url: site.page("game-state.html") });
    const session = "(function(){ var n = 0; " + "n += 1; ".repeat(200) + " return n; })()";
    expect(session.length).toBeGreaterThan(1500);
    expect(session.length).toBeLessThanOrEqual(MAX_ASK_CHARS);
    const read = await worker.result("read", { ask: session });
    expect(read["answer"]).toBe("200");
    const tooLong = "'" + "x".repeat(MAX_ASK_CHARS) + "'";
    await expect(worker.result("read", { ask: tooLong })).rejects.toThrow(/cap is/);
  });

  it("carries no answer when nothing was asked", async () => {
    await worker.result("open", { url: site.page("game-state.html") });
    const read = await worker.result("read", {});
    expect(read["answer"]).toBeUndefined();
  });
});

describe("which pages may be asked", () => {
  it("is this machine and files, and nothing else", () => {
    for (const local of ["http://localhost:8091/index.html", "http://127.0.0.1:3000/", "http://[::1]:8080/x", "file:///home/jared/game/index.html", "http://localhost/"]) {
      expect(isAPageOnThisMachine(local)).toBe(true);
    }
    for (const elsewhere of ["https://x.com/home", "http://localhost.evil.com/", "https://bank.example/localhost", "about:blank", ""]) {
      expect(isAPageOnThisMachine(elsewhere)).toBe(false);
    }
  });
});
