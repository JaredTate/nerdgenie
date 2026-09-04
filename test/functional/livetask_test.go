//go:build live

// The whole-program live test, one per real model: a real "nerdgenie serve" is
// asked for one small piece of work that cannot be done without a tool, and
// afterwards the test checks the things the model cannot author. The file is on
// disk with the words in it, the agent's own log holds a record of the task, the
// done-check on that record passes, and the reply came back inside the bound.
package functional

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// theLiveAsk is the one small task every model is given. It needs two tools, a
// write and a read, and its answer is a fact on disk rather than anything the
// model can talk its way into.
const theLiveAsk = "write the words hello nerdgenie to a file called greeting.txt in the working folder, " +
	"then read it back and tell me what it says"

// theWordsWanted are the words that have to be in the file afterwards.
const theWordsWanted = "hello nerdgenie"

// aCommandIsAnsweredWithin is how long a slash command over the socket may take.
// A command reads the log and answers; it never calls a model.
const aCommandIsAnsweredWithin = 60 * time.Second

func TestTheLocalModelWritesAFileAndReadsItBackThroughTheRealServe(t *testing.T) {
	oneSmallTaskOnARealModel(t, theLocalModel())
}

func TestOpusWritesAFileAndReadsItBackThroughTheRealServe(t *testing.T) {
	oneSmallTaskOnARealModel(t, theClaudeModel())
}

func TestGPTWritesAFileAndReadsItBackThroughTheRealServe(t *testing.T) {
	oneSmallTaskOnARealModel(t, theCodexModel())
}

// oneSmallTaskOnARealModel is the body every one of the three tests runs, with
// the model as its only difference.
func oneSmallTaskOnARealModel(t *testing.T, model liveModel) {
	t.Helper()
	agent := startTheLiveAgent(t, model)
	screen := agent.attach(t)

	// The folder is named in the message because nothing in the prompt tells the
	// model where it may write, and a model guessing at a path would be a test of
	// the sandbox rather than of the work.
	asked := theLiveAsk + ". The working folder is " + agent.work + "."
	run := agent.sendAndWaitForTheReply(t, screen, asked, model.bound)
	run.report(t, model, "the one-tool task")
	t.Logf("%s answered: %s", model.alias, run.reply.Text)

	if strings.TrimSpace(run.reply.Text) == "" {
		t.Errorf("the model %s finished the task and said nothing at all", model.alias)
	}
	checkTheGreetingIsOnDisk(t, model, filepath.Join(agent.work, "greeting.txt"))
	checkTheRecordOfTheTaskIsSound(t, model, screen)
}

// checkTheGreetingIsOnDisk is the assertion the model cannot write its way
// around: the file it was asked for is there, and it holds the words.
func checkTheGreetingIsOnDisk(t *testing.T, model liveModel, path string) {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the model %s said it had written %s, and the file cannot be read: %v", model.alias, path, err)
	}
	if !strings.Contains(strings.ToLower(string(written)), theWordsWanted) {
		t.Errorf("the file %s holds %q, and the words asked for were %q", path, string(written), theWordsWanted)
	}
}

// checkTheRecordOfTheTaskIsSound reads the record back out of the running agent
// and holds it to the rule every task closes under: every line of the done list
// points at the result that proves it.
func checkTheRecordOfTheTaskIsSound(t *testing.T, model liveModel, screen *attachedScreen) {
	t.Helper()
	number := theTaskTheAgentRan(t, screen)
	held, printed := theRecordNumbered(t, screen, number)
	t.Logf("%s wrote this record:\n%s", model.alias, printed)

	if held.Goal.Ask == "" {
		t.Errorf("the record of task %s holds no ask, and the user's own words are the first thing in it", number)
	}
	if err := record.DoneCheck(held); err != nil {
		t.Errorf("the done-check on the record the model %s wrote does not pass: %v", model.alias, err)
	}
	if held.Header.Status != contract.StatusDone {
		t.Errorf("task %s stands at %q after the model said it was finished, want %q",
			number, held.Header.Status, contract.StatusDone)
	}
}

// theTaskTheAgentRan is the number of the task the agent made for this message,
// read off the listing the tasks command prints.
func theTaskTheAgentRan(t *testing.T, screen *attachedScreen) string {
	t.Helper()
	listing := theCommandReply(t, screen, "tasks")
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "task" {
			return fields[1]
		}
	}
	t.Fatalf("the agent lists no task at all, so the model never made a record:\n%s", listing)
	return ""
}

// theRecordNumbered is one task's record, printed by the running agent and read
// back with the same parser the agent prints it with, which is the round trip
// that proves the record on disk is still a record. It returns the printed text
// as well, because a test whose assertion fails has to show what the model wrote.
func theRecordNumbered(t *testing.T, screen *attachedScreen, number string) (contract.Record, string) {
	t.Helper()
	printed := theCommandReply(t, screen, "tasks "+number)
	held, err := record.Parse([]byte(printed))
	if err != nil {
		t.Fatalf("the record of task %s cannot be read back: %v\n%s", number, err, printed)
	}
	return held, printed
}

// theCommandReply sends one slash command over the socket and returns what came
// back, which is how a test asks the running agent what it holds.
func theCommandReply(t *testing.T, screen *attachedScreen, command string) string {
	t.Helper()
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: command})
	return screen.waitFor(t, contract.SocketReply, aCommandIsAnsweredWithin).Text
}
