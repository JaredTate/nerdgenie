// Package log is the append-only event log in SQLite that is the truth about
// everything that happened.
//
// One file holds one table of events. Every message, tool call, tool result,
// permission decision, file change, record change, checkpoint, and reply is one
// row in it, carrying a sequence number that only ever grows, the time it
// happened, the task it belongs to, its kind, and its own fields as JSON.
// Nothing is ever changed or removed, which is what lets the rest of Coeus treat
// the log as the ledger: a reply is written here before it is sent, so a crash
// between the writing and the sending leaves a record saying the reply may have
// gone out, and after a crash the agent replays the log to rebuild what it was
// doing. The log is also where the full text of a tool result lives after the
// short line about it in the task record has pushed the result out of the
// model's working context, which is what "read r7" fetches.
//
// The file is opened in write-ahead mode with one connection for writing, held
// behind a mutex so there is only ever one writer, and a few connections for
// reading. There are five ways to read it and no others: by task, by kind, by
// sequence number, by a span of sequence numbers, and a replay that hands every
// event to a function in order. The three that return a list stop at
// MaxEventsPerRead rows, so no read can pull the whole log into memory by
// accident. A read that fills that cap hands back the events it read together
// with an error naming the last of them, so a caller is never quietly given part
// of an answer; the way to read the rest is ByRange, page by page, or Replay,
// which streams and has no cap. Every call takes a context and gives up when it
// is cancelled. The caller supplies the time on each event, because time in
// Coeus is read from contract.Clock and never from the machine directly.
package log
