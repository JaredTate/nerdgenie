package loop

import "fmt"

// A reply cut off at the output cap is neither an answer nor a call. The tenth
// nightly run's frontend task wrote the whole game file in one write call, the
// reply hit the cap three rounds running, the harness read each cut-off call
// as unreadable arguments and offered the three options, and on the fourth
// round the model took the first, "answer the user", and the task closed done
// with no game file written. Now the model is told what happened and how to
// write a long file in parts, and none of the cut-off reply is kept, because
// half a file in the working context is a cost paid on every later call.

// MaxLinesInOneWrite is the size of one part of a long file, chosen so that a
// part of the densest code stays well under the output cap of 8192 tokens.
const MaxLinesInOneWrite = 300

// theCutOffLine is what the model is told after a reply cut off at the cap.
func theCutOffLine(tokens int) string {
	return fmt.Sprintf("Your reply was cut off at the output cap after %d tokens, and none of it was kept. "+
		"Write less in one reply: a long file goes in parts of at most %d lines, the first by write and the rest by edit.",
		tokens, MaxLinesInOneWrite)
}
