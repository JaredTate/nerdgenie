// Package memory is what the agent knows across tasks: two small files of
// facts, a folder of notes, and a full-text index over all of it and every past
// message.
//
// MEMORY.md holds facts about the world and USER.md holds facts about the user.
// Both sit in the persona folder and both have a hard size limit from
// contract.MemoryCaps, because a memory file with no limit grows until it
// crowds out the task. A save that would push a file past its limit does not
// fail: the oldest facts in that file move into a dated note in the memory
// folder, the move is written into the event log as a file change, and the
// facts stay searchable and stay readable by their ids. Every fact is one line
// carrying its id, the date it was written down, where it came from, and the
// fact this one supersedes when it replaces an earlier one. Nothing is ever
// deleted: a new fact names the one it replaces, and the old one stays in the
// index and comes back from a search marked as superseded.
//
// The index lives in the one SQLite file beside the event log, in tables this
// package owns and creates itself; internal/log owns the events table and the
// schema version, and this package only reads events through contract.Store.
// The index is a full-text search table, which the pure-Go SQLite driver
// supports, ranked by the relevance measure that table provides. A search
// returns the best match first and, among matches that score the same, the
// newest first; a search with no words in it returns the newest facts. An
// indexer keeps the index current: it runs at every save, on each new message
// event the loop hands it, and once when the memory is opened, and every run of
// it is capped so that a very large log or a very full memory folder cannot
// take an unbounded amount of work.
//
// Two things the harness does cost no model tokens at all. Capture reads the
// events of a finished task and writes down the files it changed, the commands
// it ran, the sites it visited, the jobs it created, and any message from the
// user that begins with "no", "actually", "always", "never", or "don't", which
// is kept word for word as a correction. A task with more events than one read
// of the event log returns is read the rest of the way in pages, because a
// capture that took the first page for the whole task would forget everything
// the task did after it. Hint takes the text of the step the agent is on and
// returns at most three short lines to ride along at the end of the prompt, and
// nothing at all when the step offers no word worth searching for or when
// nothing holds enough of them. The hint is stricter than a search on purpose,
// because it rides in the model's context on every turn whether it is wanted or
// not: it searches only on the step's words of four runes or more that are not
// stop words, it leaves out every fact something later replaced, and it keeps
// only lines holding at least two of the step's own words.
package memory
