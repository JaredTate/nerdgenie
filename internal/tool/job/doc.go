// Package job is the job tool: it creates a piece of work too big for one
// sitting, adds a task to one, and lists what there is.
//
// A job is the same four parts as a task record, except that its plan is a list
// of tasks and its results are their reports, and everything about how one is
// kept and scheduled belongs to the job package behind the contract. This tool
// only reads the model's words, turns the schedule the model wrote into the one
// the contract carries, and calls the method each word means. A job without a
// schedule must name its first task, so that create records that task under the
// new job and leaves it for the loop to run next: a job that never carried a
// task is the bug this guards against. Making a job is the one thing here that
// cannot be undone, which is why the tool carries that class.
package job
