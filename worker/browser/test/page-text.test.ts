import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { MAX_PAGE_TEXT_CHARS } from "../src/limits.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { SnapshotElement } from "../src/types.js";

/**
 * The rankings page as a person reads it: one line per block, a table as one
 * line per row with its cells separated by a bar, and the empty icon button in
 * the first cell of each row leaving the row starting with the bar.
 */
const RANKINGS_TEXT = [
  "Coin rankings",
  "Every coin scored, updated each morning.",
  "Watch | Rank | Name | D-Score",
  "| 1 | Bitcoin | 97.2",
  "| 2 | DigiByte | 91.4",
  "Load more",
].join("\n");

function textOf(result: Record<string, unknown>): string {
  return String(result["text"]);
}

function elementsOf(result: Record<string, unknown>): SnapshotElement[] {
  return result["elements"] as SnapshotElement[];
}

describe("reading the text of a page beside its elements", () => {
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

  it("answers open with the page's text in reading order, one line per block", async () => {
    const result = await worker.result("open", { url: site.page("rankings.html") });
    expect(textOf(result)).toBe(RANKINGS_TEXT);
  });

  it("reads a table as one line per row with the cells separated by a bar", async () => {
    const result = await worker.result("open", { url: site.page("rankings.html") });
    expect(textOf(result)).toContain("| 2 | DigiByte | 91.4");
  });

  it("keeps the refs on the elements that can be acted on", async () => {
    const result = await worker.result("open", { url: site.page("rankings.html") });
    const button = elementsOf(result).find((element) => element.name === "Load more");
    expect(button?.role).toBe("button");
    expect(button?.ref).toMatch(/^e\d+$/);
    expect(elementsOf(result).some((element) => element.role === "heading")).toBe(true);
  });

  it("leaves out what is not drawn, and never carries the page's scripts", async () => {
    const result = await worker.result("open", { url: site.page("rankings.html") });
    expect(textOf(result)).not.toContain("Nobody sees this line");
    expect(textOf(result)).not.toContain("computedOnThePage");
    expect(textOf(result)).not.toContain("not for the model");
    expect(textOf(result)).not.toContain("<");
  });

  it("answers read with the same text as open", async () => {
    await worker.result("open", { url: site.page("rankings.html") });
    const result = await worker.result("read");
    expect(textOf(result)).toBe(RANKINGS_TEXT);
  });

  it("cuts the text at the cap and ends with a line saying how much was cut", async () => {
    const result = await worker.result("open", { url: site.page("much-text.html") });
    const lines = textOf(result).split("\n");
    const last = lines[lines.length - 1] ?? "";
    expect(last).toMatch(/^\.\.\. [1-9]\d* more characters were cut$/);
    expect(textOf(result).length - last.length - 1).toBeLessThanOrEqual(MAX_PAGE_TEXT_CHARS);
    expect(lines[0]).toBe("Much text");
    expect(lines[1]).toBe("Paragraph 1 says the same thing again.");
  });
});
