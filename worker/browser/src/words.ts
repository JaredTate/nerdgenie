/**
 * Splitting English into the words worth looking for.
 *
 * The expectation rule and the wall detector both need the same thing: take a
 * sentence and keep the words that carry meaning. A word carries meaning when it
 * is four or more letters long and is not a stop word. A stop word is a common
 * word that would match a page by accident: if "page" counted, almost any
 * expectation would be met by almost any page, and the check would be worthless.
 */

/**
 * The stop words. Every entry is four or more letters, because shorter words are
 * already dropped by the length rule, and every entry is a word that turns up in
 * ordinary web pages often enough to match by accident.
 */
export const STOP_WORDS: ReadonlySet<string> = new Set([
  "about",
  "above",
  "after",
  "again",
  "against",
  "already",
  "also",
  "although",
  "always",
  "among",
  "because",
  "been",
  "before",
  "being",
  "below",
  "beside",
  "between",
  "both",
  "coming",
  "could",
  "does",
  "doing",
  "done",
  "during",
  "each",
  "either",
  "else",
  "ever",
  "every",
  "from",
  "further",
  "have",
  "having",
  "here",
  "hers",
  "into",
  "just",
  "less",
  "like",
  "look",
  "make",
  "many",
  "might",
  "more",
  "most",
  "much",
  "must",
  "near",
  "neither",
  "next",
  "none",
  "once",
  "only",
  "onto",
  "other",
  "over",
  "page",
  "please",
  "same",
  "shall",
  "should",
  "since",
  "some",
  "such",
  "than",
  "that",
  "their",
  "them",
  "then",
  "there",
  "these",
  "they",
  "this",
  "those",
  "through",
  "thus",
  "until",
  "upon",
  "very",
  "were",
  "what",
  "when",
  "where",
  "which",
  "while",
  "whose",
  "will",
  "with",
  "within",
  "without",
  "would",
  "your",
  "yours",
]);

/** The fewest letters a word must have before it is worth looking for. */
export const SHORTEST_MEANINGFUL_WORD = 4;

/**
 * Split a sentence into the words worth looking for: lowercased, split on
 * anything that is not a letter or a digit, four letters or more, no stop words,
 * and no repeats. The order is the order they were written in.
 */
export function meaningfulWords(sentence: string): string[] {
  const kept: string[] = [];
  const seen = new Set<string>();
  for (const token of sentence.toLowerCase().split(/[^a-z0-9]+/)) {
    if ((token.length < SHORTEST_MEANINGFUL_WORD && !isANumber(token)) || STOP_WORDS.has(token) || seen.has(token)) {
      continue;
    }
    seen.add(token);
    kept.push(token);
  }
  return kept;
}

/** Does the text hold any of these words? The comparison ignores capital letters. */
export function textHoldsAnyWord(text: string, words: readonly string[]): boolean {
  const lowered = text.toLowerCase();
  return words.some((word) => lowered.includes(word));
}

/** A token made only of digits is meaningful however short: "1" is the whole of what a counter says. */
function isANumber(token: string): boolean {
  return token.length > 0 && /^[0-9]+$/.test(token);
}
