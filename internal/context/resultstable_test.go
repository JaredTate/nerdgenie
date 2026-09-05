package context

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aTaskWithThreeResults is a record holding three results, and the two
// messages that made the last two of them, each carrying the label the record
// gave it. The first result's message has already left the table.
func aTaskWithThreeResults(t *testing.T) (contract.Record, []contract.Message) {
	t.Helper()
	keeper, err := record.New(t.Context(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: "1", Origin: "terminal", Ask: "read the notes", RoundsLeft: 100, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot open a record: %v", err)
	}
	labels := []string{}
	for _, summary := range []string{"read: the product notes", "shell: 51 tests passing", "read: the draft"} {
		label, err := keeper.AddResult(t.Context(), summary, summary+"\nin full")
		if err != nil {
			t.Fatalf("cannot add a result: %v", err)
		}
		labels = append(labels, label)
	}
	messages := []contract.Message{}
	for _, label := range labels[1:] {
		messages = append(messages,
			contract.Message{Role: contract.RoleAssistant, Text: "I will look.", ToolCalls: []contract.ToolCall{{ID: "call-" + label, Name: "read"}}},
			contract.Message{Role: contract.RoleUser, ToolResults: []contract.ToolResult{{CallID: "call-" + label, Label: label, Text: "in full"}}},
		)
	}
	return keeper.Record(), messages
}

// TestTheRecordListsOnlyTheResultsThatHaveLeftTheTable is the tail the live
// game build re-read on every call: a hundred and twenty lines of results, one
// per round, three thousand tokens rewritten under the conversation each call,
// although a hundred of those results were on the table in full a few messages
// up. A result on the table carries its own label, so the list names only the
// ones that have left, which is what the list is for.
func TestTheRecordListsOnlyTheResultsThatHaveLeftTheTable(t *testing.T) {
	held, messages := aTaskWithThreeResults(t)
	builder := newGoldenBuilder(t)

	request, err := builder.Build(t.Context(), BuildInput{ContextLength: 200000, Record: held, Messages: messages, Tools: sampleTools()})
	if err != nil {
		t.Fatalf("cannot build the prompt: %v", err)
	}

	listed := theMessageHeaded(request, recordResultsHeading)
	if !strings.Contains(listed, "- r1 ") {
		t.Errorf("the list of results reads:\n%s\nwant r1, whose full text has left the table", listed)
	}
	for _, onTheTable := range []string{"- r2 ", "- r3 "} {
		if strings.Contains(listed, onTheTable) {
			t.Errorf("the list of results reads:\n%s\nand %q is on the table in full a few messages up", listed, onTheTable)
		}
	}
}

// TestAResultOnTheTableCarriesItsOwnLabel is what lets the list leave it out:
// the model still has to name r2 to pin it or point a done line at it, so the
// label rides on the result itself, in the harness's own words above the data
// marker.
func TestAResultOnTheTableCarriesItsOwnLabel(t *testing.T) {
	held, messages := aTaskWithThreeResults(t)
	builder := newGoldenBuilder(t)

	request, err := builder.Build(t.Context(), BuildInput{ContextLength: 200000, Record: held, Messages: messages, Tools: sampleTools()})
	if err != nil {
		t.Fatalf("cannot build the prompt: %v", err)
	}

	found := false
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			if result.Label == "r2" {
				found = true
				if !strings.HasPrefix(result.Text, "r2:\n") || !strings.Contains(result.Text, "--- begin tool result") {
					t.Errorf("the result r2 reads:\n%s\nwant its label on the first line and the data marker under it", result.Text)
				}
			}
		}
	}
	if !found {
		t.Error("the result r2 is not in the prompt at all")
	}
}

// TestWhenEveryResultIsOnTheTableThereIsNoList holds the other end: with
// nothing left to name, the list is not sent, blank heading and all.
func TestWhenEveryResultIsOnTheTableThereIsNoList(t *testing.T) {
	held, messages := aTaskWithThreeResults(t)
	messages = append([]contract.Message{
		{Role: contract.RoleAssistant, Text: "I will read the notes.", ToolCalls: []contract.ToolCall{{ID: "call-r1", Name: "read"}}},
		{Role: contract.RoleUser, ToolResults: []contract.ToolResult{{CallID: "call-r1", Label: "r1", Text: "in full"}}},
	}, messages...)
	builder := newGoldenBuilder(t)

	request, err := builder.Build(t.Context(), BuildInput{ContextLength: 200000, Record: held, Messages: messages, Tools: sampleTools()})
	if err != nil {
		t.Fatalf("cannot build the prompt: %v", err)
	}

	if listed := theMessageHeaded(request, recordResultsHeading); listed != "" {
		t.Errorf("the prompt carries a list of results:\n%s\nand every result is on the table", listed)
	}
}

// TestResultsWithNoLabelAreAllListed keeps a task picked up from disk whole:
// its messages carry no labels, so nothing is known to be on the table, and the
// list names every result as it always did.
func TestResultsWithNoLabelAreAllListed(t *testing.T) {
	held, messages := aTaskWithThreeResults(t)
	for at := range messages {
		for on := range messages[at].ToolResults {
			messages[at].ToolResults[on].Label = ""
		}
	}
	builder := newGoldenBuilder(t)

	request, err := builder.Build(t.Context(), BuildInput{ContextLength: 200000, Record: held, Messages: messages, Tools: sampleTools()})
	if err != nil {
		t.Fatalf("cannot build the prompt: %v", err)
	}

	listed := theMessageHeaded(request, recordResultsHeading)
	for _, label := range []string{"- r1 ", "- r2 ", "- r3 "} {
		if !strings.Contains(listed, label) {
			t.Errorf("the list of results reads:\n%s\nwant %q, because nothing says it is on the table", listed, label)
		}
	}
}
