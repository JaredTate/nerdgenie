package loop_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
	"github.com/JaredTate/coeus/internal/testkit"
)

// The two numbers this file was written against, both pinned by the tests below
// rather than left to whatever the code happens to do.
const (
	// roundsInTheShortTask is a handful of tool rounds: enough to see the
	// checkpoints of several rounds beside each other, and few enough that the
	// task is not worth a review, whose call is a model call with no round of
	// work behind it.
	roundsInTheShortTask = 4
	// roundsInTheLongTask is the forty rounds of the fixture, which is the
	// length of task the size of a checkpoint is measured on.
	roundsInTheLongTask = 40
	// wordsInTheLongAsk is how long an ask this test writes. Two thousand words
	// is a page and a half of somebody pasting a specification into the
	// terminal, which is the ask the wave 6 trial found: it is the user's own
	// words, so nothing ever shortens it where it is kept.
	wordsInTheLongAsk = 2000
	// checkpointBytesAFortyRoundTaskMayWrite is the whole size of the
	// checkpoint events one such task writes into the log. Before brief 6.6 a
	// checkpoint was saved for every change to the record, each one holding the
	// whole record with the whole ask inside it, which came to well over two
	// megabytes on this task. One checkpoint per model call, with the ask kept
	// by reference to the checkpoint that first carried it, brings it under
	// this.
	checkpointBytesAFortyRoundTaskMayWrite = 200 << 10
)

// TestOneCheckpointIsSavedForEveryModelCall is brief 6.6's second loop item. A
// checkpoint holds the whole record printed, and the loop used to save one for
// every change: the budget, the cost, every result, and the situation, which is
// four writes of the whole record for one tool call. A round is one model call,
// and a round is where a replay reads the boundary, so a round is one
// checkpoint, with one more at the end for where the task finished, which no
// round after it would ever save.
func TestOneCheckpointIsSavedForEveryModelCall(t *testing.T) {
	built := newHarness(t, aScriptOfRounds(roundsInTheShortTask), theNotesTool(roundsInTheShortTask))

	built.ask(t, "read the notes")

	calls := len(built.model.Requests())
	saved := len(built.eventsOfKind(t, contract.EventCheckpoint))
	if calls != roundsInTheShortTask+1 {
		t.Fatalf("the task made %d model calls, and the script has %d rounds and an answer",
			calls, roundsInTheShortTask)
	}
	if saved != calls+1 {
		t.Errorf("the task made %d model calls and saved %d checkpoints, and it saves one per call and one for the ending",
			calls, saved)
	}
}

// TestTheCheckpointOfARoundIsSavedBeforeItsToolsRun pins where in a round the
// checkpoint goes. The replay reads the round boundary off the budget line of
// each checkpoint, so the checkpoint has to carry the budget of the round whose
// tools are about to run, not the one that has just finished.
func TestTheCheckpointOfARoundIsSavedBeforeItsToolsRun(t *testing.T) {
	built := newHarness(t, aScriptOfRounds(roundsInTheShortTask), theNotesTool(roundsInTheShortTask))

	built.ask(t, "read the notes")

	saved := checkpointsInTheLog(t, built)
	budgets := []int{}
	for _, one := range saved {
		held, err := record.Parse([]byte(one.Text))
		if err != nil {
			t.Fatalf("checkpoint %d does not read back as a record: %v", one.Number, err)
		}
		budgets = append(budgets, held.Header.RoundsLeft)
	}
	// The last checkpoint is the ending, which spends no round of its own.
	rounds := budgets[:len(budgets)-1]
	for at := 1; at < len(rounds); at++ {
		if rounds[at] >= rounds[at-1] {
			t.Errorf("checkpoint %d has %d rounds left and the one before it %d, so a round did not spend one",
				at+1, rounds[at], rounds[at-1])
		}
	}
	ending, err := record.Parse([]byte(saved[len(saved)-1].Text))
	if err != nil {
		t.Fatalf("the last checkpoint does not read back as a record: %v", err)
	}
	if ending.Header.Status != contract.StatusWaiting {
		t.Errorf("the last checkpoint says the task is %q, and the script ends with a question", ending.Header.Status)
	}
}

