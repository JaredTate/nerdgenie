/**
 * Watching what the person does in the window, and reporting it.
 *
 * The agent is not the only one who uses this browser. A person recording a walk
 * clicks, types, and moves around the window themselves, and the worker reports
 * each of those as an `event` notification so that the Go side can write the
 * procedure down. See rule 7 of worker/browser/PROTOCOL.md.
 *
 * Two rules keep the stream honest. The worker never reports what it did itself,
 * so an event that arrives while a request is running is dropped: the model
 * already sees its own actions in the diffs. And a page that calls the watcher's
 * name itself cannot flood the pipe, because the events one worker sends are
 * capped by the minute.
 */
import type { Frame, Page } from "playwright-core";
import { MAX_ELEMENT_NAME_CHARS, MAX_PERSON_EVENTS_PER_MINUTE } from "./limits.js";
import { PERSON_BINDING, PERSON_SCRIPT } from "./person-script.js";
import type { Session } from "./session.js";
import type { PersonEvent, WhatThePersonDid } from "./types.js";

/** What the page hands over when the person does something on it. */
interface ReportedByThePage {
  kind?: unknown;
  ref?: unknown;
  text?: unknown;
  length?: unknown;
  startedAt?: unknown;
}

/** One thing the person did, and the moment on the page it started at. */
interface PersonDidSomething {
  did: WhatThePersonDid;
  startedAt: number;
}

/** One minute, which is the stretch the cap on events is counted over. */
const ONE_MINUTE_MS = 60_000;

/** Where an event goes once the worker has decided to report it. */
export type ReportEvent = (event: PersonEvent) => void;

/**
 * Watch every page for what the person does on it. Nothing here throws: a page
 * that will not take the watcher is logged and left alone, because a browser
 * that still works for the agent is better than one that stops over a watcher.
 */
export function watchWhatThePersonDoes(session: Session, report: ReportEvent): void {
  const sender = new PersonEventSender(session, report);
  const watched = new WeakSet<Page>();
  const watchOne = (page: Page): void => {
    if (watched.has(page)) {
      return;
    }
    watched.add(page);
    void installOn(session, page, sender);
  };
  session.chrome.context.on("page", watchOne);
  for (const page of session.chrome.context.pages()) {
    watchOne(page);
  }
}

/** Put the watcher into one page, and listen for that page moving somewhere else. */
async function installOn(session: Session, page: Page, sender: PersonEventSender): Promise<void> {
  page.on("framenavigated", (frame: Frame) => {
    if (frame !== page.mainFrame()) {
      return;
    }
    const address = frame.url();
    if (address === "" || address === "about:blank") {
      return;
    }
    sender.send({ did: { kind: "navigate", address }, startedAt: Date.now() });
  });
  try {
    await page.exposeBinding(PERSON_BINDING, (_source, reported: unknown) => {
      sender.send(readWhatThePageSaid(reported));
    });
    await page.addInitScript(PERSON_SCRIPT);
    // The document already open was loaded before any of that, so it is told
    // separately; a document that arrives later gets the init script instead.
    await page.evaluate(PERSON_SCRIPT);
  } catch (problem) {
    const why = problem instanceof Error ? problem.message : String(problem);
    session.log(`this page will not report what the person does on it: ${why}`);
  }
}

/** Read what the page handed over, keeping only the fields its kind may carry. */
function readWhatThePageSaid(reported: unknown): PersonDidSomething | undefined {
  if (typeof reported !== "object" || reported === null) {
    return undefined;
  }
  const said = reported as ReportedByThePage;
  const ref = typeof said.ref === "string" ? said.ref : "";
  const startedAt = typeof said.startedAt === "number" ? said.startedAt : Date.now();
  if (said.kind === "click") {
    const text = typeof said.text === "string" ? said.text.slice(0, MAX_ELEMENT_NAME_CHARS) : "";
    if (ref === "" && text === "") {
      return undefined;
    }
    return { did: { kind: "click", ref, text }, startedAt };
  }
  if (said.kind === "type") {
    const length = typeof said.length === "number" && said.length > 0 ? Math.floor(said.length) : 0;
    return { did: { kind: "type", ref, length }, startedAt };
  }
  return undefined;
}

/**
 * The one place an event becomes a line the Go side reads. It holds the two
 * rules: nothing the worker itself caused, and no more than the cap in a minute.
 */
class PersonEventSender {
  private sentThisMinute = 0;
  private minuteStartedAt = Date.now();
  private toldAboutTheCap = false;

  constructor(
    private readonly session: Session,
    private readonly report: ReportEvent,
  ) {}

  /** Report one event, unless the worker caused it or the cap has been reached. */
  send(happening: PersonDidSomething | undefined): void {
    if (happening === undefined || this.session.wasWorkingAt(happening.startedAt)) {
      return;
    }
    if (!this.thereIsRoom()) {
      return;
    }
    this.report({ ...happening.did, at: new Date(happening.startedAt).toISOString() });
  }

  /** Is this event inside the cap for the minute it arrived in? */
  private thereIsRoom(): boolean {
    const now = Date.now();
    if (now - this.minuteStartedAt >= ONE_MINUTE_MS) {
      this.minuteStartedAt = now;
      this.sentThisMinute = 0;
      this.toldAboutTheCap = false;
    }
    this.sentThisMinute += 1;
    if (this.sentThisMinute <= MAX_PERSON_EVENTS_PER_MINUTE) {
      return true;
    }
    if (!this.toldAboutTheCap) {
      this.toldAboutTheCap = true;
      this.session.log(
        `more than ${MAX_PERSON_EVENTS_PER_MINUTE} events arrived from the page in one minute, so the rest of this minute is dropped.`,
      );
    }
    return false;
  }
}
