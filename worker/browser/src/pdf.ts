/**
 * A page that is a PDF rather than a web page.
 *
 * The design says PDF pages are read as text through Chrome's viewer where
 * possible and otherwise reported as a download. On Chrome 151 it is not
 * possible: the viewer draws the document into a plugin and only builds an
 * accessibility tree for it when a screen reader is running, so neither the
 * page's own markup nor `Accessibility.getFullAXTree` holds a word of the text.
 * That was measured on this machine, not assumed.
 *
 * So the worker takes the other path. It fetches the file through the browser's
 * own session, which means the browser's cookies, so a PDF behind a login comes
 * down just as it would for the user, saves it under the profile folder, and
 * reports it as a download. The model can then read the file like any other file.
 */
import { writeFile } from "node:fs/promises";
import { basename, join } from "node:path";
import type { Page } from "playwright-core";
import { MAX_PDF_BYTES } from "./limits.js";
import type { Session } from "./session.js";
import type { DownloadReport } from "./types.js";

/** What a page says it is when it is a PDF. */
export const PDF_CONTENT_TYPE = "application/pdf";

/** How long to wait for the file itself. */
const FETCH_LIMIT_MS = 30_000;

/**
 * A safe name for the saved file: the last part of the address, with anything
 * that could climb out of the downloads folder taken off.
 */
export function fileNameFor(address: string): string {
  let last = "";
  try {
    last = basename(new URL(address).pathname);
  } catch {
    last = "";
  }
  const cleaned = last.replace(/[^A-Za-z0-9._-]/g, "_").replace(/^\.+/, "");
  return cleaned === "" ? "document.pdf" : cleaned;
}

/**
 * Fetch the PDF the page is showing and save it under the profile folder. Gives
 * back nothing when it could not be fetched or is bigger than the cap, in which
 * case the snapshot still says the page is a PDF.
 */
export async function savePdfShownOnPage(
  session: Session,
  page: Page,
): Promise<DownloadReport | null> {
  const address = page.url();
  const filename = fileNameFor(address);
  const path = join(session.chrome.downloadsFolder, filename);
  try {
    const answer = await page.context().request.get(address, { timeout: FETCH_LIMIT_MS });
    if (!answer.ok()) {
      session.log(`the PDF at ${address} answered ${answer.status()} and was not saved.`);
      return null;
    }
    const body = await answer.body();
    if (body.length > MAX_PDF_BYTES) {
      session.log(
        `the PDF at ${address} is ${body.length} bytes, past the ${MAX_PDF_BYTES} byte cap, and was not saved.`,
      );
      return null;
    }
    await writeFile(path, body);
    session.log(`saved the PDF ${filename} under the profile folder so it can be read as a file.`);
    return { filename, path };
  } catch (problem) {
    const why = problem instanceof Error ? problem.message : String(problem);
    session.log(`the PDF at ${address} could not be saved: ${why}`);
    return null;
  }
}

/** What the model is told it is looking at when the page is a PDF. */
export function pdfElementName(saved: DownloadReport | null, address: string): string {
  return saved === null
    ? `PDF document at ${address}, which could not be saved`
    : `PDF document: ${saved.filename}, saved to ${saved.path}`;
}
