//go:build integration

package sandbox

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// fetchMode is the word this test binary answers to when the fence is asked to
// run it as a command that reaches a web server. Using the test binary means the
// network test needs no program installed on the machine.
const fetchMode = "fetch-a-page"

// TestMain lets this test binary stand in for the coeus binary. The fence starts
// its helper as "<the coeus binary> sandbox-entry ...", and on this branch the
// orchestrator has not yet added that subcommand to cmd/coeus/main.go, so the
// test binary answers to the same word. It is the same function either way.
func TestMain(m *testing.M) {
	switch {
	case len(os.Args) > 1 && os.Args[1] == EntrySubcommandName:
		os.Exit(runTheHelper())
	case len(os.Args) > 2 && os.Args[1] == fetchMode:
		os.Exit(fetchOnePage(os.Args[2]))
	}
	os.Exit(m.Run())
}

// runTheHelper is what cmd/coeus/sandbox_entry.go does, written here so that the
// integration tests can run the real helper before the orchestrator registers
// the subcommand.
func runTheHelper() int {
	if err := Entry(os.Args[2:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "coeus %s: %v\n", EntrySubcommandName, err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// fetchOnePage reads one address and prints what came back, which is what the
// network test runs inside the fence.
func fetchOnePage(address string) int {
	answer, err := http.Get(address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot reach %s: %v\n", address, err)
		return contract.ExitFailure
	}
	defer func() { _ = answer.Body.Close() }()

	page, err := io.ReadAll(io.LimitReader(answer.Body, 1<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read what %s sent: %v\n", address, err)
		return contract.ExitFailure
	}
	fmt.Print(string(page))
	return contract.ExitOK
}

// standInScript is a stand-in for bwrap, used only on a machine that does not
// allow an unprivileged user namespace. It carries out the parts of the command
// line the fence's own behaviour depends on, so that the Landlock ruleset and the
// seccomp filter are still applied for real, and ignores the mounting, which is
// the part the machine will not let it do.
const standInScript = `#!/bin/sh
while [ "$#" -gt 0 ]; do
	case "$1" in
	--) shift; break ;;
	--setenv) export "$2=$3"; shift 3 ;;
	--chdir) cd "$2" || exit 1; shift 2 ;;
	--bind|--ro-bind) shift 3 ;;
	--proc|--dev|--tmpfs) shift 2 ;;
	*) shift ;;
	esac
done
exec "$@"
`

// The answer to whether the real bwrap can build a fence on this machine, asked
// once, because asking it starts a process.
var (
	bubblewrapProbe  sync.Once
	bubblewrapWorks  bool
	bubblewrapReason string
)

// realBubblewrapCanMakeAFence says whether bwrap can really make a user
// namespace here. Ubuntu's AppArmor refuses one to an unconfined program unless
// a profile allows it, and there is no way around that without root.
func realBubblewrapCanMakeAFence() (bool, string) {
	bubblewrapProbe.Do(func() {
		probe := exec.Command(bubblewrapProgram, "--unshare-user", "--ro-bind", "/", "/", "/bin/true")
		output, err := probe.CombinedOutput()
		bubblewrapWorks = err == nil
		bubblewrapReason = fmt.Sprintf("%v: %s", err, output)
	})
	return bubblewrapWorks, bubblewrapReason
}

// useAFenceProgram makes sure something answers to "bwrap" on the PATH: the real
// one when this machine allows it, and the stand-in when it does not.
func useAFenceProgram(t *testing.T) {
	t.Helper()
	if works, _ := realBubblewrapCanMakeAFence(); works {
		return
	}

	folder := t.TempDir()
	standIn := filepath.Join(folder, bubblewrapProgram)
	if err := os.WriteFile(standIn, []byte(standInScript), 0o755); err != nil {
		t.Fatalf("cannot write the stand-in for %s: %v", bubblewrapProgram, err)
	}
	t.Setenv("PATH", folder+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// aRealFence makes a temporary home holding the agent's own folder, an SSH
// folder with a fixture key in it, and one work folder as the sandbox root, and
// returns a fence whose helper is this test binary.
func aRealFence(t *testing.T, outputCap int) (*Fence, string, string) {
	t.Helper()
	home := testkit.NewTempHome(t)
	userHome := filepath.Dir(home.Root)
	useAFenceProgram(t)

	sshFolder := filepath.Join(userHome, ".ssh")
	work := filepath.Join(userHome, "work")
	for _, folder := range []string{sshFolder, work} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", folder, err)
		}
	}
	if err := os.WriteFile(filepath.Join(sshFolder, "id_fixture"), []byte(fixtureKey), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the fixture key: %v", err)
	}

	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this test binary on disk: %v", err)
	}
	fence, err := New(Settings{Roots: []string{work}, UserHome: userHome, OutputCap: outputCap, HelperProgram: thisProgram})
	if err != nil {
		t.Fatalf("cannot build a fence around %s: %v", work, err)
	}
	return fence, userHome, work
}

// fixtureKey stands in for a private key. It is not one, and nothing anywhere
// reads it; the tests only prove that a sandboxed command cannot.
const fixtureKey = "this is not a key, and a sandboxed command must not be able to read it\n"
