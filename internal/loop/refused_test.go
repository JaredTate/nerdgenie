package loop_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestARefusedRecordWriteIsNeverWrittenDownAsAChange holds the rule the first
// live run broke: the record's result line for a task call the record refused
// must say it was refused. It said "updated the record: one change" instead, and
// the model went on believing the write had happened.
func TestARefusedRecordWriteIsNeverWrittenDownAsAChange(t *testing.T) {
	built := newHarness(t, []testkit.Step{
		{
			Text:   "I will write the done list.",
			Finish: contract.FinishToolCalls,
			// A decision with no reason is refused by the record's own rules,
			// which is the shape of every refusal: the harness says no and the
			// model has to be told.
			ToolCalls: []contract.ToolCall{{ID: "call-task", Name: contract.ToolTask, Input: json.RawMessage(
				`{"decision":"use the second shelf"}`)}},
		},
		{
			Text:   "I understand, and I will say why next time.",
			Finish: contract.FinishEnd,
		},
	})

	outcome := built.ask(t, "put the jars away")

	line := theResultLineOf(t, built, outcome.TaskID)
	if strings.Contains(line, "updated the record") {
		t.Errorf("the result line is %q, and it says the record was changed by a call the record refused", line)
	}
	if !strings.Contains(strings.ToLower(line), "refused") {
		t.Errorf("the result line is %q, and it does not say the change was refused", line)
	}
}

// theResultLineOf is the one result line the task wrote into its record.
func theResultLineOf(t *testing.T, built *harness, taskID string) string {
	t.Helper()
	written := built.held(t, taskID)
	for _, result := range written.Work.Results {
		return result.Summary
	}
	t.Fatalf("the record holds no result at all, so nothing was written down:\n%+v", written.Work)
	return ""
}
