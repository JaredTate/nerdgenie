/**
 * The eleven methods of PROTOCOL.md, and the one place a request turns into work.
 */
import {
  clickMethod,
  pressMethod,
  readOrSayItCannotBeRead,
  scrollMethod,
  typeMethod,
} from "./act-methods.js";
import { actMethod, loginFillMethod, screenshotMethod } from "./batch-methods.js";
import { settle } from "./settle.js";
import { tabsMethod } from "./tabs.js";
import type { Session } from "./session.js";
import type { Diff, MethodName, WorkerRequest } from "./types.js";

/** What every method is: a session, some parameters, and a result object. */
type Method = (session: Session, params: Record<string, unknown>) => Promise<Record<string, unknown>>;

/** How long to wait for a move to a new address, which is longer than settling takes. */
const GO_TO_LIMIT_MS = 30_000;

/** Go to an address and return the page. */
const open: Method = async (session, params) => {
  session.beginAction();
  const page = session.currentPageOrNone() ?? (await session.chrome.context.newPage());
  session.makeActive(page);
  await page.goto(String(params["url"]), {
    waitUntil: "domcontentloaded",
    timeout: GO_TO_LIMIT_MS,
  });
  await settle(session, page);
  // A page just arrived at, so nothing on it counts as new. The snapshot carries
  // the wall, which is how `open` reports that a page is a login page before the
  // agent has done anything at all.
  const reading = await readOrSayItCannotBeRead(session, page, {
    visibleOnly: false,
    against: null,
  });
  session.rememberSnapshot(reading.snapshot);
  if (reading.wall !== null) {
    session.log(`the page is behind a ${reading.wall.kind} wall: ${reading.wall.detail}.`);
  }
  return { ...reading.snapshot };
};

/** Return a fresh snapshot of the page the worker is on. */
const read: Method = async (session, params) => {
  const page = session.currentPage();
  const reading = await readOrSayItCannotBeRead(session, page, {
    visibleOnly: params["visibleOnly"] === true,
    against: session.previousSnapshot(),
  });
  session.rememberSnapshot(reading.snapshot);
  return { ...reading.snapshot };
};

/** Every diff is an object of its own fields, which is what the result must be. */
function asDiffResult(diff: Diff): Record<string, unknown> {
  return { ...diff };
}

/** Say whether the worker can act on a page, and which Chrome it drove. */
const health: Method = async (session) => ({
  healthy: session.chrome.isAlive(),
  chromeVersion: session.chrome.version,
});

const METHODS: Readonly<Record<MethodName, Method>> = {
  open,
  read,
  click: async (session, params) => asDiffResult(await clickMethod(session, params)),
  type: async (session, params) => asDiffResult(await typeMethod(session, params)),
  press: async (session, params) => asDiffResult(await pressMethod(session, params)),
  scroll: async (session, params) => asDiffResult(await scrollMethod(session, params)),
  act: async (session, params) => actMethod(session, params),
  tabs: async (session, params) => tabsMethod(session, params),
  loginFill: async (session, params) => loginFillMethod(session, params),
  screenshot: async (session) => screenshotMethod(session),
  health,
};

/** Run one request against the browser. */
export function runMethod(
  session: Session,
  request: WorkerRequest,
): Promise<Record<string, unknown>> {
  return METHODS[request.method](session, request.params);
}
