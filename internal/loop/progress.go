package loop

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The progress meter. The same-call guard sees a call repeated word for word;
// it is blind to a stall that changes a character each round, which is what
// the fourth game build's sixty rounds on two tests looked like, and what the
// fifth build's eleven launches of an unnamed application looked like once
// their intents were counted. Progress is something the harness can see for
// itself: a test run that improves, a plan step or a done line marked, a page
// that changed under an action, a new file written, a look at something not
// looked at before. Rounds with none of it
// are counted, and the count climbs a ladder: one line in the harness's own
// words, then the conversation cleared with the stall written into the
// record, then, when the cleared conversation stalls the same way again, a
// stop that asks the person.
const (
	// NudgeAfterRoundsWithoutProgress is when the model is sent TheStallLine.
	NudgeAfterRoundsWithoutProgress = 10
	// RewindAfterRoundsWithoutProgress is when the conversation is cleared, the
	// first time, and when the task stops, the second time.
	RewindAfterRoundsWithoutProgress = 20
)

// TheStallLine is what the model reads after NudgeAfterRoundsWithoutProgress
// rounds in which nothing the harness can measure moved.
const TheStallLine = "That is ten rounds in which no test went green, no plan step or done line was marked, no page changed under an action, no new file was written and nothing new was read. Write what these rounds showed into the record as a failure with its cause, then take a different approach; more of the same will not move the count."

// RoundsAfterAllMarked is how many rounds of tool calls a task may go on
// for once every done line and every plan step is marked done before it is
// told to answer: the fresh Tetris build's yeti task had everything marked by
// round 174 and went on into the next task's work for twenty rounds more.
const RoundsAfterAllMarked = 3

// TheAllMarkedLine is what the model reads then.
const TheAllMarkedLine = "Every done line and every plan step is marked done. Answer the user now to end this task; anything more belongs to the next task, or to a new done line."

// TheStallLineAfterAFailure opens the nudge said instead of TheStallLine when
// the model wrote a failure within the last ten rounds: the nightly game
// build wrote its line-clear failure, was told by the stall line, which stays
// in the window, to write what the rounds showed as a failure, and wrote the
// same one six more times, each refused as already held, until the same-call
// guard stopped the task. The nudge after a fresh failure names it, quotes its
// cause, and asks for a change and no more failures.
const TheStallLineAfterAFailure = "That is ten rounds in which no test went green, no plan step or done line was marked, no page changed under an action, no new file was written and nothing new was read, and the record already holds "

// theToolsThatRead are the tools whose answer is new information when what
// they are pointed at is new: a task that reads a different file every round
// is moving, and one that reads the same file every round is not.
var theToolsThatRead = []string{contract.ToolRead, "search", "web", "browser_open", "browser_read", contract.ToolBrowserResize}

// noteAReadOfSomethingNew reads a look at something this task has not looked
// at before as progress. The fourth game build's stall re-read one file; a
// research task that reads a hundred different files is a hundred rounds of
// progress, and the meter must not stop it.
func (running *run) noteAReadOfSomethingNew(call contract.ToolCall) {
	if !slices.Contains(theToolsThatRead, call.Name) {
		return
	}
	key := call.Name + "\x00" + whatTheCallSays(call)
	if running.thingsRead == nil {
		running.thingsRead = map[string]bool{}
	}
	if !running.thingsRead[key] {
		running.thingsRead[key] = true
		running.noteProgress()
	}
}

// noteAResultThatSaysSomethingNew reads a look or a command as progress when
// what came back is something this task has not seen before, whatever the
// call was. The twelfth nightly run's engine task, with two tests left red,
// read its five-hundred-line engine in eighty-line windows, probed it with
// small node scripts and searched it with grep, each answering something new,
// and the meter counted the file once and the commands not at all, so it
// cleared the conversation in the middle of the search. The same answer
// again is more of the same, however the call was worded, which is what the
// nudge after a failure stands on; a test run is judged by its own rule, and
// a poll is waiting.
func (running *run) noteAResultThatSaysSomethingNew(call contract.ToolCall, text string) {
	if call.Name != contract.ToolShell && !slices.Contains(theToolsThatRead, call.Name) {
		return
	}
	if onlyPolls([]contract.ToolCall{call}) {
		return
	}
	if _, isATestRun := testStateIn(text); isATestRun {
		return
	}
	said := strings.TrimSpace(text)
	if said == "" {
		return
	}
	if running.thingsSaid == nil {
		running.thingsSaid = map[string]bool{}
	}
	if !running.thingsSaid[said] {
		running.thingsSaid[said] = true
		running.noteProgress()
	}
}

// theSignsOfAChangedPage are the first lines of a browser result that mean
// the action did something to the page.
var theSignsOfAChangedPage = []string{"what was expected happened", "the page moved to", "new on the page:", "a new tab opened"}

// noteProgress says the round moved the work: the count starts again.
func (running *run) noteProgress() {
	running.progressThisRound = true
}

