/**
 * Listing, switching, and closing tabs.
 *
 * Every method here ends the same way, with the list of what is open, because the
 * model's next move always depends on what is in front of it.
 */
import type { Page } from "playwright-core";
import { wrongParameters } from "./errors.js";
import { PAGE_CALL_DEADLINE_MS } from "./limits.js";
import { wait } from "./pacing.js";
import type { Session } from "./session.js";
import type { TabReport } from "./types.js";

/**
 * The title of a tab, or the empty string. A page holding an open dialog never
 * answers, so the ask is raced against a clock rather than allowed to hang.
 */
async function titleOf(page: Page): Promise<string> {
  const asked = page.title();
  asked.catch(() => {});
  const giveUp = new Promise<string>((answer) => {
    const timer = setTimeout(() => answer(""), PAGE_CALL_DEADLINE_MS);
    timer.unref();
  });
  return Promise.race([asked, giveUp]).catch(() => "");
}

/** Everything that is open, with the one being acted on marked. */
export async function listTabs(session: Session): Promise<TabReport[]> {
  const acting = session.currentPageOrNone();
  const listed: TabReport[] = [];
  for (const page of session.openTabs()) {
    const report: TabReport = {
      id: session.tabId(page),
      url: page.url(),
      title: await titleOf(page),
    };
    if (page === acting) {
      report.active = true;
    }
    listed.push(report);
  }
  return listed;
}

/** The tab with this id, or an error naming the id that was not there. */
function tabOrComplain(session: Session, tabId: string): Page {
  const page = session.tabWithId(tabId);
  if (page === undefined) {
    throw wrongParameters(
      `There is no tab called ${tabId}. Ask for the list of tabs and use one of the ids in it.`,
    );
  }
  return page;
}

/** How long to wait for a closed tab to disappear from the browser's own list. */
const FORGET_LIMIT_MS = 1_000;

/**
 * Wait until the browser has stopped listing a tab that was just closed. Closing
 * answers before the browser has finished forgetting, and a list that still holds
 * a closed tab would tell the model something untrue.
 */
async function waitUntilGone(session: Session, page: Page): Promise<void> {
  const giveUpAt = Date.now() + FORGET_LIMIT_MS;
  while (session.openTabs().includes(page) && Date.now() < giveUpAt) {
    await wait(25);
  }
}

/** List, switch, or close, and always answer with the list afterwards. */
export async function tabsMethod(
  session: Session,
  params: Record<string, unknown>,
): Promise<{ tabs: TabReport[] }> {
  const action = params["action"] ?? "list";
  if (action === "switch") {
    const page = tabOrComplain(session, String(params["tabId"]));
    session.makeActive(page);
    await page.bringToFront().catch(() => {});
    session.log(`now acting on the tab ${session.tabId(page)}.`);
  } else if (action === "close") {
    const page = tabOrComplain(session, String(params["tabId"]));
    session.log(`closing the tab ${session.tabId(page)}.`);
    await page.close({ runBeforeUnload: false });
    await waitUntilGone(session, page);
  }
  return { tabs: await listTabs(session) };
}
