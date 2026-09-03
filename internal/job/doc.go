// Package job is the job records, their task lists, and the scheduler behind
// them.
//
// A job is a piece of work too big for one sitting. It has the same four parts
// as a task record and the same rules, kept by internal/record: its plan is a
// list of tasks rather than a list of steps, and its results are the reports
// those tasks wrote. This package holds the jobs, hands the loop the next task
// that may start, takes each finished task's report back, and runs the schedule
// of the jobs that have one.
//
// Everything durable lives in the one database file. The record itself is a
// numbered checkpoint in the event log, so a job is rebuilt by replaying its
// events and nothing is ever held only in memory. The state around the record,
// which is the job's schedule, where it stands, how many of its tasks have
// failed in a row, and its notepad, is written into the same log as events of
// its own. The one thing that is not an event is the claim on a running task:
// that is a row in a small table this package owns, taken with a single
// conditional update, so that two processes reading the same file can never
// start one task twice.
//
// A job with a schedule is an ordinary job that makes one task from its
// template every time the clock says so, which is why "/cron" is only "/jobs"
// with the scheduled ones picked out. A tick that fails pushes the next one
// back, thirty seconds at first and an hour at the furthest; three failed tasks
// in a row pause a plain job, and ten switch a scheduled one off, each with a
// line to the user saying so, unless the job was told to keep running because
// its work is to report what it finds. A job may never make work that restarts
// the agent, because a job that restarts the agent starts itself again.
package job
