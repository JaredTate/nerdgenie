package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theShowClock is the moment the fakes in these tests read the time from.
var theShowClock = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

// aShowingOverFakes is a show answerer over a fake log and a fake job store:
// task 6 holds one shell call and its result r1, and job 1 waits with one ask.
// The keeper of task 6 comes back too, so a test can add to the task.
func aShowingOverFakes(t *testing.T) (*showing, *record.Keeper) {
	t.Helper()
	ctx := context.Background()
	store := testkit.NewFakeStore()
	keeper, err := record.New(ctx, store, record.Start{
		Kind: contract.RecordTask, ID: "6", Origin: contract.TerminalChannelName,
		Ask: "list the folder", RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot start the record of task 6: %v", err)
	}
	logAToolCall(t, store, "6", "shell", `{"command":"ls -l"}`)
	if _, err := keeper.AddResult(ctx, "listed the folder", "file a\nfile b"); err != nil {
		t.Fatalf("cannot add the result of the shell call to task 6: %v", err)
	}
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theShowClock))
	if _, err := jobs.Create(ctx, contract.NewJob{Ask: "water the plants"}); err != nil {
		t.Fatalf("cannot create the job: %v", err)
	}
	return &showing{store: store, jobs: jobs}, keeper
}

// logAToolCall writes one tool call event under a task, the way the loop writes
// one before it runs the tool.
func logAToolCall(t *testing.T, store contract.Store, task string, name string, input string) {
	t.Helper()
	body, err := json.Marshal(contract.ToolCall{ID: "call_" + name, Name: name, Input: json.RawMessage(input)})
	if err != nil {
		t.Fatalf("cannot write the %s call as JSON: %v", name, err)
	}
	if _, err := store.Append(context.Background(), contract.Event{TaskID: task, Kind: contract.EventToolCall, Body: body}); err != nil {
		t.Fatalf("cannot write the %s call into the log: %v", name, err)
	}
}

// TestAShowOfAResultCarriesTheCallAndTheText holds the layout a screen opens a
// result into: the tool's name on the first line, its input laid out as JSON,
// a blank line, the word result, and the whole of the stored text, with the id
// and the task carried back in the fields.
func TestAShowOfAResultCarriesTheCallAndTheText(t *testing.T) {
	answering, _ := aShowingOverFakes(t)

	got, err := answering.answer(context.Background(), map[string]string{"id": "r1", "task": "6"})
	if err != nil {
		t.Fatalf("showing r1 of task 6 failed: %v", err)
	}

	if got.Type != contract.SocketShown {
		t.Errorf("the answer is a %s, want a shown", got.Type)
	}
	want := "call: shell\n{\n  \"command\": \"ls -l\"\n}\n\nresult:\nfile a\nfile b"
	if got.Text != want {
		t.Errorf("the shown text is:\n%s\nwant:\n%s", got.Text, want)
	}
	if got.Fields["id"] != "r1" || got.Fields["task"] != "6" {
		t.Errorf("the shown carries the fields %v, want the id r1 and the task 6", got.Fields)
	}
}

// TestAShowOfAResultFindsTheCallJustBeforeIt holds that with several calls in
// the log, the one shown is the call written just before the result, not the
// first or the last call of the task.
func TestAShowOfAResultFindsTheCallJustBeforeIt(t *testing.T) {
	answering, keeper := aShowingOverFakes(t)
	ctx := context.Background()
	logAToolCall(t, answering.store, "6", "read", `{"path":"notes.md"}`)
	if _, err := keeper.AddResult(ctx, "read the notes", "the notes"); err != nil {
		t.Fatalf("cannot add the second result: %v", err)
	}
	logAToolCall(t, answering.store, "6", "write", `{"path":"out.md"}`)

	got, err := answering.answer(ctx, map[string]string{"id": "r2", "task": "6"})
	if err != nil {
		t.Fatalf("showing r2 of task 6 failed: %v", err)
	}

	if !strings.HasPrefix(got.Text, "call: read\n") {
		t.Errorf("the shown text begins %q, want the read call that made r2", firstLineOf(got.Text))
	}
	if !strings.HasSuffix(got.Text, "\nresult:\nthe notes") {
		t.Errorf("the shown text ends %q, want the stored text of r2", got.Text)
	}
}

