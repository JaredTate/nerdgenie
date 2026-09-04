package main

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// anAgentOnASocket is a stand-in for the running agent: a Unix socket that reads
// the envelopes a screen sends and answers with the ones a test scripts. It is
// here rather than in internal/testkit because it is the only thing that needs
// it, and the whole of it is thirty lines.
type anAgentOnASocket struct {
	path     string
	listener net.Listener

	guard    sync.Mutex
	received []contract.SocketEnvelope
	answers  func(sent contract.SocketEnvelope) []contract.SocketEnvelope
}

// aFakeAgent starts the socket and answers each message with what the function
// gives back. It is closed when the test ends.
func aFakeAgent(t *testing.T, answers func(sent contract.SocketEnvelope) []contract.SocketEnvelope) *anAgentOnASocket {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("opening the fake agent's socket failed: %v", err)
	}
	agent := &anAgentOnASocket{path: path, listener: listener, answers: answers}
	t.Cleanup(func() { _ = listener.Close() })
	go agent.serve()
	return agent
}

// serve answers every screen that attaches, one at a time, which is all a test
// ever asks of it.
func (agent *anAgentOnASocket) serve() {
	for {
		connection, err := agent.listener.Accept()
		if err != nil {
			return
		}
		go agent.talk(connection)
	}
}

// talk reads the envelopes one screen sends and writes back what the test said.
func (agent *anAgentOnASocket) talk(connection net.Conn) {
	defer func() { _ = connection.Close() }()
	lines := bufio.NewScanner(connection)
	for lines.Scan() {
		sent, err := contract.DecodeSocketEnvelope(lines.Bytes())
		if err != nil {
			return
		}
		agent.guard.Lock()
		agent.received = append(agent.received, sent)
		agent.guard.Unlock()
		for _, answer := range agent.answers(sent) {
			if err := contract.EncodeSocketEnvelope(connection, answer); err != nil {
				return
			}
		}
	}
}

// sent is every envelope the screen sent, in order.
func (agent *anAgentOnASocket) sent() []contract.SocketEnvelope {
	agent.guard.Lock()
	defer agent.guard.Unlock()
	return append([]contract.SocketEnvelope(nil), agent.received...)
}

// answerTheMessageWith replies to a message with the envelopes given, and says
// nothing to anything else.
func answerTheMessageWith(answers ...contract.SocketEnvelope) func(contract.SocketEnvelope) []contract.SocketEnvelope {
	return func(sent contract.SocketEnvelope) []contract.SocketEnvelope {
		if sent.Type != contract.SocketMessage {
			return nil
		}
		return answers
	}
}

// runTheRunSubcommand runs "nerdgenie run" against the fake agent and gives back
// what it printed on each output and the code it left with.
func runTheRunSubcommand(t *testing.T, agent *anAgentOnASocket, arguments ...string) (string, string, int) {
	t.Helper()
	t.Setenv(runSocketVariable, agent.path)
	var answered, said bytes.Buffer
	code := runSubcommand.run(arguments, &answered, &said)
	return answered.String(), said.String(), code
}

func TestRunSendsOnePromptAndPrintsTheReply(t *testing.T) {
	agent := aFakeAgent(t, answerTheMessageWith(
		contract.SocketEnvelope{Type: contract.SocketDelta, Text: "DigiByte launched "},
		contract.SocketEnvelope{Type: contract.SocketDelta, Text: "in 2014."},
		contract.SocketEnvelope{Type: contract.SocketReply, Text: "DigiByte launched in 2014."},
	))

	answered, _, code := runTheRunSubcommand(t, agent, "when did DigiByte launch?")

	if code != contract.ExitOK {
		t.Errorf("nerdgenie run left with %d, want %d", code, contract.ExitOK)
	}
	if !strings.Contains(answered, "DigiByte launched in 2014.") {
		t.Errorf("the reply printed as %q, want the one the agent sent", answered)
	}
	asked := agent.sent()
	if len(asked) < 2 || asked[0].Type != contract.SocketAttach || asked[1].Type != contract.SocketMessage {
		t.Fatalf("nerdgenie run sent %v, want an attach and then one message", asked)
	}
	if asked[1].Text != "when did DigiByte launch?" {
		t.Errorf("the message sent was %q, want the prompt that was typed", asked[1].Text)
	}
}

func TestRunTakesTheWholePromptAsOneMessage(t *testing.T) {
	agent := aFakeAgent(t, answerTheMessageWith(contract.SocketEnvelope{Type: contract.SocketReply, Text: "done"}))

	if _, _, code := runTheRunSubcommand(t, agent, "count", "the", "jars"); code != contract.ExitOK {
		t.Fatalf("nerdgenie run left with %d", code)
	}

	asked := agent.sent()
	if asked[1].Text != "count the jars" {
		t.Errorf("the message sent was %q, want the words joined into one prompt", asked[1].Text)
	}
}

func TestRunPrintsToolLinesAndRecordLinesOnTheErrorOutput(t *testing.T) {
	agent := aFakeAgent(t, answerTheMessageWith(
		contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
			contract.StatusFieldRecordLine: "task 3 started · count the jars",
			contract.StatusFieldToolLine:   "read /etc/hosts",
		}},
		contract.SocketEnvelope{Type: contract.SocketReply, Text: "there are four"},
	))

	answered, said, code := runTheRunSubcommand(t, agent, "count the jars")

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie run left with %d: %s", code, said)
	}
	if strings.Contains(answered, "task 3 started") || strings.Contains(answered, "read /etc/hosts") {
		t.Errorf("the record line or the tool line went to the ordinary output, which is the reply's own: %q", answered)
	}
	if !strings.Contains(said, "task 3 started") {
		t.Errorf("the record line was not printed on the error output: %q", said)
	}
	if !strings.Contains(said, "read /etc/hosts") {
		t.Errorf("the tool line was not printed on the error output: %q", said)
	}
}

