/**
 * What went wrong on a page, as a person with the console open would see it.
 *
 * On the live game build the page loaded its script from a path where there was
 * no script. The game never started, and the model, which reads a page as an
 * outline of its elements, was never told: it clicked Start, saw the menu still
 * there, and went round in circles until the task stopped. A person opens the
 * console and sees "main.js 404" at once. This is that console, kept short:
 * uncaught script errors, console errors, and the loads of the page itself, its
 * scripts and its stylesheets that failed or came back with an error status.
 * Other loads are left out, because an image or a tracker that is missing is
 * ordinary on the web and breaks nothing the model is working on.
 *
 * The book is cleared when the page moves to another address, the way the
 * console is, and it keeps the first few errors and counts the rest, because the
 * first error on a broken page is usually the cause and the rest follow from it.
 */
import type { Page, Request, Response } from "playwright-core";
import { MAX_PAGE_ERROR_CHARS, MAX_PAGE_ERRORS } from "./limits.js";

/** The kinds of load whose failure breaks the page rather than decorating it. */
const LOADS_WORTH_NOTING: ReadonlySet<string> = new Set(["document", "script", "stylesheet"]);

/**
 * The console line Chrome prints for every failed load. It is left out because
 * the response listener already says which load failed, with its status.
 */
const CHROME_LOAD_FAILURE_LINE = "Failed to load resource";

/** What one page has gone wrong with so far: the first few lines, and how many more there were. */
interface ErrorsOnOnePage {
  lines: string[];
  more: number;
}

/** Cut one line to the cap, on one line. */
function oneLine(text: string): string {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length <= MAX_PAGE_ERROR_CHARS
    ? flat
    : `${flat.slice(0, MAX_PAGE_ERROR_CHARS)}...`;
}

/** The errors of every page the worker watches. */
export class PageErrorBook {
  private readonly pages = new Map<Page, ErrorsOnOnePage>();

  /** Start listening to one page. */
  watch(page: Page): void {
    page.on("pageerror", (problem) => this.note(page, `script error: ${problem.message}`));
    page.on("console", (message) => {
      if (message.type() === "error" && !message.text().startsWith(CHROME_LOAD_FAILURE_LINE)) {
        this.note(page, `console error: ${message.text()}`);
      }
    });
    page.on("requestfailed", (request: Request) => {
      if (LOADS_WORTH_NOTING.has(request.resourceType())) {
        const why = request.failure()?.errorText ?? "no reason given";
        this.note(page, `${request.resourceType()} ${request.url()} failed to load: ${why}`);
      }
    });
    page.on("response", (response: Response) => {
      const request = response.request();
      if (response.status() >= 400 && LOADS_WORTH_NOTING.has(request.resourceType())) {
        this.note(page, `${request.resourceType()} ${request.url()} answered ${response.status()}`);
      }
    });
    // A move to another address starts a fresh console. It is noticed when the
    // request for the new document goes out, before its answer, so that the new
    // document's own failure is the first line of the new page and not the last
    // line of the old one.
    page.on("request", (request: Request) => {
      if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
        this.pages.delete(page);
      }
    });
    page.on("close", () => this.pages.delete(page));
  }

  /** Write one line down for a page, or count it once the first few are kept. */
  private note(page: Page, line: string): void {
    const held = this.pages.get(page) ?? { lines: [], more: 0 };
    if (held.lines.length < MAX_PAGE_ERRORS) {
      held.lines.push(oneLine(line));
    } else {
      held.more += 1;
    }
    this.pages.set(page, held);
  }

  /** What has gone wrong on this page since it last moved: the first few lines, then a count of the rest. */
  on(page: Page): string[] {
    const held = this.pages.get(page);
    if (held === undefined) {
      return [];
    }
    const lines = [...held.lines];
    if (held.more > 0) {
      lines.push(`... and ${held.more} more errors`);
    }
    return lines;
  }
}
