import { describe, expect, test } from "vitest"
import { pacingNamed, typingRuns, variedGap } from "../src/pacing.js"
import { DesktopErrorCode, ProtocolError } from "../src/wire.js"

describe("choosing a pacing", () => {
  test("human pacing waits and fast pacing does not", () => {
    const human = pacingNamed("human")
    const fast = pacingNamed("fast")

    expect(human.name).toBe("human")
    expect(human.betweenActions).toBeGreaterThan(fast.betweenActions)
    expect(human.clickHold).toBeGreaterThan(fast.clickHold)
    expect(human.typingGap).toBeGreaterThan(fast.typingGap)
    expect(human.dragSteps).toBeGreaterThan(fast.dragSteps)
  })

  test("a pacing nobody has heard of is refused and names itself", () => {
    try {
      pacingNamed("glacial")
      throw new Error("the pacing was accepted and it should not have been")
    } catch (failure) {
      expect(failure).toBeInstanceOf(ProtocolError)
      expect((failure as ProtocolError).code).toBe(DesktopErrorCode.BadParameters)
      expect((failure as ProtocolError).message).toContain("glacial")
    }
  })
})

describe("breaking text into runs of typing", () => {
  test("human pacing types in short runs, the way a person does", () => {
    const runs = typingRuns("nine years of DigiByte", pacingNamed("human"))

    expect(runs.join("")).toBe("nine years of DigiByte")
    expect(runs.length).toBeGreaterThan(1)
    for (const run of runs) {
      expect(run.length).toBeLessThanOrEqual(pacingNamed("human").typingRun)
    }
  })

  test("fast pacing types the whole thing at once", () => {
    expect(typingRuns("nine years of DigiByte", pacingNamed("fast"))).toEqual(["nine years of DigiByte"])
  })

  test("empty text is no runs at all", () => {
    expect(typingRuns("", pacingNamed("human"))).toEqual([])
  })
})

describe("the gap between two runs of typing", () => {
  test("under human pacing it varies from one run to the next", () => {
    const human = pacingNamed("human")
    const draws = [0, 0.25, 0.5, 0.75, 1]

    const gaps = draws.map((draw) => variedGap(human, () => draw))

    expect(new Set(gaps).size).toBe(draws.length)
    for (const gap of gaps) {
      expect(gap).toBeGreaterThan(0)
      expect(gap).toBeLessThanOrEqual(human.typingGap * 2)
    }
  })

  test("under fast pacing there is no gap to vary", () => {
    expect(variedGap(pacingNamed("fast"), () => 0.5)).toBe(0)
  })
})
