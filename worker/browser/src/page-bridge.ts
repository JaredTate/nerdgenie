/**
 * Calling into the page, with a deadline on every call.
 *
 * A page with an open dialog box never answers, and a page that is moving to a new
 * address throws instead of answering, so every call across this line is raced
 * against a clock and every failure has a name.
 */
import type { Frame, Page } from "playwright-core";
import { PAGE_CALL_DEADLINE_MS } from "./limits.js";
import { pageCall } from "./page-script.js";
import type { FrameText } from "./text.js";

/** One element as the page reported it, before the worker decides what to keep. */
export interface FoundElement {
  ref: string;
  role: string;
  name: string;
  aboveFold: boolean;
  password: boolean;
  shortNumeric: boolean;
}

/** What one frame reported about itself. */
export interface FrameScan {
  url: string;
  title: string;
  /** What the document says it is, such as "text/html" or "application/pdf". */
  contentType: string;
  elements: FoundElement[];
  /** What the frame says, as a person reads it. */
  text: FrameText;
}

/** How to scan one frame. */
export interface ScanSettings {
  refBase: number;
  roles: readonly string[];
  mostNodes: number;
  mostNameCharacters: number;
  mostTextCharacters: number;
}

/** How to look for an element whose ref went stale. */
export interface FindSettings {
  role: string;
  name: string;
  byText: boolean;
  refBase: number;
  mostNodes: number;
  mostNameCharacters: number;
}

/** Where an element sits on the screen. */
export interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** True when the page said no, rather than the call itself going wrong. */
export class PageDidNotAnswer extends Error {
  constructor(what: string, why: string) {
    super(`the page did not answer the ${what} call: ${why}`);
    this.name = "PageDidNotAnswer";
  }
}

/**
 * Run an expression inside a frame and give up after the deadline. A call that is
 * still outstanding is left to finish on its own and its failure is swallowed,
 * because nothing is waiting for it any more.
 */
export async function askPage<T>(frame: Frame | Page, what: string, expression: string): Promise<T> {
  const asked = frame.evaluate<T>(pageCall(expression));
  asked.catch(() => {});
  let timer: NodeJS.Timeout | undefined;
  const alarm = new Promise<never>((_, ringing) => {
    timer = setTimeout(
      () => ringing(new PageDidNotAnswer(what, `it was still busy after ${PAGE_CALL_DEADLINE_MS} milliseconds`)),
      PAGE_CALL_DEADLINE_MS,
    );
    timer.unref();
  });
  try {
    return await Promise.race([asked, alarm]);
  } finally {
    clearTimeout(timer);
  }
}

/** Ask one frame what is on it. */
export function scanFrame(frame: Frame, settings: ScanSettings): Promise<FrameScan> {
  return askPage<FrameScan>(frame, "scan", `window.__nerdgenieScan(${JSON.stringify(settings)})`);
}

/** Ask one frame to find an element again and hand back its ref. */
export function findLike(frame: Frame, settings: FindSettings): Promise<string | null> {
  return askPage<string | null>(
    frame,
    "find",
    `window.__nerdgenieFindLike(${JSON.stringify(settings)})`,
  );
}

/** Ask where an element sits on the screen. */
export function boxOf(frame: Frame, ref: string): Promise<Box | null> {
  return askPage<Box | null>(frame, "box", `window.__nerdgenieBoxOf(${JSON.stringify(ref)})`);
}

/** Ask how long the page has been quiet, in milliseconds. */
export function quietFor(page: Page): Promise<number> {
  return askPage<number>(page, "quiet", "window.__nerdgenieQuietFor()");
}

/** Draw numbered marks on the elements a screenshot should point at. */
export function drawMarks(
  page: Page,
  marks: ReadonlyArray<{ number: number; ref: string }>,
): Promise<number> {
  return askPage<number>(page, "marks", `window.__nerdgenieDrawMarks(${JSON.stringify(marks)})`);
}

/** Take the marks away again. */
export function clearMarks(page: Page): Promise<boolean> {
  return askPage<boolean>(page, "marks", "window.__nerdgenieClearMarks()");
}
