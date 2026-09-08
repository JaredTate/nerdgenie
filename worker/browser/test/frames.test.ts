import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";

// On the night of 7 September 2026 the flight simulator's sky task spent an
// hour proving its camera was broken: every screenshot of a parked aircraft
// under still clouds was the same picture, and nothing said whether the page
// was drawing. The worker can tell. Before every screenshot it brings the
// page's window to the front, then counts the animation frames the page's own
// script draws in a quarter of a second, and the count rides on the result as
// framesDrawn, a whole number.
describe("the frames a screenshot counts", () => {
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

  it("a screenshot of a page that keeps drawing reports the frames it drew", async () => {
    await worker.result("open", { url: site.page("live-canvas.html") });
    const picture = await worker.result("screenshot", {});
    expect(typeof picture["framesDrawn"]).toBe("number");
    expect(Number.isInteger(picture["framesDrawn"])).toBe(true);
    expect(picture["framesDrawn"]).toBeGreaterThan(0);
  });

  it("a screenshot of a page that drew once reports no frames", async () => {
    await worker.result("open", { url: site.page("still-canvas.html") });
    const picture = await worker.result("screenshot", {});
    expect(picture["framesDrawn"]).toBe(0);
  });

  it("a screenshot brings the window to the front first", async () => {
    await worker.result("open", { url: site.page("live-canvas.html") });
    // A tab behind another draws no frames at all, which is the "tab is
    // hidden" the tool's sentence names, so the window comes to the front
    // before the frames are counted and the picture is taken. A headless
    // Chrome never hides one tab behind another, so the proof here is the
    // order of the two calls on the page.
    const page = worker.personsPage();
    const calls: string[] = [];
    const bringToFront = page.bringToFront.bind(page);
    const screenshot = page.screenshot.bind(page);
    page.bringToFront = async () => {
      calls.push("bringToFront");
      await bringToFront();
    };
    page.screenshot = (async (options?: Parameters<typeof screenshot>[0]) => {
      calls.push("screenshot");
      return screenshot(options);
    }) as typeof page.screenshot;
    try {
      const picture = await worker.result("screenshot", {});
      expect(calls).toEqual(["bringToFront", "screenshot"]);
      expect(picture["framesDrawn"]).toBeGreaterThan(0);
    } finally {
      page.bringToFront = bringToFront;
      page.screenshot = screenshot;
    }
  });
});
