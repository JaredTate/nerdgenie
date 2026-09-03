package browser

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The error codes from the table in worker/browser/PROTOCOL.md. The first four
// are JSON-RPC's own and the last four are the browser's.
const (
	// codeParseError means the worker could not read the line as JSON.
	codeParseError = -32700
	// codeInvalidRequest means the line was JSON but not a JSON-RPC request.
	codeInvalidRequest = -32600
	// codeNoSuchMethod means the worker has no such method, which is a fault on
	// this side of the pipe rather than something the model can act on.
	codeNoSuchMethod = -32601
	// codeBadParameters means the parameters were wrong.
	codeBadParameters = -32602
	// codeNoSuchReference means the element is not on the page any more, after
	// every way of finding it again failed.
	codeNoSuchReference = -32000
	// codeCouldNotBeRead means the page could not be read at all after the
	// settle limit.
	codeCouldNotBeRead = -32001
	// codeNoBrowserOpen means nothing is open, so there is nothing to act on.
	codeNoBrowserOpen = -32002
	// codeChromeDied means the browser the worker was driving has gone.
	codeChromeDied = -32003
)

// RefusedError is one refusal from the browser worker, carrying the protocol's
// code and whatever the protocol sends alongside it. A reference the page no
// longer holds comes with a fresh snapshot, so that the model is handed
// something it can point at rather than told to guess.
type RefusedError struct {
	// Code is one of the eight in the protocol's table.
	Code int `json:"code"`
	// Message says what went wrong, in the worker's own words, written for the
	// model to act on.
	Message string `json:"message"`
	// Data is what rides alongside the refusal, still unread.
	Data json.RawMessage `json:"data,omitempty"`
}

// Error is what the worker said.
func (refused *RefusedError) Error() string {
	return refused.Message
}

// Page is the fresh snapshot that rides with a reference the page no longer
// holds, and nil for every other code.
func (refused *RefusedError) Page() *contract.Snapshot {
	if refused.Code != codeNoSuchReference || len(refused.Data) == 0 {
		return nil
	}
	var page contract.Snapshot
	if err := json.Unmarshal(refused.Data, &page); err != nil {
		return nil
	}
	return &page
}

// needsRestart says whether this refusal means the worker itself is broken. The
// protocol's table decides: a line the worker could not read at all, a line that
// was not a request, and a Chrome that has gone all mean a new worker.
func (refused *RefusedError) needsRestart() bool {
	switch refused.Code {
	case codeParseError, codeInvalidRequest, codeChromeDied:
		return true
	default:
		return false
	}
}

// isBugOnThisSide says whether the refusal is a fault in the Go side rather
// than something the model can do anything about, which the protocol's table
// says of a method the worker does not have.
func (refused *RefusedError) isBugOnThisSide() bool {
	return refused.Code == codeNoSuchMethod
}

// tabsAnswer is what the tabs method returns.
type tabsAnswer struct {
	// Tabs is every tab that is open, with the one being acted on marked.
	Tabs []contract.Tab `json:"tabs"`
}

// diffsAnswer is what the act method returns: one diff per step that ran.
type diffsAnswer struct {
	// Diffs are the steps that ran, in the order they ran.
	Diffs []contract.Diff `json:"diffs"`
}

// The deadline for each method. Every one is longer than the worker's own
// deadline for the same method in worker/browser/src/limits.ts, so that a worker
// which is merely slow answers rather than being restarted underneath itself.
var methodDeadlines = map[string]time.Duration{
	"open":       60 * time.Second,
	"read":       20 * time.Second,
	"click":      35 * time.Second,
	"type":       60 * time.Second,
	"press":      35 * time.Second,
	"scroll":     35 * time.Second,
	"act":        150 * time.Second,
	"tabs":       20 * time.Second,
	"loginFill":  60 * time.Second,
	"screenshot": 30 * time.Second,
	"dialog":     20 * time.Second,
	"health":     15 * time.Second,
}

// defaultMethodDeadline is what a method the table does not name gets, so that
// no call can ever wait without a limit.
const defaultMethodDeadline = 30 * time.Second

// startupHealthDeadline is how long the first health check may take. It is the
// long one because the worker launches Chrome before it answers anything, and
// the worker gives Chrome twenty seconds to come up.
const startupHealthDeadline = 60 * time.Second

// deadlineFor is how long one method may take before the worker is restarted.
func deadlineFor(method string) time.Duration {
	if deadline, named := methodDeadlines[method]; named {
		return deadline
	}
	return defaultMethodDeadline
}

// interrupted is what the model is told when the worker had to be started
// again, which the protocol's table asks for on a Chrome that died and on a line
// the worker could not read.
func interrupted(method string, err error) error {
	return fmt.Errorf("the browser was interrupted during %s and will be started again on the next call: %w", method, err)
}
