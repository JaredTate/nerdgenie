//go:build integration

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

// waitForTheFile waits, for a bounded time, until a file turns up. It is how a
// test knows a shell has got far enough to have set its signal trap, rather than
// signalling it in the instant before it was ready.
func waitForTheFile(t *testing.T, path string) {
	t.Helper()
	for waited := time.Duration(0); waited < 10*time.Second; waited += 10 * time.Millisecond {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the file %s never turned up, so the process never got as far as setting its trap", path)
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

func TestAvailableSaysThisMachineCanRunAFence(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	// The unit tests ask checkAvailability about machines they are not running
	// on. This is the one that runs on the development machine, where bwrap is
	// installed, the kernel reports Landlock, and AppArmor lets bwrap make a
	// user namespace, so the only right answer is no error at all.
	if err := fence.Available(); err != nil {
		t.Fatalf("the sandbox says this machine cannot run a fence: %v", err)
	}
}

func TestACommandNamedWithoutAFullPathIsFoundOnTheFencesPath(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	// This is how the shell tool names its program, and syscall.Exec searches no
	// path of its own, so the helper has to look the name up on the PATH the
	// fence set.
	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program:   "sh",
		Arguments: []string{"-c", "echo found me"},
		Timeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("a command named the way the shell tool names it reported %d and said %q", result.ExitCode, result.StandardError)
	}
	if !strings.Contains(string(result.StandardOutput), "found me") {
		t.Errorf("the output is %q, want what the command printed", result.StandardOutput)
	}
}

func TestACommandThatIsNowhereInsideTheFenceSaysWhereToLook(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program: "no-such-program-inside-the-fence",
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("the fence reported success for a program that is on no machine")
	}
	said := string(result.StandardError)
	if !strings.Contains(said, "full path") || !strings.Contains(said, "PATH") {
		t.Errorf("the fence said %q, and it must say to name a full path or a program on the fence's PATH", said)
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

func TestAProcessThatIgnoresBeingAskedToStopIsKilledOutright(t *testing.T) {
	// Inside a real fence the second signal is rarely reached, because bwrap dies
	// on the first one and the process namespace takes everything inside it
	// along. The stopping still has to work when the first signal is ignored, so
	// it is tried here on a process this test starts itself and on nothing else.
	ready := filepath.Join(t.TempDir(), "the-trap-is-set")
	ignoring := exec.Command("/bin/sh", "-c",
		`trap "" TERM; : > `+ready+`; index=0; while [ $index -lt 4000000000 ]; do index=$((index+1)); done`)
	ignoring.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := ignoring.Start(); err != nil {
		t.Fatalf("cannot start the process that ignores being asked to stop: %v", err)
	}

	finished := make(chan error, 1)
	go func() { finished <- ignoring.Wait() }()
	waitForTheFile(t, ready)

	started := time.Now()
	if !stopProcessGroup(ignoring.Process.Pid, finished) {
		t.Fatal("the process group was never stopped, and a killed process must be reaped")
	}
	if took := time.Since(started); took < gracePeriod {
		t.Errorf("the group stopped after %s, and a process that ignores the first signal must be given the whole %s before the second",
			took, gracePeriod)
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

// theInitialUserNamespaceMap is what /proc/self/uid_map reads like outside every
// user namespace. A command inside the fence must never see this.
const theInitialUserNamespaceMap = "0          0 4294967295"

func TestTheRealFenceGivesTheCommandItsOwnNamespacesAndNothingOfTheUsers(t *testing.T) {
	fence, userHome, _ := aRealFence(t, theToolOutputCap)

	result, err := fence.Run(context.Background(), contract.SandboxCommand{
		Program: "/bin/sh",
		Arguments: []string{"-c",
			"echo pid=$$; echo map=$(cat /proc/self/uid_map); echo home=$(ls -a " + userHome + ")"},
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("running the command failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("the command reported %d and said %q", result.ExitCode, result.StandardError)
	}
	said := string(result.StandardOutput)

	// bwrap puts a reaper at process one inside the new process namespace and
	// runs the command beside it, so a command that could still see the whole
	// machine's process table would report a far larger number than this.
	if !strings.Contains(said, "pid=1\n") && !strings.Contains(said, "pid=2\n") {
		t.Errorf("the command said %q, and inside its own process namespace it should be process one or two", said)
	}
	if strings.Contains(said, theInitialUserNamespaceMap) {
		t.Errorf("the command said %q, and it is still in the machine's own user namespace", said)
	}
	for _, mustNotBeThere := range []string{".ssh", contract.HomeFolderName} {
		if strings.Contains(said, mustNotBeThere) {
			t.Errorf("the command can see %s in the home directory, and it must stay outside the fence: %q", mustNotBeThere, said)
		}
	}
	if !strings.Contains(said, contract.WorkFolderName) {
		t.Errorf("the command cannot see its own sandbox root in %q", said)
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
	// This fence is built by hand rather than through aRealFence, because that
	// helper asks the real bwrap first and the answer is remembered for the life
	// of a fence. A fence that has never asked is the only one this can be tried
	// on.
	work := t.TempDir()
	fence, err := New(Settings{Roots: []string{work}, UserHome: filepath.Dir(work), HelperProgram: "/bin/true"})
	if err != nil {
		t.Fatalf("cannot build a fence around %s: %v", work, err)
	}

	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, bubblewrapProgram), []byte(failingBubblewrapScript), 0o755); err != nil {
		t.Fatalf("cannot write the stand-in for %s: %v", bubblewrapProgram, err)
	}
	t.Setenv("PATH", folder+string(os.PathListSeparator)+os.Getenv("PATH"))

	refused := fence.Available()
	if refused == nil {
		t.Fatal("the sandbox reported itself available with a bwrap that cannot make a user namespace")
	}
	said := strings.ToLower(refused.Error())
	for _, wanted := range []string{"apparmor", "userns"} {
		if !strings.Contains(said, wanted) {
			t.Errorf("the reason says %q, and it must name %q so that a person knows what to change", refused, wanted)
		}
	}

	if _, runErr := fence.Run(context.Background(), contract.SandboxCommand{Program: "/bin/true"}); runErr == nil {
		t.Error("the fence ran a command through a bwrap that cannot fence anything")
	}
}
