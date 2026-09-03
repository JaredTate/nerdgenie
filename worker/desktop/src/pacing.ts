// How fast the worker moves. Design section 9, "Like a human", says the agent
// holds a click for a human length of time, types one key at a time with small
// variations in speed, and pauses between actions; the desktop reuses that rule.
// The fast pacing exists only for the tests, which cannot wait for a person.

import { DesktopErrorCode, ProtocolError } from "./wire.js"

/** One pacing profile: every wait the worker makes on purpose. */
export interface Pacing {
  /** Which profile this is. */
  name: string
  /** The pause after an action, before the window is read again. */
  betweenActions: number
  /** How long a click is held down. */
  clickHold: number
  /** How many characters go into one run of typing. */
  typingRun: number
  /** The gap between two runs of typing, before its small variation. */
  typingGap: number
  /** How many steps a drag is broken into. */
  dragSteps: number
  /** How long the whole drag takes. */
  dragMilliseconds: number
  /** How long the window must hold still before it counts as settled. */
  settleQuiet: number
  /** The longest the worker waits for a window to settle. */
  settleLimit: number
  /** The longest the worker waits for a launched application to show a window. */
  launchWait: number
  /** How often it looks for that window while it waits. */
  launchCheck: number
}

/** The pacing a person would move at, which is what the agent uses. */
const humanPacing: Pacing = {
  name: "human",
  betweenActions: 250,
  clickHold: 80,
  typingRun: 6,
  typingGap: 90,
  dragSteps: 24,
  dragMilliseconds: 600,
  settleQuiet: 300,
  settleLimit: 3_000,
  launchWait: 10_000,
  launchCheck: 250,
}

/** The pacing the tests use, which waits for nothing it does not have to. */
const fastPacing: Pacing = {
  name: "fast",
  betweenActions: 0,
  clickHold: 0,
  typingRun: 10_000,
  typingGap: 0,
  dragSteps: 4,
  dragMilliseconds: 0,
  settleQuiet: 30,
  settleLimit: 600,
  launchWait: 300,
  launchCheck: 30,
}

/** The two profiles, by the name the command line writes. */
const profiles = new Map<string, Pacing>([
  [humanPacing.name, humanPacing],
  [fastPacing.name, fastPacing],
])

/** pacingNamed returns one profile, or refuses a name nobody ships. */
export function pacingNamed(name: string): Pacing {
  const found = profiles.get(name)
  if (found === undefined) {
    throw new ProtocolError(
      DesktopErrorCode.BadParameters,
      `there is no pacing called ${JSON.stringify(name)}, so ask for human or fast`,
    )
  }
  return found
}

/** typingRuns breaks text into the runs one burst of typing carries. */
export function typingRuns(text: string, pacing: Pacing): string[] {
  const runs: string[] = []
  for (let at = 0; at < text.length; at += pacing.typingRun) {
    runs.push(text.slice(at, at + pacing.typingRun))
  }
  return runs
}

/** The narrowest and widest a gap may be, as a share of the profile's gap. */
const gapSpread = { narrowest: 0.6, widest: 1.4 }

/** variedGap is the gap between two runs, with a person's unevenness on it. */
export function variedGap(pacing: Pacing, draw: () => number): number {
  if (pacing.typingGap === 0) {
    return 0
  }
  const share = gapSpread.narrowest + (gapSpread.widest - gapSpread.narrowest) * draw()
  return Math.max(1, Math.round(pacing.typingGap * share))
}

/** waitFor pauses for the given number of milliseconds, and never for none. */
export function waitFor(milliseconds: number): Promise<void> {
  if (milliseconds <= 0) {
    return Promise.resolve()
  }
  return new Promise((finish) => {
    setTimeout(finish, milliseconds)
  })
}
