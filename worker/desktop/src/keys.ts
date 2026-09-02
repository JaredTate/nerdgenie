// Reading a key combination such as "ctrl+shift+s". The design is OpenClaw's
// key normalizer at ~/Code/openclaw/extensions/cua-computer/src/actions.ts: name
// the modifiers one way, name the special keys one way, and refuse a single
// character that is not a letter, because the driver carries a base key and not
// the shift state a keyboard layout puts on a punctuation mark. Written fresh
// for Coeus.

import { DesktopErrorCode, ProtocolError } from "./wire.js"

/** The most parts a combination may have: three modifiers and a key. */
export const maximumChordParts = 4

/** One key combination, in the shape the driver takes. */
export interface KeyChord {
  /** The one key that is not a modifier. */
  key: string
  /** The modifiers held down while it is pressed, in the order written. */
  modifiers: string[]
}

/** The many ways people write the four modifier keys. */
const modifierNames = new Map<string, string>([
  ["ctrl", "ctrl"],
  ["control", "ctrl"],
  ["shift", "shift"],
  ["alt", "alt"],
  ["option", "alt"],
  ["meta", "meta"],
  ["cmd", "meta"],
  ["command", "meta"],
  ["super", "meta"],
  ["win", "meta"],
  ["windows", "meta"],
])

/** The keys that have a name rather than a character. */
const namedKeys = new Map<string, string>([
  ["enter", "enter"],
  ["return", "enter"],
  ["tab", "tab"],
  ["escape", "escape"],
  ["esc", "escape"],
  ["space", "space"],
  ["backspace", "backspace"],
  ["delete", "delete"],
  ["del", "delete"],
  ["insert", "insert"],
  ["home", "home"],
  ["end", "end"],
  ["pageup", "pageup"],
  ["pgup", "pageup"],
  ["pagedown", "pagedown"],
  ["pgdn", "pagedown"],
  ["up", "up"],
  ["down", "down"],
  ["left", "left"],
  ["right", "right"],
])

for (let number = 1; number <= 12; number += 1) {
  namedKeys.set(`f${number}`, `f${number}`)
}

/** parseKeyChord turns "ctrl+shift+s" into the modifiers and the one key. */
export function parseKeyChord(written: string): KeyChord {
  const parts = written.split("+").map((part) => part.trim())
  if (parts.length > maximumChordParts) {
    throw badChord(`a key combination may have at most ${maximumChordParts} parts, and this one has ${parts.length}`)
  }
  const last = parts.pop()
  if (last === undefined || last === "") {
    throw badChord("the key combination is empty, so write the key to press, such as ctrl+s")
  }
  return { key: readKey(last), modifiers: parts.map(readModifier) }
}

/** readModifier turns one written modifier into the one name the driver takes. */
function readModifier(written: string): string {
  const found = modifierNames.get(written.toLowerCase())
  if (found === undefined) {
    throw badChord(`the modifier ${JSON.stringify(written)} is not one this desktop knows, so use ctrl, shift, alt, or meta`)
  }
  return found
}

/** readKey turns the last part of a combination into the key to press. */
function readKey(written: string): string {
  const lowered = written.toLowerCase()
  const named = namedKeys.get(lowered)
  if (named !== undefined) {
    return named
  }
  if (/^[a-z]$/u.test(lowered)) {
    return lowered
  }
  if (written.length === 1) {
    throw badChord(
      `the key ${JSON.stringify(written)} is one character that is not a letter, and its shift state depends on the keyboard layout, so type it with the type method instead`,
    )
  }
  throw badChord(`the key ${JSON.stringify(written)} is not one this desktop knows, so use a letter or a named key such as enter`)
}

/** badChord is the one failure this file reports, with the protocol's code on it. */
function badChord(message: string): ProtocolError {
  return new ProtocolError(DesktopErrorCode.BadParameters, message)
}
