package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
)

func TestApproveTakesAlwaysAsTheSecondWord(t *testing.T) {
	kept := []answered{}
	answer := runOne(t, recordingAnswer(&kept), "/approve 3 "+contract.ApproveAlwaysText)

	if len(kept) != 1 {
		t.Fatalf("the approve command answered %d previews rather than one", len(kept))
	}
	if kept[0].previewID != "3" || kept[0].answer != contract.AnswerAlways {
		t.Errorf("the approve command answered %+v rather than 3 for the rest of the session", kept[0])
	}
	if !strings.Contains(answer, "session") {
		t.Errorf("the approve command does not say that the answer stands for the session: %q", answer)
	}
}

func TestApproveRefusesASecondWordThatIsNotAlways(t *testing.T) {
	kept := []answered{}
	answer := runOne(t, recordingAnswer(&kept), "/approve 3 forever")

	if len(kept) != 0 {
		t.Fatalf("the approve command answered a preview after a word it does not understand: %+v", kept)
	}
	if !strings.Contains(answer, contract.ApproveAlwaysText) {
		t.Errorf("the refusal does not say the one word it understands: %q", answer)
	}
}

func TestApproveWithNoSecondWordStandsForThisOnceOnly(t *testing.T) {
	kept := []answered{}
	if _, err := answerOf(t, recordingAnswer(&kept), "/approve 3"); err != nil {
		t.Fatalf("the approve command failed: %v", err)
	}

	if len(kept) != 1 || kept[0].answer != contract.AnswerOnce {
		t.Errorf("a bare approve answered %+v rather than this once", kept)
	}
}

func TestApproveAlwaysHandsBackWhatTheAnswerFailedWith(t *testing.T) {
	_, err := answerOf(t, command.Deps{
		Answer: func(context.Context, string, contract.PreviewAnswer, string) error { return errBrokenJobStore },
	}, "/approve 3 "+contract.ApproveAlwaysText)
	if err == nil {
		t.Fatalf("the approve command hid what the answer failed with")
	}
}
