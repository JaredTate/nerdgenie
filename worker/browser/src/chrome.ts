/**
 * Launching the real Google Chrome and attaching to it.
 *
 * The rules from PROTOCOL.md: the real Chrome binary, its own profile folder and
 * never the user's daily one, a loopback DevTools port with a token made fresh for
 * each launch, and an attach over the Chrome DevTools Protocol. The window is
 * visible on the machine's own display; there is no headless mode.
 *
 * The launch, the readiness wait, and the stopping ladder are borrowed from
 * OpenClaw's Chrome launcher at
 * ~/Code/openclaw/extensions/browser/src/browser/chrome.ts, and the attach from
 * its transport at
 * ~/Code/openclaw/extensions/browser/src/browser/pw-session-cdp-transport.ts.
 * OpenClaw picks the port itself and polls for it; this worker lets the operating
 * system pick the port and reads the address Chrome prints, which removes the
 * whole business of reserving a port. The code is written fresh.
 */
import { spawn, type ChildProcess } from "node:child_process";
import { mkdir, rm } from "node:fs/promises";
import { join } from "node:path";
import { chromium, type Browser, type BrowserContext } from "playwright-core";
import { chromeDied } from "./errors.js";
import { CHROME_READY_LIMIT_MS, CHROME_STDERR_TAIL_BYTES, CHROME_STOP_LIMIT_MS } from "./limits.js";
import type { Logger } from "./log.js";
import { wait } from "./pacing.js";

/** Where downloads are saved, inside the profile folder the worker was given. */
export const DOWNLOADS_FOLDER = "downloads";

/** Chrome only ever listens on this address, and the worker only ever reaches it here. */
const LOOPBACK = "127.0.0.1";

/** The line Chrome prints once its DevTools server is listening. */
const LISTENING_LINE = /DevTools listening on (ws:\/\/\S+)/;

/** A Chrome the worker launched, and everything it needs to drive and stop it. */
export interface RunningChrome {
  browser: Browser;
  context: BrowserContext;
  /** The version Chrome reported, such as "151.0.7922.137". */
  version: string;
  /** Where downloads are saved. */
  downloadsFolder: string;
  isAlive(): boolean;
  stop(): Promise<void>;
}

/** What to launch and where to keep its profile. */
export interface LaunchOptions {
  profile: string;
  chromePath?: string | undefined;
  log: Logger;
}

/**
 * The flags Chrome is launched with, and why each one is there.
 *
 * There is deliberately no headless flag: a logged-in account is only safe in a
 * window the user can see. There is no proxy flag either, so Chrome uses the
 * machine's own network settings, which is what "the user's own connection"
 * means in the design.
 */
function launchArguments(profile: string): string[] {
  return [
    // Its own profile folder, never the user's daily one.
    `--user-data-dir=${profile}`,
    // Let the operating system pick a free port and tell us which one.
    "--remote-debugging-port=0",
    // Listen on loopback only, so nothing off this machine can drive the browser.
    `--remote-debugging-address=${LOOPBACK}`,
    // A fresh profile would otherwise stop to ask the user questions.
    "--no-first-run",
    "--no-default-browser-check",
    "--disable-session-crashed-bubble",
    "--hide-crash-restore-bubble",
    // An agent's profile is never signed in to Chrome and never phones home.
    "--disable-sync",
    "--disable-background-networking",
    "--disable-component-update",
    "--disable-features=Translate,MediaRouter",
    // Keep cookie encryption from asking the desktop keyring for a password.
    "--password-store=basic",
    // Chrome's shared memory folder is small in a container and this avoids it.
    "--disable-dev-shm-usage",
    "about:blank",
  ];
}

/** Keep only the tail of Chrome's own logging, so an error can quote it. */
function collectStandardError(child: ChildProcess, keep: { tail: string }): void {
  child.stderr?.setEncoding("utf8");
  child.stderr?.on("data", (piece: string) => {
    keep.tail = (keep.tail + piece).slice(-CHROME_STDERR_TAIL_BYTES);
  });
}

