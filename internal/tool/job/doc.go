// Package job is the job tool: it creates a piece of work too big for one
// sitting, adds a task to one, and lists what there is.
//
// A job is the same four parts as a task record, except that its plan is a list
// of tasks and its results are their reports, and everything about how one is
// kept and scheduled belongs to the job package behind the contract. This tool
// reads the model's words and the record of the task running now, takes the
// job's ask from that record word for word so the model never retypes or
// rewrites it, turns the schedule the model wrote into the one the contract
// carries, and calls the method each word means. A job without a schedule must
// list at least one task, and create writes the whole list under the new job in
// order and leaves the first for the loop to run next: a job that never carried
// a task is the bug this guards against. Making a job is the one thing here
// that cannot be undone, which is why the tool carries that class.
package job
