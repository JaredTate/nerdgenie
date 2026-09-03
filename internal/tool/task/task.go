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

// input is what the model writes when it calls this tool.
type input struct {
	// Operation says which of the seven this call is, in this tool's own
	// spelling, and is empty for a call that writes the goal's sections.
	Operation string
	// Why is the one line on why the user wants this.
	Why string
	// DoneWhen is the whole done list.
	DoneWhen []contract.DoneLine
	// StopWhen is the whole stop list.
	StopWhen []string
	// Plan is the whole plan, one line per step.
	Plan []string
	// Text is the choice or the thing that went wrong.
	Text string
	// Reason is why a choice was made.
	Reason string
	// Cause is why something went wrong.
	Cause string
	// Line is which done line to pin a result to, counting from one.
	Line int
	// Result is the result to pin, such as r7.
	Result string
	// Decision is the choice this call adds, with the reason it must carry.
	Decision *writtenPair
	// Failure is what went wrong, with the cause it must carry.
	Failure *writtenPair
}

// operationSections is the call that writes the goal's own sections — the why,
// the done list, the stop list and the plan — which is what a call that names
// no operation at all usually is.
const operationSections = ""

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
			{Name: "why", Type: "string", Description: "The one line on why the user wants this, written once (the text field is taken for it too)."},
			{Name: "done_when", Type: "array", Description: "The whole done list: each line as a string, or as an object with text, done, and the result that proves it."},
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
	case OperationDecision:
		return record.Update{Decision: &record.NewDecision{Text: asked.Decision.Text, Reason: asked.Decision.Reason}}, nil
	case OperationFailure:
		return record.Update{Failure: &record.NewFailure{Text: asked.Failure.Text, Cause: asked.Failure.Cause}}, nil
	case OperationPinResult:
		return tool.pinResult(asked)
	default:
		return sectionsUpdate(asked), nil
	}
}

// pinResult marks one done line satisfied and points it at the result that
// proves it, by writing the whole done list back with that one line changed.
func (tool *Tool) pinResult(asked input) (record.Update, error) {
	lines := tool.settings.Records.Record().Goal.DoneWhen
	if asked.Line < 1 || int(asked.Line) > len(lines) {
		return record.Update{}, fmt.Errorf("there is no done line numbered %d, and the done list has %d lines in it",
			asked.Line, len(lines))
	}
	lines[asked.Line-1].Done = true
	lines[asked.Line-1].ResultID = asked.Result
	lines[asked.Line-1].UserReply = ""
	return record.Update{DoneWhen: lines}, nil
}

// sectionsUpdate turns every section the call carries into one update, so a
// model that writes the why, the done list and the plan together is taken at
// its word rather than told to make three calls. A decision or a failure
// written in the same call rides along, because the forty-step fixture writes
// both that way and dropping one would lose what the model had learned.
func sectionsUpdate(asked input) record.Update {
	update := record.Update{
		Why:      strings.TrimSpace(asked.Why),
		DoneWhen: asked.DoneWhen,
		StopWhen: asked.StopWhen,
		Plan:     asked.Plan,
	}
	if asked.Decision != nil {
		update.Decision = &record.NewDecision{Text: asked.Decision.Text, Reason: asked.Decision.Reason}
	}
	if asked.Failure != nil {
		update.Failure = &record.NewFailure{Text: asked.Failure.Text, Cause: asked.Failure.Cause}
	}
	return update
}
