package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aHomeWithNoModelServer gives a test its own home folder with a configuration
// naming a model nothing is listening for, which is enough for everything but a
// model call.
func aHomeWithNoModelServer(t *testing.T) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	work := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(work, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the work folder failed: %v", err)
	}
	settings := fmt.Sprintf("default_model = \"local\"\nsandbox_roots = [%q]\n", work)
	if err := os.WriteFile(home.ConfigFile(), []byte(settings), contract.DataFileMode); err != nil {
		t.Fatalf("writing the configuration failed: %v", err)
	}
	return home
}

func TestServeTakesNoArguments(t *testing.T) {
	var output, problems bytes.Buffer

	code := serveSubcommand.run([]string{"now"}, &output, &problems)

	if code != contract.ExitUsage {
		t.Errorf("coeus serve left with %d rather than %d when given a word it does not understand", code, contract.ExitUsage)
	}
}

func TestTheRunLockRefusesASecondHolderAndNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coeus.lock")

	first, err := takeRunLock(path)
	if err != nil {
		t.Fatalf("taking the lock failed: %v", err)
	}
	defer func() { _ = first.release() }()

	if _, err := takeRunLock(path); err == nil {
		t.Fatal("a second holder took the same lock, so two agents could run on one home folder")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("the refusal %q does not name the lock file that stopped it", err)
	}
}

func TestTheRunLockIsFreeAgainOnceItIsReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coeus.lock")
	first, err := takeRunLock(path)
	if err != nil {
		t.Fatalf("taking the lock failed: %v", err)
	}
	if err := first.release(); err != nil {
		t.Fatalf("releasing the lock failed: %v", err)
	}

	second, err := takeRunLock(path)
	if err != nil {
		t.Fatalf("the lock was never free again after it was released: %v", err)
	}
	_ = second.release()
}

func TestTheAgentRegistersTheCommandsEveryPackageOwns(t *testing.T) {
	home := aHomeWithNoModelServer(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	running, err := openAgent(ctx, home, func(string) {})
	if err != nil {
		t.Fatalf("opening the agent failed: %v", err)
	}
	defer func() { _ = running.close() }()

	names := []string{}
	for _, command := range running.registry.All() {
		names = append(names, command.Name)
	}
	for _, wanted := range []string{
		"help", "status", "model", "approve", "deny",
		"pause", "resume", "undo", "vault", "memory", readyName,
		"tasks", "stop", "jobs", "cron", "skills", "screen",
	} {
		if !slices.Contains(names, wanted) {
			t.Errorf("the registry has no /%s command; it holds %v", wanted, names)
		}
	}
}

func TestASecondAgentOnTheSameHomeIsRefused(t *testing.T) {
	home := aHomeWithNoModelServer(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	first, err := openAgent(ctx, home, func(string) {})
	if err != nil {
		t.Fatalf("opening the agent failed: %v", err)
	}
	defer func() { _ = first.close() }()

	if _, err := openAgent(ctx, home, func(string) {}); err == nil {
		t.Error("a second agent opened the same home folder, so two of them would answer on one socket")
	}
}

func TestTheReadyCommandSaysTheAgentIsReady(t *testing.T) {
	answer, err := readyCommand().Run(context.Background(), "", contract.CommandContext{})

	if err != nil {
		t.Fatalf("the ready command failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(answer), "ready") {
		t.Errorf("the ready command answered %q, want a line saying the agent is ready", answer)
	}
}