func TestRunDeniesAPreviewUnlessYesWasGiven(t *testing.T) {
	agent := aFakeAgent(t, func(sent contract.SocketEnvelope) []contract.SocketEnvelope {
		if sent.Type == contract.SocketMessage {
			return []contract.SocketEnvelope{{
				Type: contract.SocketPreview, ID: "3", Title: "run a command", Text: "rm -rf /tmp/x",
			}}
		}
		if sent.Type == contract.SocketDeny {
			return []contract.SocketEnvelope{{Type: contract.SocketReply, Text: "I did not run it."}}
		}
		return nil
	})

	answered, said, code := runTheRunSubcommand(t, agent, "delete the folder")

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie run left with %d: %s", code, said)
	}
	if !strings.Contains(answered, "I did not run it.") {
		t.Errorf("the reply printed as %q", answered)
	}
	if !strings.Contains(said, "rm -rf /tmp/x") {
		t.Errorf("the preview was not printed before it was answered: %q", said)
	}
	denied := false
	for _, one := range agent.sent() {
		if one.Type == contract.SocketDeny && one.ID == "3" {
			denied = true
		}
	}
	if !denied {
		t.Errorf("the preview was not denied, and nothing may run without a yes: %v", agent.sent())
	}
}

func TestRunApprovesAPreviewWhenYesWasGiven(t *testing.T) {
	agent := aFakeAgent(t, func(sent contract.SocketEnvelope) []contract.SocketEnvelope {
		if sent.Type == contract.SocketMessage {
			return []contract.SocketEnvelope{{Type: contract.SocketPreview, ID: "3", Text: "rm -rf /tmp/x"}}
		}
		if sent.Type == contract.SocketApprove {
			return []contract.SocketEnvelope{{Type: contract.SocketReply, Text: "I ran it."}}
		}
		return nil
	})

	answered, said, code := runTheRunSubcommand(t, agent, "--yes", "delete the folder")

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie run left with %d: %s", code, said)
	}
	if !strings.Contains(answered, "I ran it.") {
		t.Errorf("the reply printed as %q", answered)
	}
	approved := false
	for _, one := range agent.sent() {
		if one.Type == contract.SocketApprove && one.ID == "3" {
			approved = true
		}
	}
	if !approved {
		t.Errorf("the preview was not approved even though --yes was given: %v", agent.sent())
	}
}

func TestRunLeavesWithAFailureWhenTheAgentSendsAnError(t *testing.T) {
	agent := aFakeAgent(t, answerTheMessageWith(
		contract.SocketEnvelope{Type: contract.SocketError, Text: "the model could not be reached"},
	))

	_, said, code := runTheRunSubcommand(t, agent, "anything at all")

	if code != contract.ExitFailure {
		t.Errorf("nerdgenie run left with %d on an error, want %d", code, contract.ExitFailure)
	}
	if !strings.Contains(said, "the model could not be reached") {
		t.Errorf("the error was not printed: %q", said)
	}
}

func TestRunGivesUpAfterItsTimeoutAndSaysSo(t *testing.T) {
	agent := aFakeAgent(t, func(contract.SocketEnvelope) []contract.SocketEnvelope { return nil })

	_, said, code := runTheRunSubcommand(t, agent, "--timeout", "150ms", "say nothing")

	if code != contract.ExitFailure {
		t.Errorf("nerdgenie run left with %d after its timeout, want %d", code, contract.ExitFailure)
	}
	if !strings.Contains(said, "150ms") {
		t.Errorf("the message does not say how long it waited: %q", said)
	}
}

func TestRunSaysWhenThereIsNoAgentToTalkTo(t *testing.T) {
	t.Setenv(runSocketVariable, filepath.Join(t.TempDir(), "nothing-here.sock"))
	var answered, said bytes.Buffer

	code := runSubcommand.run([]string{"hello"}, &answered, &said)

	if code != contract.ExitFailure {
		t.Errorf("nerdgenie run left with %d when nothing was listening, want %d", code, contract.ExitFailure)
	}
	if !strings.Contains(said.String(), "nerdgenie serve") {
		t.Errorf("the message does not say how to start the agent: %q", said.String())
	}
}

func TestRunNeedsAPrompt(t *testing.T) {
	var answered, said bytes.Buffer

	if code := runSubcommand.run(nil, &answered, &said); code != contract.ExitUsage {
		t.Errorf("nerdgenie run with no prompt left with %d, want %d", code, contract.ExitUsage)
	}
}

func TestRunWaitsForTheTaskToFinishWhenAskedTo(t *testing.T) {
	agent := aFakeAgent(t, answerTheMessageWith(
		contract.SocketEnvelope{Type: contract.SocketReply, Text: "working on it"},
		contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
			contract.StatusFieldRecordLine: "task 3 " + string(contract.StatusDone) + " · count the jars",
		}},
	))

	answered, _, code := runTheRunSubcommand(t, agent, "--wait", "count the jars")

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie run --wait left with %d", code)
	}
	if !strings.Contains(answered, "working on it") {
		t.Errorf("the reply printed as %q", answered)
	}
}

// TestRunReadsTheSocketFromTheHomeFolder proves the ordinary path: with nothing
// in the environment, the socket is the one the home folder names.
func TestRunReadsTheSocketFromTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NERDGENIE_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "run"), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the run folder failed: %v", err)
	}

	path, err := whereTheAgentIsListening()
	if err != nil {
		t.Fatalf("working out where the agent listens failed: %v", err)
	}
	if path != filepath.Join(home, "run", "agent.sock") {
		t.Errorf("nerdgenie run would talk to %q, want the home folder's own socket", path)
	}
}
