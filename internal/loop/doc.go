// Package loop runs one turn of work: orient, call the model, guard, permit,
// run the tools, update the record, and repeat until the task ends.
//
// This is the core of Coeus. One task runs at a time, and every dependency it
// has is an interface in internal/contract, so the whole of it is driven by the
// fakes in internal/testkit in tests. A task starts from a message or from a
// job's next task and ends in one of four states: done, waiting on the user,
// stopped because a line of the stop list fired, or failed. Before any tool
// runs, the guard checks the stop list, the budget, and whether the model is
// asking for the same thing over and over, and the permission function rules on
// the call. After a tool runs, its one line goes into the task record and its
// whole text goes into the event log, where "read r7" can fetch it back. A task
// cannot close until every line of its done list points at something that
// proves it, and a task worth reviewing is reviewed in four questions whose
// last answer is the only thing kept.
package loop
