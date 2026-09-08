// The rethink turns a stall into a new angle. On the night of 6 September
// 2026 the three-bit model was handed the rewind line after a run of the same
// call, wrote "I have been looping, time to stop circling" on its first line
// for fifty rounds, and made the same call on every one of them. That night it
// solved a different bug in four rounds by writing small scripts that traced
// the code as it ran; for the bug it never solved it wrote no probe and re-read
// the same twenty-five lines. And it wrote one failure with the wrong cause,
// and every round after built on it, because the record is the truth and
// nothing made it check. Words do not change what a small model does; what is
// in front of it does. So a stall now buys one call with the tools off, over
// the record and the thing the model kept asking for in full, that asks what it
// was trying to learn, what the rounds showed, the symptom, two causes not yet
// on the record, and the one call that tells them apart. The answer goes into
// the record through the record's own rules, the call it was repeating is
// closed for a while (closedcalls.go), and a fresh window opens on the answer
// with the instruction to do its Next line. The answer's two causes are held
// against every failure's cause on the record: the sky task of the
// flight-simulator work order was asked twice for causes not on the record
// and named the one it already held both times, because nothing checked the
// answer. When both are already there, the question is asked once more with
// one line in front of it naming the failures that say so, and the second
// answer is taken as it stands. A task picked up after a stop with a failure
// on its record opens with the same rethink (freshwindow.go).

package loop

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The bounds of the rethink.
const (
	// MaxRethinkResultBytes is the most of the newest result the rethink's
	// background holds: sixteen kilobytes, the same cap as one read, so that
	// the model has once and in full the thing it kept asking for, and a
	// result of megabytes does not fill the window on its own.
	MaxRethinkResultBytes = 16384
	// ClosedForRounds is how many rounds the call the model was repeating is
	// refused after a rethink. Ten, because the model that wrote "time to
	// stop circling" made the same call on each of the fifty rounds after it,
	// and a closed door is what changes what it does next; ten rounds is room
	// to do the Next line and read what it shows, and short enough that a
	// call that was right after all is not lost for the task.
	ClosedForRounds = 10
)

// TheRethinkQuestion is the second and last message of the rethink's call:
// five labelled lines, one thing each.
const TheRethinkQuestion = `The work has stalled, so the tools are off for this one reply. Answer in five lines, each beginning with its label:
Learning: what you were trying to learn.
Showed: what the last rounds actually showed.
Symptom: the symptom, in one line.
Causes: two different causes that could explain it, neither one already written as a failure's cause on the record.
Next: the one cheapest call that tells those two causes apart, written as something that can be run or read.`

// TheCausesTriedLine is the one sentence of the background that says where
// the causes already on the record stand.
const TheCausesTriedLine = "Every cause already written as a failure's cause on the record was tried and did not hold."

// TheNameNewCausesLine is the second half of the line put in front of the
// question when both of the answer's causes are already on the record; the
// first half names the failures that hold them, "F11, F12 and F13 already
// say this; ".
const TheNameNewCausesLine = "name two causes the record does not hold"

// MaxCausesCompared is how many causes of the answer's Causes line are held
// against the record: two, because the question asks for two.
const MaxCausesCompared = 2

// theCauseSeparators are what a model writes between its two causes, tried
// in this order, so that "the file is wrong or missing, or the folder is" is
// cut at the comma and not inside the first cause.
var theCauseSeparators = []string{";", ", or ", " or "}

// TheRethinkSentBackOutcome is what the log says came of the question asked
// once more.
const TheRethinkSentBackOutcome = "the first answer's causes were already on the record, so the question was asked once more and this answer taken as it stands"

// TheRethinkLine opens the one message of the fresh window a rethink leaves
// the model in: what happened, where everything is, and that the rethink
// under it is the model's own.
const TheRethinkLine = "The work had stalled, so this is a fresh window on the same task: the block above holds the newest results in full, " +
	"the record holds every step and every result by its id, and \"read r7\" brings any of them back. Here is the rethink you just wrote:"

// TheDoTheNextLine closes that message.
const TheDoTheNextLine = "Do the Next line now."

// TheTriedByRethinkLine opens the line a stopped report gains after a rethink,
// which lists every rethink's decision.
const TheTriedByRethinkLine = "Tried by rethink: "

// theRethinkLabels are the five labels the answer is read by, in the order
// they are asked for.
var theRethinkLabels = []string{"Learning", "Showed", "Symptom", "Causes", "Next"}

// rethink is the model's answer to the question, read by its labels. A label
// the model left out takes the whole answer.
type rethink struct {
	showed  string
	symptom string
	causes  string
	next    string
}

