/**
 * Asking a page a question.
 *
 * A model building a web page needs to read the page's own state: the game's
 * phase, the score, whether a piece is on the board. The outline of elements
 * cannot say, and on the fifth game build the model wrote six play-test drivers
 * through the shell to read window.__engine.state, because no browser tool
 * would answer. The read method takes one expression and answers with its
 * value as JSON.
 *
 * It answers only on a page served from this machine or from a file, because
 * the browser holds the person's logins and the design says a script is never
 * run on anyone else's page. A page on localhost or 127.0.0.1 is the model's
 * own work, and the person's, to inspect.
 */
import type { Page } from "playwright-core";
import { MAX_ANSWER_CHARS, PAGE_CALL_DEADLINE_MS } from "./limits.js";

/** The host names that mean this machine. */
const THIS_MACHINE: ReadonlySet<string> = new Set(["localhost", "127.0.0.1", "[::1]", "::1"]);

/** Is this address a page on this machine, or a file, which the model may ask? */
export function isAPageOnThisMachine(address: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(address);
  } catch {
    return false;
  }
  if (parsed.protocol === "file:") {
    return true;
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return false;
  }
  return THIS_MACHINE.has(parsed.hostname);
}

/** Cut an answer to the cap, saying how much was cut. */
function cutAnswer(text: string): string {
  if (text.length <= MAX_ANSWER_CHARS) {
    return text;
  }
  return `${text.slice(0, MAX_ANSWER_CHARS)} ... ${text.length - MAX_ANSWER_CHARS} more characters were cut`;
}

/**
 * Evaluate one expression on the page and give its value back as JSON, or the
 * error the page threw, in words. A page whose script never yields cannot
 * answer, and that is reported the way the scan reports it, by the deadline.
 */
export async function askThePage(page: Page, ask: string): Promise<string> {
  let timer: NodeJS.Timeout | undefined;
  const alarm = new Promise<string>((resolve) => {
    timer = setTimeout(
      () => resolve(`the page did not answer within ${PAGE_CALL_DEADLINE_MS} milliseconds: its own script is keeping it busy`),
      PAGE_CALL_DEADLINE_MS,
    );
    timer.unref();
  });
  const asking = page
    .evaluate((expression: string) => {
      const value = (0, eval)(expression);
      const written = JSON.stringify(value);
      return written === undefined ? String(value) : written;
    }, ask)
    .then((value) => cutAnswer(String(value)))
    .catch((problem: unknown) => {
      const message = problem instanceof Error ? problem.message : String(problem);
      return `the page threw: ${message.split("\n")[0]}`;
    });
  try {
    return await Promise.race([asking, alarm]);
  } finally {
    clearTimeout(timer);
  }
}
