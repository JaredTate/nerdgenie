/**
 * Checking the parameters of every method before anything touches the browser.
 *
 * A request that fails here is answered with -32602 and a message that says what
 * was expected, which is the one error the Go side hands straight to the model.
 *
 * The shape is flat on purpose: one object per method, one object per act step,
 * no nested unions. That is the design OpenClaw's tool schema uses at
 * ~/Code/openclaw/extensions/browser/src/browser-tool.schema.ts, because model
 * providers reject deeply nested schemas. The code here is written fresh.
 */
import { wrongParameters } from "./errors.js";
import {
  MAX_ACT_STEPS,
  MAX_SCROLL_STEPS,
  MAX_TYPE_CHARS,
  MIN_SCROLL_STEPS,
} from "./limits.js";
import {
  DIALOG_ACTIONS,
  STEP_METHOD_NAMES,
  type DialogAnswer,
  type MethodName,
  type StepMethodName,
} from "./types.js";

/** Say what a value was, in a form that reads well inside an error message. */
function describeValue(value: unknown): string {
  const written = JSON.stringify(value);
  return written === undefined ? "nothing" : written;
}

/** Read a required piece of text, or explain what was missing. */
function requiredText(params: Record<string, unknown>, field: string, complaint: string): string {
  const value = params[field];
  if (typeof value !== "string" || value.trim() === "") {
    throw wrongParameters(complaint);
  }
  return value;
}

/** Check a piece of text that may be left out. */
function optionalText(params: Record<string, unknown>, field: string, complaint: string): void {
  const value = params[field];
  if (value !== undefined && typeof value !== "string") {
    throw wrongParameters(complaint);
  }
}

/** Every action carries an expectation, and it is always text when it is there at all. */
function checkExpectation(params: Record<string, unknown>, method: string): void {
  optionalText(params, "expectation", `The ${method} method needs the expectation to be text.`);
}

function checkOpen(params: Record<string, unknown>): void {
  const url = requiredText(
    params,
    "url",
    'The open method needs a url, such as "https://example.com/".',
  );
  let scheme = "";
  try {
    scheme = new URL(url).protocol;
  } catch {
    throw wrongParameters(`The open method could not read ${url} as a web address.`);
  }
  if (scheme !== "http:" && scheme !== "https:") {
    throw wrongParameters(
      `The open method only opens http and https addresses, and this one was ${url}.`,
    );
  }
}

function checkRead(params: Record<string, unknown>): void {
  if (params["visibleOnly"] !== undefined && typeof params["visibleOnly"] !== "boolean") {
    throw wrongParameters("The read method needs visibleOnly to be true or false.");
  }
}

function checkClick(params: Record<string, unknown>): void {
  requiredText(params, "ref", 'The click method needs a ref, such as "e7".');
  checkExpectation(params, "click");
}

function checkType(params: Record<string, unknown>): void {
  requiredText(params, "ref", 'The type method needs a ref, such as "e3".');
  const text = params["text"];
  if (typeof text !== "string") {
    throw wrongParameters("The type method needs the text to type.");
  }
  if (text.length > MAX_TYPE_CHARS) {
    throw wrongParameters(
      `The type method takes at most ${MAX_TYPE_CHARS} characters of text, and this was ${text.length}.`,
    );
  }
  checkExpectation(params, "type");
}

function checkPress(params: Record<string, unknown>): void {
  requiredText(params, "key", 'The press method needs a key, such as "Enter" or "Control+s".');
  checkExpectation(params, "press");
}

function checkScroll(params: Record<string, unknown>): void {
  const direction = params["direction"];
  if (direction !== "up" && direction !== "down") {
    throw wrongParameters(
      `The scroll method needs a direction of "up" or "down", and this was ${describeValue(direction)}.`,
    );
  }
  const amount = params["amount"];
  if (amount !== undefined) {
    const isInRange =
      typeof amount === "number" &&
      Number.isInteger(amount) &&
      amount >= MIN_SCROLL_STEPS &&
      amount <= MAX_SCROLL_STEPS;
    if (!isInRange) {
      throw wrongParameters(
        `The scroll method takes an amount from ${MIN_SCROLL_STEPS} to ${MAX_SCROLL_STEPS} steps, and this was ${describeValue(amount)}.`,
      );
    }
  }
  checkExpectation(params, "scroll");
}

