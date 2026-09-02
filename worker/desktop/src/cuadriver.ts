// The one implementation of DesktopDriver that drives this machine, over
// @trycua/cua-driver. The lifecycle is OpenClaw's driver session at
// ~/Code/openclaw/extensions/cua-computer/src/driver-client.ts: load the native
// library only when a desktop is really going to be driven, keep one runtime for
// the life of the worker, and shut it down in order. Written fresh for Coeus and
// narrowed to the eleven calls DesktopDriver names.

import type { DesktopDriver, DriverSnapshot, DriverWindow, Point, WindowTarget } from "./driver.js"
import type { KeyChord } from "./keys.js"
import type { DriverElement } from "./marks.js"
import { waitFor } from "./pacing.js"
import { DesktopErrorCode, ProtocolError } from "./wire.js"

/** The most rows of an accessibility tree one reading will walk. */
const maximumTreeRows = 400

/** What one call to the driver hands back, in the parts this worker reads. */
interface DriverAnswer {
  /** What the driver said in words, which is what a refusal explains itself in. */
  text: string
  /** The machine-readable half of the answer. */
  structured: Record<string, unknown>
  /** The first picture the answer carried, as base64 text. */
  picture: string
}

/** CuaDesktopDriver drives the real screen, mouse, and keyboard. */
export class CuaDesktopDriver implements DesktopDriver {
  private runtime: import("@trycua/cua-driver").CuaDriverLike | undefined

  constructor(private readonly note: (line: string) => void) {}

  /** version is what the driver calls itself. */
  async version(): Promise<string> {
    const runtime = await this.started()
    return (await runtime.metadata()).driverVersion
  }

  /** listWindows is every window on the machine right now. */
  async listWindows(): Promise<DriverWindow[]> {
    const answer = await this.call("list_windows", {})
    return readWindows(answer.structured["windows"])
  }

  /** launch starts an application and returns the windows it opened. */
  async launch(application: string): Promise<DriverWindow[]> {
    this.prepareForXWayland()
    const answer = await this.call("launch_app", { name: application })
    this.note(`the desktop worker launched ${application}: ${answer.text}`)
    return readWindows(answer.structured["windows"])
  }

  /** bringToFront puts the window in front and gives it the keyboard. */
  async bringToFront(target: WindowTarget): Promise<void> {
    await this.call("bring_to_front", { pid: target.pid, window_id: target.windowId })
  }

  /** read returns the window's accessibility tree, and its picture when asked. */
  async read(target: WindowTarget, withPicture: boolean): Promise<DriverSnapshot> {
    const answer = await this.call("get_window_state", {
      ...targetOf(target),
      include_screenshot: withPicture,
      max_elements: maximumTreeRows,
    })
    const elements = readElements(answer.structured["elements"])
    return { title: elements[0]?.label ?? "", elements, pictureBase64: answer.picture }
  }

  /** click presses and releases the mouse on one control. */
  async click(target: WindowTarget, token: string, holdMilliseconds: number): Promise<void> {
    await this.call("click", { ...targetOf(target), element_token: token, delivery_mode: "foreground" })
    await waitFor(holdMilliseconds)
  }

  /** type sends text to one control, or to whatever holds the keyboard focus. */
  async type(target: WindowTarget, token: string | undefined, text: string): Promise<void> {
    const aimed = token === undefined ? {} : { element_token: token }
    await this.call("type_text", { ...targetOf(target), ...aimed, text, delivery_mode: "foreground" })
  }

  /** press sends one key combination to the window. */
  async press(target: WindowTarget, chord: KeyChord): Promise<void> {
    await this.call("press_key", {
      ...targetOf(target),
      key: chord.key,
      modifiers: chord.modifiers,
      delivery_mode: "foreground",
    })
  }

  /** drag moves the mouse from one point to another with the button held down. */
  async drag(target: WindowTarget, from: Point, to: Point, steps: number, milliseconds: number): Promise<void> {
    await this.call("drag", {
      ...targetOf(target),
      from_x: from.x,
      from_y: from.y,
      to_x: to.x,
      to_y: to.y,
      steps,
      duration_ms: milliseconds,
    })
  }

  /** readClipboard reads the machine's clipboard as plain text. */
  async readClipboard(): Promise<string> {
    const answer = await this.call("clipboard_read", { include_text: true })
    const written = answer.structured["text"]
    return typeof written === "string" ? written : ""
  }

  /** writeClipboard replaces the machine's clipboard with plain text. */
  async writeClipboard(text: string): Promise<void> {
    await this.call("clipboard_write", { text })
  }

  /** close ends the driver's runtime and releases the native handle. */
  async close(): Promise<void> {
    const runtime = this.runtime
    if (!runtime) {
      return
    }
    this.runtime = undefined
    await runtime.shutdown()
    // Releasing the native handle is the last step and only some runtimes have
    // it, so it is asked for rather than assumed.
    ;(runtime as { uniffiDestroy?: () => void }).uniffiDestroy?.()
  }

