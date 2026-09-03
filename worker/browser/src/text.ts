/**
 * The page's text, as the model reads it beside the elements.
 *
 * An outline of a page's elements shows what can be acted on and nothing else.
 * The first human trial asked for a number in a table whose cells were icon
 * buttons with no name, and the number was on nothing the model was shown. So a
 * snapshot also carries what the page says, as a person reads it, under a cap of
 * its own with a last line saying how much was cut.
 */

/** One frame's text as the page reported it: what fit under the cap, and how many characters did not. */
export interface FrameText {
  text: string;
  cut: number;
}

/** The line that ends a text that was cut. */
function cutLine(cut: number): string {
  return `... ${cut} more characters were cut`;
}

/**
 * Join the frames' text in order under one cap. A cut lands on a line end when
 * there is one under the cap, so that a row of a table is whole or not there,
 * and the last line says how much was cut, counting what the frames cut before
 * the text ever reached here.
 */
export function pageTextOf(frames: readonly FrameText[], mostCharacters: number): string {
  let cut = 0;
  const parts: string[] = [];
  for (const frame of frames) {
    cut += frame.cut;
    if (frame.text !== "") {
      parts.push(frame.text);
    }
  }
  let text = parts.join("\n");
  if (text.length > mostCharacters) {
    const lineEnd = text.lastIndexOf("\n", mostCharacters);
    const keep = lineEnd > 0 ? lineEnd : mostCharacters;
    cut += text.length - keep;
    text = text.slice(0, keep);
  }
  if (cut === 0) {
    return text;
  }
  return text === "" ? cutLine(cut) : `${text}\n${cutLine(cut)}`;
}
