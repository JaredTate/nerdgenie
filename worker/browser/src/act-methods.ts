/**
 * The methods that change the page: click, type, press, and scroll.
 *
 * They all share one shape, which is the act-and-assert loop from design section
 * 9: take the page as it is, do the one thing, wait for the page to settle, take
 * the page again, and hand back what changed together with a verdict on the
 * expectation the model stated. A good mechanic tightens a bolt and then tries to
 * turn it by hand; that hand check is what these methods do.
 */
import type { Page } from "playwright-core";
import {
  actionOrDialog,
  clickAgainAtItsPlace,
  clickTarget,
  pressOneKey,
  scrollInSteps,
  targetOf,
  typeIntoTarget,
} from "./actions.js";
import { buildDiff } from "./diff.js";
import { somethingChanged, type AimedAt } from "./expectation.js";
import { DEFAULT_SCROLL_STEPS } from "./limits.js";
import { couldNotBeRead, wrongParameters, WorkerError } from "./errors.js";
import { SETTLE_LIMIT_MS } from "./limits.js";
import { readPage, type PageReading } from "./snapshot.js";
import { settle } from "./settle.js";
import type { Session } from "./session.js";
import type { DialogAnswer, Diff, Snapshot } from "./types.js";

/** How many times a read is tried again when a move to a new address interrupts it. */
const READ_ATTEMPTS = 3;

/**
 * Read the page, and turn a page that cannot be read at all into -32001.
 *
 * A read that lands in the middle of a move to a new address fails, because the
 * document it was reading is gone. That is not a page that cannot be read, only a
 * page that was busy for a moment, so the read waits for the new document and
 * tries again, a few times and no more. A page that throws its own document away
 * faster than it can be looked at runs out of tries and answers -32001.
 */
export async function readOrSayItCannotBeRead(
  session: Session,
  page: Page,
  settings: { visibleOnly: boolean; against: Snapshot | null },
): Promise<PageReading> {
  let last = "";
  for (let attempt = 1; attempt <= READ_ATTEMPTS; attempt += 1) {
    try {
      return await readPage(session, page, settings);
    } catch (problem) {
      if (problem instanceof WorkerError) {
        throw problem;
      }
      last = problem instanceof Error ? problem.message : String(problem);
      session.log(`reading the page was interrupted, so it was tried again: ${last}`);
      await page.waitForLoadState("domcontentloaded", { timeout: SETTLE_LIMIT_MS }).catch(() => {});
    }
  }
  throw couldNotBeRead(SETTLE_LIMIT_MS, last);
}

/** Read the page as it is right now, without measuring anything against it. */
async function pageAsItIs(session: Session, page: Page): Promise<Snapshot> {
  const reading = await readPage(session, page, { visibleOnly: false, against: null });
  return reading.snapshot;
}

/** The snapshot to measure this action against: the last one, if it is of this same tab. */
async function snapshotBefore(session: Session, page: Page): Promise<Snapshot> {
  const last = session.previousSnapshot();
  if (last !== null && last.tabId === session.tabId(page)) {
    return last;
  }
  return pageAsItIs(session, page);
}

/** A fresh snapshot to hand back with a -32000, so the model can point at something real. */
export function freshSnapshotFor(
  session: Session,
  page: Page,
): () => Promise<Record<string, unknown>> {
  return async () => ({ snapshot: await pageAsItIs(session, page) });
}

/**
 * Do one thing to the page, then wait for it to settle, read it again, and build
 * the diff. This is the whole of act and assert, and every action goes through it.
 */
export async function actAndAssert(
  session: Session,
  expectation: string,
  work: (page: Page, before: Snapshot) => Promise<AimedAt | undefined>,
): Promise<Diff> {
  const page = session.currentPage();
  session.beginAction();
  const before = await snapshotBefore(session, page);
  let aimedAt: AimedAt | undefined;
  await actionOrDialog(session, page, async () => {
    aimedAt = await work(page, before);
  });
  const settled = await settle(session, page);
  const reading = await readOrSayItCannotBeRead(session, page, {
    visibleOnly: false,
    against: before,
  });
  session.rememberSnapshot(reading.snapshot);
  if (reading.wall !== null) {
    session.log(`the action ran into a ${reading.wall.kind} wall: ${reading.wall.detail}.`);
  }
  return buildDiff({
    before,
    after: reading.snapshot,
    expectation,
    newTab: session.tabOpenedDuring(),
    wall: reading.wall,
    settled,
    aimedAt: aimedAt ?? null,
  });
}

