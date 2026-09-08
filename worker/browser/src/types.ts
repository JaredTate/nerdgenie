/**
 * The shapes in worker/browser/PROTOCOL.md, written as TypeScript types.
 *
 * The Go side reads exactly these fields, and `internal/contract/browser.go`
 * says the same thing in Go. When the two disagree, the protocol document wins.
 */

/** The twelve methods, in the order the protocol lists them. */
export const METHOD_NAMES = [
  "open",
  "read",
  "click",
  "type",
  "press",
  "scroll",
  "act",
  "tabs",
  "loginFill",
  "screenshot",
  "dialog",
  "health",
  "resize",
] as const;

export type MethodName = (typeof METHOD_NAMES)[number];

/** The four methods that may appear as a step inside an act batch. */
export const STEP_METHOD_NAMES = ["click", "type", "press", "scroll"] as const;

export type StepMethodName = (typeof STEP_METHOD_NAMES)[number];

/** One element of a snapshot: what the model points at. */
export interface SnapshotElement {
  /** The short label the model points at later, always the letter e and a number. */
  ref: string;
  role: string;
  name: string;
  /** Present and true only when the element was not in the previous snapshot. */
  new?: true;
  /**
   * Present and true only when the markup says hidden, on the element or above
   * it, and yet the element is drawn, because a style rule overrides the
   * attribute. On the fresh Tetris build every overlay was drawn at once, the
   * GAME OVER card on top of the start card, and the model read the page's
   * outline and its screenshot without seeing why.
   */
  hiddenYetDrawn?: true;
}

/** A dialog box that the page opened and that nobody has answered. */
export interface DialogReport {
  kind: "alert" | "confirm" | "prompt" | "beforeunload";
  message: string;
}

/** A file the page started downloading, saved under the profile folder. */
export interface DownloadReport {
  filename: string;
  path: string;
}

/** What the model sees of a page: a compact tree, never the page's markup. */
export interface Snapshot {
  url: string;
  title: string;
  tabId: string;
  elements: SnapshotElement[];
  /**
   * What the page says, as a person reads it: its visible text in reading
   * order, one line per block, a table as one line per row with the cells
   * separated by " | ", capped, with a last line saying how much was cut.
   */
  text: string;
  /** How many elements a person would have to scroll to see, plus any the cap cut. */
  belowFold: number;
  /**
   * How many nodes the markup says are hidden and the browser draws all the
   * same, because a style rule overrides the attribute. Nodes with no role
   * count too, which never reach `elements`: a badge, a row of touch controls.
   */
  hiddenYetDrawn: number;
  dialog: DialogReport | null;
  download: DownloadReport | null;
  /**
   * The wall the page shows, or null. `open` and `read` report a wall here,
   * because a page can be a login page before any action; an action reports the
   * wall it ran into on its diff instead.
   */
  wall: Wall | null;
  /**
   * What went wrong on the page since it last moved to an address, as a person
   * with the console open sees it: uncaught script errors, console errors, and
   * the page, scripts and stylesheets that failed to load or answered an error
   * status. The first few, each on one line, then one line counting the rest.
   */
  errors: string[];
  /**
   * The page's answer to the expression the read asked, as JSON, or the error
   * the page threw in words. Present only when a read asked something, which
   * only a page on this machine or a file may be.
   */
  answer?: string;
}

/** One of the three things that stop the agent and hand the browser to the user. */
export interface Wall {
  kind: "login" | "two-factor" | "captcha";
  detail: string;
}

/** What one action changed, and whether what the model expected actually happened. */
export interface Diff {
  urlChanged: boolean;
  url: string;
  newElements: SnapshotElement[];
  /**
   * The lines of the page's text that were not there before, which is how a
   * number or a message that changed is seen: a counter going from 0 to 1 adds
   * no element, and a diff that looked only at elements clicked it again.
   */
  newText: string[];
  dialog: DialogReport | null;
  /** The id of a tab that appeared during the action, or the empty string. */
  newTab: string;
  download: DownloadReport | null;
  expectationMet: boolean;
  /** Plain words for what happened instead, filled in only when the expectation was not met. */
  seen: string;
  /**
   * For a click at a point: the listed element under the point, such as
   * `e2 button "cell 1"`, or "nothing the outline lists", so a click that
   * changed nothing says what it hit. Empty for a click by reference.
   */
  under: string;
  wall: Wall | null;
  /**
   * Whether the page came to rest within the limit. When it did not, the diff is
   * still the page as it stood, and `seen` says the page kept changing.
   */
  settled: boolean;
  snapshot: Snapshot;
}

/** One open tab. */
export interface TabReport {
  id: string;
  url: string;
  title: string;
  /** Present and true only on the tab the worker is acting on. */
  active?: true;
}

/** What to do with an open dialog box. */
export const DIALOG_ACTIONS = ["accept", "dismiss"] as const;

export type DialogAnswer = (typeof DIALOG_ACTIONS)[number];

/** One numbered mark drawn on a screenshot. */
export interface ScreenshotMark {
  number: number;
  ref: string;
  role: string;
  name: string;
}

/** One step of an act batch: a flat object, the way OpenClaw's tool schema keeps them flat. */
export interface ActStep {
  method: StepMethodName;
  ref?: string;
  /** A click step may name a point instead of a ref: whole CSS pixels from the top left of the viewport. */
  x?: number;
  y?: number;
  text?: string;
  key?: string;
  direction?: "up" | "down";
  amount?: number;
  expectation?: string;
}

/** The three things a person does in the window that the worker reports. */
export const PERSON_EVENT_KINDS = ["click", "type", "navigate"] as const;

export type PersonEventKind = (typeof PERSON_EVENT_KINDS)[number];

/**
 * One thing the person did in the window themselves. What they typed is never
 * carried: a typing event says how many characters the box holds and no more,
 * so that a recording of somebody signing in cannot hold their password.
 */
export interface PersonEvent {
  kind: PersonEventKind;
  /** The element clicked or typed into, and absent on a navigation. */
  ref?: string;
  /** What the clicked element says, and absent on the other two kinds. */
  text?: string;
  /** How many characters the box holds, and absent on the other two kinds. */
  length?: number;
  /** Where the window went, and absent on the other two kinds. */
  address?: string;
  /** When it happened, as an ISO 8601 moment such as 2026-09-03T10:00:00.000Z. */
  at: string;
}

/** The same thing before the worker stamps the moment on it. */
export type WhatThePersonDid = Omit<PersonEvent, "at">;

/** A line the worker sends that nobody asked for, which is only ever an event. */
export interface EventNotification {
  jsonrpc: "2.0";
  method: "event";
  params: PersonEvent;
}

/** A request the worker accepted and is about to run. */
export interface WorkerRequest {
  id: number;
  method: MethodName;
  params: Record<string, unknown>;
}

/** A JSON-RPC response, in the two shapes the protocol allows. */
export type JsonRpcResponse =
  | { jsonrpc: "2.0"; id: number; result: Record<string, unknown> }
  | {
      jsonrpc: "2.0";
      id: number | null;
      error: { code: number; message: string; data?: Record<string, unknown> };
    };
