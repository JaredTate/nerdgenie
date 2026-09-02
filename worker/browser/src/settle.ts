/**
 * Waiting for the page to settle.
 *
 * PROTOCOL.md says settled means either that a move to a new address has finished
 * or that nothing on the page has changed for three hundred milliseconds, with a
 * limit of three seconds. Past the limit the answer is -32001, because a page that
 * is still rearranging itself cannot be compared with the page before the action.
 */
import type { Page } from "playwright-core";
import { didNotSettle } from "./errors.js";
import { SETTLE_LIMIT_MS, SETTLE_POLL_MS, SETTLE_QUIET_MS } from "./limits.js";
import { quietFor } from "./page-bridge.js";
import { wait } from "./pacing.js";
import type { Session } from "./session.js";

/**
 * Wait until the page is quiet. A page holding an open dialog is settled by
 * definition: nothing more can happen on it until somebody answers the dialog, and
 * asking it anything would only hang.
 */
export async function settle(session: Session, page: Page): Promise<void> {
  const giveUpAt = Date.now() + SETTLE_LIMIT_MS;
  for (;;) {
    if (session.dialogOn(page) !== null) {
      return;
    }
    try {
      if ((await quietFor(page)) >= SETTLE_QUIET_MS) {
        return;
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
      throw didNotSettle(SETTLE_LIMIT_MS);
    }
    await wait(SETTLE_POLL_MS);
  }
}
