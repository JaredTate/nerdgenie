package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// RoundsThatDeserveAReview is how many rounds a task has to have taken before
// it is reviewed even though nothing went wrong.
const RoundsThatDeserveAReview = 5

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
	answer := running.theLoop.askTheFourQuestions(ctx, string(record.Print(held)))
	if answer == "" {
		return nil
	}
	return running.theLoop.keepTheLesson(ctx, running.channel, "task "+running.keeper.ID(), answer)
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
		ToolsOff:        true,
		MaxOutputTokens: theLoop.options.Caps.OutputTokensPerCall,
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

// keepTheLesson saves the fourth answer as a fact with the work it came from as
// its source, and offers it to the user as a skill when it describes a way of
// doing something.
func (theLoop *Loop) keepTheLesson(ctx context.Context, where contract.Channel, source string, answer string) error {
	if theLoop.options.Memory != nil {
		if err := theLoop.options.Memory.Save(ctx, []contract.Fact{{
			ID:       "review-" + strings.ReplaceAll(source, " ", "-"),
			Text:     answer,
			Source:   source,
			Recorded: theLoop.options.Clock.Now(),
		}}); err != nil {
			return fmt.Errorf("cannot save what %s taught us: %w", source, err)
		}
	}
	if !looksLikeAProcedure(answer) {
		return nil
	}
	return theLoop.offerASkill(ctx, where, source, answer)
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
	if err := theLoop.options.Skills.Save(ctx, name, map[string][]byte{"SKILL.md": []byte(body)}); err != nil {
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
