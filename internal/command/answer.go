package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Approve is the "/approve" command: it answers one waiting preview or question
// with yes, for that one call and nothing more, or for every call like it for
// the rest of the session when the word "always" follows the id.
//
// The terminal shows those two answers as buttons; over Signal there is nothing
// but this command, so the word has to be typeable or a Signal user could never
// say "always" at all.
func (commands *Commands) Approve() contract.Command {
	return contract.Command{
		Name: "approve",
		Help: "Answers a waiting preview or question with yes: /approve 3, or /approve 3 always for the rest of the session.",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			previewID, rest := SplitLine(arguments)
			if previewID == "" {
				return askWhichOne("approve"), nil
			}
			if rest != "" && rest != contract.ApproveAlwaysText {
				return fmt.Sprintf("the only word that may follow the id is %q, so write /approve %s or /approve %s %s.",
					contract.ApproveAlwaysText, previewID, previewID, contract.ApproveAlwaysText), nil
			}
			if rest == contract.ApproveAlwaysText {
				if err := commands.answer(ctx, previewID, contract.AnswerAlways, ""); err != nil {
					return "", err
				}
				return fmt.Sprintf("%s is approved, and so is every call like it for the rest of this session.", previewID), nil
			}
			if err := commands.answer(ctx, previewID, contract.AnswerOnce, ""); err != nil {
				return "", err
			}
			return fmt.Sprintf("%s is approved, and the task is going on with it.", previewID), nil
		},
	}
}

// Deny is the "/deny" command: it refuses one waiting preview or question, and
// passes on whatever reason the user wrote after the id, in their own words.
func (commands *Commands) Deny() contract.Command {
	return contract.Command{
		Name: "deny",
		Help: "Answers a waiting preview or question with no, and passes on your reason: /deny 3 the post is wrong.",
		Run: func(ctx context.Context, arguments string, _ contract.CommandContext) (string, error) {
			previewID, reason := SplitLine(arguments)
			if previewID == "" {
				return askWhichOne("deny"), nil
			}
			if err := commands.answer(ctx, previewID, contract.AnswerReject, reason); err != nil {
				return "", err
			}
			if reason == "" {
				return fmt.Sprintf("%s is denied.", previewID), nil
			}
			return fmt.Sprintf("%s is denied, and the task was told: %s", previewID, reason), nil
		},
	}
}

// answer hands one decision to the part of the program that is waiting for it.
func (commands *Commands) answer(ctx context.Context, previewID string, decision contract.PreviewAnswer, reason string) error {
	if commands.deps.Answer == nil {
		return notWiredUp("approve", "Answer")
	}
	if err := commands.deps.Answer(ctx, previewID, decision, reason); err != nil {
		return fmt.Errorf("%s could not be answered: %w", previewID, err)
	}
	return nil
}

// askWhichOne is the line a bare "/approve" or "/deny" answers with, which says
// where to find the id to use.
func askWhichOne(name string) string {
	return strings.Join([]string{
		fmt.Sprintf("say which one, as in /%s 3.", name),
		"Type /status to see what is waiting for an answer.",
	}, " ")
}
