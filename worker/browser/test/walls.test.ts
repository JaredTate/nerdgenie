import { describe, expect, it } from "vitest";
import {
  CAPTCHA_PHRASES,
  LOGIN_PHRASES,
  TWO_FACTOR_PHRASES,
  TWO_FACTOR_WEAK_PHRASES,
  findWall,
  type WallEvidence,
} from "../src/walls.js";

function evidence(part: Partial<WallEvidence>): WallEvidence {
  return { title: "Example", candidates: [], frames: [], ...part };
}

// One test per entry in the fixture list, so that adding a phrase without a test
// is impossible: the loop is the test.
describe("the captcha fixture list", () => {
  for (const phrase of CAPTCHA_PHRASES) {
    it(`sees a captcha in a heading that says "${phrase}"`, () => {
      expect(
        findWall(evidence({ candidates: [{ role: "heading", name: `Please ${phrase} now` }] })),
      ).toEqual({ kind: "captcha", detail: `a heading named Please ${phrase} now` });
    });

    it(`sees a captcha in a frame whose address says "${phrase}"`, () => {
      const address = `https://example.com/${phrase.replace(/ /g, "-")}/frame`;
      expect(findWall(evidence({ frames: [address] }))).toEqual({
        kind: "captcha",
        detail: `a frame at ${address}`,
      });
    });
  }

  it("sees a captcha in the page title", () => {
    expect(findWall(evidence({ title: "reCAPTCHA challenge" }))).toEqual({
      kind: "captcha",
      detail: "a page titled reCAPTCHA challenge",
    });
  });

  it("sees a captcha named on any element, not only a heading", () => {
    expect(findWall(evidence({ candidates: [{ role: "button", name: "hCaptcha" }] }))).toEqual({
      kind: "captcha",
      detail: "a button named hCaptcha",
    });
  });
});

describe("the two-factor fixture list", () => {
  for (const phrase of TWO_FACTOR_PHRASES) {
    it(`sees a second-factor prompt in a heading that says "${phrase}"`, () => {
      expect(
        findWall(evidence({ candidates: [{ role: "heading", name: `Enter your ${phrase}` }] })),
      ).toEqual({ kind: "two-factor", detail: `a heading named Enter your ${phrase}` });
    });
  }

  for (const phrase of TWO_FACTOR_WEAK_PHRASES) {
    it(`sees a second-factor prompt in a field that says "${phrase}" beside a short numeric field`, () => {
      expect(
        findWall(
          evidence({
            candidates: [{ role: "textbox", name: `Your ${phrase}`, shortNumeric: true }],
          }),
        ),
      ).toEqual({ kind: "two-factor", detail: `a textbox named Your ${phrase}` });
    });

    it(`ignores "${phrase}" on its own, because the word is too common to trust`, () => {
      expect(
        findWall(evidence({ candidates: [{ role: "textbox", name: `Your ${phrase}` }] })),
      ).toBeNull();
    });
  }
});

describe("the login fixture list", () => {
  for (const phrase of LOGIN_PHRASES) {
    it(`sees a login wall in a heading that says "${phrase}"`, () => {
      expect(
        findWall(evidence({ candidates: [{ role: "heading", name: `Please ${phrase}` }] })),
      ).toEqual({ kind: "login", detail: `a heading named Please ${phrase}` });
    });

    it(`sees a login wall in a form named "${phrase}"`, () => {
      expect(findWall(evidence({ candidates: [{ role: "form", name: phrase }] }))).toEqual({
        kind: "login",
        detail: `a form named ${phrase}`,
      });
    });
  }

  it("sees a login wall from a password field on its own", () => {
    expect(
      findWall(evidence({ candidates: [{ role: "textbox", name: "Password", password: true }] })),
    ).toEqual({ kind: "login", detail: "a password field named Password" });
  });

  it("does not see a login wall from an ordinary link that says log in", () => {
    expect(findWall(evidence({ candidates: [{ role: "link", name: "Log in" }] }))).toBeNull();
  });

  it("does not see a login wall from a button that says sign in", () => {
    expect(findWall(evidence({ candidates: [{ role: "button", name: "Sign in" }] }))).toBeNull();
  });
});

describe("the order the walls are checked in", () => {
  const loginForm = { role: "form", name: "Sign in" };
  const codeHeading = { role: "heading", name: "Enter your authenticator code" };
  const captchaHeading = { role: "heading", name: "Please solve the captcha" };

  it("reports the captcha when a page has all three, because it is the hardest to pass", () => {
    expect(
      findWall(evidence({ candidates: [loginForm, codeHeading, captchaHeading] }))?.kind,
    ).toBe("captcha");
  });

  it("reports the second factor before the login form, because it comes after the password", () => {
    expect(findWall(evidence({ candidates: [loginForm, codeHeading] }))?.kind).toBe("two-factor");
  });
});

describe("ordinary pages", () => {
  const ordinary: Array<[string, WallEvidence]> = [
    ["an empty page", evidence({})],
    [
      "a page of links and buttons",
      evidence({
        title: "Coeus test page",
        candidates: [
          { role: "link", name: "Home" },
          { role: "button", name: "Post" },
          { role: "textbox", name: "Search" },
          { role: "heading", name: "Welcome" },
        ],
      }),
    ],
    [
      "a page with a frame that is not a captcha",
      evidence({ frames: ["https://example.com/advertisement.html"] }),
    ],
    [
      "a search box that takes a short number, with nothing about codes",
      evidence({ candidates: [{ role: "textbox", name: "Quantity", shortNumeric: true }] }),
    ],
  ];

  for (const [description, page] of ordinary) {
    it(`finds no wall on ${description}`, () => {
      expect(findWall(page)).toBeNull();
    });
  }
});

describe("the phrase lists themselves", () => {
  it("holds every phrase in lower case, because the search lowercases the page", () => {
    for (const phrase of [
      ...CAPTCHA_PHRASES,
      ...TWO_FACTOR_PHRASES,
      ...TWO_FACTOR_WEAK_PHRASES,
      ...LOGIN_PHRASES,
    ]) {
      expect(phrase).toBe(phrase.toLowerCase());
    }
  });
});
