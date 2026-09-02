import { describe, expect, test } from "vitest"
import {
  DesktopErrorCode,
  LineReader,
  ProtocolError,
  failureResponse,
  maximumLineBytes,
  readRequest,
  successResponse,
} from "../src/wire.js"

describe("reading one request", () => {
  test("a good request comes back with its id, its method, and its parameters", () => {
    const request = readRequest('{"jsonrpc":"2.0","id":3,"method":"click","params":{"mark":2}}')

    expect(request.id).toBe(3)
    expect(request.method).toBe("click")
    expect(request.params).toEqual({ mark: 2 })
  })

  test("a request with no parameters is read as a request with empty parameters", () => {
    const request = readRequest('{"jsonrpc":"2.0","id":1,"method":"health"}')

    expect(request.params).toEqual({})
  })

  test("a line that is not JSON is a parse error", () => {
    expect(() => readRequest("this is not JSON at all")).toThrow(ProtocolError)
    try {
      readRequest("this is not JSON at all")
    } catch (failure) {
      expect((failure as ProtocolError).code).toBe(DesktopErrorCode.ParseError)
    }
  })

  test.each([
    ["a line holding a bare number", "17"],
    ["a line holding an array", "[1,2,3]"],
    ["a request with the wrong protocol version", '{"jsonrpc":"1.0","id":1,"method":"health"}'],
    ["a request with no method", '{"jsonrpc":"2.0","id":1}'],
    ["a request whose method is not a name", '{"jsonrpc":"2.0","id":1,"method":7}'],
    ["a request whose parameters are a list", '{"jsonrpc":"2.0","id":1,"method":"health","params":[1]}'],
    ["a request with no id at all", '{"jsonrpc":"2.0","method":"health"}'],
  ])("%s is an invalid request", (_name, line) => {
    try {
      readRequest(line)
      throw new Error("the line was accepted and it should not have been")
    } catch (failure) {
      expect(failure).toBeInstanceOf(ProtocolError)
      expect((failure as ProtocolError).code).toBe(DesktopErrorCode.InvalidRequest)
    }
  })

  test("an invalid request keeps no id, because a line that was not a request has none to echo", () => {
    try {
      readRequest("17")
    } catch (failure) {
      expect((failure as ProtocolError).requestID).toBeNull()
    }
  })
})

describe("writing one response", () => {
  test("a success is one line of JSON-RPC carrying the result", () => {
    const line = successResponse(4, { healthy: true })

    expect(JSON.parse(line)).toEqual({ jsonrpc: "2.0", id: 4, result: { healthy: true } })
    expect(line).not.toContain("\n")
  })

  test("a failure carries the code, the message, and the data the protocol promises", () => {
    const failure = new ProtocolError(DesktopErrorCode.NoSuchMark, "there is no control numbered 9 on the screen", {
      marks: [],
    })

    const answered = JSON.parse(failureResponse(2, failure))

    expect(answered.error.code).toBe(DesktopErrorCode.NoSuchMark)
    expect(answered.error.message).toContain("numbered 9")
    expect(answered.error.data).toEqual({ marks: [] })
    expect(answered.id).toBe(2)
  })

  test("a failure with no data leaves the data out rather than sending nothing", () => {
    const answered = JSON.parse(failureResponse(null, new ProtocolError(DesktopErrorCode.ParseError, "the line was not JSON")))

    expect(answered.id).toBeNull()
    expect("data" in answered.error).toBe(false)
  })
})

describe("splitting standard input into lines", () => {
  test("whole lines come out and a partial line waits for the rest", () => {
    const reader = new LineReader()

    expect(reader.push('{"a":1}\n{"b":2}\n{"c"')).toEqual(['{"a":1}', '{"b":2}'])
    expect(reader.push(":3}\n")).toEqual(['{"c":3}'])
  })

  test("a blank line is dropped rather than answered", () => {
    const reader = new LineReader()

    expect(reader.push("\n  \n" + '{"a":1}' + "\n")).toEqual(['{"a":1}'])
  })

  test("a carriage return at the end of a line is not part of the line", () => {
    const reader = new LineReader()

    expect(reader.push('{"a":1}\r\n')).toEqual(['{"a":1}'])
  })

  test("a line longer than the cap is refused and the rest of it is thrown away", () => {
    const reader = new LineReader()
    const tooLong = "x".repeat(maximumLineBytes + 10)

    expect(reader.push(tooLong)).toEqual([])
    expect(reader.overLongLines).toBe(1)
    expect(reader.push("still the same line\n" + '{"a":1}' + "\n")).toEqual(['{"a":1}'])
    expect(reader.overLongLines).toBe(1)
  })
})
