package testkit_test

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeBrowserWorkerAnswersADialogAndRefusesAnUnknownAnswer(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the fixture page failed: %v", err)
	}
	worker.NextActionOpensADialog(contract.Dialog{Kind: "confirm", Message: "Post this?"})
	opened, err := worker.Click(ctx, testkit.FixtureButtonRef, "a dialog asks")
	if err != nil {
		t.Fatalf("the click failed: %v", err)
	}
	if opened.Dialog == nil {
		t.Fatal("the click did not open the dialog the test asked for")
	}

	answered, err := worker.Dialog(ctx, contract.DialogAccept, "")
	if err != nil {
		t.Fatalf("answering the dialog failed: %v", err)
	}
	if answered.Dialog != nil || answered.Snapshot.Dialog != nil {
		t.Errorf("the dialog is still open after it was accepted: %+v", answered)
	}
	if got := worker.DialogAnswers(); len(got) != 1 || got[0].Action != contract.DialogAccept {
		t.Errorf("the recorded dialog answers are %+v, want one accept", got)
	}

	if _, err := worker.Dialog(ctx, "ignore", ""); err == nil {
		t.Error("an unknown dialog action was accepted, want an error naming the two actions")
	}
	if _, err := worker.Dialog(ctx, contract.DialogDismiss, ""); err == nil {
		t.Error("dismissing when no dialog is open returned no error")
	}
}
