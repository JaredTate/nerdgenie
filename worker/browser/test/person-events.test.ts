/**
 * What the person does in the window, watched by the worker.
 *
 * The person here is Playwright acting on the page outside the worker's own
 * request path, which is exactly what a real person's mouse and keyboard look
 * like to the page: real events on the same Chrome, with no request in flight.
 */
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { startTestWorker, type TestWorker } from "./harness.js";
import { startFixtureServer, type FixtureServer } from "./server.js";
import { formatEvent } from "../src/wire.js";
import type { PersonEvent, SnapshotElement } from "../src/types.js";

/** How long a test waits for an event before it says none arrived. */
const WAIT_FOR_AN_EVENT_MS = 10_000;

/** How long a test waits to be sure that no event is going to arrive. */
const WAIT_FOR_QUIET_MS = 1_500;

function refFor(snapshot: Record<string, unknown>, name: string): string {
  const found = (snapshot["elements"] as SnapshotElement[]).find((element) => element.name === name);
  if (found === undefined) {
    throw new Error(`no element named ${name} in ${JSON.stringify(snapshot["elements"])}`);
  }
  return found.ref;
}

async function wait(howLong: number): Promise<void> {
  await new Promise((next) => setTimeout(next, howLong));
}

/** Wait for the next event of one kind, or say that none arrived. */
async function waitForEvent(worker: TestWorker, kind: PersonEvent["kind"]): Promise<PersonEvent> {
  const giveUpAt = Date.now() + WAIT_FOR_AN_EVENT_MS;
  while (Date.now() < giveUpAt) {
    const found = worker.events.find((event) => event.kind === kind);
    if (found !== undefined) {
      return found;
    }
    await wait(25);
  }
  throw new Error(`no ${kind} event arrived. What did arrive: ${JSON.stringify(worker.events)}`);
}

describe("the events a person makes in the window", () => {
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

  it("reports a click the person made, with the element and what it says", async () => {
    const page = await worker.result("open", { url: site.page("links-and-form.html") });
    worker.events.length = 0;

    await worker.personsPage().click("#post-button");
    const clicked = await waitForEvent(worker, "click");

    expect(clicked.ref).toBe(refFor(page, "Post"));
    expect(clicked.text).toBe("Post");
    expect(typeof clicked.at).toBe("string");
  });

  it("reports how much the person typed and never what they typed", async () => {
    worker.events.length = 0;

    await worker.personsPage().click("#post-text");
    await worker.personsPage().keyboard.type("a secret nobody may keep");
    const typed = await waitForEvent(worker, "type");

    expect(typed.length).toBe("a secret nobody may keep".length);
    expect(typed.ref).not.toBe("");
    expect(JSON.stringify(typed)).not.toContain("secret");
    expect(typed.text).toBeUndefined();
  });

  it("reports the person taking the window somewhere else", async () => {
    worker.events.length = 0;
    const address = site.page("changes-on-click.html");

    await worker.personsPage().goto(address, { waitUntil: "domcontentloaded" });
    const moved = await waitForEvent(worker, "navigate");

    expect(moved.address).toBe(address);
  });

  it("says nothing about what the worker itself did", async () => {
    const page = await worker.result("open", { url: site.page("changes-on-click.html") });
    worker.events.length = 0;

    await worker.result("click", { ref: refFor(page, "Compose"), expectation: "a post box appears" });
    await worker.result("type", {
      ref: refFor(await worker.result("read"), "Post text"),
      text: "Nine years of DigiByte.",
      expectation: "the box holds the post",
    });
    await wait(WAIT_FOR_QUIET_MS);

    expect(worker.events).toEqual([]);
  });
});

describe("the line the worker writes an event on", () => {
  it("is a notification with no id, so it can never be read as an answer to a call", () => {
    const line = formatEvent({
      kind: "click",
      ref: "e7",
      text: "Post",
      at: "2026-09-03T10:00:00.000Z",
    });

    expect(line.endsWith("\n")).toBe(true);
    const written = JSON.parse(line) as Record<string, unknown>;
    expect(written["jsonrpc"]).toBe("2.0");
    expect(written["method"]).toBe("event");
    expect("id" in written).toBe(false);
    expect(written["params"]).toEqual({
      kind: "click",
      ref: "e7",
      text: "Post",
      at: "2026-09-03T10:00:00.000Z",
    });
  });
});
