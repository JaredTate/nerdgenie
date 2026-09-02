/**
 * What the worker remembers between one request and the next.
 *
 * Tabs get short ids that stay with them, so the model can say "t2" and mean the
 * same window a minute later. Dialogs and downloads are noticed as they happen
 * and reported on the next answer, because a page holding an open dialog cannot
 * be read at all.
 *
 * Tracking popups and new tabs as first-class tabs is the gap Moltis's browser
 * manager at docs/reference/moltis/manager.rs leaves open: it keeps one page per
 * session for ever, so a link that opens a new tab leaves the agent looking at
 * the old page. This worker notices instead.
 */
import { join } from "node:path";
import type { BrowserContext, Download, Page } from "playwright-core";
import type { RunningChrome } from "./chrome.js";
import { noBrowserOpen } from "./errors.js";
import { MAX_TABS } from "./limits.js";
import type { Logger } from "./log.js";
import { keystrokeGaps, type Chance, type Pacing, type Point } from "./pacing.js";
import { RefBook } from "./refs.js";
import type { DialogReport, DownloadReport, Snapshot } from "./types.js";

/** Everything the worker needs to drive one browser. */
export interface SessionParts {
  chrome: RunningChrome;
  pacing: Pacing;
  chance: Chance;
  log: Logger;
}

export class Session {
  readonly chrome: RunningChrome;
  readonly pacing: Pacing;
  readonly chance: Chance;
  readonly log: Logger;
  /** What every ref handed out was, so a stale one can be looked for again. */
  readonly refs = new RefBook();

  private readonly tabIds = new Map<Page, string>();
  private readonly dialogs = new Map<Page, DialogReport>();
  private readonly downloads = new Map<Page, DownloadReport>();
  private readonly saving: Array<Promise<void>> = [];
  private nextTabNumber = 1;
  private active: Page | undefined;
  /** The snapshot before the current action, which is what "new" is measured against. */
  private previous: Snapshot | null = null;
  /** A tab that opened during the action now running, or the empty string. */
  private tabOpenedDuringAction = "";
  /** Where the mouse was left, so the next move starts from there and not from a corner. */
  private mousePlace: Point = { x: 0, y: 0 };

  constructor(parts: SessionParts) {
    this.chrome = parts.chrome;
    this.pacing = parts.pacing;
    this.chance = parts.chance;
    this.log = parts.log;
  }

  private get context(): BrowserContext {
    return this.chrome.context;
  }

  /** Start watching for tabs, dialogs, and downloads, and adopt the tabs already open. */
  async watchForNewTabs(): Promise<void> {
    this.context.on("page", (page) => this.adopt(page));
    for (const page of this.context.pages()) {
      this.adopt(page);
    }
    await Promise.resolve();
  }

  /** Give a tab an id and start listening to it. */
  private adopt(page: Page): void {
    if (this.tabIds.has(page)) {
      return;
    }
    const id = `t${this.nextTabNumber}`;
    this.nextTabNumber += 1;
    this.tabIds.set(page, id);
    if (this.active === undefined) {
      this.active = page;
    } else {
      this.tabOpenedDuringAction = id;
      this.log(`a new tab opened and was given the id ${id}.`);
    }
    // A dialog is reported, never answered. Answering for the user would be
    // deciding for the user, and the design says the worker never does that.
    page.on("dialog", (dialog) => {
      this.dialogs.set(page, { kind: dialog.type() as DialogReport["kind"], message: dialog.message() });
      this.log(`the page opened a ${dialog.type()} dialog and it is waiting for an answer.`);
    });
    page.on("download", (download) => this.save(page, download));
    page.on("close", () => {
      this.tabIds.delete(page);
      this.dialogs.delete(page);
      this.downloads.delete(page);
      if (this.active === page) {
        this.active = this.context.pages().find((other) => !other.isClosed());
      }
    });
  }

  /** Save a download under the profile folder and remember it for the next answer. */
  private save(page: Page, download: Download): void {
    const filename = download.suggestedFilename();
    const path = join(this.chrome.downloadsFolder, filename);
    const saved = download
      .saveAs(path)
      .then(() => {
        this.downloads.set(page, { filename, path });
        this.log(`the page saved a download named ${filename}.`);
      })
      .catch((problem: Error) => {
        this.log(`a download named ${filename} could not be saved: ${problem.message}`);
      });
    this.saving.push(saved);
  }

  /** Wait for any download that is still being written, so the answer can report it. */
  async downloadsFinished(): Promise<void> {
    const waiting = this.saving.splice(0, this.saving.length);
    await Promise.all(waiting);
  }

  /** The tab the worker is acting on, or an error saying to open a page first. */
  currentPage(): Page {
    if (this.active === undefined || this.active.isClosed()) {
      throw noBrowserOpen();
    }
    return this.active;
  }

  /** The tab the worker is acting on, or nothing when no page is open. */
  currentPageOrNone(): Page | undefined {
    return this.active !== undefined && !this.active.isClosed() ? this.active : undefined;
  }

  /** Act on this tab from now on. */
  makeActive(page: Page): void {
    this.adopt(page);
    this.active = page;
  }

  /** The short id of one tab. */
  tabId(page: Page): string {
    return this.tabIds.get(page) ?? "t0";
  }

  /** The open tabs, oldest first, capped so a runaway page cannot flood the answer. */
  openTabs(): Page[] {
    return this.context.pages().filter((page) => !page.isClosed()).slice(0, MAX_TABS);
  }

  /** The tab with this id, or nothing. */
  tabWithId(id: string): Page | undefined {
    for (const [page, known] of this.tabIds) {
      if (known === id && !page.isClosed()) {
        return page;
      }
    }
    return undefined;
  }

  /** The dialog waiting on a tab, if one is. */
  dialogOn(page: Page): DialogReport | null {
    return this.dialogs.get(page) ?? null;
  }

  /** The download a tab started, which is reported once and then forgotten. */
  takeDownload(page: Page): DownloadReport | null {
    const download = this.downloads.get(page) ?? null;
    this.downloads.delete(page);
    return download;
  }

  /** The snapshot the next diff is measured against. */
  previousSnapshot(): Snapshot | null {
    return this.previous;
  }

  /** Remember this snapshot as the one the next diff is measured against. */
  rememberSnapshot(snapshot: Snapshot): void {
    this.previous = snapshot;
  }

  /** Start an action: forget any tab that opened before it. */
  beginAction(): void {
    this.tabOpenedDuringAction = "";
  }

  /** The tab that opened while the action was running, or the empty string. */
  tabOpenedDuring(): string {
    return this.tabOpenedDuringAction;
  }

  /** Where the mouse is now. */
  mouseAt(): Point {
    return this.mousePlace;
  }

  /** Remember where the mouse was left. */
  mouseMovedTo(place: Point): void {
    this.mousePlace = place;
  }

  /** The gap to leave after each key, at this worker's pacing. */
  keyGaps(keyCount: number): number[] {
    return keystrokeGaps(this.pacing, keyCount, this.chance);
  }
}
