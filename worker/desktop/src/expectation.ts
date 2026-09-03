// Whether what the model said it expected actually happened. The worker cannot
// judge English, so the rule is fixed and written down in
// worker/desktop/PROTOCOL.md, and it is the browser worker's rule with the
// page's parts swapped for the window's.

import type { Mark } from "./marks.js"

/** The shortest word that can carry meaning in an expectation. */
const shortestMeaningfulWord = 4

/** How many control names the sentence about what changed will list. */
const namesListed = 5

/**
 * The words of four letters or more that say nothing about what a window did.
 * Anything not here and long enough is treated as meaning something.
 */
const stopWords = new Set([
  "about", "after", "again", "also", "another", "back", "because", "been", "before", "being",
  "both", "does", "done", "down", "each", "else", "even", "ever", "every", "from",
  "have", "here", "into", "just", "like", "made", "make", "many", "more", "most",
  "much", "must", "next", "only", "onto", "other", "over", "same", "shall", "should",
  "some", "such", "than", "that", "their", "them", "then", "there", "these", "they",
  "this", "those", "under", "upon", "very", "were", "what", "when", "where", "which",
  "while", "will", "with", "would", "your",
])

/** What one action did to the window, before the expectation is judged. */
export interface WindowChange {
  /** True when the window's title is not the one it had before the action. */
  titleChanged: boolean
  /** The title the window carries now. */
  title: string
  /** The controls that were not on the window before the action. */
  newMarks: Mark[]
  /** How many controls that were there before are gone. */
  goneMarks: number
  /** The control the action was aimed at, when it was aimed at one. */
  aimedAt?: Mark
  /** False when the window never came to rest within the settle limit. */
  settled?: boolean
}

/** The verdict on one expectation, in the two fields the protocol promises. */
export interface Verdict {
  /** True when the expectation was met. */
  expectationMet: boolean
  /** One sentence saying what happened instead, empty when it was met. */
  seen: string
}

/** meaningfulWords is the expectation cut down to the words that carry meaning. */
export function meaningfulWords(expectation: string): string[] {
  const found: string[] = []
  for (const word of expectation.toLowerCase().split(/[^a-z0-9]+/u)) {
    if (word.length < shortestMeaningfulWord || stopWords.has(word) || found.includes(word)) {
      continue
    }
    found.push(word)
  }
  return found
}

/** judgeExpectation applies the fixed rule from PROTOCOL.md to one action. */
export function judgeExpectation(expectation: string, change: WindowChange): Verdict {
  if (change.settled === false) {
    return { expectationMet: false, seen: `the window kept changing rather than settling; ${describeChange(change)}` }
  }
  const words = meaningfulWords(expectation)
  if (words.length === 0) {
    const somethingChanged = change.titleChanged || change.newMarks.length > 0 || change.goneMarks > 0
    return somethingChanged ? { expectationMet: true, seen: "" } : { expectationMet: false, seen: describeChange(change) }
  }
  const haystack = whereWordsAreLookedFor(change)
  if (words.some((word) => haystack.includes(word))) {
    return { expectationMet: true, seen: "" }
  }
  return { expectationMet: false, seen: describeChange(change) }
}

/** whereWordsAreLookedFor is the one lowercase text the rule searches. */
function whereWordsAreLookedFor(change: WindowChange): string {
  const parts: string[] = []
  if (change.titleChanged) {
    parts.push(change.title)
  }
  for (const mark of change.newMarks) {
    parts.push(mark.name, mark.role)
  }
  if (change.aimedAt) {
    parts.push(change.aimedAt.name, change.aimedAt.role)
  }
  return parts.join(" ").toLowerCase()
}

/** describeChange says in one sentence what the action did to the window. */
export function describeChange(change: WindowChange): string {
  const parts: string[] = []
  if (change.titleChanged) {
    parts.push(`the title changed to ${change.title}`)
  }
  if (change.newMarks.length > 0) {
    parts.push(`${countWord(change.newMarks.length)} new ${plural("control", change.newMarks.length)} appeared: ${listNames(change.newMarks)}`)
  }
  if (change.goneMarks > 0) {
    parts.push(`${countWord(change.goneMarks)} ${plural("control", change.goneMarks)} went away`)
  }
  return parts.length === 0 ? "nothing changed" : parts.join(" and ")
}

/** listNames names the first few controls and counts the rest. */
function listNames(marks: Mark[]): string {
  const shown = marks.slice(0, namesListed).map((mark) => mark.name)
  const rest = marks.length - shown.length
  return rest > 0 ? `${shown.join(", ")} and ${rest} more` : shown.join(", ")
}

/** The count words a sentence reads better with than digits. */
const countWords = ["no", "one", "two", "three", "four", "five"]

/** countWord writes a small count as a word and a larger one as a number. */
function countWord(count: number): string {
  return countWords[count] ?? String(count)
}

/** plural adds the s an English count needs. */
function plural(word: string, count: number): string {
  return count === 1 ? word : `${word}s`
}
