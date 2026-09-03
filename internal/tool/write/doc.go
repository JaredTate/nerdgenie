// Package write is the write tool: it creates a file or overwrites one, inside
// the folders the agent may work in, and records what the file held first.
//
// The recording is the point. Before a byte is written, what the file held, the
// mode it had, and whether it was there at all go into the event log as a
// file-change event, and only then is the file touched. That order is what makes
// the undo command of wave three true rather than hopeful: a change that was
// never recorded is never made. The same door is used by the edit tool, so that
// there is one shape of file-change event and one place that writes it. The
// content of one call is capped, because a model with a run away with it must
// not be able to fill a disk.
package write
