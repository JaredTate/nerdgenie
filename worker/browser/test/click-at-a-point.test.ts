import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { ERROR_CODES } from "../src/errors.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { Diff } from "../src/types.js";

/** Where the fixture draws its planet, in CSS pixels from the top left of the viewport. */
const PLANET = { x: 160, y: 120 };

function asDiff(result: Record<string, unknown>): Diff {
  return result as unknown as Diff;
}

// On 7 September 2026 the solar-system job spent an hour on a canvas that no
// outline lists: the model invented a ref for a spot on the canvas, was refused,
// and fell back to dispatching its own clicks through the read method's ask. A
// click names a point now, x and y in CSS pixels from the top left of the
// viewport, and does everything a click on an element does once the element is
// found: the same pacing, the same settle, the same diff and verdict.
describe("clicking a point on the page", () => {
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

  it("clicks the point and reads the heading the page wrote a frame later", async () => {
    await worker.result("open", { url: site.page("planet-canvas.html") });
    const diff = asDiff(
      await worker.result("click", {
        x: PLANET.x,
        y: PLANET.y,
        expectation: "the info panel names Jupiter",
      }),
    );
    expect(diff.expectationMet).toBe(true);
    expect(diff.seen).toBe("");
    expect(diff.settled).toBe(true);
    expect(diff.newText).toContain("Jupiter");
    expect(diff.snapshot.text).toContain("Jupiter");
    expect(diff.urlChanged).toBe(false);
    expect(diff.url).toBe(site.page("planet-canvas.html"));
    expect(diff.dialog).toBeNull();
    expect(diff.newTab).toBe("");
    expect(diff.download).toBeNull();
    expect(diff.wall).toBeNull();
    // One click landed on the scene, and it was not tried again at its place.
    const read = await worker.result("read", { ask: "window.clicksOnTheScene" });
    expect(read["answer"]).toBe("1");
  });

  it("refuses a request with both a ref and a point, and one with neither", async () => {
    const both = await worker.fails("click", {
      ref: "e1",
      x: PLANET.x,
      y: PLANET.y,
      expectation: "anything at all",
    });
    expect(both.code).toBe(ERROR_CODES.wrongParameters);
    expect(both.message).toContain("both");
    expect(both.message).toContain("one or the other");

    const neither = await worker.fails("click", { expectation: "anything at all" });
    expect(neither.code).toBe(ERROR_CODES.wrongParameters);
    expect(neither.message).toContain("ref");
    expect(neither.message).toContain("x and y");
  });

  it("refuses a point outside the viewport", async () => {
    await worker.result("open", { url: site.page("planet-canvas.html") });
    const far = await worker.fails("click", {
      x: 100_000,
      y: PLANET.y,
      expectation: "anything at all",
    });
    expect(far.code).toBe(ERROR_CODES.wrongParameters);
    expect(far.message).toContain("outside");
    expect(far.message).toMatch(/\d+ by \d+/);

    const above = await worker.fails("click", {
      x: PLANET.x,
      y: -1,
      expectation: "anything at all",
    });
    expect(above.code).toBe(ERROR_CODES.wrongParameters);
    expect(above.message).toContain("outside");

    // Neither refused click reached the page.
    const read = await worker.result("read", { ask: "window.clicksOnTheScene" });
    expect(read["answer"]).toBe("0");
  });

  it("an act step can click a point", async () => {
    await worker.result("open", { url: site.page("planet-canvas.html") });
    const result = await worker.result("act", {
      steps: [
        { method: "click", x: PLANET.x, y: PLANET.y, expectation: "the heading names Jupiter" },
      ],
    });
    const diffs = result["diffs"] as Diff[];
    expect(diffs).toHaveLength(1);
    expect(diffs[0]?.expectationMet).toBe(true);
    expect(diffs[0]?.newText).toContain("Jupiter");
  });
});

describe("a click at a point says what was under the point", () => {
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

  // Run 28's model clicked at stale coordinates more than a hundred times,
  // each answered "nothing changed", and never learned what its points hit.
  it("names the listed element under the point, or says nothing is listed there", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    const where = await worker.result("read", {
      ask: "JSON.stringify((() => { const r = document.querySelector('button').getBoundingClientRect(); return [Math.round(r.left + r.width / 2), Math.round(r.top + r.height / 2)]; })())",
    });
    const [x, y] = JSON.parse(JSON.parse(String(where["answer"]))) as [number, number];
    const onTheButton = (await worker.result("click", { x, y, expectation: "the post is sent" })) as unknown as Diff;
    expect(onTheButton.under).toMatch(/^e\d+ button "Post"/);

    await worker.result("open", { url: site.page("links-and-form.html") });
    const onNothing = (await worker.result("click", { x: 2, y: 2, expectation: "something happens" })) as unknown as Diff;
    expect(onNothing.under).toBe("nothing the outline lists");
  });
});

