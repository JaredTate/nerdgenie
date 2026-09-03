import { beforeEach, describe, expect, test } from "vitest"
import { DesktopSession } from "../src/session.js"
import { maximumPictureLength, maximumTypedCharacters, maximumWindowsListed } from "../src/limits.js"
import { maximumNameLength } from "../src/marks.js"
import { pacingNamed } from "../src/pacing.js"
import { DesktopErrorCode, ProtocolError } from "../src/wire.js"
import { FakeDriver, aWindow } from "./fakedriver.js"

const fast = pacingNamed("fast")

let driver: FakeDriver
let session: DesktopSession

beforeEach(() => {
  driver = new FakeDriver()
  session = new DesktopSession(driver, fast, () => {})
})

/** opened launches the fixture window and hands back the session ready to act. */
async function opened(): Promise<void> {
  driver.launchable.set("zenity", aWindow())
  await session.launch("zenity", "a window opens")
  driver.calls.length = 0
}

const failure = async (run: () => Promise<unknown>): Promise<ProtocolError> => {
  try {
    await run()
  } catch (thrown) {
    return thrown as ProtocolError
  }
  throw new Error("the call worked and it should not have")
}

describe("acting before an application is open", () => {
  test.each([
    ["click", () => session.click(1, "something happens")],
    ["type", () => session.type("hello", undefined, "something happens")],
    ["press", () => session.press("ctrl+s", "something happens")],
    ["drag", () => session.drag(1, 2, "something happens")],
  ])("%s says no application is open", async (_name, run) => {
    const thrown = await failure(run)

    expect(thrown).toBeInstanceOf(ProtocolError)
    expect(thrown.code).toBe(DesktopErrorCode.NoApplicationOpen)
    expect(thrown.message).toContain("launch")
  })
})

describe("launching an application", () => {
  test("an application that is not open yet is launched and becomes the granted one", async () => {
    driver.launchable.set("zenity", aWindow())

    const launched = await session.launch("zenity", "a window opens")

    expect(driver.calls).toContain("launch zenity")
    expect(launched.application).toBe("zenity")
    expect(launched.title).toBe("Coeus fixture window")
    expect(launched.marks.map((mark) => mark.name)).toEqual(["Type here", "Cancel", "OK"])
    expect(launched.expectationMet).toBe(true)
  })

  test("an application that is already open is brought forward rather than opened twice", async () => {
    driver.windows = [aWindow()]

    await session.launch("zenity", "the window comes forward")

    expect(driver.calls).not.toContain("launch zenity")
    expect(driver.calls).toContain("bring window 77 to the front")
  })

  test("an application that resolves to nothing launchable names itself", async () => {
    const thrown = await failure(() => session.launch("nosuchprogram", "a window opens"))

    expect(thrown.code).toBe(DesktopErrorCode.LaunchFailed)
    expect(thrown.message).toContain("nosuchprogram")
  })

  test("a driver that will not answer at all is reported as unavailable", async () => {
    driver.brokenWith = "the native library is missing"

    const thrown = await failure(() => session.launch("zenity", "a window opens"))

    expect(thrown.code).toBe(DesktopErrorCode.DriverUnavailable)
    expect(thrown.message).toContain("native library")
  })

  test("the window that matches the application by name is chosen when there are several", async () => {
    driver.windows = [aWindow({ windowId: 5, application: "digibyte-qt", title: "somebody else" }), aWindow({ windowId: 9 })]

    const launched = await session.launch("zenity", "")

    expect(launched.title).toBe("Coeus fixture window")
    expect(driver.calls).toContain("bring window 9 to the front")
  })
})

