import { describe, expect, it } from "vitest";
import { buildDiff, markNewElements } from "../src/diff.js";
import type { Snapshot, SnapshotElement } from "../src/types.js";

function element(ref: string, role: string, name: string): SnapshotElement {
  return { ref, role, name };
}

function snapshot(part: Partial<Snapshot>): Snapshot {
  return {
    url: "https://example.com/start",
    title: "Start",
    tabId: "t1",
    elements: [],
    belowFold: 0,
    dialog: null,
    download: null,
    ...part,
  };
}

describe("marking the elements that were not there before", () => {
  it("marks nothing on the very first snapshot, because everything would be new", () => {
    const marked = markNewElements(null, [element("e1", "button", "Post")]);
    expect(marked).toEqual([{ ref: "e1", role: "button", name: "Post" }]);
  });

  it("marks an element whose ref was not in the snapshot before", () => {
    const before = [element("e1", "button", "Post")];
    const marked = markNewElements(before, [element("e1", "button", "Post"), element("e2", "link", "Help")]);
    expect(marked).toEqual([
      { ref: "e1", role: "button", name: "Post" },
      { ref: "e2", role: "link", name: "Help", new: true },
    ]);
  });

  it("does not mark an element that only changed its name, because it is the same element", () => {
    const before = [element("e1", "button", "Post")];
    const marked = markNewElements(before, [element("e1", "button", "Posting...")]);
    expect(marked[0]).toEqual({ ref: "e1", role: "button", name: "Posting..." });
  });

  it("leaves the elements it was given alone", () => {
    const current = [element("e2", "link", "Help")];
    markNewElements([], current);
    expect(current[0]).toEqual({ ref: "e2", role: "link", name: "Help" });
  });
});

describe("building the diff an action returns", () => {
  it("holds every field the protocol names, even when nothing happened", () => {
    const after = snapshot({});
    const diff = buildDiff({
      before: snapshot({}),
      after,
      expectation: "",
      newTab: "",
      wall: null,
    });
    expect(diff).toEqual({
      urlChanged: false,
      url: "https://example.com/start",
      newElements: [],
      dialog: null,
      newTab: "",
      download: null,
      expectationMet: false,
      seen: "nothing changed",
      wall: null,
      snapshot: after,
    });
  });

  it("says the address changed and carries the new one", () => {
    const diff = buildDiff({
      before: snapshot({}),
      after: snapshot({ url: "https://example.com/home", title: "Home" }),
      expectation: "the home screen",
      newTab: "",
      wall: null,
    });
    expect(diff.urlChanged).toBe(true);
    expect(diff.url).toBe("https://example.com/home");
    expect(diff.expectationMet).toBe(true);
  });

  it("collects the elements that are marked new", () => {
    const after = snapshot({
      elements: [element("e1", "button", "Post"), { ...element("e2", "link", "Help"), new: true }],
    });
    const diff = buildDiff({ before: snapshot({}), after, expectation: "", newTab: "", wall: null });
    expect(diff.newElements).toEqual([{ ref: "e2", role: "link", name: "Help", new: true }]);
  });

  it("passes a dialog and a download straight through from the snapshot", () => {
    const after = snapshot({
      dialog: { kind: "confirm", message: "Are you sure?" },
      download: { filename: "report.pdf", path: "/tmp/report.pdf" },
    });
    const diff = buildDiff({ before: snapshot({}), after, expectation: "", newTab: "", wall: null });
    expect(diff.dialog).toEqual({ kind: "confirm", message: "Are you sure?" });
    expect(diff.download).toEqual({ filename: "report.pdf", path: "/tmp/report.pdf" });
  });

  it("reports a new tab", () => {
    const diff = buildDiff({
      before: snapshot({}),
      after: snapshot({}),
      expectation: "",
      newTab: "t2",
      wall: null,
    });
    expect(diff.newTab).toBe("t2");
    expect(diff.expectationMet).toBe(true);
  });

  it("counts an element that went away as a change", () => {
    const diff = buildDiff({
      before: snapshot({ elements: [element("e1", "button", "Post")] }),
      after: snapshot({}),
      expectation: "",
      newTab: "",
      wall: null,
    });
    expect(diff.expectationMet).toBe(true);
    expect(diff.seen).toBe("");
  });

  it("treats a missing earlier snapshot as no change of address", () => {
    const diff = buildDiff({
      before: null,
      after: snapshot({ url: "https://example.com/home" }),
      expectation: "",
      newTab: "",
      wall: null,
    });
    expect(diff.urlChanged).toBe(false);
  });
});

describe("a diff that ran into a wall", () => {
  const wall = { kind: "captcha", detail: "a frame at https://example.com/captcha" } as const;

  it("fills in the wall, refuses the expectation, and says what stopped it", () => {
    const diff = buildDiff({
      before: snapshot({}),
      after: snapshot({ url: "https://example.com/captcha", title: "Captcha" }),
      expectation: "the captcha page",
      newTab: "",
      wall,
    });
    expect(diff.wall).toEqual(wall);
    expect(diff.expectationMet).toBe(false);
    expect(diff.seen).toBe(
      "the browser hit a captcha wall: a frame at https://example.com/captcha",
    );
  });
});
