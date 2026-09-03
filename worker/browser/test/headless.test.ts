import { describe, expect, it } from "vitest";
import { launchArguments } from "../src/chrome.js";

// Rule two in worker/browser/PROTOCOL.md says the window is visible on the
// machine's own display, with one exception: the project's own tests pass
// --headless through `make test-browser`, so that a test run puts no Chrome
// window on the screen of whoever is running it. These tests hold both halves of
// that rule. The command line that carries the option is read in src/main.ts,
// which cannot be imported here because importing it starts a worker, so it is
// covered by test/process.test.ts, which runs the built program.
describe("the headless option the tests pass", () => {
  it("puts --headless=new in Chrome's own arguments only when it is on", () => {
    expect(launchArguments("/tmp/a-profile")).not.toContain("--headless=new");
    expect(launchArguments("/tmp/a-profile", false)).not.toContain("--headless=new");
    expect(launchArguments("/tmp/a-profile", true)).toContain("--headless=new");
  });

  it("leaves every other flag exactly as it was", () => {
    const visible = launchArguments("/tmp/a-profile", false);
    const hidden = launchArguments("/tmp/a-profile", true);
    expect(hidden.filter((flag) => flag !== "--headless=new" && flag !== "--disable-gpu")).toEqual(visible);
  });
});
