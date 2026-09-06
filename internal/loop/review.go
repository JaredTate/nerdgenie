package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// RoundsThatDeserveAReview is how many rounds a task has to have taken before
// it is reviewed even though nothing went wrong.
const RoundsThatDeserveAReview = 5

// ReviewTime is how long the four questions may take to answer. It is a model
// call, so it gets a bound of its own rather than the ten seconds the ending's
// bookkeeping gets: on the local model a cold prompt of sixty thousand tokens
// takes two and a half minutes to read before a word is written.
const ReviewTime = 3 * time.Minute

// errReviewTimeUp is why the review's own context is cancelled when the model
// will not answer the four questions inside ReviewTime.
var errReviewTimeUp = errors.New("the review was given three minutes to answer the four questions, and used all of it")

// theSignsOfToolMarkup are what a small model writes when the four questions
// confuse it into starting a tool call instead of answering. An answer that
// carries one is no lesson.
var theSignsOfToolMarkup = []string{"<parameter=", "</function>", "<function=", "<tool_call", "</tool_call>", "<invoke"}

// TheFourQuestions is the after-action review, borrowed whole from the U.S.
// Army's own: what was supposed to happen, what happened, why they differ, and
// what to keep or change. Only the fourth answer is ever kept.
const TheFourQuestions = `The work is over. Answer these four questions, one line each, in this order and nothing else:
1. What was supposed to happen?
2. What actually happened?
3. Why was there a difference?
4. What do we keep, and what do we change?`

// theOpeningsOfAProcedure are how a fourth answer that describes a way of doing
// something begins. An answer that starts with one of these is offered to the
// user as a skill; anything else is kept as a plain fact.
var theOpeningsOfAProcedure = []string{"to ", "the steps", "first,", "first ", "start by", "begin by"}

// review asks the four questions when the task was worth reviewing: it had a
// correction, a failure, a stop, or more than five rounds.
func (running *run) review(ctx context.Context) error {
	if running.keeper == nil || !running.worthReviewing() {
		return nil
	}
	held := running.keeper.Record()
	ctx, done := running.theLoop.timeForTheReview(ctx)
	defer done()
	answer := running.theLoop.askTheFourQuestions(ctx, string(record.Print(held)))
	if answer == "" {
		return nil
	}
	where := running.channel
	if running.task.Unattended {
		// Nobody is there to answer a preview, so an unattended run keeps the
		// lesson as a fact and does not offer it as a skill. Offering it would
		// show a preview to nobody and wait out the whole answer deadline at the
		// end of every scheduled job that learned a procedure.
		where = nil
		running.lessonUnoffered = looksLikeAProcedure(answer)
	}
	// A lesson that cannot be kept costs the lesson and nothing more: the
	// nightly game build of 6 September lost its job's first report, and the
	// mark on the job's list, to a review whose fact memory refused, because
	// the refusal ended the task with an error.
	if err := running.theLoop.keepTheLesson(ctx, where, "task "+running.keeper.ID(), answer); err != nil {
		running.lessonUnkept = err.Error()
	}
	return nil
}

// withTheLesson adds the one line an unattended run owes the user: the review
// found a way of doing something, and nobody was there to be asked whether to
// keep it as a skill, so it was kept as a fact instead.
func (running *run) withTheLesson(report string) string {
	if running.lessonUnkept != "" {
		return report + "\nThe lesson of this run could not be kept: " + running.lessonUnkept
	}
	if !running.lessonUnoffered {
		return report
	}
	return report + "\nThis run learned a way of doing this and kept it as a fact, " +
		"because nobody was there to be asked whether to save it as a skill."
}

// worthReviewing says whether this task earned its review.
func (running *run) worthReviewing() bool {
	return running.hadCorrection || running.hadFailure || running.hadStop ||
		running.roundsUsed > RoundsThatDeserveAReview
}

// askTheFourQuestions makes one call with the tools off and returns the fourth
// answer, which is the only one that is kept. A review that cannot be asked
// costs a lesson and nothing more, so nothing here fails the task.
func (theLoop *Loop) askTheFourQuestions(ctx context.Context, background string) string {
	request, err := theLoop.options.Context.Build(ctx, BuildInput{
		Messages: []contract.Message{
			{Role: contract.RoleUser, Text: background},
			{Role: contract.RoleUser, Text: TheFourQuestions},
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
	return fourthAnswerIn(reply.Text)
}

// fourthAnswerIn picks the fourth line out of the review, which is the answer to
// "what do we keep, and what do we change?". A model that wrote fewer lines has
// its last one taken, because that is the one it was answering.
func fourthAnswerIn(text string) string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, withoutItsNumber(trimmed))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) >= 4 {
		return lines[3]
	}
	return lines[len(lines)-1]
}

