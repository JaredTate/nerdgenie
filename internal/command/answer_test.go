package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
)

// answered is one call to Deps.Answer, kept so that a test can say exactly what
// the user's yes or no did.
type answered struct {
	previewID string
	answer    contract.PreviewAnswer
	reason    string
}

// recordingAnswer returns a Deps whose Answer writes down what it was told.
func recordingAnswer(kept *[]answered) command.Deps {
	return command.Deps{
		Answer: func(_ context.Context, previewID string, answer contract.PreviewAnswer, reason string) error {
			*kept = append(*kept, answered{previewID: previewID, answer: answer, reason: reason})
			return nil
		},
	}
}

func TestApproveAnswersThePreviewWithYesForThisOnce(t *testing.T) {
	kept := []answered{}
	answer := runOne(t, recordingAnswer(&kept), "/approve 3")

	if len(kept) != 1 {
		t.Fatalf("the approve command answered %d previews rather than one", len(kept))
	}
	if kept[0].previewID != "3" || kept[0].answer != contract.AnswerOnce {
		t.Errorf("the approve command answered %+v rather than 3 with once", kept[0])
	}
	if !strings.Contains(answer, "3") {
		t.Errorf("the approve command does not say what it answered: %q", answer)
	}
}

func TestDenyAnswersThePreviewWithNoAndTheReasonGiven(t *testing.T) {
	kept := []answered{}
	answer := runOne(t, recordingAnswer(&kept), "/deny 3 the post leads with the features")

	if len(kept) != 1 {
		t.Fatalf("the deny command answered %d previews rather than one", len(kept))
	}
	if kept[0].previewID != "3" || kept[0].answer != contract.AnswerReject {
		t.Errorf("the deny command answered %+v rather than 3 with a refusal", kept[0])
	}
	if kept[0].reason != "the post leads with the features" {
		t.Errorf("the deny command passed on the reason %q rather than the user's own words", kept[0].reason)
	}
	if !strings.Contains(answer, "3") {
		t.Errorf("the deny command does not say what it answered: %q", answer)
	}
}

func TestDenyWithNoReasonStillAnswers(t *testing.T) {
	kept := []answered{}
	runOne(t, recordingAnswer(&kept), "/deny 3")

	if len(kept) != 1 || kept[0].reason != "" {
		t.Fatalf("the deny command with no reason gave %+v", kept)
	}
}

func TestApproveAndDenyAskWhichOneWhenNoIDIsGiven(t *testing.T) {
	for _, line := range []string{"/approve", "/deny"} {
		kept := []answered{}
		answer := runOne(t, recordingAnswer(&kept), line)

		if len(kept) != 0 {
			t.Fatalf("%q answered a preview without being told which one", line)
		}
		if !strings.Contains(answer, "/status") {
			t.Errorf("%q does not say how to find the id: %q", line, answer)
		}
	}
}

func TestApproveHandsBackWhatTheAnswerFailedWith(t *testing.T) {
	broken := errors.New("there is no preview numbered 3, so run /status to see what is waiting")
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{
		Answer: func(_ context.Context, _ string, _ contract.PreviewAnswer, _ string) error { return broken },
	}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/approve 3", contract.CommandContext{Channel: terminalChannel()}); !errors.Is(err, broken) {
		t.Errorf("the approve command hid what the answer failed with: %v", err)
	}
}

func TestApproveSaysWhenNothingCanAnswer(t *testing.T) {
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/approve 3", contract.CommandContext{Channel: terminalChannel()}); err == nil {
		t.Fatalf("the approve command claimed to answer a preview with nothing wired up to answer one")
	}
}
