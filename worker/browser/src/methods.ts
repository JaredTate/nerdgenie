/**
 * The eleven methods of PROTOCOL.md, and the one place a request turns into work.
 */
import { chromeDied } from "./errors.js";
import type { Session } from "./session.js";
import type { MethodName, WorkerRequest } from "./types.js";

/** What every method is: a session, some parameters, and a result object. */
type Method = (session: Session, params: Record<string, unknown>) => Promise<Record<string, unknown>>;

/** Say whether the worker can act on a page, and which Chrome it drove. */
const health: Method = async (session) => {
  return {
    healthy: session.chrome.isAlive(),
    chromeVersion: session.chrome.version,
  };
};

const notBuiltYet: Method = async (_session, _params) => {
  throw chromeDied("that method has not been built yet.");
};

const METHODS: Readonly<Record<MethodName, Method>> = {
  open: notBuiltYet,
  read: notBuiltYet,
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
