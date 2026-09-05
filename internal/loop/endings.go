package loop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// WrapUpTime is how long the ending of a task may take to write its checkpoint,
// mark the record, and tell the user, measured on the harness's clock. It is a
// short span of the ending's own rather than the turn's, because a turn cut off
// by a limit, a stop, or a shutdown has a cancelled context, and an ending
// written under that context failed at its first write and left the task
// standing at running for ever as far as the program could see.
const WrapUpTime = 10 * time.Second

// errWrapUpTimeUp is why the ending's own context is cancelled when the log will
// not answer inside WrapUpTime.
var errWrapUpTimeUp = errors.New("the ending of the task was given ten seconds to write its checkpoint and its report, and used all of it")

// timeToWrapUp is the context the ending of a task writes under: the turn's own
// with its cancellation taken off, and WrapUpTime of its own on the harness's
// clock, so that a task cut off for any reason is still marked and reported,
// and a log that will not answer cannot hold the ending forever.
func (running *run) timeToWrapUp(ctx context.Context) (context.Context, context.CancelFunc) {
	fresh, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	go func() {
		if err := running.theLoop.options.Clock.Sleep(fresh, WrapUpTime); err == nil {
			cancel(errWrapUpTimeUp)
		}
	}()
	return fresh, func() { cancel(context.Canceled) }
}

// endOfTurn is what happens when the model replies with no tool calls. The
// harness tells a question from an answer by the finish state and by what the
// reply has behind it: a question puts the task into waiting, and an answer goes
// to the done-check.
//
// A message that arrived while the model was writing this reply is read first,
// because it is this task's to read and not the next task's: the person said
// stop, so the task stops, or they corrected it, so the model is sent round once
// more with their words in front of it and this reply does not stand. Messages
// that arrive during a tool call are read at the end of the tool calls, so
// this reads only what came during a reply with no tool call after it.
//
// The reply itself goes into the conversation before any of that is read, so
// that the round a correction or a done-check nudge sends the model on shows
// the person's words after the reply they answer. Only a reply with tool calls
// used to be written down, and the one more round was handed a correction with
// nothing before it but tool results.
func (running *run) endOfTurn(ctx context.Context, text string, why contract.FinishReason) (Outcome, bool, error) {
	if err := running.writeSituation(ctx); err != nil {
		return Outcome{}, false, err
	}
	running.remember(contract.Message{Role: contract.RoleAssistant, Text: text})
	ended, more, corrections, err := running.readTheMessages(ctx, running.theLoop.takeDelivered())
	if err != nil || !more {
		return ended, false, err
	}
	if corrections > 0 {
		return Outcome{}, true, nil
	}
	if running.keeper == nil {
		if running.isAQuestion(text, why) {
			outcome, err := running.waitHere(ctx, text)
			return outcome, false, err
		}
		outcome, err := running.answerWithNoRecord(ctx, text)
		return outcome, false, err
	}
	return running.closeOrWait(ctx, text, why)
}

