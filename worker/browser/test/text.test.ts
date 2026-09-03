import { describe, expect, it } from "vitest";
import { pageTextOf } from "../src/text.js";

describe("joining the frames' text under one cap", () => {
  it("joins the frames in order with a line break between them", () => {
    expect(pageTextOf([{ text: "one\ntwo", cut: 0 }, { text: "three", cut: 0 }], 100)).toBe(
      "one\ntwo\nthree",
    );
  });

  it("ends with no cut line when nothing was cut", () => {
    expect(pageTextOf([{ text: "whole", cut: 0 }], 100)).not.toContain("cut");
  });

  it("leaves out a frame that had no text at all", () => {
    expect(pageTextOf([{ text: "", cut: 0 }, { text: "a", cut: 0 }], 100)).toBe("a");
  });

  it("says in the last line how much the page itself cut", () => {
    expect(pageTextOf([{ text: "abc", cut: 5 }], 100)).toBe("abc\n... 5 more characters were cut");
  });

  it("cuts at a line end when the frames together pass the cap, counting what the frames cut too", () => {
    const frames = [
      { text: "one\ntwo", cut: 0 },
      { text: "three\nfour", cut: 2 },
    ];
    expect(pageTextOf(frames, 8)).toBe("one\ntwo\n... 13 more characters were cut");
  });

  it("cuts inside a line when the first line alone is longer than the cap", () => {
    expect(pageTextOf([{ text: "abcdefghij", cut: 0 }], 4)).toBe(
      "abcd\n... 6 more characters were cut",
    );
  });

  it("answers nothing for a page with no text", () => {
    expect(pageTextOf([], 100)).toBe("");
  });
});
