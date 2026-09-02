//go:build integration

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theToolOutputCap is the cap the fence uses when a test does not care about it.
const theToolOutputCap = 30000

func TestASandboxedReadOfTheSshFolderFails(t *testing.T) {
	fence, userHome, _ := aRealFence(t, theToolOutputCap)

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/cat",
		Arguments: []string{filepath.Join(userHome, ".ssh", "id_fixture")},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed before the fence could refuse it: %v", err)
	}

	if result.ExitCode == 0 {
		t.Errorf("a sandboxed command read the SSH folder and got %q, and the keys must stay outside the fence", result.StandardOutput)
	}
	if strings.Contains(string(result.StandardOutput), "not a key") {
		t.Error("the fixture key came back out of the fence")
	}
}

func TestASandboxedWriteToTheAgentsOwnHomeFails(t *testing.T) {
	fence, userHome, _ := aRealFence(t, theToolOutputCap)
	target := filepath.Join(userHome, contract.HomeFolderName, "written-by-the-sandbox")

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", "echo taken > " + target},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed before the fence could refuse it: %v", err)
	}

	if result.ExitCode == 0 {
		t.Error("a sandboxed command wrote into the agent's own home folder")
	}
	if _, err := os.Stat(target); err == nil {
		t.Errorf("the file %s is there, and the agent's home folder must stay outside the fence", target)
	}
}

func TestASandboxedWriteInsideTheRootSucceeds(t *testing.T) {
	fence, _, work := aRealFence(t, theToolOutputCap)
	note := filepath.Join(work, "note.txt")

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", "echo hello > " + note},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("writing inside the root reported %d and said %q", result.ExitCode, result.StandardError)
	}

	written, err := os.ReadFile(note)
	if err != nil {
		t.Fatalf("the file the sandboxed command wrote is not there: %v", err)
	}
	if string(written) != "hello\n" {
		t.Errorf("the file holds %q, want %q", written, "hello\n")
	}
}

func TestASandboxedNetworkCallReachesALoopbackServer(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)
	search := testkit.NewFakeSearchServer()
	defer search.Close()

	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this test binary on disk: %v", err)
	}
	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   thisProgram,
		Arguments: []string{fetchMode, search.PageAddress("/notes")},
		Timeout:   30 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("the sandboxed fetch reported %d and said %q", result.ExitCode, result.StandardError)
	}
	if !strings.Contains(string(result.StandardOutput), "DigiByte product notes") {
		t.Errorf("the page came back as %q, want the fixture page", result.StandardOutput)
	}
}

func TestACommandThatRunsPastItsTimeoutIsKilledWithEverythingItStarted(t *testing.T) {
	fence, _, work := aRealFence(t, theToolOutputCap)
	ticks := filepath.Join(work, "ticks")

	started := time.Now()
	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", "( while true; do echo tick >> " + ticks + "; sleep 0.1; done ) & sleep 120"},
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if !result.TimedOut {
		t.Error("the command was not reported as timed out")
	}
	if time.Since(started) > 30*time.Second {
		t.Errorf("the fence took %s to stop a command with a one-second timeout", time.Since(started))
	}

	// The grandchild is the "while true" loop, which the shell started and which
	// nothing else would stop. If the whole process group went, the file stops
	// growing.
	settled := sizeOf(t, ticks)
	time.Sleep(2 * time.Second)
	if grown := sizeOf(t, ticks); grown != settled {
		t.Errorf("the ticks file grew from %d to %d bytes after the command was killed, so the grandchild is still running", settled, grown)
	}
}

// sizeOf is how many bytes a file holds, or zero when it was never made.
func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	details, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return details.Size()
}

func TestOutputPastTheCapIsDroppedWithTheNote(t *testing.T) {
	fence, _, _ := aRealFence(t, 64)

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", "index=0; while [ $index -lt 500 ]; do echo abcdefghij; index=$((index+1)); done"},
		Timeout:   30 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}

	whole := string(result.StandardOutput)
	if !strings.Contains(whole, "coeus dropped") {
		t.Errorf("the output is %q, and it must end with a note saying how much was dropped", whole)
	}
	if len(whole) > 64+200 {
		t.Errorf("the output is %d bytes, and the cap was 64 with room for one note", len(whole))
	}
}

func TestTheExitCodeAndTheInputAndOutputComeBackAsTheyWere(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:       "/bin/sh",
		Arguments:     []string{"-c", "cat; exit 7"},
		StandardInput: []byte("what went in"),
		Timeout:       20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode != 7 {
		t.Errorf("the command reported %d, want the 7 it exited with", result.ExitCode)
	}
	if !strings.Contains(string(result.StandardOutput), "what went in") {
		t.Errorf("the output is %q, want what was fed to the command", result.StandardOutput)
	}
	if result.TimedOut {
		t.Error("a command that finished on its own was reported as timed out")
	}
}

