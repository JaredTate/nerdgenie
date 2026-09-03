// The worker driving a real window on this machine's own display. The fixture
// is a zenity entry box these tests launch themselves and kill by its exact
// process id; no window the user opened is ever touched, and the picture is
// only ever taken of the fixture window.
//
// These tests need a display and the cua driver's native library. When either
// is missing they say so and are skipped, rather than failing a build on a
// machine that has no screen.
//
// They also need to be asked for by name. Driving the screen means talking to
// the accessibility bus the desktop publishes for screen readers, and on a
// machine where somebody is logged in that wakes the screen reader, which then
// speaks every window it is shown through the speakers. So these are live
// tests in the sense docs/WORK_PLAN.md gives the word, and they are gated the
// way the Go side gates `//go:build live`: they run only when
// COEUS_LIVE_DESKTOP is set to 1, and `npm test` on its own never runs them.

import { spawn, spawnSync, type ChildProcessByStdio } from "node:child_process"
import type { Readable } from "node:stream"
import { afterAll, beforeAll, describe, expect, test } from "vitest"
import { CuaDesktopDriver } from "../src/cuadriver.js"
import { pacingNamed } from "../src/pacing.js"
import { DesktopSession } from "../src/session.js"
import type { Mark } from "../src/marks.js"

/** The title the fixture window carries, which is how the worker finds it. */
const fixtureTitle = "Coeus desktop fixture"

/** haveZenity says whether the fixture application is on this machine. */
function haveZenity(): boolean {
  return spawnSync("sh", ["-c", "command -v zenity"], { encoding: "utf8" }).status === 0
}

/** Why these tests are not being run this time, or "" when they are. */
function whyNotRunning(): string {
  if (process.env["COEUS_LIVE_DESKTOP"] !== "1") {
    return "they drive this machine's real screen, which wakes the screen reader, so they run only when COEUS_LIVE_DESKTOP=1 asks for them"
  }
  if (!process.env["DISPLAY"]) {
    return "there is no display to drive"
  }
  if (!haveZenity()) {
    return "zenity is not installed"
  }
  return ""
}

const missing = whyNotRunning()
if (missing !== "") {
  console.warn(`the fixture-window tests are skipped because ${missing}`)
}

describe.skipIf(missing !== "")("driving a real fixture window", () => {
  let fixture: ChildProcessByStdio<null, Readable, Readable>
  let printed = ""
  let session: DesktopSession
  let marks: Mark[] = []

  /** numbered finds the number of the control with that label. */
  const numbered = (name: string): number => {
    const found = marks.find((mark) => mark.name === name)
    if (!found) {
      throw new Error(`the fixture window has no control called ${name}, only ${marks.map((mark) => mark.name).join(", ")}`)
    }
    return found.number
  }

  beforeAll(async () => {
    fixture = spawn("zenity", ["--entry", `--title=${fixtureTitle}`, "--text=Type here"], {
      env: { ...process.env, GDK_BACKEND: "x11" },
      stdio: ["ignore", "pipe", "pipe"],
    })
    fixture.stdout.on("data", (chunk: Buffer) => {
      printed += String(chunk)
    })
    session = new DesktopSession(new CuaDesktopDriver(() => {}), pacingNamed("fast"), () => {})
    await new Promise((ready) => setTimeout(ready, 2_500))
  })

  afterAll(async () => {
    await session.close().catch(() => undefined)
    if (fixture.pid !== undefined && fixture.exitCode === null) {
      process.kill(fixture.pid, "SIGTERM")
    }
  })

  test("the worker says it is healthy and names the driver and the display", async () => {
    const health = await session.health()

    expect(health.healthy).toBe(true)
    expect(health.driverVersion).not.toBe("")
    expect(health.display).toMatch(/x11|wayland/u)
  })

  test("a screenshot before any launch is a real picture of the whole screen that names the fixture window", async () => {
    const picture = await session.screenshot()

    expect(picture.application).toBe("")
    expect(picture.marks).toEqual([])
    expect(picture.pngBase64.length).toBeGreaterThan(1_000)
    expect(Buffer.from(picture.pngBase64.slice(0, 12), "base64").subarray(1, 4).toString()).toBe("PNG")
    expect(picture.windows.some((title) => title.includes(fixtureTitle))).toBe(true)
  })

  test("launch finds the fixture window that is already open and brings it forward", async () => {
    const launched = await session.launch(fixtureTitle, "a window with a text box opens")

    expect(launched.title).toContain(fixtureTitle)
    expect(launched.expectationMet).toBe(true)
    marks = launched.marks
    expect(marks.map((mark) => mark.name)).toContain("OK")
    expect(marks.map((mark) => mark.role)).toContain("text box")
  })

  test("a screenshot of the fixture window is a real picture with its controls numbered", async () => {
    const picture = await session.screenshot()

    expect(picture.application).toBe(fixtureTitle)
    expect(picture.title).toContain(fixtureTitle)
    expect(picture.pngBase64.length).toBeGreaterThan(1_000)
    expect(Buffer.from(picture.pngBase64.slice(0, 12), "base64").subarray(1, 4).toString()).toBe("PNG")
    expect(picture.marks.length).toBeGreaterThanOrEqual(3)
    marks = picture.marks
  })

  test("clicking the text box lands", async () => {
    const diff = await session.click(numbered("Type here"), "the text box takes the typing")

    expect(diff.settled).toBe(true)
    expect(diff.expectationMet).toBe(true)
  })

  test("typing puts the text in the box", async () => {
    const diff = await session.type("nine years of DigiByte", undefined, "the text box holds the post")

    expect(diff.expectationMet).toBe(true)
  })

  test("an expectation that is not met is reported rather than guessed at", async () => {
    const diff = await session.type("", numbered("Type here"), "the printer starts")

    expect(diff.expectationMet).toBe(false)
    expect(diff.seen).toBe("nothing changed")
  })

  test("a key combination is pressed without throwing", async () => {
    const diff = await session.press("ctrl+a", "the text is selected")

    expect(typeof diff.expectationMet).toBe("boolean")
    expect(diff.settled).toBe(true)
  })

  test("a drag runs from one control to another", async () => {
    const diff = await session.drag(numbered("Cancel"), numbered("OK"), "nothing in particular")

    expect(diff.settled).toBe(true)
  })

  test("a number that is not on the screen is refused with a fresh list of controls", async () => {
    await expect(session.click(999, "anything at all")).rejects.toThrow(/999/u)
  })

  test("the clipboard takes what is put on it, and the user's own is put back", async () => {
    const theirs = await session.clipboardGet()

    await session.clipboardSet("nine years of DigiByte")
    const ours = await session.clipboardGet()
    await session.clipboardSet(theirs.text)

    expect(ours.text).toBe("nine years of DigiByte")
  })

  test("clicking OK closes the window, and the fixture reports what was typed into it", async () => {
    await session.click(numbered("OK"), "the dialog closes").catch(() => undefined)
    await new Promise((ready) => setTimeout(ready, 1_500))

    expect(printed.trim()).toBe("nine years of DigiByte")
  })
})
