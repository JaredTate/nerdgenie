// The worker process: it reads one JSON-RPC request per line from standard
// input, answers it on standard output, and logs a line per event to standard
// error. One request runs at a time, because a desktop has one mouse.

import { CuaDesktopDriver } from "./cuadriver.js"
import { dispatch } from "./methods.js"
import { pacingNamed, type Pacing } from "./pacing.js"
import { DesktopSession } from "./session.js"
import {
  DesktopErrorCode,
  LineReader,
  ProtocolError,
  failureResponse,
  readRequest,
  successResponse,
  type RequestID,
} from "./wire.js"

/** How the worker is started and what it talks to. */
export interface WorkerOptions {
  /** The command-line arguments, without the program name. */
  argv: string[]
  /** Where requests arrive. */
  input: NodeJS.ReadableStream
  /** Where responses go, one line each and nothing else. */
  output: NodeJS.WritableStream
  /** Where the worker's own log lines go. */
  errors: NodeJS.WritableStream
  /** How the session is built, so that a test can hand in a driver of its own. */
  openSession?: (pacing: Pacing, note: (line: string) => void) => DesktopSession
}

/** pacingFromArguments reads the one flag the worker takes. */
export function pacingFromArguments(argv: readonly string[]): Pacing {
  let chosen = "human"
  for (let at = 0; at < argv.length; at += 1) {
    const argument = argv[at]
    if (argument !== "--pacing") {
      throw new ProtocolError(
        DesktopErrorCode.BadParameters,
        `the argument ${JSON.stringify(argument ?? "")} is not one this worker takes, so pass only --pacing human or --pacing fast`,
      )
    }
    const value = argv[at + 1]
    if (value === undefined) {
      throw new ProtocolError(DesktopErrorCode.BadParameters, "the --pacing flag needs a value after it, human or fast")
    }
    chosen = value
    at += 1
  }
  return pacingNamed(chosen)
}

/** runWorker reads standard input until it ends, answering one request at a time. */
export async function runWorker(options: WorkerOptions): Promise<void> {
  const note = (line: string): void => {
    options.errors.write(`${line}\n`)
  }
  const pacing = pacingFromArguments(options.argv)
  const session = (options.openSession ?? openRealSession)(pacing, note)
  note(`the desktop worker is ready at ${pacing.name} pacing`)

  const reader = new LineReader()
  for await (const chunk of options.input) {
    for (const line of reader.push(String(chunk))) {
      options.output.write(`${await answer(session, line, note)}\n`)
    }
  }
  if (reader.overLongLines > 0) {
    note(`${reader.overLongLines} line or lines were longer than the cap and were thrown away`)
  }
  note("standard input ended, so the desktop worker is stopping")
  await session.close()
}

/** answer turns one line into the one response line it deserves. */
async function answer(session: DesktopSession, line: string, note: (line: string) => void): Promise<string> {
  let identifier: RequestID = null
  try {
    const request = readRequest(line)
    identifier = request.id
    note(`the desktop worker was asked to ${request.method}`)
    return successResponse(identifier, await dispatch(session, request.method, request.params))
  } catch (failure) {
    const reported = failure instanceof ProtocolError ? failure : unexpected(failure)
    note(`the desktop worker could not do it: ${reported.message}`)
    return failureResponse(identifier, reported)
  }
}

/** unexpected wraps anything that was not already one of the protocol's failures. */
function unexpected(failure: unknown): ProtocolError {
  const said = failure instanceof Error ? failure.message : String(failure)
  return new ProtocolError(DesktopErrorCode.DriverUnavailable, `the desktop worker hit something it did not expect: ${said}`)
}

/** openRealSession builds the session that drives this machine's own screen. */
function openRealSession(pacing: Pacing, note: (line: string) => void): DesktopSession {
  return new DesktopSession(new CuaDesktopDriver(note), pacing, note)
}
