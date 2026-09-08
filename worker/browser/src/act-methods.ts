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
  clickAtPoint,
  clickTarget,
  pressOneKey,
  scrollInSteps,
  targetOf,
  typeIntoTarget,
} from "./actions.js";
import { buildDiff } from "./diff.js";
import { somethingChanged, type AimedAt } from "./expectation.js";
import { DEFAULT_SCROLL_STEPS, LATE_REACTION_MS } from "./limits.js";
import { couldNotBeRead, wrongParameters, WorkerError } from "./errors.js";
import { SETTLE_LIMIT_MS } from "./limits.js";
import type { Point } from "./pacing.js";
import { viewportOf } from "./page-bridge.js";
import type { FoundTarget } from "./refs.js";
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
  against?: Snapshot,
): Promise<Diff> {
  const page = session.currentPage();
  session.beginAction();
  const before = against ?? (await snapshotBefore(session, page));
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
/**
 * The listed element under a point, as the outline names it, or the words
 * for none. The page script's role and name readers are on the page once it
 * has been read; a page not yet read answers by tag and text.
 */
async function whatIsUnder(page: Page, at: Point): Promise<string> {
  const x = Math.round(at.x);
  const y = Math.round(at.y);
  const script = `(function () {
    var hit = document.elementFromPoint(${x}, ${y});
    if (!hit) { return "nothing the outline lists"; }
    var listed = hit.closest("[data-nerdgenie-ref]");
    if (!listed || !window.__nerdgenieRoleOf || !window.__nerdgenieNameOf) { return "nothing the outline lists"; }
    var role = window.__nerdgenieRoleOf(listed);
    if (!role) { return "nothing the outline lists"; }
    var name = window.__nerdgenieNameOf(listed, role, 80);
    return listed.getAttribute("data-nerdgenie-ref") + ' ' + role + ' "' + name + '"';
  })()`;
  try {
    return String(await page.evaluate(script));
  } catch {
    return "";
  }
}

/**
 * The aimed element's state after the action, in the words the diff's aimed-state
 * field carries, or the empty string when the element is not found or is gone.
 *
 * A click is aimed either at an element the model named by ref or at whatever a
 * point lands on, and either way this reports what became of that element, so a
 * click that changed nothing the outline names still shows the mark it left: a
 * game cell whose only mark is an aria-hidden drawing keeps the same accessible
 * name before and after, and the model kept clicking it again. The phrase is the
 * element's role and name, then its telltales joined by commas: the word
 * disabled, then pressed, checked, or selected, then each data attribute, then
 * the short text it holds, capped at six with an ellipsis when there are more.
 * The role and name readers are the same ones the outline uses, and they are on
 * the page once it has been read.
 */
async function aimedElementState(
  page: Page,
  aim: { ref: string } | { point: Point },
): Promise<string> {
  const finder =
    "ref" in aim
      ? `document.querySelector('[data-nerdgenie-ref="${aim.ref}"]')`
      : `(function () { var hit = document.elementFromPoint(${Math.round(aim.point.x)}, ${Math.round(aim.point.y)}); return hit ? hit.closest('[data-nerdgenie-ref]') : null; })()`;
  const script = `(function () {
    var element = ${finder};
    if (!element || !window.__nerdgenieRoleOf || !window.__nerdgenieNameOf) { return ''; }
    var role = window.__nerdgenieRoleOf(element);
    if (!role) { return ''; }
    var name = window.__nerdgenieNameOf(element, role, 80);
    var telltales = [];
    if (element.disabled === true || element.getAttribute('aria-disabled') === 'true') { telltales.push('disabled'); }
    if (element.getAttribute('aria-pressed') === 'true') { telltales.push('pressed'); }
    if (element.getAttribute('aria-checked') === 'true') { telltales.push('checked'); }
    if (element.getAttribute('aria-selected') === 'true') { telltales.push('selected'); }
    var attributes = element.attributes;
    for (var at = 0; at < attributes.length; at += 1) {
      var attribute = attributes[at];
      if (attribute.name.indexOf('data-') === 0 && attribute.name !== 'data-nerdgenie-ref') {
        telltales.push(attribute.name + '="' + attribute.value + '"');
      }
    }
    var held = (element.textContent || '').replace(/\\s+/g, ' ').trim();
    if (held !== '' && held.length <= 40) { telltales.push('holds "' + held + '"'); }
    if (telltales.length > 6) { telltales = telltales.slice(0, 6); telltales.push('\\u2026'); }
    var phrase = role + ' "' + name + '"';
    if (telltales.length > 0) { phrase += ', ' + telltales.join(', '); }
    return phrase;
  })()`;
  try {
    return String(await page.evaluate(script));
  } catch {
    return "";
  }
}

function nothingHappened(before: Snapshot, diff: Diff): boolean {
  return !somethingChanged({
    urlChanged: diff.urlChanged,
    url: diff.url,
    titleChanged: before.title !== diff.snapshot.title,
    title: diff.snapshot.title,
    newElements: diff.newElements,
    newText: diff.newText,
    removedCount: 0,
    dialog: diff.dialog,
    newTab: diff.newTab,
    download: diff.download,
    aimedAt: null,
  });
}

/** The point a click names, when it names one rather than a ref. */
function pointOf(params: Record<string, unknown>): Point | undefined {
  const x = params["x"];
  const y = params["y"];
  return typeof x === "number" && typeof y === "number" ? { x, y } : undefined;
}

/**
 * Refuse, with -32602, a point that is not on the page's viewport. A click
 * there would land on nothing, so the model is told the size and what to send.
 */
async function refuseAPointOffThePage(page: Page, at: Point): Promise<void> {
  const size = await viewportOf(page);
  if (at.x < 0 || at.y < 0 || at.x >= size.width || at.y >= size.height) {
    throw wrongParameters(
      `The click method's point (${at.x}, ${at.y}) is outside the page's viewport, which is ${size.width} by ${size.height} CSS pixels from its top left. Send a point inside it, or read the page and click by ref.`,
    );
  }
}

/**
 * Click one element, or one point. A click that seems to have changed nothing
 * is given one more look a moment later, because a page can react late: the
 * game the thirteenth nightly run built hid its start overlay half a second
 * after the click, once its sound was set up, and a second click on it started
 * a game that had already started. A click on an element that has still
 * changed nothing is tried once more at the element's place on the screen,
 * because a page that swallows a click on the element often takes one on the
 * pixels; a click at a point is already at its place, so it is not tried again.
 * The thing clicked is no evidence of what the click did, so the action names
 * no aim: what changed on the page is the whole of what the expectation is
 * judged against, and every look is judged against the page as it was before
 * the first click.
 */
export async function clickMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<Diff> {
  const expectation = String(params["expectation"] ?? "");
  const point = pointOf(params);
  if (point !== undefined) {
    await refuseAPointOffThePage(session.currentPage(), point);
  }
  let clicked: FoundTarget | undefined;
  let before: Snapshot | undefined;
  const first = await actAndAssert(session, expectation, async (page, was) => {
    before = was;
    if (point !== undefined) {
      await clickAtPoint(session, page, point);
      return undefined;
    }
    clicked = await targetOf(session, page, String(params["ref"]), freshSnapshotFor(session, page));
    await clickTarget(session, page, clicked);
    return undefined;
  });
  // A page holding an open dialog cannot be evaluated at all: reading it through
  // the dialog would hang the worker until the dialog was answered, the same
  // reason a snapshot of such a page is left empty. So when a click opens a
  // dialog, neither what was under the point nor the aimed element's state is
  // read; the dialog is reported on the diff instead, and both are left empty.
  if (session.dialogOn(session.currentPage()) === null) {
    if (point !== undefined) {
      // A click at a point says what was under it: run 28's model clicked at
      // stale coordinates more than a hundred times, each answered "nothing
      // changed", and never learned what its points hit.
      first.under = await whatIsUnder(session.currentPage(), point);
      first.aimedState = await aimedElementState(session.currentPage(), { point });
    } else {
      // A click by ref says what became of the element it aimed at, so a click
      // that changed nothing the outline names still shows the mark it left: the
      // tic-tac-toe cells kept the same accessible name and the model clicked
      // them again and again. The ref used is the one the element carries now,
      // which is a fresh one when a stale ref had to be found again.
      first.aimedState = await aimedElementState(session.currentPage(), {
        ref: clicked?.ref ?? String(params["ref"]),
      });
    }
  }
  if (!nothingHappened(first.snapshot, first) || before === undefined) {
    return first;
  }
  const later = await actAndAssert(
    session,
    expectation,
    async () => {
      await new Promise((resolve) => setTimeout(resolve, LATE_REACTION_MS));
      return undefined;
    },
    before,
  );
  if (point !== undefined) {
    later.under = first.under;
  }
  // The later look is only a wait, so the element it aimed at is the same one,
  // and it carries the state the first look found, as it carries what a point hit.
  later.aimedState = first.aimedState;
  if (!nothingHappened(later.snapshot, later)) {
    session.log("the click changed nothing at first, and the page had changed a moment later.");
    return later;
  }
  if (clicked === undefined) {
    return later;
  }
  let triedAgain = false;
  const second = await actAndAssert(
    session,
    expectation,
    async (page) => {
      triedAgain = await clickAgainAtItsPlace(session, page, clicked!);
      return undefined;
    },
    before,
  );
  // This look clicked the element again, so its aimed state is read fresh: a page
  // that swallowed the first click and took the second leaves the element changed
  // only now, and a copy of the first look's state would still show it untouched.
  // A re-click that opened a dialog leaves the page unreadable, so the state stays
  // empty and the dialog on the diff is what the model sees.
  if (session.dialogOn(session.currentPage()) === null) {
    second.aimedState = await aimedElementState(session.currentPage(), {
      ref: clicked.ref,
    });
  }
  return triedAgain ? second : later;
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
