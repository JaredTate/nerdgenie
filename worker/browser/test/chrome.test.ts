import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { readdir } from "node:fs/promises";
import { startTestWorker, type TestWorker } from "./harness.js";

describe("launching a real Chrome and attaching to it", () => {
  let worker: TestWorker;

  beforeAll(async () => {
    worker = await startTestWorker();
  });

  afterAll(async () => {
    await worker.stop();
  });

  it("reports that it is healthy and says which Chrome it drove", async () => {
    const result = await worker.result("health");
    expect(result["healthy"]).toBe(true);
    expect(result["chromeVersion"]).toMatch(/^\d+\.\d+\.\d+\.\d+$/);
  });

  it("uses the profile folder it was given, not the user's daily one", async () => {
    const inside = await readdir(worker.profile);
    expect(inside).toContain("Default");
  });

  it("makes a downloads folder inside the profile for anything a page saves", async () => {
    const inside = await readdir(worker.profile);
    expect(inside).toContain("downloads");
  });

  it("writes its logging in plain sentences and never to standard output", () => {
    expect(worker.logLines.length).toBeGreaterThan(0);
    for (const line of worker.logLines) {
      expect(line).not.toContain("\n");
      expect(line.startsWith("browser worker: ")).toBe(true);
    }
    expect(worker.logLines.join(" ")).toContain("Chrome");
  });
});