/** Check one step of an act batch, and say which step went wrong. */
function checkActStep(step: unknown, position: number): void {
  const place = `Step ${position} of the act method`;
  if (typeof step !== "object" || step === null || Array.isArray(step)) {
    throw wrongParameters(`${place} was not an object.`);
  }
  const fields = step as Record<string, unknown>;
  const method = fields["method"];
  if (!STEP_METHOD_NAMES.includes(method as StepMethodName)) {
    throw wrongParameters(
      `${place} used the method ${describeValue(method)}, and a step may only be click, type, press, or scroll.`,
    );
  }
  try {
    checkParams(method as MethodName, fields);
  } catch (thrown) {
    const complaint = thrown instanceof Error ? thrown.message : String(thrown);
    throw wrongParameters(`${place}: ${complaint}`);
  }
}

function checkAct(params: Record<string, unknown>): void {
  const steps = params["steps"];
  if (!Array.isArray(steps) || steps.length < 1 || steps.length > MAX_ACT_STEPS) {
    throw wrongParameters(
      `The act method needs a steps list holding 1 to ${MAX_ACT_STEPS} steps.`,
    );
  }
  steps.forEach((step, index) => checkActStep(step, index + 1));
}

function checkTabs(params: Record<string, unknown>): void {
  const action = params["action"] ?? "list";
  if (action !== "list" && action !== "switch" && action !== "close") {
    throw wrongParameters(
      `The tabs method needs an action of "list", "switch", or "close", and this was ${describeValue(action)}.`,
    );
  }
  if (action !== "list") {
    requiredText(
      params,
      "tabId",
      'The tabs method needs a tabId, such as "t2", to switch or close a tab.',
    );
  }
}

/** The three values loginFill types, each with the field it goes into. */
const LOGIN_FIELDS: ReadonlyArray<{ value: string; ref: string; complaint: string }> = [
  {
    value: "username",
    ref: "usernameRef",
    complaint: 'The loginFill method needs a usernameRef, such as "e2".',
  },
  {
    value: "password",
    ref: "passwordRef",
    complaint: 'The loginFill method needs a passwordRef, such as "e3".',
  },
  {
    value: "code",
    ref: "codeRef",
    complaint:
      "The loginFill method was given a code but no codeRef saying which field to type it into.",
  },
];

function checkLoginFill(params: Record<string, unknown>): void {
  let filled = 0;
  for (const field of LOGIN_FIELDS) {
    const value = params[field.value];
    const ref = params[field.ref];
    if (value === undefined && ref === undefined) {
      continue;
    }
    if (typeof value !== "string" || value === "") {
      throw wrongParameters(
        `The loginFill method needs the ${field.value} to be text when a ${field.ref} is given.`,
      );
    }
    if (typeof ref !== "string" || ref.trim() === "") {
      throw wrongParameters(field.complaint);
    }
    filled += 1;
  }
  if (filled === 0) {
    throw wrongParameters(
      "The loginFill method needs at least one of a username, a password, or a code, each with the ref of the field it goes into.",
    );
  }
}

function checkDialog(params: Record<string, unknown>): void {
  const action = params["action"];
  if (!DIALOG_ACTIONS.includes(action as DialogAnswer)) {
    throw wrongParameters(
      `The dialog method needs an action of "accept" or "dismiss", and this was ${describeValue(action)}.`,
    );
  }
  optionalText(
    params,
    "text",
    "The dialog method needs the text to be text, because it is typed into a prompt.",
  );
}

const CHECKERS: Readonly<Record<MethodName, (params: Record<string, unknown>) => void>> = {
  open: checkOpen,
  read: checkRead,
  click: checkClick,
  type: checkType,
  press: checkPress,
  scroll: checkScroll,
  act: checkAct,
  tabs: checkTabs,
  loginFill: checkLoginFill,
  screenshot: () => {},
  dialog: checkDialog,
  health: () => {},
};

/** Check the parameters of one method, throwing a -32602 error when they are wrong. */
export function checkParams(method: MethodName, params: Record<string, unknown>): void {
  CHECKERS[method](params);
}
