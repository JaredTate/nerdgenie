/**
 * The three methods built out of the other ones: act, loginFill, and screenshot.
 */
import type { Page } from "playwright-core";
import {
  actAndAssert,
  clickMethod,
  pressMethod,
  scrollMethod,
  typeMethod,
} from "./act-methods.js";
import { targetOf, typeIntoTarget } from "./actions.js";
import { freshSnapshotFor } from "./act-methods.js";
import { FRAME_COUNT_MS, MAX_ACT_STEPS, MAX_SCREENSHOT_MARKS } from "./limits.js";
import { clearMarks, drawMarks, framesDrawnIn } from "./page-bridge.js";
import { redactDeep } from "./redact.js";
import { readPage } from "./snapshot.js";
import type { Session } from "./session.js";
import type { ActStep, Diff, ScreenshotMark, StepMethodName } from "./types.js";

/** The four methods a step may be, and the function that runs each one. */
const STEP_RUNNERS: Readonly<
  Record<StepMethodName, (session: Session, params: Record<string, unknown>) => Promise<Diff>>
> = {
  click: clickMethod,
  type: typeMethod,
  press: pressMethod,
  scroll: scrollMethod,
};

/**
 * Should the batch stop after this step? It stops on a failed expectation, on a
 * wall, and on the page moving to a new address, because after any of those the
 * refs the later steps were written against may mean nothing at all.
 */
function batchShouldStop(diff: Diff): boolean {
  return !diff.expectationMet || diff.wall !== null || diff.urlChanged;
}

/**
 * Run a short batch of steps, stopping as soon as one expectation fails or the
 * page changes underneath. One diff per step that ran, so the model can see
 * exactly where the batch stopped.
 */
export async function actMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<{ diffs: Diff[] }> {
  const steps = (params["steps"] as ActStep[]).slice(0, MAX_ACT_STEPS);
  const diffs: Diff[] = [];
  for (const [position, step] of steps.entries()) {
    const diff = await STEP_RUNNERS[step.method](session, step as unknown as Record<string, unknown>);
    diffs.push(diff);
    if (batchShouldStop(diff)) {
      session.log(`the batch stopped after step ${position + 1} of ${steps.length}.`);
      break;
    }
  }
  return { diffs };
}

/** One value from the vault and the field it belongs in. */
interface LoginField {
  ref: string;
  value: string;
}

/** The values loginFill was given, in the order they are typed. */
function loginFieldsOf(params: Record<string, unknown>): LoginField[] {
  const pairs: Array<[string, string]> = [
    ["usernameRef", "username"],
    ["passwordRef", "password"],
    ["codeRef", "code"],
  ];
  const fields: LoginField[] = [];
  for (const [refName, valueName] of pairs) {
    const ref = params[refName];
    const value = params[valueName];
    if (typeof ref === "string" && typeof value === "string" && value !== "") {
      fields.push({ ref, value });
    }
  }
  return fields;
}

/**
 * Type a username, a password, and a code into the fields the model pointed at,
 * press Enter in the last one, and answer with a diff in which none of the three
 * values survives anywhere. The whole answer is walked, not only the fields they
 * were typed into, because a page can echo a value into an address or a title.
 */
export async function loginFillMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const fields = loginFieldsOf(params);
  const secrets = fields.map((field) => field.value);
  const diff = await actAndAssert(session, "", async (page) => {
    for (const [position, field] of fields.entries()) {
      const target = await targetOf(session, page, field.ref, freshSnapshotFor(session, page));
      await typeIntoTarget(session, page, target, field.value, true);
      if (position === fields.length - 1) {
        await target.locator.press("Enter").catch(() => {});
      }
    }
    return undefined;
  });
  session.log(`filled ${fields.length} login field or fields and pressed Enter in the last one.`);
  // Everything the worker remembers about this page could hold a value too, so
  // the remembered snapshot is cleaned along with the answer.
  const clean = redactDeep(diff, secrets) as Diff;
  session.rememberSnapshot(clean.snapshot);
  return { ...clean };
}

/** The roles a person clicks, which are the ones worth numbering on a picture. */
const CLICKABLE_ROLES: ReadonlySet<string> = new Set([
  "link",
  "button",
  "textbox",
  "checkbox",
  "radio",
  "combobox",
  "menuitem",
]);

/**
 * How many animation frames the page's own script drew in a quarter of a
 * second, as a whole number. A page that does not answer, because its own
 * script never yields, drew none that the worker could see.
 */
async function countFramesDrawn(session: Session, page: Page): Promise<number> {
  try {
    const counted = await framesDrawnIn(page, FRAME_COUNT_MS);
    return Number.isFinite(counted) && counted > 0 ? Math.floor(counted) : 0;
  } catch (problem) {
    const why = problem instanceof Error ? problem.message : String(problem);
    session.log(`the page did not say how many frames it drew, so the picture says none: ${why}`);
    return 0;
  }
}

/**
 * Take a picture of the page with its clickable elements numbered, which is what a
 * handoff sends to the user. Only what a person can see is numbered, because that
 * is all the picture shows. The window comes to the front first, because a tab
 * behind another draws no frames, and then the frames the page draws in a
 * quarter of a second are counted, so that the same picture twice can be told
 * apart from a camera that is broken.
 */
export async function screenshotMethod(session: Session): Promise<Record<string, unknown>> {
  const page = session.currentPage();
  await page.bringToFront().catch(() => {});
  const framesDrawn = await countFramesDrawn(session, page);
  const reading = await readPage(session, page, { visibleOnly: true, against: null });
  const marks: ScreenshotMark[] = reading.snapshot.elements
    .filter((element) => CLICKABLE_ROLES.has(element.role))
    .slice(0, MAX_SCREENSHOT_MARKS)
    .map((element, at) => ({
      number: at + 1,
      ref: element.ref,
      role: element.role,
      name: element.name,
    }));
  await drawMarks(page, marks).catch(() => 0);
  try {
    const picture = await page.screenshot({ type: "png" });
    return { pngBase64: picture.toString("base64"), marks, framesDrawn };
  } finally {
    await clearMarks(page).catch(() => false);
  }
}
