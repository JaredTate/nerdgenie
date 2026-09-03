/**
 * The worker: one request in, one response out, with a deadline on every method.
 *
 * The process shell in main.ts reads lines and writes lines. Everything that
 * decides what a request means lives here, which is what lets a test drive the
 * whole worker in one process and still exercise the same code the Go side does.
 */
import { asWorkerError, chromeDied } from "./errors.js";
import { launchChrome, type RunningChrome } from "./chrome.js";
import { METHOD_DEADLINE_MS } from "./limits.js";
import { prefixed, toNowhere, type Logger } from "./log.js";
import { runMethod } from "./methods.js";
import { Session } from "./session.js";
import type { Chance, Pacing } from "./pacing.js";
import type { JsonRpcResponse, WorkerRequest } from "./types.js";
import { responseForError, successResponse } from "./wire.js";

/** How to start the worker. */
export interface WorkerOptions {
  /** The Chrome profile folder. Never the user's daily profile. */
  profile: string;
  /** The Chrome binary, or nothing to take google-chrome from the PATH. */
  chromePath?: string | undefined;
  /** How fast to act. `fast` is only for the tests. */
  pacing: Pacing;
  /** Run Chrome with no window at all. Only the tests ask for this. */
  headless?: boolean | undefined;
  /** Where chance comes from, so a test can hand in a fixed sequence. */
  chance?: Chance | undefined;
  /** Where the worker's own logging goes. */
  log?: Logger | undefined;
}

/** A running worker. */
export interface BrowserWorker {
  handle(request: WorkerRequest): Promise<JsonRpcResponse>;
  stop(): Promise<void>;
}

/** Run something, but give up after the deadline for its method. */
async function withDeadline<T>(method: string, work: Promise<T>): Promise<T> {
  const limit = METHOD_DEADLINE_MS[method] ?? 30_000;
  let timer: NodeJS.Timeout | undefined;
  const alarm = new Promise<never>((_, ringing) => {
    timer = setTimeout(
      () =>
        ringing(
          chromeDied(
            `the ${method} method was still running after ${limit} milliseconds, so the browser can no longer be trusted.`,
          ),
        ),
      limit,
    );
    timer.unref();
  });
  try {
    return await Promise.race([work, alarm]);
  } finally {
    clearTimeout(timer);
  }
}

/** Launch Chrome, attach to it, and hand back a worker that answers requests. */
export async function startWorker(options: WorkerOptions): Promise<BrowserWorker> {
  const log = prefixed(options.log ?? toNowhere);
  const chrome: RunningChrome = await launchChrome({
    profile: options.profile,
    chromePath: options.chromePath,
    headless: options.headless ?? false,
    log,
  });
  const session = new Session({
    chrome,
    pacing: options.pacing,
    chance: options.chance ?? Math.random,
    log,
  });
  await session.watchForNewTabs();

  // The Go side sends one request at a time, but a chain of promises makes that
  // true here as well, so two requests can never share a page halfway through.
  let inLine: Promise<unknown> = Promise.resolve();

  async function answer(request: WorkerRequest): Promise<JsonRpcResponse> {
    if (!chrome.isAlive()) {
      return responseForError(request.id, chromeDied("the browser window is gone."));
    }
    try {
      const result = await withDeadline(request.method, runMethod(session, request));
      return successResponse(request.id, result);
    } catch (problem) {
      const failure = asWorkerError(problem);
      log(`the ${request.method} method failed with ${failure.code}: ${failure.message}`);
      return responseForError(request.id, failure);
    }
  }

  return {
    handle(request) {
      const mine = inLine.then(() => answer(request));
      inLine = mine.catch(() => {});
      return mine;
    },
    async stop() {
      await inLine.catch(() => {});
      await chrome.stop();
      log("stopped.");
    },
  };
}