/**
 * What the model was pointing at when it asked for this action, in its own
 * words. It comes from what the ref meant at the time, not from what the page
 * holds now, because the expectation was written against the former.
 */
export function aimedAtRef(session: Session, ref: string): AimedAt | undefined {
  const was = session.refs.recall(ref);
  return was === undefined ? undefined : { role: was.role, name: was.name };
}

/** Did this diff show anything at all happening? */
function nothingHappened(before: Snapshot, diff: Diff): boolean {
  return !somethingChanged({
    urlChanged: diff.urlChanged,
    url: diff.url,
    titleChanged: before.title !== diff.snapshot.title,
    title: diff.snapshot.title,
    newElements: diff.newElements,
    removedCount: 0,
    dialog: diff.dialog,
    newTab: diff.newTab,
    download: diff.download,
    aimedAt: null,
  });
}

/**
 * Click one element. A click that produces no visible change is tried once more at
 * the element's place on the screen, because a page that swallows a click on the
 * element often takes one on the pixels. The button clicked is no evidence of
 * what the click did, so the action names no aim: what changed on the page is
 * the whole of what the expectation is judged against.
 */
export async function clickMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<Diff> {
  const ref = String(params["ref"]);
  const expectation = String(params["expectation"] ?? "");
  let clicked: Awaited<ReturnType<typeof targetOf>> | undefined;
  const first = await actAndAssert(session, expectation, async (page) => {
    clicked = await targetOf(session, page, ref, freshSnapshotFor(session, page));
    await clickTarget(session, page, clicked);
    return undefined;
  });
  const before = first.snapshot;
  if (!nothingHappened(before, first) || clicked === undefined) {
    return first;
  }
  let triedAgain = false;
  const second = await actAndAssert(session, expectation, async (page) => {
    triedAgain = await clickAgainAtItsPlace(session, page, clicked!);
    return undefined;
  });
  return triedAgain ? second : first;
}

/** Type text into one element. */
export async function typeMethod(session: Session, params: Record<string, unknown>): Promise<Diff> {
  const ref = String(params["ref"]);
  const text = String(params["text"] ?? "");
  return actAndAssert(session, String(params["expectation"] ?? ""), async (page) => {
    const target = await targetOf(session, page, ref, freshSnapshotFor(session, page));
    await typeIntoTarget(session, page, target, text, false);
    return aimedAtRef(session, ref);
  });
}

/** Press one key or key combination. */
export async function pressMethod(session: Session, params: Record<string, unknown>): Promise<Diff> {
  const key = String(params["key"]);
  return actAndAssert(session, String(params["expectation"] ?? ""), async (page) => {
    await pressOneKey(session, page, key);
    return undefined;
  });
}

/**
 * Answer the open dialog box. Chrome stops the whole tab until a dialog is
 * answered, so until this runs the tab can be neither read nor acted on, and this
 * is the only way it comes back to life.
 *
 * The dialog is taken off the tab before it is answered, so that the wait for a
 * dialog inside the action does not fire on the very dialog being answered.
 */
export async function dialogMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<Diff> {
  const action = params["action"] as DialogAnswer;
  const text = typeof params["text"] === "string" ? params["text"] : "";
  const page = session.currentPage();
  const waiting = session.takeOpenDialog(page);
  if (waiting === undefined) {
    throw wrongParameters(
      "There is no dialog box open on this page, so there is nothing to answer. Read the page to see what is on it.",
    );
  }
  session.log(`answering the open dialog with ${action}.`);
  return actAndAssert(session, String(params["expectation"] ?? ""), async () => {
    if (action === "accept") {
      await waiting.accept(text);
    } else {
      await waiting.dismiss();
    }
    return undefined;
  });
}

/** Scroll the page in steps. */
export async function scrollMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<Diff> {
  const direction = params["direction"] === "up" ? "up" : "down";
  const amount = typeof params["amount"] === "number" ? params["amount"] : DEFAULT_SCROLL_STEPS;
  return actAndAssert(session, String(params["expectation"] ?? ""), async (page) => {
    await scrollInSteps(session, page, direction, amount);
    return undefined;
  });
}
