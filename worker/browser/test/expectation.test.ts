import { describe, expect, it } from "vitest";
import {
  describeChange,
  isExpectationMet,
  judge,
  meaningfulWords,
  somethingChanged,
  type Change,
} from "../src/expectation.js";
import type { SnapshotElement } from "../src/types.js";

/** A change in which nothing at all happened, to be filled in one field at a time. */
function nothing(): Change {
  return {
    urlChanged: false,
    url: "https://example.com/start",
    titleChanged: false,
    title: "Start",
    newElements: [],
    removedCount: 0,
    dialog: null,
    newTab: "",
    download: null,
  };
}

function change(part: Partial<Change>): Change {
  return { ...nothing(), ...part };
}

function element(ref: string, role: string, name: string): SnapshotElement {
  return { ref, role, name, new: true };
}

describe("splitting an expectation into the words that can be looked for", () => {
  it("keeps words of four or more letters and drops the short ones", () => {
    expect(meaningfulWords("the post appears in the timeline")).toEqual([
      "post",
      "appears",
      "timeline",
    ]);
  });

  it("drops stop words, which are common enough to match by accident", () => {
    expect(meaningfulWords("this page should have your name")).toEqual(["name"]);
  });

  it("lowercases and splits on anything that is not a letter or a digit", () => {
    expect(meaningfulWords("Sign-In: Two_Factor codes!")).toEqual(["sign", "factor", "codes"]);
  });

  it("gives nothing back for an expectation made only of filler", () => {
    expect(meaningfulWords("it is now")).toEqual([]);
  });

  it("never repeats a word", () => {
    expect(meaningfulWords("timeline the timeline shows timeline")).toEqual(["timeline", "shows"]);
  });
});

describe("deciding whether the expectation was met", () => {
  const met: Array<[string, string, Change]> = [
    [
      "a word appears in a new element's name",
      "the post button is ready",
      change({ newElements: [element("e7", "button", "Post")] }),
    ],
    [
      "a word appears in a new element's role",
      "a text box for the message",
      change({ newElements: [element("e3", "textbox", "Message")] }),
    ],
    [
      "a word appears in the new address",
      "the timeline loads",
      change({ urlChanged: true, url: "https://example.com/timeline" }),
    ],
    [
      "a word appears in the new title",
      "the compose screen opens",
      change({ titleChanged: true, title: "Compose post" }),
    ],
    [
      "a word appears in a dialog's message",
      "a warning about leaving",
      change({ dialog: { kind: "confirm", message: "Leaving will lose your draft" } }),
    ],
    ["the expectation is empty and something changed", "", change({ urlChanged: true })],
    [
      "the expectation is only filler and something changed",
      "it is now",
      change({ urlChanged: true }),
    ],
    [
      "the match ignores capital letters",
      "the POST appears",
      change({ newElements: [element("e7", "button", "post")] }),
    ],
  ];

  for (const [description, expectation, evidence] of met) {
    it(`is met when ${description}`, () => {
      expect(isExpectationMet(expectation, evidence)).toBe(true);
    });
  }

  const notMet: Array<[string, string, Change]> = [
    ["nothing changed at all", "the post appears", nothing()],
    [
      "something changed but no word matches",
      "the timeline appears",
      change({ newElements: [element("e9", "button", "Cancel")] }),
    ],
    ["the expectation is empty and nothing changed", "", nothing()],
    [
      "a word only matches the address that was already there",
      "the start page",
      change({ newElements: [element("e9", "button", "Cancel")] }),
    ],
  ];

  for (const [description, expectation, evidence] of notMet) {
    it(`is not met when ${description}`, () => {
      expect(isExpectationMet(expectation, evidence)).toBe(false);
    });
  }

  it("only looks at the address when the address actually changed", () => {
    expect(isExpectationMet("start", nothing())).toBe(false);
  });
});

describe("noticing that anything changed at all", () => {
  it("is false when the page is exactly as it was", () => {
    expect(somethingChanged(nothing())).toBe(false);
  });

  const changes: Array<[string, Change]> = [
    ["the address moved", change({ urlChanged: true })],
    ["the title moved", change({ titleChanged: true })],
    ["an element appeared", change({ newElements: [element("e1", "button", "Go")] })],
    ["elements went away", change({ removedCount: 2 })],
    ["a dialog opened", change({ dialog: { kind: "alert", message: "Saved" } })],
    ["a tab opened", change({ newTab: "t2" })],
    ["a download started", change({ download: { filename: "a.txt", path: "/tmp/a.txt" } })],
  ];

  for (const [description, evidence] of changes) {
    it(`is true when ${description}`, () => {
      expect(somethingChanged(evidence)).toBe(true);
    });
  }
});

describe("saying in one sentence what did change", () => {
  const sentences: Array<[Change, string]> = [
    [nothing(), "nothing changed"],
    [
      change({ urlChanged: true, url: "https://example.com/home" }),
      "the address changed to https://example.com/home",
    ],
    [
      change({ newElements: [element("e7", "button", "Post")] }),
      'one new button appeared: "Post"',
    ],
    [
      change({
        newElements: [element("e7", "button", "Post"), element("e8", "button", "Cancel")],
      }),
      'two new buttons appeared: "Post", "Cancel"',
    ],
    [
      change({
        newElements: [element("e1", "button", "Post"), element("e2", "link", "Help")],
      }),
      'two new elements appeared: "Post", "Help"',
    ],
    [
      change({
        newElements: [
          element("e1", "button", "One"),
          element("e2", "button", "Two"),
          element("e3", "button", "Three"),
          element("e4", "button", "Four"),
        ],
      }),
      'four new buttons appeared: "One", "Two", "Three", and 1 more',
    ],
    [
      change({ dialog: { kind: "confirm", message: "Are you sure?" } }),
      'a dialog appeared saying "Are you sure?"',
    ],
    [change({ newTab: "t2" }), "a new tab opened"],
    [
      change({ download: { filename: "report.pdf", path: "/tmp/report.pdf" } }),
      "a download started: report.pdf",
    ],
    [change({ titleChanged: true, title: "Home" }), 'the title changed to "Home"'],
    [change({ removedCount: 3 }), "three elements went away"],
    [change({ removedCount: 1 }), "one element went away"],
  ];

  for (const [evidence, sentence] of sentences) {
    it(`says ${JSON.stringify(sentence)}`, () => {
      expect(describeChange(evidence)).toBe(sentence);
    });
  }

  it("reports the address before anything else, because it is the biggest change", () => {
    expect(
      describeChange(
        change({
          urlChanged: true,
          url: "https://example.com/home",
          newElements: [element("e1", "button", "Post")],
        }),
      ),
    ).toBe("the address changed to https://example.com/home");
  });

  it("reports a dialog before the address, because a dialog blocks the page", () => {
    expect(
      describeChange(
        change({ urlChanged: true, dialog: { kind: "alert", message: "Saved" } }),
      ),
    ).toBe('a dialog appeared saying "Saved"');
  });
});

describe("judging an action", () => {
  it("says nothing extra when the expectation was met", () => {
    expect(judge("the post button", change({ newElements: [element("e7", "button", "Post")] }))).toEqual({
      expectationMet: true,
      seen: "",
    });
  });

  it("says what was seen instead when the expectation was not met", () => {
    expect(judge("the timeline", change({ urlChanged: true, url: "https://example.com/home" }))).toEqual({
      expectationMet: false,
      seen: "the address changed to https://example.com/home",
    });
  });

  it("says nothing changed when the page did not move", () => {
    expect(judge("the timeline", nothing())).toEqual({
      expectationMet: false,
      seen: "nothing changed",
    });
  });
});
