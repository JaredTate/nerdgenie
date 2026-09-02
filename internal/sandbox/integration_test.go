//go:build integration

package sandbox

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
func TestMain(tests *testing.M) {
	switch {
	case len(os.Args) > 1 && os.Args[1] == EntrySubcommandName:
		os.Exit(runTheHelper())
	case len(os.Args) > 2 && os.Args[1] == fetchMode:
		os.Exit(fetchOnePage(os.Args[2]))
	}
	os.Exit(tests.Run())
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

// aTemporaryUserHome makes a home directory holding the agent's own folder, an
// SSH folder with a fixture key in it, and one work folder, and returns the home
// directory and the work folder.
func aTemporaryUserHome(t *testing.T) (string, string) {
	t.Helper()
	home := testkit.NewTempHome(t)
	userHome := filepath.Dir(home.Root)

	sshFolder := filepath.Join(userHome, ".ssh")
	work := filepath.Join(userHome, contract.WorkFolderName)
	for _, folder := range []string{sshFolder, work} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", folder, err)
		}
	}
	if err := os.WriteFile(filepath.Join(sshFolder, "id_fixture"), []byte(fixtureKey), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the fixture key: %v", err)
	}
	return userHome, work
}

// aRealFenceAround builds a fence around one sandbox root, with this test binary
// as the helper the fence starts inside itself.
//
// Every test that uses it runs against the real bwrap and the real Landlock.
// When this machine will not allow a fence, the test stops here with the reason
// rather than going on to fail somewhere less obvious.
func aRealFenceAround(t *testing.T, userHome string, root string, outputCap int) *Fence {
	t.Helper()
	thisProgram, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot find this test binary on disk: %v", err)
	}
	fence, err := New(Settings{Roots: []string{root}, UserHome: userHome, OutputCap: outputCap, HelperProgram: thisProgram})
	if err != nil {
		t.Fatalf("cannot build a fence around %s: %v", root, err)
	}
	if err := fence.Available(); err != nil {
		t.Fatalf("this machine cannot make a fence, so none of these tests can run: %v", err)
	}
	return fence
}

// aRealFence makes a temporary home with one work folder as the sandbox root and
// returns a fence around it, the home directory, and the work folder.
func aRealFence(t *testing.T, outputCap int) (*Fence, string, string) {
	t.Helper()
	userHome, work := aTemporaryUserHome(t)
	return aRealFenceAround(t, userHome, work, outputCap), userHome, work
}

// fixtureKey stands in for a private key. It is not one, and nothing anywhere
// reads it; the tests only prove that a sandboxed command cannot.
const fixtureKey = "this is not a key, and a sandboxed command must not be able to read it\n"
