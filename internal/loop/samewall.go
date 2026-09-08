// The same-wall detector. The guard (guard.go) catches the same call coming
// back with the same result, and the progress meter (progress.go) catches
// rounds in which nothing new was marked, read, written or changed. Between the
// two is a gap: a model can stay busy — a new file each round, a new command, a
// new thing read — while the one result that measures its goal never moves, and
// that masks the stall from both, because each net reads "did something new" as
// "made progress". On 8 September 2026 the tic-tac-toe-against-the-computer task
// ran past ninety rounds with its test suite frozen at twelve failing, editing
// a different file almost every round, and neither net fired until the meter had
// finally counted twenty rounds the slow way. This detector watches for one
// call coming back with the same result over and over, whatever the calls
// between: the guard would catch that too, but it starts its count again the
// moment a call in between returns something new, so the new files masked the
// frozen check from it. When the same wall is hit enough times, the same
// rethink the other nets use asks the model to question what it took for
// granted — including its own test.
//
// What it leaves to others by design: the same frozen result reached through
// different commands each round (npm test, then node --test) has a different
// mark each time and never builds a pair, but each result is one the task has
// seen, so the progress meter's own count catches it; a test runner's changing
// duration is left in the fingerprint on purpose, so a stuck run is the
// test-state tracker's (teststate.go) to name first; and a poll or a tail is
// left out here (isAPollOrTail), while a repeated non-shell read of an
// unchanging page or file is not, because six identical reads with no step
// marked between them is a wall like any other.

package loop

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The numbers the same-wall detector works to.
const (
	// SameWallWindow is how many of the newest results it remembers. Wide
	// enough that a wall hit between other work is still all in view, and
	// bounded so the memory is small.
	SameWallWindow = 24
	// SameWallRecurrences is how many times one result may come back inside
	// the window before the wall is called. Six: high enough that ordinary
	// work never trips it, because a check that is converging returns
	// something new as it goes, and low enough to catch a model circling one
	// frozen result long before the meter's twenty rounds would.
	SameWallRecurrences = 6
)

// walledResult is one result the detector remembers: the fingerprint of what
// came back, with the volatile tokens taken out, and the mark of the call that
// produced it, so the wall can name the call that kept hitting it.
type walledResult struct {
	mark        string
	fingerprint string
}

// sameWall remembers the newest results of a task, to see when one of them
// keeps coming back.
type sameWall struct {
	recent []walledResult
}

// note remembers one result as a normalized fingerprint against the call that
// produced it, and forgets the oldest when the window is full. The caller
// leaves out a poll or a tail, whose repeats are waiting, not a wall.
func (wall *sameWall) note(mark string, resultText string) {
	wall.recent = append(wall.recent, walledResult{mark: mark, fingerprint: normalizedFingerprint(resultText)})
	if len(wall.recent) > SameWallWindow {
		wall.recent = wall.recent[len(wall.recent)-SameWallWindow:]
	}
}

// hit says whether one call has come back with the same result
// SameWallRecurrences times inside the window, and the mark of that call. It
// counts a call-and-result pair wherever it sits in the window, whatever else
// the model did between the recurrences, which is the one thing the guard does
// not: the guard starts its count again the moment a call in between comes back
// with something new (guard.go, sameResultStreak), so a model that runs the
// same frozen check between edits that each write a new file is invisible to
// it. A pair, not a bare result, so that an unchanging confirmation from a
// changing call — "edited the file" after each of a hundred different edits —
// is not read as a wall while the work is really moving.
func (wall *sameWall) hit() (string, bool) {
	counts := map[string]int{}
	for _, past := range wall.recent {
		key := past.mark + "\x00" + past.fingerprint
		counts[key]++
		if counts[key] >= SameWallRecurrences {
			return past.mark, true
		}
	}
	return "", false
}

// forget empties the window, which is what a fresh window does — a rethink or a
// cut — so the recurrences before it do not fire the wall again at once.
func (wall *sameWall) forget() {
	wall.recent = nil
}

// isAPollOrTail says whether a call is a shell poll or tail, whose result is
// meant to be read again and again while something runs, and so is not counted
// toward a wall.
func isAPollOrTail(call contract.ToolCall) bool {
	if call.Name != contract.ToolShell {
		return false
	}
	action := fieldOfCall(call, "action")
	return action == "poll" || action == "tail"
}

// wallStalls acts on a wall the round hit: one result that has come back over
// and over whatever else the model did. It buys a rethink the way the meter's
// rewind does (progress.go), closing the newest call that is not the tests so
// the model must change something before it checks again, and it stops the
// task when the stalls have run out, on the same budget as the meter. It says
// whether it fired. Nothing here fails the task on its own. It leaves a wall
// the guard or the meter already caught this round to them, so a stall is
// never counted twice.
func (running *run) wallStalls(ctx context.Context, calls []contract.ToolCall) (*Outcome, bool, error) {
	if running.rewindDue {
		return nil, false, nil
	}
	// A wall is only a wall when the work is not otherwise moving. A step or a
	// done line marked within the last SameWallRecurrences rounds means the
	// model is getting somewhere, so a result that keeps coming back beside
	// real progress — a status check run again and again while steps are being
	// marked — is left alone rather than stopped.
	if running.sinceAMarkMade < SameWallRecurrences {
		return nil, false, nil
	}
	if _, hit := running.wall.hit(); !hit {
		return nil, false, nil
	}
	running.wall.forget()
	if running.stallsAfterARewind >= StallsBeforeStop-1 {
		ended, err := running.stopOnTheGuard(ctx, fmt.Sprintf(
			"the same result came back %d times and nothing the model changed moved it, %d times over",
			SameWallRecurrences, StallsBeforeStop))
		return &ended, true, err
	}
	running.stallsAfterARewind++
	running.rewindDue = true
	running.stallText = fmt.Sprintf(
		"stalled: the same result came back %d times while the model kept working on other things, so it has hit a wall and nothing it changed has moved it",
		SameWallRecurrences)
	running.stallMark = running.theMarkToCloseAfterTheMeter(calls)
	return nil, true, nil
}
