package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/record"
)

// The seven operations, and no more. Anything else in the record belongs to the
// user or to the harness.
const (
	// OperationWhy sets the one line on why the user wants this.
	OperationWhy = "why"
	// OperationDoneWhen writes the whole done list.
	OperationDoneWhen = "done_when"
	// OperationStopWhen writes the whole stop list.
	OperationStopWhen = "stop_when"
	// OperationPlan writes the whole plan.
	OperationPlan = "plan"
	// OperationDecision adds one choice with its reason.
	OperationDecision = "decision"
	// OperationFailure adds one thing that went wrong with its cause.
	OperationFailure = "failure"
	// OperationPinResult points one done line at the result that proves it.
	OperationPinResult = "pin_result"
)

// MaxLines is how many lines one list may hold, because a done list or a plan
// longer than this is a job rather than a task.
const MaxLines = 50

// Records is the record this tool writes through, which is the keeper in
// internal/record. Only the two methods here are used, because every other way
// into a record belongs to the harness.
type Records interface {
	// Record returns a copy of the record as it stands.
	Record() contract.Record
	// Apply writes the model's half of the record, all or nothing.
	Apply(ctx context.Context, update record.Update) error
}

// Settings is what the task tool needs to do its work.
type Settings struct {
	// Records is the record of the task running now.
	Records Records
}

// writtenDoneLine is one line of the done list as the model writes it.
type writtenDoneLine struct {
	// Text is the line itself.
	Text string `json:"text"`
	// Done says the model believes the line is satisfied.
	Done bool `json:"done"`
	// Result is the result that proves it, such as r7.
	Result string `json:"result"`
	// UserReply is the user's own words standing in for a result.
	UserReply string `json:"user_reply"`
}

// input is what the model writes when it calls this tool.
type input struct {
	// Operation says which of the seven this call is.
	Operation string `json:"operation"`
	// Why is the one line on why the user wants this.
	Why string `json:"why"`
	// DoneWhen is the whole done list.
	DoneWhen []writtenDoneLine `json:"done_when"`
	// StopWhen is the whole stop list.
	StopWhen []string `json:"stop_when"`
	// Plan is the whole plan, one line per step.
	Plan []string `json:"plan"`
	// Text is the choice or the thing that went wrong.
	Text string `json:"text"`
	// Reason is why a choice was made.
	Reason string `json:"reason"`
	// Cause is why something went wrong.
	Cause string `json:"cause"`
	// Line is which done line to pin a result to, counting from one.
	Line int `json:"line"`
	// Result is the result to pin, such as r7.
	Result string `json:"result"`
}

// Tool is the task tool.
type Tool struct {
	settings Settings
}

// New returns the task tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolTask,
		Description: "Writes the task record: the why, the done list, the stop list, the plan, a decision with its reason, " +
			"a failure with its cause, or a result pinned to the done line it proves.",
		Fields: []contract.ToolField{
			{Name: "operation", Type: "string", Description: "One of why, done_when, stop_when, plan, decision, failure, pin_result.", Required: true},
			{Name: "why", Type: "string", Description: "The one line on why the user wants this, written once."},
			{Name: "done_when", Type: "array", Description: "The whole done list, each line with the result that proves it."},
			{Name: "stop_when", Type: "array", Description: "The whole stop list, one line each."},
			{Name: "plan", Type: "array", Description: "The whole plan, one line per step, in order."},
			{Name: "text", Type: "string", Description: "The choice, or the thing that went wrong."},
			{Name: "reason", Type: "string", Description: "Why the choice was made."},
			{Name: "cause", Type: "string", Description: "Why the thing went wrong."},
			{Name: "line", Type: "integer", Description: "Which done line to pin a result to, counting from one."},
			{Name: "result", Type: "string", Description: "The result to pin, such as r7."},
		},
		Classes: []contract.PermissionClass{contract.ClassWrite},
	}
}

// Run writes one change into the record, through the record's own rules.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Records == nil {
		return contract.ToolOutput{}, errors.New("this tool has no record behind it, so wire the record in before using it")
	}

	update, err := tool.updateFor(asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if err := tool.settings.Records.Apply(ctx, update); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the record refused this change: %w", err)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("the record's %s is written\n", said(asked.Operation))}, nil
}

// said is how one operation reads in the line the model gets back.
func said(operation string) string {
	return strings.ReplaceAll(operation, "_", " ")
}

// updateFor turns one call into the update the record takes, which is the only
// shape the model can write a record in.
func (tool *Tool) updateFor(asked input) (record.Update, error) {
	switch asked.Operation {
	case OperationWhy:
		return record.Update{Why: asked.Why}, nil
	case OperationDoneWhen:
		return record.Update{DoneWhen: doneLines(asked.DoneWhen)}, nil
	case OperationStopWhen:
		return record.Update{StopWhen: asked.StopWhen}, nil
	case OperationPlan:
		return record.Update{Plan: asked.Plan}, nil
	case OperationDecision:
		return record.Update{Decision: &record.NewDecision{Text: asked.Text, Reason: asked.Reason}}, nil
	case OperationFailure:
		return record.Update{Failure: &record.NewFailure{Text: asked.Text, Cause: asked.Cause}}, nil
	default:
		return tool.pinResult(asked)
	}
}

// pinResult marks one done line satisfied and points it at the result that
// proves it, by writing the whole done list back with that one line changed.
func (tool *Tool) pinResult(asked input) (record.Update, error) {
	lines := tool.settings.Records.Record().Goal.DoneWhen
	if asked.Line < 1 || asked.Line > len(lines) {
		return record.Update{}, fmt.Errorf("there is no done line numbered %d, and the done list has %d lines in it",
			asked.Line, len(lines))
	}
	lines[asked.Line-1].Done = true
	lines[asked.Line-1].ResultID = asked.Result
	lines[asked.Line-1].UserReply = ""
	return record.Update{DoneWhen: lines}, nil
}

// doneLines turns the done list the model wrote into the record's own.
func doneLines(written []writtenDoneLine) []contract.DoneLine {
	lines := make([]contract.DoneLine, 0, len(written))
	for _, line := range written {
		lines = append(lines, contract.DoneLine{
			Text: line.Text, Done: line.Done, ResultID: line.Result, UserReply: line.UserReply,
		})
	}
	return lines
}
