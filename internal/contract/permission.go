package contract

import (
	"context"
	"encoding/json"
)

// PermissionRuling is the permission function's answer about one tool call.
type PermissionRuling string

const (
	// RulingAllow runs the call without asking.
	RulingAllow PermissionRuling = "allow"
	// RulingAsk shows the user a preview and waits.
	RulingAsk PermissionRuling = "ask"
	// RulingDeny refuses the call and tells the model why.
	RulingDeny PermissionRuling = "deny"
	// RulingStop means the call would have asked, but the run is unattended and
	// nobody is there to answer, so the task stops and reports what it needed.
	RulingStop PermissionRuling = "stop"
)

// KnownPermissionRuling says whether the ruling is one of the four.
func KnownPermissionRuling(ruling PermissionRuling) bool {
	switch ruling {
	case RulingAllow, RulingAsk, RulingDeny, RulingStop:
		return true
	default:
		return false
	}
}

// PermissionRule is one line of the user's rulebook in config.toml: a tool name
// or "*", a pattern matched against the call's reduced form, and what to do.
// The last rule that matches wins, and no match means allow.
type PermissionRule struct {
	// Tool is the tool the rule applies to, or "*" for every tool.
	Tool string `toml:"tool"`
	// Pattern is a glob matched against the call's reduced form.
	Pattern string `toml:"pattern"`
	// Action is allow, ask, or deny.
	Action PermissionRuling `toml:"action"`
}

// The three entries the ask-me-first list ships with, from design section 11.
// The user can add to this list or empty it.
const (
	// AskFirstBulkDelete covers deleting many files at once.
	AskFirstBulkDelete = "deleting many files at once, such as rm -rf"
	// AskFirstSudo covers running a command with administrator powers.
	AskFirstSudo = "running a command with sudo"
	// AskFirstSpendMoney covers anything that spends the user's money.
	AskFirstSpendMoney = "spending money"
)

// DefaultAskMeFirst returns the three entries the ask-me-first list ships with.
func DefaultAskMeFirst() []string {
	return []string{AskFirstBulkDelete, AskFirstSudo, AskFirstSpendMoney}
}

// PermissionRequest is one tool call put to the permission function.
type PermissionRequest struct {
	// ToolName is the tool the model asked for.
	ToolName string
	// Input is the arguments the model wrote.
	Input json.RawMessage
	// CommandPrefix is a shell command reduced to the readable part a rule
	// matches on, such as "git push", and is empty for other tools.
	CommandPrefix string
	// Unattended says nobody is there to answer, which is true for a scheduled
	// job. An unattended run that needs an answer stops and reports.
	Unattended bool
}

// PermissionDecision is the permission function's answer, with what to show the
// user when the answer is to ask.
type PermissionDecision struct {
	// Ruling is allow, ask, or deny.
	Ruling PermissionRuling
	// Reason says why, in plain words the model and the user can both read.
	Reason string
	// PreviewText is exactly what is about to happen, and is filled in when the
	// ruling is to ask.
	PreviewText string
}

// Permission is the harness's rulebook for what a tool call is allowed to do.
// The model decides what to do and the permission function decides what is
// allowed, and the two never share a layer.
type Permission interface {
	// Decide rules on one call.
	Decide(ctx context.Context, request PermissionRequest) (PermissionDecision, error)
	// Remember records the user's answer so that "always" does not ask again for
	// the rest of the session, and "reject" is refused with the same reason.
	Remember(request PermissionRequest, answer PreviewAnswer, reason string) error
}
