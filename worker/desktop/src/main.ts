// The desktop worker's entry point. Everything it does is in worker.ts, which
// the tests drive with streams of their own; this file only hands it the real
// standard input, standard output, and standard error.

import { runWorker } from "./worker.js"

runWorker({
  argv: process.argv.slice(2),
  input: process.stdin,
  output: process.stdout,
  errors: process.stderr,
}).then(
  () => {
    process.exitCode = 0
  },
  (failure: unknown) => {
    const said = failure instanceof Error ? failure.message : String(failure)
    process.stderr.write(`the desktop worker stopped because ${said}\n`)
    process.exitCode = 1
  },
)
