package replay_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/replay"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// twoRoundScript is a task the model finishes in two rounds of tools and a
// third reply that answers, which is the smallest run with more than one round.
func twoRoundScript() scriptedTask {
	return scriptedTask{
		ask:   "count the files in the notes folder",
		tools: []contract.Tool{scriptedTool("search", "three files", "notes.md, list.md, plan.md")},
		steps: []testkit.Step{
			{
				Text: "I am starting on the notes folder.",
				ToolCalls: []contract.ToolCall{
					callTo("c1", "search", `{"pattern":"*"}`),
					callTo("c2", "task", `{"why":"the user wants a count","done_when":["the files are counted"],`+
						`"plan":["read the folder","name the files"]}`),
				},
			},
			{
				Text: "I have the count and now want the names.",
				ToolCalls: []contract.ToolCall{
					callTo("c3", "search", `{"pattern":"*.md"}`),
					callTo("c4", "task", `{"done_when":[{"text":"the files are counted","done":true,"resultId":"r3"}]}`),
				},
			},
			{Text: "There are three files: notes.md, list.md, plan.md."},
		},
	}
}

func TestReadFindsEveryRoundOfARecordedTask(t *testing.T) {
	made := runScript(t, twoRoundScript())

	recording, err := replay.Read(context.Background(), made.store, made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot read the recording of task %s: %v", made.outcome.TaskID, err)
	}

	if recording.TaskID != made.outcome.TaskID {
		t.Errorf("the recording is of task %q and the run was task %q", recording.TaskID, made.outcome.TaskID)
	}
	if recording.Ask != "count the files in the notes folder" {
		t.Errorf("the recording's ask is %q and the user asked %q", recording.Ask, "count the files in the notes folder")
	}
	if len(recording.Rounds) != 2 {
		t.Fatalf("the recording holds %d rounds and the run made two that called tools: %+v", len(recording.Rounds), recording.Rounds)
	}
	first := recording.Rounds[0]
	if first.Orient != "I am starting on the notes folder." {
		t.Errorf("the first round's orient line is %q and the model wrote %q", first.Orient, "I am starting on the notes folder.")
	}
	if len(first.Calls) != 2 || first.Calls[0].Name != "search" || first.Calls[1].Name != "task" {
		t.Fatalf("the first round holds %+v and the model asked for a search and a record write", first.Calls)
	}
	if first.Calls[0].Result != "three files" {
		t.Errorf("the first call's result is %q and the tool returned %q", first.Calls[0].Result, "three files")
	}
	if !first.Calls[0].Ran {
		t.Error("the first call ran in the recording and the recording says it did not")
	}
	if recording.Answer != "There are three files: notes.md, list.md, plan.md." {
		t.Errorf("the recording's answer is %q and the task replied %q", recording.Answer, "There are three files: notes.md, list.md, plan.md.")
	}
	if recording.Final.Header.Status != contract.StatusDone && recording.Final.Header.Status != contract.StatusFailed {
		t.Errorf("the recorded record ends %q, which is not an end state", recording.Final.Header.Status)
	}
}

func TestReadKeepsACallThatNeverReachedATool(t *testing.T) {
	scripted := twoRoundScript()
	scripted.rules = map[string]contract.PermissionDecision{
		"search": {Ruling: contract.RulingDeny, Reason: "the test refuses every search"},
	}
	made := runScript(t, scripted)

	recording, err := replay.Read(context.Background(), made.store, made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot read the recording of the refused task: %v", err)
	}
	if len(recording.Rounds) == 0 {
		t.Fatal("the refused task recorded no rounds at all")
	}
	first := recording.Rounds[0].Calls[0]
	if first.Ran {
		t.Error("the refused call is recorded as having run, and no tool ever saw it")
	}
	if first.Result != "" {
		t.Errorf("the refused call carries the result %q, and a refused call has none", first.Result)
	}
}

