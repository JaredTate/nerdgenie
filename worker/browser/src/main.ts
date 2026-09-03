/**
 * The process: read one request per line from standard input, write one response
 * per line to standard output, and put everything else on standard error.
 *
 * Run it as
 *   node worker/browser/dist/main.js --profile <folder> [--chrome <path>] [--pacing human|fast] [--headless]
 *
 * `--pacing fast` exists only for the tests, which cannot wait for human pacing.
 * `--headless` exists only for the tests as well, so that a test run puts no
 * Chrome window on the screen of whoever is running it. In a real run the window
 * is always visible, because a logged-in account is only safe in a window the
 * user can see.
 */
import { ERROR_CODES } from "./errors.js";
import { createLineReader } from "./lines.js";
import { MAX_LINE_BYTES } from "./limits.js";
import { toStandardError } from "./log.js";
import type { Pacing } from "./pacing.js";
import { startWorker, type BrowserWorker } from "./worker.js";
import { errorResponse, formatEvent, formatResponse, parseLine } from "./wire.js";

/** What the command line asked for. */
interface Settings {
  profile: string;
  chromePath: string | undefined;
  pacing: Pacing;
  headless: boolean;
}

/** The exit code for a command line the worker could not make sense of. */
const BAD_COMMAND_LINE = 2;

/** The exit code for a browser that would not start. */
const BROWSER_WOULD_NOT_START = 1;

const HOW_TO_RUN =
  "Run it as: node worker/browser/dist/main.js --profile <folder> [--chrome <path>] [--pacing human|fast] [--headless]";

/** Read the command line, or say exactly what was wrong with it. */
export function readSettings(argv: readonly string[]): Settings | string {
  let profile = "";
  let chromePath: string | undefined;
  let pacing: Pacing = "human";
  let headless = false;
  for (let at = 0; at < argv.length; at += 1) {
    const name = argv[at];
    const value = argv[at + 1];
    if (name === "--headless") {
      headless = true;
      continue;
    }
    if (name === "--profile" || name === "--chrome" || name === "--pacing") {
      if (value === undefined) {
        return `The ${name} option needs a value after it. ${HOW_TO_RUN}`;
      }
      at += 1;
      if (name === "--profile") {
        profile = value;
      } else if (name === "--chrome") {
        chromePath = value;
      } else if (value === "human" || value === "fast") {
        pacing = value;
      } else {
        return `The --pacing option takes "human" or "fast", and this was "${value}". ${HOW_TO_RUN}`;
      }
      continue;
    }
    return `There is no option called "${String(name)}". ${HOW_TO_RUN}`;
  }
  if (profile === "") {
    return `The --profile option is required, and it must never be the user's daily Chrome profile. ${HOW_TO_RUN}`;
  }
  return { profile, chromePath, pacing, headless };
}

/** Write one response, and nothing else, to standard output. */
function answer(response: Parameters<typeof formatResponse>[0]): void {
  process.stdout.write(formatResponse(response));
}

/** Read lines and answer them, one at a time, in the order they arrived. */
function serve(worker: BrowserWorker): void {
  let inLine: Promise<unknown> = Promise.resolve();
  const take = createLineReader({
    mostBytes: MAX_LINE_BYTES,
    onLine: (line) => {
      if (line.trim() === "") {
        return;
      }
      const parsed = parseLine(line);
      if (parsed.kind === "response") {
        answer(parsed.response);
        return;
      }
      inLine = inLine.then(() => worker.handle(parsed.request).then(answer));
      inLine.catch(() => {});
    },
    onTooLong: () => {
      answer(
        errorResponse(
          null,
          ERROR_CODES.notJson,
          `The line was longer than ${MAX_LINE_BYTES} bytes. Send one JSON-RPC request object per line.`,
        ),
      );
    },
  });
  process.stdin.on("data", (chunk: Buffer) => take(chunk));
}

/** Start the worker, serve standard input, and stop cleanly when it closes. */
async function run(): Promise<void> {
  const settings = readSettings(process.argv.slice(2));
  if (typeof settings === "string") {
    toStandardError(`browser worker: ${settings}`);
    process.exit(BAD_COMMAND_LINE);
  }
  let worker: BrowserWorker;
  try {
    worker = await startWorker({
      ...settings,
      log: toStandardError,
      onEvent: (event) => process.stdout.write(formatEvent(event)),
    });
  } catch (problem) {
    const why = problem instanceof Error ? problem.message : String(problem);
    toStandardError(`browser worker: the browser would not start: ${why}`);
    process.exit(BROWSER_WOULD_NOT_START);
  }

  let stopping = false;
  const stop = (): void => {
    if (stopping) {
      return;
    }
    stopping = true;
    worker.stop().then(
      () => process.exit(0),
      () => process.exit(BROWSER_WOULD_NOT_START),
    );
  };
  process.stdin.on("end", stop);
  process.stdin.on("close", stop);
  process.on("SIGINT", stop);
  process.on("SIGTERM", stop);
  serve(worker);
}

await run();
