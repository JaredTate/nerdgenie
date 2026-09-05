package loop

import (
	"path"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxProbesBetweenEdits is how many throwaway scripts a task may run in a row
// with no write or edit between them before the harness says so. A small
// model reasons out loud by writing a little script, running it, and writing
// the next: the live game build wrote twenty in a row, each a character
// different so the same-call guard never fired, and changed nothing for
// thirty rounds. Five is enough to learn something from and too few to be
// lost in.
const MaxProbesBetweenEdits = 5

// TheProbeLine is the one line the model is sent, in the harness's own words
// and not inside a tool result, when it has run MaxProbesBetweenEdits probes
// since it last changed a file.
const TheProbeLine = "That is five probe scripts since the last edit. Write what they showed into the record as a failure with its cause, then change the code; one more probe will not tell you anything the last five did not."

// theShapesOfAProbe are what a throwaway script looks like on the command
// line: a small program written and run in one breath, or a file written into
// a temporary place to be run once. Running the tests, building, listing a
// folder or reading a file are not probes, however many come in a row.
var theShapesOfAProbe = []string{"node -e ", "node --eval ", "python3 -c ", "python -c ", "deno eval "}

// theNamesOfAProbe are the names a model gives a script it means to throw
// away, matched on the start of the file's name.
var theNamesOfAProbe = []string{"dbg", "debug", "trace", "probe", "scratch", "tmp", "check", "repro"}

// looksLikeAProbe says whether a shell call runs a throwaway script: one of
// the eval shapes, or a file written with a heredoc whose name says it is
// temporary or which lives under /tmp.
func looksLikeAProbe(call contract.ToolCall) bool {
	if call.Name != contract.ToolShell {
		return false
	}
	command := fieldOfCall(call, "command")
	for _, shape := range theShapesOfAProbe {
		if strings.Contains(command, shape) {
			return true
		}
	}
	at := strings.Index(command, "cat > ")
	if at < 0 || !strings.Contains(command[at:], "<<") {
		return false
	}
	target := strings.Fields(command[at+len("cat > "):])
	if len(target) == 0 {
		return false
	}
	file := strings.Trim(target[0], `"'`)
	if strings.HasPrefix(file, "/tmp/") {
		return true
	}
	name := strings.ToLower(path.Base(file))
	for _, start := range theNamesOfAProbe {
		if strings.HasPrefix(name, start) {
			return true
		}
	}
	return false
}

// countTheProbe writes down one call for the probe rule: a write or an edit
// starts the count again, a probe raises it, and reaching the cap puts the
// probe line due for the end of the round and starts the count again, so the
// line is said once per run of five and not on every call after.
func (running *run) countTheProbe(call contract.ToolCall) {
	switch {
	case call.Name == contract.ToolWrite || call.Name == contract.ToolEdit:
		running.probesSinceEdit = 0
	case looksLikeAProbe(call):
		running.probesSinceEdit++
		if running.probesSinceEdit >= MaxProbesBetweenEdits {
			running.probeLineDue = true
			running.probesSinceEdit = 0
		}
	}
}

// sayTheProbeLine puts the probe line into the conversation after the round's
// results, once, when the round earned it.
func (running *run) sayTheProbeLine() {
	if !running.probeLineDue {
		return
	}
	running.probeLineDue = false
	running.remember(contract.Message{Role: contract.RoleUser, Text: TheProbeLine})
}
