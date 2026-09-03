// The nine methods worker/desktop/PROTOCOL.md names, and the reading of their
// parameters. Every refusal here names the parameter that was wrong, because a
// message that does not say what to fix is a message the reader has to guess at.

import type { DesktopSession } from "./session.js"
import { DesktopErrorCode, ProtocolError } from "./wire.js"

/** The nine methods this worker answers. */
export const methodNames = [
  "launch",
  "screenshot",
  "click",
  "type",
  "press",
  "drag",
  "clipboardGet",
  "clipboardSet",
  "health",
] as const

/** One of the nine method names. */
export type MethodName = (typeof methodNames)[number]

/** dispatch runs one method of the session and returns what the protocol promises. */
export async function dispatch(
  session: DesktopSession,
  method: string,
  params: Record<string, unknown>,
): Promise<unknown> {
  switch (method) {
    case "launch":
      return session.launch(text(params, "application"), expectationIn(params))
    case "screenshot":
      return session.screenshot()
    case "click":
      return session.click(wholeNumber(params, "mark"), expectationIn(params))
    case "type":
      return session.type(text(params, "text"), optionalWholeNumber(params, "mark"), expectationIn(params))
    case "press":
      return session.press(text(params, "keys"), expectationIn(params))
    case "drag":
      return session.drag(wholeNumber(params, "fromMark"), wholeNumber(params, "toMark"), expectationIn(params))
    case "clipboardGet":
      return session.clipboardGet()
    case "clipboardSet":
      return session.clipboardSet(text(params, "text"))
    case "health":
      return session.health()
    default:
      throw new ProtocolError(
        DesktopErrorCode.NoSuchMethod,
        `there is no method called ${JSON.stringify(method)}, so ask for one of ${methodNames.join(", ")}`,
      )
  }
}

/** text reads one parameter that has to be there and has to be written text. */
function text(params: Record<string, unknown>, name: string): string {
  const written = params[name]
  if (typeof written !== "string") {
    throw badParameter(`the ${name} parameter has to be text, and this request sent ${describe(written)}`)
  }
  return written
}

/** expectationIn reads the expectation, which may be left out entirely. */
function expectationIn(params: Record<string, unknown>): string {
  const written = params["expectation"]
  if (written === undefined || written === null) {
    return ""
  }
  if (typeof written !== "string") {
    throw badParameter(`the expectation has to be text, and this request sent ${describe(written)}`)
  }
  return written
}

/** wholeNumber reads a control number that has to be there. */
function wholeNumber(params: Record<string, unknown>, name: string): number {
  const written = params[name]
  if (typeof written !== "number" || !Number.isInteger(written)) {
    throw badParameter(`the ${name} parameter has to be a whole number from the last screenshot, and this request sent ${describe(written)}`)
  }
  return written
}

/** optionalWholeNumber reads a control number that may be left out. */
function optionalWholeNumber(params: Record<string, unknown>, name: string): number | undefined {
  if (params[name] === undefined || params[name] === null) {
    return undefined
  }
  return wholeNumber(params, name)
}

/** badParameter is the one failure this file reports. */
function badParameter(message: string): ProtocolError {
  return new ProtocolError(DesktopErrorCode.BadParameters, message)
}

/** describe says in a word what the request sent instead. */
function describe(written: unknown): string {
  if (written === undefined) {
    return "nothing"
  }
  if (written === null) {
    return "null"
  }
  return Array.isArray(written) ? "a list" : typeof written
}
