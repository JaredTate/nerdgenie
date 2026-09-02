package testkit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeSandboxReturnsTheResultScriptedForAPrefix(t *testing.T) {
	ctx := context.Background()
	sandbox := testkit.NewFakeSandbox()
	sandbox.Script("git status", contract.SandboxResult{StandardOutput: []byte("nothing to commit\n")})

	result, err := sandbox.Run(ctx, contract.SandboxCommand{Program: "git", Arguments: []string{"status"}})
	if err != nil {
		t.Fatalf("running a scripted command failed: %v", err)
	}
	if string(result.StandardOutput) != "nothing to commit\n" {
		t.Errorf("the command printed %q, want the scripted output", result.StandardOutput)
	}
	if len(sandbox.Commands()) != 1 {
		t.Errorf("the sandbox recorded %d commands, want 1", len(sandbox.Commands()))
	}
}

func TestTheFakeSandboxSaysSoWhenNothingWasScripted(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()

	if _, err := sandbox.Run(context.Background(), contract.SandboxCommand{Program: "whoami"}); err == nil {
		t.Fatal("running a command nobody scripted was reported as a success, want an error naming the command")
	}
}

func TestTheFakeSandboxCanBeMadeUnavailableTheWayAMissingBwrapWouldBe(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	if err := sandbox.Available(); err != nil {
		t.Fatalf("a fresh fake sandbox says it is unavailable: %v", err)
	}

	missing := errors.New("bwrap is not installed, so install bubblewrap and try again")
	sandbox.SetAvailable(missing)

	if err := sandbox.Available(); err == nil {
		t.Error("the sandbox still says it is available after being told bwrap is missing")
	}
	if _, err := sandbox.Run(context.Background(), contract.SandboxCommand{Program: "ls"}); err == nil {
		t.Error("an unavailable sandbox ran a command, and it should refuse")
	}
}

func TestTheFakeSandboxKeepsTheSandboxContractAvailableAndNot(t *testing.T) {
	ctx := context.Background()
	sandbox := testkit.NewFakeSandbox()

	if err := testkit.CheckSandbox(ctx, sandbox); err != nil {
		t.Fatalf("the fake sandbox does not keep the contract while it is available: %v", err)
	}
	if len(sandbox.Commands()) == 0 {
		t.Error("the sandbox check ran nothing against an available sandbox, so it asserted nothing at all")
	}

	sandbox.SetAvailable(errors.New("bwrap is not installed, so install bubblewrap and try again"))

	if err := testkit.CheckSandbox(ctx, sandbox); err != nil {
		t.Fatalf("the fake sandbox does not keep the contract once it is unavailable: %v", err)
	}
}
