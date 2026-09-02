package testkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// FortyStepFixturePath is where the fixture lives, from the root of the
// repository.
const FortyStepFixturePath = "test/fixtures/forty-step/task.json"

// FortyStepRound is one round of the fixture: what the model says, what it asks
// for, what it writes into the record, and what the tool gives back.
//
// The last round is the only one with no tool call: its reply is the final
// report, written with the tools turned off.
type FortyStepRound struct {
	// Number is the round's position, starting at one.
	Number int `json:"number"`
	// Orient is the line the model writes before it acts.
	Orient string `json:"orient"`
	// Reply is the model's plain text, which is empty on every round but the
	// last.
	Reply string `json:"reply"`
	// ToolName is the tool the model asks for, and is empty on the last round.
	ToolName string `json:"toolName"`
	// ToolInput is the arguments the model writes.
	ToolInput map[string]any `json:"toolInput"`
	// TaskUpdate is what the model writes into the record this round, through
	// the task tool, or nil when it writes nothing.
	TaskUpdate map[string]any `json:"taskUpdate"`
	// ResultSummary is the one line the record keeps.
	ResultSummary string `json:"resultSummary"`
	// ResultText is the full text the log keeps, which "read r7" brings back.
	ResultText string `json:"resultText"`
}

// FortyStepDoneLine is one line of the fixture's done list, with the result that
// proves it.
type FortyStepDoneLine struct {
	// Text is the line itself.
	Text string `json:"text"`
	// ResultID is the result that proves it, such as "r38".
	ResultID string `json:"resultId"`
}

// FortyStepResult is one result the fixture's tools produced.
type FortyStepResult struct {
	// ID is the result's identifier, "r1" upwards.
	ID string
	// Summary is the one line the record keeps.
	Summary string
	// Text is the full text the log keeps.
	Text string
}

// FortyStepTask is the scripted task the whole design is proved against: forty
// rounds, a correction at round twelve, a stop condition that fires at round
// thirty, and a final report where every done line points at a result.
type FortyStepTask struct {
	// Name is the fixture's name, which is also the fake model's name.
	Name string `json:"name"`
	// ContextLength is the window the fake model reports.
	ContextLength int `json:"contextLength"`
	// Origin is the channel the ask came in on.
	Origin string `json:"origin"`
	// TaskID is the task's number.
	TaskID string `json:"taskId"`
	// Ask is the user's message, word for word, and is never edited.
	Ask string `json:"ask"`
	// Why is the one line on why the user wants it.
	Why string `json:"why"`
	// DoneWhen is the done list, each line with the result that proves it.
	DoneWhen []FortyStepDoneLine `json:"doneWhen"`
	// StopWhen is the stop list the model writes in round one.
	StopWhen []string `json:"stopWhen"`
	// CorrectionRound is the round the user's correction arrives in.
	CorrectionRound int `json:"correctionRound"`
	// Correction is the user's words, kept exactly.
	Correction string `json:"correction"`
	// StopRound is the round whose result fires the stop condition.
	StopRound int `json:"stopRound"`
	// UserReplyAfterStop is what the user says to let the task carry on.
	UserReplyAfterStop string `json:"userReplyAfterStop"`
	// Rounds are the forty rounds, in order.
	Rounds []FortyStepRound `json:"rounds"`
}

// LoadFortyStepTask reads the fixture from the repository.
func LoadFortyStepTask() (FortyStepTask, error) {
	root, err := RepositoryRoot()
	if err != nil {
		return FortyStepTask{}, err
	}
	path := filepath.Join(root, filepath.FromSlash(FortyStepFixturePath))
	content, err := os.ReadFile(path)
	if err != nil {
		return FortyStepTask{}, fmt.Errorf("cannot read the forty-step fixture at %s: %w", path, err)
	}

	var task FortyStepTask
	if err := json.Unmarshal(content, &task); err != nil {
		return FortyStepTask{}, fmt.Errorf("cannot read the forty-step fixture as JSON: %w", err)
	}
	if len(task.Rounds) == 0 {
		return FortyStepTask{}, errors.New("the forty-step fixture has no rounds in it, so check the file on disk")
	}
	return task, nil
}

// RepositoryRoot walks up from the working directory until it finds the go.mod
// file. The walk is bounded, because every loop in Coeus is bounded.
func RepositoryRoot() (string, error) {
	here, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot find the working directory: %w", err)
	}
	for range 20 {
		if _, err := os.Stat(filepath.Join(here, "go.mod")); err == nil {
			return here, nil
		}
		parent := filepath.Dir(here)
		if parent == here {
			break
		}
		here = parent
	}
	return "", fmt.Errorf("cannot find go.mod above %s, so the repository root is unknown", here)
}

// Script turns the fixture into a script the fake model plays, one step per
// round. Every step after the correction expects the correction to still be in
// the request, which is how the fixture catches a harness that dropped it, and
// every round that writes to the record asks for the task tool in the same
// reply, which is how it catches a harness that never writes the record at all.
func (task FortyStepTask) Script() Script {
	steps := make([]Step, 0, len(task.Rounds))
	for _, round := range task.Rounds {
		steps = append(steps, task.stepFor(round))
	}
	return Script{Name: task.Name, ContextLength: task.ContextLength, Steps: steps}
}

