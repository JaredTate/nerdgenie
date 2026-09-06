/**
 * Building the compact tree the model sees.
 *
 * A snapshot is a few hundred tokens: every link, button, text box, checkbox,
 * combobox, menu item, heading, and article that is drawn on the page, each with a
 * ref, a role, and a name. Never the page's markup, and never a page's scripts.
 *
 * Which elements are worth keeping, marking the ones that were not there before,
 * and counting what a person would have to scroll to see are all borrowed from
 * browser-use's page serializer at docs/reference/browser-use/serializer.py. The
 * cap on how many elements one answer may hold is the gap that serializer leaves
 * open, and the code here is written fresh. The page's text rides beside the
 * tree, because the first human trial asked for a number in a table whose cells
 * were icon buttons with no name, and the number was on no element at all.
 */
import type { Frame, Page } from "playwright-core";
import { markNewElements } from "./diff.js";
import {
  MAX_ELEMENT_NAME_CHARS,
  MAX_FRAMES,
  MAX_PAGE_TEXT_CHARS,
  MAX_SNAPSHOT_ELEMENTS,
  REF_NUMBERS_PER_FRAME,
} from "./limits.js";
import { scanFrame, type FoundElement } from "./page-bridge.js";
import { PDF_CONTENT_TYPE, pdfElementName, savePdfShownOnPage } from "./pdf.js";
import type { Session } from "./session.js";
import { pageTextOf, type FrameText } from "./text.js";
import type { Snapshot, SnapshotElement, Wall } from "./types.js";
import { findWall } from "./walls.js";

/** The roles that reach the model. */
export const SNAPSHOT_ROLES: readonly string[] = [
  "link",
  "button",
  "textbox",
  "checkbox",
  "radio",
  "combobox",
  "menuitem",
  "heading",
  "article",
];

/**
 * The roles the page is asked about. A form never reaches the model, but the wall
 * detector needs its name to tell a sign-in form from an ordinary one.
 */
const SCANNED_ROLES: readonly string[] = [...SNAPSHOT_ROLES, "form"];

/** The most elements the walk looks at on one frame before it stops. */
const MOST_NODES_WALKED = 20_000;

/**
 * The ref on the one line a PDF page answers with. It is a label rather than
 * something to act on: pointing at it answers -32000, which is the truth.
 */
const PDF_LABEL_REF = "e0";

/** A page read once: what the model sees, and the wall that stops it if there is one. */
export interface PageReading {
  snapshot: Snapshot;
  wall: Wall | null;
}

/** How to read the page. */
export interface ReadSettings {
  /** Leave out everything a person would have to scroll to see. */
  visibleOnly: boolean;
  /** The snapshot the "new" marks are measured against, or null for none. */
  against: Snapshot | null;
}

/** Ask every frame on the page what is on it, skipping any that will not answer. */
async function scanEveryFrame(
  page: Page,
  log: (line: string) => void,
): Promise<{
  url: string;
  title: string;
  contentType: string;
  found: FoundElement[];
  texts: FrameText[];
  frameUrls: string[];
  hiddenYetDrawn: number;
}> {
  const frames: Frame[] = page.frames().slice(0, MAX_FRAMES);
  const found: FoundElement[] = [];
  let hiddenYetDrawn = 0;
  const texts: FrameText[] = [];
  const frameUrls: string[] = [];
  let url = page.url();
  let title = "";
  let contentType = "";
  for (const [position, frame] of frames.entries()) {
    try {
      const scan = await scanFrame(frame, {
        refBase: position * REF_NUMBERS_PER_FRAME + 1,
        roles: SCANNED_ROLES,
        mostNodes: MOST_NODES_WALKED,
        mostNameCharacters: MAX_ELEMENT_NAME_CHARS,
        mostTextCharacters: MAX_PAGE_TEXT_CHARS,
      });
      if (position === 0) {
        url = scan.url;
        title = scan.title;
        contentType = scan.contentType;
      } else {
        frameUrls.push(scan.url);
      }
      found.push(...scan.elements);
      hiddenYetDrawn += scan.hiddenYetDrawn;
      texts.push(scan.text);
    } catch (problem) {
      const why = problem instanceof Error ? problem.message : String(problem);
      log(`a frame at ${frame.url()} could not be read and was left out: ${why}`);
      if (position === 0) {
        throw problem;
      }
      frameUrls.push(frame.url());
    }
  }
  return { url, title, contentType, found, texts, frameUrls, hiddenYetDrawn };
}

