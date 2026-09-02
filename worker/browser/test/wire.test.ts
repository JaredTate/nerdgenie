import { describe, expect, it } from "vitest";
import { formatResponse, parseLine } from "../src/wire.js";
import { MAX_LINE_BYTES } from "../src/limits.js";

// Every case here is a golden response: the exact object the Go side will read.
// The codes come from the error table in worker/browser/PROTOCOL.md.
describe("parsing one line from standard input", () => {
  it("accepts a good request and hands back the method and the parameters", () => {
    const parsed = parseLine('{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"https://example.com/"}}');
    expect(parsed).toEqual({
      kind: "request",
      request: { id: 1, method: "open", params: { url: "https://example.com/" } },
    });
  });

  it("treats a missing params object as an empty one", () => {
    const parsed = parseLine('{"jsonrpc":"2.0","id":11,"method":"health"}');
    expect(parsed).toEqual({ kind: "request", request: { id: 11, method: "health", params: {} } });
  });

  it("answers a line that is not JSON with -32700 and no id", () => {
    expect(parseLine("this is not JSON")).toEqual({
      kind: "response",
      response: {
        jsonrpc: "2.0",
        id: null,
        error: {
          code: -32700,
          message: "The line was not JSON. Send one JSON-RPC request object per line.",
        },
      },
    });
  });

  it("answers a line longer than the cap with -32700", () => {
    const tooLong = `{"jsonrpc":"2.0","id":1,"method":"health","params":{"pad":"${"x".repeat(MAX_LINE_BYTES)}"}}`;
    expect(parseLine(tooLong)).toEqual({
      kind: "response",
      response: {
        jsonrpc: "2.0",
        id: null,
        error: {
          code: -32700,
          message: `The line was longer than ${MAX_LINE_BYTES} bytes. Send one JSON-RPC request object per line.`,
        },
      },
    });
  });

  const notValidRequests: Array<[string, string]> = [
    ["a JSON array", "[1,2,3]"],
    ["a JSON number", "7"],
    ["a JSON string", '"open"'],
    ["JSON null", "null"],
    ["the wrong protocol version", '{"jsonrpc":"1.0","id":1,"method":"health"}'],
    ["a missing id", '{"jsonrpc":"2.0","method":"health"}'],
    ["an id that is not a number", '{"jsonrpc":"2.0","id":"one","method":"health"}'],
    ["an id that is not a whole number", '{"jsonrpc":"2.0","id":1.5,"method":"health"}'],
    ["a missing method", '{"jsonrpc":"2.0","id":1}'],
    ["a method that is not a string", '{"jsonrpc":"2.0","id":1,"method":7}'],
    ["params that are not an object", '{"jsonrpc":"2.0","id":1,"method":"health","params":[]}'],
  ];

  for (const [description, line] of notValidRequests) {
    it(`answers ${description} with -32600`, () => {
      expect(parseLine(line)).toEqual({
        kind: "response",
        response: {
          jsonrpc: "2.0",
          id: null,
          error: {
            code: -32600,
            message:
              'The request was not a valid JSON-RPC request. It needs jsonrpc "2.0", a whole number id, a method name, and params as an object.',
          },
        },
      });
    });
  }

  it("answers an unknown method with -32601 and lists the methods", () => {
    expect(parseLine('{"jsonrpc":"2.0","id":4,"method":"fly","params":{}}')).toEqual({
      kind: "response",
      response: {
        jsonrpc: "2.0",
        id: 4,
        error: {
          code: -32601,
          message:
            'There is no method named "fly". The methods are open, read, click, type, press, scroll, act, tabs, loginFill, screenshot, and health.',
        },
      },
    });
  });
});