// rethinkIfAnswered makes the rethink where the plain cut used to be: one
// call with the tools off, the answer into the record, the repeated call
// closed, and a fresh window on the answer. It returns false when the model
// gave no answer, and the plain cut stands instead. Nothing here fails the
// task.
func (running *run) rethinkIfAnswered(ctx context.Context) bool {
	label, background := running.theRethinkBackground(ctx)
	answer := strings.TrimSpace(running.theLoop.askForARethink(ctx, background, TheRethinkQuestion))
	if answer == "" {
		running.theLoop.logTheQuestion(ctx, running.taskID(), "rethink", TheRethinkQuestion, "", TheRethinkUnansweredOutcome)
		return false
	}
	running.theLoop.logTheQuestion(ctx, running.taskID(), "rethink", TheRethinkQuestion, answer, TheRethinkAnsweredOutcome)
	answer = running.askOnceMoreIfTheCausesAreHeld(ctx, background, answer)
	running.writeTheRethink(ctx, readTheRethink(answer))
	running.closeTheStalledCall(label)
	running.openTheWindowOnTheRethink(ctx, answer)
	return true
}

// askOnceMoreIfTheCausesAreHeld holds each of the answer's two causes against
// every failure's cause on the record, and when both are already there asks
// the question once more, with one line in front of it naming the failures
// that say so, and hands back the second answer as it stands, whatever it
// says. One send-back per rethink, and a second call that gives nothing
// leaves the first answer standing.
func (running *run) askOnceMoreIfTheCausesAreHeld(ctx context.Context, background string, answer string) string {
	labels, allHeld := running.theFailuresNamingTheCauses(readTheRethink(answer).causes)
	if !allHeld {
		return answer
	}
	question := theAlreadySayThisLine(labels) + "\n" + TheRethinkQuestion
	again := strings.TrimSpace(running.theLoop.askForARethink(ctx, background, question))
	running.theLoop.logTheQuestion(ctx, running.taskID(), "rethink", question, again, TheRethinkSentBackOutcome)
	if again == "" {
		return answer
	}
	return again
}

// theFailuresNamingTheCauses is the label of every failure on the record
// whose cause already says what one of the causes on this line says, in the
// record's order, and whether every cause on the line is held by one. A line
// with no cause on it is held by none.
func (running *run) theFailuresNamingTheCauses(line string) ([]string, bool) {
	causes := theTwoCauses(line)
	failures := running.keeper.Record().Lessons.Failures
	named := map[string]bool{}
	for _, cause := range causes {
		held, found := record.FailureNamingTheCause(failures, cause)
		if !found {
			return nil, false
		}
		named[held.ID] = true
	}
	labels := []string{}
	for _, failure := range failures {
		if named[failure.ID] {
			labels = append(labels, failure.ID)
		}
	}
	return labels, len(causes) > 0
}

// theTwoCauses cuts the answer's Causes line into the causes it names, at
// the first separator found, at most MaxCausesCompared of them. A line with
// no separator is one cause.
func theTwoCauses(line string) []string {
	pieces := []string{line}
	for _, separator := range theCauseSeparators {
		if strings.Contains(line, separator) {
			pieces = strings.SplitN(line, separator, MaxCausesCompared)
			break
		}
	}
	causes := []string{}
	for _, piece := range pieces {
		if piece = strings.TrimSpace(piece); piece != "" {
			causes = append(causes, piece)
		}
	}
	return causes
}

// theAlreadySayThisLine is the line put in front of the question asked once
// more: "F11, F12 and F13 already say this; name two causes the record does
// not hold".
func theAlreadySayThisLine(labels []string) string {
	verb := "say"
	if len(labels) == 1 {
		verb = "says"
	}
	return theLabelsInASentence(labels) + " already " + verb + " this; " + TheNameNewCausesLine
}

// theLabelsInASentence writes labels the way a sentence lists them: "F11,
// F12 and F13".
func theLabelsInASentence(labels []string) string {
	if len(labels) <= 1 {
		return strings.Join(labels, "")
	}
	return strings.Join(labels[:len(labels)-1], ", ") + " and " + labels[len(labels)-1]
}

// TheRethinkAnsweredOutcome and TheRethinkUnansweredOutcome are what the
// rethink's question event says came of the answer: the record keeps one line
// of each half, and the log keeps the whole answer beside them, so that a
// stall can be read back the way the review and the section question can.
const (
	TheRethinkAnsweredOutcome   = "wrote the stall as a failure and the Next line as a decision, then opened a fresh window on the answer"
	TheRethinkUnansweredOutcome = "no answer, so the stalled rounds were cut from the conversation"
)

// theRethinkBackground is the first message of the rethink's call: the record
// as it stands, the newest result on it in full up to the cap, the state of
// the tests when a run has been seen, and the sentence on the causes already
// tried. It returns the newest result's label too, for the closed call's line.
func (running *run) theRethinkBackground(ctx context.Context) (label string, background string) {
	held := running.keeper.Record()
	parts := []string{"The record as it stands:", string(record.Print(held))}
	if results := held.Work.Results; len(results) > 0 {
		newest := results[len(results)-1]
		if text, err := running.keeper.Read(ctx, newest.ID); err == nil {
			label = newest.ID
			parts = append(parts, "The newest result on the record, "+label+", in full:", cutToBytes(text, MaxRethinkResultBytes))
		}
	}
	if running.testsFact != "" {
		parts = append(parts, "The state of the "+running.testsFact)
	}
	parts = append(parts, TheCausesTriedLine)
	return label, strings.Join(parts, "\n")
}

