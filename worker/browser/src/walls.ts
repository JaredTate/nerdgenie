/**
 * The wall detector: the three things that stop the agent and hand the browser
 * to the user, and the fixture lists of text that name them.
 *
 * A wall is a login form, a prompt for a second code, or a captcha. The worker
 * never tries to get past one. It fills in `wall` on the diff, sets
 * `expectationMet` to false, and stops.
 *
 * Every phrase below is written the way the matcher sees it: lower case, one
 * space between words. The matcher whittles a page's text down to the same shape
 * and then looks for whole words, so "recaptcha" matches "g-recaptcha-response"
 * but "password" does not match "passwordless".
 */
import type { Wall } from "./types.js";

/** One thing on the page that a wall could be named on. */
export interface WallCandidate {
  role: string;
  name: string;
  /** True when this is a field that hides what is typed into it. */
  password?: boolean;
  /** True when this is a field that takes a short run of digits, the shape of a code. */
  shortNumeric?: boolean;
}

/** Everything the detector is allowed to look at. */
export interface WallEvidence {
  title: string;
  candidates: WallCandidate[];
  /** The addresses of the frames on the page, which is where captchas usually live. */
  frames: string[];
}

/** Text that names a captcha. A captcha may be named on anything, so all roles count. */
export const CAPTCHA_PHRASES: readonly string[] = [
  "captcha",
  "hcaptcha",
  "recaptcha",
  "turnstile",
  "verify you are human",
  "verify you are a human",
  "i am not a robot",
  "security check",
];

/** Text that names a prompt for a second code on its own, with no other evidence needed. */
export const TWO_FACTOR_PHRASES: readonly string[] = [
  "two factor",
  "authenticator",
  "verification code",
  "authentication code",
  "one time code",
  "one time passcode",
  "security code",
  "2fa",
];

/** Text that suggests a second code but is too common to trust without a short numeric field. */
export const TWO_FACTOR_WEAK_PHRASES: readonly string[] = ["code", "verification", "verify"];

/** Text that names a login form. Only a heading or a form counts, never a link or a button. */
export const LOGIN_PHRASES: readonly string[] = [
  "sign in",
  "signin",
  "sign into",
  "log in",
  "login",
  "logon",
  "password",
];

/** The roles that may carry the words of a login form or a prompt for a code. */
const HEADING_OR_FIELD_ROLES: ReadonlySet<string> = new Set(["heading", "form", "textbox"]);
const HEADING_OR_FORM_ROLES: ReadonlySet<string> = new Set(["heading", "form"]);

/**
 * Whittle text down to lower-case words separated by single spaces, with a space
 * at each end, so that a phrase can be looked for as whole words.
 */
function asWords(text: string): string {
  return ` ${text.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim()} `;
}

/** Does this text hold one of these phrases, as whole words? */
function holdsPhrase(text: string, phrases: readonly string[]): boolean {
  const words = asWords(text);
  return phrases.some((phrase) => words.includes(` ${phrase} `));
}

/** Name one thing on the page the way a person would describe it. */
function describe(candidate: WallCandidate): string {
  const role = candidate.password === true ? "password field" : candidate.role;
  return `a ${role} named ${candidate.name}`;
}

/** The first candidate with one of these roles whose name holds one of these phrases. */
function firstNamed(
  candidates: readonly WallCandidate[],
  phrases: readonly string[],
  roles?: ReadonlySet<string>,
): WallCandidate | undefined {
  return candidates.find(
    (candidate) =>
      (roles === undefined || roles.has(candidate.role)) && holdsPhrase(candidate.name, phrases),
  );
}

/** A captcha may be named on any element, in any frame's address, or in the page title. */
function findCaptcha(evidence: WallEvidence): Wall | null {
  const named = firstNamed(evidence.candidates, CAPTCHA_PHRASES);
  if (named !== undefined) {
    return { kind: "captcha", detail: describe(named) };
  }
  const frame = evidence.frames.find((address) => holdsPhrase(address, CAPTCHA_PHRASES));
  if (frame !== undefined) {
    return { kind: "captcha", detail: `a frame at ${frame}` };
  }
  if (holdsPhrase(evidence.title, CAPTCHA_PHRASES)) {
    return { kind: "captcha", detail: `a page titled ${evidence.title}` };
  }
  return null;
}

/**
 * A prompt for a second code. The strong phrases stand on their own. The weak
 * ones are ordinary words, so they only count when the page also holds a field
 * that takes a short run of digits.
 */
function findTwoFactor(evidence: WallEvidence): Wall | null {
  const strong = firstNamed(evidence.candidates, TWO_FACTOR_PHRASES, HEADING_OR_FIELD_ROLES);
  if (strong !== undefined) {
    return { kind: "two-factor", detail: describe(strong) };
  }
  if (holdsPhrase(evidence.title, TWO_FACTOR_PHRASES)) {
    return { kind: "two-factor", detail: `a page titled ${evidence.title}` };
  }
  const hasShortNumericField = evidence.candidates.some(
    (candidate) => candidate.shortNumeric === true,
  );
  if (!hasShortNumericField) {
    return null;
  }
  const weak = firstNamed(evidence.candidates, TWO_FACTOR_WEAK_PHRASES, HEADING_OR_FIELD_ROLES);
  return weak === undefined ? null : { kind: "two-factor", detail: describe(weak) };
}

/**
 * A login form. A password field is proof on its own. Otherwise only a heading or
 * a form counts, because almost every page in the world has a link that says
 * "Log in" and almost none of them are a wall.
 */
function findLogin(evidence: WallEvidence): Wall | null {
  const passwordField = evidence.candidates.find((candidate) => candidate.password === true);
  if (passwordField !== undefined) {
    return { kind: "login", detail: describe(passwordField) };
  }
  const named = firstNamed(evidence.candidates, LOGIN_PHRASES, HEADING_OR_FORM_ROLES);
  return named === undefined ? null : { kind: "login", detail: describe(named) };
}

/**
 * Find the wall on a page, or nothing. The three are checked hardest first: a
 * captcha stops the agent even when it has the password, and a prompt for a
 * second code comes after the login form it follows.
 */
export function findWall(evidence: WallEvidence): Wall | null {
  return findCaptcha(evidence) ?? findTwoFactor(evidence) ?? findLogin(evidence);
}
