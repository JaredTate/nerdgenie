/**
 * Finding the element a ref points at, and finding it again when the ref has
 * gone stale.
 *
 * A ref is written onto the element as an attribute, so most of the time finding
 * it is one query. But a page that rebuilds part of itself throws the attribute
 * away with the old element, and the model is still holding the old ref. So the
 * worker remembers what each ref was, and when the attribute is gone it looks for
 * the element again by its role and its name, and then by its visible text.
 *
 * Moltis's snapshot at docs/reference/moltis/snapshot.rs has no recovery at all: a
 * vanished ref is a hard error carrying only a number, and the model is told
 * nothing about what it was pointing at. This is the part worth improving on, and
 * PROTOCOL.md asks for exactly that improvement.
 */
import type { Frame, Locator, Page } from "playwright-core";
import { MAX_ELEMENT_NAME_CHARS, MAX_FRAMES, MAX_REMEMBERED_REFS } from "./limits.js";
import { findLike } from "./page-bridge.js";
import { REF_NUMBERS_PER_FRAME } from "./limits.js";

/** The shape every ref has: the letter e and a number. */
const REF_SHAPE = /^e\d+$/;

/** The most elements the search walks on one frame before it stops. */
const MOST_NODES_WALKED = 20_000;

/** What a ref was, so that it can be looked for again after the page rebuilds. */
export interface RememberedRef {
  role: string;
  name: string;
}

/**
 * What the worker knows about the refs it has handed out. It is capped, and the
 * oldest is forgotten first, so a long session cannot grow without end.
 */
export class RefBook {
  private readonly known = new Map<string, RememberedRef>();

  remember(ref: string, role: string, name: string): void {
    this.known.delete(ref);
    this.known.set(ref, { role, name });
    while (this.known.size > MAX_REMEMBERED_REFS) {
      const oldest = this.known.keys().next();
      if (oldest.done === true) {
        return;
      }
      this.known.delete(oldest.value);
    }
  }

  recall(ref: string): RememberedRef | undefined {
    return this.known.get(ref);
  }
}

/** An element the worker found, and the frame it lives in. */
export interface FoundTarget {
  frame: Frame;
  locator: Locator;
  /** The ref the element carries now, which is a new one when it had to be found again. */
  ref: string;
  /** How it was found, for the log. */
  how: "by its ref" | "by its role and name" | "by its visible text";
}

function locatorFor(frame: Frame, ref: string): Locator {
  return frame.locator(`[data-nerdgenie-ref="${ref}"]`);
}

/** Look for the attribute the ref was written onto, across every frame. */
async function byRef(frames: Frame[], ref: string): Promise<FoundTarget | undefined> {
  for (const frame of frames) {
    const locator = locatorFor(frame, ref);
    const count = await locator.count().catch(() => 0);
    if (count > 0) {
      return { frame, locator: locator.first(), ref, how: "by its ref" };
    }
  }
  return undefined;
}

/** Look for an element that matches what the ref used to be. */
async function byDescription(
  frames: Frame[],
  was: RememberedRef,
  byText: boolean,
): Promise<FoundTarget | undefined> {
  for (const [position, frame] of frames.entries()) {
    const fresh = await findLike(frame, {
      role: byText ? "" : was.role,
      name: was.name,
      byText,
      refBase: position * REF_NUMBERS_PER_FRAME + 1,
      mostNodes: MOST_NODES_WALKED,
      mostNameCharacters: MAX_ELEMENT_NAME_CHARS,
    }).catch(() => null);
    if (fresh !== null) {
      return {
        frame,
        locator: locatorFor(frame, fresh).first(),
        ref: fresh,
        how: byText ? "by its visible text" : "by its role and name",
      };
    }
  }
  return undefined;
}

/**
 * Find the element a ref points at: by the ref, then by the role and name it had,
 * then by its visible text. Nothing found means the caller answers -32000.
 */
export async function findRef(
  page: Page,
  book: RefBook,
  ref: string,
): Promise<FoundTarget | undefined> {
  if (!REF_SHAPE.test(ref)) {
    return undefined;
  }
  const frames = page.frames().slice(0, MAX_FRAMES);
  const straight = await byRef(frames, ref);
  if (straight !== undefined) {
    return straight;
  }
  const was = book.recall(ref);
  if (was === undefined || was.name === "") {
    return undefined;
  }
  return (await byDescription(frames, was, false)) ?? (await byDescription(frames, was, true));
}
