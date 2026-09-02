/**
 * The error codes in worker/browser/PROTOCOL.md, and the one error class the
 * worker throws when it wants to answer with one of them.
 */

/** The codes from the error table in PROTOCOL.md, with the meaning each one carries. */
export const ERROR_CODES = {
  /** The line was not JSON. The Go side restarts the worker. */
  notJson: -32700,
  /** The request was not a valid JSON-RPC request. The Go side restarts the worker. */
  notAValidRequest: -32600,
  /** No such method. A bug in the Go side; it reports it. */
  noSuchMethod: -32601,
  /** The parameters were wrong. The Go side returns the message to the model. */
  wrongParameters: -32602,
  /** No such reference on the page, after every way of finding it failed. */
  noSuchReference: -32000,
  /** The page could not be read at all after the settle limit. */
  didNotSettle: -32001,
  /** No browser is open. */
  noBrowserOpen: -32002,
  /** Chrome died. The Go side restarts the worker and tells the model it was interrupted. */
  chromeDied: -32003,
} as const;

export type ErrorCode = (typeof ERROR_CODES)[keyof typeof ERROR_CODES];

/**
 * An error the worker means to send back as a JSON-RPC error object. Anything
 * else that goes wrong becomes a -32003, because an unexplained failure inside
 * the worker means the browser can no longer be trusted.
 */
export class WorkerError extends Error {
  readonly code: ErrorCode;
  readonly data: Record<string, unknown> | undefined;

  constructor(code: ErrorCode, message: string, data?: Record<string, unknown>) {
    super(message);
    this.name = "WorkerError";
    this.code = code;
    this.data = data;
  }
}

/** The parameters were wrong. Says what was expected. */
export function wrongParameters(message: string): WorkerError {
  return new WorkerError(ERROR_CODES.wrongParameters, message);
}

/** No such reference. Carries a fresh snapshot so the model can point at something that exists. */
export function noSuchReference(ref: string, data: Record<string, unknown>): WorkerError {
  return new WorkerError(
    ERROR_CODES.noSuchReference,
    `There is no element ${ref} on the page any more, and looking for it by its role, its name, and its text all failed. Point at an element in the snapshot below.`,
    data,
  );
}

/**
 * The page could not be read at all after the settle limit. A page that merely
 * keeps changing is read as it stands and reported with `settled: false`; this is
 * for a page that throws its own document away faster than it can be looked at.
 */
export function couldNotBeRead(limitMs: number, why: string): WorkerError {
  return new WorkerError(
    ERROR_CODES.didNotSettle,
    `The page could not be read at all after ${limitMs} milliseconds: ${why}. Open the page again, or open a different one.`,
  );
}

/** No browser is open. */
export function noBrowserOpen(): WorkerError {
  return new WorkerError(
    ERROR_CODES.noBrowserOpen,
    "No page is open in the browser. Open a page first.",
  );
}

/** Chrome died, or something inside the worker failed in a way that leaves the browser untrustworthy. */
export function chromeDied(reason: string): WorkerError {
  return new WorkerError(ERROR_CODES.chromeDied, `Chrome stopped working: ${reason}`);
}

/** Turn anything thrown into the error the Go side will read. */
export function asWorkerError(thrown: unknown): WorkerError {
  if (thrown instanceof WorkerError) {
    return thrown;
  }
  const reason = thrown instanceof Error ? thrown.message : String(thrown);
  return chromeDied(reason);
}
