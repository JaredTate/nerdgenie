import { describe, expect, it } from "vitest";
import {
  HUMAN_CLICK_HOLD_MS,
  HUMAN_KEY_GAP_MS,
  HUMAN_MOUSE_STEPS,
  HUMAN_PAUSE_MS,
  SCROLL_STEP_PIXELS,
  clickHoldMs,
  keystrokeGaps,
  mousePath,
  pauseMs,
  scrollDeltas,
} from "../src/pacing.js";

/** A stand-in for chance that walks a fixed list, so a test can pin exact numbers. */
function fixedChance(values: number[]): () => number {
  let next = 0;
  return () => {
    const value = values[next % values.length] ?? 0;
    next += 1;
    return value;
  };
}

describe("typing one key at a time", () => {
  it("gives one gap per key", () => {
    expect(keystrokeGaps("human", 5, Math.random)).toHaveLength(5);
  });

  it("varies the gaps in human pacing, the way a person's hands do", () => {
    const gaps = keystrokeGaps("human", 40, Math.random);
    expect(new Set(gaps).size).toBeGreaterThan(1);
  });

  it("keeps every human gap inside the range", () => {
    for (const gap of keystrokeGaps("human", 200, Math.random)) {
      expect(gap).toBeGreaterThanOrEqual(HUMAN_KEY_GAP_MS.least);
      expect(gap).toBeLessThanOrEqual(HUMAN_KEY_GAP_MS.most);
    }
  });

  it("uses the bottom of the range when chance says nothing and the top when it says everything", () => {
    expect(keystrokeGaps("human", 2, fixedChance([0, 0.999999]))).toEqual([
      HUMAN_KEY_GAP_MS.least,
      HUMAN_KEY_GAP_MS.most,
    ]);
  });

  it("waits for nothing in fast pacing, which is only for the tests", () => {
    expect(keystrokeGaps("fast", 4, Math.random)).toEqual([0, 0, 0, 0]);
  });
});

describe("holding a click for a human length of time", () => {
  it("holds inside the range in human pacing", () => {
    const held = clickHoldMs("human", Math.random);
    expect(held).toBeGreaterThanOrEqual(HUMAN_CLICK_HOLD_MS.least);
    expect(held).toBeLessThanOrEqual(HUMAN_CLICK_HOLD_MS.most);
  });

  it("holds for nothing in fast pacing", () => {
    expect(clickHoldMs("fast", Math.random)).toBe(0);
  });
});

describe("pausing between actions", () => {
  it("pauses inside the range in human pacing", () => {
    const paused = pauseMs("human", Math.random);
    expect(paused).toBeGreaterThanOrEqual(HUMAN_PAUSE_MS.least);
    expect(paused).toBeLessThanOrEqual(HUMAN_PAUSE_MS.most);
  });

  it("pauses for nothing in fast pacing", () => {
    expect(pauseMs("fast", Math.random)).toBe(0);
  });
});

describe("moving the mouse along a curve", () => {
  const from = { x: 10, y: 20 };
  const to = { x: 400, y: 300 };

  it("has more than two points in human pacing, so the path is a curve and not a jump", () => {
    expect(mousePath(from, to, "human", Math.random).length).toBeGreaterThan(2);
  });

  it("keeps the number of points inside the range", () => {
    const points = mousePath(from, to, "human", Math.random);
    expect(points.length).toBeGreaterThanOrEqual(HUMAN_MOUSE_STEPS.least + 1);
    expect(points.length).toBeLessThanOrEqual(HUMAN_MOUSE_STEPS.most + 1);
  });

  it("starts where the mouse was and ends where it was sent", () => {
    const points = mousePath(from, to, "human", Math.random);
    expect(points[0]).toEqual(from);
    expect(points[points.length - 1]).toEqual(to);
  });

  it("bends away from the straight line at least somewhere", () => {
    const points = mousePath(from, to, "human", fixedChance([1]));
    const middle = points[Math.floor(points.length / 2)];
    const straightY = from.y + ((to.y - from.y) * (middle!.x - from.x)) / (to.x - from.x);
    expect(Math.abs(middle!.y - straightY)).toBeGreaterThan(1);
  });

  it("gives every point a real number", () => {
    for (const point of mousePath(from, to, "human", Math.random)) {
      expect(Number.isFinite(point.x)).toBe(true);
      expect(Number.isFinite(point.y)).toBe(true);
    }
  });

  it("goes straight there in fast pacing", () => {
    expect(mousePath(from, to, "fast", Math.random)).toEqual([from, to]);
  });

  it("does not divide by zero when the mouse is already there", () => {
    const points = mousePath(from, from, "human", Math.random);
    expect(points[0]).toEqual(from);
    expect(points[points.length - 1]).toEqual(from);
    for (const point of points) {
      expect(Number.isFinite(point.x)).toBe(true);
    }
  });
});

describe("scrolling in steps", () => {
  it("gives one step down for each unit of the amount", () => {
    expect(scrollDeltas("down", 3)).toEqual([
      SCROLL_STEP_PIXELS,
      SCROLL_STEP_PIXELS,
      SCROLL_STEP_PIXELS,
    ]);
  });

  it("gives negative steps going up", () => {
    expect(scrollDeltas("up", 2)).toEqual([-SCROLL_STEP_PIXELS, -SCROLL_STEP_PIXELS]);
  });

  it("scrolls in steps rather than one jump, whatever the pacing", () => {
    expect(scrollDeltas("down", 5)).toHaveLength(5);
  });
});
