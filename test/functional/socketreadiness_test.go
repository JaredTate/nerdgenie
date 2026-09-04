// Socket readiness for the whole-program tests: how long to wait for a
// "nerdgenie serve" started under load to bind its socket, and the dial that
// retries a refused connection until it does. Kept beside serve_test.go's
// process control rather than in it, so each file does one thing.
package functional

import (
	"net"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theWaitForTheSocketToOpen is how long the harness waits for a slow "nerdgenie
// serve" to bind its socket. It is far larger than the second a serve takes on
// an idle machine, because the whole suite building and starting many serves at
// once, beside the several agents that share this machine, starve each serve of
// processor time, so a serve that is merely slow to come up must still be waited
// for. It is bounded to the test's own timeout so that a serve which never comes
// up fails here with its log rather than hanging until the test binary is killed
// with no reason.
func theWaitForTheSocketToOpen(t *testing.T) time.Duration {
	t.Helper()
	const generous = 90 * time.Second
	deadline, isSet := t.Deadline()
	if !isSet {
		return generous
	}
	// A margin is left below the deadline so this wait ends, and prints the
	// serve's log, before the test binary's own timeout ends the run without
	// saying why.
	room := time.Until(deadline) - 5*time.Second
	if room > 0 && room < generous {
		return room
	}
	return generous
}

// dialTheSocketWithin dials the agent's socket and, because a serve that is slow
// to come up under load may not have bound it yet, retries a refused connection
// every fiftieth of a second until the socket answers or the bound runs out. It
// returns the open connection, or fails the test with the serve's log when the
// socket never appears, so a serve that is merely slow is waited for while one
// that never comes up still fails with a clear message rather than hanging.
func dialTheSocketWithin(t *testing.T, home contract.Home, saidPath string, bound time.Duration) net.Conn {
	t.Helper()
	giveUp := time.Now().Add(bound)
	var refusal error
	for {
		connection, err := net.Dial("unix", home.SocketFile())
		if err == nil {
			return connection
		}
		refusal = err
		if !time.Now().Before(giveUp) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("nerdgenie serve never answered on its socket at %s within %s (last refusal: %v); it said:\n%s",
		home.SocketFile(), bound, refusal, whatItSaid(saidPath))
	return nil
}

// waitForTheSocket waits until the agent answers on its socket, and fails with
// whatever the agent printed when it never does.
func waitForTheSocket(t *testing.T, home contract.Home, saidPath string) {
	t.Helper()
	connection := dialTheSocketWithin(t, home, saidPath, theWaitForTheSocketToOpen(t))
	_ = connection.Close()
}
