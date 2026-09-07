package shell

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
)

// RunsRemembered is how many past runs the tool keeps the output hash of, so
// that the same command with the same output can be answered short. Twenty
// covers the health probes and listings a task repeats without holding a
// whole session.
const RunsRemembered = 20

// pastRun is one finished run the tool remembers: the command as written and
// a hash of what it wrote and quit with.
type pastRun struct {
	command string
	hash    string
}

// pastRuns is the bounded memory of finished runs, newest last.
type pastRuns struct {
	guard sync.Mutex
	runs  []pastRun
}

// answerRun is what a finished run says: the whole result the first time, and
// "same as the last run of this command, unchanged" when the same command
// wrote the same thing and quit clean the time before, with the id to tail for
// the output. The command still runs, so the answer is honest; only the tokens
// go. 84 rounds in one day's logs ran a command identical to an earlier one
// and read the same output again, one health probe 26 times.
func (tool *Tool) answerRun(command string, entry *entry) string {
	text := entry.finishedText()
	entry.guard.Lock()
	clean := entry.err == nil && !entry.result.TimedOut && entry.result.ExitCode == 0
	lines := countLines(entry.result.StandardOutput) + countLines(entry.result.StandardError)
	entry.guard.Unlock()
	if !clean {
		tool.past.forget(command)
		return text
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	if tool.past.remember(command, hash) {
		return fmt.Sprintf("same as the last run of this command, unchanged: exit 0, %d lines; tail %s to read it again\n", lines, entry.id)
	}
	return text
}

// remember puts the run in the memory, newest last, and says whether the same
// command wrote the same thing the time before. The memory is bounded; the
// oldest run leaves when it is full.
func (past *pastRuns) remember(command string, hash string) bool {
	past.guard.Lock()
	defer past.guard.Unlock()
	same := false
	for at, run := range past.runs {
		if run.command == command {
			same = run.hash == hash
			past.runs = append(past.runs[:at], past.runs[at+1:]...)
			break
		}
	}
	past.runs = append(past.runs, pastRun{command: command, hash: hash})
	if len(past.runs) > RunsRemembered {
		past.runs = past.runs[len(past.runs)-RunsRemembered:]
	}
	return same
}

// forget drops the memory of a command, so that a run after a failure is
// always shown in full.
func (past *pastRuns) forget(command string) {
	past.guard.Lock()
	defer past.guard.Unlock()
	for at, run := range past.runs {
		if run.command == command {
			past.runs = append(past.runs[:at], past.runs[at+1:]...)
			return
		}
	}
}

// countLines counts the lines a stream holds.
func countLines(stream []byte) int {
	if len(stream) == 0 {
		return 0
	}
	text := strings.TrimSuffix(string(stream), "\n")
	return strings.Count(text, "\n") + 1
}
