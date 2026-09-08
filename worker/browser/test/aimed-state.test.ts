import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { Diff, SnapshotElement } from "../src/types.js";

function asDiff(result: Record<string, unknown>): Diff {
  return result as unknown as Diff;
}

/** The ref the page gave the element with this accessible name. */
function refFor(snapshot: Record<string, unknown>, name: string): string {
  const found = (snapshot["elements"] as SnapshotElement[]).find(
    (element) => element.name === name,
  );
  if (found === undefined) {
    throw new Error(`no element named ${name} in ${JSON.stringify(snapshot["elements"])}`);
  }
  return found.ref;
}

// On a tic-tac-toe board the model built, a cell's mark was an aria-hidden SVG
// and a data attribute, and the button's accessible name never changed, so the
// diff's outline looked the same before and after the click. The model could not
// tell its click had landed and clicked the same cell again and again. A click
// now reports the aimed element's state after the action in its own field, so a
// click that changed nothing visible still says what became of the thing clicked.
describe("a click reports the aimed element's state after the action", () => {
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

  it("a click by ref says the cell went disabled and holds the move, though its name held still", async () => {
    const page = await worker.result("open", { url: site.page("aimed-cell.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Cell 3"),
        expectation: "the cell is claimed",
      }),
    );
    expect(diff.aimedState).toContain('button "Cell 3"');
    expect(diff.aimedState).toContain("disabled");
    expect(diff.aimedState).toContain('data-value="O"');
  });

  it("a click at a point on the same cell says the same", async () => {
    await worker.result("open", { url: site.page("aimed-cell.html") });
    const where = await worker.result("read", {
      ask: "JSON.stringify((() => { const r = document.getElementById('cell').getBoundingClientRect(); return [Math.round(r.left + r.width / 2), Math.round(r.top + r.height / 2)]; })())",
    });
    const [x, y] = JSON.parse(JSON.parse(String(where["answer"]))) as [number, number];
    const diff = asDiff(
      await worker.result("click", { x, y, expectation: "the cell is claimed" }),
    );
    expect(diff.aimedState).toContain('button "Cell 3"');
    expect(diff.aimedState).toContain("disabled");
    expect(diff.aimedState).toContain('data-value="O"');
  });

  it("a press names no aimed element, because a key press is aimed at nothing", async () => {
    await worker.result("open", { url: site.page("aimed-cell.html") });
    const diff = asDiff(
      await worker.result("press", { key: "Tab", expectation: "focus moves" }),
    );
    expect(diff.aimedState).toBe("");
  });

  it("a scroll names no aimed element", async () => {
    await worker.result("open", { url: site.page("aimed-cell.html") });
    const diff = asDiff(
      await worker.result("scroll", { direction: "down", expectation: "the page moves" }),
    );
    expect(diff.aimedState).toBe("");
  });
});