func TestReadKeepsAMessageThatArrivedMidTask(t *testing.T) {
	made := runScriptWithACorrection(t)

	recording, err := replay.Read(context.Background(), made.store, made.outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot read the recording of the corrected task: %v", err)
	}
	delivered := []string{}
	for _, round := range recording.Rounds {
		for _, message := range round.Delivered {
			delivered = append(delivered, message.Text)
		}
	}
	if len(delivered) != 1 || delivered[0] != theCorrection {
		t.Fatalf("the recording carries the mid-task messages %v and the user sent %q", delivered, theCorrection)
	}
	if len(recording.Final.Rules.Corrections) != 1 {
		t.Errorf("the recorded record holds %d corrections and the user made one", len(recording.Final.Rules.Corrections))
	}
}

func TestReadRefusesATaskTheLogDoesNotHold(t *testing.T) {
	store := testkit.NewFakeStore()

	_, err := replay.Read(context.Background(), store, "17")
	if err == nil {
		t.Fatal("reading a task that is not in the log gave no error")
	}
	if !strings.Contains(err.Error(), "17") {
		t.Errorf("the error is %q and it must name the task that is missing", err)
	}
}

func TestReadRefusesATaskWithNoCheckpoints(t *testing.T) {
	store := testkit.NewFakeStore()
	if _, err := store.Append(context.Background(), contract.Event{
		TaskID: "17", Kind: contract.EventToolCall, Body: []byte(`{"Name":"search"}`),
	}); err != nil {
		t.Fatalf("cannot write the one event the test needs: %v", err)
	}

	_, err := replay.Read(context.Background(), store, "17")
	if err == nil {
		t.Fatal("reading a task with no checkpoint gave no error")
	}
	if !strings.Contains(err.Error(), "checkpoint") {
		t.Errorf("the error is %q and it must say that the task saved no checkpoint", err)
	}
}

// aLogThatCutTheReadShort is the event log when a task holds more events than
// one read returns: the events it could hand back, and beside them the paging
// advice internal/log gives, which is advice for a caller that means to read on
// and not for a person who asked for a replay.
type aLogThatCutTheReadShort struct {
	*testkit.FakeStore
}

// ByTask hands back what it read and says the read stopped short, word for word
// as the real log says it.
func (cutShort aLogThatCutTheReadShort) ByTask(ctx context.Context, taskID string) ([]contract.Event, error) {
	events, err := cutShort.FakeStore.ByTask(ctx, taskID)
	if err != nil {
		return events, err
	}
	return events, fmt.Errorf("this read of the log at %s stopped at %d events, which is all one read returns, so read the rest in pages with ByRange starting after event %d",
		"/home/somebody/.coeus/coeus.db", len(events), len(events))
}

func TestReadSaysATaskIsTooLongToReplayRatherThanPassingOnThePagingAdvice(t *testing.T) {
	made := runScript(t, twoRoundScript())

	_, err := replay.Read(context.Background(), aLogThatCutTheReadShort{made.store}, made.outcome.TaskID)

	if err == nil {
		t.Fatal("a task whose events did not fit in one read was replayed as though the whole of it had been read")
	}
	if !strings.Contains(err.Error(), "too long to replay") {
		t.Errorf("the error is %q and it must say that the task is too long to replay", err)
	}
	if !strings.Contains(err.Error(), made.outcome.TaskID) {
		t.Errorf("the error is %q and it must name the task that cannot be replayed", err)
	}
}

// theCorrection is what the user says in the middle of the corrected task.
const theCorrection = "no, count the folders instead"

// runScriptWithACorrection plays a task the user corrects while it runs, which
// is the case the recording has to carry through a replay.
func runScriptWithACorrection(t *testing.T) recorded {
	t.Helper()
	scripted := twoRoundScript()
	scripted.steps[1].Expect = []string{theCorrection}
	scripted.steps[2].Expect = []string{theCorrection}
	made := runScriptDelivering(t, scripted, theCorrection)
	return made
}
