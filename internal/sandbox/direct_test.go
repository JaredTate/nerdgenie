package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theShell is what every command below is handed to, because the shell tool
// hands the direct runner a shell and a command line exactly as it hands them to
// the fence.
const theShell = "/bin/sh"

func TestADirectRunnerIsASandboxAsTheContractDescribesIt(t *testing.T) {
	var _ contract.Sandbox = (*Direct)(nil)
	runner := NewDirect(Settings{})

	if err := runner.Available(); err != nil {
		t.Errorf("the direct runner says it cannot run: %v", err)
	}
	if err := testkit.CheckSandbox(context.Background(), runner); err != nil {
		t.Fatalf("the direct runner does not keep the sandbox contract: %v", err)
	}
}

func TestACommandRunStraightOnTheMachineComesBackWithWhatItPrinted(t *testing.T) {
	folder := t.TempDir()
	runner := NewDirect(Settings{})

	result, err := runner.Run(context.Background(), contract.SandboxCommand{
		Program:          theShell,
		Arguments:        []string{"-c", "pwd; cat; echo trouble >&2"},
		WorkingDirectory: folder,
		StandardInput:    []byte("what was typed in\n"),
	})
	if err != nil {
		t.Fatalf("running a command straight on the machine failed: %v", err)
	}

	printed := string(result.StandardOutput)
	if !strings.Contains(printed, whereTheTemporaryFolderReallyIs(t, folder)) {
		t.Errorf("the command printed %q, and it did not run in the folder it was given, %s", printed, folder)
	}
	if !strings.Contains(printed, "what was typed in") {
		t.Errorf("the command printed %q, and what was fed to it never arrived", printed)
	}
	if !strings.Contains(string(result.StandardError), "trouble") {
		t.Errorf("the error output is %q, want what the command wrote there", result.StandardError)
	}
	if result.ExitCode != 0 || result.TimedOut {
		t.Errorf("a command that worked came back with code %d and timed out %v", result.ExitCode, result.TimedOut)
	}
}

func TestTheExitCodeOfACommandRunStraightOnTheMachineComesBack(t *testing.T) {
	runner := NewDirect(Settings{})

	result, err := runner.Run(context.Background(), contract.SandboxCommand{
		Program:   theShell,
		Arguments: []string{"-c", "exit 7"},
	})
	if err != nil {
		t.Fatalf("running a command that quits with seven failed: %v", err)
	}
	if result.ExitCode != 7 {
		t.Errorf("the command came back with code %d, want the 7 it quit with", result.ExitCode)
	}
}

func TestACommandThatCannotBeStartedIsRefusedByName(t *testing.T) {
	runner := NewDirect(Settings{})

	_, err := runner.Run(context.Background(), contract.SandboxCommand{Program: "/no/such/program/anywhere"})
	if err == nil {
		t.Fatal("a program that is on no machine was reported as having run")
	}
	if !strings.Contains(err.Error(), "/no/such/program/anywhere") {
		t.Errorf("the refusal says %q, and it must name the program that could not be started", err)
	}
}

func TestOutputPastTheCapIsCutWithALineSayingHowMuchWasDropped(t *testing.T) {
	runner := NewDirect(Settings{OutputCap: 20})

	result, err := runner.Run(context.Background(), contract.SandboxCommand{
		Program:   theShell,
		Arguments: []string{"-c", "printf '%0500d' 0"},
	})
	if err != nil {
		t.Fatalf("running a command that prints five hundred bytes failed: %v", err)
	}

	printed := string(result.StandardOutput)
	if !strings.HasPrefix(printed, strings.Repeat("0", 20)) {
		t.Errorf("the output begins %q, want the first twenty bytes the command printed", printed)
	}
	if !strings.Contains(printed, "dropped 480 more bytes") {
		t.Errorf("the output is %q, and it does not say how much was dropped", printed)
	}
}

func TestTheDefaultOutputCapIsTheOneTheConfigurationShips(t *testing.T) {
	if capped := NewDirect(Settings{}).outputCap; capped != contract.DefaultConfig().Caps.ToolOutputBytes {
		t.Errorf("the direct runner caps output at %d bytes, want the shipped %d",
			capped, contract.DefaultConfig().Caps.ToolOutputBytes)
	}
}

func TestACommandThatOutlivesItsTimeoutIsStoppedAlongWithWhatItStarted(t *testing.T) {
	folder := t.TempDir()
	late := filepath.Join(folder, "late.txt")
	runner := NewDirect(Settings{})

	began := time.Now()
	result, err := runner.Run(context.Background(), contract.SandboxCommand{
		Program:   theShell,
		Arguments: []string{"-c", "(sleep 1; echo late > " + late + ") & sleep 60"},
		Timeout:   200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("running a command that outlives its timeout failed: %v", err)
	}
	if !result.TimedOut {
		t.Errorf("the command was stopped by its timeout and the result does not say so")
	}
	if waited := time.Since(began); waited > 30*time.Second {
		t.Fatalf("the command was waited out for %s rather than stopped at its timeout", waited)
	}

	time.Sleep(2 * time.Second)
	if _, err := os.Stat(late); err == nil {
		t.Errorf("the command's own child outlived it and wrote %s, so the whole process group was not stopped", late)
	}
}

func TestCancellingTheRunStopsTheCommandStraightAway(t *testing.T) {
	runner := NewDirect(Settings{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)

	began := time.Now()
	result, err := runner.Run(ctx, contract.SandboxCommand{
		Program:   theShell,
		Arguments: []string{"-c", "sleep 60"},
	})
	if err != nil {
		t.Fatalf("running a command that is cancelled failed: %v", err)
	}
	if waited := time.Since(began); waited > 30*time.Second {
		t.Fatalf("the cancelled command was waited out for %s rather than stopped", waited)
	}
	if !result.TimedOut {
		t.Errorf("the cancelled command came back as one that finished on its own")
	}
}

func TestTheCommandRunsAsTheUserWithTheirOwnPathAndWhatTheCallerAdds(t *testing.T) {
	t.Setenv("NERDGENIE_A_WORD_FROM_THE_MACHINE", "the machine said this")
	runner := NewDirect(Settings{})

	result, err := runner.Run(context.Background(), contract.SandboxCommand{
		Program:     theShell,
		Arguments:   []string{"-c", "echo $NERDGENIE_A_WORD_FROM_THE_MACHINE; echo $NERDGENIE_A_WORD_FROM_THE_CALLER"},
		Environment: []string{"NERDGENIE_A_WORD_FROM_THE_CALLER=the caller said this"},
	})
	if err != nil {
		t.Fatalf("running a command that reads its environment failed: %v", err)
	}

	printed := string(result.StandardOutput)
	if !strings.Contains(printed, "the machine said this") {
		t.Errorf("the command printed %q, and it did not inherit the environment the agent runs in", printed)
	}
	if !strings.Contains(printed, "the caller said this") {
		t.Errorf("the command printed %q, and what the caller added never reached it", printed)
	}
}

// whereTheTemporaryFolderReallyIs follows the links along a temporary folder's
// path, because a machine whose /tmp is a link prints the folder the link leads
// to when a command asks where it is running.
func whereTheTemporaryFolderReallyIs(t *testing.T, folder string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return folder
	}
	return resolved
}
