import { describe, expect, test } from "vitest"
import { describeChange, judgeExpectation, meaningfulWords } from "../src/expectation.js"
import type { Mark } from "../src/marks.js"

const mark = (number: number, role: string, name: string): Mark => ({ number, role, name })

const nothingChanged = {
  titleChanged: false,
  title: "Coeus fixture window",
  newMarks: [] as Mark[],
  goneMarks: 0,
}

describe("picking the words that carry meaning", () => {
  test("short words and stop words are left out", () => {
    expect(meaningfulWords("the save button will appear in the window")).toEqual(["save", "button", "appear", "window"])
  })

  test("capital letters and punctuation do not change the words", () => {
    expect(meaningfulWords("A Save Dialog appears!")).toEqual(["save", "dialog", "appears"])
  })

  test("an expectation of nothing but stop words has no words in it", () => {
    expect(meaningfulWords("that will have been")).toEqual([])
  })
})

describe("judging an expectation", () => {
  test("a word of the expectation in a new control's name means it was met", () => {
    const judged = judgeExpectation("a save dialog appears", {
      ...nothingChanged,
      newMarks: [mark(5, "button", "Save")],
    })

    expect(judged.expectationMet).toBe(true)
    expect(judged.seen).toBe("")
  })

  test("a word of the expectation in a new control's role means it was met", () => {
    const judged = judgeExpectation("a button appears", { ...nothingChanged, newMarks: [mark(5, "button", "OK")] })

    expect(judged.expectationMet).toBe(true)
  })

  test("a word of the expectation in the new window title means it was met", () => {
    const judged = judgeExpectation("the document is unsaved", {
      ...nothingChanged,
      titleChanged: true,
      title: "Unsaved Document",
    })

    expect(judged.expectationMet).toBe(true)
  })

  test("a word of the expectation in the control the action was aimed at means it was met", () => {
    const judged = judgeExpectation("the text box holds the post", {
      ...nothingChanged,
      aimedAt: mark(1, "text box", "Type here"),
    })

    expect(judged.expectationMet).toBe(true)
  })

  test("an empty expectation is met when something changed", () => {
    const judged = judgeExpectation("", { ...nothingChanged, newMarks: [mark(2, "button", "Save")] })

    expect(judged.expectationMet).toBe(true)
  })

  test("an empty expectation is not met when nothing changed", () => {
    const judged = judgeExpectation("   ", nothingChanged)

    expect(judged.expectationMet).toBe(false)
    expect(judged.seen).toBe("nothing changed")
  })

  test("an expectation that was not met says what happened instead", () => {
    const judged = judgeExpectation("the printer starts", {
      ...nothingChanged,
      newMarks: [mark(5, "button", "Save"), mark(6, "button", "Cancel")],
    })

    expect(judged.expectationMet).toBe(false)
    expect(judged.seen).toBe("two new controls appeared: Save, Cancel")
  })

  test("an expectation made only of stop words is judged as an empty one", () => {
    const judged = judgeExpectation("that will have been", { ...nothingChanged, titleChanged: true, title: "Done" })

    expect(judged.expectationMet).toBe(true)
  })

  test("a window that never settled is not met and says the window kept changing", () => {
    const judged = judgeExpectation("a save dialog appears", { ...nothingChanged, settled: false })

    expect(judged.expectationMet).toBe(false)
    expect(judged.seen).toContain("kept changing")
  })
})

describe("saying what changed", () => {
  test("nothing at all", () => {
    expect(describeChange(nothingChanged)).toBe("nothing changed")
  })

  test("a new title", () => {
    expect(describeChange({ ...nothingChanged, titleChanged: true, title: "Unsaved Document" })).toBe(
      "the title changed to Unsaved Document",
    )
  })

  test("one new control", () => {
    expect(describeChange({ ...nothingChanged, newMarks: [mark(3, "button", "Save")] })).toBe(
      "one new control appeared: Save",
    )
  })

  test("controls that went away", () => {
    expect(describeChange({ ...nothingChanged, goneMarks: 3 })).toBe("three controls went away")
  })

  test("a title and new controls together", () => {
    const said = describeChange({
      ...nothingChanged,
      titleChanged: true,
      title: "Save As",
      newMarks: [mark(3, "button", "Save")],
    })

    expect(said).toBe("the title changed to Save As and one new control appeared: Save")
  })

  test("a long list of new controls names only the first few", () => {
    const many = [1, 2, 3, 4, 5, 6, 7].map((number) => mark(number, "button", `Button ${number}`))

    expect(describeChange({ ...nothingChanged, newMarks: many })).toBe(
      "7 new controls appeared: Button 1, Button 2, Button 3, Button 4, Button 5 and 2 more",
    )
  })
})
