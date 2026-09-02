/**
 * Act and assert: every action says what it expects, and the worker checks it.
 *
 * The worker cannot judge English, so the rule is fixed and every part of it is
 * tested. Split the expectation into words of four or more letters that are not
 * stop words. The expectation is met when any of those words turns up in a new
 * element's name or role, in the new address, in the new title, or in a dialog's
 * message. An expectation with no such words counts as no expectation at all,
 * and is met when anything changed.
 *
 * The idea of judging every step against what the model said it expected, and of
 * writing back one plain sentence about what happened instead, is borrowed from
 * browser-use's per-step evaluation fields at docs/reference/browser-use/views.py.
 * The code here is written fresh.
 */
import type { DialogReport, DownloadReport, SnapshotElement } from "./types.js";
import { meaningfulWords, textHoldsAnyWord } from "./words.js";

export { meaningfulWords } from "./words.js";

/** Everything one action changed, which is all the evidence the rule may use. */
export interface Change {
  urlChanged: boolean;
  url: string;
  titleChanged: boolean;
  title: string;
  newElements: SnapshotElement[];
  removedCount: number;
  dialog: DialogReport | null;
  newTab: string;
  download: DownloadReport | null;
}

/** Did anything at all happen? An empty expectation is met when this is true. */
export function somethingChanged(change: Change): boolean {
  return (
    change.urlChanged ||
    change.titleChanged ||
    change.newElements.length > 0 ||
    change.removedCount > 0 ||
    change.dialog !== null ||
    change.newTab !== "" ||
    change.download !== null
  );
}

/** The places a word from the expectation is allowed to match. */
function placesToLook(change: Change): string[] {
  const places: string[] = [];
  for (const element of change.newElements) {
    places.push(element.name, element.role);
  }
  if (change.urlChanged) {
    places.push(change.url);
  }
  if (change.titleChanged) {
    places.push(change.title);
  }
  if (change.dialog !== null) {
    places.push(change.dialog.message);
  }
  return places;
}

/** Was what the model said it expected actually seen? */
export function isExpectationMet(expectation: string, change: Change): boolean {
  const words = meaningfulWords(expectation);
  if (words.length === 0) {
    return somethingChanged(change);
  }
  return placesToLook(change).some((place) => textHoldsAnyWord(place, words));
}

const NUMBER_WORDS = [
  "no",
  "one",
  "two",
  "three",
  "four",
  "five",
  "six",
  "seven",
  "eight",
  "nine",
  "ten",
] as const;

/** Write a small count as a word, the way a person would say it out loud. */
function countInWords(count: number): string {
  return NUMBER_WORDS[count] ?? String(count);
}

/** The most element names one sentence lists before it says how many are left. */
const MOST_NAMES_LISTED = 3;

/** "two new buttons appeared: "Post", "Cancel"" and its one-element and many-element forms. */
function describeNewElements(elements: SnapshotElement[]): string {
  const roles = new Set(elements.map((element) => element.role));
  const oneRole = roles.size === 1 ? [...roles][0] : undefined;
  const noun = oneRole ?? "element";
  const plural = elements.length === 1 ? noun : `${noun}s`;
  const listed = elements
    .slice(0, MOST_NAMES_LISTED)
    .map((element) => JSON.stringify(element.name));
  const leftOver = elements.length - listed.length;
  const names = leftOver > 0 ? `${listed.join(", ")}, and ${leftOver} more` : listed.join(", ");
  return `${countInWords(elements.length)} new ${plural} appeared: ${names}`;
}

/**
 * One sentence for what happened, in the order that matters most to a person: a
 * dialog blocks the page, a move to a new address replaces it, and only then do
 * the smaller changes get a turn.
 */
export function describeChange(change: Change): string {
  if (change.dialog !== null) {
    return `a dialog appeared saying ${JSON.stringify(change.dialog.message)}`;
  }
  if (change.urlChanged) {
    return `the address changed to ${change.url}`;
  }
  if (change.newTab !== "") {
    return "a new tab opened";
  }
  if (change.download !== null) {
    return `a download started: ${change.download.filename}`;
  }
  if (change.newElements.length > 0) {
    return describeNewElements(change.newElements);
  }
  if (change.titleChanged) {
    return `the title changed to ${JSON.stringify(change.title)}`;
  }
  if (change.removedCount > 0) {
    const noun = change.removedCount === 1 ? "element" : "elements";
    return `${countInWords(change.removedCount)} ${noun} went away`;
  }
  return "nothing changed";
}

/**
 * Judge one action: was the expectation met, and if not, what was seen instead.
 * When the expectation was met there is nothing more to say, so `seen` is empty.
 */
export function judge(expectation: string, change: Change): { expectationMet: boolean; seen: string } {
  if (isExpectationMet(expectation, change)) {
    return { expectationMet: true, seen: "" };
  }
  return { expectationMet: false, seen: describeChange(change) };
}
