/**
 * Acting at the speed a person acts.
 *
 * The design says the agent moves the mouse along a curve, holds a click for a
 * human length of time, types one key at a time with small variations in speed,
 * scrolls in steps, and pauses between actions. All of that is about keeping the
 * user's accounts safe: a real Chrome on the user's own network that behaves like
 * a person is not worth banning.
 *
 * Everything here is a plain function that plans the pacing and returns numbers.
 * Nothing here touches the browser, which is what lets the tests check the plan
 * without waiting for it, and what lets `--pacing fast` be one branch rather than
 * a second code path.
 */

/** How the worker paces itself. `fast` exists only for the tests. */
export type Pacing = "human" | "fast";

/** Where the mouse is, in the page's own coordinates. */
export interface Point {
  x: number;
  y: number;
}

/** A source of chance, so that a test can hand in a fixed sequence instead. */
export type Chance = () => number;

/** A range of milliseconds or steps, both ends allowed. */
export interface Range {
  least: number;
  most: number;
}

/** The gap between one key and the next, when a person is typing steadily. */
export const HUMAN_KEY_GAP_MS: Range = { least: 40, most: 160 };

/** How long a person holds a mouse button down for an ordinary click. */
export const HUMAN_CLICK_HOLD_MS: Range = { least: 60, most: 140 };

/** How long a person waits after one action before starting the next. */
export const HUMAN_PAUSE_MS: Range = { least: 150, most: 400 };

/** How many small moves a person's hand makes crossing the screen. */
export const HUMAN_MOUSE_STEPS: Range = { least: 12, most: 20 };

/** How far one turn of a scroll wheel moves the page. */
export const SCROLL_STEP_PIXELS = 400;

/** How long a person waits between one turn of the wheel and the next. */
export const HUMAN_SCROLL_PAUSE_MS: Range = { least: 80, most: 200 };

/** Pick a whole number somewhere in the range, both ends allowed. */
function somewhereIn(range: Range, chance: Chance): number {
  const span = range.most - range.least;
  return range.least + Math.floor(chance() * (span + 1));
}

/** One gap per key. In human pacing they vary; in fast pacing there are no gaps. */
export function keystrokeGaps(pacing: Pacing, keyCount: number, chance: Chance): number[] {
  const gaps: number[] = [];
  for (let key = 0; key < keyCount; key += 1) {
    gaps.push(pacing === "fast" ? 0 : somewhereIn(HUMAN_KEY_GAP_MS, chance));
  }
  return gaps;
}

/** How long to hold the mouse button down for one click. */
export function clickHoldMs(pacing: Pacing, chance: Chance): number {
  return pacing === "fast" ? 0 : somewhereIn(HUMAN_CLICK_HOLD_MS, chance);
}

/** How long to wait after finishing one action. */
export function pauseMs(pacing: Pacing, chance: Chance): number {
  return pacing === "fast" ? 0 : somewhereIn(HUMAN_PAUSE_MS, chance);
}

/** How long to wait between one turn of the scroll wheel and the next. */
export function scrollPauseMs(pacing: Pacing, chance: Chance): number {
  return pacing === "fast" ? 0 : somewhereIn(HUMAN_SCROLL_PAUSE_MS, chance);
}

/**
 * The points the mouse passes through on its way across the screen. A person's
 * hand does not travel in a straight line, so the path is a curve: the straight
 * line between the two points, pushed sideways at the middle by an amount that
 * depends on how far the mouse is going.
 */
export function mousePath(from: Point, to: Point, pacing: Pacing, chance: Chance): Point[] {
  if (pacing === "fast") {
    return [from, to];
  }
  const steps = somewhereIn(HUMAN_MOUSE_STEPS, chance);
  const acrossX = to.x - from.x;
  const acrossY = to.y - from.y;
  const distance = Math.hypot(acrossX, acrossY);
  // How far the curve bends: a fraction of the trip, to one side or the other.
  const bend = distance * 0.12 * (chance() - 0.5) * 2;
  // The sideways direction is the trip turned a quarter turn. A trip of no
  // distance has no sideways, so the curve flattens into a stand-still.
  const sidewaysX = distance === 0 ? 0 : -acrossY / distance;
  const sidewaysY = distance === 0 ? 0 : acrossX / distance;
  const controlX = from.x + acrossX / 2 + sidewaysX * bend;
  const controlY = from.y + acrossY / 2 + sidewaysY * bend;
  const points: Point[] = [];
  for (let step = 0; step <= steps; step += 1) {
    const along = step / steps;
    const back = 1 - along;
    points.push({
      x: back * back * from.x + 2 * back * along * controlX + along * along * to.x,
      y: back * back * from.y + 2 * back * along * controlY + along * along * to.y,
    });
  }
  points[0] = from;
  points[points.length - 1] = to;
  return points;
}

/** One wheel turn per unit of the amount, up the page or down it. */
export function scrollDeltas(direction: "up" | "down", amount: number): number[] {
  const step = direction === "up" ? -SCROLL_STEP_PIXELS : SCROLL_STEP_PIXELS;
  return Array.from({ length: amount }, () => step);
}

/** Wait, unless there is nothing to wait for. */
export function wait(milliseconds: number): Promise<void> {
  if (milliseconds <= 0) {
    return Promise.resolve();
  }
  return new Promise((finished) => setTimeout(finished, milliseconds));
}