// withoutItsNumber takes the "4." or "4)" off the front of an answer.
func withoutItsNumber(line string) string {
	for _, numbering := range []string{"1.", "2.", "3.", "4.", "1)", "2)", "3)", "4)"} {
		if rest, found := strings.CutPrefix(line, numbering); found {
			return strings.TrimSpace(rest)
		}
	}
	return line
}

// timeForTheReview is the review's own context: the turn's, still cancelled by
// whatever cancels the turn, with ReviewTime on top, so that a slow model gets
// its minutes and a hung one does not get the hour.
func (theLoop *Loop) timeForTheReview(ctx context.Context) (context.Context, context.CancelFunc) {
	bounded, cancel := context.WithCancelCause(ctx)
	go func() {
		if err := theLoop.options.Clock.Sleep(bounded, ReviewTime); err == nil {
			cancel(errReviewTimeUp)
		}
	}()
	return bounded, func() { cancel(nil) }
}

// keepTheLesson saves the fourth answer as a fact with the work it came from as
// its source, and offers it to the user as a skill when it describes a way of
// doing something. An answer that is the opening of a tool call is not a lesson
// and is not kept: the live home's memory file held "<parameter=file_path>" and
// "</function>" as two of its five facts.
func (theLoop *Loop) keepTheLesson(ctx context.Context, where contract.Channel, source string, answer string) error {
	if looksLikeToolMarkup(answer) {
		return nil
	}
	if theLoop.options.Memory != nil {
		// The id carries the moment of the review, so the reviews of two
		// tasks with the same number, in two homes or two runs of one, never
		// collide.
		now := theLoop.options.Clock.Now()
		if err := theLoop.options.Memory.Save(ctx, []contract.Fact{{
			ID:       "review-" + strings.ReplaceAll(source, " ", "-") + "-" + now.UTC().Format("20060102T150405Z"),
			Text:     answer,
			Source:   source,
			Recorded: now,
		}}); err != nil {
			return fmt.Errorf("cannot save what %s taught us: %w", source, err)
		}
	}
	if !looksLikeAProcedure(answer) {
		return nil
	}
	return theLoop.offerASkill(ctx, where, source, answer)
}

// looksLikeToolMarkup says whether an answer is the start of a tool call rather
// than words.
func looksLikeToolMarkup(answer string) bool {
	for _, sign := range theSignsOfToolMarkup {
		if strings.Contains(answer, sign) {
			return true
		}
	}
	return false
}

// looksLikeAProcedure says whether an answer describes a way of doing something
// rather than a fact about the world.
func looksLikeAProcedure(answer string) bool {
	opening := strings.ToLower(strings.TrimSpace(answer))
	for _, start := range theOpeningsOfAProcedure {
		if strings.HasPrefix(opening, start) {
			return true
		}
	}
	return false
}

// offerASkill shows the user what would be saved and saves it when they say
// yes, because a skill the user did not approve is a skill nobody asked for.
func (theLoop *Loop) offerASkill(ctx context.Context, where contract.Channel, source string, answer string) error {
	if theLoop.options.Skills == nil || where == nil {
		return nil
	}
	name := skillNameFor(answer)
	answered, err := where.ShowPreview(ctx, contract.Preview{
		ID:    "skill-" + name,
		Title: "Save this as a skill called " + name + "?",
		Body:  answer,
	})
	if err != nil {
		return fmt.Errorf("cannot offer the skill from %s to the user: %w", source, err)
	}
	if answered.Answer == contract.AnswerReject {
		return nil
	}
	body := fmt.Sprintf("# %s\n\nWhat it is for: %s\n\nLearned from %s.\n", name, answer, source)
	if err := theLoop.options.Skills.Save(ctx, contract.SkillSavedByPerson, name, map[string][]byte{"SKILL.md": []byte(body)}); err != nil {
		return fmt.Errorf("cannot save the skill the user approved: %w", err)
	}
	return nil
}

// skillNameFor makes a folder name out of the first few words of the answer.
func skillNameFor(answer string) string {
	words := strings.Fields(strings.ToLower(answer))
	kept := []string{}
	for _, word := range words {
		plain := strings.Map(onlyLettersAndDigits, word)
		if plain != "" {
			kept = append(kept, plain)
		}
		if len(kept) == 4 {
			break
		}
	}
	if len(kept) == 0 {
		return "learned-skill"
	}
	return strings.Join(kept, "-")
}

// onlyLettersAndDigits keeps the letters and digits of a word and drops the
// rest, so that a folder name is always a name a filesystem will take.
func onlyLettersAndDigits(letter rune) rune {
	switch {
	case letter >= 'a' && letter <= 'z', letter >= '0' && letter <= '9':
		return letter
	default:
		return -1
	}
}
