package memory

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// toolResultBody is what a tool-result event holds, in the shape the record's
// StoredResult writes it. Only the fields capture reads are named.
type toolResultBody struct {
	// Summary is the one line the record kept, which names a refusal.
	Summary string `json:"summary"`
	// Text is the whole of what the tool returned, which names an exit code.
	Text string `json:"text"`
}

// permissionDecisionBody is what a permission-decision event holds, in the
// shape the loop writes it. Only the ruling is read.
type permissionDecisionBody struct {
	// Ruling is allow, ask, deny, or stop.
	Ruling contract.PermissionRuling `json:"ruling"`
}

// theRefusedMark is what the loop puts between a tool's name and the reason in
// the summary of a result it refused.
const theRefusedMark = " was refused: "

// theExitCodeOpening is how the shell tool begins the first line of a finished
// command's result.
const theExitCodeOpening = "finished with exit code "

// callOutcome is what a call's result or ruling says happened to it.
type callOutcome int

const (
	// callRan means the tool ran the call and reported no failure.
	callRan callOutcome = iota
	// callFailed means the tool ran the call and it failed, as a command with
	// an exit code other than zero did.
	callFailed
	// callRefused means the call never ran: the permission function denied it,
	// or the tool or the record refused it.
	callRefused
)

// capturing walks a task's events in order and keeps the facts worth writing
// down. A tool call is held until its outcome arrives, because what the model
// asked for is not what happened: the result says whether the call ran, failed
// or was refused, and a call with no outcome at all, such as the one a turn
// ended on, did nothing worth remembering.
type capturing struct {
	taskID string
	facts  []contract.Fact
	// held is the tool call waiting for its outcome, while holding is true.
	held    contract.Event
	holding bool
}

// add reads one page of the event log, passing over the events of other tasks,
// and stops at the cap on captured facts. A new call lets go of one still
// waiting, which is how a call the turn ended on goes unwritten.
func (walk *capturing) add(events []contract.Event) {
	for _, event := range events {
		if len(walk.facts) >= maxCapturedFacts {
			return
		}
		if event.TaskID != walk.taskID {
			continue
		}
		switch event.Kind {
		case contract.EventToolCall:
			walk.held, walk.holding = event, true
		case contract.EventToolResult:
			walk.settle(event)
		case contract.EventPermissionDecision:
			if decisionRefuses(event) {
				walk.settle(event)
			}
		default:
			if text, aboutTheUser, worthKeeping := capturedFrom(event); worthKeeping {
				walk.keep(event, text, aboutTheUser)
			}
		}
	}
}

// settle writes down the held call with the outcome its result or its ruling
// gave it, and lets go of it. A result with no call held, such as the reply the
// harness writes as a result of its own, is passed over.
func (walk *capturing) settle(outcome contract.Event) {
	if !walk.holding {
		return
	}
	walk.holding = false
	call := toolCallBody{}
	if err := json.Unmarshal(walk.held.Body, &call); err != nil {
		return
	}
	what := callRefused
	if outcome.Kind == contract.EventToolResult {
		read, readable := outcomeOfResult(call, outcome)
		if !readable {
			return
		}
		what = read
	}
	if text := whatTheCallDid(call, what); text != "" {
		walk.keep(walk.held, text, false)
	}
}

// keep adds one fact, with its id and its time taken from the event it is about.
func (walk *capturing) keep(event contract.Event, text string, aboutTheUser bool) {
	walk.facts = append(walk.facts, contract.Fact{
		ID:       capturedFactID(walk.taskID, event.Sequence, aboutTheUser),
		Text:     cutToBytes(text, maxFactTextBytes, "the whole of it is in the event log"),
		Source:   "task " + walk.taskID,
		Recorded: event.Occurred,
	})
}

// outcomeOfResult reads a result the way the loop wrote it: a summary that
// begins with the tool's name and "was refused" is a refusal, and a command
// whose first line reports an exit code other than zero failed. A result that
// cannot be read says nothing about the call.
func outcomeOfResult(call toolCallBody, event contract.Event) (callOutcome, bool) {
	result := toolResultBody{}
	if err := json.Unmarshal(event.Body, &result); err != nil {
		return callRan, false
	}
	if strings.HasPrefix(result.Summary, call.Name+theRefusedMark) {
		return callRefused, true
	}
	if call.Name == contract.ToolShell && commandFailed(result.Text) {
		return callFailed, true
	}
	return callRan, true
}

// commandFailed reads the exit code off a shell result's first line. A result
// that is not a finished command, such as a poll or a serve, did not fail, and
// neither did a pipe the shell tool itself says to treat as success.
func commandFailed(text string) bool {
	first, _, _ := strings.Cut(text, "\n")
	rest, found := strings.CutPrefix(first, theExitCodeOpening)
	if !found || strings.Contains(first, "treat this as success") {
		return false
	}
	words := strings.Fields(rest)
	if len(words) == 0 {
		return false
	}
	code, err := strconv.Atoi(words[0])
	return err == nil && code != 0
}

// decisionRefuses says whether a permission ruling kept the call from running:
// a deny, or a stop because nobody was there to answer.
func decisionRefuses(event contract.Event) bool {
	decision := permissionDecisionBody{}
	if err := json.Unmarshal(event.Body, &decision); err != nil {
		return false
	}
	return decision.Ruling == contract.RulingDeny || decision.Ruling == contract.RulingStop
}

// whatTheCallDid says in plain words what happened to one call, reading the
// argument that says what it was for, and says nothing for a call that is not
// worth writing down or that named no argument.
func whatTheCallDid(call toolCallBody, outcome callOutcome) string {
	what, opening, closing := "", "", ""
	switch call.Name {
	case contract.ToolShell:
		what, opening = call.field("command"), "ran the command "
		if outcome == callFailed {
			closing = ", which failed"
		}
		if outcome == callRefused {
			opening = "was refused the command "
		}
	case contract.ToolWeb, contract.ToolBrowserOpen:
		what, opening = call.field("url"), "visited the site "
		if outcome == callRefused {
			opening = "was not allowed to visit "
		}
	case contract.ToolJob:
		what, opening = call.field("name"), "created the job "
		if outcome == callRefused {
			opening = "was not allowed to create the job "
		}
	}
	if what == "" {
		return ""
	}
	return opening + what + closing
}