// closeOrWait ends a task that has a record: the done-check says whether the
// work is done, and a question is read only where there is something left to
// ask about.
//
// The done-check comes before the question, because a small model ends a
// finished reply with a chatty question, "Anything else?", and the loop used
// to read that one character first: a task with every done line proven, the
// file written and the test printing PASS, went into waiting instead of
// closing, and a job's task so ended put the job down at "1 of 3" with
// nothing left to do. A record with no done list is the one exception, and
// keeps its old reading: there is nothing to close on, so a question mark or
// a plain-words ask on the last line is a question, and only a reply that
// asks nothing is written in as the whole of the done list.
func (running *run) closeOrWait(ctx context.Context, text string, why contract.FinishReason) (Outcome, bool, error) {
	if running.theRecordHasNoDoneList() {
		if running.isAQuestion(text, why) {
			outcome, err := running.waitHere(ctx, text)
			return outcome, false, err
		}
		if err := running.theAnswerIsTheWholeDoneList(ctx, text); err != nil {
			return Outcome{}, false, err
		}
	} else if err := running.theReplyProvesItsLines(ctx, text); err != nil {
		return Outcome{}, false, err
	}
	problem, err := running.doneCheck(ctx)
	if err != nil {
		return Outcome{}, false, err
	}
	if problem == "" {
		outcome, err := running.finish(ctx, text)
		return outcome, false, err
	}
	if running.isAQuestion(text, why) {
		outcome, err := running.waitHere(ctx, text)
		return outcome, false, err
	}
	return running.backToWork(ctx, problem)
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

// TheWorkIsTheAnswer is the one done line the harness writes into a record the
// model never wrote a done list into. It is the harness's own words, not the
// model's, and it says exactly what it stands on: the work was done and the
// answer is the report of it.
const TheWorkIsTheAnswer = "the work was done and the user was told what changed"

// theAnswerIsTheWholeDoneList is the rule the live suite asked for. On a small
// ask no model writes a done list: it does the work, answers, and never calls
// the task tool. A record with an empty done list is one the done-check can only
// refuse, so the model would be sent back three times and the task given up on.
// The answer is the report of the work, so the harness writes it into the record
// as a result and puts one done line of its own behind it, and the done-check
// then runs on a record that says what done looked like.
//
// A record the model did write a done list into is left exactly as it is: what
// done looks like is the model's to say whenever it has said it.
func (running *run) theAnswerIsTheWholeDoneList(ctx context.Context, text string) error {
	if len(running.keeper.Record().Goal.DoneWhen) > 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	label, err := running.keeper.AddResult(ctx, "the reply to the user: "+firstLine(text), text)
	if err != nil {
		return fmt.Errorf("cannot write the answer of task %s into the record as the result that proves it: %w",
			running.keeper.ID(), err)
	}
	written := record.Update{DoneWhen: []contract.DoneLine{{Text: TheWorkIsTheAnswer, Done: true, ResultID: label}}}
	if err := running.keeper.Apply(ctx, written); err != nil {
		return fmt.Errorf("cannot write the done line of task %s that its answer proves: %w", running.keeper.ID(), err)
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

// theWaysAReplyAsks is the short fixed list of ways a last line asks the user
// for something without ending in a question mark. It is a fixed list for the
// same reason theWordsThatMeanStop is one: a rule a reader can hold in their
// head is a rule they can argue with.
//
// It is read on the last line only, and only on a reply the record has no done
// list to close on, because outside those two fences the same words are
// ordinary: the harness's own stopped report ends "Tell me how to carry on".
var theWaysAReplyAsks = []string{
	"tell me", "let me know", "i need to know", "please confirm", "please say",
	"should i ", "would you like", "do you want",
}

// isAQuestion says whether the model asked the user something rather than
// answering them. A question mark on the last line is the plain sign of one, and
// a reply the record has nothing to close on is read by the words of its last
// line as well, because the reviewer's own probe asked politely and ended in a
// full stop.
//
// An empty done list used to be the whole of that second reading: a reply with
// no tool calls and no done list was taken for a question whatever it said. The
// live suite showed why that is wrong on all three real models. Asked to write
// hello to a file and read it back, none of them writes a done list at all — it
// does the work, answers, and never calls the task tool — so every small task
// ended waiting on a user who had been asked nothing, and the done-check never
// ran.
func (running *run) isAQuestion(text string, why contract.FinishReason) bool {
	if why != contract.FinishEnd {
		return false
	}
	last := lastLine(text)
	if strings.HasSuffix(last, "?") {
		return true
	}
	return running.theRecordHasNoDoneList() && asksInPlainWords(last)
}

// theRecordHasNoDoneList says the record holds nothing that says what done looks
// like, so there is nothing the done-check could close the task on yet.
func (running *run) theRecordHasNoDoneList() bool {
	return running.keeper != nil && len(running.keeper.Record().Goal.DoneWhen) == 0
}

// asksInPlainWords says whether this line is one of the plain ways of asking the
// user for something.
func asksInPlainWords(line string) bool {
	said := strings.ToLower(line)
	for _, asking := range theWaysAReplyAsks {
		if strings.Contains(said, asking) {
			return true
		}
	}
	return false
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
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
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
// Nothing is held in any model's memory until the user answers. A job task's
// question is held back here, the way its report is, because it goes to the
// person with the job named under it once the job has been put down on it.
func (running *run) waitHere(ctx context.Context, text string) (Outcome, error) {
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
	if err := running.setStatus(ctx, contract.StatusWaiting); err != nil {
		return Outcome{}, err
	}
	if err := running.sendUnlessAJob(ctx, text); err != nil {
		return Outcome{}, err
	}
	return Outcome{TaskID: running.taskID(), Status: contract.StatusWaiting, Report: text}, nil
}

// answerWithNoRecord ends a task that never needed a tool, which is the one
// kind of task that makes no record at all.
func (running *run) answerWithNoRecord(ctx context.Context, text string) (Outcome, error) {
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
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

// stopForThePerson stops the task because the person asked it to, with Escape
// or with the word stop, and asks the model nothing more. The four review
// questions used to be asked here as well, which is a model call the stop
// could not reach: the person pressed Escape, the call in flight was
// cancelled, and the loop at once made another call they had to press Escape
// at again. A stop the person asked for ends with the call they stopped.
func (running *run) stopForThePerson(ctx context.Context, line string) (Outcome, error) {
	outcome, err := running.stopAndSay(ctx, line, "Where it stands: "+running.whereItStands(), false)
	outcome.ByThePerson = true
	return outcome, err
}

// stopAndSay ends the task as stopped and sends the one report the user gets:
// which line stopped it, where the work stands, and how to carry it on. A job's
// task is told how to carry on by the line the job puts under the report, which
// names the one word that picks it up, so that line is left off here rather
// than giving two instructions. The four review questions are asked unless the
// caller has already spent the ending's model call on the words in the middle
// of that report.
func (running *run) stopAndSay(ctx context.Context, line string, standing string, askTheQuestions bool) (Outcome, error) {
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
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
	report := fmt.Sprintf("I stopped this task, because %s.\n%s", line, standing)
	if running.task.FromJob == nil {
		report += "\nTell me how to carry on and I will pick it up from here."
	}
	report = running.withTheLesson(report)
	if err := running.sendUnlessAJob(ctx, report); err != nil {
		return Outcome{}, err
	}
	return Outcome{TaskID: running.taskID(), Status: contract.StatusStopped, Report: report, StopLine: line}, nil
}

// failHere ends a task that could not be finished, and says what went wrong. A
// task whose turn was cut off under it, which is what a cancelled context
// means, is marked and reported and asked nothing more: the review is a model
// call, and the turn it would run in is over.
func (running *run) failHere(ctx context.Context, reason error) (Outcome, error) {
	cutOff := ctx.Err() != nil
	ctx, done := running.timeToWrapUp(ctx)
	defer done()
	running.hadFailure = true
	if err := running.setStatus(ctx, contract.StatusFailed); err != nil {
		return Outcome{}, err
	}
	if !cutOff {
		if err := running.review(ctx); err != nil {
			return Outcome{}, err
		}
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
