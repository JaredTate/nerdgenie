/**
 * The wire: one JSON-RPC request per line in, one response per line out.
 *
 * Nothing here touches the browser. It turns a line of text into either a
 * request the worker can run or the exact response the Go side should read,
 * which is what makes every error code in PROTOCOL.md testable on its own.
 */
import { ERROR_CODES, WorkerError } from "./errors.js";
import { MAX_LINE_BYTES } from "./limits.js";
import { checkParams } from "./params.js";
import { METHOD_NAMES, type JsonRpcResponse, type MethodName, type WorkerRequest } from "./types.js";

/** Either a request the worker should run, or the response to send instead. */
export type ParsedLine =
  | { kind: "request"; request: WorkerRequest }
  | { kind: "response"; response: JsonRpcResponse };

const NOT_JSON_ADVICE = "Send one JSON-RPC request object per line.";

const NOT_A_VALID_REQUEST_MESSAGE =
  'The request was not a valid JSON-RPC request. It needs jsonrpc "2.0", a whole number id, a method name, and params as an object.';

/** Build an error response. The id is null when the line was too broken to hold one. */
export function errorResponse(
  id: number | null,
  code: number,
  message: string,
  data?: Record<string, unknown>,
): JsonRpcResponse {
  const error = data === undefined ? { code, message } : { code, message, data };
  return { jsonrpc: "2.0", id, error };
}

/** Build a result response. */
export function successResponse(id: number, result: Record<string, unknown>): JsonRpcResponse {
  return { jsonrpc: "2.0", id, result };
}

/** Turn an error the worker threw into the response for it. */
export function responseForError(id: number, thrown: WorkerError): JsonRpcResponse {
  return errorResponse(id, thrown.code, thrown.message, thrown.data);
}

function asResponse(response: JsonRpcResponse): ParsedLine {
  return { kind: "response", response };
}

/** Is this a plain JSON object, rather than an array, a null, or something else? */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Does the parsed line have the four things a JSON-RPC request needs? */
function readValidRequest(parsed: unknown): WorkerRequest | undefined {
  if (!isPlainObject(parsed)) {
    return undefined;
  }
  const { jsonrpc, id, method, params } = parsed;
  if (jsonrpc !== "2.0" || typeof id !== "number" || !Number.isInteger(id)) {
    return undefined;
  }
  if (typeof method !== "string") {
    return undefined;
  }
  if (params !== undefined && !isPlainObject(params)) {
    return undefined;
  }
  return { id, method: method as MethodName, params: params ?? {} };
}

/**
 * Read one line from standard input. The checks run in the order of the error
 * table in PROTOCOL.md, so the worst failure is always reported first.
 */
export function parseLine(line: string): ParsedLine {
  if (Buffer.byteLength(line, "utf8") > MAX_LINE_BYTES) {
    return asResponse(
      errorResponse(
        null,
        ERROR_CODES.notJson,
        `The line was longer than ${MAX_LINE_BYTES} bytes. ${NOT_JSON_ADVICE}`,
      ),
    );
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(line);
  } catch {
    return asResponse(
      errorResponse(null, ERROR_CODES.notJson, `The line was not JSON. ${NOT_JSON_ADVICE}`),
    );
  }
  const request = readValidRequest(parsed);
  if (request === undefined) {
    return asResponse(errorResponse(null, ERROR_CODES.notAValidRequest, NOT_A_VALID_REQUEST_MESSAGE));
  }
  if (!METHOD_NAMES.includes(request.method)) {
    return asResponse(
      errorResponse(
        request.id,
        ERROR_CODES.noSuchMethod,
        `There is no method named ${JSON.stringify(request.method)}. The methods are open, read, click, type, press, scroll, act, tabs, loginFill, screenshot, and health.`,
      ),
    );
  }
  try {
    checkParams(request.method, request.params);
  } catch (thrown) {
    const failure =
      thrown instanceof WorkerError
        ? thrown
        : new WorkerError(ERROR_CODES.wrongParameters, String(thrown));
    return asResponse(responseForError(request.id, failure));
  }
  return { kind: "request", request };
}

/**
 * Write one response as one line. JSON.stringify escapes every newline the page
 * could have put in a title or a name, so a response can never break the framing.
 */
export function formatResponse(response: JsonRpcResponse): string {
  return `${JSON.stringify(response)}\n`;
}
