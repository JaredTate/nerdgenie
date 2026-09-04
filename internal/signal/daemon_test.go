package signal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// writeProgram writes a stand-in for signal-cli that records the arguments it
// was given and then does what the body says, and returns its path and the file
// the arguments land in.
func writeProgram(t *testing.T, body string) (string, string) {
	t.Helper()
	folder := t.TempDir()
	arguments := filepath.Join(folder, "arguments")
	path := filepath.Join(folder, "signal-cli")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %q\n%s\n", arguments, body)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("cannot write the stand-in for signal-cli: %v", err)
	}
	return path, arguments
}

// newTestDaemon builds a supervisor for a stand-in program whose health is
// whatever the test says it is.
func newTestDaemon(t *testing.T, program string, clock *testkit.FakeClock, healthy func() bool) *Daemon {
	t.Helper()
	daemon, err := NewDaemon(DaemonOptions{
		Program: program,
		Account: "+15125550100",
		Address: "127.0.0.1:18420",
		Clock:   clock,
		Healthy: func(context.Context) bool { return healthy() },
	})
	if err != nil {
		t.Fatalf("cannot build the daemon supervisor: %v", err)
	}
	t.Cleanup(func() { _ = daemon.Stop() })
	return daemon
}

func TestDaemonStartsSignalCliWithTheArgumentsTheDesignNames(t *testing.T) {
	program, argumentsFile := writeProgram(t, "while true; do sleep 0.1; done")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	daemon := newTestDaemon(t, program, clock, func() bool { return true })

	if err := daemon.Start(context.Background()); err != nil {
		t.Fatalf("starting the daemon failed: %v", err)
	}

	waitFor(t, "the program writes down its arguments", func() bool {
		written, err := os.ReadFile(argumentsFile)
		return err == nil && len(written) > 0
	})
	written, err := os.ReadFile(argumentsFile)
	if err != nil {
		t.Fatalf("cannot read what the program was given: %v", err)
	}
	given := strings.TrimSpace(string(written))
	want := "-a +15125550100 daemon --http 127.0.0.1:18420 --no-receive-stdout"
	if given != want {
		t.Errorf("signal-cli was started with %q, want %q", given, want)
	}
}

func TestDaemonRunsSignalCliInItsOwnProcessGroupAndKillsItByItsOwnIdentifier(t *testing.T) {
	program, _ := writeProgram(t, "while true; do sleep 0.1; done")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	daemon := newTestDaemon(t, program, clock, func() bool { return true })

	if err := daemon.Start(context.Background()); err != nil {
		t.Fatalf("starting the daemon failed: %v", err)
	}
	identifier := daemon.ProcessID()
	if identifier <= 0 {
		t.Fatalf("the daemon has no process identifier after it started")
	}
	group, err := syscall.Getpgid(identifier)
	if err != nil {
		t.Fatalf("cannot ask which process group signal-cli is in: %v", err)
	}
	if group != identifier {
		t.Errorf("signal-cli is in process group %d and its own identifier is %d, want it to lead its own group so the whole group can be killed", group, identifier)
	}

	if err := daemon.Stop(); err != nil {
		t.Fatalf("stopping the daemon failed: %v", err)
	}
	waitFor(t, "the process goes away", func() bool {
		return syscall.Kill(identifier, 0) != nil
	})
}

func TestDaemonStartsSignalCliAgainAfterItDies(t *testing.T) {
	program, _ := writeProgram(t, "exit 3")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	daemon := newTestDaemon(t, program, clock, func() bool { return true })

	if err := daemon.Start(context.Background()); err != nil {
		t.Fatalf("starting the daemon failed: %v", err)
	}
	waitFor(t, "the supervisor waits before starting it again", func() bool { return clock.Sleepers() > 0 })
	if daemon.Starts() != 1 {
		t.Fatalf("the supervisor started signal-cli %d times without waiting, and a dead daemon is waited out", daemon.Starts())
	}

	clock.Advance(ReconnectMinimumWait)
	waitFor(t, "the supervisor starts it again", func() bool { return daemon.Starts() >= 2 })
}

func TestDaemonGivesUpWhenSignalCliNeverBecomesHealthy(t *testing.T) {
	program, _ := writeProgram(t, "while true; do sleep 0.1; done")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	daemon := newTestDaemon(t, program, clock, func() bool { return false })

	failed := make(chan error, 1)
	go func() { failed <- daemon.Start(context.Background()) }()

	waitFor(t, "the supervisor waits between health checks", func() bool { return clock.Sleepers() > 0 })
	clock.Advance(DaemonStartupTimeout)

	select {
	case err := <-failed:
		if err == nil {
			t.Fatalf("the supervisor said signal-cli was ready when it never answered its health check")
		}
		if !strings.Contains(err.Error(), "health") {
			t.Errorf("the error is %q, want it to say the health check never answered", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the supervisor never gave up on a daemon that never became healthy")
	}
	if daemon.ProcessID() != 0 {
		t.Errorf("the supervisor left signal-cli running after it gave up on it")
	}
}

func TestDaemonSaysSoWhenThereIsNoSignalCli(t *testing.T) {
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	daemon := newTestDaemon(t, filepath.Join(t.TempDir(), "there-is-no-such-program"), clock, func() bool { return true })

	err := daemon.Start(context.Background())
	if err == nil {
		t.Fatalf("the supervisor started a program that is not there")
	}
	if !strings.Contains(err.Error(), "signal-cli") {
		t.Errorf("the error is %q, want it to name signal-cli and say what to do", err)
	}
}

func TestNewDaemonRefusesWhatItCannotRun(t *testing.T) {
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	cases := []struct {
		name    string
		options DaemonOptions
	}{
		{"no program", DaemonOptions{Account: "+1", Address: "127.0.0.1:1", Clock: clock}},
		{"no account", DaemonOptions{Program: "signal-cli", Address: "127.0.0.1:1", Clock: clock}},
		{"no address", DaemonOptions{Program: "signal-cli", Account: "+1", Clock: clock}},
		{"no clock", DaemonOptions{Program: "signal-cli", Account: "+1", Address: "127.0.0.1:1"}},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			if _, err := NewDaemon(oneCase.options); err == nil {
				t.Errorf("the supervisor was built with %s", oneCase.name)
			}
		})
	}
}
