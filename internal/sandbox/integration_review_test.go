//go:build integration

package sandbox

// What the wave 6 security review got past the fence. Every test here runs
// against the real bwrap, the real Landlock, and the real seccomp filter, under
// a temporary home, and none of them touches the user's own ~/.nerdgenie or ~/.ssh.

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// insideTheFence runs one line of shell inside a real fence and returns what it
// wrote on both streams, so that a probe reads as the one line it is.
func insideTheFence(t *testing.T, fence *Fence, command string) string {
	t.Helper()
	ctx, stopWaiting := context.WithTimeout(context.Background(), 40*time.Second)
	defer stopWaiting()

	result, err := fence.Run(ctx, contract.SandboxCommand{
		Program:   "/bin/sh",
		Arguments: []string{"-c", command},
		Timeout:   35 * time.Second,
	})
	if err != nil {
		t.Fatalf("the fence could not run %q: %v", command, err)
	}
	return strings.TrimSpace(string(result.StandardOutput)) + strings.TrimSpace(string(result.StandardError))
}

// theLimitsACommandMustHave are the bounds that stop one sandboxed command from
// taking the whole machine down. Design section 11 says every loop has a limit
// and every buffer a cap; a fence that leaves the machine's own bounds exactly
// as they are has set neither, and one line of shell inside it can wedge the
// machine the agent runs on.
var theLimitsACommandMustHave = []struct {
	name string
	line string
}{
	{"how many processes it may start", "process"},
}

// The bound on memory is deliberately not in the list above: Node's engine and
// Go's runtime reserve tens of gigabytes of virtual space they never touch, and
// a four-gigabyte address-space bound made a test runner abort inside the fence
// on the first live run. The process count and the two folder sizes bound what
// one command can take; memory itself is bounded by the machine.

func TestASandboxedCommandCannotExhaustTheMachine(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)
	inside := insideTheFence(t, fence, "ulimit -a")
	outside := onThisMachine(t, "ulimit -a")

	for _, limit := range theLimitsACommandMustHave {
		fenced, machine := limitLine(inside, limit.line), limitLine(outside, limit.line)
		if fenced == "" {
			t.Fatalf("%s: the fence said %q rather than a list of limits", limit.name, inside)
		}
		if strings.Contains(fenced, "unlimited") || fenced == machine {
			t.Errorf("%s: inside the fence it is %q and outside it is %q, so the fence set no bound of its own"+
				" and one command inside it can take the whole machine down; give the fence a bound", limit.name, fenced, machine)
		}
	}
}

// limitLine picks one limit out of what "ulimit -a" printed.
func limitLine(said string, name string) string {
	for _, line := range strings.Split(said, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), name) {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	return ""
}

// onThisMachine asks the same question of a shell outside the fence, so that a
// bound the fence set can be told from the machine's own.
func onThisMachine(t *testing.T, question string) string {
	t.Helper()
	said, err := exec.Command("/bin/sh", "-c", question).Output()
	if err != nil {
		t.Fatalf("cannot ask this machine %q: %v", question, err)
	}
	return strings.TrimSpace(string(said))
}

// mostTemporaryKilobytes is how large the fence's own /tmp may be. A hundred
// megabytes is more than any tool needs for a scratch file and small enough that
// filling it cannot take the machine's memory with it.
const mostTemporaryKilobytes = 100 * 1024

func TestTheFencesOwnTemporaryFolderHasASizeOnIt(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	// bwrap is asked for "--tmpfs /tmp" with no size, and a tmpfs with no size
	// is half of this machine's memory, which a command inside the fence can
	// fill a byte at a time.
	said := insideTheFence(t, fence, "df -k /tmp 2>/dev/null | awk 'NR==2 {print $2}'")
	kilobytes, err := strconv.Atoi(strings.TrimSpace(said))
	if err != nil {
		t.Fatalf("the fence said %q rather than the size of its own temporary folder: %v", said, err)
	}
	if kilobytes > mostTemporaryKilobytes {
		t.Errorf("the fence's own /tmp holds %d kilobytes and the cap should be %d; a tmpfs asked for with no size is half this machine's"+
			" memory, so pass a size to --tmpfs", kilobytes, mostTemporaryKilobytes)
	}
}

func TestASandboxedCommandCannotReachAServiceOnThisMachine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("a private service on this machine answered\n"))
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("cannot read the test server's port: %v", err)
	}

	fence, _, _ := aRealFence(t, theToolOutputCap)
	said := insideTheFence(t, fence, "curl -s --max-time 5 http://127.0.0.1:"+port+"/")

	if strings.Contains(said, "a private service on this machine answered") {
		t.Errorf("a sandboxed command reached a service on this machine and read %q; the web tool refuses every private address, "+
			"and the shell tool inside the fence has to refuse them too or the guard is only a guard against the web tool", said)
	}
}

// The system call numbers that make a new user namespace without unshare. They
// are written out here rather than taken from the syscall package for the reason
// numbers_amd64.go gives: the package is missing several of them.
const (
	cloneSystemCallOnAmd64  = 56
	clone3SystemCallOnAmd64 = 435
)

func TestTheFilterRefusesANewUserNamespaceHoweverItIsAskedFor(t *testing.T) {
	if seccompArchitecture != 0xc000003e {
		t.Skip("these system call numbers are the ones for a 64-bit Intel or AMD machine")
	}
	program := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)

	for _, call := range []struct {
		name   string
		number uint32
	}{
		{"clone", cloneSystemCallOnAmd64},
		{"clone3", clone3SystemCallOnAmd64},
	} {
		guarded := false
		for _, word := range program {
			if word.value == call.number {
				guarded = true
			}
		}
		if !guarded {
			t.Errorf("the filter refuses unshare with a new user namespace and says nothing at all about %s, "+
				"which makes the same namespace; a rule the caller can walk round by naming another system call is not a rule", call.name)
		}
	}
}

// TestAProgramInsideTheFenceCanReadTheKernelsOwnFolders holds what the fourth
// Tetris run found: /proc was mounted but every read of it was refused, so
// Node counted zero processors and vitest waited forever for workers it never
// started. The kernel's own folders are readable inside the fence.
func TestAProgramInsideTheFenceCanReadTheKernelsOwnFolders(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)
	said := insideTheFence(t, fence, "head -c 40 /proc/cpuinfo >/dev/null && ls /proc/self >/dev/null && cat /proc/self/status | head -1 && echo READABLE")
	if !strings.Contains(said, "READABLE") {
		t.Errorf("a program inside the fence cannot read /proc: %q", said)
	}
}
