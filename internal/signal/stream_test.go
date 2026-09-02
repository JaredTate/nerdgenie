package signal

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/testkit"
)

// waitFor polls a condition until it holds, and fails the test when it never
// does. It is how a test waits for a goroutine without waiting on a real clock
// for anything the fake clock controls.
func waitFor(t *testing.T, what string, holds func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if holds() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waited five seconds and %s never happened", what)
}

// runStream starts a stream against a daemon and collects the messages it reads.
func runStream(t *testing.T, address string, clock *testkit.FakeClock) (*Stream, func() []Event) {
	t.Helper()
	client, _ := newTestClient(t, address)
	stream := NewStream(client, clock)

	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)

	guard := sync.Mutex{}
	read := []Event{}
	go func() {
		_ = stream.Run(ctx, func(event Event) {
			guard.Lock()
			defer guard.Unlock()
			read = append(read, event)
		})
	}()

	return stream, func() []Event {
		guard.Lock()
		defer guard.Unlock()
		copied := make([]Event, len(read))
		copy(copied, read)
		return copied
	}
}

func TestStreamReadsAMessageOffTheDaemon(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	stream, collected := runStream(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath), clock)

	waitFor(t, "the stream connects", func() bool { return stream.Connections() == 1 })
	daemon.PushMessage("+15125550123", "hello there")

	waitFor(t, "the message arrives", func() bool { return len(collected()) > 0 })
	arrived := collected()
	if arrived[0].Sender != "+15125550123" || arrived[0].Text != "hello there" {
		t.Errorf("the message that arrived is %+v, want the one that was pushed", arrived[0])
	}
}

func TestStreamWaitsAndConnectsAgainAfterTheStreamDrops(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	stream, _ := runStream(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath), clock)

	waitFor(t, "the stream connects", func() bool { return stream.Connections() == 1 })
	daemon.DropStream()

	waitFor(t, "the stream waits before trying again", func() bool { return clock.Sleepers() > 0 })
	if stream.Connections() != 1 {
		t.Fatalf("the stream connected again without waiting at all, and a dropped stream is waited out")
	}

	clock.Advance(ReconnectMinimumWait)
	waitFor(t, "the stream connects again", func() bool { return stream.Connections() >= 2 })
}

func TestStreamConnectsAgainAfterTwoMinutesOfSilence(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	stream, _ := runStream(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath), clock)

	waitFor(t, "the stream connects", func() bool { return stream.Connections() == 1 })
	clock.Advance(SilenceBeforeReconnect)

	waitFor(t, "the quiet stream is dropped and opened again", func() bool { return stream.Connections() >= 2 })
}

func TestStreamKeepsTryingWhenNoDaemonIsThere(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	address := strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath)
	daemon.Close()

	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	stream, _ := runStream(t, address, clock)

	waitFor(t, "the stream waits before trying again", func() bool { return clock.Sleepers() > 0 })
	if stream.Connections() != 0 {
		t.Errorf("the stream says it connected %d times to a daemon that is not there", stream.Connections())
	}
	clock.Advance(ReconnectMinimumWait)
	waitFor(t, "the stream waits again after the second try", func() bool { return clock.Sleepers() > 0 })
}

func TestStreamStopsWhenItsContextIsCancelled(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	client, _ := newTestClient(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath))
	stream := NewStream(client, clock)

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- stream.Run(ctx, func(Event) {}) }()

	waitFor(t, "the stream connects", func() bool { return stream.Connections() == 1 })
	stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("the stream did not stop when its context was cancelled")
	}
}

func TestNextWaitGrowsFromTwoSecondsToSixtyAndStops(t *testing.T) {
	wanted := []time.Duration{
		4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second,
		60 * time.Second, 60 * time.Second,
	}
	wait := ReconnectMinimumWait
	if wait != 2*time.Second {
		t.Fatalf("the first wait is %v, want two seconds", wait)
	}
	for step, want := range wanted {
		wait = nextWait(wait)
		if wait != want {
			t.Fatalf("wait %d is %v, want %v", step+2, wait, want)
		}
	}
	if ReconnectMaximumWait != 60*time.Second {
		t.Errorf("the longest wait is %v, want sixty seconds", ReconnectMaximumWait)
	}
}

func TestReadFrameJoinsTheLinesOfOneEvent(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"one data line", []string{"data: {\"a\":1}", ""}, `{"a":1}`},
		{"two data lines", []string{"data: {", "data: }", ""}, "{\n}"},
		{"a keepalive comment is not an event", []string{": keepalive", ""}, ""},
		{"the event name is not part of the payload", []string{"event: receive", "data: hi", ""}, "hi"},
		{"a carriage return is not part of the payload", []string{"data: hi\r", ""}, "hi"},
		{"an identifier is not part of the payload", []string{"id: 7", "data: hi", ""}, "hi"},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			reader := &frameReader{}
			payload := ""
			for _, line := range oneCase.lines {
				if done, ready := reader.take(line); ready {
					payload = done
				}
			}
			if payload != oneCase.want {
				t.Errorf("the frame came out as %q, want %q", payload, oneCase.want)
			}
		})
	}
}

func TestReadFrameDropsAFrameLargerThanTheCap(t *testing.T) {
	reader := &frameReader{}
	huge := "data: " + strings.Repeat("a", MaxEventBytes+1)
	if _, ready := reader.take(huge); ready {
		t.Errorf("a frame larger than the cap was handed on, and every buffer has a cap")
	}
	if _, ready := reader.take(""); ready {
		t.Errorf("the end of an over-long frame handed it on anyway")
	}
}