describe("checking the parameters of each method", () => {
  const wrongParameters: Array<[string, string, string]> = [
    [
      "open without a url",
      '{"jsonrpc":"2.0","id":1,"method":"open","params":{}}',
      'The open method needs a url, such as "https://example.com/".',
    ],
    [
      "open with a url that is not http or https",
      '{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"file:///etc/passwd"}}',
      "The open method only opens http and https addresses, and this one was file:///etc/passwd.",
    ],
    [
      "read with visibleOnly that is not a true or false",
      '{"jsonrpc":"2.0","id":2,"method":"read","params":{"visibleOnly":"yes"}}',
      "The read method needs visibleOnly to be true or false.",
    ],
    [
      "click without a ref",
      '{"jsonrpc":"2.0","id":3,"method":"click","params":{}}',
      'The click method needs a ref, such as "e7".',
    ],
    [
      "click with an expectation that is not text",
      '{"jsonrpc":"2.0","id":3,"method":"click","params":{"ref":"e7","expectation":9}}',
      "The click method needs the expectation to be text.",
    ],
    [
      "type without text",
      '{"jsonrpc":"2.0","id":4,"method":"type","params":{"ref":"e3"}}',
      "The type method needs the text to type.",
    ],
    [
      "type with text past the cap",
      `{"jsonrpc":"2.0","id":4,"method":"type","params":{"ref":"e3","text":"${"a".repeat(10_001)}"}}`,
      "The type method takes at most 10000 characters of text, and this was 10001.",
    ],
    [
      "press without a key",
      '{"jsonrpc":"2.0","id":5,"method":"press","params":{}}',
      'The press method needs a key, such as "Enter" or "Control+s".',
    ],
    [
      "scroll in a direction that is neither up nor down",
      '{"jsonrpc":"2.0","id":6,"method":"scroll","params":{"direction":"sideways"}}',
      'The scroll method needs a direction of "up" or "down", and this was "sideways".',
    ],
    [
      "scroll with an amount past the cap",
      '{"jsonrpc":"2.0","id":6,"method":"scroll","params":{"direction":"down","amount":99}}',
      "The scroll method takes an amount from 1 to 10 steps, and this was 99.",
    ],
    [
      "act without steps",
      '{"jsonrpc":"2.0","id":7,"method":"act","params":{}}',
      "The act method needs a steps list holding 1 to 10 steps.",
    ],
    [
      "act with more steps than the cap",
      `{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[${'{"method":"press","key":"Enter"},'.repeat(10)}{"method":"press","key":"Enter"}]}}`,
      "The act method needs a steps list holding 1 to 10 steps.",
    ],
    [
      "act with a step whose method cannot be batched",
      '{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[{"method":"open","url":"https://example.com/"}]}}',
      'Step 1 of the act method used the method "open", and a step may only be click, type, press, or scroll.',
    ],
    [
      "act with a step that is missing its own parameters",
      '{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[{"method":"click"}]}}',
      'Step 1 of the act method: The click method needs a ref, such as "e7".',
    ],
    [
      "tabs with an unknown action",
      '{"jsonrpc":"2.0","id":8,"method":"tabs","params":{"action":"reorder"}}',
      'The tabs method needs an action of "list", "switch", or "close", and this was "reorder".',
    ],
    [
      "tabs asking to switch without saying which tab",
      '{"jsonrpc":"2.0","id":8,"method":"tabs","params":{"action":"switch"}}',
      'The tabs method needs a tabId, such as "t2", to switch or close a tab.',
    ],
    [
      "loginFill without a passwordRef",
      '{"jsonrpc":"2.0","id":9,"method":"loginFill","params":{"usernameRef":"e2","username":"someone","password":"secret"}}',
      'The loginFill method needs a passwordRef, such as "e3".',
    ],
    [
      "loginFill with a code but no codeRef to put it in",
      '{"jsonrpc":"2.0","id":9,"method":"loginFill","params":{"usernameRef":"e2","passwordRef":"e3","username":"someone","password":"secret","code":"123456"}}',
      "The loginFill method was given a code but no codeRef saying which field to type it into.",
    ],
  ];

  for (const [description, line, message] of wrongParameters) {
    it(`answers ${description} with -32602`, () => {
      const parsed = parseLine(line);
      expect(parsed.kind).toBe("response");
      expect(parsed).toMatchObject({
        response: { jsonrpc: "2.0", error: { code: -32602, message } },
      });
    });
  }

  it("accepts a full act batch", () => {
    const parsed = parseLine(
      '{"jsonrpc":"2.0","id":7,"method":"act","params":{"steps":[{"method":"click","ref":"e3","expectation":"the box takes focus"},{"method":"type","ref":"e3","text":"Nine years of DigiByte."}]}}',
    );
    expect(parsed.kind).toBe("request");
  });

  it("accepts loginFill without a code", () => {
    const parsed = parseLine(
      '{"jsonrpc":"2.0","id":9,"method":"loginFill","params":{"usernameRef":"e2","passwordRef":"e3","username":"someone","password":"secret"}}',
    );
    expect(parsed.kind).toBe("request");
  });
});

describe("writing one response per line", () => {
  it("ends the line with a newline and holds no newline inside it", () => {
    const line = formatResponse({ jsonrpc: "2.0", id: 1, result: { healthy: true } });
    expect(line.endsWith("\n")).toBe(true);
    expect(line.slice(0, -1)).not.toContain("\n");
    expect(JSON.parse(line)).toEqual({ jsonrpc: "2.0", id: 1, result: { healthy: true } });
  });

  it("escapes a newline that came from the page rather than breaking the line", () => {
    const line = formatResponse({ jsonrpc: "2.0", id: 1, result: { title: "one\ntwo" } });
    expect(line.split("\n")).toHaveLength(2);
    expect(JSON.parse(line).result.title).toBe("one\ntwo");
  });
});
