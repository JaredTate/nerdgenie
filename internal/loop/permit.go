package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// denial is a call the permission function would not let through.
type denial struct {
	// reason is what the model is told, in words it can act on.
	reason string
	// said is the rulebook's or the user's own words on their own, for a caller
	// that has a sentence of its own to put them in.
	said string
	// stopsTheTask says nobody was there to answer, so the task stops and
	// reports what it needed rather than waiting for somebody who is not there.
	stopsTheTask bool
}

// permit puts one call to the permission function and, when the answer is to
// ask, shows the user the preview and remembers what they said. Every ruling is
// written into the log as a permission-decision event.
func (running *run) permit(ctx context.Context, call contract.ToolCall) (bool, denial, error) {
	asked := contract.PermissionRequest{
		ToolName:      call.Name,
		Input:         call.Input,
		CommandPrefix: commandPrefixOf(call),
		Unattended:    running.task.Unattended,
	}
	decision, err := running.theLoop.options.Permission.Decide(ctx, asked)
	if err != nil {
		return false, denial{}, fmt.Errorf("cannot rule on the call to %s: %w", call.Name, err)
	}
	if err := running.logRuling(ctx, call, decision); err != nil {
		return false, denial{}, err
	}
	switch decision.Ruling {
	case contract.RulingAllow:
		return true, denial{}, nil
	case contract.RulingDeny:
		return false, denial{reason: refusedBecause(call, decision.Reason), said: decision.Reason}, nil
	case contract.RulingStop:
		return false, denial{reason: refusedBecause(call, decision.Reason), said: decision.Reason, stopsTheTask: true}, nil
	default:
		return running.askTheUser(ctx, call, asked, decision)
	}
}

// askTheUser shows the preview of exactly what is about to happen and takes the
// answer, which is remembered for the rest of the session.
func (running *run) askTheUser(ctx context.Context, call contract.ToolCall,
	asked contract.PermissionRequest, decision contract.PermissionDecision) (bool, denial, error) {
	answer, err := running.channel.ShowPreview(ctx, contract.Preview{
		ID:    call.ID,
		Title: "About to run " + call.Name + ": " + decision.Reason,
		Body:  previewBody(decision, call),
	})
	if err != nil {
		return false, denial{}, fmt.Errorf("cannot show the user what %s is about to do: %w", call.Name, err)
	}
	refusal := refusalReason(answer)
	if err := running.theLoop.options.Permission.Remember(asked, answer.Answer, refusal); err != nil {
		return false, denial{}, fmt.Errorf("cannot remember what the user answered about %s: %w", call.Name, err)
	}
	if answer.Answer == contract.AnswerReject {
		return false, denial{reason: refusedBecause(call, refusal), said: refusal}, nil
	}
	return true, denial{}, nil
}

// refusalReason is the user's own words about why they said no, and a plain
// line saying they did when they gave no words at all.
func refusalReason(answer contract.PreviewAnswerWithReason) string {
	if said := strings.TrimSpace(answer.Reason); said != "" {
		return said
	}
	return "the user said no to this call"
}

// previewBody is exactly what is about to happen, which is what the user reads
// before they answer.
func previewBody(decision contract.PermissionDecision, call contract.ToolCall) string {
	if decision.PreviewText != "" {
		return decision.PreviewText
	}
	return call.Name + " " + string(call.Input)
}

// refusedBecause is what the model is told about a call that was not allowed.
func refusedBecause(call contract.ToolCall, reason string) string {
	return fmt.Sprintf("The call to %s was not allowed: %s", call.Name, reason)
}

// logRuling writes one ruling into the event log, because every permission
// decision is on the record.
func (running *run) logRuling(ctx context.Context, call contract.ToolCall, decision contract.PermissionDecision) error {
	return running.theLoop.logEvent(ctx, running.taskID(), contract.EventPermissionDecision, struct {
		Tool    string                    `json:"tool"`
		Call    string                    `json:"call"`
		Ruling  contract.PermissionRuling `json:"ruling"`
		Reason  string                    `json:"reason"`
		Preview string                    `json:"preview,omitempty"`
	}{
		Tool: call.Name, Call: call.ID, Ruling: decision.Ruling,
		Reason: decision.Reason, Preview: decision.PreviewText,
	})
}

// commandPrefixOf is the readable start of a shell command, which is what the
// user's answer of "always" is remembered against. The permission function
// reduces the call properly for its own rules; this is the loop's own short
// form of the same thing.
func commandPrefixOf(call contract.ToolCall) string {
	if call.Name != contract.ToolShell {
		return ""
	}
	var written struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(call.Input, &written); err != nil {
		return ""
	}
	words := strings.Fields(written.Command)
	if len(words) > 2 {
		words = words[:2]
	}
	return strings.Join(words, " ")
}
