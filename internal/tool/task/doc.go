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
//
// The seven are strict; the way they are written is not. The operation is found
// by its plain name, so the case, the underscores and a leading set or add do
// not matter, and a call that names no operation at all is read from the fields
// it carries. Every field is read on its own, under either name a model gives
// it, and a field in a shape this tool cannot read is refused by name with the
// shapes it does take, because one badly written field must not cost the whole
// call. What comes back is the sentence naming what was written and the whole
// update as JSON under it.
package task
