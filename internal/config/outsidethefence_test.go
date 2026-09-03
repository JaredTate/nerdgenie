package config_test

// Design section 11 says the browser profile is always outside the sandbox. The
// paths that must stay outside used to be worked out from the default browser
// folder alone, so a browser_profile_path or a backup_path the user had moved
// into a sandbox root sat inside the fence, where a sandboxed command could read
// the cookies that are the agent's logins. Both configured paths are now handed
// to contract.CheckSandboxRoot, and these two tests hold that.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestASandboxRootThatHoldsTheConfiguredBrowserProfileIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	work := t.TempDir()
	profile := filepath.Join(work, "chrome-profile")

	problem := refusesRootWith(t, home, work, "browser_profile_path = \""+profile+"\"\n")
	if !strings.Contains(problem.Advice, profile) {
		t.Errorf("the advice is %q, want it to name the browser profile %s that must stay outside the sandbox", problem.Advice, profile)
	}
}

func TestASandboxRootThatHoldsTheConfiguredBackupFolderIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	work := t.TempDir()
	backups := filepath.Join(work, "backups")

	problem := refusesRootWith(t, home, work, "backup_path = \""+backups+"\"\n")
	if !strings.Contains(problem.Advice, backups) {
		t.Errorf("the advice is %q, want it to name the backup folder %s that must stay outside the sandbox", problem.Advice, backups)
	}
}

// refusesRootWith writes a configuration whose only sandbox root is the path,
// with the extra lines after it, and returns the problem it produces.
func refusesRootWith(t testing.TB, home contract.Home, root string, extra string) config.Problem {
	t.Helper()
	document := "sandbox_roots = [\"" + root + "\"]\n" + extra
	if err := os.WriteFile(home.ConfigFile(), []byte(document), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration file for the test: %v", err)
	}
	if _, err := config.Load(home); err != nil {
		var problem config.Problem
		if !errors.As(err, &problem) {
			t.Fatalf("the error is %v of type %T, want a config.Problem naming the key", err, err)
		}
		if problem.Key != "sandbox_roots" {
			t.Errorf("the problem names the key %q, want sandbox_roots", problem.Key)
		}
		return problem
	}
	t.Fatalf("the sandbox root %q was accepted with %q in the same file, and a root that holds a path the fence must never bind"+
		" has to be refused", root, strings.TrimSpace(extra))
	return config.Problem{}
}
