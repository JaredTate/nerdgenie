/**
 * The worker's own logging: one line per event, in plain English, on standard
 * error.
 *
 * Standard output carries responses and nothing else, because a stray line there
 * would break the framing the Go side reads. Everything the worker wants to say
 * about itself goes here instead.
 */

/** Somewhere a line of logging can go. */
export type Logger = (line: string) => void;

/** Write a line to standard error. */
export const toStandardError: Logger = (line) => {
  process.stderr.write(`${line}\n`);
};

/** Throw away every line, for a worker that should say nothing. */
export const toNowhere: Logger = () => {};

/**
 * Wrap a place to write in the worker's own shape: one line, one event, always
 * starting with who is speaking, and never a newline in the middle to break it
 * in two.
 */
export function prefixed(sink: Logger): Logger {
  return (message) => {
    sink(`browser worker: ${message.replace(/\s+/g, " ").trim()}`);
  };
}
