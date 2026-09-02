import { describe, expect, it } from "vitest";
import { REDACTED, redactDeep, redactText } from "../src/redact.js";

describe("replacing a secret in one piece of text", () => {
  it("replaces every occurrence, not only the first", () => {
    expect(redactText("hunter2 and hunter2 again", ["hunter2"])).toBe(
      `${REDACTED} and ${REDACTED} again`,
    );
  });

  it("replaces a secret that sits inside a longer word", () => {
    expect(redactText("myhunter2password", ["hunter2"])).toBe(`my${REDACTED}password`);
  });

  it("replaces the longest secret first, so a shorter one cannot cut it in half", () => {
    expect(redactText("hunter2extra", ["hunter2", "hunter2extra"])).toBe(REDACTED);
  });

  it("treats a secret as plain text, never as a pattern", () => {
    expect(redactText("a.c and abc", ["a.c"])).toBe(`${REDACTED} and abc`);
  });

  it("leaves the text alone when no secret is in it", () => {
    expect(redactText("nothing to hide", ["hunter2"])).toBe("nothing to hide");
  });

  it("ignores an empty secret, which would otherwise blot out the whole text", () => {
    expect(redactText("still here", ["", "  "])).toBe("still here");
  });

  it("ignores capital letters, because a page may echo a value back in a different case", () => {
    expect(redactText("HUNTER2", ["hunter2"])).toBe(REDACTED);
  });
});

describe("replacing a secret everywhere in a whole object", () => {
  it("reaches into nested objects and arrays", () => {
    const before = {
      url: "https://example.com/?user=someone",
      elements: [
        { ref: "e1", role: "textbox", name: "someone" },
        { ref: "e2", role: "button", name: "Sign in" },
      ],
      snapshot: { title: "Welcome someone", elements: [] },
    };
    expect(redactDeep(before, ["someone"])).toEqual({
      url: `https://example.com/?user=${REDACTED}`,
      elements: [
        { ref: "e1", role: "textbox", name: REDACTED },
        { ref: "e2", role: "button", name: "Sign in" },
      ],
      snapshot: { title: `Welcome ${REDACTED}`, elements: [] },
    });
  });

  it("replaces a secret that a page used as a key", () => {
    expect(redactDeep({ someone: "here" }, ["someone"])).toEqual({ [REDACTED]: "here" });
  });

  it("leaves numbers, booleans, and nulls exactly as they were", () => {
    const before = { belowFold: 24, expectationMet: true, dialog: null, wall: null };
    expect(redactDeep(before, ["24"])).toEqual(before);
  });

  it("gives back the same shape when there are no secrets at all", () => {
    const before = { a: [1, "two", { three: true }] };
    expect(redactDeep(before, [])).toEqual(before);
  });

  it("stops at a depth cap rather than following a loop for ever", () => {
    const looping: Record<string, unknown> = { name: "someone" };
    looping["self"] = looping;
    const after = redactDeep(looping, ["someone"]) as Record<string, unknown>;
    expect(after["name"]).toBe(REDACTED);
    expect(JSON.stringify(after)).not.toContain("someone");
  });

  it("never lets a secret survive anywhere in the written-out response", () => {
    const before = {
      jsonrpc: "2.0",
      id: 9,
      result: {
        url: "https://example.com/home?token=s3cr3t",
        snapshot: { elements: [{ ref: "e1", role: "textbox", name: "s3cr3t" }] },
      },
    };
    expect(JSON.stringify(redactDeep(before, ["s3cr3t"]))).not.toContain("s3cr3t");
  });
});
