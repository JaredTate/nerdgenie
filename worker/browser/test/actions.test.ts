import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { ERROR_CODES } from "../src/errors.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { Diff, Snapshot, SnapshotElement } from "../src/types.js";

function asDiff(result: Record<string, unknown>): Diff {
  return result as unknown as Diff;
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

describe("clicking, typing, pressing, and scrolling", () => {
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

  it("clicks an element and reports every field the protocol names for a diff", async () => {
    const page = await worker.result("open", { url: site.page("changes-on-click.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Compose"),
        expectation: "a Post button appears",
      }),
    );
    expect(diff.urlChanged).toBe(false);
    expect(diff.url).toBe(site.page("changes-on-click.html"));
    expect(diff.dialog).toBeNull();
    expect(diff.newTab).toBe("");
    expect(diff.download).toBeNull();
    expect(diff.wall).toBeNull();
    expect(diff.expectationMet).toBe(true);
    expect(diff.seen).toBe("");
    expect(diff.newElements.map((element) => element.name).sort()).toEqual(["Post", "Post text"]);
    for (const element of diff.newElements) {
      expect(element.new).toBe(true);
    }
    expect((diff.snapshot as Snapshot).title).toBe("Changes on click");
  });

  it("says what it saw when the expectation does not match what happened", async () => {
    const page = await worker.result("open", { url: site.page("changes-on-click.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Compose"),
        expectation: "the timeline loads",
      }),
    );
    expect(diff.expectationMet).toBe(false);
    expect(diff.seen).toBe('two new elements appeared: "Post text", "Post"');
  });

  it("types one key at a time and the page sees every keystroke", async () => {
    const page = await worker.result("open", { url: site.page("type-echo.html") });
    const diff = asDiff(
      await worker.result("type", {
        ref: refFor(page, "Post text"),
        text: "Nine years of DigiByte",
        expectation: "a heading shows the DigiByte draft",
      }),
    );
    expect(diff.expectationMet).toBe(true);
    expect(diff.newElements.some((element) => element.name === "Nine years of DigiByte")).toBe(
      true,
    );
  });

  it("meets an expectation that names the box it typed into, though the page did not change", async () => {
    const page = await worker.result("open", { url: site.page("links-and-form.html") });
    const diff = asDiff(
      await worker.result("type", {
        ref: refFor(page, "Post text"),
        text: "Nine years of DigiByte.",
        expectation: "the text box holds the post",
      }),
    );
    // Nothing on the page changed: a value typed into a box is not in the tree.
    expect(diff.newElements).toEqual([]);
    expect(diff.urlChanged).toBe(false);
    // The rule's fifth place is the element the action was aimed at, and this
    // box is named "Post text", so both "text" and "post" are found.
    expect(diff.expectationMet).toBe(true);
    expect(diff.seen).toBe("");
  });

  it("still refuses an expectation the box it typed into has nothing to do with", async () => {
    const page = await worker.result("open", { url: site.page("links-and-form.html") });
    const diff = asDiff(
      await worker.result("type", {
        ref: refFor(page, "Post text"),
        text: "hello",
        expectation: "the timeline loads",
      }),
    );
    expect(diff.expectationMet).toBe(false);
    expect(diff.seen).toBe("nothing changed");
  });

  it("presses a key and follows the page to a new address", async () => {
    const page = await worker.result("open", { url: site.page("keyboard.html") });
    await worker.result("type", {
      ref: refFor(page, "What are you looking for"),
      text: "coeus",
    });
    const diff = asDiff(
      await worker.result("press", { key: "Enter", expectation: "the welcome screen" }),
    );
    expect(diff.urlChanged).toBe(true);
    expect(diff.url).toContain("signed-in.html");
    expect(diff.expectationMet).toBe(true);
  });

  it("scrolls in steps and reports the posts the page put up as a result", async () => {
    await worker.result("open", { url: site.page("more-on-scroll.html") });
    const diff = asDiff(
      await worker.result("scroll", {
        direction: "down",
        amount: 6,
        expectation: "another post",
      }),
    );
    expect(diff.expectationMet).toBe(true);
    expect(diff.newElements.length).toBeGreaterThan(0);
    expect(diff.newElements.every((element) => element.role === "article")).toBe(true);
  });

  it("scrolls back up again", async () => {
    await worker.result("open", { url: site.page("long.html") });
    await worker.result("scroll", { direction: "down", amount: 6 });
    const diff = asDiff(await worker.result("scroll", { direction: "up", amount: 6 }));
    expect(diff.wall).toBeNull();
    expect((diff.snapshot as Snapshot).elements.length).toBeGreaterThan(0);
  });
});

describe("a click that changes nothing", () => {
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

  it("is tried once more at the element's place on the screen, then reported", async () => {
    const page = await worker.result("open", { url: site.page("no-change-on-click.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Do nothing"),
        expectation: "something happens",
      }),
    );
    expect(diff.expectationMet).toBe(false);
    expect(diff.seen).toBe("nothing changed");
    expect(worker.logLines.join(" ")).toContain("clicked it again at its place on the screen");
  });
});

describe("a ref that has gone stale", () => {
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

  it("is found again by its role and its name", async () => {
    const page = await worker.result("open", { url: site.page("stale-ref.html") });
    const target = refFor(page, "Save the draft");
    await worker.result("click", { ref: refFor(page, "Rebuild the target") });
    const diff = asDiff(await worker.result("click", { ref: target, expectation: "" }));
    expect(diff.wall).toBeNull();
    expect(worker.logLines.join(" ")).toContain("found again by its role and name");
  });

  it("is found again by its visible text when even the role has changed", async () => {
    const page = await worker.result("open", { url: site.page("stale-ref.html") });
    const target = refFor(page, "Save the draft");
    await worker.result("click", { ref: refFor(page, "Change the kind of the target") });
    await worker.result("click", { ref: target, expectation: "" });
    expect(worker.logLines.join(" ")).toContain("found again by its visible text");
  });

  it("answers -32000 with a fresh snapshot when the element is gone for good", async () => {
    const page = await worker.result("open", { url: site.page("stale-ref.html") });
    const target = refFor(page, "Save the draft");
    await worker.result("click", { ref: refFor(page, "Remove the target") });
    const failure = await worker.fails("click", { ref: target });
    expect(failure.code).toBe(ERROR_CODES.noSuchReference);
    expect(failure.message).toContain(target);
    const fresh = failure.data?.["snapshot"] as Snapshot;
    expect(fresh.url).toBe(site.page("stale-ref.html"));
    expect(fresh.elements.length).toBeGreaterThan(0);
  });

  it("answers -32000 for a ref that was never a ref at all", async () => {
    await worker.result("open", { url: site.page("stale-ref.html") });
    const failure = await worker.fails("click", { ref: "not-a-ref" });
    expect(failure.code).toBe(ERROR_CODES.noSuchReference);
  });
});

describe("a page that never stops changing", () => {
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

  it("can still be opened, because a page with a ticker on it is still a page", async () => {
    const result = await worker.result("open", { url: site.page("never-settles.html") });
    expect(result["title"]).toBe("A page that never settles");
  });

  it("answers -32001 when an action cannot be told apart from the page's own churn", async () => {
    const page = await worker.result("open", { url: site.page("never-settles.html") });
    const failure = await worker.fails("click", { ref: refFor(page, "Poke it") });
    expect(failure.code).toBe(ERROR_CODES.didNotSettle);
    expect(failure.message).toContain("3000");
  });
});
