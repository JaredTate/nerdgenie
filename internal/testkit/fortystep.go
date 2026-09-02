package testkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
// the request, which is how the fixture catches a harness that dropped it.
func (task FortyStepTask) Script() Script {
	steps := make([]Step, 0, len(task.Rounds))
	for _, round := range task.Rounds {
		steps = append(steps, task.stepFor(round))
	}
	return Script{Name: task.Name, ContextLength: task.ContextLength, Steps: steps}
}

// stepFor turns one round into one step of the script.
func (task FortyStepTask) stepFor(round FortyStepRound) Step {
	step := Step{Text: round.Orient, Finish: contract.FinishToolCalls}
	if round.Number > task.CorrectionRound {
		step.Expect = []string{task.Correction}
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
	return step
}

// ToolResults are the results the fixture's tools produced, in order, numbered
// from r1.
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
	}
	return results
}