// askForARethink makes one call of the rethink with the tools off, the way
// the review's is made and under the review's own time, over the background
// and the question given, and returns the model's whole answer. A call that
// fails returns nothing, and the plain cut stands.
func (theLoop *Loop) askForARethink(ctx context.Context, background string, question string) string {
	ctx, done := theLoop.timeForTheReview(ctx)
	defer done()
	request, err := theLoop.options.Context.Build(ctx, BuildInput{
		Messages: []contract.Message{
			{Role: contract.RoleUser, Text: background},
			{Role: contract.RoleUser, Text: question},
		},
		ToolsOff:      true,
		ContextLength: theLoop.options.Model.ContextLength(),
	})
	if err != nil {
		return ""
	}
	reply, err := theLoop.options.Model.Send(ctx, request, nil)
	if err != nil {
		return ""
	}
	return reply.Text
}

// readTheRethink reads the five labelled lines off the answer. A label the
// model left out takes the whole answer, so that the record's failure and
// decision always have both their halves and the Next line is always
// something to do.
func readTheRethink(answer string) rethink {
	found := map[string]string{}
	for _, line := range strings.Split(answer, "\n") {
		bare := strings.Trim(strings.TrimSpace(line), "*#-_ ")
		for _, label := range theRethinkLabels {
			if rest, has := cutTheLabel(bare, label); has && found[label] == "" {
				found[label] = rest
			}
		}
	}
	orTheWhole := func(label string) string {
		if found[label] != "" {
			return found[label]
		}
		return answer
	}
	return rethink{showed: orTheWhole("Showed"), symptom: orTheWhole("Symptom"), causes: orTheWhole("Causes"), next: orTheWhole("Next")}
}

// cutTheLabel takes a label and its colon off the front of a line, however the
// model cased it, and returns what is left.
func cutTheLabel(line string, label string) (string, bool) {
	for _, written := range []string{label + ":", strings.ToLower(label) + ":", strings.ToUpper(label) + ":"} {
		if rest, has := strings.CutPrefix(line, written); has {
			return strings.Trim(rest, "* "), true
		}
	}
	return "", false
}

// writeTheRethink puts the answer into the record through the record's own
// rules: the stall as a failure, the Showed line as what went wrong and the
// Symptom as its cause, and the Next line as a decision whose reason is the
// two causes. The stall keeps its "stalled:" prefix because the run report
// counts rewinds by it, and the record cuts each line to a lesson's length
// itself. The two go in as two writes, because a failure the record already
// holds is refused by it, which is no error here and must not cost the
// decision. Nothing else here can fail the task either.
func (running *run) writeTheRethink(ctx context.Context, thought rethink) {
	_ = running.keeper.Apply(ctx, record.Update{Failure: &record.NewFailure{
		Text:  "stalled: " + thought.showed,
		Cause: thought.symptom,
	}})
	_ = running.keeper.Apply(ctx, record.Update{Decision: &record.NewDecision{
		Text:   "Rethink: next " + thought.next,
		Reason: "two causes: " + thought.causes,
	}})
}

// openTheWindowOnTheRethink opens the fresh window a rethink leaves the model
// in, the way the cap opens one: the messages cleared, the orientation with
// the newest results in full, and one message holding the rethink line, the
// model's whole answer, and the instruction to do its Next line. The
// detector's window and the cut's mark start again with it.
func (running *run) openTheWindowOnTheRethink(ctx context.Context, answer string) {
	running.messages = nil
	running.recentCalls = nil
	running.rememberTheOrientation(ctx, true)
	running.remember(contract.Message{Role: contract.RoleUser, Text: TheRethinkLine + "\n\n" + answer + "\n\n" + TheDoTheNextLine})
	running.keepThrough = len(running.messages)
}

// theTriedByRethinkLine is the one line a stopped report gains after a
// rethink: every decision on the record whose text begins "Rethink:", by its
// label, so that the person reads the ledger of what was tried and not only
// the last stall. It is empty for a task that had no rethink.
func (running *run) theTriedByRethinkLine() string {
	if running.keeper == nil {
		return ""
	}
	tried := []string{}
	for _, decision := range running.keeper.Record().Lessons.Decisions {
		if strings.HasPrefix(decision.Text, "Rethink:") {
			tried = append(tried, decision.ID+" "+decision.Text)
		}
	}
	if len(tried) == 0 {
		return ""
	}
	return TheTriedByRethinkLine + strings.Join(tried, "; ")
}

// cutToBytes keeps the first bytes of a text up to the cap, ending on a whole
// letter, and says how much was cut.
func cutToBytes(text string, most int) string {
	if len(text) <= most {
		return text
	}
	cut := most
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + fmt.Sprintf("\n[cut here: the result is %d bytes long and the first %d are shown]", len(text), cut)
}
