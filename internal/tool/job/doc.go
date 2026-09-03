// Package job is the job tool: it creates a piece of work too big for one
// sitting, adds a task to one, and lists what there is.
//
// A job is the same four parts as a task record, except that its plan is a list
// of tasks and its results are their reports, and everything about how one is
// kept and scheduled belongs to the job package behind the contract. This tool
// only reads the model's three words, turns the schedule the model wrote into
// the one the contract carries, and calls the method each word means. Making a
// job is the one thing here that cannot be undone, which is why the tool carries
// that class.
package job
