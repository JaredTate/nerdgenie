/**
 * Taking the vault's secrets back out of anything the worker is about to say.
 *
 * `loginFill` is handed a username, a password, and a code, types them into the
 * page, and must never hand any of them back. A page can echo a value into a
 * field's name, a title, or an address, so the safe rule is not "do not read
 * those fields" but "walk the whole answer and replace every occurrence". The
 * test for this greps the entire written-out response.
 *
 * The idea of never letting a password reach the model, because a page could
 * talk the model into repeating it, is borrowed from browser-use's serializer at
 * docs/reference/browser-use/serializer.py, which refuses to emit the value of a
 * password field. The code here is written fresh and goes further: it replaces
 * the value wherever it turns up, not only in the field it was typed into.
 */

/** What a secret is replaced with. */
export const REDACTED = "[redacted]";

/** How deep the walk goes before it gives up, so a looping object cannot trap it. */
const MOST_LEVELS = 24;

/**
 * What stands in for anything deeper than the cap. It is a marker rather than the
 * value itself, because handing back an unwalked subtree would hand back a secret
 * the walk never reached.
 */
export const TOO_DEEP = "[too deep to check]";

/** Sort the secrets longest first and drop the blank ones. */
function usableSecrets(secrets: readonly string[]): string[] {
  return secrets
    .filter((secret) => secret.trim() !== "")
    .sort((left, right) => right.length - left.length);
}

/** Replace every occurrence of every secret in one piece of text, ignoring capital letters. */
export function redactText(text: string, secrets: readonly string[]): string {
  let cleaned = text;
  for (const secret of usableSecrets(secrets)) {
    const lowered = cleaned.toLowerCase();
    const wanted = secret.toLowerCase();
    if (!lowered.includes(wanted)) {
      continue;
    }
    // Split on the lower-cased copy so that the pieces line up with the original,
    // then rebuild from the original text. This keeps the search
    // case-insensitive without ever treating the secret as a pattern.
    let rebuilt = "";
    let from = 0;
    for (;;) {
      const at = cleaned.toLowerCase().indexOf(wanted, from);
      if (at === -1) {
        rebuilt += cleaned.slice(from);
        break;
      }
      rebuilt += cleaned.slice(from, at) + REDACTED;
      from = at + wanted.length;
    }
    cleaned = rebuilt;
  }
  return cleaned;
}

function redactAtLevel(value: unknown, secrets: readonly string[], level: number): unknown {
  if (typeof value === "string") {
    return redactText(value, secrets);
  }
  if (value === null || typeof value !== "object") {
    return value;
  }
  if (level >= MOST_LEVELS) {
    return TOO_DEEP;
  }
  if (Array.isArray(value)) {
    return value.map((entry) => redactAtLevel(entry, secrets, level + 1));
  }
  const cleaned: Record<string, unknown> = {};
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    cleaned[redactText(key, secrets)] = redactAtLevel(entry, secrets, level + 1);
  }
  return cleaned;
}

/**
 * Walk a whole value and replace every occurrence of every secret, in the text,
 * in nested objects and arrays, and in the keys. Numbers, booleans, and nulls
 * come back exactly as they were.
 */
export function redactDeep<T>(value: T, secrets: readonly string[]): unknown {
  if (usableSecrets(secrets).length === 0) {
    return value;
  }
  return redactAtLevel(value, secrets, 0);
}
