package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestAFenceIsASandboxAsTheContractDescribesIt(t *testing.T) {
	var _ contract.Sandbox = (*Fence)(nil)

	t.Setenv("PATH", "")
	fence, _ := aFenceForTesting(t)

	if err := testkit.CheckSandbox(context.Background(), fence); err != nil {
		t.Fatalf("the fence does not keep the sandbox contract: %v", err)
	}
}

func TestRunRefusesEveryCommandWhenBwrapIsNotOnThePath(t *testing.T) {
	t.Setenv("PATH", "")
	fence, _ := aFenceForTesting(t)

	_, err := fence.Run(context.Background(), contract.SandboxCommand{Program: "/bin/true"})
	if err == nil {
		t.Fatal("the fence ran a command with no bwrap on the PATH")
	}
	if !strings.Contains(err.Error(), "bwrap") {
		t.Errorf("the refusal says %q, and it must name what is missing", err)
	}
}

func TestACommandThatWasNeverWaitedForReportsAFailureRatherThanASuccess(t *testing.T) {
	if code := exitCodeOf(&exec.Cmd{}); code != contract.ExitFailure {
		t.Errorf("a command with no result at all reported %d, want %d", code, contract.ExitFailure)
	}
}

func TestRunRefusesACommandThePlanWillNotAllowBeforeItStartsAnything(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	for what, command := range map[string]contract.SandboxCommand{
		"no program":                      {},
		"a working directory outside all": {Program: "/bin/true", WorkingDirectory: "/etc"},
		"a zero byte in the program":      {Program: "/bin/true\x00"},
	} {
		if _, err := fence.Run(context.Background(), command); err == nil {
			t.Errorf("the fence accepted a command with %s", what)
		}
	}
}
