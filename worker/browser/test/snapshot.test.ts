import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { MAX_SNAPSHOT_ELEMENTS } from "../src/limits.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { SnapshotElement } from "../src/types.js";

function elementsOf(result: Record<string, unknown>): SnapshotElement[] {
  return result["elements"] as SnapshotElement[];
}

function named(result: Record<string, unknown>, name: string): SnapshotElement | undefined {
  return elementsOf(result).find((element) => element.name === name);
}

describe("reading a page as a compact tree", () => {
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

  it("opens a page and answers with every field the protocol names", async () => {
    const result = await worker.result("open", { url: site.page("links-and-form.html") });
    expect(result["url"]).toBe(site.page("links-and-form.html"));
    expect(result["title"]).toBe("Links and a form");
    expect(result["tabId"]).toMatch(/^t\d+$/);
    expect(Array.isArray(result["elements"])).toBe(true);
    expect(typeof result["belowFold"]).toBe("number");
  });

  it("gives every kind of element the design names a role and a name", async () => {
    const result = await worker.result("open", { url: site.page("links-and-form.html") });
    const byRole = new Map(elementsOf(result).map((element) => [element.name, element.role]));
    expect(byRole.get("Changes on click")).toBe("link");
    expect(byRole.get("Post")).toBe("button");
    expect(byRole.get("Post text")).toBe("textbox");
    expect(byRole.get("Agree to the rules")).toBe("checkbox");
    expect(byRole.get("Audience")).toBe("combobox");
    expect(byRole.get("Save a draft")).toBe("menuitem");
    expect(byRole.get("Links and a form")).toBe("heading");
    expect(byRole.get("A post")).toBe("article");
  });

  it("keeps a native control that carries a widget role, names a nameless one by its id, and lists a focusable widget", async () => {
    // Run 22 built its board as <button role="gridcell"> and the nine cells
    // vanished from the outline: the stated role won over the button and
    // gridcell is no kind the outline lists. A stated role wins only when it
    // is a kind the outline knows; a native control keeps its own kind; a
    // focusable element with a widget role is a button; a control with no
    // name is named by its id; and a plain data cell is nothing to click.
    const result = await worker.result("open", { url: site.page("widget-roles.html") });
    const byRole = new Map(elementsOf(result).map((element) => [element.name, element.role]));
    expect(byRole.get("cell-0")).toBe("button");
    expect(byRole.get("X")).toBe("button");
    expect(byRole.get("Second tab")).toBe("link");
    expect(byRole.get("Third choice")).toBe("button");
    expect(byRole.get("Find")).toBe("textbox");
    expect(byRole.has("Plain data")).toBe(false);
  });

  it("gives every element a ref that is the letter e and a number", async () => {
    const result = await worker.result("open", { url: site.page("links-and-form.html") });
    for (const element of elementsOf(result)) {
      expect(element.ref).toMatch(/^e\d+$/);
    }
    const refs = elementsOf(result).map((element) => element.ref);
    expect(new Set(refs).size).toBe(refs.length);
  });

  it("keeps a ref on the same element when the page is read again", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    const first = await worker.result("read");
    const second = await worker.result("read");
    expect(named(second, "Post")?.ref).toBe(named(first, "Post")?.ref);
  });

  it("marks an element that its markup hides but a style rule draws", async () => {
    const page = await worker.result("open", { url: site.page("hidden-overlays.html") });
    const elements = page["elements"] as Array<Record<string, unknown>>;
    const gameOver = elements.find((element) => element["name"] === "Game over");
    expect(gameOver?.["hiddenYetDrawn"]).toBe(true);
    const start = elements.find((element) => element["name"] === "Press start");
    expect(start?.["hiddenYetDrawn"]).toBeUndefined();
    expect(elements.some((element) => element["name"] === "Not drawn, because nothing overrides it")).toBe(false);
    // The page's own count covers nodes with no role, which never reach the
    // outline: the game-over card here; the paragraph nothing overrides is not drawn.
    expect(page["hiddenYetDrawn"]).toBe(1);
  });

  it("counts no hidden-yet-drawn element on a page that honours its markup", async () => {
    const page = await worker.result("open", { url: site.page("counter.html") });
    expect(page["hiddenYetDrawn"]).toBe(0);
  });

  it("never puts the page's markup or its scripts into the answer", async () => {
    const result = await worker.result("open", { url: site.page("changes-on-click.html") });
    const written = JSON.stringify(result);
    expect(written).not.toContain("<");
    expect(written).not.toContain("addEventListener");
    expect(written).not.toContain("createElement");
  });

  it("does not list a form, which exists only so the wall detector can read its name", async () => {
    const result = await worker.result("open", { url: site.page("links-and-form.html") });
    expect(elementsOf(result).some((element) => element.role === "form")).toBe(false);
  });

  it("reads a fresh snapshot with read, and the same page it was already on", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    const result = await worker.result("read");
    expect(result["url"]).toBe(site.page("links-and-form.html"));
    expect(elementsOf(result).length).toBeGreaterThan(0);
  });

  it("walks into a frame and lists what is inside it", async () => {
    const result = await worker.result("open", { url: site.page("frame.html") });
    expect(named(result, "Outside the frame")).toBeDefined();
    expect(named(result, "Inside the frame")).toBeDefined();
  });

  it("gives a frame's elements refs that cannot collide with the main page's", async () => {
    const result = await worker.result("open", { url: site.page("frame.html") });
    const refs = elementsOf(result).map((element) => element.ref);
    expect(new Set(refs).size).toBe(refs.length);
  });
});

describe("a page with far more on it than one snapshot may hold", () => {
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

  it("stays under the cap and says in belowFold how much was not shown", async () => {
    const result = await worker.result("open", { url: site.page("long.html") });
    expect(elementsOf(result)).toHaveLength(MAX_SNAPSHOT_ELEMENTS);
    expect(result["belowFold"]).toBeGreaterThan(MAX_SNAPSHOT_ELEMENTS);
  });

  it("lists what a person can see before what they would have to scroll to", async () => {
    const result = await worker.result("open", { url: site.page("long.html") });
    expect(named(result, "A very long page")).toBeDefined();
    expect(named(result, "Button 1")).toBeDefined();
  });

  it("reads only what is above the fold when asked for that", async () => {
    await worker.result("open", { url: site.page("long.html") });
    const everything = await worker.result("read", { visibleOnly: false });
    const visible = await worker.result("read", { visibleOnly: true });
    expect(elementsOf(visible).length).toBeLessThan(elementsOf(everything).length);
    expect(result_belowFold(visible)).toBe(result_belowFold(everything));
  });
});

function result_belowFold(result: Record<string, unknown>): number {
  return result["belowFold"] as number;
}
