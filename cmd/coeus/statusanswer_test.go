package main

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestAStatusQuestionIsRecognisedOnlyWhenItIsTheWholeMessage(t *testing.T) {
	// The whole message, however it is cased or punctuated, is one of the set.
	for _, said := range []string{
		"status", "Status", "STATUS", "status.", "status?",
		"update", "update me", "Update me.", "where are we", "Where are we?",
		"where are we at", "what happened", "what's the status", "whats the status",
		"  where   are   we  ", "what’s the status",
	} {
		if !saysStatusQuestion(said) {
			t.Errorf("%q was not read as a status question, and it asks only where the work stands", said)
		}
	}
	// A message that merely holds those words as part of a real ask is not one.
	for _, said := range []string{
		"update the readme to say where we are",
		"what happened to the login page yesterday",
		"tell me the status of the deploy and then restart it",
		"where are we going for lunch",
		"", "go on", "continue",
	} {
		if saysStatusQuestion(said) {
			t.Errorf("%q was read as a status question, and it is a real ask that must be handled normally", said)
		}
	}
}

func TestTheHeaderLineAndStandingLineReadTheRecord(t *testing.T) {
	withSituation := contract.Record{
		Header: contract.Header{ID: "7", Status: contract.StatusStopped, Origin: "terminal"},
		Goal:   contract.Goal{Ask: "write the release notes for the month"},
		Work:   contract.Work{Situation: []string{"on the drafts page", "two files changed"}},
	}
	header := taskHeaderLine(withSituation)
	for _, want := range []string{"task 7", "stopped", "terminal", "release notes"} {
		if !strings.Contains(header, want) {
			t.Errorf("the header line %q does not carry %q", header, want)
		}
	}
	if standing := taskStandingLine(withSituation); !strings.Contains(standing, "on the drafts page") {
		t.Errorf("the standing line %q does not carry the situation", standing)
	}

	withPlan := contract.Record{
		Header: contract.Header{ID: "8", Status: contract.StatusWaiting},
		Work: contract.Work{Plan: []contract.PlanStep{
			{Number: 1, Text: "draft", Done: true},
			{Number: 2, Text: "review"},
		}},
	}
	if standing := taskStandingLine(withPlan); !strings.Contains(standing, "1 of 2") {
		t.Errorf("the standing line %q does not say how many steps are done", standing)
	}

	bare := contract.Record{Header: contract.Header{ID: "9", Status: contract.StatusRunning}}
	if standing := taskStandingLine(bare); standing == "" {
		t.Error("a record with no situation and no plan has an empty standing line, so a person is told nothing")
	}
}

func TestTheStatusAnswerCarriesTheMostRecentTask(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aTaskInTheLog(t, running.events, "3", fromTheTerminal, contract.StatusStopped)
	aTaskInTheLog(t, running.events, "5", fromTheTerminal, contract.StatusWaiting)

	answer := running.statusAnswer(context.Background())

	if !strings.Contains(answer, "task 5") {
		t.Errorf("the status answer %q does not name task 5, the most recent task", answer)
	}
	if !strings.Contains(answer, string(contract.StatusWaiting)) {
		t.Errorf("the status answer %q does not carry task 5's real standing", answer)
	}
}

func TestTheStatusAnswerSaysNothingInFlightWhenNoTaskWasRecorded(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if answer := running.statusAnswer(context.Background()); answer != nothingInFlight {
		t.Errorf("the status answer with no task recorded is %q, want %q", answer, nothingInFlight)
	}
}

// TestAStatusQuestionIsAnsweredFromTheRecordsWithNoNewTask is the 16:19
// scenario: a task exists, the person sends "where are we", and the answer
// carries that task's real standing rather than a fresh blind task's "no active
// work". No task is started and the model is never called.
func TestAStatusQuestionIsAnsweredFromTheRecordsWithNoNewTask(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	aTaskInTheLog(t, running.events, "7", fromTheTerminal, contract.StatusStopped)
	reader := aScreenAttachedTo(t, running)

	// The memory is empty, as it is after the restart the real user hit, so the
	// old code would have started a fresh task here.
	lastTasks := newScreenTasks()
	message := contract.Inbound{Channel: contract.TerminalChannelName, Sender: contract.TerminalChannelName, Text: "where are we"}
	if err := running.startTask(context.Background(), lastTasks, message); err != nil {
		t.Fatalf("answering the status question failed: %v", err)
	}

	reply := readReplyText(t, reader)
	if !strings.Contains(reply, "task 7") || !strings.Contains(reply, string(contract.StatusStopped)) {
		t.Errorf("the status answer %q does not carry task 7's real standing", reply)
	}
	if running.loop.Running() != "" {
		t.Errorf("a task %q is running after a status question, and a status question starts no task", running.loop.Running())
	}
	if running.loopIsBusy() {
		t.Error("the loop is busy after a status question, and answering from the records takes no task up")
	}
}

// aScreenAttachedTo starts the socket accepting and attaches one screen to it,
// so that a test can read the replies the agent sends the user.
func aScreenAttachedTo(t *testing.T, running *agent) *bufio.Reader {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	go func() { _ = running.socket.Serve(ctx) }()

	connection, err := net.Dial("unix", running.home.SocketFile())
	if err != nil {
		t.Fatalf("cannot attach a screen to the socket at %s: %v", running.home.SocketFile(), err)
	}
	t.Cleanup(func() { _ = connection.Close(); stop() })
	if err := contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{Type: contract.SocketAttach}); err != nil {
		t.Fatalf("cannot attach the screen: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if running.socket.Attached() >= 1 {
			if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatalf("cannot put a deadline on the screen's link: %v", err)
			}
			return bufio.NewReader(connection)
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the screen never attached to the socket")
	return nil
}

// readReplyText reads envelopes off the screen's link until a reply arrives,
// stepping over the status the socket sends a screen that has just attached.
func readReplyText(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("no reply reached the screen: %v", err)
		}
		envelope, err := contract.DecodeSocketEnvelope(line)
		if err != nil {
			continue
		}
		if envelope.Type == contract.SocketReply {
			return envelope.Text
		}
	}
}
