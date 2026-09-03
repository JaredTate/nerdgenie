package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theFixturePlan is a fence that does not depend on the machine the test runs
// on, so that its command line can be compared with a golden file.
func theFixturePlan() fencePlan {
	return fencePlan{
		systemFolders:    []string{"/usr", "/bin", "/lib", "/lib64", "/etc"},
		resolverFile:     "/run/systemd/resolve/stub-resolv.conf",
		roots:            []string{"/home/example/work", "/home/example/projects"},
		helperProgram:    "/home/example/.coeus/releases/1.0.0/coeus",
		workingDirectory: "/home/example/work",
		environment: allowedEnvironment(
			"/home/example/work/"+scratchHomeName, "en_US.UTF-8", "xterm-256color",
			[]string{"GIT_AUTHOR_NAME=Example"}),
		program:   "/bin/sh",
		arguments: []string{"-c", "echo hello"},
	}
}

func TestTheCommandLineForTheFixtureConfigurationIsWhatTheGoldenFileHolds(t *testing.T) {
	line := append([]string{bubblewrapProgram}, buildArguments(theFixturePlan())...)

	testkit.Golden(t, "fixture-command-line.golden", []byte(strings.Join(line, "\n")+"\n"))
}

func TestTheCommandLineNeverBindsTheHomeFolderOrAnythingForbidden(t *testing.T) {
	arguments := buildArguments(theFixturePlan())

	for index, argument := range arguments {
		if argument != "--bind" {
			continue
		}
		source := arguments[index+1]
		if !slices.Contains(theFixturePlan().roots, source) {
			t.Errorf("the command line binds %q read-write, and only a sandbox root may be bound read-write", source)
		}
	}
}

func TestTheCommandLineSetsTheMarkerThatTellsTheHelperTheFenceStartedIt(t *testing.T) {
	arguments := buildArguments(theFixturePlan())

	for index := 0; index+2 < len(arguments); index++ {
		if arguments[index] == "--setenv" && arguments[index+1] == FenceMarkerVariable {
			return
		}
	}
	t.Fatalf("the command line never sets %s, and the helper refuses to run without it", FenceMarkerVariable)
}

func TestTheCommandLineUnsharesEveryNamespaceTheFenceKeepsToItself(t *testing.T) {
	arguments := buildArguments(theFixturePlan())

	for _, wanted := range []string{"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--die-with-parent", "--new-session", "--clearenv"} {
		if !slices.Contains(arguments, wanted) {
			t.Errorf("the command line is missing %s", wanted)
		}
	}
}

func TestTheCommandLineUnsharesTheNetworkUnlessTheCallerAsksForIt(t *testing.T) {
	arguments := buildArguments(theFixturePlan())

	if !slices.Contains(arguments, "--unshare-net") {
		t.Error("the command line leaves the network alone, so a sandboxed command reaches every service on this machine; " +
			"the fence gets a network namespace of its own unless the caller asks for the network")
	}
}

func TestTheCommandLineLeavesTheNetworkAloneWhenTheCallerAsksForIt(t *testing.T) {
	plan := theFixturePlan()
	plan.network = true

	if slices.Contains(buildArguments(plan), "--unshare-net") {
		t.Error("the caller asked for the network and the command line unshares it anyway, so a build that fetches its dependencies cannot run")
	}
}

func TestTheCommandLineHandsTheHelperTheFoldersItMayReach(t *testing.T) {
	plan := theFixturePlan()
	arguments := buildArguments(plan)

	entry := slices.Index(arguments, EntrySubcommandName)
	if entry < 0 {
		t.Fatalf("the command line never starts the helper subcommand %q", EntrySubcommandName)
	}
	options := arguments[entry+1:]

	for _, folder := range plan.systemFolders {
		if !hasOptionFor(options, readableOption, folder) {
			t.Errorf("the helper is not told it may read %q", folder)
		}
	}
	for _, root := range plan.roots {
		if !hasOptionFor(options, writableOption, root) {
			t.Errorf("the helper is not told it may write %q", root)
		}
	}
	for _, folder := range freshFolders {
		if !hasOptionFor(options, writableOption, folder) {
			t.Errorf("the helper is not told it may write the fresh folder %q", folder)
		}
	}
}

// hasOptionFor says whether the helper's options hold one option and its value,
// next to each other, before the double dash that ends them.
func hasOptionFor(options []string, name string, value string) bool {
	for index := 0; index+1 < len(options); index++ {
		if options[index] == optionsEndMarker {
			return false
		}
		if options[index] == name && options[index+1] == value {
			return true
		}
	}
	return false
}

func TestTheEnvironmentIsTheShortAllowListPlusWhatTheCallerPasses(t *testing.T) {
	environment := allowedEnvironment("/home/example/work/scratch", "en_GB.UTF-8", "screen", []string{"NINJA_STATUS=go"})

	want := []string{
		"PATH=" + sandboxPath,
		"HOME=/home/example/work/scratch",
		"LANG=en_GB.UTF-8",
		"TERM=screen",
		"NINJA_STATUS=go",
	}
	if !slices.Equal(environment, want) {
		t.Errorf("the environment is %v, want %v", environment, want)
	}
}

func TestTheEnvironmentLeavesOutTheInheritedNamesThatAreNotSet(t *testing.T) {
	environment := allowedEnvironment("/home/example/work/scratch", "", "", nil)

	want := []string{"PATH=" + sandboxPath, "HOME=/home/example/work/scratch"}
	if !slices.Equal(environment, want) {
		t.Errorf("the environment is %v, want %v", environment, want)
	}
}

