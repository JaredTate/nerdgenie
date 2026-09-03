/**
 * Every bound the browser worker keeps, in one place.
 *
 * Coeus says: bound everything. Every loop has a limit, every wait a timeout,
 * every buffer a cap, every outside call a failure path. Keeping the numbers
 * together means the checker that validates a request and the code that runs it
 * can never disagree about a limit.
 *
 * The design of gathering the model-facing limits and the runtime limits into
 * one shared module is borrowed from OpenClaw's action policy at
 * ~/Code/openclaw/extensions/browser/src/browser/act-policy.ts. The numbers here
 * are ours, and the code is written fresh.
 */

/** The longest line the worker will read from standard input, in bytes. */
export const MAX_LINE_BYTES = 1_000_000;

/** The most elements one snapshot may hold. Everything past this is counted, not listed. */
export const MAX_SNAPSHOT_ELEMENTS = 300;

/** The most frames the snapshot walks on one page, counting the main frame. */
export const MAX_FRAMES = 20;

/** Ref numbers are spaced this far apart between frames so two frames never mint the same ref. */
export const REF_NUMBERS_PER_FRAME = 10_000;

/** The longest accessible name kept for one element. Longer names are cut and end in an ellipsis. */
export const MAX_ELEMENT_NAME_CHARS = 120;

/** The most characters of the page's text one snapshot carries. Its last line says how much was cut. */
export const MAX_PAGE_TEXT_CHARS = 8_000;

/** The most characters the type method will type in one call. */
export const MAX_TYPE_CHARS = 10_000;

/** The most steps one act batch may hold. */
export const MAX_ACT_STEPS = 10;

/** The range of scroll steps the scroll method accepts, and what it uses when none is given. */
export const MIN_SCROLL_STEPS = 1;
export const MAX_SCROLL_STEPS = 10;
export const DEFAULT_SCROLL_STEPS = 3;

/** The most tabs the tabs method will list. */
export const MAX_TABS = 50;

/** The most elements a screenshot numbers with drawn marks. */
export const MAX_SCREENSHOT_MARKS = 60;

/** The largest PDF the worker will save out of a page, in bytes. */
export const MAX_PDF_BYTES = 64 * 1024 * 1024;

/** The most refs the worker remembers, so that a long session cannot grow without end. */
export const MAX_REMEMBERED_REFS = 2_000;

/**
 * Settling. The page has settled when a move to a new address has finished, or
 * when nothing on the page has changed for the quiet stretch below. The limit is
 * how long the worker waits before it gives up and answers -32001.
 */
export const SETTLE_QUIET_MS = 300;
export const SETTLE_LIMIT_MS = 3_000;
export const SETTLE_POLL_MS = 50;

/** How long the worker waits for Chrome to print its DevTools address, and how often it looks. */
export const CHROME_READY_LIMIT_MS = 20_000;
export const CHROME_READY_POLL_MS = 100;

/** How long the worker waits for Chrome to stop after asking politely, before it uses force. */
export const CHROME_STOP_LIMIT_MS = 3_000;

/** The most bytes of Chrome's own logging kept for an error message. */
export const CHROME_STDERR_TAIL_BYTES = 64 * 1024;

/**
 * Watching what the person does. A burst of typing is reported once, after the
 * keyboard has been quiet this long, and one worker sends no more than this many
 * events in a minute, so that a page calling the watcher itself cannot flood the
 * pipe to the Go side.
 */
export const PERSON_TYPING_QUIET_MS = 400;
export const MAX_PERSON_EVENTS_PER_MINUTE = 240;

/** The deadline for each method, in milliseconds. The Go side has its own, longer, deadline. */
export const METHOD_DEADLINE_MS: Readonly<Record<string, number>> = {
  open: 45_000,
  read: 15_000,
  click: 25_000,
  type: 40_000,
  press: 25_000,
  scroll: 25_000,
  act: 120_000,
  tabs: 15_000,
  loginFill: 45_000,
  screenshot: 20_000,
  health: 10_000,
};

/** The deadline for one call into the page. A page holding an open dialog never answers. */
export const PAGE_CALL_DEADLINE_MS = 5_000;
