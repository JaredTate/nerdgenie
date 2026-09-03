package desktop

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAWorkerCommandWithNothingInItIsRefused(t *testing.T) {
	if _, err := ProcessStart(nil, nil); err == nil {
		t.Fatal("an empty worker command was accepted, and there would be nothing to start")
	}
	if _, err := ProcessStart([]string{"   "}, nil); err == nil {
		t.Fatal("a worker command of nothing but spaces was accepted, and there would be nothing to start")
	}
}

func TestAWorkerProgramThatIsNotThereIsReportedByName(t *testing.T) {
	start, err := ProcessStart([]string{"no-such-program-anywhere", "--pacing", "fast"}, nil)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}

	_, err = start(context.Background())

	if err == nil || !strings.Contains(err.Error(), "no-such-program-anywhere") {
		t.Fatalf("the error is %v, want one naming the program that could not be started", err)
	}
}

func TestAWorkerStartedAsAProcessAnswersAndIsStoppedByItsExactProcessID(t *testing.T) {
	// A shell standing in for the worker: it reads one request line and answers
	// it the way worker/desktop does, then waits to be stopped.
	answer := `read line; printf '{"jsonrpc":"2.0","id":1,"result":{"healthy":true,"driverVersion":"stand-in"}}\n'; sleep 30`
	lines := []string{}
	start, err := ProcessStart([]string{"sh", "-c", answer}, func(format string, arguments ...any) {
		lines = append(lines, format)
	})
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}

	connection, err := start(context.Background())
	if err != nil {
		t.Fatalf("starting the stand-in worker failed: %v", err)
	}
	if connection.ProcessID <= 0 {
		t.Fatalf("the connection carries process id %d, and the worker can only be killed by an exact one", connection.ProcessID)
	}

	var health healthAnswer
	ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if err := newClient(connection).call(ctx, "health", map[string]any{}, &health); err != nil {
		t.Fatalf("asking the stand-in worker whether it is healthy failed: %v", err)
	}
	if health.DriverVersion != "stand-in" {
		t.Errorf("the worker reported driver %q, want the stand-in's own", health.DriverVersion)
	}

	if err := connection.Stop(); err != nil {
		t.Fatalf("stopping the stand-in worker failed: %v", err)
	}
	if err := syscall.Kill(connection.ProcessID, 0); err == nil {
		t.Errorf("the process %d is still there after it was stopped", connection.ProcessID)
	}
}

func TestTheWorkerIsHandedAnEnvironmentWithNoSecretsInIt(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "a key nobody should pass on")
	t.Setenv("DISPLAY", ":0")
	t.Setenv("PATH", os.Getenv("PATH"))

	handed := workerEnvironment()

	for _, line := range handed {
		if strings.HasPrefix(line, "ANTHROPIC_API_KEY=") {
			t.Error("the worker was handed an API key, and the driver is another project's program")
		}
	}
	var sawDisplay bool
	for _, line := range handed {
		if line == "DISPLAY=:0" {
			sawDisplay = true
		}
	}
	if !sawDisplay {
		t.Errorf("the worker was handed %v, want the display it has to draw on", handed)
	}
}