// TestAShowOfAResultWithNoCallRecordedSaysSo holds that a result the log holds
// no call for, such as the reply the harness writes as a result of its own, is
// still shown, with a first line saying no call was recorded.
func TestAShowOfAResultWithNoCallRecordedSaysSo(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewFakeStore()
	keeper, err := record.New(ctx, store, record.Start{
		Kind: contract.RecordTask, ID: "9", Origin: contract.TerminalChannelName,
		Ask: "say hello", RoundsLeft: 10, MinutesLeft: 10,
	})
	if err != nil {
		t.Fatalf("cannot start the record of task 9: %v", err)
	}
	if _, err := keeper.AddResult(ctx, "the reply", "hello there"); err != nil {
		t.Fatalf("cannot add the reply as a result: %v", err)
	}
	answering := &showing{store: store}

	got, err := answering.answer(ctx, map[string]string{"id": "r1", "task": "9"})
	if err != nil {
		t.Fatalf("showing r1 of task 9 failed: %v", err)
	}

	if !strings.HasPrefix(got.Text, "call: ") || !strings.Contains(firstLineOf(got.Text), "no call") {
		t.Errorf("the shown text begins %q, want a first line saying no call was recorded", firstLineOf(got.Text))
	}
	if !strings.HasSuffix(got.Text, "\n\nresult:\nhello there") {
		t.Errorf("the shown text ends %q, want the stored text after the result line", got.Text)
	}
}

// TestAShowOfATaskIsItsPrintedRecord holds that a show naming only a task
// answers with the record as the record package prints it, with the task
// carried back in the fields.
func TestAShowOfATaskIsItsPrintedRecord(t *testing.T) {
	answering, keeper := aShowingOverFakes(t)

	got, err := answering.answer(context.Background(), map[string]string{"task": "6"})
	if err != nil {
		t.Fatalf("showing task 6 failed: %v", err)
	}

	if got.Type != contract.SocketShown {
		t.Errorf("the answer is a %s, want a shown", got.Type)
	}
	if want := string(record.Print(keeper.Record())); got.Text != want {
		t.Errorf("the shown text is:\n%s\nwant the printed record:\n%s", got.Text, want)
	}
	if got.Fields["task"] != "6" || got.Fields["id"] != "" {
		t.Errorf("the shown carries the fields %v, want only the task 6", got.Fields)
	}
}

// TestAShowOfAJobIsItsPrintedRecord holds that a show naming only a job answers
// with the job's record as the job store loads it and the record package prints
// it, with the job carried back in the fields.
func TestAShowOfAJobIsItsPrintedRecord(t *testing.T) {
	answering, _ := aShowingOverFakes(t)
	ctx := context.Background()
	held, err := answering.jobs.Load(ctx, "1")
	if err != nil {
		t.Fatalf("cannot load job 1 from the fake store: %v", err)
	}

	got, err := answering.answer(ctx, map[string]string{"job": "1"})
	if err != nil {
		t.Fatalf("showing job 1 failed: %v", err)
	}

	if want := string(record.Print(held)); got.Text != want {
		t.Errorf("the shown text is:\n%s\nwant the printed job:\n%s", got.Text, want)
	}
	if got.Fields["job"] != "1" {
		t.Errorf("the shown carries the fields %v, want the job 1", got.Fields)
	}
}

// TestAShowOfSomethingNotThereIsAnErrorThatSaysWhatToAsk holds the four ways a
// show can name nothing the program has: a result nobody wrote, a task and a
// job nobody made, and a job when there is no job store. Each error names what
// was asked and says what to ask instead.
func TestAShowOfSomethingNotThereIsAnErrorThatSaysWhatToAsk(t *testing.T) {
	answering, _ := aShowingOverFakes(t)
	withoutJobs := &showing{store: answering.store}
	ctx := context.Background()

	for _, missing := range []struct {
		answering *showing
		fields    map[string]string
		named     string
	}{
		{answering, map[string]string{"id": "r9", "task": "6"}, "r9"},
		{answering, map[string]string{"task": "404"}, "404"},
		{answering, map[string]string{"job": "9"}, "9"},
		{withoutJobs, map[string]string{"job": "1"}, "1"},
	} {
		_, err := missing.answering.answer(ctx, missing.fields)
		if err == nil {
			t.Errorf("showing %v answered, want an error", missing.fields)
			continue
		}
		if !strings.Contains(err.Error(), missing.named) {
			t.Errorf("the error for %v reads %q, and it has to name what was asked", missing.fields, err)
		}
		if len(strings.Fields(err.Error())) < 6 {
			t.Errorf("the error for %v reads %q, and it has to say what to ask instead", missing.fields, err)
		}
	}
}

