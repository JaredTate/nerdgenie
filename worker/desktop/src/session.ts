// The desktop session: one granted application, and the act-and-assert loop
// every action goes through. The loop is design section 9's, which the desktop
// reuses: act with human pacing, wait for the window to settle, read it again,
// compare, and say whether what the model expected actually happened. The
// separation of "the call worked" from "the effect was confirmed" is Hermes'
// action result at ~/Code/hermes-agent/tools/computer_use/backend.py.

import type { DesktopDriver, DriverWindow, WindowTarget } from "./driver.js"
import { judgeExpectation, type WindowChange } from "./expectation.js"
import { parseKeyChord } from "./keys.js"
import {
  maximumApplicationNameLength,
  maximumPictureLength,
  maximumSettleReadings,
  maximumTypedCharacters,
} from "./limits.js"
import { compareMarks, middleOf, numberTheControls, type Mark, type MarkedControl } from "./marks.js"
import { typingRuns, variedGap, waitFor, type Pacing } from "./pacing.js"
import { DesktopErrorCode, ProtocolError } from "./wire.js"

/** What one action returns, as PROTOCOL.md's Diff says. */
export interface Diff {
  /** True when the window's title is not the one it had before the action. */
  titleChanged: boolean
  /** The title the window carries now. */
  title: string
  /** The controls that were not on the window before the action. */
  newMarks: Mark[]
  /** How many of the controls that were there before are gone. */
  goneMarks: number
  /** Every control on the window now, numbered afresh. */
  marks: Mark[]
  /** True when what the model said it expected actually happened. */
  expectationMet: boolean
  /** One sentence saying what happened instead, empty when it was met. */
  seen: string
  /** False when the window never came to rest within the limit. */
  settled: boolean
}

/** What `launch` returns: a diff and the application it granted. */
export interface LaunchResult extends Diff {
  /** The application every later call acts in. */
  application: string
}

/** What `screenshot` returns. */
export interface ScreenshotResult {
  /** The picture of the window, encoded as base64 text. */
  pngBase64: string
  /** What each number on the picture points at. */
  marks: Mark[]
  /** The application the picture is of. */
  application: string
  /** What its title bar says. */
  title: string
  /** How many controls the cap left out. */
  hidden: number
}

/** What `health` returns. */
export interface Health {
  /** True when the worker can act on the desktop. */
  healthy: boolean
  /** What the driver calls itself. */
  driverVersion: string
  /** Which display server this desktop runs on. */
  display: string
  /** What to fix when it is not healthy. */
  detail: string
}

/** One reading of the granted window. */
interface Reading {
  /** The controls on it, numbered. */
  controls: MarkedControl[]
  /** What its title bar says. */
  title: string
  /** How many controls the cap left out. */
  hidden: number
  /** Its picture, empty when none was asked for. */
  picture: string
}

/** DesktopSession holds the one granted application and acts inside it. */
export class DesktopSession {
  private granted: { application: string; target: WindowTarget } | undefined
  private controls: MarkedControl[] = []
  private title = ""
  private aimed: MarkedControl | undefined

  constructor(
    private readonly driver: DesktopDriver,
    private readonly pacing: Pacing,
    private readonly note: (line: string) => void,
  ) {}

  /** launch opens an application, or brings it forward when it is already open. */
  async launch(application: string, expectation: string): Promise<LaunchResult> {
    const wanted = application.trim()
    if (wanted === "" || wanted.length > maximumApplicationNameLength) {
      throw new ProtocolError(
        DesktopErrorCode.BadParameters,
        `the application name must be between one and ${maximumApplicationNameLength} characters`,
      )
    }
    const window = (await this.findWindow(wanted)) ?? (await this.openWindow(wanted))
    this.granted = { application: wanted, target: { pid: window.pid, windowId: window.windowId } }
    this.controls = []
    this.title = ""
    this.aimed = undefined
    this.note(`the desktop is granted the application ${wanted}, window ${window.windowId}`)
    const diff = await this.act(expectation, undefined, async () => {})
    return { ...diff, application: wanted }
  }

  /** findWindow looks for a window the application already has open. */
  private async findWindow(application: string): Promise<DriverWindow | undefined> {
    const wanted = application.toLowerCase()
    const windows = await this.callDriver(() => this.driver.listWindows())
    return windows.find(
      (window) => window.application.toLowerCase().includes(wanted) || window.title.toLowerCase().includes(wanted),
    )
  }

