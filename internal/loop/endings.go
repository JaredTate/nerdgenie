package loop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// endOfTurn is what happens when the model replies with no tool calls. The
// harness tells a question from an answer by the finish state and by what the
// reply has behind it: a question puts the task into waiting, and an answer goes
// to the done-check.
func (running *run) endOfTurn(ctx context.Context, text string, why contract.FinishReason) (Outcome, bool, error) {
	if err := running.writeSituation(ctx); err != nil {
		return Outcome{}, false, err
	}
	if running.isAQuestion(text, why) {
		outcome, err := running.waitHere(ctx, text)
		return outcome, false, err
	}
	if running.keeper == nil {
		outcome, err := running.answerWithNoRecord(ctx, text)
		return outcome, false, err
	}
	if err := running.theReplyProvesItsLines(ctx, text); err != nil {
		return Outcome{}, false, err
	}
	problem, err := running.doneCheck(ctx)
	if err != nil {
		return Outcome{}, false, err
	}
	if problem != "" {
		return running.backToWork(ctx, problem)
	}
	outcome, err := running.finish(ctx, text)
	return outcome, false, err
}

// theReplyProvesItsLines writes the answer the model has just given into the
// record as a result and points at it every done line the model said the answer
// proves, so that a task whose done list is the answer itself can close on the
// same proof every other line stands on.
func (running *run) theReplyProvesItsLines(ctx context.Context, text string) error {
	if len(running.provedByTheReply) == 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	label, err := running.keeper.AddResult(ctx, "the reply to the user: "+firstLine(text), text)
	if err != nil {
		return fmt.Errorf("cannot write the reply of task %s into the record as the result that proves its done list: %w",
			running.keeper.ID(), err)
	}
	lines := slices.Clone(running.keeper.Record().Goal.DoneWhen)
	for at := range lines {
		if slices.Contains(running.provedByTheReply, lines[at].Text) {
			lines[at].Done, lines[at].ResultID = true, label
		}
	}
	running.provedByTheReply = nil
	if err := running.keeper.Apply(ctx, record.Update{DoneWhen: lines}); err != nil {
		return fmt.Errorf("cannot point the done lines of task %s at the reply that proves them: %w",
			running.keeper.ID(), err)
	}
	return nil
}

// backToWork sends the model back with one line naming the rule its done list
// broke, and gives the task up when it will not fix it.
func (running *run) backToWork(ctx context.Context, problem string) (Outcome, bool, error) {
	running.doneNudges++
	if running.doneNudges > MaxDoneCheckNudges {
		outcome, err := running.failHere(ctx, errors.New(problem))
		return outcome, false, err
	}
	running.remember(contract.Message{Role: contract.RoleUser, Text: problem + "\n" + ThreeOptions})
	return Outcome{}, true, nil
}

// isAQuestion says whether the model asked the user something rather than
// answering them. A question mark on the last line is the plain sign of one, and
// a model that writes none is read by what it did instead: a reply with no tool
// calls, an empty done list, and no result written this round has nothing behind
// it that could close a task, so it is the model asking for something and not
// the model saying the work is finished.
func (running *run) isAQuestion(text string, why contract.FinishReason) bool {
	if why != contract.FinishEnd {
		return false
	}
	if strings.HasSuffix(lastLine(text), "?") {
		return true
	}
	return running.nothingBehindTheReply()
}

// nothingBehindTheReply says the reply could close nothing: the record holds no
// done list to prove, and this round wrote no result into it.
func (running *run) nothingBehindTheReply() bool {
	if running.keeper == nil {
		return false
	}
	return len(running.keeper.Record().Goal.DoneWhen) == 0 && running.resultsThisRound == 0
}