// TestAShowAskingForNothingKnownIsAnError holds that a show in no shape the
// contract names, such as one with no fields, an id with no task, or a task
// and a job at once, is refused with the three shapes it may take.
func TestAShowAskingForNothingKnownIsAnError(t *testing.T) {
	answering, _ := aShowingOverFakes(t)
	ctx := context.Background()

	for _, fields := range []map[string]string{
		nil,
		{},
		{"id": "r1"},
		{"task": "6", "job": "1"},
		{"colour": "blue"},
	} {
		_, err := answering.answer(ctx, fields)
		if err == nil {
			t.Errorf("showing %v answered, want an error", fields)
			continue
		}
		for _, word := range []string{"id", "task", "job"} {
			if !strings.Contains(err.Error(), word) {
				t.Errorf("the error for %v reads %q, and it has to name the field %q a show may carry", fields, err, word)
			}
		}
	}
}

// TestAShownTextIsCappedAndSaysHowMuchWasLeftOut holds the bound: a result
// longer than the cap is cut to it, and the last line says how much was left
// out rather than leaving the reader to guess.
func TestAShownTextIsCappedAndSaysHowMuchWasLeftOut(t *testing.T) {
	answering, keeper := aShowingOverFakes(t)
	ctx := context.Background()
	long := strings.Repeat("a line of the very long result\n", 4000)
	if _, err := keeper.AddResult(ctx, "a long result", long); err != nil {
		t.Fatalf("cannot add the long result: %v", err)
	}

	got, err := answering.answer(ctx, map[string]string{"id": "r2", "task": "6"})
	if err != nil {
		t.Fatalf("showing the long result failed: %v", err)
	}

	if len(got.Text) > maxShownBytes {
		t.Errorf("the shown text is %d bytes, want it within the cap of %d", len(got.Text), maxShownBytes)
	}
	if len(got.Text) < maxShownBytes/2 {
		t.Errorf("the shown text is %d bytes, so far under the cap that most of the result was thrown away", len(got.Text))
	}
	last := lastLineOf(got.Text)
	if !strings.Contains(last, "left out") {
		t.Errorf("the last line reads %q, and it has to say how much was left out", last)
	}
	if !strings.HasPrefix(got.Text, "call: shell\n") {
		t.Errorf("the shown text begins %q, and the cap must not take the call away", firstLineOf(got.Text))
	}
}

// TestAShortShownTextIsNotCut holds the other side of the cap: a text inside
// it comes back whole, with no line about anything left out.
func TestAShortShownTextIsNotCut(t *testing.T) {
	if got := withinTheShownCap("a short text"); got != "a short text" {
		t.Errorf("a short text was changed to %q", got)
	}
}

// TestTheAgentAnswersAShowOverItsSocket holds the wiring: the agent's own
// socket answers a show of a task in its log with that task's printed record.
func TestTheAgentAnswersAShowOverItsSocket(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aTaskInTheLog(t, running.events, "7", fromTheTerminal, contract.StatusStopped)
	held, ok := loadTaskRecord(context.Background(), running.events, "7")
	if !ok {
		t.Fatal("task 7 was written and cannot be loaded back")
	}
	connection := aScreenAsking(t, running)

	if err := contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
		Type: contract.SocketShow, Fields: map[string]string{"task": "7"},
	}); err != nil {
		t.Fatalf("cannot send the show: %v", err)
	}

	line, err := bufio.NewReader(connection).ReadBytes('\n')
	if err != nil {
		t.Fatalf("nothing came back for the show: %v", err)
	}
	got, err := contract.DecodeSocketEnvelope(line)
	if err != nil {
		t.Fatalf("the answer to the show is not a message: %v", err)
	}
	if got.Type != contract.SocketShown {
		t.Fatalf("the show was answered with a %s carrying %q, want a shown", got.Type, got.Text)
	}
	if want := string(record.Print(held)); got.Text != want {
		t.Errorf("the shown text is:\n%s\nwant the printed record of task 7:\n%s", got.Text, want)
	}
}

// aScreenAsking starts the socket accepting and dials it without attaching, so
// that the one line that comes back is the answer to what the screen sends.
func aScreenAsking(t *testing.T, running *agent) net.Conn {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	go func() { _ = running.socket.Serve(ctx) }()

	connection, err := net.Dial("unix", running.home.SocketFile())
	if err != nil {
		t.Fatalf("cannot dial the socket at %s: %v", running.home.SocketFile(), err)
	}
	t.Cleanup(func() { _ = connection.Close(); stop() })
	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("cannot put a deadline on the screen's link: %v", err)
	}
	return connection
}

// firstLineOf is the first line of a text.
func firstLineOf(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	return first
}

// lastLineOf is the last line of a text, passing over a newline at its end.
func lastLineOf(text string) string {
	trimmed := strings.TrimRight(text, "\n")
	return trimmed[strings.LastIndex(trimmed, "\n")+1:]
}