  /** openWindow starts the application and waits for its window to appear. */
  private async openWindow(application: string): Promise<DriverWindow> {
    const opened = await this.callDriver(() => this.driver.launch(application))
    if (opened.length > 0) {
      return opened[0] as DriverWindow
    }
    const until = Date.now() + this.pacing.launchWait
    while (Date.now() < until) {
      await waitFor(this.pacing.launchCheck)
      const found = await this.findWindow(application)
      if (found) {
        return found
      }
    }
    throw new ProtocolError(
      DesktopErrorCode.LaunchFailed,
      `the application ${JSON.stringify(application)} opened no window, so check that the name is one this machine can run`,
    )
  }

  /** screenshot photographs the granted window and numbers its controls. */
  async screenshot(): Promise<ScreenshotResult> {
    const granted = this.requireGranted()
    await this.callDriver(() => this.driver.bringToFront(granted.target))
    const reading = await this.readWindow(granted.target, true)
    if (reading.picture.length > maximumPictureLength) {
      throw new ProtocolError(
        DesktopErrorCode.UnreadableWindow,
        `the picture of the window is too big to send at ${reading.picture.length} characters, so make the window smaller and take it again`,
      )
    }
    this.remember(reading)
    return {
      pngBase64: reading.picture,
      marks: marksOf(reading.controls),
      application: granted.application,
      title: reading.title,
      hidden: reading.hidden,
    }
  }

  /** click presses the control with that number and checks the expectation. */
  async click(mark: number, expectation: string): Promise<Diff> {
    const granted = this.requireGranted()
    const control = this.controlNumbered(mark)
    const diff = await this.act(expectation, control, () =>
      this.driver.click(granted.target, control.token, this.pacing.clickHold),
    )
    this.aimed = control
    return diff
  }

  /** type types text at human pacing into the control the last click landed on. */
  async type(text: string, mark: number | undefined, expectation: string): Promise<Diff> {
    const granted = this.requireGranted()
    if (text.length > maximumTypedCharacters) {
      throw new ProtocolError(
        DesktopErrorCode.BadParameters,
        `the text is ${text.length} characters and at most ${maximumTypedCharacters} may be typed at once, so send it in pieces`,
      )
    }
    const aimed = mark === undefined ? this.aimed : this.controlNumbered(mark)
    return this.act(expectation, aimed, async () => {
      const runs = typingRuns(text, this.pacing)
      for (const [index, run] of runs.entries()) {
        if (index > 0) {
          await waitFor(variedGap(this.pacing, Math.random))
        }
        await this.driver.type(granted.target, aimed?.token, run)
      }
    })
  }

  /** press presses one key combination and checks the expectation. */
  async press(keys: string, expectation: string): Promise<Diff> {
    const granted = this.requireGranted()
    const chord = parseKeyChord(keys)
    return this.act(expectation, this.aimed, () => this.driver.press(granted.target, chord))
  }

  /** drag drags from the middle of one numbered control to the middle of another. */
  async drag(fromMark: number, toMark: number, expectation: string): Promise<Diff> {
    const granted = this.requireGranted()
    const from = this.controlNumbered(fromMark)
    const to = this.controlNumbered(toMark)
    return this.act(expectation, from, () =>
      this.driver.drag(
        granted.target,
        middleOf(from.frame),
        middleOf(to.frame),
        this.pacing.dragSteps,
        this.pacing.dragMilliseconds,
      ),
    )
  }

  /** clipboardGet reads the machine's clipboard as plain text. */
  async clipboardGet(): Promise<{ text: string }> {
    return { text: await this.callDriver(() => this.driver.readClipboard()) }
  }

  /** clipboardSet replaces the machine's clipboard with plain text. */
  async clipboardSet(text: string): Promise<{ characters: number }> {
    await this.callDriver(() => this.driver.writeClipboard(text))
    return { characters: text.length }
  }

  /** health says whether the worker can act on the desktop, and what to fix. */
  async health(): Promise<Health> {
    const display = describeDisplay()
    try {
      return { healthy: true, driverVersion: await this.driver.version(), display, detail: "" }
    } catch (failure) {
      return { healthy: false, driverVersion: "", display, detail: `the desktop driver could not be reached: ${saidBy(failure)}` }
    }
  }

  /** close ends the driver's session. */
  async close(): Promise<void> {
    await this.driver.close()
  }

