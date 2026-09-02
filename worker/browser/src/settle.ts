/**
 * Waiting for the page to settle.
 *
 * PROTOCOL.md says settled means either that a move to a new address has finished
 * or that nothing on the page has changed for three hundred milliseconds, with a
 * limit of three seconds. Changes to attributes alone do not count.
 *
 * Past the limit the page is read as it stands and the diff says `settled: false`,
 * because a chat, a clock, and a live feed are ordinary pages and refusing to work
 * on them would rule out much of the web. The error -32001 is kept for the page
 * that cannot be read at all, which is a different thing.
 */
import type { Page } from "playwright-core";
import { SETTLE_LIMIT_MS, SETTLE_POLL_MS, SETTLE_QUIET_MS } from "./limits.js";
import { quietFor } from "./page-bridge.js";
import { wait } from "./pacing.js";
import type { Session } from "./session.js";

/**
 * Wait until the page is quiet, and say whether it got there. A page holding an
 * open dialog is settled by definition: nothing more can happen on it until
 * somebody answers the dialog, and asking it anything would only hang.
 */
export async function settle(session: Session, page: Page): Promise<boolean> {
  const startedAt = Date.now();
  const giveUpAt = startedAt + SETTLE_LIMIT_MS;
  for (;;) {
    if (session.dialogOn(page) !== null) {
      return true;
    }
    try {
      // The quiet stretch is measured from the action, not from whatever the page
      // last did on its own. A key press mutates nothing, so a page that had been
      // still for a minute would otherwise count as settled before the browser had
      // even begun to act on the press.
      const sinceTheAction = Date.now() - startedAt;
      const quiet = Math.min(await quietFor(page), sinceTheAction);
      if (quiet >= SETTLE_QUIET_MS) {
        // A page can be perfectly still and still be on its way somewhere else:
        // the browser has taken the request but not yet thrown the document
        // away. Waiting for the new document here is what stops the snapshot
        // that follows from being taken out from under itself.
        await page
          .waitForLoadState("domcontentloaded", { timeout: Math.max(0, giveUpAt - Date.now()) })
          .catch(() => {});
        return true;
      }
    } catch {
      // The page is between documents, which means a move to a new address is
      // under way. Wait for the new document and then start looking again.
      const left = giveUpAt - Date.now();
      if (left > 0) {
        await page.waitForLoadState("domcontentloaded", { timeout: left }).catch(() => {});
      }
    }
    if (Date.now() >= giveUpAt) {
      session.log(
        `the page was still changing after ${SETTLE_LIMIT_MS} milliseconds, so it was read as it stood.`,
      );
      return false;
    }
    await wait(SETTLE_POLL_MS);
  }
}
