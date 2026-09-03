// Package replay re-runs a recorded task against the code as it stands now,
// with the model and the tools answering out of the event log.
//
// This is the fourth tier of the self-fixing table in design section 11: a task
// that went wrong is replayed as a test once somebody has made a fix. Nothing
// new is asked of the model and nothing new is asked of the world. The log
// already holds what the model said and what every tool gave back, so the
// replay hands those same answers to the turn loop and watches what the loop
// does with them. What is left free is the harness itself: the loop, the
// record, the working context, and the permission function. A replay that ends
// with the same record and the same done-check answer says the fix changed
// nothing it should not have; a replay that ends somewhere else names the first
// step where the two runs parted company.
//
// The package also holds the nightly self-check, which is a job with a daily
// schedule whose task asks the memory twenty questions, runs every skill's dry
// run, and sends the user one line saying how many passed and which failed.
package replay