// lastLine is the last line of a piece of text with anything on it.
func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	for at := len(lines) - 1; at >= 0; at-- {
		if trimmed := strings.TrimSpace(lines[at]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// finish closes a task whose done list is proven: the record says done, the
// review runs if the task was worth reviewing, and the user gets the report.
func (running *run) finish(ctx context.Context, text string) (Outcome, error) {
	if err := running.setStatus(ctx, contract.StatusDone); err != nil {
		return Outcome{}, err
	}
	if err := running.review(ctx); err != nil {
		return Outcome{}, err
	}
	report := text
	if strings.TrimSpace(report) == "" {
		report = "The task is done. Every line of the done list points at the result that proves it."
	}
	report = running.withTheLesson(report)
	if err := running.sendUnlessAJob(ctx, report); err != nil {
		return Outcome{}, err
	}
	return Outcome{TaskID: running.taskID(), Status: contract.StatusDone, Report: report}, nil
}

// waitHere puts the task into waiting, which is where a question leaves it.
// Nothing is held in any model's memory until the user answers.
func (running *run) waitHere(ctx context.Context, text string) (Outcome, error) {
	if err := running.setStatus(ctx, contract.StatusWaiting); err != nil {
		return Outcome{}, err
	}
	if err := running.send(ctx, text); err != nil {
		return Outcome{}, err
	}
	return Outcome{TaskID: running.taskID(), Status: contract.StatusWaiting, Report: text}, nil
}

// answerWithNoRecord ends a task that never needed a tool, which is the one
// kind of task that makes no record at all.
func (running *run) answerWithNoRecord(ctx context.Context, text string) (Outcome, error) {
	if err := running.send(ctx, text); err != nil {
		return Outcome{}, err
	}
	return Outcome{Status: contract.StatusDone, Report: text}, nil
}

// stopHere stops the task because a line of the stop list fired, and tells the
// user which line it was.
func (running *run) stopHere(ctx context.Context, line string) (Outcome, error) {
	return running.stopAndSay(ctx, line, "Where it stands: "+running.whereItStands(), true)
}

// stopAndSay ends the task as stopped and sends the one report the user gets:
// which line stopped it, where the work stands, and how to carry it on. The four
// review questions are asked unless the caller has already spent the ending's
// model call on the words in the middle of that report.
func (running *run) stopAndSay(ctx context.Context, line string, standing string, askTheQuestions bool) (Outcome, error) {
	running.hadStop, running.stopLine = true, line
	running.forgetTheCalls()
	if err := running.setStatus(ctx, contract.StatusStopped); err != nil {
		return Outcome{}, err
	}
	if askTheQuestions {
		if err := running.review(ctx); err != nil {
			return Outcome{}, err
		}
	}
	report := running.withTheLesson(fmt.Sprintf(
		"I stopped this task, because %s.\n%s\nTell me how to carry on and I will pick it up from here.",
		line, standing))
	if err := running.sendUnlessAJob(ctx, report); err != nil {
		return Outcome{}, err
	}
	return Outcome{TaskID: running.taskID(), Status: contract.StatusStopped, Report: report, StopLine: line}, nil
}

// failHere ends a task that could not be finished, and says what went wrong.
func (running *run) failHere(ctx context.Context, reason error) (Outcome, error) {
	running.hadFailure = true
	if err := running.setStatus(ctx, contract.StatusFailed); err != nil {
		return Outcome{}, err
	}
	if err := running.review(ctx); err != nil {
		return Outcome{}, err
	}
	report := running.withTheLesson(
		fmt.Sprintf("I could not finish this task: %s\nWhere it stands: %s", reason, running.whereItStands()))
	if err := running.sendUnlessAJob(ctx, report); err != nil {
		return Outcome{}, err
	}
	return Outcome{TaskID: running.taskID(), Status: contract.StatusFailed, Report: report}, nil
}

// finalReport is the one call with the tools turned off that the budget buys:
// the model says what it did and what is left, and the user gets that, once, in
// the middle of the one report this ending sends. The four review questions are
// not asked on top of it, because this ending's one call has been spent on the
// words the user actually reads.
func (running *run) finalReport(ctx context.Context, why string) (Outcome, error) {
	running.remember(contract.Message{
		Role: contract.RoleUser,
		Text: "Your budget is spent, because " + why + ". The tools are off now. In a few lines, say what you did, what you checked, and what is left.",
	})
	said := "The budget ran out before the task was finished."
	if request, err := running.buildRequest(ctx, true); err == nil {
		if reply, err := running.theLoop.options.Model.Send(ctx, request, running.theLoop.options.Deltas); err == nil {
			running.roundsUsed++
			said = reply.Text
		}
	}
	return running.stopAndSay(ctx, why, said, false)
}

// whereItStands is the one line the harness can always write about a task: the
// last thing the model said about where the work was.
func (running *run) whereItStands() string {
	if running.lastOrient == "" {
		return "nothing had been done yet"
	}
	return running.lastOrient
}

// setStatus writes where the record stands, and does nothing at all when the
// task never made one.
func (running *run) setStatus(ctx context.Context, status contract.RecordStatus) error {
	if running.keeper == nil {
		return nil
	}
	if err := running.keeper.SetStatus(ctx, status); err != nil {
		return fmt.Errorf("cannot mark task %s as %s: %w", running.keeper.ID(), status, err)
	}
	return nil
}

// send writes a reply into the log and then sends it, in that order, so that a
// crash between the two cannot lose it.
func (running *run) send(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := running.theLoop.logEvent(ctx, running.taskID(), contract.EventReply, struct {
		Text string `json:"text"`
	}{Text: text}); err != nil {
		return err
	}
	if err := running.channel.Send(ctx, text); err != nil {
		return fmt.Errorf("cannot send the reply of task %q to the user: %w", running.taskID(), err)
	}
	return nil
}

// sendUnlessAJob holds a job task's report back, because that one goes to the
// user with the job's progress line on it once the job has taken it.
func (running *run) sendUnlessAJob(ctx context.Context, text string) error {
	if running.task.FromJob != nil {
		return nil
	}
	return running.send(ctx, text)
}
