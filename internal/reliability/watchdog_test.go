package reliability_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// watchdogInterval is the interval the fake service manager announces in these
// tests, so that the feed is expected at half of it.
const watchdogInterval = 60 * time.Second

// aNotifySocket opens a socket that stands in for the one systemd listens on,
// and points the environment at it the way systemd points a service at its own.
// The folder is made outside the test's own name, because a unix socket path is
// only about a hundred characters long and test names here are not short.
func aNotifySocket(t *testing.T, watchdogIsOn bool) net.PacketConn {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-notify-*")
	if err != nil {
		t.Fatalf("making the folder for the fake notify socket failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })

	listening, err := net.ListenPacket("unixgram", filepath.Join(folder, "notify.sock"))
	if err != nil {
		t.Fatalf("opening the fake notify socket failed: %v", err)
	}
	t.Cleanup(func() { _ = listening.Close() })

	t.Setenv("NOTIFY_SOCKET", filepath.Join(folder, "notify.sock"))
	if watchdogIsOn {
		t.Setenv("WATCHDOG_USEC", strconv.FormatInt(watchdogInterval.Microseconds(), 10))
		t.Setenv("WATCHDOG_PID", strconv.Itoa(os.Getpid()))
	}
	return listening
}

// whatArrived reads one message off the fake notify socket, or an empty string
// when nothing arrives before the wait is over.
func whatArrived(t *testing.T, listening net.PacketConn, wait time.Duration) string {
	t.Helper()
	if err := listening.SetReadDeadline(time.Now().Add(wait)); err != nil {
		t.Fatalf("setting the read deadline on the fake notify socket failed: %v", err)
	}
	written := make([]byte, 128)
	read, _, err := listening.ReadFrom(written)
	if err != nil {
		return ""
	}
	return string(written[:read])
}

func TestTheWatchdogSaysTheProgramIsReady(t *testing.T) {
	listening := aNotifySocket(t, true)
	watchdog, err := reliability.NewWatchdog(testkit.NewFakeClock(startOfTime))
	if err != nil {
		t.Fatalf("building the watchdog failed: %v", err)
	}

	if err := watchdog.Ready(); err != nil {
		t.Fatalf("saying the program is ready failed: %v", err)
	}

	if arrived := whatArrived(t, listening, time.Second); arrived != "READY=1" {
		t.Errorf("the service manager was told %q, want READY=1", arrived)
	}
	if watchdog.Interval() != watchdogInterval {
		t.Errorf("the watchdog interval is %s, want the %s systemd announced", watchdog.Interval(), watchdogInterval)
	}
}

func TestTheWatchdogIsFedAtHalfTheIntervalSystemdAnnounced(t *testing.T) {
	listening := aNotifySocket(t, true)
	clock := testkit.NewFakeClock(startOfTime)
	watchdog, err := reliability.NewWatchdog(clock)
	if err != nil {
		t.Fatalf("building the watchdog failed: %v", err)
	}
	feeding, stopFeeding := context.WithCancel(context.Background())
	defer stopFeeding()
	go func() { _ = watchdog.Feed(feeding) }()

	arrived := ""
	for range 100 {
		clock.Advance(watchdogInterval / 2)
		if arrived = whatArrived(t, listening, 50*time.Millisecond); arrived != "" {
			break
		}
	}

	if arrived != "WATCHDOG=1" {
		t.Errorf("the service manager was told %q while the program was healthy, want WATCHDOG=1", arrived)
	}
}

func TestTheFeedEndsWithTheContextItWasGiven(t *testing.T) {
	aNotifySocket(t, true)
	clock := testkit.NewFakeClock(startOfTime)
	watchdog, err := reliability.NewWatchdog(clock)
	if err != nil {
		t.Fatalf("building the watchdog failed: %v", err)
	}
	feeding, stopFeeding := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- watchdog.Feed(feeding) }()
	stopFeeding()

	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("the feed ended with %v, want nothing when it was told to stop", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("the feed did not end when the context was cancelled")
	}
}

func TestOutsideSystemdTheWatchdogDoesNothingAndSaysSo(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	t.Setenv("WATCHDOG_USEC", "")
	t.Setenv("WATCHDOG_PID", "")
	watchdog, err := reliability.NewWatchdog(testkit.NewFakeClock(startOfTime))
	if err != nil {
		t.Fatalf("building the watchdog outside systemd failed: %v", err)
	}

	if watchdog.Interval() != 0 {
		t.Errorf("the watchdog interval is %s outside systemd, want none", watchdog.Interval())
	}
	if err := watchdog.Ready(); err != nil {
		t.Errorf("saying the program is ready outside systemd failed: %v", err)
	}
	if err := watchdog.Feed(context.Background()); err != nil {
		t.Errorf("feeding a watchdog that is not there failed: %v", err)
	}
}
