package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// FuzzTheFenceNeverBindsAForbiddenFolder throws any root, any program, and any
// arguments at the fence and asserts the two things that must always hold: it
// does not panic, and nothing it binds read-write is, or sits inside, a path
// that must stay outside the sandbox.
func FuzzTheFenceNeverBindsAForbiddenFolder(f *testing.F) {
	userHome := f.TempDir()
	for _, folder := range []string{
		filepath.Join(userHome, contract.HomeFolderName, "browser"),
		filepath.Join(userHome, ".ssh"),
		filepath.Join(userHome, "work"),
	} {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			f.Fatalf("cannot make the folder %s for the fuzz test: %v", folder, err)
		}
	}
	helper := filepath.Join(userHome, "work", "coeus")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		f.Fatalf("cannot write the stand-in helper program: %v", err)
	}
	forbidden := contract.ExcludedFromSandbox(userHome, "")

	f.Add("work", "/bin/sh", "-c", "")
	f.Add(".ssh", "/bin/cat", "id_rsa", "")
	f.Add(contract.HomeFolderName, "/bin/cat", "vault.age", "")
	f.Add(contract.HomeFolderName+"/browser", "/bin/ls", "", "")
	f.Add("work/../..", "/bin/true", "", "/etc")
	f.Add("work/deep/deeper", "", "", "")

	f.Fuzz(func(t *testing.T, suffix string, program string, argument string, workingDirectory string) {
		root := filepath.Join(userHome, suffix)
		if len(root) > 200 || !strings.HasPrefix(root, userHome+string(filepath.Separator)) {
			return
		}
		if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
			return
		}

		fence, err := New(Settings{Roots: []string{root}, UserHome: userHome, HelperProgram: helper})
		if err != nil {
			return
		}
		plan, err := fence.planFor(contract.SandboxCommand{
			Program:          program,
			Arguments:        []string{argument},
			WorkingDirectory: workingDirectory,
		})
		if err != nil {
			return
		}
		checkBinds(t, buildArguments(plan), plan, forbidden)
	})
}

// checkBinds asserts that the command line binds read-write only the sandbox
// roots, and binds read-only only the system folders and the helper program.
func checkBinds(t *testing.T, arguments []string, plan fencePlan, forbidden []string) {
	t.Helper()
	for index, argument := range arguments {
		if index+1 >= len(arguments) {
			break
		}
		source := arguments[index+1]
		switch argument {
		case "--bind":
			if !slices.Contains(plan.roots, source) {
				t.Errorf("the fence binds %q read-write, and only a checked sandbox root may be bound read-write", source)
			}
			for _, outside := range forbidden {
				if source == outside || strings.HasPrefix(outside, source+string(filepath.Separator)) ||
					strings.HasPrefix(source, outside+string(filepath.Separator)) {
					t.Errorf("the fence binds %q read-write, and %q must stay outside the sandbox", source, outside)
				}
			}
		case "--ro-bind":
			if !slices.Contains(plan.systemFolders, source) && source != plan.helperProgram && source != plan.resolverFile {
				t.Errorf("the fence binds %q read-only, and only the system folders, the helper program, and the resolver settings may be bound read-only", source)
			}
		}
	}
}

// FuzzTheEnvironmentAllowListNeverLosesItsFirstTwoEntries throws any text at the
// environment builder and asserts that the PATH and the home folder are always
// there, so that no caller can drop them by passing something odd.
func FuzzTheEnvironmentAllowListNeverLosesItsFirstTwoEntries(f *testing.F) {
	f.Add("/home/example/scratch", "en_US.UTF-8", "xterm", "NAME=value")
	f.Add("", "", "", "")

	f.Fuzz(func(t *testing.T, scratchHome string, language string, terminal string, extra string) {
		environment := allowedEnvironment(scratchHome, language, terminal, []string{extra})

		if len(environment) < 3 {
			t.Fatalf("the environment is %v, and it must always hold the PATH, the home folder, and what the caller passed", environment)
		}
		if environment[0] != "PATH="+sandboxPath {
			t.Errorf("the first entry is %q, want the sandbox PATH", environment[0])
		}
		if environment[1] != "HOME="+scratchHome {
			t.Errorf("the second entry is %q, want the scratch home folder", environment[1])
		}
		if environment[len(environment)-1] != extra {
			t.Errorf("the last entry is %q, want what the caller passed", environment[len(environment)-1])
		}
	})
}
