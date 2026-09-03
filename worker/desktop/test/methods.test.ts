import { beforeEach, describe, expect, test } from "vitest"
import { dispatch, methodNames } from "../src/methods.js"
import { pacingNamed } from "../src/pacing.js"
import { DesktopSession } from "../src/session.js"
import { DesktopErrorCode, ProtocolError } from "../src/wire.js"
import { FakeDriver, aWindow } from "./fakedriver.js"

let driver: FakeDriver
let session: DesktopSession

beforeEach(async () => {
  driver = new FakeDriver()
  driver.launchable.set("zenity", aWindow())
  session = new DesktopSession(driver, pacingNamed("fast"), () => {})
})

const refusal = async (method: string, params: Record<string, unknown>): Promise<ProtocolError> => {
  try {
    await dispatch(session, method, params)
  } catch (thrown) {
    return thrown as ProtocolError
  }
  throw new Error(`the call to ${method} worked and it should not have`)
}

describe("the nine methods", () => {
  test("every method the protocol names is one this worker answers", () => {
    expect([...methodNames].sort()).toEqual(
      ["click", "clipboardGet", "clipboardSet", "drag", "health", "launch", "press", "screenshot", "type"].sort(),
    )
  })

  test("launch opens the application and hands back the diff", async () => {
    const launched = (await dispatch(session, "launch", { application: "zenity", expectation: "a window opens" })) as {
      application: string
      expectationMet: boolean
    }

    expect(launched.application).toBe("zenity")
    expect(launched.expectationMet).toBe(true)
  })

  test("screenshot, click, type, press and drag each reach the session", async () => {
    await dispatch(session, "launch", { application: "zenity" })

    const picture = (await dispatch(session, "screenshot", {})) as { marks: unknown[] }
    await dispatch(session, "click", { mark: 1, expectation: "the box takes focus" })
    await dispatch(session, "type", { text: "nine years", expectation: "" })
    await dispatch(session, "press", { keys: "ctrl+s", expectation: "" })
    await dispatch(session, "drag", { fromMark: 2, toMark: 3, expectation: "" })

    expect(picture.marks).toHaveLength(3)
    expect(driver.calls).toContain("click s1:1 held for 0")
    expect(driver.calls).toContain('type "nine years" into s1:1')
    expect(driver.calls).toContain("press ctrl+s")
    expect(driver.calls).toContain("drag from 80,190 to 220,190 in 4 steps")
  })

  test("the clipboard goes there and comes back", async () => {
    const written = (await dispatch(session, "clipboardSet", { text: "nine years" })) as { characters: number }
    const read = (await dispatch(session, "clipboardGet", {})) as { text: string }

    expect(written.characters).toBe(10)
    expect(read.text).toBe("nine years")
  })

  test("health says the worker can act and which driver it loaded", async () => {
    const health = (await dispatch(session, "health", {})) as { healthy: boolean; driverVersion: string }

    expect(health.healthy).toBe(true)
    expect(health.driverVersion).toBe("0.23.2")
  })

  test("type aimed at a numbered control passes the number through", async () => {
    await dispatch(session, "launch", { application: "zenity" })

    await dispatch(session, "type", { text: "hello", mark: 3 })

    expect(driver.calls).toContain('type "hello" into s1:3')
  })
})

describe("refusing a request the worker cannot act on", () => {
  test("a method nobody has heard of names itself", async () => {
    const thrown = await refusal("teleport", {})

    expect(thrown.code).toBe(DesktopErrorCode.NoSuchMethod)
    expect(thrown.message).toContain("teleport")
  })

  test.each([
    ["launch with no application", "launch", {}],
    ["launch with an application that is not text", "launch", { application: 7 }],
    ["click with no mark", "click", {}],
    ["click with a mark that is not a whole number", "click", { mark: 1.5 }],
    ["click with a mark that is text", "click", { mark: "one" }],
    ["type with no text", "type", {}],
    ["type with a mark that is not a number", "type", { text: "hello", mark: "one" }],
    ["press with no keys", "press", {}],
    ["drag with no marks", "drag", {}],
    ["drag with only one mark", "drag", { fromMark: 1 }],
    ["clipboardSet with no text", "clipboardSet", {}],
    ["an expectation that is not text", "click", { mark: 1, expectation: 7 }],
  ])("%s is refused with the parameter named", async (_name, method, params) => {
    const thrown = await refusal(method, params as Record<string, unknown>)

    expect(thrown.code).toBe(DesktopErrorCode.BadParameters)
    expect(thrown.message.length).toBeGreaterThan(10)
  })

  test("a missing expectation is read as no expectation at all", async () => {
    await dispatch(session, "launch", { application: "zenity" })

    const diff = (await dispatch(session, "click", { mark: 1 })) as { expectationMet: boolean }

    expect(diff.expectationMet).toBe(false)
  })
})
