import { Readable, Writable } from "node:stream"
import fc from "fast-check"
import { describe, expect, test } from "vitest"
import { judgeExpectation, meaningfulWords } from "../src/expectation.js"
import { parseKeyChord } from "../src/keys.js"
import { compareMarks, numberTheControls } from "../src/marks.js"
import { pacingNamed } from "../src/pacing.js"
import { DesktopSession } from "../src/session.js"
import { ProtocolError } from "../src/wire.js"
import { runWorker } from "../src/worker.js"
import { FakeDriver, aWindow } from "./fakedriver.js"

const runs = 200

describe("any expectation at all", () => {
  test("never throws, whatever the model wrote", () => {
    fc.assert(
      fc.property(fc.string(), fc.string(), (expectation, title) => {
        const judged = judgeExpectation(expectation, {
          titleChanged: true,
          title,
          newMarks: [{ number: 1, role: "button", name: title }],
          goneMarks: 0,
        })
        expect(typeof judged.expectationMet).toBe("boolean")
        expect(typeof judged.seen).toBe("string")
        expect(meaningfulWords(expectation).every((word) => word.length >= 4)).toBe(true)
      }),
      { numRuns: runs },
    )
  })
})

describe("any key combination at all", () => {
  test("is either read into a chord or refused with the protocol's own code", () => {
    fc.assert(
      fc.property(fc.string(), (written) => {
        try {
          const chord = parseKeyChord(written)
          expect(typeof chord.key).toBe("string")
          expect(chord.key.length).toBeGreaterThan(0)
        } catch (failure) {
          expect(failure).toBeInstanceOf(ProtocolError)
        }
      }),
      { numRuns: runs },
    )
  })
})

describe("any accessibility tree at all", () => {
  const anElement = fc.record(
    {
      element_index: fc.integer({ min: 0, max: 500 }),
      element_token: fc.string(),
      role: fc.string(),
      label: fc.string(),
      value: fc.string(),
      enabled: fc.boolean(),
      frame: fc.record({
        x: fc.integer({ min: -1000, max: 4000 }),
        y: fc.integer({ min: -1000, max: 4000 }),
        w: fc.integer({ min: -10, max: 4000 }),
        h: fc.integer({ min: -10, max: 4000 }),
      }),
    },
    { requiredKeys: ["element_index"] },
  )

  test("numbers its controls without throwing, and never past the cap", () => {
    fc.assert(
      fc.property(fc.array(anElement, { maxLength: 60 }), (elements) => {
        const numbered = numberTheControls(elements)
        expect(numbered.hidden).toBeGreaterThanOrEqual(0)
        numbered.controls.forEach((control, at) => {
          expect(control.mark.number).toBe(at + 1)
          expect(control.mark.name.length).toBeLessThanOrEqual(120)
        })
      }),
      { numRuns: runs },
    )
  })

  test("comparing two readings never counts more gone controls than there were", () => {
    const aMark = fc.record({ number: fc.integer({ min: 1, max: 50 }), role: fc.string(), name: fc.string() })
    fc.assert(
      fc.property(fc.array(aMark, { maxLength: 20 }), fc.array(aMark, { maxLength: 20 }), (before, after) => {
        const compared = compareMarks(before, after)
        expect(compared.goneMarks).toBeLessThanOrEqual(before.length)
        expect(compared.newMarks.length).toBeLessThanOrEqual(after.length)
      }),
      { numRuns: runs },
    )
  })
})

describe("any bytes on standard input", () => {
  /** answered runs the worker over one chunk of bytes and returns what it wrote. */
  async function answered(bytes: string): Promise<string[]> {
    const driver = new FakeDriver()
    driver.launchable.set("zenity", aWindow())
    const written: string[] = []
    const output = new Writable({
      write(chunk, _encoding, done) {
        written.push(String(chunk))
        done()
      },
    })
    const errors = new Writable({
      write(_chunk, _encoding, done) {
        done()
      },
    })
    await runWorker({
      argv: ["--pacing", "fast"],
      input: Readable.from([bytes]),
      output,
      errors,
      openSession: (pacing, note) => new DesktopSession(driver, pacing, note),
    })
    return written.join("").split("\n").filter((line) => line !== "")
  }

  test("produce a well-formed response or nothing at all, and never a crash", async () => {
    await fc.assert(
      fc.asyncProperty(fc.string({ maxLength: 400 }), async (bytes) => {
        for (const line of await answered(`${bytes}\n`)) {
          const answer = JSON.parse(line) as { jsonrpc: string; id: unknown; result?: unknown; error?: { code: number } }
          expect(answer.jsonrpc).toBe("2.0")
          expect("result" in answer || "error" in answer).toBe(true)
        }
      }),
      { numRuns: 60 },
    )
  })

  test("a request whose parameters are anything at all is still answered", async () => {
    await fc.assert(
      fc.asyncProperty(
        fc.constantFrom("launch", "click", "type", "press", "drag", "screenshot", "health", "clipboardSet", "nonsense"),
        fc.jsonValue(),
        async (method, params) => {
          const line = JSON.stringify({ jsonrpc: "2.0", id: 1, method, params })
          for (const answer of await answered(`${line}\n`)) {
            expect(JSON.parse(answer).id).toBe(1)
          }
        },
      ),
      { numRuns: 60 },
    )
  })
})

describe("any pacing", () => {
  test("every wait a profile names is a number that is not negative", () => {
    for (const name of ["human", "fast"]) {
      const pacing = pacingNamed(name)
      for (const [field, value] of Object.entries(pacing)) {
        if (field === "name") {
          continue
        }
        expect(typeof value).toBe("number")
        expect(value as number).toBeGreaterThanOrEqual(0)
      }
    }
  })
})
