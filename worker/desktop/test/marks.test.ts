import { describe, expect, test } from "vitest"
import {
  compareMarks,
  maximumMarks,
  maximumNameLength,
  middleOf,
  numberTheControls,
} from "../src/marks.js"
import type { DriverElement, Mark } from "../src/marks.js"

const element = (index: number, role: string, label: string, extra: Partial<DriverElement> = {}): DriverElement => ({
  element_index: index,
  element_token: `s00000001:${index}`,
  role,
  label,
  enabled: true,
  frame: { x: 10 * index, y: 20, w: 100, h: 30 },
  ...extra,
})

const fixtureWindow: DriverElement[] = [
  element(0, "dialog", "Coeus fixture window"),
  element(1, "text box", "Type here"),
  element(2, "button", "Cancel"),
  element(3, "button", "OK"),
  element(4, "scroll bar", "0.0"),
]

describe("numbering the controls of a window", () => {
  test("only the controls a person can act on are numbered, starting at one", () => {
    const numbered = numberTheControls(fixtureWindow)

    expect(numbered.controls.map((control) => control.mark)).toEqual([
      { number: 1, role: "text box", name: "Type here" },
      { number: 2, role: "button", name: "Cancel" },
      { number: 3, role: "button", name: "OK" },
    ])
    expect(numbered.hidden).toBe(0)
  })

  test("each numbered control keeps the driver's handle and its place on the screen", () => {
    const numbered = numberTheControls(fixtureWindow)

    expect(numbered.controls[0]?.token).toBe("s00000001:1")
    expect(numbered.controls[0]?.frame).toEqual({ x: 10, y: 20, w: 100, h: 30 })
  })

  test.each([
    ["a push button", "push button", true],
    ["a text box", "text box", true],
    ["a check box", "check box", true],
    ["a combo box", "combo box", true],
    ["a menu item", "menu item", true],
    ["a link", "link", true],
    ["a page tab", "page tab", true],
    ["a table cell", "table cell", true],
    ["a scroll bar", "scroll bar", false],
    ["a tool bar", "tool bar", false],
    ["a panel", "panel", false],
    ["a plain label", "label", false],
    ["the application itself", "application", false],
    ["a generic box", "generic", false],
  ])("%s is numbered: %s", (_name, role, wanted) => {
    const numbered = numberTheControls([element(1, role, "something")])

    expect(numbered.controls.length === 1).toBe(wanted)
  })

  test("a control that is switched off is not numbered, because it cannot be acted on", () => {
    const numbered = numberTheControls([element(1, "button", "Save", { enabled: false })])

    expect(numbered.controls).toEqual([])
  })

  test("a control with no place on the screen is not numbered", () => {
    const numbered = numberTheControls([element(1, "button", "Save", { frame: { x: 0, y: 0, w: 0, h: 0 } })])

    expect(numbered.controls).toEqual([])
  })

  test("a whole document as a label is trimmed to one short line", () => {
    const document = "a passphrase\nand a second line\n".repeat(40)
    const numbered = numberTheControls([element(1, "text box", document)])

    const name = numbered.controls[0]?.mark.name ?? ""
    expect(name.length).toBeLessThanOrEqual(maximumNameLength)
    expect(name).not.toContain("\n")
    expect(name.endsWith("...")).toBe(true)
  })

  test("a control with no label of its own falls back to its value and then to nothing", () => {
    const withValue = numberTheControls([element(1, "text box", "", { value: "seventeen" })])
    const withNeither = numberTheControls([element(1, "button", "")])

    expect(withValue.controls[0]?.mark.name).toBe("seventeen")
    expect(withNeither.controls[0]?.mark.name).toBe("")
  })

  test("more controls than the cap are cut off and counted as hidden", () => {
    const crowd = Array.from({ length: maximumMarks + 7 }, (_unused, index) => element(index + 1, "button", `Button ${index}`))

    const numbered = numberTheControls(crowd)

    expect(numbered.controls).toHaveLength(maximumMarks)
    expect(numbered.hidden).toBe(7)
  })

  test("a driver element with fields missing does not throw", () => {
    const numbered = numberTheControls([{ element_index: 1 } as DriverElement])

    expect(numbered.controls).toEqual([])
  })
})

describe("the middle of a control", () => {
  test("is the middle of its place on the screen, rounded to whole pixels", () => {
    expect(middleOf({ x: 10, y: 20, w: 101, h: 31 })).toEqual({ x: 60, y: 36 })
  })
})

describe("comparing the controls before and after an action", () => {
  const before: Mark[] = [
    { number: 1, role: "text box", name: "Type here" },
    { number: 2, role: "button", name: "Cancel" },
  ]

  test("a control that was not there before is new", () => {
    const after: Mark[] = [...before, { number: 3, role: "button", name: "Save" }]

    expect(compareMarks(before, after)).toEqual({ newMarks: [{ number: 3, role: "button", name: "Save" }], goneMarks: 0 })
  })

  test("a control that is no longer there is counted as gone", () => {
    const after: Mark[] = [{ number: 1, role: "text box", name: "Type here" }]

    expect(compareMarks(before, after)).toEqual({ newMarks: [], goneMarks: 1 })
  })

  test("renumbering alone is not a change", () => {
    const after: Mark[] = [
      { number: 7, role: "button", name: "Cancel" },
      { number: 8, role: "text box", name: "Type here" },
    ]

    expect(compareMarks(before, after)).toEqual({ newMarks: [], goneMarks: 0 })
  })

  test("two controls that look alike are counted one at a time", () => {
    const twoSaves: Mark[] = [
      { number: 1, role: "button", name: "Save" },
      { number: 2, role: "button", name: "Save" },
    ]

    expect(compareMarks([{ number: 1, role: "button", name: "Save" }], twoSaves)).toEqual({
      newMarks: [{ number: 2, role: "button", name: "Save" }],
      goneMarks: 0,
    })
  })
})
