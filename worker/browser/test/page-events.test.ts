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

  it("answers it when told to, and the page comes back to life", async () => {
    const diff = asDiff(await worker.result("dialog", { action: "accept" }));
    expect(diff.dialog).toBeNull();
    expect(diff.snapshot.dialog).toBeNull();
    expect(diff.settled).toBe(true);
    // The page only writes this once the confirm has been answered yes, so it is
    // proof that the answer really went through.
    expect(diff.newElements.some((element) => element.name === "Confirmed by you")).toBe(true);
  });

  it("can be read again afterwards, which it could not while the dialog was open", async () => {
    const again = await worker.result("read");
    expect(again["dialog"]).toBeNull();
    expect((again["elements"] as SnapshotElement[]).length).toBeGreaterThan(1);
  });

  it("dismisses a dialog without confirming it", async () => {
    const page = await worker.result("open", { url: site.page("dialog.html") });
    await worker.result("click", { ref: refFor(page, "Ask me something") });
    const diff = asDiff(await worker.result("dialog", { action: "dismiss" }));
    expect(diff.newElements.some((element) => element.name === "Refused by you")).toBe(true);
  });

  it("types the text it was given into a prompt before accepting it", async () => {
    const page = await worker.result("open", { url: site.page("dialog.html") });
    await worker.result("click", { ref: refFor(page, "Ask me for a name") });
    const diff = asDiff(await worker.result("dialog", { action: "accept", text: "Nerd Genie" }));
    expect(diff.newElements.some((element) => element.name === "Hello Nerd Genie")).toBe(true);
  });

  it("gives a prompt no answer at all when it is dismissed", async () => {
    const page = await worker.result("open", { url: site.page("dialog.html") });
    await worker.result("click", { ref: refFor(page, "Ask me for a name") });
    const diff = asDiff(await worker.result("dialog", { action: "dismiss" }));
    expect(diff.newElements.some((element) => element.name === "No name given")).toBe(true);
  });

  it("refuses any answer that is neither accept nor dismiss", async () => {
    const failure = await worker.fails("dialog", { action: "maybe" });
    expect(failure.code).toBe(-32602);
    expect(failure.message).toContain("maybe");
  });

  it("says plainly when there is no dialog to answer", async () => {
    await worker.result("open", { url: site.page("links-and-form.html") });
    const failure = await worker.fails("dialog", { action: "accept" });
    expect(failure.code).toBe(-32602);
    expect(failure.message).toContain("no dialog");
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
    const survivors = listed.filter((tab) => tab.id !== doomed!.id).map((tab) => tab.id);
    const left = (await worker.result("tabs", { action: "close", tabId: doomed!.id }))[
      "tabs"
    ] as TabReport[];
    // Comparing the ids rather than the count says which tab is unexpectedly
    // there, if this ever fails again.
    expect(left.map((tab) => tab.id)).toEqual(survivors);
  });

  it("says so plainly when asked about a tab that is not there", async () => {
    const failure = await worker.fails("tabs", { action: "switch", tabId: "t99" });
    expect(failure.code).toBe(-32602);
    expect(failure.message).toContain("t99");
  });
});

describe("a page that is a PDF", () => {
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

  it("saves the file under the profile folder and says plainly what it is", async () => {
    const result = await worker.result("open", { url: site.page("report.pdf") });
    expect(result["download"]).toEqual({
      filename: "report.pdf",
      path: join(worker.profile, "downloads", "report.pdf"),
    });
    const saved = await readFile(join(worker.profile, "downloads", "report.pdf"));
    expect(saved.subarray(0, 5).toString("latin1")).toBe("%PDF-");
    const elements = result["elements"] as SnapshotElement[];
    expect(elements).toHaveLength(1);
    expect(elements[0]?.role).toBe("article");
    expect(elements[0]?.name).toContain("report.pdf");
  });

  it("finds no wall on it, and can still be read again afterwards", async () => {
    await worker.result("open", { url: site.page("report.pdf") });
    const again = await worker.result("read");
    expect(again["wall"]).toBeNull();
    expect(again["url"]).toContain("report.pdf");
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
