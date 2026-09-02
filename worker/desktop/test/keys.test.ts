import { describe, expect, test } from "vitest"
import { maximumChordParts, parseKeyChord } from "../src/keys.js"
import { DesktopErrorCode, ProtocolError } from "../src/wire.js"

describe("reading a key combination", () => {
  test.each([
    ["one modifier and a letter", "ctrl+s", { key: "s", modifiers: ["ctrl"] }],
    ["capital letters do not matter", "Control+Shift+S", { key: "s", modifiers: ["ctrl", "shift"] }],
    ["a named key on its own", "Enter", { key: "enter", modifiers: [] }],
    ["the return key is the enter key", "return", { key: "enter", modifiers: [] }],
    ["the escape key by its short name", "esc", { key: "escape", modifiers: [] }],
    ["a function key", "F5", { key: "f5", modifiers: [] }],
    ["the command key is the meta key on Linux", "cmd+a", { key: "a", modifiers: ["meta"] }],
    ["the windows key is the meta key too", "super+l", { key: "l", modifiers: ["meta"] }],
    ["spaces around the parts are ignored", " ctrl + shift + tab ", { key: "tab", modifiers: ["ctrl", "shift"] }],
    ["an arrow key", "alt+Left", { key: "left", modifiers: ["alt"] }],
  ])("%s", (_name, written, wanted) => {
    expect(parseKeyChord(written)).toEqual(wanted)
  })
})

describe("refusing a key combination that cannot be pressed", () => {
  const refusal = (written: string): ProtocolError => {
    try {
      parseKeyChord(written)
    } catch (failure) {
      return failure as ProtocolError
    }
    throw new Error(`the combination ${written} was accepted and it should not have been`)
  }

  test("an empty combination says so", () => {
    const failure = refusal("   ")

    expect(failure).toBeInstanceOf(ProtocolError)
    expect(failure.code).toBe(DesktopErrorCode.BadParameters)
    expect(failure.message).toContain("empty")
  })

  test("a combination that ends in a plus sign has no key", () => {
    expect(refusal("ctrl+").code).toBe(DesktopErrorCode.BadParameters)
  })

  test("a single punctuation mark is refused and the message says to type it instead", () => {
    const failure = refusal("ctrl+#")

    expect(failure.message).toContain("type")
  })

  test("a key nobody has heard of names itself", () => {
    expect(refusal("ctrl+banana").message).toContain("banana")
  })

  test("a modifier nobody has heard of names itself", () => {
    expect(refusal("flurb+s").message).toContain("flurb")
  })

  test("a combination with more parts than the cap is refused", () => {
    const tooMany = new Array(maximumChordParts + 1).fill("ctrl").concat("s").join("+")

    expect(refusal(tooMany).message).toContain(String(maximumChordParts))
  })
})