describe("taking a screenshot with nothing open", () => {
  test("the whole screen is photographed, no control is numbered, and the windows are named by title", async () => {
    driver.windows = [aWindow(), aWindow({ windowId: 5, title: "DigiByte - Firefox", application: "firefox" })]

    const picture = await session.screenshot()

    expect(picture.pngBase64).toBe("iVBORw0KGgoWHOLE")
    expect(picture.marks).toEqual([])
    expect(picture.application).toBe("")
    expect(picture.windows).toEqual(["Coeus fixture window", "DigiByte - Firefox"])
    expect(driver.calls).toContain("read the whole screen")
    expect(driver.calls.some((call) => call.startsWith("read window") || call.startsWith("bring window"))).toBe(false)
  })

  test("a window with no title is named by its application, and one with neither is left out", async () => {
    driver.windows = [
      aWindow({ windowId: 5, title: "", application: "gedit" }),
      aWindow({ windowId: 6, title: "  ", application: "" }),
      aWindow({ windowId: 7, title: "  Notes  ", application: "gedit" }),
    ]

    const picture = await session.screenshot()

    expect(picture.windows).toEqual(["gedit", "Notes"])
  })

  test("no more windows than the cap are named, each on one short line", async () => {
    driver.windows = Array.from({ length: maximumWindowsListed + 25 }, (_, at) =>
      aWindow({ windowId: at, title: `Window ${at}\n${"x".repeat(300)}` }),
    )

    const picture = await session.screenshot()

    expect(picture.windows).toHaveLength(maximumWindowsListed)
    expect(picture.windows[0]).not.toContain("\n")
    expect((picture.windows[0] as string).length).toBeLessThanOrEqual(maximumNameLength)
  })

  test("a screen the driver cannot photograph whole still names the windows, with no picture and a logged reason", async () => {
    const logged: string[] = []
    const quiet = new DesktopSession(driver, fast, (line) => logged.push(line))
    driver.windows = [aWindow()]
    driver.screenBrokenWith = "X11 error BadMatch from GetImage"

    const picture = await quiet.screenshot()

    expect(picture.pngBase64).toBe("")
    expect(picture.windows).toEqual(["Coeus fixture window"])
    expect(logged.some((line) => line.includes("BadMatch"))).toBe(true)
  })

  test("a driver that will not answer at all is still reported as unavailable", async () => {
    driver.brokenWith = "the native library is missing"

    const thrown = await failure(() => session.screenshot())

    expect(thrown.code).toBe(DesktopErrorCode.DriverUnavailable)
  })

  test("a picture of the whole screen bigger than the cap is refused rather than sent", async () => {
    driver.screen = "x".repeat(maximumPictureLength + 1)

    const thrown = await failure(() => session.screenshot())

    expect(thrown.code).toBe(DesktopErrorCode.UnreadableWindow)
    expect(thrown.message).toContain("too big")
  })
})

describe("taking a screenshot", () => {
  test("the picture comes back with its controls numbered", async () => {
    await opened()

    const picture = await session.screenshot()

    expect(picture.pngBase64).toBe("iVBORw0KGgoFAKE")
    expect(picture.marks).toEqual([
      { number: 1, role: "text box", name: "Type here" },
      { number: 2, role: "button", name: "Cancel" },
      { number: 3, role: "button", name: "OK" },
    ])
    expect(picture.application).toBe("zenity")
    expect(picture.hidden).toBe(0)
  })

  test("the picture is of the granted window alone, and still names every window on the screen", async () => {
    await opened()
    driver.windows = [...driver.windows, aWindow({ windowId: 5, title: "DigiByte - Firefox", application: "firefox" })]

    const picture = await session.screenshot()

    expect(picture.windows).toEqual(["Coeus fixture window", "DigiByte - Firefox"])
    expect(driver.calls).not.toContain("read the whole screen")
    expect(driver.calls).toContain("read window 77 with a picture")
  })

  test("a picture bigger than the cap is refused rather than sent", async () => {
    await opened()
    driver.picture = "x".repeat(maximumPictureLength + 1)

    const thrown = await failure(() => session.screenshot())

    expect(thrown.code).toBe(DesktopErrorCode.UnreadableWindow)
    expect(thrown.message).toContain("too big")
  })
})

describe("clicking a numbered control", () => {
  test("the control is brought forward, clicked by its handle, and the window is read again", async () => {
    await opened()
    driver.afterAction = () => {
      driver.elements = driver.elements.filter((element) => element.label !== "Cancel")
    }

    const diff = await session.click(3, "the cancel button goes away")

    expect(driver.calls[0]).toBe("bring window 77 to the front")
    expect(driver.calls).toContain("click s1:3 held for 0")
    expect(diff.goneMarks).toBe(1)
    expect(diff.marks.map((mark) => mark.name)).toEqual(["Type here", "OK"])
  })

  test("an expectation that was not met says what happened instead", async () => {
    await opened()

    const diff = await session.click(3, "the printer starts")

    expect(diff.expectationMet).toBe(false)
    expect(diff.seen).toBe("nothing changed")
  })

  test("a number that is not on the screen is refused with a fresh screenshot", async () => {
    await opened()

    const thrown = await failure(() => session.click(9, "anything at all"))

    expect(thrown.code).toBe(DesktopErrorCode.NoSuchMark)
    expect(thrown.message).toContain("9")
    expect((thrown.data as { marks: unknown[] }).marks).toHaveLength(3)
  })

  test("a new control that appears is reported as new", async () => {
    await opened()
    driver.afterAction = () => {
      driver.elements = [...driver.elements, { element_index: 4, element_token: "s1:4", role: "button", label: "Save", enabled: true, frame: { x: 0, y: 0, w: 10, h: 10 } }]
    }

    const diff = await session.click(3, "a save button appears")

    expect(diff.newMarks.map((mark) => mark.name)).toEqual(["Save"])
    expect(diff.expectationMet).toBe(true)
  })
})

