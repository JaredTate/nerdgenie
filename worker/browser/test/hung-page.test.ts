import { afterAll, beforeAll, describe, expect, it } from "vitest";
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

// On the fifth game build the Start button ran a loop that never yielded, the
// worker could not scan the page, and the error told the model to open the
// page again. It did, clicked again, and hung again. A page whose own script
// keeps it busy is a bug in the page, and the error has to say so.
describe("a page whose script never yields after a click", () => {
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

  it("says the page's own script is what keeps it busy, and not to open it again", async () => {
    const page = await worker.result("open", { url: site.page("hangs-on-click.html") });
    const failure = await worker.fails("click", {
      ref: refFor(page, "Start Game"),
      expectation: "the game starts",
    });
    expect(failure.message).toContain("the page's own script");
    expect(failure.message).toContain("does not yield");
    expect(failure.message).not.toContain("Open the page again");
  }, 90_000);
});