  /**
   * started loads the native library the first time a desktop is really going
   * to be driven, so that a machine without it can still answer health.
   */
  private async started(): Promise<import("@trycua/cua-driver").CuaDriverLike> {
    if (this.runtime) {
      return this.runtime
    }
    const sdk = await import("@trycua/cua-driver")
    const runtime = sdk.CuaDriver.create(undefined)
    if (!runtime.isAvailable()) {
      throw new ProtocolError(
        DesktopErrorCode.DriverUnavailable,
        "the cua driver loaded but says it cannot drive this machine, so run cua-driver doctor and fix what it names",
      )
    }
    this.runtime = runtime
    this.note("the desktop worker loaded the cua driver")
    return runtime
  }

  /** call runs one of the driver's tools and reads the parts this worker needs. */
  private async call(name: string, args: Record<string, unknown>): Promise<DriverAnswer> {
    const runtime = await this.started()
    const result = await runtime.callTool(name, JSON.stringify(args))
    const said = result.text ?? ""
    if (result.isError) {
      throw new Error(`the desktop driver refused ${name}: ${said}`)
    }
    return { text: said, structured: readObject(result.structuredJson), picture: firstPicture(result) }
  }

  /**
   * prepareForXWayland asks the applications this worker starts to run through
   * XWayland, where the accessibility tree and input delivery both work. It
   * changes nothing about the windows the user opened themselves.
   */
  private prepareForXWayland(): void {
    if (!process.env["WAYLAND_DISPLAY"] || !process.env["DISPLAY"]) {
      return
    }
    process.env["GDK_BACKEND"] = "x11"
    process.env["QT_QPA_PLATFORM"] = "xcb"
  }
}

/** targetOf writes a window target the way the driver's tools name it. */
function targetOf(target: WindowTarget): Record<string, unknown> {
  return { pid: target.pid, window_id: target.windowId }
}

/** readObject parses the driver's structured answer, or gives back nothing. */
function readObject(written: string | undefined): Record<string, unknown> {
  if (!written) {
    return {}
  }
  try {
    const parsed: unknown = JSON.parse(written)
    return typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {}
  } catch {
    return {}
  }
}

/** firstPicture is the first image the driver's answer carried, as base64 text. */
function firstPicture(result: { images?: readonly { dataBase64?: string }[] }): string {
  return result.images?.[0]?.dataBase64 ?? ""
}

/** readWindows turns the driver's window list into the worker's own shape. */
function readWindows(written: unknown): DriverWindow[] {
  if (!Array.isArray(written)) {
    return []
  }
  const windows: DriverWindow[] = []
  for (const entry of written) {
    if (typeof entry !== "object" || entry === null) {
      continue
    }
    const fields = entry as Record<string, unknown>
    const pid = fields["pid"]
    const windowId = fields["window_id"]
    if (typeof pid !== "number" || typeof windowId !== "number") {
      continue
    }
    windows.push({
      pid,
      windowId,
      title: typeof fields["title"] === "string" ? fields["title"] : "",
      application: typeof fields["app_name"] === "string" ? fields["app_name"] : "",
    })
  }
  return windows
}

/** readElements turns the driver's accessibility rows into the worker's own shape. */
function readElements(written: unknown): DriverElement[] {
  if (!Array.isArray(written)) {
    return []
  }
  const elements: DriverElement[] = []
  for (const entry of written) {
    if (typeof entry !== "object" || entry === null) {
      continue
    }
    const fields = entry as Record<string, unknown>
    const index = fields["element_index"]
    if (typeof index !== "number") {
      continue
    }
    const element: DriverElement = { element_index: index }
    copyText(element, fields, "element_token")
    copyText(element, fields, "role")
    copyText(element, fields, "label")
    copyText(element, fields, "value")
    if (typeof fields["enabled"] === "boolean") {
      element.enabled = fields["enabled"]
    }
    const frame = readFrame(fields["frame"])
    if (frame) {
      element.frame = frame
    }
    elements.push(element)
  }
  return elements
}

/** copyText copies one text field across when the driver sent one. */
function copyText(element: DriverElement, fields: Record<string, unknown>, name: "element_token" | "role" | "label" | "value"): void {
  const written = fields[name]
  if (typeof written === "string") {
    element[name] = written
  }
}

/** readFrame reads where a control sits, when the driver knows where that is. */
function readFrame(written: unknown): { x: number; y: number; w: number; h: number } | undefined {
  if (typeof written !== "object" || written === null) {
    return undefined
  }
  const fields = written as Record<string, unknown>
  const corners = ["x", "y", "w", "h"].map((name) => fields[name])
  if (!corners.every((corner) => typeof corner === "number")) {
    return undefined
  }
  const [x, y, w, h] = corners as [number, number, number, number]
  return { x, y, w, h }
}
