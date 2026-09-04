package browser

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// One worker is started on the first browser call and used by every call after
// it, because a browser is a long-lived thing and starting Chrome is slow.
func TestOneWorkerIsStartedAndUsedByEveryCall(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	ctx := context.Background()

	if browser.Running() {
		t.Fatal("a browser that has been asked for nothing should not have started a worker")
	}
	openTheSimplePage(t, browser)
	for round := 0; round < 3; round++ {
		if _, err := browser.Read(ctx, contract.ReadOptions{}); err != nil {
			t.Fatalf("reading the page failed: %v", err)
		}
	}
	if started, stopped := world.starts.counts(); started != 1 || stopped != 0 {
		t.Fatalf("%d workers were started and %d stopped, and four calls should have shared one worker", started, stopped)
	}
	if !browser.Running() || browser.ProcessID() != 4242 {
		t.Fatalf("the browser says it is running %v with process id %d, and one worker should be up", browser.Running(), browser.ProcessID())
	}
}

// A worker nobody has used for the idle stretch is stopped, so that no browser
// window sits on the screen all day for nothing.
func TestAWorkerNobodyIsUsingIsStopped(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) { options.IdleStop = 30 * time.Minute })
	openTheSimplePage(t, browser)

	waitForSleepers(t, world.clock, 1)
	world.clock.Advance(31 * time.Minute)
	waitUntil(t, "the idle worker to be stopped", func() bool { return !browser.Running() })

	if _, stopped := world.starts.counts(); stopped != 1 {
		t.Fatalf("%d workers were stopped, and the idle one should have been", stopped)
	}
	if !strings.Contains(world.notes.written(), "with nothing to do") {
		t.Fatalf("the log says %q, and it should say the worker was stopped with nothing to do", world.notes.written())
	}
	openTheSimplePage(t, browser)
	if started, _ := world.starts.counts(); started != 2 {
		t.Fatalf("%d workers were started, and the next call after an idle stop should have started another", started)
	}
}

// A worker that dies is stopped by the exact process identifier it was started
// with, the model is told the browser was interrupted, and the next call starts
// a new one.
func TestADeadWorkerIsRestartedAndTheInterruptionIsReported(t *testing.T) {
	world := newWorld(t)
	var chromeHasDied atomic.Bool
	chromeHasDied.Store(true)
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(scriptedStart(healthyThen(func(line []byte) string {
			if chromeHasDied.Swap(false) {
				return `{"jsonrpc":"2.0","id":` + identifierIn(line) +
					`,"error":{"code":-32003,"message":"Chrome stopped working: the browser was closed"}}` + "\n"
			}
			return `{"jsonrpc":"2.0","id":` + identifierIn(line) +
				`,"result":{"url":"https://fixture.test/simple","title":"A simple page","tabId":"t1","elements":[],"belowFold":0}}` + "\n"
		})))
	})

	_, err := browser.Open(context.Background(), testkit.FixtureSimplePage)
	if err == nil || !strings.Contains(err.Error(), "the browser was interrupted") {
		t.Fatalf("a Chrome that died said %v, and the protocol's table says the model is told it was interrupted", err)
	}
	if started, stopped := world.starts.counts(); started != 1 || stopped != 1 {
		t.Fatalf("%d workers were started and %d stopped, and the dead one should have been stopped", started, stopped)
	}

	page, err := browser.Open(context.Background(), testkit.FixtureSimplePage)
	if err != nil || page.URL != testkit.FixtureSimplePage {
		t.Fatalf("the call after the interruption answered %+v and %v, and it should have started a new worker", page, err)
	}
	if started, _ := world.starts.counts(); started != 2 {
		t.Fatalf("%d workers were started, and the call after an interruption should have started a second", started)
	}
}

// A line the worker could not read at all, and a line that was not a request,
// both mean a new worker, which is what the protocol's error table says.
func TestTheTwoUnreadableAnswersAlsoStartANewWorker(t *testing.T) {
	for _, refusal := range []struct {
		name string
		code int
	}{
		{name: "a line that was not JSON", code: codeParseError},
		{name: "a line that was not a request", code: codeInvalidRequest},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			world := newWorld(t)
			code := refusal.code
			browser := world.browser(t, func(options *Options) {
				options.Start = world.startedBy(scriptedStart(healthyThen(func([]byte) string {
					return `{"jsonrpc":"2.0","id":null,"error":{"code":` + strconv.Itoa(code) +
						`,"message":"the line was not JSON, so send one JSON object per line"}}` + "\n"
				})))
			})
			_, err := browser.Read(context.Background(), contract.ReadOptions{})
			if err == nil || !strings.Contains(err.Error(), "the browser was interrupted") {
				t.Fatalf("code %d said %v, and the table says the worker is started again", code, err)
			}
			if _, stopped := world.starts.counts(); stopped != 1 {
				t.Fatalf("%d workers were stopped after code %d, and the table says one is", stopped, code)
			}
		})
	}
}