describe("typing", () => {
  test("the typing lands in the control the last click landed on", async () => {
    await opened()
    await session.click(1, "the box takes focus")

    await session.type("nine years of DigiByte", undefined, "the text box holds the post")

    expect(driver.calls).toContain('type "nine years of DigiByte" into s1:1')
  })

  test("a stated control is typed into instead", async () => {
    await opened()

    await session.type("hello", 3, "")

    expect(driver.calls).toContain('type "hello" into s1:3')
  })

  test("with nothing clicked yet the typing goes to whatever holds the focus", async () => {
    await opened()

    await session.type("hello", undefined, "")

    expect(driver.calls).toContain('type "hello" into the focused control')
  })

  test("human pacing types in several short runs", async () => {
    driver.launchable.set("zenity", aWindow())
    const human = new DesktopSession(driver, pacingNamed("human"), () => {})
    await human.launch("zenity", "")
    driver.calls.length = 0

    await human.type("nine years of DigiByte", 1, "")

    const runs = driver.calls.filter((call) => call.startsWith("type "))
    expect(runs.length).toBeGreaterThan(1)
    expect(runs.map((call) => JSON.parse(call.slice(5, call.lastIndexOf(" into")))).join("")).toBe("nine years of DigiByte")
  })

  test("text longer than the cap is refused", async () => {
    await opened()

    const thrown = await failure(() => session.type("x".repeat(maximumTypedCharacters + 1), undefined, ""))

    expect(thrown.code).toBe(DesktopErrorCode.BadParameters)
    expect(thrown.message).toContain(String(maximumTypedCharacters))
  })

  test("typing that changes nothing still meets an expectation about the control it aimed at", async () => {
    await opened()

    const diff = await session.type("nine years", 1, "the text box holds the post")

    expect(diff.expectationMet).toBe(true)
  })
})

describe("pressing a key combination", () => {
  test("the combination is read and pressed", async () => {
    await opened()

    await session.press("ctrl+shift+s", "a save dialog appears")

    expect(driver.calls).toContain("press ctrl+shift+s")
  })

  test("a combination that cannot be pressed is refused before anything happens", async () => {
    await opened()

    const thrown = await failure(() => session.press("ctrl+#", "anything"))

    expect(thrown.code).toBe(DesktopErrorCode.BadParameters)
    expect(driver.calls.filter((call) => call.startsWith("press "))).toHaveLength(0)
  })
})

describe("dragging from one control to another", () => {
  test("the drag runs from the middle of one to the middle of the other", async () => {
    await opened()

    await session.drag(2, 3, "the file moves")

    expect(driver.calls).toContain(`drag from 80,190 to 220,190 in ${fast.dragSteps} steps`)
  })

  test("a number that is not on the screen is refused and named", async () => {
    await opened()

    const thrown = await failure(() => session.drag(1, 9, "anything"))

    expect(thrown.code).toBe(DesktopErrorCode.NoSuchMark)
    expect(thrown.message).toContain("9")
  })
})

describe("the clipboard", () => {
  test("what is put on it comes back off it", async () => {
    const written = await session.clipboardSet("nine years of DigiByte")
    const read = await session.clipboardGet()

    expect(written.characters).toBe(22)
    expect(read.text).toBe("nine years of DigiByte")
  })

  test("the clipboard works with no application open, because it belongs to the machine", async () => {
    await expect(session.clipboardGet()).resolves.toEqual({ text: "" })
  })
})

describe("settling", () => {
  test("a window that never comes to rest is read as it stands and says so", async () => {
    await opened()
    driver.restless = true

    const diff = await session.click(3, "the dialog closes")

    expect(diff.settled).toBe(false)
    expect(diff.expectationMet).toBe(false)
    expect(diff.seen).toContain("kept changing")
  })
})

describe("saying whether the worker is healthy", () => {
  test("a working driver reports its version", async () => {
    const health = await session.health()

    expect(health.healthy).toBe(true)
    expect(health.driverVersion).toBe("0.23.2")
    expect(health.detail).toBe("")
  })

  test("a driver that cannot be reached says what is wrong", async () => {
    driver.brokenWith = "the native library is missing"

    const health = await session.health()

    expect(health.healthy).toBe(false)
    expect(health.detail).toContain("native library")
  })
})