/**
 * A page that is a PDF has no elements to point at, so the answer is one line
 * saying what it is and where the file was put.
 */
async function readPdfPage(
  session: Session,
  page: Page,
  url: string,
  title: string,
  tabId: string,
): Promise<PageReading> {
  const saved = await savePdfShownOnPage(session, page);
  return {
    snapshot: {
      url,
      title,
      tabId,
      elements: [{ ref: PDF_LABEL_REF, role: "article", name: pdfElementName(saved, url) }],
      text: "",
      belowFold: 0,
      hiddenYetDrawn: 0,
      dialog: null,
      download: saved,
      wall: null,
      errors: [],
    },
    wall: null,
  };
}

/**
 * Choose which elements go into the answer. What a person can see comes first, so
 * that the cap, when it bites, cuts what is furthest out of sight.
 */
function chooseElements(found: FoundElement[], visibleOnly: boolean): SnapshotElement[] {
  const bare = (element: FoundElement): SnapshotElement => ({
    ref: element.ref,
    role: element.role,
    name: element.name,
    ...(element.hiddenYetDrawn ? { hiddenYetDrawn: true as const } : {}),
  });
  const visible = found.filter((element) => element.aboveFold).map(bare);
  if (visibleOnly) {
    return visible.slice(0, MAX_SNAPSHOT_ELEMENTS);
  }
  const rest = found.filter((element) => !element.aboveFold).map(bare);
  return [...visible, ...rest].slice(0, MAX_SNAPSHOT_ELEMENTS);
}

/** Are these two snapshots of the same page, so that "new" means anything? */
function comparable(against: Snapshot | null, url: string, tabId: string): boolean {
  return against !== null && against.url === url && against.tabId === tabId;
}

/**
 * Read the page once. A page holding an open dialog cannot be read at all, so the
 * dialog is reported on its own and the rest of the answer is left empty rather
 * than the worker hanging on a page that will never reply.
 */
export async function readPage(
  session: Session,
  page: Page,
  settings: ReadSettings,
): Promise<PageReading> {
  await session.downloadsFinished();
  const tabId = session.tabId(page);
  const dialog = session.dialogOn(page);
  const download = session.takeDownload(page);
  if (dialog !== null) {
    return {
      snapshot: {
        url: page.url(),
        title: settings.against?.title ?? "",
        tabId,
        elements: [],
        text: "",
        belowFold: 0,
        hiddenYetDrawn: 0,
        dialog,
        download,
        wall: null,
        errors: session.pageErrors.on(page),
      },
      wall: null,
    };
  }

  const { url, title, contentType, found, texts, frameUrls, hiddenYetDrawn } = await scanEveryFrame(
    page,
    session.log,
  );
  if (contentType === PDF_CONTENT_TYPE) {
    return readPdfPage(session, page, url, title, tabId);
  }
  // Remember what every ref was, so that one that goes stale can be looked for
  // again by the role and the name it had.
  for (const element of found) {
    session.refs.remember(element.ref, element.role, element.name);
  }
  const shown = found.filter((element) => element.role !== "form");
  const chosen = chooseElements(shown, settings.visibleOnly);
  const outOfSight = shown.filter((element) => !element.aboveFold).length;
  const snapshot: Snapshot = {
    url,
    title,
    tabId,
    elements: markNewElements(
      comparable(settings.against, url, tabId) ? settings.against!.elements : null,
      chosen,
    ),
    text: pageTextOf(texts, MAX_PAGE_TEXT_CHARS),
    belowFold: outOfSight,
    hiddenYetDrawn,
    dialog: null,
    download,
    wall: null,
    errors: session.pageErrors.on(page),
  };
  const wall = findWall({
    title,
    candidates: found.map((element) => ({
      role: element.role,
      name: element.name,
      password: element.password,
      shortNumeric: element.shortNumeric,
    })),
    frames: frameUrls,
  });
  // The wall rides on the snapshot as well as being handed back on its own, so
  // that `open` and `read`, whose answer is a snapshot and nothing else, can
  // report it at all.
  snapshot.wall = wall;
  return { snapshot, wall };
}
