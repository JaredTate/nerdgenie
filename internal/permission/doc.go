// Package permission decides allow, ask, or deny for every tool call.
//
// The model decides what to do and this package decides what is allowed, and the
// two never share a layer. Every call is first reduced to a readable form: a
// shell command cut down to its program and the words that matter, or any other
// tool's name and the one field that says what it will do. That readable form is
// what the rules match on, what the event log records, and what the user is
// shown. A readable form is either the whole story or it says so out loud: a
// command line the reducer could not read to the end, and a command that works
// out part of itself while it runs, both end with a note saying what is left
// out, and a call carrying such a note is put to the user even when no rule
// covers it, because a form that hides the end of a command cannot be ruled on.
// A script handed to a shell is a command line in its own right, so it is
// reduced as one, and so is a command written behind a program whose one job is
// to run another program, such as nohup, timeout, xargs or env. The rules come
// from the user's ask-me-first list, which ships with three entries and which
// the user may add to or empty; the last rule that matches wins, and a call no
// rule matches runs on its own. A shell command is matched in a second form as
// well, with its flags spelled out one to a pair of brackets, so that a rule
// about a flag holds whatever order the flags were written in; nobody is shown
// that form. When the answer is to ask, the decision carries a preview of
// exactly what is about to happen.
//
// The order the answers are read in is what keeps the list a list: a rule that
// refuses, then the answer the user gave in this session, then the ask-me-first
// list, and only then the standing approval a skill may hold with a limit and an
// expiry, so that a skill can never hand itself the things the user asked to see
// first. A run with nobody there to answer is ruled stop before any approval is
// read, so the task reports what it needed rather than waiting forever.
package permission
