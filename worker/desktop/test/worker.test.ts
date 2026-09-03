import { Readable, Writable } from "node:stream"
import { describe, expect, test } from "vitest"
import { pacingFromArguments, runWorker } from "../src/worker.js"
import { DesktopSession } from "../src/session.js"
import { DesktopErrorCode, ProtocolError } from "../src/wire.js"
import { FakeDriver, aWindow } from "./fakedriver.js"

/** collected is a stream that keeps every line written to it. */
function collected(): { stream: Writable; lines: () => string[] } {
  const chunks: string[] = []
  const stream = new Writable({
    write(chunk, _encoding, done) {
      chunks.push(String(chunk))
      done()
    },
  })
  return { stream, lines: () => chunks.join("").split("\n").filter((line) => line !== "") }
}

/** ran drives the worker over a fixed list of input lines and returns what it wrote. */
async function ran(lines: string[], argv: string[] = ["--pacing", "fast"]): Promise<{ out: string[]; errors: string[]; driver: FakeDriver }> {
  const driver = new FakeDriver()
  driver.launchable.set("zenity", aWindow())
  const output = collected()
  const errors = collected()
  await runWorker({
    argv,
    input: Readable.from(lines.map((line) => `${line}\n`)),
    output: output.stream,
    errors: errors.stream,
    openSession: (pacing, note) => new DesktopSession(driver, pacing, note),
  })
  return { out: output.lines(), errors: errors.lines(), driver }
}

describe("the worker process", () => {
  test("a good request is answered on standard output with the same id", async () => {
    const { out } = await ran(['{"jsonrpc":"2.0","id":1,"method":"health","params":{}}'])

    expect(out).toHaveLength(1)
    const answered = JSON.parse(out[0] as string)
    expect(answered.id).toBe(1)
    expect(answered.result.healthy).toBe(true)
  })

  test("requests are answered one per line, in the order they arrived", async () => {
    const { out } = await ran([
      '{"jsonrpc":"2.0","id":1,"method":"launch","params":{"application":"zenity"}}',
      '{"jsonrpc":"2.0","id":2,"method":"screenshot","params":{}}',
      '{"jsonrpc":"2.0","id":3,"method":"health","params":{}}',
    ])

    expect(out.map((line) => JSON.parse(line).id)).toEqual([1, 2, 3])
  })

  test("a line that is not JSON is answered with a parse error and no id", async () => {
    const { out } = await ran(["this is not JSON at all"])

    const answered = JSON.parse(out[0] as string)
    expect(answered.id).toBeNull()
    expect(answered.error.code).toBe(DesktopErrorCode.ParseError)
  })

  test("the worker keeps answering after a bad line", async () => {
    const { out } = await ran(["oops", '{"jsonrpc":"2.0","id":9,"method":"health","params":{}}'])

    expect(out).toHaveLength(2)
    expect(JSON.parse(out[1] as string).id).toBe(9)
  })

  test("a method that fails is answered with the protocol's own code", async () => {
    const { out } = await ran(['{"jsonrpc":"2.0","id":4,"method":"click","params":{"mark":1}}'])

    expect(JSON.parse(out[0] as string).error.code).toBe(DesktopErrorCode.NoApplicationOpen)
  })

  test("a screenshot before any launch is answered with a picture rather than refused", async () => {
    const { out } = await ran(['{"jsonrpc":"2.0","id":5,"method":"screenshot","params":{}}'])

    const answered = JSON.parse(out[0] as string)
    expect(answered.error).toBeUndefined()
    expect(answered.result.pngBase64).toBe("iVBORw0KGgoWHOLE")
    expect(answered.result.windows).toEqual(["Coeus fixture window"])
  })

  test("a failure that is not the protocol's own is still answered rather than thrown", async () => {
    const { out } = await ran([
      '{"jsonrpc":"2.0","id":1,"method":"launch","params":{"application":"zenity"}}',
      '{"jsonrpc":"2.0","id":2,"method":"screenshot","params":{}}',
    ].concat([]))

    expect(out.every((line) => "id" in JSON.parse(line))).toBe(true)
  })

  test("nothing but responses is written to standard output", async () => {
    const { out, errors } = await ran(['{"jsonrpc":"2.0","id":1,"method":"launch","params":{"application":"zenity"}}'])

    for (const line of out) {
      expect(JSON.parse(line).jsonrpc).toBe("2.0")
    }
    expect(errors.length).toBeGreaterThan(0)
  })

  test("the session is closed when standard input ends", async () => {
    const { driver } = await ran(['{"jsonrpc":"2.0","id":1,"method":"launch","params":{"application":"zenity"}}'])

    expect(driver.calls).toContain("close")
  })
})

describe("reading the command line", () => {
  test("no pacing at all means the human one, which is what the agent uses", () => {
    expect(pacingFromArguments([]).name).toBe("human")
  })

  test("the tests ask for the fast one", () => {
    expect(pacingFromArguments(["--pacing", "fast"]).name).toBe("fast")
  })

  test("a pacing with no value after it is refused", () => {
    expect(() => pacingFromArguments(["--pacing"])).toThrow(ProtocolError)
  })

  test("an argument nobody has heard of names itself", () => {
    try {
      pacingFromArguments(["--turbo"])
      throw new Error("the argument was accepted and it should not have been")
    } catch (failure) {
      expect((failure as ProtocolError).message).toContain("--turbo")
    }
  })
})
