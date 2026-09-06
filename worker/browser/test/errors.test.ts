/**
 * What becomes of an error thrown inside the worker: a worker error is itself,
 * a Playwright action that ran out of its own time on a living page is a
 * refusal the model can act on, and anything unexplained is a dead Chrome.
 */
import { describe, expect, it } from "vitest";
import { asWorkerError, ERROR_CODES } from "../src/errors.js";

describe("turning a thrown error into the one the Go side reads", () => {
  it("reports an action that ran out of time on a busy page as a refusal, not a dead browser", () => {
    // The fresh game build's play-test: the click could not scroll the button
    // into view within eight seconds on a page drawing sixty frames a second,
    // and the worker called Chrome dead, so the Go side restarted it under the
    // model and the page was lost.
    const late = new Error("locator.scrollIntoViewIfNeeded: Timeout 7998ms exceeded.\nCall log:\n  - attempting scroll into view action");
    late.name = "TimeoutError";
    const failure = asWorkerError(late);
    expect(failure.code).toBe(ERROR_CODES.didNotSettle);
    expect(failure.message).toContain("busy, not gone");
    expect(failure.message).toContain("scrollIntoViewIfNeeded");
    expect(failure.message).not.toContain("Call log");
  });

  it("still calls an unexplained failure a dead Chrome", () => {
    const failure = asWorkerError(new Error("Target page, context or browser has been closed"));
    expect(failure.code).toBe(ERROR_CODES.chromeDied);
  });
});