func TestTheFenceRefusesACallerEntryThatWouldReplaceItsOwnPathOrHome(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	for _, entry := range []string{
		"PATH=/tmp/somewhere-else",
		"HOME=/home/example",
		"LD_PRELOAD=/tmp/mine.so",
		"LD_LIBRARY_PATH=/tmp",
	} {
		_, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Environment: []string{entry}})
		if err == nil {
			t.Errorf("the caller passed %q and the fence took it; the last --setenv wins, so the caller's value would replace the fence's own", entry)
			continue
		}
		if !strings.Contains(err.Error(), entry) {
			t.Errorf("the refusal says %q, and it must name the entry it refused", err)
		}
	}
}

func TestTheFenceTakesACallerEntryThatIsNotOneOfItsOwn(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	plan, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Environment: []string{"GIT_AUTHOR_NAME=Example", "LDFLAGS=-s"}})
	if err != nil {
		t.Fatalf("an ordinary environment entry was refused: %v", err)
	}
	if !slices.Contains(plan.environment, "GIT_AUTHOR_NAME=Example") || !slices.Contains(plan.environment, "LDFLAGS=-s") {
		t.Errorf("the environment is %v, want both of the caller's entries in it", plan.environment)
	}
}

func TestTheFencesOwnPathAndHomeAreTheFirstTwoEntriesAndTheOnlyOnes(t *testing.T) {
	fence, work := aFenceForTesting(t)

	plan, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Environment: []string{"NINJA_STATUS=go"}})
	if err != nil {
		t.Fatalf("planning a command failed: %v", err)
	}

	wantPath, wantHome := "PATH="+sandboxPath, "HOME="+filepath.Join(work, scratchHomeName)
	if plan.environment[0] != wantPath || plan.environment[1] != wantHome {
		t.Fatalf("the environment starts %v, want %q then %q", plan.environment[:2], wantPath, wantHome)
	}
	for _, entry := range plan.environment[2:] {
		if strings.HasPrefix(entry, "PATH=") || strings.HasPrefix(entry, "HOME=") {
			t.Errorf("the environment holds a second %q after the fence's own, and bwrap gives the last one to the command", entry)
		}
	}
}

func TestTheFencePlanRefusesACommandWithNoProgram(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	if _, err := fence.planFor(contract.SandboxCommand{}); err == nil {
		t.Fatal("a command with no program was accepted")
	}
}

func TestTheFencePlanRefusesAWorkingDirectoryOutsideEveryRoot(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	_, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", WorkingDirectory: "/etc"})
	if err == nil {
		t.Fatal("a working directory outside every root was accepted")
	}
	if !strings.Contains(err.Error(), "/etc") {
		t.Errorf("the refusal says %q, and it must name the directory it refused", err)
	}
}

func TestTheFencePlanFallsBackToTheFirstRootAsTheWorkingDirectory(t *testing.T) {
	fence, work := aFenceForTesting(t)

	plan, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true"})
	if err != nil {
		t.Fatalf("planning a command with no working directory failed: %v", err)
	}
	if plan.workingDirectory != work {
		t.Errorf("the working directory is %q, want the first root %q", plan.workingDirectory, work)
	}
}

func TestTheFencePlanAcceptsAFolderInsideARootAsTheWorkingDirectory(t *testing.T) {
	fence, work := aFenceForTesting(t)
	inside := filepath.Join(work, "project")
	if err := os.MkdirAll(inside, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder for the test: %v", err)
	}

	plan, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", WorkingDirectory: inside})
	if err != nil {
		t.Fatalf("a working directory inside a root was refused: %v", err)
	}
	if plan.workingDirectory != inside {
		t.Errorf("the working directory is %q, want %q", plan.workingDirectory, inside)
	}
}

func TestTheFencePlanRefusesMoreArgumentsThanTheCap(t *testing.T) {
	fence, _ := aFenceForTesting(t)
	many := make([]string, MaxArguments+1)

	if _, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Arguments: many}); err == nil {
		t.Fatalf("%d arguments were accepted, and the cap is %d", len(many), MaxArguments)
	}
}

func TestTheFencePlanRefusesMoreEnvironmentEntriesThanTheCap(t *testing.T) {
	fence, _ := aFenceForTesting(t)
	many := make([]string, MaxEnvironmentEntries+1)
	for index := range many {
		many[index] = "NAME=value"
	}

	if _, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Environment: many}); err == nil {
		t.Fatalf("%d environment entries were accepted, and the cap is %d", len(many), MaxEnvironmentEntries)
	}
}

func TestTheFencePlanRefusesAnEnvironmentEntryWithNoNameInIt(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	for _, broken := range []string{"no-equals-sign", "=value"} {
		if _, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Environment: []string{broken}}); err == nil {
			t.Errorf("the environment entry %q was accepted, and it has no name in it", broken)
		}
	}
}

func TestTheFencePlanRefusesTextWithAZeroByteInIt(t *testing.T) {
	fence, _ := aFenceForTesting(t)

	if _, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true\x00rm"}); err == nil {
		t.Error("a program name holding a zero byte was accepted, and a zero byte ends a string for the kernel")
	}
	if _, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Arguments: []string{"a\x00b"}}); err == nil {
		t.Error("an argument holding a zero byte was accepted")
	}
	if _, err := fence.planFor(contract.SandboxCommand{Program: "/bin/true", Environment: []string{"NAME=a\x00b"}}); err == nil {
		t.Error("an environment entry holding a zero byte was accepted")
	}
}
