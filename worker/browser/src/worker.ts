/**
 * The worker: one request in, one response out, with a deadline on every method.
 *
 * The process shell in main.ts reads lines and writes lines. Everything that
 * decides what a request means lives here, which is what lets a test drive the
 * whole worker in one process and still exercise the same code the Go side does.
 */
import { askThePage } from "./ask.js";
import { asWorkerError, chromeDied } from "./errors.js";
import { watchWhatThePersonDoes, type ReportEvent } from "./events.js";
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
  /**
   * Where an event saying what the person did in the window goes. The process
   * shell writes it to standard output as a notification; a test collects it.
   */
  onEvent?: ReportEvent | undefined;
  /** A deadline for a method, over the built-in table. Only the tests ask for this. */
  deadlines?: Partial<Record<string, number>> | undefined;
}

/** A running worker. */
export interface BrowserWorker {
  handle(request: WorkerRequest): Promise<JsonRpcResponse>;
  stop(): Promise<void>;
  /**
   * The session it is driving. A test acts on this page the way a person would,
   * which is the only way to prove that what a person does is reported; the
   * process shell never touches it.
   */
  session: Session;
}

/** What the model is told when a method ran out of time and the browser is started again. */
function hungMessage(method: string, afterMs: number): string {
  return (
    `the ${method} method was still running after ${afterMs} milliseconds, so the browser is started again. ` +
    "If this is a page you are building, the page's own script is the likely cause: code that does not yield, " +
    "such as an endless loop that starts on this action, keeps the page from answering anything. Look in the code this action runs, " +
    "including what it draws, for a while or a for whose condition never changes, and fix the script before opening the page again."
  );
}

/** Wait for the work, or for the alarm, whichever comes first. */
async function untilTheAlarm<T>(
  work: Promise<T>,
  limit: number,
): Promise<{ finished: true; value: T } | { finished: false }> {
  let timer: NodeJS.Timeout | undefined;
  const alarm = new Promise<{ finished: false }>((resolve) => {
    timer = setTimeout(() => resolve({ finished: false }), limit);
    timer.unref();
  });
  try {
    return await Promise.race([work.then((value) => ({ finished: true as const, value })), alarm]);
  } finally {
    clearTimeout(timer);
  }
}

/**
 * Run something, but give up after the deadline for its method.
 *
 * A page that still answers when the deadline rings is alive, only slow: a game
 * drawing sixty frames a second makes every look at the page slow, and a click
 * on it with human pacing, two looks and a second click ran past twenty-five
 * seconds on the fresh Tetris build. Such a page gets the same time again. A
 * page that cannot answer is hung, and a new browser is the only way out.
 */
async function withDeadline<T>(
  method: string,
  limit: number,
  work: Promise<T>,
  pageStillAnswers: () => Promise<boolean>,
  log: Logger,
): Promise<T> {
  const first = await untilTheAlarm(work, limit);
  if (first.finished) {
    return first.value;
  }
  if (!(await pageStillAnswers())) {
    throw chromeDied(hungMessage(method, limit));
  }
  log(`the ${method} method has run for ${limit} milliseconds, but the page still answers, so the worker waits as long again.`);
  const second = await untilTheAlarm(work, limit);
  if (second.finished) {
    return second.value;
  }
  throw chromeDied(hungMessage(method, 2 * limit));
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
  if (options.onEvent !== undefined) {
    watchWhatThePersonDoes(session, options.onEvent);
  }

  // The Go side sends one request at a time, but a chain of promises makes that
  // true here as well, so two requests can never share a page halfway through.
  let inLine: Promise<unknown> = Promise.resolve();

  async function pageStillAnswers(): Promise<boolean> {
    const page = session.currentPageOrNone();
    return page !== undefined && (await askThePage(page, "1")) === "1";
  }

  async function answer(request: WorkerRequest): Promise<JsonRpcResponse> {
    if (!chrome.isAlive()) {
      return responseForError(request.id, chromeDied("the browser window is gone."));
    }
    const limit = options.deadlines?.[request.method] ?? METHOD_DEADLINE_MS[request.method] ?? 30_000;
    // Everything that happens on the page from here until the answer is the
    // worker's own doing, and none of it is reported as something the person did.
    session.startedWorking();
    try {
      const result = await withDeadline(request.method, limit, runMethod(session, request), pageStillAnswers, log);
      return successResponse(request.id, result);
    } catch (problem) {
      const failure = asWorkerError(problem);
      log(`the ${request.method} method failed with ${failure.code}: ${failure.message}`);
      return responseForError(request.id, failure);
    } finally {
      session.finishedWorking();
    }
  }

  return {
    session,
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
