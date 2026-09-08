/**
 * A page on this machine is loaded fresh. It is the page the agent is building,
 * and run 23 lost seven minutes to a server that kept handing the browser an
 * old render.js from the cache after every open. Anyone else's page is left to
 * the cache the way a person's browser leaves it.
 *
 * The cache is switched off through the Chrome DevTools Protocol on the page,
 * `Network.setCacheDisabled`, which is the one switch that covers every request
 * the page makes, scripts and styles included; a route that rewrote headers
 * would have to guess which responses to touch. The switch is set before each
 * move to a new address, on for a page on this machine and off again for any
 * other, so a tab that goes from the agent's page to a public site is not
 * left slower than it was.
 */
import type { CDPSession, Page } from "playwright-core";
import { isAPageOnThisMachine } from "./ask.js";

/** Whether an address is loaded fresh: a page on this machine, or a file. */
export function loadsFresh(address: string): boolean {
  return isAPageOnThisMachine(address);
}

/** The page's DevTools session, made once, and whether its cache is off now. */
interface Freshness {
  session: CDPSession;
  off: boolean;
}

const freshness = new WeakMap<Page, Freshness>();

/**
 * Set the page's cache for the address it is about to go to: off for a page on
 * this machine, on again for any other. A page that has never been to this
 * machine is not touched at all.
 */
export async function keepFresh(page: Page, address: string): Promise<void> {
  const wanted = loadsFresh(address);
  let held = freshness.get(page);
  if (held === undefined) {
    if (!wanted) {
      return;
    }
    const session = await page.context().newCDPSession(page);
    // The cache switch is a Network command, and the domain answers it only
    // once it is on.
    await session.send("Network.enable");
    held = { session, off: false };
    freshness.set(page, held);
  }
  if (held.off === wanted) {
    return;
  }
  await held.session.send("Network.setCacheDisabled", { cacheDisabled: wanted });
  held.off = wanted;
}