// A worker that will not start is tried again after a growing wait, so that a
// browser that cannot start does not spin.
func TestAWorkerThatWillNotStartIsTriedAgainAfterAGrowingWait(t *testing.T) {
	world := newWorld(t)
	var refusals atomic.Int32
	refusals.Store(2)
	inner := world.starts.start
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(func(ctx context.Context) (*Connection, error) {
			if refusals.Add(-1) >= 0 {
				return nil, errors.New("google-chrome is not installed on this machine")
			}
			return inner(ctx)
		})
	})

	opened := make(chan error, 1)
	go func() {
		_, err := browser.Open(context.Background(), testkit.FixtureSimplePage)
		opened <- err
	}()
	waitForSleepers(t, world.clock, 1)
	world.clock.Advance(firstRestartWait)
	waitForSleepers(t, world.clock, 1)
	world.clock.Advance(2 * firstRestartWait)

	if err := <-opened; err != nil {
		t.Fatalf("the third attempt should have opened the page, and it said: %v", err)
	}
	moments := world.starts.moments()
	if len(moments) != 3 {
		t.Fatalf("the worker was started %d times, and it should have taken three attempts", len(moments))
	}
	if first, second := moments[1].Sub(moments[0]), moments[2].Sub(moments[1]); first != firstRestartWait || second != 2*firstRestartWait {
		t.Fatalf("the waits between attempts were %s and %s, and they should have been %s and %s",
			first, second, firstRestartWait, 2*firstRestartWait)
	}
}

// A worker that will not start at all is given up on, and the message says what
// to check.
func TestAWorkerThatNeverStartsIsGivenUpOn(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(func(context.Context) (*Connection, error) {
			return nil, errors.New("google-chrome is not installed on this machine")
		})
	})

	failed := make(chan error, 1)
	go func() {
		_, err := browser.Read(context.Background(), contract.ReadOptions{})
		failed <- err
	}()
	for attempt := 1; attempt < mostStartAttempts; attempt++ {
		waitForSleepers(t, world.clock, 1)
		world.clock.Advance(longestRestartWait)
	}
	err := <-failed
	if err == nil || !strings.Contains(err.Error(), "Node and Google Chrome") {
		t.Fatalf("giving up said %v, and it should say to check that Node and Google Chrome are installed", err)
	}
	if started, _ := world.starts.counts(); started != mostStartAttempts {
		t.Fatalf("the worker was started %d times, and the limit is %d", started, mostStartAttempts)
	}
}

// A worker that says it cannot drive a browser is stopped, because a worker that
// cannot work is worse than none.
func TestAnUnhealthyWorkerIsStopped(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(scriptedStart(func(line []byte) string {
			return `{"jsonrpc":"2.0","id":` + identifierIn(line) +
				`,"result":{"healthy":false,"detail":"there is no display on this machine"}}` + "\n"
		}))
	})

	_, err := browser.Read(context.Background(), contract.ReadOptions{})
	if err == nil || !strings.Contains(err.Error(), "no display on this machine") {
		t.Fatalf("an unhealthy worker said %v, and it should have passed on what the worker said was wrong", err)
	}
	if _, stopped := world.starts.counts(); stopped != 1 {
		t.Fatalf("%d workers were stopped, and an unhealthy one should have been", stopped)
	}
}

// A closed browser starts nothing again.
func TestAClosedBrowserStartsNothingAgain(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	if err := browser.Close(); err != nil {
		t.Fatalf("closing the browser failed: %v", err)
	}
	if _, err := browser.Read(context.Background(), contract.ReadOptions{}); err == nil ||
		!strings.Contains(err.Error(), "has been closed") {
		t.Fatalf("a call after Close said %v, and it should say the browser has been closed", err)
	}
	if started, stopped := world.starts.counts(); started != 1 || stopped != 1 {
		t.Fatalf("%d workers were started and %d stopped, and Close should have stopped the one there was", started, stopped)
	}
}

// The waits between attempts double and then stop at the cap.
func TestTheWaitBetweenAttemptsDoublesAndThenStops(t *testing.T) {
	wanted := []time.Duration{firstRestartWait, firstRestartWait, 2 * firstRestartWait, 4 * firstRestartWait}
	for attempt, want := range wanted {
		if got := waitBeforeAttempt(attempt + 1); got != want {
			t.Fatalf("attempt %d waits %s, and it should wait %s", attempt+1, got, want)
		}
	}
	if got := waitBeforeAttempt(50); got != longestRestartWait {
		t.Fatalf("the fiftieth attempt waits %s, and the cap is %s", got, longestRestartWait)
	}
}