func TestACommandThatIgnoresBeingAskedToStopIsKilledOutright(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", `trap "" TERM; index=0; while [ $index -lt 200000000 ]; do index=$((index+1)); done`},
		Timeout:   500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if !result.TimedOut {
		t.Error("a command that ignores being asked to stop was not reported as timed out")
	}
}

func TestRunSaysSoWhenItCannotMakeTheScratchHomeFolder(t *testing.T) {
	fence, _, work := aRealFence(t, theToolOutputCap)
	if err := os.Chmod(work, 0o500); err != nil {
		t.Fatalf("cannot make the root read-only for the test: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(work, contract.HomeFolderMode) })

	_, err := fence.Run(context.Background(), contract.SandboxCommand{Program: "/bin/true", Timeout: 20 * time.Second})
	if err == nil {
		t.Fatal("the fence ran a command with no home folder to give it")
	}
	if !strings.Contains(err.Error(), scratchHomeName) {
		t.Errorf("the refusal says %q, and it must name the folder it could not make", err)
	}
}

func TestTheHelperRefusesToRunWhenNothingStartedItInsideAFence(t *testing.T) {
	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this test binary on disk: %v", err)
	}

	outside := exec.Command(thisProgram, EntrySubcommandName, "--write", "/tmp", "--", "/bin/true")
	outside.Env = []string{}
	output, err := outside.CombinedOutput()

	if err == nil {
		t.Fatalf("the helper ran outside a fence and said %q", output)
	}
	if !strings.Contains(string(output), EntrySubcommandName) {
		t.Errorf("the helper said %q, and it must name the subcommand a person should not be typing", output)
	}
}

func TestAvailableNamesBwrapWhenNothingIsOnThePath(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)
	t.Setenv("PATH", "")

	err := fence.Available()
	if err == nil {
		t.Fatal("the sandbox reported itself available with nothing at all on the PATH")
	}
	if !strings.Contains(err.Error(), bubblewrapProgram) {
		t.Errorf("the reason says %q, and it must name %s", err, bubblewrapProgram)
	}
}

func TestTheRealBubblewrapMakesTheFenceOnThisMachine(t *testing.T) {
	works, reason := realBubblewrapCanMakeAFence()
	if !works {
		t.Skipf("bwrap cannot make a user namespace on this machine, so the tests above ran against the stand-in "+
			"and this one cannot run at all. Fix it with root: either write an AppArmor profile for /usr/bin/bwrap "+
			"that allows userns, or set kernel.apparmor_restrict_unprivileged_userns to 0. The probe said: %s", reason)
	}

	fence, userHome, _ := aRealFence(t, theToolOutputCap)
	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", "ls " + filepath.Join(userHome, ".ssh")},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode == 0 {
		t.Errorf("the SSH folder was listed inside a real bwrap fence and gave %q", result.StandardOutput)
	}
}

func TestARealFenceKeepsTheSandboxContract(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	if err := testkit.CheckSandbox(context.Background(), fence); err != nil {
		t.Fatalf("the fence does not keep the sandbox contract: %v", err)
	}
}

// failingBubblewrapScript stands in for a bwrap that is installed but cannot make
// a user namespace, which is what an unfixed Ubuntu looks like.
const failingBubblewrapScript = `#!/bin/sh
echo "bwrap: setting up uid map: Permission denied" >&2
exit 1
`

func TestAvailableNamesTheAppArmorFixWhenBwrapCannotMakeANamespace(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, bubblewrapProgram), []byte(failingBubblewrapScript), 0o755); err != nil {
		t.Fatalf("cannot write the stand-in for %s: %v", bubblewrapProgram, err)
	}
	t.Setenv("PATH", folder+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := fence.Available()
	if err == nil {
		t.Fatal("the sandbox reported itself available with a bwrap that cannot make a user namespace")
	}
	said := strings.ToLower(err.Error())
	for _, wanted := range []string{"apparmor", "userns"} {
		if !strings.Contains(said, wanted) {
			t.Errorf("the reason says %q, and it must name %q so that a person knows what to change", err, wanted)
		}
	}

	if _, runErr := fence.Run(context.Background(), contract.SandboxCommand{Program: "/bin/true"}); runErr == nil {
		t.Error("the fence ran a command through a bwrap that cannot fence anything")
	}
}
