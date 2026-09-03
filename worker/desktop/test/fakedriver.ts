// A desktop driver nobody can see. It holds one window's accessibility tree,
// records every call, and lets a test change the tree after an action, which is
// how the act-and-assert loop is tested without a screen.

import type {
  DesktopDriver,
  DriverSnapshot,
  DriverWindow,
  Point,
  WindowTarget,
} from "../src/driver.js"
import type { DriverElement } from "../src/marks.js"
import type { KeyChord } from "../src/keys.js"

/** The fixture window's tree, which is the zenity entry the real tests drive. */
export function fixtureElements(): DriverElement[] {
  return [
    { element_index: 0, element_token: "s1:0", role: "dialog", label: "Coeus fixture window", enabled: true, frame: { x: 0, y: 0, w: 300, h: 220 } },
    { element_index: 1, element_token: "s1:1", role: "text box", label: "Type here", enabled: true, frame: { x: 50, y: 100, w: 200, h: 30 } },
    { element_index: 2, element_token: "s1:2", role: "button", label: "Cancel", enabled: true, frame: { x: 20, y: 170, w: 120, h: 40 } },
    { element_index: 3, element_token: "s1:3", role: "button", label: "OK", enabled: true, frame: { x: 160, y: 170, w: 120, h: 40 } },
  ]
}

/** One thing the fake driver was asked to do, in plain words. */
export type RecordedCall = string

/** FakeDriver is a desktop driver a test drives entirely by hand. */
export class FakeDriver implements DesktopDriver {
  /** Everything the driver was asked to do, in order. */
  readonly calls: RecordedCall[] = []
  /** The windows the machine is showing. */
  windows: DriverWindow[] = []
  /** The tree the granted window carries now. */
  elements: DriverElement[] = fixtureElements()
  /** The title the granted window carries now. */
  title = "Coeus fixture window"
  /** The picture the window returns, already encoded as base64 text. */
  picture = "iVBORw0KGgoFAKE"
  /** The picture of the whole screen, told apart from the window's by its text. */
  screen = "iVBORw0KGgoWHOLE"
  /** What the clipboard holds. */
  clipboard = ""
  /** The version the driver reports. */
  driverVersion = "0.23.2"
  /** When set, every call fails with this message. */
  brokenWith: string | undefined
  /** When set, the tree changes on every reading, so nothing ever settles. */
  restless = false
  /** What launching an application does, by name. */
  launchable = new Map<string, DriverWindow>()
  /** What to do to the tree after the next action. */
  afterAction: (() => void) | undefined
  private readings = 0

  async version(): Promise<string> {
    this.record("version")
    return this.driverVersion
  }

  async listWindows(): Promise<DriverWindow[]> {
    this.record("list the windows")
    return this.windows
  }

  async launch(application: string): Promise<DriverWindow[]> {
    this.record(`launch ${application}`)
    const window = this.launchable.get(application)
    if (!window) {
      return []
    }
    this.windows = [...this.windows, window]
    return [window]
  }

  async bringToFront(target: WindowTarget): Promise<void> {
    this.record(`bring window ${target.windowId} to the front`)
  }

  async read(target: WindowTarget, withPicture: boolean): Promise<DriverSnapshot> {
    this.record(`read window ${target.windowId}${withPicture ? " with a picture" : ""}`)
    this.readings += 1
    const elements = this.restless
      ? [...this.elements, { element_index: 99, element_token: `s${this.readings}:99`, role: "button", label: `Tick ${this.readings}`, enabled: true, frame: { x: 0, y: 0, w: 10, h: 10 } }]
      : this.elements
    return { title: this.title, elements, pictureBase64: withPicture ? this.picture : "" }
  }

  async readScreen(): Promise<string> {
    this.record("read the whole screen")
    return this.screen
  }

  async click(_target: WindowTarget, token: string, holdMilliseconds: number): Promise<void> {
    this.record(`click ${token} held for ${holdMilliseconds}`)
    this.finishAction()
  }

  async type(_target: WindowTarget, token: string | undefined, text: string): Promise<void> {
    this.record(`type ${JSON.stringify(text)} into ${token ?? "the focused control"}`)
    this.finishAction()
  }

  async press(_target: WindowTarget, chord: KeyChord): Promise<void> {
    this.record(`press ${[...chord.modifiers, chord.key].join("+")}`)
    this.finishAction()
  }

  async drag(_target: WindowTarget, from: Point, to: Point, steps: number): Promise<void> {
    this.record(`drag from ${from.x},${from.y} to ${to.x},${to.y} in ${steps} steps`)
    this.finishAction()
  }

  async readClipboard(): Promise<string> {
    this.record("read the clipboard")
    return this.clipboard
  }

  async writeClipboard(text: string): Promise<void> {
    this.record("write the clipboard")
    this.clipboard = text
  }

  async close(): Promise<void> {
    this.record("close")
  }

  /** record notes one call, and fails it when the test asked for a broken driver. */
  private record(what: string): void {
    this.calls.push(what)
    if (this.brokenWith !== undefined) {
      throw new Error(this.brokenWith)
    }
  }

  /** finishAction applies whatever the test said this action does to the tree. */
  private finishAction(): void {
    const change = this.afterAction
    this.afterAction = undefined
    change?.()
  }
}

/** aWindow is one entry of the machine's window list. */
export function aWindow(overrides: Partial<DriverWindow> = {}): DriverWindow {
  return { pid: 4242, windowId: 77, title: "Coeus fixture window", application: "zenity", ...overrides }
}