// noteTheTestsImprovedOrNot reads a test run as progress when it is the first
// run seen or fewer tests fail than on the run before, and remembers the
// count for the next run.
func (running *run) noteTheTestsImprovedOrNot(state testState) {
	if !running.sawATestRun || state.failed < running.lastFailedCount {
		running.noteProgress()
	}
	running.sawATestRun = true
	running.lastFailedCount = state.failed
}

// noteAChangedPage reads a browser action's result as progress when its
// first line says the page changed under it.
func (running *run) noteAChangedPage(text string) {
	first := strings.ToLower(firstLine(text))
	for _, sign := range theSignsOfAChangedPage {
		if strings.HasPrefix(first, sign) {
			running.noteProgress()
			return
		}
	}
}

// noteAllMarked counts the rounds of tool calls since every done line and
// every plan step was marked done, and at RoundsAfterAllMarked says to answer.
func (running *run) noteAllMarked() {
	if !running.allMarked() {
		running.roundsAllMarked = 0
		return
	}
	running.roundsAllMarked++
	if running.roundsAllMarked == RoundsAfterAllMarked {
		running.remember(contract.Message{Role: contract.RoleUser, Text: TheAllMarkedLine})
	}
}

// allMarked says whether the record holds a done list with every line done and
// no plan step left open.
func (running *run) allMarked() bool {
	if running.keeper == nil {
		return false
	}
	held := running.keeper.Record()
	if len(held.Goal.DoneWhen) == 0 {
		return false
	}
	for _, line := range held.Goal.DoneWhen {
		if !line.Done {
			return false
		}
	}
	for _, step := range held.Work.Plan {
		if !step.Done {
			return false
		}
	}
	return true
}

// marksMade counts the plan steps and the done lines the record holds marked
// done, which is what the round is measured against.
func (running *run) marksMade() int {
	if running.keeper == nil {
		return 0
	}
	held := running.keeper.Record()
	marks := 0
	for _, step := range held.Work.Plan {
		if step.Done {
			marks++
		}
	}
	for _, line := range held.Goal.DoneWhen {
		if line.Done {
			marks++
		}
	}
	return marks
}

// onlyPolls says whether every call of the round asked after a command that
// is still running, which is waiting rather than a round of work.
func onlyPolls(calls []contract.ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		if call.Name != contract.ToolShell {
			return false
		}
		if action := fieldOfCall(call, "action"); action != "poll" && action != "tail" {
			return false
		}
	}
	return true
}

// countTheRound settles the round against the meter once its calls have run:
// progress starts the count again, a round of polls is not counted, and the
// count climbing to a rung of the ladder does what that rung says. It hands
// back the ending when the rung is the stop.
func (running *run) countTheRound(ctx context.Context, marksBefore int, calls []contract.ToolCall) (*Outcome, error) {
	moved := running.progressThisRound || running.marksMade() > marksBefore
	running.progressThisRound = false
	running.roundsSinceAFailureWrite++
	running.noteAllMarked()
	switch {
	case moved:
		running.roundsSinceProgress = 0
		return nil, nil
	case onlyPolls(calls):
		return nil, nil
	}
	running.roundsSinceProgress++
	switch running.roundsSinceProgress {
	case NudgeAfterRoundsWithoutProgress:
		running.remember(contract.Message{Role: contract.RoleUser, Text: running.theNudge()})
	case RewindAfterRoundsWithoutProgress:
		if running.stallsAfterARewind > 0 {
			ended, err := running.stopHere(ctx, fmt.Sprintf("%d rounds without progress, twice over", RewindAfterRoundsWithoutProgress))
			return &ended, err
		}
		running.stallsAfterARewind++
		running.rewindDue = true
		running.stallText = fmt.Sprintf("stalled: %d rounds in which no test went green, no step or done line was marked, no page changed, no new file was written and nothing new was read, so the conversation was cleared",
			RewindAfterRoundsWithoutProgress)
	}
	return nil, nil
}

// theNudge is the line said at ten rounds without progress: the plain stall
// line, which asks for a failure with its cause, or, when the model wrote a
// failure within the last ten rounds, the line that names that failure,
// quotes its cause, and asks for a change and no more failures.
func (running *run) theNudge() string {
	// A failure written within the ten rounds the meter counts, or the two
	// before them, is a fresh one: the round it was written in and one round
	// of reading something new both sit before the count.
	if running.keeper == nil || running.roundsSinceAFailureWrite > NudgeAfterRoundsWithoutProgress+2 {
		return TheStallLine
	}
	failures := running.keeper.Record().Lessons.Failures
	if len(failures) == 0 {
		return TheStallLine
	}
	newest := failures[len(failures)-1]
	said := TheStallLineAfterAFailure + newest.ID
	if newest.Cause != "" {
		said += ", whose cause is \"" + newest.Cause + "\""
	}
	return said + ". Write no more failures: make the change it calls for, in the file it names, and run the tests."
}

// theProgressLine is the situation's line on the meter, and is empty while the
// work is moving.
func (running *run) theProgressLine() string {
	if running.roundsSinceProgress == 0 {
		return ""
	}
	return fmt.Sprintf("rounds since progress: %d", running.roundsSinceProgress)
}