  /** act is the one path every action takes: do it, settle, read, compare, judge. */
  private async act(
    expectation: string,
    aimedAt: MarkedControl | undefined,
    run: () => Promise<void>,
  ): Promise<Diff> {
    const granted = this.requireGranted()
    const wasTitle = this.title
    const wasMarks = marksOf(this.controls)
    await this.callDriver(() => this.driver.bringToFront(granted.target))
    await this.callDriver(run)
    await waitFor(this.pacing.betweenActions)
    const settled = await this.settle(granted.target)
    this.remember(settled.reading)
    const marks = marksOf(this.controls)
    const change: WindowChange = {
      titleChanged: this.title !== wasTitle,
      title: this.title,
      settled: settled.settled,
      ...compareMarks(wasMarks, marks),
    }
    if (aimedAt) {
      change.aimedAt = aimedAt.mark
    }
    const verdict = judgeExpectation(expectation, change)
    return {
      titleChanged: change.titleChanged,
      title: change.title,
      newMarks: change.newMarks,
      goneMarks: change.goneMarks,
      marks,
      expectationMet: verdict.expectationMet,
      seen: verdict.seen,
      settled: settled.settled,
    }
  }

  /** settle waits until the window holds still, or until the limit runs out. */
  private async settle(target: WindowTarget): Promise<{ reading: Reading; settled: boolean }> {
    const until = Date.now() + this.pacing.settleLimit
    let previous = await this.readWindow(target, false)
    for (let readings = 1; readings < maximumSettleReadings; readings += 1) {
      await waitFor(this.pacing.settleQuiet)
      const now = await this.readWindow(target, false)
      if (sameReading(previous, now)) {
        return { reading: now, settled: true }
      }
      previous = now
      if (Date.now() >= until) {
        return { reading: now, settled: false }
      }
    }
    return { reading: previous, settled: false }
  }

  /** readWindow reads the granted window's tree, and its picture when asked. */
  private async readWindow(target: WindowTarget, withPicture: boolean): Promise<Reading> {
    const snapshot = await this.callDriver(() => this.driver.read(target, withPicture))
    const numbered = numberTheControls(snapshot.elements)
    return {
      controls: numbered.controls,
      title: snapshot.title,
      hidden: numbered.hidden,
      picture: snapshot.pictureBase64,
    }
  }

  /** remember keeps the reading the next comparison and the next click use. */
  private remember(reading: Reading): void {
    this.controls = reading.controls
    this.title = reading.title
  }

  /** requireGranted refuses to act when no application has been opened yet. */
  private requireGranted(): { application: string; target: WindowTarget } {
    if (!this.granted) {
      throw new ProtocolError(
        DesktopErrorCode.NoApplicationOpen,
        "no application is open on the desktop, so launch one before acting on it",
      )
    }
    return this.granted
  }

  /** controlNumbered finds the control the model pointed at, or says it is gone. */
  private controlNumbered(mark: number): MarkedControl {
    const found = this.controls.find((control) => control.mark.number === mark)
    if (!found) {
      throw new ProtocolError(
        DesktopErrorCode.NoSuchMark,
        `there is no control numbered ${mark} on the screen, so take a screenshot and use a number from it`,
        { marks: marksOf(this.controls) },
      )
    }
    return found
  }

  /** callDriver turns anything the driver throws into the protocol's own error. */
  private async callDriver<Answer>(run: () => Promise<Answer>): Promise<Answer> {
    try {
      return await run()
    } catch (failure) {
      if (failure instanceof ProtocolError) {
        throw failure
      }
      throw new ProtocolError(
        DesktopErrorCode.DriverUnavailable,
        `the desktop driver could not be reached: ${saidBy(failure)}`,
      )
    }
  }
}

/** marksOf is what the model sees of a list of controls. */
function marksOf(controls: readonly MarkedControl[]): Mark[] {
  return controls.map((control) => control.mark)
}

/** sameReading says whether two readings of a window are the same reading. */
function sameReading(before: Reading, after: Reading): boolean {
  if (before.title !== after.title || before.controls.length !== after.controls.length) {
    return false
  }
  return before.controls.every((control, at) => {
    const other = after.controls[at]
    return other !== undefined && control.mark.role === other.mark.role && control.mark.name === other.mark.name
  })
}

/** describeDisplay says which display server this desktop runs on. */
function describeDisplay(): string {
  const wayland = process.env["WAYLAND_DISPLAY"]
  const x11 = process.env["DISPLAY"]
  if (wayland) {
    return x11 ? "wayland with xwayland" : "wayland"
  }
  return x11 ? "x11" : "none"
}

/** saidBy is what a thrown thing says, whatever kind of thing it is. */
function saidBy(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure)
}
