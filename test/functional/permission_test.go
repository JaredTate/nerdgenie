package functional

// This file is the functional test for the permission function: a user asks for
// something, the model asks for a tool call that is on the ask-me-first list, the
// user sees a preview of exactly what would happen and answers, and the right
// thing comes back. There is no agent loop yet, so the smallest possible
// stand-in wires the fakes and the permission function together. From wave 3 the
// same assertions run against the real loop through the local socket.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheUserSeesTheWholeCommandAndRefusingItStopsTheCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerPreviewsWith(contract.AnswerReject)
	decider := newPermission(t)

	ran, err := askAndRun(ctx, decider, channel, clearTheBuildFolder())
	if err != nil {
		t.Fatalf("putting the call to the user failed: %v", err)
	}

	if ran {
		t.Error("the command ran after the user refused it")
	}
	shown := channel.Previews()
	if len(shown) != 1 {
		t.Fatalf("the user saw %d previews, want 1", len(shown))
	}
	if !strings.Contains(shown[0].Body, "rm -rf /home/jared/build") {
		t.Errorf("the preview said %q, and the user has to see the whole command", shown[0].Body)
	}
	if shown[0].Title != contract.AskFirstBulkDelete {
		t.Errorf("the preview was headed %q, want the ask-me-first entry %q", shown[0].Title, contract.AskFirstBulkDelete)
	}
	if sent := channel.Sent(); len(sent) != 1 || !strings.Contains(sent[0], "refused") {
		t.Errorf("the user was told %v, want one message saying the call was refused", sent)
	}
}

func TestSayingAlwaysMeansTheSameCommandNeverAsksAgainThisSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerPreviewsWith(contract.AnswerAlways)
	decider := newPermission(t)

	for round := 1; round <= 3; round++ {
		ran, err := askAndRun(ctx, decider, channel, clearTheBuildFolder())
		if err != nil {
			t.Fatalf("round %d failed: %v", round, err)
		}
		if !ran {
			t.Fatalf("round %d did not run the command after the user allowed it", round)
		}
	}

	if shown := channel.Previews(); len(shown) != 1 {
		t.Errorf("the user saw %d previews, want 1, because always means do not ask again", len(shown))
	}
}

func TestAnOrdinaryCommandNeverReachesTheUserAtAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	channel := testkit.NewFakeChannel("terminal")
	decider := newPermission(t)

	call := contract.ToolCall{ID: "1", Name: contract.ToolShell, Input: json.RawMessage(`{"command":"git commit -m \"the nightly report\""}`)}
	ran, err := askAndRun(ctx, decider, channel, call)
	if err != nil {
		t.Fatalf("running an ordinary command failed: %v", err)
	}

	if !ran {
		t.Error("an ordinary command did not run, and the agent works on its own by default")
	}
	if shown := channel.Previews(); len(shown) != 0 {
		t.Errorf("the user was shown %v for an ordinary command, and only the ask-me-first list reaches them", shown)
	}
}

func TestAScheduledRunTellsTheUserWhatItWouldHaveAskedAndStops(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	channel := testkit.NewFakeChannel("signal")
	decider := newPermission(t)
	call := clearTheBuildFolder()

	decision, err := decider.Decide(ctx, contract.PermissionRequest{
		ToolName:   call.Name,
		Input:      call.Input,
		Unattended: true,
	})
	if err != nil {
		t.Fatalf("ruling on a scheduled call failed: %v", err)
	}
	if decision.Ruling != contract.RulingStop {
		t.Fatalf("a scheduled call was ruled %q, want %q", decision.Ruling, contract.RulingStop)
	}
	if err := channel.Send(ctx, "the task stopped: "+decision.Reason+"\n\n"+decision.PreviewText); err != nil {
		t.Fatalf("sending the report failed: %v", err)
	}

	if len(channel.Previews()) != 0 {
		t.Error("a scheduled run asked the user something, and there is nobody there to answer")
	}
	if sent := channel.Sent(); len(sent) != 1 || !strings.Contains(sent[0], "rm -rf /home/jared/build") {
		t.Errorf("the report was %v, and it has to say what the task would have asked about", sent)
	}
}

// clearTheBuildFolder is the tool call every test in this file puts to the
// permission function: a recursive delete, which is the first thing on the
// ask-me-first list.
func clearTheBuildFolder() contract.ToolCall {
	return contract.ToolCall{
		ID:    "1",
		Name:  contract.ToolShell,
		Input: json.RawMessage(`{"command":"rm -rf /home/jared/build"}`),
	}
}

// newPermission builds the permission function a fresh install would have.
func newPermission(t *testing.T) *permission.Decider {
	t.Helper()
	decider, err := permission.New(contract.DefaultConfig(), testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("building the permission function failed: %v", err)
	}
	return decider
}

// askAndRun is the smallest possible stand-in for the guard inside the agent
// loop: rule on the call, put it to the user when the ruling says to ask,
// remember the answer, and say whether the tool would have run. Wave 3 replaces
// it with internal/loop, and these tests keep their assertions.
func askAndRun(ctx context.Context, decider *permission.Decider, channel contract.Channel, call contract.ToolCall) (bool, error) {
	request := contract.PermissionRequest{ToolName: call.Name, Input: call.Input}
	decision, err := decider.Decide(ctx, request)
	if err != nil {
		return false, err
	}
	if decision.Ruling == contract.RulingAllow {
		return true, nil
	}
	if decision.Ruling == contract.RulingDeny {
		return false, channel.Send(ctx, "the call was refused: "+decision.Reason)
	}

	answer, err := channel.ShowPreview(ctx, contract.Preview{ID: call.ID, Title: decision.Reason, Body: decision.PreviewText})
	if err != nil {
		return false, err
	}
	if err := decider.Remember(request, answer.Answer, ""); err != nil {
		return false, err
	}
	if answer.Answer == contract.AnswerReject {
		return false, channel.Send(ctx, "the call was refused by you")
	}
	return true, nil
}
