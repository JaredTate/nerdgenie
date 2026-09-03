// The wire the desktop worker speaks: JSON-RPC 2.0 over standard input and
// standard output, one object per line, as worker/desktop/PROTOCOL.md says.
// Every bound in this file is there so that a runaway writer on the other end
// cannot fill this process's memory.

/** The error codes from the table in PROTOCOL.md. */
export const DesktopErrorCode = {
  /** The line was not JSON, or it was longer than the cap. */
  ParseError: -32700,
  /** The request was not a valid JSON-RPC request. */
  InvalidRequest: -32600,
  /** The method is not one of the nine. */
  NoSuchMethod: -32601,
  /** The parameters were wrong. */
  BadParameters: -32602,
  /** There is no control with that number on the screen. */
  NoSuchMark: -32000,
  /** The window could not be read at all after the settle limit. */
  UnreadableWindow: -32001,
  /** No application is open, so there is nothing to act in. */
  NoApplicationOpen: -32002,
  /** The desktop driver is not available on this machine. */
  DriverUnavailable: -32003,
  /** The application could not be launched or brought forward. */
  LaunchFailed: -32004,
} as const

/** One of the nine codes above. */
export type DesktopErrorCodeValue = (typeof DesktopErrorCode)[keyof typeof DesktopErrorCode]

/** The longest line the worker will read, in bytes. */
export const maximumLineBytes = 1_048_576

/** The identifier a request carries, which is null when the line was not one. */
export type RequestID = number | string | null

/** One request read off standard input. */
export interface DesktopRequest {
  /** The identifier the response must echo. */
  id: RequestID
  /** The method name, one of the nine in PROTOCOL.md. */
  method: string
  /** The parameters, which are an empty object when the request sent none. */
  params: Record<string, unknown>
}

/**
 * ProtocolError is a failure with one of the protocol's codes on it, so that
 * every layer can throw and the one place that writes responses can answer.
 */
export class ProtocolError extends Error {
  /** The code from the table in PROTOCOL.md. */
  readonly code: DesktopErrorCodeValue
  /** What the protocol says to send alongside, such as a fresh screenshot. */
  readonly data: Record<string, unknown> | undefined
  /** The id to echo, which is null for a line that was never a request. */
  readonly requestID: RequestID

  constructor(
    code: DesktopErrorCodeValue,
    message: string,
    data?: Record<string, unknown>,
    requestID: RequestID = null,
  ) {
    super(message)
    this.name = "ProtocolError"
    this.code = code
    this.data = data
    this.requestID = requestID
  }
}

/** readRequest turns one line into a request, or throws a ProtocolError. */
export function readRequest(line: string): DesktopRequest {
  let parsed: unknown
  try {
    parsed = JSON.parse(line)
  } catch {
    throw new ProtocolError(DesktopErrorCode.ParseError, "the line was not JSON, so send one JSON object per line")
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new ProtocolError(DesktopErrorCode.InvalidRequest, "the line was not a JSON-RPC request object")
  }
  const fields = parsed as Record<string, unknown>
  if (fields["jsonrpc"] !== "2.0") {
    throw new ProtocolError(DesktopErrorCode.InvalidRequest, 'the request must carry "jsonrpc":"2.0"')
  }
  if (typeof fields["method"] !== "string" || fields["method"] === "") {
    throw new ProtocolError(DesktopErrorCode.InvalidRequest, "the request must carry a method name")
  }
  const identifier = fields["id"]
  if (typeof identifier !== "number" && typeof identifier !== "string") {
    throw new ProtocolError(DesktopErrorCode.InvalidRequest, "the request must carry an id to answer")
  }
  return { id: identifier, method: fields["method"], params: readParams(fields["params"], identifier) }
}

/**
 * readParams accepts an object or nothing, and refuses everything else. A
 * request whose parameters are the wrong shape is still a request, so the
 * refusal carries its id and the code that sends the message back to the model
 * rather than the one that restarts the worker.
 */
function readParams(params: unknown, identifier: RequestID): Record<string, unknown> {
  if (params === undefined || params === null) {
    return {}
  }
  if (typeof params !== "object" || Array.isArray(params)) {
    throw new ProtocolError(
      DesktopErrorCode.BadParameters,
      "the parameters must be a JSON object, and this request sent something else",
      undefined,
      identifier,
    )
  }
  return params as Record<string, unknown>
}

/** successResponse is the one line that answers a request that worked. */
export function successResponse(id: RequestID, result: unknown): string {
  return JSON.stringify({ jsonrpc: "2.0", id, result })
}

/** failureResponse is the one line that answers a request that did not work. */
export function failureResponse(id: RequestID, failure: ProtocolError): string {
  const body: Record<string, unknown> = { code: failure.code, message: failure.message }
  if (failure.data !== undefined) {
    body["data"] = failure.data
  }
  return JSON.stringify({ jsonrpc: "2.0", id, error: body })
}

/**
 * LineReader turns the chunks that arrive on standard input into whole lines. A
 * line longer than the cap is counted and thrown away rather than kept, because
 * a line that long is a mistake at the other end and holding it would be this
 * process's own undoing.
 */
export class LineReader {
  private held = ""
  private dropping = false
  /** How many lines were thrown away for being longer than the cap. */
  overLongLines = 0

  /** push takes one chunk and returns the whole lines it completed. */
  push(chunk: string): string[] {
    const lines: string[] = []
    let rest = chunk
    for (;;) {
      const breakAt = rest.indexOf("\n")
      if (breakAt < 0) {
        this.holdOrDrop(rest)
        return lines
      }
      const line = this.held + rest.slice(0, breakAt)
      this.held = ""
      const wasDropping = this.dropping
      this.dropping = false
      rest = rest.slice(breakAt + 1)
      const trimmed = line.replace(/\r$/, "").trim()
      if (!wasDropping && trimmed !== "") {
        lines.push(trimmed)
      }
    }
  }

  /** holdOrDrop keeps a partial line, unless it has grown past the cap. */
  private holdOrDrop(part: string): void {
    if (this.dropping) {
      return
    }
    this.held += part
    if (this.held.length <= maximumLineBytes) {
      return
    }
    this.held = ""
    this.dropping = true
    this.overLongLines += 1
  }
}
