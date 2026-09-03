// Package task is the task tool: the only way the model writes anything into
// the task record.
//
// There are seven operations and no more: set the one line on why the user wants
// this, write the done list, write the stop list, write the plan, add a decision
// with the reason it must carry, add a failure with the cause it must carry, and
// point a done line at the result that proves it. There is nothing here for the
// ask, for a correction, for the header, for the situation, or for a result,
// because those belong to the user and to the harness, and the way to keep them
// safe is to give the model no door to them rather than a door with a lock on
// it. Every operation goes through the record's own rules, and a refused one
// hands the model back the rule it broke, word for word, so that it can fix the
// call rather than guess. It is meant to ride in the same reply as the model's
// other tool calls, so it does no work of its own and never waits on anything.
package task
