/**
 * Comparing one snapshot with the one before it.
 *
 * Every action returns a diff: what changed, and whether what the model said it
 * expected actually happened. The comparison is what lets a small model see the
 * result of its own action without re-reading the whole page.
 *
 * The idea of marking the elements that were not in the previous snapshot, so
 * that the model can see what its action produced, is borrowed from browser-use's
 * page serializer at docs/reference/browser-use/serializer.py, which stars new
 * elements and marks nothing at all on the first snapshot. The code is fresh.
 */
import { judge, type Change } from "./expectation.js";
import type { Diff, Snapshot, SnapshotElement, Wall } from "./types.js";

/**
 * Mark the elements whose ref was not in the previous snapshot. A ref stays with
 * an element for as long as it is on the page, so a ref that is new means an
 * element that is new. On the very first snapshot nothing is marked, because
 * everything would be, and a page of stars tells the model nothing.
 */
export function markNewElements(
  before: readonly SnapshotElement[] | null,
  current: readonly SnapshotElement[],
): SnapshotElement[] {
  if (before === null) {
    return current.map((element) => ({ ...element }));
  }
  const known = new Set(before.map((element) => element.ref));
  return current.map((element) =>
    known.has(element.ref) ? { ...element } : { ...element, new: true as const },
  );
}

/** What one action changed, worked out from the two snapshots around it. */
function changeBetween(before: Snapshot | null, after: Snapshot, newTab: string): Change {
  const stillHere = new Set(after.elements.map((element) => element.ref));
  const removedCount =
    before === null
      ? 0
      : before.elements.filter((element) => !stillHere.has(element.ref)).length;
  return {
    urlChanged: before !== null && before.url !== after.url,
    url: after.url,
    titleChanged: before !== null && before.title !== after.title,
    title: after.title,
    newElements: after.elements.filter((element) => element.new === true),
    removedCount,
    dialog: after.dialog,
    newTab,
    download: after.download,
  };
}

/** Everything the diff builder needs to know about one action. */
export interface DiffInput {
  /** The snapshot before the action, or null when there was none. */
  before: Snapshot | null;
  /** The snapshot after the page settled. */
  after: Snapshot;
  /** What the model said it expected to happen. */
  expectation: string;
  /** The id of a tab that appeared during the action, or the empty string. */
  newTab: string;
  /** The wall the action ran into, or null. */
  wall: Wall | null;
}

/**
 * Build the diff for one action. A wall always wins: the expectation is not met,
 * and `seen` says which wall stopped the worker, because the model's next move is
 * to hand the browser to the user rather than to try again.
 */
export function buildDiff(input: DiffInput): Diff {
  const change = changeBetween(input.before, input.after, input.newTab);
  const verdict =
    input.wall === null
      ? judge(input.expectation, change)
      : {
          expectationMet: false,
          seen: `the browser hit a ${input.wall.kind} wall: ${input.wall.detail}`,
        };
  return {
    urlChanged: change.urlChanged,
    url: change.url,
    newElements: change.newElements,
    dialog: change.dialog,
    newTab: change.newTab,
    download: change.download,
    expectationMet: verdict.expectationMet,
    seen: verdict.seen,
    wall: input.wall,
    snapshot: input.after,
  };
}
