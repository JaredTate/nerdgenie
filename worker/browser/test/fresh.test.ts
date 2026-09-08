import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { loadsFresh } from "../src/fresh.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { LIVE_PAGE, startFixtureServer, type FixtureServer } from "./server.js";
import type { SnapshotElement } from "../src/types.js";

// Run 23 lost seven minutes to a server that kept handing the browser an old
// render.js: the page was opened again and again and the old file came out of
// the cache each time. A page on this machine is what the agent is building,
// so it is loaded fresh every time; anyone else's page is left to the cache.

function headingOf(result: Record<string, unknown>): string {
  const elements = result["elements"] as SnapshotElement[];
  return elements.find((element) => element.role === "heading")?.name ?? "";
}

describe("a page on this machine is loaded fresh", () => {
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

  it("shows the new text when a cacheable local page is opened again after it changed", async () => {
    site.setLiveText("first");
    const before = await worker.result("open", { url: site.page(LIVE_PAGE) });
    expect(headingOf(before)).toBe("first");

    site.setLiveText("second");
    const after = await worker.result("open", { url: site.page(LIVE_PAGE) });
    expect(headingOf(after)).toBe("second");
  });

  it("knows which addresses are this machine's and which are not", () => {
    for (const local of ["http://127.0.0.1:8097/", "http://localhost/game", "http://[::1]:3000/", "file:///home/user/index.html"]) {
      expect(loadsFresh(local), local).toBe(true);
    }
    for (const elsewhere of ["https://example.com/", "http://192.168.1.10/", "not a url", "mailto:a@b.c"]) {
      expect(loadsFresh(elsewhere), elsewhere).toBe(false);
    }
  });
});
