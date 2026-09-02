import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import type { Diff, SnapshotElement, TabReport } from "../src/types.js";

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

describe("a page that opens a dialog", () => {
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

  it("reports the dialog and never answers it for the user", async () => {
    const page = await worker.result("open", { url: site.page("dialog.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Ask me something"),
        expectation: "a question",
      }),
    );
    expect(diff.dialog).toEqual({ kind: "confirm", message: "Are you sure?" });
    expect(diff.snapshot.dialog).toEqual({ kind: "confirm", message: "Are you sure?" });
  });

  it("still reports it on the next read, which proves nobody answered it", async () => {
    const again = await worker.result("read");
    expect(again["dialog"]).toEqual({ kind: "confirm", message: "Are you sure?" });
  });
});

describe("a page that saves a file", () => {
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

  it("saves the download under the profile folder and reports it", async () => {
    const page = await worker.result("open", { url: site.page("download.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Save the notes"),
        expectation: "the notes are saved",
      }),
    );
    expect(diff.download?.filename).toBe("notes.txt");
    expect(diff.download?.path).toBe(join(worker.profile, "downloads", "notes.txt"));
    const saved = await readFile(diff.download!.path, "utf8");
    expect(saved).toContain("Nine years of DigiByte.");
  });
});

describe("a link that opens a new tab", () => {
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

  it("gives the new tab an id and reports it on the diff", async () => {
    const page = await worker.result("open", { url: site.page("new-tab.html") });
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Open the form in a new tab"),
        expectation: "a second window",
      }),
    );
    expect(diff.newTab).toMatch(/^t\d+$/);
  });

  it("lists both tabs, with the one being acted on marked active", async () => {
    const result = await worker.result("tabs", { action: "list" });
    const tabs = result["tabs"] as TabReport[];
    expect(tabs.length).toBe(2);
    expect(tabs.filter((tab) => tab.active === true)).toHaveLength(1);
    expect(tabs.map((tab) => tab.id)).toEqual(tabs.map((tab) => tab.id).filter(Boolean));
    expect(tabs.some((tab) => tab.title === "Links and a form")).toBe(true);
  });

  it("switches to the other tab and acts on that one from then on", async () => {
    const listed = (await worker.result("tabs", { action: "list" }))["tabs"] as TabReport[];
    const other = listed.find((tab) => tab.active !== true);
    const switched = (await worker.result("tabs", { action: "switch", tabId: other!.id }))[
      "tabs"
    ] as TabReport[];
    expect(switched.find((tab) => tab.active === true)?.id).toBe(other!.id);
    const now = await worker.result("read");
    expect(now["tabId"]).toBe(other!.id);
  });

  it("closes a tab and lists what is left", async () => {
    const listed = (await worker.result("tabs", { action: "list" }))["tabs"] as TabReport[];
    const doomed = listed.find((tab) => tab.active !== true);
    const left = (await worker.result("tabs", { action: "close", tabId: doomed!.id }))[
      "tabs"
    ] as TabReport[];
    expect(left).toHaveLength(1);
    expect(left.some((tab) => tab.id === doomed!.id)).toBe(false);
  });

  it("says so plainly when asked about a tab that is not there", async () => {
    const failure = await worker.fails("tabs", { action: "switch", tabId: "t99" });
    expect(failure.code).toBe(-32602);
    expect(failure.message).toContain("t99");
  });
});

describe("the walls, on real pages", () => {
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

  it("sees the login wall on a page with a password field", async () => {
    const result = await worker.result("open", { url: site.page("login.html") });
    expect(result["wall"]).toEqual({ kind: "login", detail: "a password field named Password" });
  });

  it("sees the second-factor wall on a page asking for a code", async () => {
    const result = await worker.result("open", { url: site.page("two-factor.html") });
    expect((result["wall"] as { kind: string }).kind).toBe("two-factor");
  });

  it("sees the captcha wall on a page with a captcha frame", async () => {
    const result = await worker.result("open", { url: site.page("captcha.html") });
    expect((result["wall"] as { kind: string }).kind).toBe("captcha");
  });

  it("sees no wall on an ordinary page of links and a form", async () => {
    const result = await worker.result("open", { url: site.page("links-and-form.html") });
    expect(result["wall"]).toBeNull();
  });

  it("sees no wall on a page that merely changes when you click it", async () => {
    const result = await worker.result("open", { url: site.page("changes-on-click.html") });
    expect(result["wall"]).toBeNull();
  });

  it("refuses the expectation and says which wall stopped it", async () => {
    const page = await worker.result("open", { url: site.page("login.html") });
    // Clicking the username field keeps the browser on the login page, which is
    // the case that matters: the wall is still there after the action.
    const diff = asDiff(
      await worker.result("click", {
        ref: refFor(page, "Username"),
        expectation: "the members area",
      }),
    );
    expect(diff.wall?.kind).toBe("login");
    expect(diff.expectationMet).toBe(false);
    expect(diff.seen).toContain("login wall");
  });
});
