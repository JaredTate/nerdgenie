package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pinned Node runtime is the one thing a release fetches from outside this
// repository, so these tests pin down what it does when the network is not there
// and what it does when what came back is not what was pinned.

// nodeScript copies scripts/release/node.sh into a temporary folder, along with
// a fake curl that always fails, so that no test in this package can reach the
// network by accident.
func nodeScript(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	fakes := filepath.Join(root, "bin")
	writeExecutable(t, filepath.Join(fakes, "curl"),
		"#!/bin/sh\necho 'the fake curl refuses: no test may reach the network' >&2\nexit 7\n")
	script := copyScript(t, filepath.Join(root, "scripts"), "scripts/release/node.sh")
	return script, filepath.Join(root, "cache"), fakes
}

func TestTheNodeRuntimeInTheCacheIsUsedWithoutDownloadingItAgain(t *testing.T) {
	script, cache, fakes := nodeScript(t)
	version := pinnedNodeVersion(t)
	unpacked := filepath.Join(cache, "node-"+version+"-amd64")
	writeExecutable(t, filepath.Join(unpacked, "bin", "node"), "#!/bin/sh\nexit 0\n")
	environment := cleanEnvironment(t.TempDir(), fakes, "COEUS_NODE_CACHE="+cache)

	printed, code := runScript(t, script, filepath.Dir(script), environment, "amd64")

	if code != 0 {
		t.Fatalf("a runtime already in the cache was not used: exit %d, and it printed:\n%s", code, printed)
	}
	if got := strings.TrimSpace(printed); got != unpacked {
		t.Errorf("node.sh printed %q, want the cached runtime folder %q", got, unpacked)
	}
}

func TestTheNodeRuntimeIsRefusedWhenItsChecksumDoesNotMatch(t *testing.T) {
	script, cache, fakes := nodeScript(t)
	version := pinnedNodeVersion(t)
	downloaded := filepath.Join(cache, "node-v"+version+"-linux-x64.tar.xz")
	writeFile(t, downloaded, "this is not the Node runtime anybody pinned")
	environment := cleanEnvironment(t.TempDir(), fakes, "COEUS_NODE_CACHE="+cache)

	printed, code := runScript(t, script, filepath.Dir(script), environment, "amd64")

	if code == 0 {
		t.Fatalf("a Node runtime whose checksum was wrong was accepted. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, "checksum", "the refusal says what did not match")
	requirePrinted(t, printed, downloaded, "the refusal names the file to delete")
	if _, err := os.Stat(filepath.Join(cache, "node-"+version+"-amd64", "bin", "node")); err == nil {
		t.Error("a runtime whose checksum was wrong was unpacked anyway")
	}
}

func TestTheNodeRuntimeRefusesAnArchitectureItHasNoPinFor(t *testing.T) {
	script, cache, fakes := nodeScript(t)
	environment := cleanEnvironment(t.TempDir(), fakes, "COEUS_NODE_CACHE="+cache)

	printed, code := runScript(t, script, filepath.Dir(script), environment, "sparc")

	if code == 0 {
		t.Fatalf("an architecture with no pinned runtime was accepted. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, "sparc", "the refusal names the architecture it was asked for")
}

func TestTheNodeRuntimeSaysWhatToDoWhenTheDownloadFails(t *testing.T) {
	script, cache, fakes := nodeScript(t)
	environment := cleanEnvironment(t.TempDir(), fakes, "COEUS_NODE_CACHE="+cache)

	printed, code := runScript(t, script, filepath.Dir(script), environment, "amd64")

	if code == 0 {
		t.Fatalf("a download that failed was treated as a success. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, "nodejs.org", "the failure says where the runtime comes from")
}
