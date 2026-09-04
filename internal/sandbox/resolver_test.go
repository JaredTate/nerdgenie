package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aMachineWithAResolverLink builds a settings folder holding a resolv.conf that
// is a link into a folder the fence does not bind, which is how Ubuntu with
// systemd-resolved is laid out, and returns the link and the file it leads to.
func aMachineWithAResolverLink(t *testing.T) (string, string) {
	t.Helper()
	machine := t.TempDir()
	settings := filepath.Join(machine, "etc")
	elsewhere := filepath.Join(machine, "run", "systemd", "resolve")
	for _, folder := range []string{settings, elsewhere} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", folder, err)
		}
	}

	realFile := filepath.Join(elsewhere, "stub-resolv.conf")
	if err := os.WriteFile(realFile, []byte("nameserver 127.0.0.53\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the resolver settings: %v", err)
	}
	link := filepath.Join(settings, "resolv.conf")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("cannot make the resolver link: %v", err)
	}
	return link, realFile
}

func TestTheResolverFileIsTheOneTheLinkLeadsToWhenItLeavesTheBoundFolders(t *testing.T) {
	link, realFile := aMachineWithAResolverLink(t)

	found := resolverFileToBind(link, []string{filepath.Dir(link)})

	if found != realFile {
		t.Errorf("the fence binds %q for the resolver settings, want %q; without it every name lookup inside the fence fails", found, realFile)
	}
}

func TestNothingExtraIsBoundWhenTheResolverFileIsAlreadyInsideABoundFolder(t *testing.T) {
	settings := t.TempDir()
	plainFile := filepath.Join(settings, "resolv.conf")
	if err := os.WriteFile(plainFile, []byte("nameserver 192.0.2.1\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the resolver settings: %v", err)
	}

	if found := resolverFileToBind(plainFile, []string{settings}); found != "" {
		t.Errorf("the fence binds %q as well as the folder it already sits inside, and one bind is enough", found)
	}
}

func TestNothingIsBoundWhenTheResolverLinkLeadsNowhere(t *testing.T) {
	settings := t.TempDir()
	link := filepath.Join(settings, "resolv.conf")
	if err := os.Symlink(filepath.Join(settings, "gone"), link); err != nil {
		t.Fatalf("cannot make the resolver link: %v", err)
	}

	if found := resolverFileToBind(link, []string{settings}); found != "" {
		t.Errorf("the fence binds %q, which is not there, and bwrap stops before it starts when a bind names a missing file", found)
	}
}

func TestNothingIsBoundWhenTheMachineHasNoResolverFileAtAll(t *testing.T) {
	if found := resolverFileToBind(filepath.Join(t.TempDir(), "resolv.conf"), nil); found != "" {
		t.Errorf("the fence binds %q on a machine that has no resolver settings at all", found)
	}
}

func TestTheCommandLineBindsTheResolverFileReadOnlyAndLetsTheHelperReadIt(t *testing.T) {
	plan := theFixturePlan()
	arguments := buildArguments(plan)

	bound := false
	for index := 0; index+2 < len(arguments); index++ {
		if arguments[index] == "--ro-bind" && arguments[index+1] == plan.resolverFile && arguments[index+2] == plan.resolverFile {
			bound = true
		}
	}
	if !bound {
		t.Errorf("the command line never binds the resolver settings at %q, so a name lookup inside the fence has nothing to read", plan.resolverFile)
	}

	entry := slices.Index(arguments, EntrySubcommandName)
	if entry < 0 {
		t.Fatalf("the command line never starts the helper subcommand %q", EntrySubcommandName)
	}
	if !hasOptionFor(arguments[entry+1:], readableOption, plan.resolverFile) {
		t.Errorf("the helper is not told it may read the resolver settings at %q, so Landlock denies the read that the bind allows", plan.resolverFile)
	}
}

func TestAFenceFindsTheResolverFileThisMachineReallyUses(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	// Whatever this machine's layout is, the fence must agree with the same
	// question asked of the machine directly, so that neither a link into /run
	// nor a plain file in /etc is a surprise.
	want := resolverFileToBind(resolverFilePath, fence.systemFolders)
	if fence.resolverFile != want {
		t.Errorf("the fence binds %q for name lookups, want %q", fence.resolverFile, want)
	}
}