/** Wait for Chrome to print its DevTools address, or for it to die, or for the limit. */
async function waitForDevToolsAddress(
  child: ChildProcess,
  keep: { tail: string },
): Promise<string> {
  const giveUpAt = Date.now() + CHROME_READY_LIMIT_MS;
  for (;;) {
    const found = LISTENING_LINE.exec(keep.tail);
    if (found?.[1] !== undefined) {
      return found[1];
    }
    if (child.exitCode !== null || child.signalCode !== null) {
      throw chromeDied(
        `it stopped before it was ready. Chrome said: ${keep.tail.trim() || "nothing"}`,
      );
    }
    if (Date.now() >= giveUpAt) {
      throw chromeDied(
        `it did not print a DevTools address within ${CHROME_READY_LIMIT_MS} milliseconds. Chrome said: ${keep.tail.trim() || "nothing"}`,
      );
    }
    await wait(50);
  }
}

/**
 * Ask Chrome which version it is, and check that the address it hands back is the
 * same loopback port we asked. A DevTools server that points somewhere else is
 * not one we launched, and it never gets driven.
 */
async function readVersion(port: string): Promise<string> {
  const answer = await fetch(`http://${LOOPBACK}:${port}/json/version`, {
    signal: AbortSignal.timeout(5_000),
  });
  const told = (await answer.json()) as Record<string, unknown>;
  const socket = told["webSocketDebuggerUrl"];
  if (typeof socket !== "string" || new URL(socket).host !== `${LOOPBACK}:${port}`) {
    throw chromeDied(
      `its DevTools server pointed at ${String(socket)} rather than at the loopback port ${port} we launched it on.`,
    );
  }
  const browser = told["Browser"];
  return typeof browser === "string" ? browser.replace(/^\D*\//, "") : "unknown";
}

/** Stop Chrome politely, then firmly, and only ever the process we started. */
async function stopChrome(child: ChildProcess, browser: Browser, log: Logger): Promise<void> {
  await browser.close().catch(() => {});
  if (child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  child.kill("SIGTERM");
  const giveUpAt = Date.now() + CHROME_STOP_LIMIT_MS;
  while (child.exitCode === null && child.signalCode === null && Date.now() < giveUpAt) {
    await wait(50);
  }
  if (child.exitCode === null && child.signalCode === null) {
    log(`Chrome did not stop when asked, so it was ended by force.`);
    child.kill("SIGKILL");
  }
  // Wait for the process to really be gone. Until it is, Chrome is still writing
  // to the profile folder, and anything that tries to tidy the folder away will
  // find files appearing under it.
  const goneBy = Date.now() + CHROME_STOP_LIMIT_MS;
  while (child.exitCode === null && child.signalCode === null && Date.now() < goneBy) {
    await wait(25);
  }
}

/**
 * Launch Chrome with its own profile on a port the operating system picks, read
 * the address it prints, and attach to it.
 */
export async function launchChrome(options: LaunchOptions): Promise<RunningChrome> {
  const downloadsFolder = join(options.profile, DOWNLOADS_FOLDER);
  await mkdir(downloadsFolder, { recursive: true });
  // A leftover file from a previous run would name a port that is long gone.
  await rm(join(options.profile, "DevToolsActivePort"), { force: true });

  const binary = options.chromePath ?? "google-chrome";
  const child = spawn(binary, launchArguments(options.profile), {
    stdio: ["ignore", "ignore", "pipe"],
  });
  const keep = { tail: "" };
  collectStandardError(child, keep);
  child.on("error", (problem) => {
    keep.tail += `\ncould not start ${binary}: ${problem.message}`;
  });

  let address: string;
  try {
    address = await waitForDevToolsAddress(child, keep);
  } catch (problem) {
    child.kill("SIGKILL");
    throw problem;
  }
  const port = new URL(address).port;
  const version = await readVersion(port);
  const browser = await chromium.connectOverCDP(address, { timeout: CHROME_READY_LIMIT_MS });
  const context = browser.contexts()[0];
  if (context === undefined) {
    await stopChrome(child, browser, options.log);
    throw chromeDied("it attached but offered no browser window to drive.");
  }
  options.log(`launched Chrome ${version} on the loopback DevTools port ${port}.`);

  return {
    browser,
    context,
    version,
    downloadsFolder,
    isAlive: () => browser.isConnected() && child.exitCode === null && child.signalCode === null,
    stop: () => stopChrome(child, browser, options.log),
  };
}