// stepFor turns one round into one step of the script.
//
// A round after the correction expects the correction to still be in the
// request, and a round after the stop expects the user's reply as well, because
// the stop condition fired at round thirty and nothing may run until the user
// has answered. A harness that runs straight through the stop fails on the
// round after it.
func (task FortyStepTask) stepFor(round FortyStepRound) Step {
	step := Step{Text: round.Orient, Finish: contract.FinishToolCalls}
	if round.Number > task.CorrectionRound {
		step.Expect = append(step.Expect, task.Correction)
	}
	if round.Number > task.StopRound {
		step.Expect = append(step.Expect, task.UserReplyAfterStop)
	}
	if round.ToolName == "" {
		step.Text = round.Reply
		step.Finish = contract.FinishEnd
		return step
	}
	input, err := json.Marshal(round.ToolInput)
	if err != nil {
		input = []byte("{}")
	}
	step.ToolCalls = []contract.ToolCall{{
		ID:    fmt.Sprintf("call_%d", round.Number),
		Name:  round.ToolName,
		Input: input,
	}}
	if round.TaskUpdate != nil {
		step.ToolCalls = append(step.ToolCalls, taskCallFor(round))
	}
	return step
}

// taskCallFor is the record write one round makes, as a call to the task tool.
// The design says the model writes the record in the same reply as its other
// tool calls, so that updating it never costs an extra call.
func taskCallFor(round FortyStepRound) contract.ToolCall {
	return contract.ToolCall{
		ID:    fmt.Sprintf("call_%d_task", round.Number),
		Name:  contract.ToolTask,
		Input: asJSON(round.TaskUpdate),
	}
}

// ToolResults are the results the fixture's tool calls produced, in order,
// numbered from r1. A record write is a tool call like any other, so it gets an
// id too, and the numbering counts both.
func (task FortyStepTask) ToolResults() []FortyStepResult {
	results := []FortyStepResult{}
	for _, round := range task.Rounds {
		if round.ToolName == "" {
			continue
		}
		results = append(results, FortyStepResult{
			ID:      contract.ResultID(len(results) + 1),
			Summary: round.ResultSummary,
			Text:    round.ResultText,
		})
		if round.TaskUpdate != nil {
			results = append(results, taskUpdateResult(round, contract.ResultID(len(results)+1)))
		}
	}
	return results
}

// taskUpdateResult is what the task tool gives back for one record write: a one
// line summary for the record, and the whole update for the log, so that a
// record write can be read back by its id just like any other result.
func taskUpdateResult(round FortyStepRound, id string) FortyStepResult {
	return FortyStepResult{
		ID:      id,
		Summary: "updated the record: " + strings.Join(slices.Sorted(maps.Keys(round.TaskUpdate)), ", "),
		Text:    string(asJSON(round.TaskUpdate)),
	}
}

// asJSON writes a tool call's arguments, falling back to an empty object,
// because a fixture that cannot be written as JSON is a fixture nobody can use
// and the loader has already said so.
func asJSON(value any) json.RawMessage {
	written, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return written
}

// PlanAtTheEnd is the plan the model wrote through the task tool, every step
// ticked because the task finished. The fixture does not say which result
// finished which step, because that is the harness's own bookkeeping.
func (task FortyStepTask) PlanAtTheEnd() []contract.PlanStep {
	steps := []contract.PlanStep{}
	for _, round := range task.Rounds {
		for _, text := range textsIn(round.TaskUpdate["plan"]) {
			steps = append(steps, contract.PlanStep{Number: len(steps) + 1, Text: text, Done: true})
		}
	}
	return steps
}

// DecisionsAtTheEnd is every choice the model wrote through the task tool, with
// its reason, in the order the rounds made them.
func (task FortyStepTask) DecisionsAtTheEnd() []contract.Decision {
	decisions := []contract.Decision{}
	for _, round := range task.Rounds {
		written, found := round.TaskUpdate["decision"].(map[string]any)
		if !found {
			continue
		}
		decisions = append(decisions, contract.Decision{
			ID:     contract.DecisionID(len(decisions) + 1),
			Text:   textOf(written["text"]),
			Reason: textOf(written["reason"]),
		})
	}
	return decisions
}

// FailuresAtTheEnd is everything that went wrong, with its cause, in the order
// the rounds hit them.
func (task FortyStepTask) FailuresAtTheEnd() []contract.Failure {
	failures := []contract.Failure{}
	for _, round := range task.Rounds {
		written, found := round.TaskUpdate["failure"].(map[string]any)
		if !found {
			continue
		}
		failures = append(failures, contract.Failure{
			ID:    contract.FailureID(len(failures) + 1),
			Text:  textOf(written["text"]),
			Cause: textOf(written["cause"]),
		})
	}
	return failures
}

// textsIn reads a list of lines out of a piece of a record write, which arrives
// from the fixture's JSON as a list of unknown values.
func textsIn(value any) []string {
	items, isList := value.([]any)
	if !isList {
		return nil
	}
	texts := []string{}
	for _, item := range items {
		texts = append(texts, textOf(item))
	}
	return texts
}

// textOf reads one line out of a piece of a record write, and is empty when the
// fixture holds something other than text there.
func textOf(value any) string {
	text, isText := value.(string)
	if !isText {
		return ""
	}
	return text
}
