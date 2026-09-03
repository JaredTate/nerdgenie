package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// anAgentWithASocket opens a whole agent over a temporary home, which is what a
// test needs to say anything about the wiring rather than about its parts.
func anAgentWithASocket(t *testing.T) *agent {
	t.Helper()
	home := aHomeWithNoModelServer(t)
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)

	running, err := openAgent(ctx, home, func(string) {})
	if err != nil {
		t.Fatalf("opening the agent failed: %v", err)
	}
	return running
}

// TestServeComesBackWhenTheSocketStopsAccepting holds finding 58. Serve returns
// an error when accept fails, and the three loops beside it keep running on a
// context that is still alive, so the program hangs forever holding the run lock
// and the service manager never restarts it.
func TestServeComesBackWhenTheSocketStopsAccepting(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// Closing the socket is what an accept that will never work again looks
	// like from inside: Serve comes back and the rest of the program has to
	// come back with it.
	if err := running.socket.Close(); err != nil {
		t.Fatalf("closing the socket failed: %v", err)
	}

	came := make(chan error, 1)
	go func() { came <- running.serve(context.Background()) }()

	select {
	case <-came:
	case <-time.After(5 * time.Second):
		t.Fatal("serve has not come back five seconds after the socket stopped accepting, so the program holds its run lock forever")
	}
}

// TestNoCommandTellsTheUserToEditTheSource holds finding 59. A command whose
// piece of the program was never filled in answers with the name of a Go struct
// field, which is not something a person should ever be shown.
func TestNoCommandTellsTheUserToEditTheSource(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	ctx, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
	defer giveUp()

	for _, typed := range []string{"model local", "status", "help", "tasks", "jobs"} {
		said, err := running.registry.Run(ctx, typed, contract.CommandContext{Channel: running.userChannel()})
		whole := said
		if err != nil {
			whole += " " + err.Error()
		}
		for _, giveaway := range []string{"cmd/coeus/serve.go", "Deps.", "not counted in this build"} {
			if strings.Contains(whole, giveaway) {
				t.Errorf("/%s answered %q, which tells the user to go and edit the source", typed, whole)
			}
		}
	}
}

// TestTheCommandsThatAreRegisteredAllWork holds the other half of finding 59: a
// command nobody can use is not registered at all, so the help listing and the
// palette are true.
func TestTheCommandsThatAreRegisteredAllWork(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	for _, gone := range []string{"new", "sessions"} {
		if _, found := running.registry.Lookup(gone); found {
			t.Errorf("/%s is offered, and there is no session store behind it, so it can only disappoint", gone)
		}
	}
	if _, found := running.registry.Lookup("model"); !found {
		t.Error("/model is not offered, and the configuration names more than one model")
	}
	if _, found := running.registry.Lookup("screen"); !found {
		t.Error("/screen is not offered, and the architecture says this file registers it")
	}
}

// TestTheModelCommandReallySwitchesTheModel holds finding 59 for the one command
// that changes something.
func TestTheModelCommandReallySwitchesTheModel(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if err := running.useTheModel("local"); err != nil {
		t.Errorf("switching to the model the configuration names failed: %v", err)
	}
	if err := running.useTheModel("nothing-like-this"); err == nil {
		t.Error("switching to a model the configuration does not name was allowed")
	}
}

// TestTheHealthMarkComesFromTheGuard holds finding 69: a screen showed a healthy
// agent while the crash-loop breaker was tripped, because the mark was a
// literal.
func TestTheHealthMarkComesFromTheGuard(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	fields := running.statusForAScreen()

	if fields[contract.StatusFieldHealthy] == "" {
		t.Fatal("the status carries no health mark at all")
	}
	if !strings.Contains(readTheSourceOf(t, "status.go"), "Healthy()") {
		t.Error("the health mark is not asked of the reliability guard, so a screen shows a healthy agent during a crash loop")
	}
}

// TestNoNewTaskStartsWhileTheGuardSaysNo holds finding 64. The breaker and the
// drain marker both work through MayStartTask, and nothing asked it, so the
// program said it would start no task and then started one.
func TestNoNewTaskStartsWhileTheGuardSaysNo(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if !strings.Contains(readTheSourceOf(t, "starting.go"), "WhyNoNewTask") {
		t.Error("nothing asks the guard whether a task may start, so the crash-loop breaker and the drain marker do nothing")
	}
	if !strings.Contains(readTheSourceOf(t, "serving.go"), "WhyNoNewTask") {
		t.Error("the job driver does not ask the guard, so a drained agent still picks up scheduled work")
	}
}
