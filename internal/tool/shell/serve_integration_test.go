//go:build integration

package shell_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// TestARealServerIsServedCheckedAndKilled starts a real web server through the
// tool on a port the test chose, reads the kernel's own socket tables to see it
// listen, checks it over HTTP, and kills it, all on the real clock.
func TestARealServerIsServedCheckedAndKilled(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("python3 is needed to serve a folder, and it is not on this machine: %v", err)
	}
	port := aFreePort(t)
	folder := t.TempDir()
	if err := writeFile(filepath.Join(folder, "index.html"), "<h1>served</h1>"); err != nil {
		t.Fatalf("cannot write the page: %v", err)
	}
	tool := shell.New(shell.Settings{
		Sandbox:          runningSandbox{},
		Permission:       testkit.NewFakePermission(contract.RulingAllow),
		Clock:            clock.System(),
		Home:             testkit.NewTempHome(t),
		NerdGenieProgram: filepath.Join(t.TempDir(), "nerdgenie"),
		WorkingDirectory: folder,
		Timeout:          time.Minute,
	})

	served, err := run(t, tool, map[string]any{
		"action":  "serve",
		"command": fmt.Sprintf("python3 -m http.server %d --bind 127.0.0.1", port),
		"port":    port,
	})
	if err != nil {
		t.Fatalf("serving failed: %v", err)
	}
	if served.Text != fmt.Sprintf("listening on %d as p1; check it with action check, stop it with kill\n", port) {
		t.Fatalf("the serve said %q", served.Text)
	}

	checked, err := run(t, tool, map[string]any{"action": "check", "port": port, "path": "/index.html"})
	if err != nil || !strings.HasPrefix(checked.Text, fmt.Sprintf("port %d answers: HTTP 200 in ", port)) {
		t.Errorf("the check said %q, %v; want HTTP 200", checked.Text, err)
	}

	if _, err := run(t, tool, map[string]any{"action": "kill", "id": "p1"}); err != nil {
		t.Fatalf("killing the server failed: %v", err)
	}
	waitUntilFinished(t, tool, "p1")
	gone, err := run(t, tool, map[string]any{"action": "check", "port": port})
	if err != nil || gone.Text != fmt.Sprintf("nothing listens on %d\n", port) {
		t.Errorf("after the kill the check said %q, %v; want nothing listens", gone.Text, err)
	}
}

// writeFile writes one small file.
func writeFile(path string, text string) error {
	return exec.Command("sh", "-c", "printf '%s' \""+text+"\" > "+path).Run()
}
