/**
 * Starting a real browser worker for a test, and talking to it the way the Go
 * side does.
 *
 * Every request goes through the same line parser the process uses, so a test
 * exercises the wire and the method together, and a test can never send a request
 * the real Go side could not send.
 */
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "playwright-core";
import { formatResponse, parseLine } from "../src/wire.js";
import { startWorker, type BrowserWorker } from "../src/worker.js";
import type { JsonRpcResponse, PersonEvent } from "../src/types.js";

/** A worker under test, with a throwaway profile folder that is removed at the end. */
export interface TestWorker {
  profile: string;
  logLines: string[];
  /** Every event the worker reported about what the person did in the window. */
  events: PersonEvent[];
  /** The page the worker is on, which a test acts on the way a person would. */
  personsPage(): Page;
  send(method: string, params?: Record<string, unknown>): Promise<JsonRpcResponse>;
  /** Send a request and give back the result, failing the test on an error response. */
  result(method: string, params?: Record<string, unknown>): Promise<Record<string, unknown>>;
  /** Send a request and give back the error, failing the test on a result response. */
  fails(
    method: string,
    params?: Record<string, unknown>,
  ): Promise<{ code: number; message: string; data?: Record<string, unknown> }>;
  stop(): Promise<void>;
}

let nextRequestId = 1;

/**
 * Take the profile folder away. Chrome flushes its last files as it goes, so a
 * removal that runs at the same moment can find the folder filling up again. Try
 * a few times before giving up.
 */
async function removeWhenChromeHasLetGo(folder: string): Promise<void> {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    try {
      await rm(folder, { recursive: true, force: true });
      return;
    } catch {
      await new Promise((next) => setTimeout(next, 50));
    }
  }
}

/**
 * Start a worker with a throwaway profile folder and no waiting between actions.
 *
 * COEUS_HEADLESS_TESTS asks for a Chrome with no window, which is the same name
 * `make test-browser` sets, so that a test run on a machine somebody is using
 * puts no window on their screen.
 */
export async function startTestWorker(): Promise<TestWorker> {
  const profile = await mkdtemp(join(tmpdir(), "coeus-browser-test-"));
  const logLines: string[] = [];
  const events: PersonEvent[] = [];
  let worker: BrowserWorker;
  try {
    worker = await startWorker({
      profile,
      pacing: "fast",
      headless: process.env["COEUS_HEADLESS_TESTS"] !== undefined,
      log: (line) => logLines.push(line),
      onEvent: (event) => events.push(event),
    });
  } catch (problem) {
    await rm(profile, { recursive: true, force: true });
    throw problem;
  }

  async function send(
    method: string,
    params: Record<string, unknown> = {},
  ): Promise<JsonRpcResponse> {
    nextRequestId += 1;
    const line = JSON.stringify({ jsonrpc: "2.0", id: nextRequestId, method, params });
    const parsed = parseLine(line);
    if (parsed.kind === "response") {
      return parsed.response;
    }
    const response = await worker.handle(parsed.request);
    // Going through the formatter proves the answer survives one line of text.
    return JSON.parse(formatResponse(response)) as JsonRpcResponse;
  }

  return {
    profile,
    logLines,
    events,
    personsPage: () => worker.session.currentPage(),
    send,
    async result(method, params) {
      const response = await send(method, params);
      if ("error" in response) {
        throw new Error(
          `${method} answered with error ${response.error.code}: ${response.error.message}`,
        );
      }
      return response.result;
    },
    async fails(method, params) {
      const response = await send(method, params);
      if (!("error" in response)) {
        throw new Error(`${method} was supposed to fail but answered ${JSON.stringify(response)}`);
      }
      return response.error;
    },
    async stop() {
      await worker.stop();
      await removeWhenChromeHasLetGo(profile);
    },
  };
}
