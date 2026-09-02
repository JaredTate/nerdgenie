import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { REDACTED } from "../src/redact.js";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { Diff, ScreenshotMark, SnapshotElement } from "../src/types.js";

function refFor(snapshot: Record<string, unknown>, name: string): string {
  const found = (snapshot["elements"] as SnapshotElement[]).find(
    (element) => element.name === name,
  );
  if (found === undefined) {
    throw new Error(`no element named ${name} in ${JSON.stringify(snapshot["elements"])}`);
  }
  return found.ref;
}

describe("running a short batch of steps", () => {
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

  it("returns one diff per step that ran", async () => {
    const page = await worker.result("open", { url: site.page("type-echo.html") });
    const box = refFor(page, "Post text");
    const result = await worker.result("act", {
      steps: [
        { method: "type", ref: box, text: "Nine years", expectation: "a heading saying Nine" },
        { method: "type", ref: box, text: " of DigiByte", expectation: "a heading saying DigiByte" },
      ],
    });
    const diffs = result["diffs"] as Diff[];
    expect(diffs).toHaveLength(2);
    for (const diff of diffs) {
      expect(diff.expectationMet).toBe(true);
      expect(diff.snapshot.url).toBe(site.page("type-echo.html"));
    }
  });

  it("stops as soon as one expectation fails", async () => {
    const page = await worker.result("open", { url: site.page("no-change-on-click.html") });
    const result = await worker.result("act", {
      steps: [
        { method: "click", ref: refFor(page, "Do nothing"), expectation: "something happens" },
        { method: "press", key: "Tab", expectation: "never reached" },
      ],
    });
    const diffs = result["diffs"] as Diff[];
    expect(diffs).toHaveLength(1);
    expect(diffs[0]?.expectationMet).toBe(false);
  });

  it("stops when the page moves out from under the batch", async () => {
    const page = await worker.result("open", { url: site.page("changes-on-click.html") });
    const result = await worker.result("act", {
      steps: [
        {
          method: "click",
          ref: refFor(page, "Compose"),
          expectation: "a Post button appears",
        },
        { method: "click", ref: refFor(page, "Done for now"), expectation: "the welcome screen" },
        { method: "press", key: "Tab", expectation: "never reached" },
      ],
    });
    const diffs = result["diffs"] as Diff[];
    expect(diffs).toHaveLength(2);
    expect(diffs[0]?.expectationMet).toBe(true);
    expect(diffs[1]?.urlChanged).toBe(true);
  });
});

describe("filling in a login from the vault", () => {
  let worker: TestWorker;
  let site: FixtureServer;
  const username = "coeus-operator";
  const password = "hunter2-and-then-some";

  beforeAll(async () => {
    site = await startFixtureServer();
    worker = await startTestWorker();
  });

  afterAll(async () => {
    await worker.stop();
    await site.stop();
  });

  it("types the values, submits, and never hands any of them back", async () => {
    const page = await worker.result("open", { url: site.page("login.html") });
    const response = await worker.send("loginFill", {
      usernameRef: refFor(page, "Username"),
      passwordRef: refFor(page, "Password"),
      username,
      password,
    });
    const whole = JSON.stringify(response);
    expect(whole).not.toContain(username);
    expect(whole).not.toContain(password);
    expect(whole).toContain(REDACTED);
    expect("result" in response).toBe(true);
  });

  it("really did type them, because the form went through to the next page", async () => {
    const now = await worker.result("read");
    expect(now["url"]).toContain("signed-in.html");
    expect(now["title"]).toBe("Welcome back");
  });

  it("types a second-factor code into the field it was told to", async () => {
    const page = await worker.result("open", { url: site.page("two-factor.html") });
    const response = await worker.send("loginFill", {
      codeRef: refFor(page, "Code"),
      code: "314159",
    });
    expect(JSON.stringify(response)).not.toContain("314159");
  });
});

describe("taking a picture with the clickable elements numbered", () => {
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

  it("answers with a real picture and a list of the marks drawn on it", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    const result = await worker.result("screenshot");
    const picture = Buffer.from(String(result["pngBase64"]), "base64");
    expect([...picture.subarray(0, 4)]).toEqual([0x89, 0x50, 0x4e, 0x47]);
    const marks = result["marks"] as ScreenshotMark[];
    expect(marks.length).toBeGreaterThan(0);
    expect(marks.map((mark) => mark.number)).toEqual(marks.map((_, at) => at + 1));
    for (const mark of marks) {
      expect(mark.ref).toMatch(/^e\d+$/);
      expect(typeof mark.name).toBe("string");
    }
    expect(marks.some((mark) => mark.name === "Post" && mark.role === "button")).toBe(true);
  });

  it("does not number a heading or an article, which nobody clicks", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    const marks = (await worker.result("screenshot"))["marks"] as ScreenshotMark[];
    expect(marks.some((mark) => mark.role === "heading")).toBe(false);
    expect(marks.some((mark) => mark.role === "article")).toBe(false);
  });

  it("leaves no marks behind on the page it photographed", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    await worker.result("screenshot");
    const after = await worker.result("read");
    const names = (after["elements"] as SnapshotElement[]).map((element) => element.name);
    expect(names.some((name) => /^\d+$/.test(name))).toBe(false);
  });
});
