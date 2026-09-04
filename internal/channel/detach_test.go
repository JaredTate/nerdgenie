package channel

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// A screen is a thin client: it attaches to a running agent over the socket and
// reads the event stream, but the task itself runs in the loop, not on the
// screen. So closing the terminal window, or a screen detaching, must take that
// screen off the stream and nothing more. The task keeps running, its record is
// left alone, and the person reattaches to work in progress. The tests here lock
// that down: the socket has no route to end a task except an explicit stop that
// goes through the queue, and detaching never sends one.

// theRunningTaskRecordLine is the one line a screen has to be handed the moment
// it attaches to an agent that already has a task running, so that a person who
// reattaches lands on that task rather than on an empty screen.
const theRunningTaskRecordLine = "task 3 started · count the jars"

// runningTaskStatus is what cmd/coeus reports while a task is working: the
// task's number, its record status, and its newest record line, under a state
// word a screen knows. A stream given this reports it to every screen the moment
// it attaches.
func runningTaskStatus() map[string]string {
	return map[string]string{
		contract.StatusFieldState:      contract.StateThinking,
		contract.StatusFieldTask:       "3",
		contract.StatusFieldTaskState:  string(contract.StatusRunning),
		contract.StatusFieldRecordLine: theRunningTaskRecordLine,
		contract.StatusFieldCommands:   theCommandList,
	}
}

// waitForExactlyAttached waits until the socket says exactly the wanted number
// of screens are reading the event stream, which is how a test knows a detach
// has been taken in rather than guessing it has by now. waitForAttached only
// waits for at least the wanted number, and every wanted number is at least
// zero, so it cannot be used to watch a screen leave.
func waitForExactlyAttached(t *testing.T, harness *socketHarness, wanted int) {
	t.Helper()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		if harness.socket.Attached() == wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%d screens are reading the event stream after %s, want exactly %d", harness.socket.Attached(), aReadWait, wanted)
}

// waitForExactlyClients waits until the socket holds exactly the wanted number
// of connections, which is how a test watches a screen that simply hung up be
// forgotten.
func waitForExactlyClients(t *testing.T, harness *socketHarness, wanted int) {
	t.Helper()
	deadline := time.Now().Add(aReadWait)
	for time.Now().Before(deadline) {
		if harness.socket.Clients() == wanted {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the socket holds %d clients after %s, want exactly %d", harness.socket.Clients(), aReadWait, wanted)
}

// nothingIsWaitingInTheQueue fails the test when anything at all is waiting in
// the queue, which is how a test proves that a detach or a hang-up put no stop
// there. The queue is the socket's one and only route to the loop, so a queue
// with nothing in it is a task the socket has left running.
func nothingIsWaitingInTheQueue(t *testing.T, harness *socketHarness) {
	t.Helper()
	taken, held, err := harness.queue.Take(context.Background())
	if err != nil {
		t.Fatalf("reading the queue failed: %v", err)
	}
	if held {
		t.Fatalf("the queue holds %q, and a detach or a hang-up must put nothing there", taken.Message.Text)
	}
}

// assertRunningTaskStatus fails the test unless the envelope is the status a
// screen is handed the moment it attaches to an agent with a task running: the
// running task's record line, its number, and its record status.
func assertRunningTaskStatus(t *testing.T, got contract.SocketEnvelope) {
	t.Helper()
	if got.Type != contract.SocketStatus {
		t.Fatalf("the screen was sent a %s, want a status carrying the running task's record", got.Type)
	}
	if got.Fields[contract.StatusFieldRecordLine] != theRunningTaskRecordLine {
		t.Errorf("the status carries the record line %q, want %q", got.Fields[contract.StatusFieldRecordLine], theRunningTaskRecordLine)
	}
	if got.Fields[contract.StatusFieldTask] != "3" {
		t.Errorf("the status names task %q, want %q", got.Fields[contract.StatusFieldTask], "3")
	}
	if got.Fields[contract.StatusFieldTaskState] != string(contract.StatusRunning) {
		t.Errorf("the status says the task is %q, want %q", got.Fields[contract.StatusFieldTaskState], contract.StatusRunning)
	}
}

func TestDetachingEndsNoTaskAndPutsNoStopInTheQueue(t *testing.T) {
	harness := newSocketHarness(t)
	client := harness.attach(t)

	client.send(contract.SocketEnvelope{Type: contract.SocketDetach})
	waitForExactlyAttached(t, harness, 0)

	// Detaching is purely "stop listening": it puts nothing in the queue, which
	// is the socket's one route to the loop, so no stop and no cancel could have
	// reached the running task.
	nothingIsWaitingInTheQueue(t, harness)

	// The screen is still connected, so a message it sends after detaching still
	// reaches the queue, and it is the first thing there: nothing the detach did
	// came before it.
	client.send(contract.SocketEnvelope{Type: contract.SocketMessage, Text: "still here"})
	if taken := harness.waitForQueued(t); taken.Message.Text != "still here" {
		t.Errorf("the queue holds %q, want %q", taken.Message.Text, "still here")
	}
}

func TestAScreenSimplyHangingUpEndsNoTask(t *testing.T) {
	// Closing the terminal window, or a screen the network dropped, hangs up on
	// the socket without a detach first. That must end no task either.
	harness := newSocketHarness(t)
	client := harness.attach(t)

	if err := client.socket.Close(); err != nil {
		t.Fatalf("closing the screen's connection failed: %v", err)
	}
	waitForExactlyClients(t, harness, 0)

	if attached := harness.socket.Attached(); attached != 0 {
		t.Errorf("%d screens are still reading the stream after a hang-up, want none", attached)
	}
	nothingIsWaitingInTheQueue(t, harness)
}

func TestReattachingAfterADetachLandsOnTheRunningTasksRecord(t *testing.T) {
	// A stream that reports a running task stands in for an agent mid-task: every
	// screen that attaches is handed the running task's record line at once.
	harness := newSocketHarnessReporting(t, theAnswerDeadline, runningTaskStatus)

	first := harness.attach(t)
	assertRunningTaskStatus(t, first.next())

	first.send(contract.SocketEnvelope{Type: contract.SocketDetach})
	waitForExactlyAttached(t, harness, 0)

	// A fresh screen attaches after the first one left. Because the detach ended
	// nothing, the task is still running, and the new screen lands on it rather
	// than on an empty screen.
	second := harness.attach(t)
	assertRunningTaskStatus(t, second.next())
}

func TestOnlyAnExplicitStopReachesTheQueueToEndTheTask(t *testing.T) {
	// The contrast that makes the point: a detach puts nothing in the queue, but
	// an explicit stop is a command that goes through the queue as "/stop", which
	// is what the loop turns into ending the task. Detaching is not that.
	harness := newSocketHarness(t)
	client := harness.attach(t)

	client.send(contract.SocketEnvelope{Type: contract.SocketDetach})
	waitForExactlyAttached(t, harness, 0)
	nothingIsWaitingInTheQueue(t, harness)

	client.send(contract.SocketEnvelope{Type: contract.SocketCommand, Text: "stop"})
	if taken := harness.waitForQueued(t); taken.Message.Text != CommandPrefix+"stop" {
		t.Errorf("an explicit stop reached the queue as %q, want %q", taken.Message.Text, CommandPrefix+"stop")
	}
}
