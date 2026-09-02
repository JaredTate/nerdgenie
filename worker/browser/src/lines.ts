/**
 * Splitting standard input into lines, with a cap.
 *
 * The Go side sends one JSON-RPC request per line. A sender that never sends a
 * newline would otherwise grow the worker's memory without end, so the reader
 * holds at most one line's worth: past the cap it reports the line as unreadable
 * and throws away everything up to the next newline.
 */

/** How to read lines, and what to do with each one. */
export interface LineReaderSettings {
  /** The most bytes one line may hold. */
  mostBytes: number;
  /** Called once per complete line, without its newline. */
  onLine(line: string): void;
  /** Called once for a line that went past the cap, which is answered with -32700. */
  onTooLong(): void;
}

/**
 * Build a reader. Feed it each chunk as it arrives; it calls back for every
 * complete line and never keeps more than the cap.
 */
export function createLineReader(settings: LineReaderSettings): (chunk: Buffer) => void {
  let pending: Buffer<ArrayBufferLike> = Buffer.alloc(0);
  let skippingToNextLine = false;

  function handOver(line: Buffer<ArrayBufferLike>): void {
    settings.onLine(line.toString("utf8").replace(/\r$/, ""));
  }

  return (chunk: Buffer): void => {
    pending = pending.length === 0 ? chunk : Buffer.concat([pending, chunk]);
    for (;;) {
      const end = pending.indexOf(0x0a);
      if (end === -1) {
        break;
      }
      const line = pending.subarray(0, end);
      pending = pending.subarray(end + 1);
      if (skippingToNextLine) {
        skippingToNextLine = false;
        continue;
      }
      if (line.length > settings.mostBytes) {
        settings.onTooLong();
        continue;
      }
      handOver(line);
    }
    if (pending.length > settings.mostBytes) {
      if (!skippingToNextLine) {
        settings.onTooLong();
        skippingToNextLine = true;
      }
      pending = Buffer.alloc(0);
    }
  };
}
