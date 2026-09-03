package update_test

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// fakeService is the systemctl a test puts on the PATH: it writes down what it
// was told, starts whatever the current link points at, and remembers whether
// that program came up. No test ever touches the real service manager.
type fakeService struct {
	// record is the file every call writes its arguments into.
	record string
	// state is the file holding what the service is doing now.
	state string
	// refusal is the file whose presence makes every call fail, which is how a
	// test sees what happens when the service manager will not do as it is told.
	refusal string
}

// aShortHome makes the whole home folder layout under a short path, because a
// Unix socket path is far shorter than a file path may be and the readiness
// check opens the socket in the run folder.
func aShortHome(t *testing.T) contract.Home {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-update")
	if err != nil {
		t.Fatalf("cannot make a folder for the home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })
	t.Setenv("HOME", folder)

	home := contract.NewHome(filepath.Join(folder, contract.HomeFolderName))
	for _, one := range home.Folders() {
		if err := os.MkdirAll(one, contract.HomeFolderMode); err != nil {
			t.Fatalf("cannot make the folder %s: %v", one, err)
		}
	}
	return home
}

// aMachineWithAService gives a test its own home folder, its own systemctl on
// the PATH and nothing else on it, and an agent on the local socket that answers
// the readiness check exactly while the service is up.
func aMachineWithAService(t *testing.T) (contract.Home, *fakeService) {
	t.Helper()
	home := aShortHome(t)
	folder := t.TempDir()
	service := &fakeService{
		record:  filepath.Join(folder, "told.txt"),
		state:   filepath.Join(folder, "state.txt"),
		refusal: filepath.Join(folder, "refuse.txt"),
	}
	writeFakeServiceManager(t, home, service, folder)
	startFakeAgent(t, home, service)
	t.Setenv("PATH", folder)
	t.Setenv("COEUS_RECORD", filepath.Join(folder, "program.txt"))
	return home, service
}

// writeFakeServiceManager writes the systemctl the test puts on its PATH. It
// uses nothing but shell builtins, because that PATH holds only itself.
func writeFakeServiceManager(t *testing.T, home contract.Home, service *fakeService, folder string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> " + service.record + "\n" +
		"if [ -f " + filepath.Join(home.RunFolder(), "drain.json") + " ]; then echo drained >> " + service.record + "; fi\n" +
		"if [ -f " + service.refusal + " ]; then echo 'the service manager refused' >&2; exit 1; fi\n" +
		"[ \"$1\" = --user ] && shift\n" +
		"case \"$1\" in\n" +
		"  stop) echo inactive > " + service.state + " ;;\n" +
		"  start|restart)\n" +
		"    if " + home.CurrentReleaseLink() + " serve >/dev/null 2>&1; then echo active > " + service.state +
		"; else echo failed > " + service.state + "; fi ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(folder, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fake systemctl failed: %v", err)
	}
}

// startFakeAgent listens on the home's local socket and answers whatever is
// asked of it while the service is up, which is what the readiness check looks
// for. A service that is not up is a socket that hangs up without answering,
// which is what a version that died the moment it started looks like from
// outside.
func startFakeAgent(t *testing.T, home contract.Home, service *fakeService) {
	t.Helper()
	listener, err := net.Listen("unix", home.SocketFile())
	if err != nil {
		t.Fatalf("listening on the agent's socket at %s failed: %v", home.SocketFile(), err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			if service.isUp() {
				_ = contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
					Type: contract.SocketReply, Text: "ready",
				})
			}
			_ = connection.Close()
		}
	}()
}

// isUp says whether the last start left the service running.
func (service *fakeService) isUp() bool {
	written, err := os.ReadFile(service.state)
	return err == nil && strings.TrimSpace(string(written)) == "active"
}

// refuse makes every later call to the service manager fail, the way a manager
// that has lost the unit does.
func (service *fakeService) refuse(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(service.refusal, []byte("no"), 0o644); err != nil {
		t.Fatalf("telling the fake service manager to refuse failed: %v", err)
	}
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
	return service.isUp()
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
