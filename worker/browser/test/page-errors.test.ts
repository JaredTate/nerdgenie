import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { MAX_PAGE_ERRORS } from "../src/limits.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { Diff, SnapshotElement } from "../src/types.js";

function errorsOf(result: Record<string, unknown>): string[] {
  return result["errors"] as string[];
}

function refFor(snapshot: Record<string, unknown>, name: string): string {
  const found = (snapshot["elements"] as SnapshotElement[]).find(
    (element) => element.name === name,
  );
  if (found === undefined) {
    throw new Error(`no element named ${name} in ${JSON.stringify(snapshot["elements"])}`);
  }
  return found.ref;
}

// On the live game build the page loaded its script from a path where there was
// no script, the game never started, and the model was never told: it clicked
// Start, read the menu still on the screen, and went round in circles until the
// task stopped. A person with the console open sees the 404 at once. These tests
// hold that the snapshot carries what the console shows.
describe("what went wrong on a page", () => {
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

  it("names a script and a stylesheet the page asked for and did not get, with the status", async () => {
    const page = await worker.result("open", { url: site.page("broken-page.html") });
    const errors = errorsOf(page);
    expect(errors.some((line) => /^script .*missing\.js answered 404$/.test(line))).toBe(true);
    expect(errors.some((line) => /^stylesheet .*missing\.css answered 404$/.test(line))).toBe(true);
  });

  it("names an error a script threw, in the script's own words", async () => {
    const page = await worker.result("open", { url: site.page("broken-page.html") });
    expect(errorsOf(page)).toContain("script error: the board never drew");
  });

  it("keeps them on the next read and on the next action, the way a console does", async () => {
    const page = await worker.result("open", { url: site.page("broken-page.html") });
    const again = await worker.result("read");
    expect(errorsOf(again)).toEqual(errorsOf(page));
    const diff = (await worker.result("click", {
      ref: refFor(page, "Start Game"),
      expectation: "the game starts",
    })) as unknown as Diff;
    expect(diff.snapshot.errors).toEqual(errorsOf(page));
  });

  it("forgets them when the page moves to another address", async () => {
    await worker.result("open", { url: site.page("broken-page.html") });
    const next = await worker.result("open", { url: site.page("links-and-form.html") });
    expect(errorsOf(next)).toEqual([]);
  });

  it("keeps the first few and counts the rest", async () => {
    const page = await worker.result("open", { url: site.page("many-errors.html") });
    const errors = errorsOf(page);
    expect(errors).toHaveLength(MAX_PAGE_ERRORS + 1);
    expect(errors[0]).toBe("script error: error number 1");
    expect(errors[MAX_PAGE_ERRORS]).toBe(`... and ${8 - MAX_PAGE_ERRORS} more errors`);
  });

  it("carries nothing on a page where nothing went wrong", async () => {
    const page = await worker.result("open", { url: site.page("links-and-form.html") });
    expect(errorsOf(page)).toEqual([]);
  });
});
