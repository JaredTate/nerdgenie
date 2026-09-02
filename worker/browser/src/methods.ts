/**
 * The eleven methods of PROTOCOL.md, and the one place a request turns into work.
 */
import { chromeDied } from "./errors.js";
import { readPage } from "./snapshot.js";
import { settle } from "./settle.js";
import type { Session } from "./session.js";
import type { MethodName, Snapshot, Wall, WorkerRequest } from "./types.js";

/** What every method is: a session, some parameters, and a result object. */
type Method = (session: Session, params: Record<string, unknown>) => Promise<Record<string, unknown>>;

/** How long to wait for a move to a new address, which is longer than settling takes. */
const GO_TO_LIMIT_MS = 30_000;

/**
 * A snapshot as the Go side reads it. The `wall` is not in the protocol's own
 * example for open and read, but the brief says the worker looks for a wall after
 * every open, and a wall nobody is told about would be no use at all.
 */
function asResult(snapshot: Snapshot, wall: Wall | null): Record<string, unknown> {
  return { ...snapshot, wall };
}

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
  // A page just arrived at, so nothing on it counts as new.
  const reading = await readPage(session, page, { visibleOnly: false, against: null });
  session.rememberSnapshot(reading.snapshot);
  if (reading.wall !== null) {
    session.log(`the page is behind a ${reading.wall.kind} wall: ${reading.wall.detail}.`);
  }
  return asResult(reading.snapshot, reading.wall);
};

/** Return a fresh snapshot of the page the worker is on. */
const read: Method = async (session, params) => {
  const page = session.currentPage();
  const reading = await readPage(session, page, {
    visibleOnly: params["visibleOnly"] === true,
    against: session.previousSnapshot(),
  });
  session.rememberSnapshot(reading.snapshot);
  return asResult(reading.snapshot, reading.wall);
};

const notBuiltYet: Method = async () => {
  throw chromeDied("that method has not been built yet.");
};

/** Say whether the worker can act on a page, and which Chrome it drove. */
const health: Method = async (session) => ({
  healthy: session.chrome.isAlive(),
  chromeVersion: session.chrome.version,
});

const METHODS: Readonly<Record<MethodName, Method>> = {
  open,
  read,
  click: notBuiltYet,
  type: notBuiltYet,
  press: notBuiltYet,
  scroll: notBuiltYet,
  act: notBuiltYet,
  tabs: notBuiltYet,
  loginFill: notBuiltYet,
  screenshot: notBuiltYet,
  health,
};

/** Run one request against the browser. */
export function runMethod(
  session: Session,
  request: WorkerRequest,
): Promise<Record<string, unknown>> {
  return METHODS[request.method](session, request.params);
}
