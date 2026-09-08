package contract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// headlessLauncher is the wrapper script found at /usr/local/bin/google-chrome
// on jared-rosie on 7 September 2026, which started every Chrome with no window.
const headlessLauncher = "#!/bin/sh\nexec nice -n 15 taskset -c 0-15 /usr/bin/google-chrome-stable --headless=new --disable-gpu --disable-gpu-compositing \"$@\"\n"

// writeProgram puts one executable file with that text into the folder and
// returns its path.
func writeProgram(t *testing.T, folder string, name string, text string) string {
	t.Helper()
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
		t.Fatalf("cannot write the pretend program %s: %v", name, err)
	}
	return path
}

func TestFindChromePrefersTheRealBinaryOverAScriptThatForcesHeadless(t *testing.T) {
	for _, arrangement := range []struct {
		name     string
		realName string
	}{
		{name: "the real binary comes first in the order", realName: "google-chrome-stable"},
		{name: "the launcher comes first in the order", realName: "chromium"},
	} {
		t.Run(arrangement.name, func(t *testing.T) {
			folder := t.TempDir()
			launcher := writeProgram(t, folder, "google-chrome", headlessLauncher)
			real := writeProgram(t, folder, arrangement.realName, "#!/bin/sh\nexec /opt/google/chrome/chrome \"$@\"\n")
			t.Setenv("PATH", folder)

			path, passedOver, err := contract.FindChrome()
			if err != nil {
				t.Fatalf("finding Chrome failed: %v", err)
			}
			if path != real {
				t.Errorf("Chrome was found at %q, want the real binary at %q rather than the launcher that forces headless", path, real)
			}
			for _, over := range passedOver {
				if over != launcher {
					t.Errorf("%q was passed over, and the only launcher that forces headless is %q", over, launcher)
				}
			}
			if arrangement.realName == "chromium" && len(passedOver) != 1 {
				t.Errorf("the launcher at %q was reached before the real binary and was not named as passed over: %v", launcher, passedOver)
			}
		})
	}
}

func TestFindChromeNamesEveryCandidateWhenNoneIsLeft(t *testing.T) {
	folder := t.TempDir()
	launcher := writeProgram(t, folder, "google-chrome", headlessLauncher)
	t.Setenv("PATH", folder)

	path, passedOver, err := contract.FindChrome()
	if err == nil {
		t.Fatalf("Chrome was found at %q, and the only one on the PATH is a launcher that forces headless", path)
	}
	if path != "" {
		t.Errorf("a path %q came back with the error, want none", path)
	}
	if len(passedOver) != 1 || passedOver[0] != launcher {
		t.Errorf("the launchers passed over are %v, want the one at %q", passedOver, launcher)
	}
	for _, candidate := range []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser", launcher, "headless", "install"} {
		if !strings.Contains(err.Error(), candidate) {
			t.Errorf("the error reads %q and does not mention %q", err, candidate)
		}
	}
}

// TestFindChromeLooksPastALauncherToTheSameNameLaterOnThePath: on jared-rosie
// on 7 September 2026 both google-chrome and google-chrome-stable in
// /usr/local/bin were launcher scripts forcing headless, and the real
// google-chrome-stable sat behind them in /usr/bin. Asking the PATH for the
// first match by name never reached it, so the doctor said the browser tools
// were switched off on a machine with Chrome installed.
func TestFindChromeLooksPastALauncherToTheSameNameLaterOnThePath(t *testing.T) {
	front, back := t.TempDir(), t.TempDir()
	launcher := writeProgram(t, front, "google-chrome-stable", headlessLauncher)
	writeProgram(t, front, "google-chrome", headlessLauncher)
	real := writeProgram(t, back, "google-chrome-stable", "#!/bin/sh\nexec /opt/google/chrome/chrome \"$@\"\n")
	t.Setenv("PATH", front+string(os.PathListSeparator)+back)

	path, passedOver, err := contract.FindChrome()
	if err != nil {
		t.Fatalf("finding Chrome failed: %v", err)
	}
	if path != real {
		t.Errorf("Chrome was found at %q, want the real one at %q behind the launcher on the PATH", path, real)
	}
	if len(passedOver) != 1 || passedOver[0] != launcher {
		t.Errorf("passed over %v, want only the launcher at %q that hid the real one", passedOver, launcher)
	}
}
