package browserhandoff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
	"github.com/JaredTate/nerdgenie/internal/tool/loose"
)

// MaxReasonRunes is how long the reason put to the user may be. A person reading
// it on a phone will not read more than this.
const MaxReasonRunes = 500

// AskUser brings the browser window forward, puts the reason to the user, and
// waits for their reply or for the handoff timeout. The harness wires the
// current channel behind it.
type AskUser func(ctx context.Context, reason string) (string, error)

// Settings is what the handoff tool needs to do its work.
type Settings struct {
	// AskUser is how the user is reached.
	AskUser AskUser
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for.
	Intent string
	// Reason is what the user is told, in the model's own words.
	Reason string
}

// Tool is the browser handoff tool.
type Tool struct {
	settings Settings
}

// New returns the browser handoff tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserHandoff,
		Description: "Brings the browser window forward and gives it to the user, then waits for what they say back. " +
			"Use it for a login wall, a code, or a captcha, and never work round one.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "reason", Type: "string", Description: "What to tell the user they need to do.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassIrreversible},
	}
}

// Run puts the reason to the user and hands their reply back.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.AskUser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no way to reach the user, so wire a channel in before handing the browser over")
	}

	said, err := tool.settings.AskUser(ctx, asked.Reason)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the browser was given to the user and nothing came back: %w", err)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("the user took the browser and said: %s\n", oneLine(said))}, nil
}

// reasonNames are the names a model writes for what the user is told.
var reasonNames = []string{"reason", "message", "ask", "to_the_user"}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent and a reason")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	reason, wroteReason := fields.Text(reasonNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if !wroteReason {
		return input{}, fields.Missing("reason", "what to tell the user they need to do in the browser,")
	}
	if err := checkReason(reason); err != nil {
		return input{}, err
	}
	return input{Intent: intent, Reason: reason}, nil
}

// checkReason holds the rule that a handoff tells the user what is needed.
func checkReason(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("this call tells the user nothing, so write in one line what they need to do in the browser")
	}
	if len([]rune(reason)) > MaxReasonRunes {
		return fmt.Errorf("the reason is %d characters and the cap is %d, so say it in a line or two",
			len([]rune(reason)), MaxReasonRunes)
	}
	return nil
}

// oneLine puts what the user said on a single line, because the result keeps one.
func oneLine(said string) string {
	return strings.Join(strings.Fields(said), " ")
}
