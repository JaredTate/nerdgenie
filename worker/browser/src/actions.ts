/**
 * Doing one thing to a page, the way a person would.
 *
 * Every action follows the same steps, which are the ones design section 9 lays
 * out: find the element, scroll it into view, wait until it is visible, enabled,
 * and no longer moving, act with human pacing, and hand back to the caller, which
 * waits for the page to settle and builds the diff.
 *
 * A page can open a dialog box in the middle of a click, and a page holding an
 * open dialog answers nothing at all, so every action is raced against a dialog
 * appearing. Without that race a single `confirm()` would hang the worker.
 */
import type { Locator, Page } from "playwright-core";
import { noSuchReference } from "./errors.js";
import { findRef, type FoundTarget } from "./refs.js";
import {
  clickHoldMs,
  mousePath,
  pauseMs,
  scrollDeltas,
  scrollPauseMs,
  wait,
  type Point,
} from "./pacing.js";
import { boxOf } from "./page-bridge.js";
import type { Session } from "./session.js";

/** How long to wait for an element to become ready to be acted on. */
const READY_LIMIT_MS = 8_000;

/** How often to look for a dialog while an action is running. */
const DIALOG_WATCH_MS = 25;

/**
 * Run an action, but stop waiting for it the moment a dialog opens. The action's
 * own promise is left to finish or fail on its own, because nothing is waiting on
 * it any more and an unwatched failure must not take the worker down.
 */
export async function actionOrDialog(
  session: Session,
  page: Page,
  work: () => Promise<void>,
): Promise<void> {
  const running = work();
  running.catch(() => {});
  let watch: NodeJS.Timeout | undefined;
  const dialogOpened = new Promise<void>((opened) => {
    watch = setInterval(() => {
      if (session.dialogOn(page) !== null) {
        opened();
      }
    }, DIALOG_WATCH_MS);
    watch.unref();
  });
  try {
    await Promise.race([running, dialogOpened]);
  } finally {
    clearInterval(watch);
  }
}

/** Find the element a ref points at, or fail with -32000 and a fresh snapshot. */
export async function targetOf(
  session: Session,
  page: Page,
  ref: string,
  freshSnapshot: () => Promise<Record<string, unknown>>,
): Promise<FoundTarget> {
  const found = await findRef(page, session.refs, ref);
  if (found === undefined) {
    throw noSuchReference(ref, await freshSnapshot());
  }
  if (found.how !== "by its ref") {
    session.log(`the ref ${ref} had gone stale and the element was found again ${found.how}.`);
  }
  return found;
}

/** Wait until an element is scrolled into view, visible, still, and enabled. */
async function readyToAct(locator: Locator): Promise<void> {
  await locator.scrollIntoViewIfNeeded({ timeout: READY_LIMIT_MS });
  await locator.waitFor({ state: "visible", timeout: READY_LIMIT_MS });
  if (!(await locator.isEnabled({ timeout: READY_LIMIT_MS }))) {
    await locator.waitFor({ state: "attached", timeout: READY_LIMIT_MS });
  }
}

/** The middle of an element, in the coordinates the mouse uses. */
async function middleOf(locator: Locator): Promise<Point | undefined> {
  const box = await locator.boundingBox({ timeout: READY_LIMIT_MS });
  return box === null ? undefined : { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

/** Move the mouse there along a curve and press the button for a human length of time. */
export async function pressMouseAt(session: Session, page: Page, at: Point): Promise<void> {
  const from = session.mouseAt();
  for (const step of mousePath(from, at, session.pacing, session.chance)) {
    await page.mouse.move(step.x, step.y);
  }
  session.mouseMovedTo(at);
  await page.mouse.down();
  await wait(clickHoldMs(session.pacing, session.chance));
  await page.mouse.up();
}

/** Click one element. */
export async function clickTarget(session: Session, page: Page, target: FoundTarget): Promise<void> {
  await readyToAct(target.locator);
  const at = await middleOf(target.locator);
  if (at === undefined) {
    await target.locator.click({ timeout: READY_LIMIT_MS });
    return;
  }
  await pressMouseAt(session, page, at);
  await wait(pauseMs(session.pacing, session.chance));
}

/** Click again at the place on the screen where the element was, and nowhere else. */
export async function clickAgainAtItsPlace(
  session: Session,
  page: Page,
  target: FoundTarget,
): Promise<boolean> {
  const box = await boxOf(target.frame, target.ref).catch(() => null);
  if (box === null || box.width === 0 || box.height === 0) {
    return false;
  }
  session.log("the click changed nothing, so the worker clicked it again at its place on the screen.");
  await pressMouseAt(session, page, {
    x: box.x + box.width / 2,
    y: box.y + box.height / 2,
  });
  return true;
}

/** Type into one element, one key at a time, with a small pause between keys. */
export async function typeIntoTarget(
  session: Session,
  page: Page,
  target: FoundTarget,
  text: string,
  clearFirst: boolean,
): Promise<void> {
  await readyToAct(target.locator);
  await target.locator.focus({ timeout: READY_LIMIT_MS });
  if (clearFirst) {
    // Emptying a field is not paced: there is nothing in an empty field for a
    // page to watch the speed of.
    await target.locator.fill("", { timeout: READY_LIMIT_MS });
  }
  const gaps = session.keyGaps(text.length);
  for (const [position, character] of [...text].entries()) {
    await page.keyboard.type(character);
    await wait(gaps[position] ?? 0);
  }
  await wait(pauseMs(session.pacing, session.chance));
}

/** Press one key or key combination. */
export async function pressOneKey(session: Session, page: Page, key: string): Promise<void> {
  await page.keyboard.press(key);
  await wait(pauseMs(session.pacing, session.chance));
}

/** Scroll the page in steps, the way a person turns a wheel. */
export async function scrollInSteps(
  session: Session,
  page: Page,
  direction: "up" | "down",
  amount: number,
): Promise<void> {
  for (const delta of scrollDeltas(direction, amount)) {
    await page.mouse.wheel(0, delta);
    await wait(scrollPauseMs(session.pacing, session.chance));
  }
  await wait(pauseMs(session.pacing, session.chance));
}
