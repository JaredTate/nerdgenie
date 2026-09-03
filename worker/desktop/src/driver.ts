// The seam between the worker's own logic and the program that actually moves
// the mouse. The shape is OpenClaw's driver session at
// ~/Code/openclaw/extensions/cua-computer/src/driver-client.ts: one object owns
// the driver's lifetime, every call goes through it, and the rest of the worker
// never imports the driver at all. Written fresh for Coeus, and narrowed to the
// nine things worker/desktop/PROTOCOL.md promises.

import type { KeyChord } from "./keys.js"
import type { DriverElement } from "./marks.js"

/** One window on the machine, as the driver reports it. */
export interface DriverWindow {
  /** The process the window belongs to. */
  pid: number
  /** The display server's own number for the window. */
  windowId: number
  /** What the title bar says. */
  title: string
  /** The name of the application that owns it. */
  application: string
}

/** The window every later call acts on. */
export interface WindowTarget {
  /** The process the window belongs to. */
  pid: number
  /** The display server's own number for the window. */
  windowId: number
}

/** One point on a window's picture, in screenshot pixels. */
export interface Point {
  /** How far from the left edge. */
  x: number
  /** How far from the top edge. */
  y: number
}

/** One reading of a window: what it is called and what is on it. */
export interface DriverSnapshot {
  /** What the title bar says now. */
  title: string
  /** The rows of the accessibility tree. */
  elements: DriverElement[]
  /** The picture as base64 text, empty when none was asked for. */
  pictureBase64: string
}

/**
 * DesktopDriver is everything the worker needs a driver to do. One
 * implementation drives the real machine and one is a fake in the tests, and
 * they are written against these same lines.
 */
export interface DesktopDriver {
  /** version is what the driver calls itself, for the health answer. */
  version(): Promise<string>
  /** listWindows is every window on the machine right now. */
  listWindows(): Promise<DriverWindow[]>
  /** launch starts an application and returns the windows it opened. */
  launch(application: string): Promise<DriverWindow[]>
  /** bringToFront puts one window in front and gives it the keyboard. */
  bringToFront(target: WindowTarget): Promise<void>
  /** read returns the window's tree, and its picture when one is asked for. */
  read(target: WindowTarget, withPicture: boolean): Promise<DriverSnapshot>
  /** readScreen returns the whole screen's picture as base64 text. */
  readScreen(): Promise<string>
  /** click presses and releases the mouse on one control. */
  click(target: WindowTarget, token: string, holdMilliseconds: number): Promise<void>
  /** type sends text to one control, or to whatever holds the focus. */
  type(target: WindowTarget, token: string | undefined, text: string): Promise<void>
  /** press sends one key combination. */
  press(target: WindowTarget, chord: KeyChord): Promise<void>
  /** drag moves the mouse from one point to another with the button down. */
  drag(target: WindowTarget, from: Point, to: Point, steps: number, milliseconds: number): Promise<void>
  /** readClipboard reads the machine's clipboard as plain text. */
  readClipboard(): Promise<string>
  /** writeClipboard replaces the machine's clipboard with plain text. */
  writeClipboard(text: string): Promise<void>
  /** close ends the driver's session and releases what it holds. */
  close(): Promise<void>
}
