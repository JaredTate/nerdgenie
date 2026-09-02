import { describe, expect, it } from "vitest";
import fc from "fast-check";
import { ERROR_CODES } from "../src/errors.js";
import { describeChange, judge, meaningfulWords, type Change } from "../src/expectation.js";
import { createLineReader } from "../src/lines.js";
import { REDACTED, redactDeep, redactText } from "../src/redact.js";
import { findWall } from "../src/walls.js";
import { formatResponse, parseLine } from "../src/wire.js";

const KNOWN_CODES: number[] = Object.values(ERROR_CODES);

describe("anything at all arriving on standard input", () => {
  it("always produces either a request or a response, and never throws", () => {
    fc.assert(
      fc.property(fc.string(), (line) => {
        const parsed = parseLine(line);
        if (parsed.kind === "response") {
          expect("error" in parsed.response).toBe(true);
          if ("error" in parsed.response) {
            expect(KNOWN_CODES).toContain(parsed.response.error.code);
          }
        }
      }),
      { numRuns: 400 },
    );
  });

  it("survives arbitrary bytes, not only text somebody meant to send", () => {
    fc.assert(
      fc.property(fc.uint8Array({ maxLength: 200 }), (bytes) => {
        const parsed = parseLine(Buffer.from(bytes).toString("utf8"));
        expect(["request", "response"]).toContain(parsed.kind);
      }),
      { numRuns: 400 },
    );
  });

  it("writes every response as exactly one line", () => {
    fc.assert(
      fc.property(fc.string(), (line) => {
        const parsed = parseLine(line);
        if (parsed.kind === "response") {
          expect(formatResponse(parsed.response).split("\n")).toHaveLength(2);
        }
      }),
      { numRuns: 200 },
    );
  });
});

describe("splitting standard input into lines", () => {
  function reader(mostBytes: number): { lines: string[]; tooLong: number; feed(text: string): void } {
    const lines: string[] = [];
    let tooLong = 0;
    const take = createLineReader({
      mostBytes,
      onLine: (line) => lines.push(line),
      onTooLong: () => {
        tooLong += 1;
      },
    });
    return { lines, get tooLong() { return tooLong; }, feed: (text) => take(Buffer.from(text)) };
  }

  it("reads two lines out of one chunk", () => {
    const it1 = reader(100);
    it1.feed("one\ntwo\n");
    expect(it1.lines).toEqual(["one", "two"]);
  });

  it("joins a line that arrived in pieces", () => {
    const it1 = reader(100);
    it1.feed("half");
    it1.feed(" and half\n");
    expect(it1.lines).toEqual(["half and half"]);
  });

  it("drops the carriage return a Windows sender would add", () => {
    const it1 = reader(100);
    it1.feed("one\r\n");
    expect(it1.lines).toEqual(["one"]);
  });

  it("reports a line past the cap once and then carries on with the next", () => {
    const it1 = reader(8);
    it1.feed("far too long to be allowed\nshort\n");
    expect(it1.tooLong).toBe(1);
    expect(it1.lines).toEqual(["short"]);
  });

  it("holds no more than the cap in memory, however much arrives without a newline", () => {
    const it1 = reader(16);
    for (let piece = 0; piece < 100; piece += 1) {
      it1.feed("x".repeat(100));
    }
    expect(it1.tooLong).toBe(1);
    expect(it1.lines).toEqual([]);
  });
});

describe("any string at all as an expectation", () => {
  const anyChange: fc.Arbitrary<Change> = fc.record({
    urlChanged: fc.boolean(),
    url: fc.string(),
    titleChanged: fc.boolean(),
    title: fc.string(),
    newElements: fc.array(
      fc.record({ ref: fc.string(), role: fc.string(), name: fc.string() }),
      { maxLength: 5 },
    ),
    removedCount: fc.nat({ max: 20 }),
    dialog: fc.option(fc.record({ kind: fc.constant("alert" as const), message: fc.string() }), {
      nil: null,
    }),
    newTab: fc.string(),
    download: fc.option(fc.record({ filename: fc.string(), path: fc.string() }), { nil: null }),
  });

  it("never throws, and always gives a verdict and a sentence", () => {
    fc.assert(
      fc.property(fc.string(), anyChange, (expectation, change) => {
        const verdict = judge(expectation, change);
        expect(typeof verdict.expectationMet).toBe("boolean");
        expect(typeof verdict.seen).toBe("string");
        if (verdict.expectationMet) {
          expect(verdict.seen).toBe("");
        } else {
          expect(verdict.seen.length).toBeGreaterThan(0);
        }
      }),
      { numRuns: 400 },
    );
  });

  it("splits into words that are all four letters or more and never repeat", () => {
    fc.assert(
      fc.property(fc.string(), (sentence) => {
        const words = meaningfulWords(sentence);
        expect(new Set(words).size).toBe(words.length);
        for (const word of words) {
          expect(word.length).toBeGreaterThanOrEqual(4);
        }
      }),
      { numRuns: 400 },
    );
  });

  it("always writes one sentence about what changed", () => {
    fc.assert(
      fc.property(anyChange, (change) => {
        expect(describeChange(change).length).toBeGreaterThan(0);
      }),
      { numRuns: 200 },
    );
  });
});

describe("any page at all put to the wall detector", () => {
  it("answers either nothing or one of the three walls, and never throws", () => {
    fc.assert(
      fc.property(
        fc.record({
          title: fc.string(),
          candidates: fc.array(
            fc.record({
              role: fc.string(),
              name: fc.string(),
              password: fc.boolean(),
              shortNumeric: fc.boolean(),
            }),
            { maxLength: 6 },
          ),
          frames: fc.array(fc.string(), { maxLength: 4 }),
        }),
        (evidence) => {
          const wall = findWall(evidence);
          if (wall !== null) {
            expect(["login", "two-factor", "captcha"]).toContain(wall.kind);
            expect(wall.detail.length).toBeGreaterThan(0);
          }
        },
      ),
      { numRuns: 400 },
    );
  });
});

describe("any answer at all put through redaction", () => {
  // Secrets made only of letters and digits, so that looking for one in the
  // written-out answer is a plain search and not a question about JSON escaping.
  const plainSecret = fc.stringMatching(/^[0-9a-z]{3,16}$/);
  const anyAnswer = fc.jsonValue({ maxDepth: 4 });

  it("leaves no trace of a secret anywhere in the written-out answer", () => {
    fc.assert(
      fc.property(anyAnswer, fc.array(plainSecret, { maxLength: 3 }), (answer, secrets) => {
        const usable = secrets.filter(
          (secret) => secret !== "" && !REDACTED.toLowerCase().includes(secret.toLowerCase()),
        );
        const written = JSON.stringify(redactDeep(answer, usable) ?? null).toLowerCase();
        for (const secret of usable) {
          expect(written).not.toContain(secret.toLowerCase());
        }
      }),
      { numRuns: 300 },
    );
  });

  it("never throws on any text at all", () => {
    fc.assert(
      fc.property(fc.string(), fc.array(fc.string(), { maxLength: 3 }), (text, secrets) => {
        expect(typeof redactText(text, secrets)).toBe("string");
      }),
      { numRuns: 300 },
    );
  });
});
