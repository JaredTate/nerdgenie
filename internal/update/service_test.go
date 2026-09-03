package update_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// fakeService is the systemctl a test puts on the PATH: it writes down what it
// was told, starts whatever the current link points at, and answers is-active
// with what happened. No test ever touches the real service manager.
type fakeService struct {
	// record is the file every call writes its arguments into.
	record string
	// state is the file holding what the service is doing now.
	state string
}

// aMachineWithAService gives a test its own home folder and a systemctl of its
// own on the PATH, and nothing else on it.
func aMachineWithAService(t *testing.T) (contract.Home, *fakeService) {
	t.Helper()
	home := testkit.NewTempHome(t)
	folder := t.TempDir()
	service := &fakeService{
		record: filepath.Join(folder, "told.txt"),
		state:  filepath.Join(folder, "state.txt"),
	}
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> " + service.record + "\n" +
		"if [ -f " + filepath.Join(home.RunFolder(), "drain.json") + " ]; then echo drained >> " + service.record + "; fi\n" +
		"[ \"$1\" = --user ] && shift\n" +
		"case \"$1\" in\n" +
		"  stop) echo inactive > " + service.state + " ;;\n" +
		"  start|restart)\n" +
		"    if " + home.CurrentReleaseLink() + " serve >/dev/null 2>&1; then echo active > " + service.state +
		"; else echo failed > " + service.state + "; fi ;;\n" +
		"  is-active)\n" +
		"    state=$(cat " + service.state + " 2>/dev/null || echo unknown)\n" +
		"    echo \"$state\"\n" +
		"    [ \"$state\" = active ] || exit 3 ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(folder, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fake systemctl failed: %v", err)
	}
	t.Setenv("PATH", folder)
	t.Setenv("COEUS_RECORD", filepath.Join(folder, "program.txt"))
	return home, service
}

// told is every line the fake service manager wrote down.
func (service *fakeService) told(t *testing.T) string {
	t.Helper()
	written, err := os.ReadFile(service.record)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("reading what the service manager was told failed: %v", err)
	}
	return string(written)
}

// running says whether the service came up the last time it was started.
func (service *fakeService) running(t *testing.T) bool {
	t.Helper()
	written, err := os.ReadFile(service.state)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(written)) == "active"
}

// programTold is every argument list the release binaries were run with, which
// is how a test sees the migration step happen.
func programTold(t *testing.T) string {
	t.Helper()
	written, err := os.ReadFile(os.Getenv("COEUS_RECORD"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("reading what the release program was told failed: %v", err)
	}
	return string(written)
}