// TestAFortyRoundTaskWithALongAskWritesFewCheckpointBytes is the size half of
// the finding. The ask is the user's own words and is never shortened where it
// is kept, so a long one used to be copied into every checkpoint of the task:
// twenty kilobytes, four times a tool call, forty rounds deep.
func TestAFortyRoundTaskWithALongAskWritesFewCheckpointBytes(t *testing.T) {
	built := newHarness(t, aScriptOfRounds(roundsInTheLongTask), theNotesTool(roundsInTheLongTask))
	ask := aLongAsk(wordsInTheLongAsk)

	outcome := built.ask(t, ask)

	written, saved := 0, built.eventsOfKind(t, contract.EventCheckpoint)
	for _, event := range saved {
		written += len(event.Body)
	}
	t.Logf("a %d-round task with a %d-word ask wrote %d checkpoints and %d bytes of them",
		roundsInTheLongTask, wordsInTheLongAsk, len(saved), written)
	if len(saved) != roundsInTheLongTask+2 {
		t.Errorf("the task saved %d checkpoints, and %d rounds and an answer save one each with one more for the ending",
			len(saved), roundsInTheLongTask)
	}
	if written > checkpointBytesAFortyRoundTaskMayWrite {
		t.Errorf("the task wrote %d bytes of checkpoints and the bound is %d, so the record is being copied into the log more often or more whole than it needs to be",
			written, checkpointBytesAFortyRoundTaskMayWrite)
	}
	checkTheLongAskReadsBackWhole(t, built, outcome.TaskID, ask)
}

// checkTheLongAskReadsBackWhole proves nothing was lost by keeping the ask in
// one checkpoint instead of all of them: the record loaded out of the log holds
// the user's words as the user wrote them, and its printed form carries them.
func checkTheLongAskReadsBackWhole(t *testing.T, built *harness, taskID string, ask string) {
	t.Helper()
	keeper, err := record.Load(t.Context(), built.store, contract.RecordTask, taskID)
	if err != nil {
		t.Fatalf("cannot load the record of task %s: %v", taskID, err)
	}
	if held := keeper.Record().Goal.Ask; held != ask {
		t.Errorf("the ask reads back as %d characters and the user wrote %d", len(held), len(ask))
	}
	if !strings.Contains(keeper.Text(), ask) {
		t.Error("the record's printed form does not carry the whole ask, and it is what goes in front of the model")
	}
}

// checkpointsInTheLog is every checkpoint the loop saved, in the order it saved
// them.
func checkpointsInTheLog(t *testing.T, built *harness) []record.Checkpoint {
	t.Helper()
	saved := []record.Checkpoint{}
	for _, event := range built.eventsOfKind(t, contract.EventCheckpoint) {
		one := record.Checkpoint{}
		if err := json.Unmarshal(event.Body, &one); err != nil {
			t.Fatalf("an event of the checkpoint kind does not read back as one: %v", err)
		}
		saved = append(saved, one)
	}
	return saved
}

// aScriptOfRounds is a task of the given number of tool rounds, each reading a
// file of its own so that the identical-call guard has nothing to catch, ending
// with a question that leaves the task waiting.
func aScriptOfRounds(rounds int) []testkit.Step {
	steps := []testkit.Step{
		callStep("Nothing is read yet. I will start with the first note.",
			callFor("c1", "read", `{"path":"notes-1.md"}`),
			taskCall("c1t", `{"why":"the user wants the notes read","doneWhen":["every note is read"]}`)),
	}
	for at := 2; at <= rounds; at++ {
		steps = append(steps, callStep(fmt.Sprintf("Note %d is read. I will read the next one.", at-1),
			callFor(fmt.Sprintf("c%d", at), "read", fmt.Sprintf(`{"path":"notes-%d.md"}`, at))))
	}
	return append(steps, answerStep("Every note I was given is read. Which folder should I read next?"))
}

// theNotesTool answers one line per round, so that every round of the script
// has a result to write into the record.
func theNotesTool(rounds int) contract.Tool {
	answers := make([]string, 0, rounds)
	for at := 1; at <= rounds; at++ {
		answers = append(answers, fmt.Sprintf("note %d says the meeting is on Tuesday and the room has changed", at))
	}
	return scriptedTool("read", answers...)
}

// aLongAsk is a page and a half of the user's own words, which is what somebody
// pasting a specification into the terminal writes.
func aLongAsk(words int) string {
	said := make([]string, 0, words)
	for at := range words {
		said = append(said, fmt.Sprintf("word%d", at+1))
	}
	return "Read every note in the folder and tell me what changed. " + strings.Join(said, " ")
}
