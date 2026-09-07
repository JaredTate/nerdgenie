package shell_test

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aFreePort is a port on the loopback interface that nothing listens on.
func aFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot find a free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

// aPageServer serves one page at /index.html on a loopback port and hands the
// port back; everything else is not found.
func aPageServer(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.html" {
			_, _ = w.Write([]byte("<h1>hi</h1>"))
			return
		}
		http.NotFound(w, r)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}

// TestACheckSaysWhetherAPortAnswers: one task ran the same curl health probe
// twenty-six times, a round each. A check is the probe with no shell in it.
func TestACheckSaysWhetherAPortAnswers(t *testing.T) {
	port := aPageServer(t)
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"action": "check", "port": port, "path": "/index.html"})
	if err != nil {
		t.Fatalf("checking a listening port failed: %v", err)
	}
	if !strings.HasPrefix(output.Text, fmt.Sprintf("port %d answers: HTTP 200 in ", port)) || !strings.HasSuffix(output.Text, " ms\n") {
		t.Errorf("the check said %q, want the status and the milliseconds", output.Text)
	}

	output, err = run(t, tool, map[string]any{"action": "check", "port": port, "path": "/missing"})
	if err != nil || !strings.HasPrefix(output.Text, fmt.Sprintf("port %d answers: HTTP 404 in ", port)) {
		t.Errorf("a missing page said %q, %v; want HTTP 404", output.Text, err)
	}

	output, err = run(t, tool, map[string]any{"action": "check", "port": port})
	if err != nil || !strings.HasPrefix(output.Text, fmt.Sprintf("port %d answers: a connection opened in ", port)) {
		t.Errorf("a bare check said %q, %v; want the connection", output.Text, err)
	}
}

func TestACheckOfAClosedPortSaysNothingListens(t *testing.T) {
	port := aFreePort(t)
	tool := newTool(t, testkit.NewFakeSandbox(), testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))

	output, err := run(t, tool, map[string]any{"action": "check", "port": port, "path": "/"})
	if err != nil {
		t.Fatalf("checking a closed port failed as a call: %v", err)
	}
	if output.Text != fmt.Sprintf("nothing listens on %d\n", port) {
		t.Errorf("a closed port said %q", output.Text)
	}
}

func TestACheckNeedsAPortAndRunsNothing(t *testing.T) {
	sandbox := testkit.NewFakeSandbox()
	tool := newTool(t, sandbox, testkit.NewFakePermission(contract.RulingAllow), testkit.NewFakeClock(theMoment))
	for _, fields := range []map[string]any{
		{"action": "check"},
		{"action": "check", "port": 0},
		{"action": "check", "port": 70000},
	} {
		if _, err := run(t, tool, fields); err == nil || !strings.Contains(err.Error(), "port") {
			t.Errorf("%v was not refused for its port: %v", fields, err)
		}
	}
	if _, err := run(t, tool, map[string]any{"action": "check", "port": aFreePort(t)}); err != nil {
		t.Fatalf("a check failed as a call: %v", err)
	}
	if len(sandbox.Commands()) != 0 {
		t.Errorf("a check ran %d commands through the sandbox, want none", len(sandbox.Commands()))
	}
}
