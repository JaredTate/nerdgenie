// Package record is the task record and the job record: parsed, printed, ruled,
// and checkpointed.
//
// A record is what one piece of work knows about itself, written in the four
// parts of an operations order: the goal, the rules, the work, and the lessons.
// It is what the model reads on every call in place of a transcript, so it has
// one text form and only the printer in this package writes it. Every change to
// a record goes through a method here, because the record's worth comes from its
// rules: the user's ask and corrections are never edited, a decision must carry
// a reason and a failure a cause, anything marked done must name the result that
// proves it, and every result keeps its full text in the event log where "read
// r7" can fetch it back long after the text left the model's window. Every
// change also saves a numbered checkpoint into that same log, so a task can be
// put down for days, picked up on a different model, or wound back three steps
// to try another path.
//
// A job record is the same four parts with the same rules. Its work holds a list
// of tasks rather than a list of plan steps, its results are the reports those
// tasks produced, and its header counts progress rather than budget. The kind
// field on the header says which of the two a record is, and this package
// refuses an operation that belongs to the other kind.
package record
