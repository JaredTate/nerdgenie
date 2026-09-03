// The worker's lifecycle follows OpenClaw's Chrome launcher at
// ~/Code/openclaw/extensions/browser/src/browser/chrome.ts and its attach at
// ~/Code/openclaw/extensions/browser/src/browser/pw-session-cdp-transport.ts:
// one long-lived browser per profile, started on first use, checked before it is
// trusted, and taken down as a whole when it stops answering. The waiting, the
// idle stop, and the backoff are written fresh in Go.

package browser

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// call sends one request to the worker, starting the worker first when there is
// none. A worker that dies, or that answers with one of the three codes the
// protocol's table says to restart on, is stopped by its exact process
// identifier and started again on the next call.
func (browser *Browser) call(ctx context.Context, method string, params any, result any) error {
	talker, err := browser.workerReady(ctx)
	if err != nil {
		return err
	}
	callCtx, done := context.WithTimeout(ctx, deadlineFor(method))
	defer done()

	err = talker.call(callCtx, method, params, result)
	browser.markUsed()
	if err == nil {
		return nil
	}
	return browser.afterAFailedCall(method, err)
}

// afterAFailedCall decides what one failure means: something the model can act
// on, a fault on this side of the pipe, or a worker that has to be started
// again.
func (browser *Browser) afterAFailedCall(method string, err error) error {
	var refused *RefusedError
	if errors.As(err, &refused) {
		if refused.isBugOnThisSide() {
			return fmt.Errorf("the browser worker has no method called %s, which is a fault in Coeus rather than in the page: %w", method, err)
		}
		if !refused.needsRestart() {
			return err
		}
	}

	browser.guard.Lock()
	pid := browser.workerProcessID()
	stopped := browser.stopWorker()
	browser.guard.Unlock()
	browser.options.Note("the browser worker with process id %d was stopped after %s failed: %v", pid, method, err)
	if stopped != nil {
		browser.options.Note("stopping the browser worker with process id %d did not go cleanly: %v", pid, stopped)
	}
	return interrupted(method, err)
}

// markUsed records that the worker has just done something, which is what the
// idle watcher counts from.
func (browser *Browser) markUsed() {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	browser.lastUsed = browser.options.Clock.Now()
}

// workerReady returns the running worker, starting and checking one when there
// is none. A worker that says it is unhealthy is stopped, because a worker that
// cannot drive a browser is worse than none.
func (browser *Browser) workerReady(ctx context.Context) (*client, error) {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	if browser.closed {
		return nil, errors.New("the browser has been closed, so start a new one before opening a page")
	}
	if browser.talker != nil {
		// A call that is about to go out is not an idle worker, so the clock on
		// the idle stop starts again here rather than only when the call answers.
		browser.lastUsed = browser.options.Clock.Now()
		return browser.talker, nil
	}
	if err := browser.startOneWorker(ctx); err != nil {
		return nil, err
	}
	if err := browser.checkHealth(ctx); err != nil {
		_ = browser.stopWorker()
		return nil, err
	}
	browser.watchForIdleness()
	return browser.talker, nil
}

// startOneWorker starts a worker and waits a growing stretch between attempts,
// so that a browser that cannot start does not spin. The caller holds the guard.
func (browser *Browser) startOneWorker(ctx context.Context) error {
	var lastProblem error
	for attempt := 1; attempt <= mostStartAttempts; attempt++ {
		if attempt > 1 {
			if err := browser.options.Clock.Sleep(ctx, waitBeforeAttempt(attempt)); err != nil {
				return fmt.Errorf("the browser worker could not be started and the wait was cut short: %w", err)
			}
		}
		connection, err := browser.options.Start(ctx)
		if err != nil {
			lastProblem = err
			browser.options.Note("attempt %d to start the browser worker failed: %v", attempt, err)
			continue
		}
		if connection == nil {
			lastProblem = errors.New("the browser worker started as nothing at all, which is a fault in how it was wired")
			continue
		}
		browser.events = newEventStream(browser.options.BufferedEvents, browser.options.Note)
		browser.connection, browser.talker = connection, newClient(connection, browser.events)
		browser.lastUsed = browser.options.Clock.Now()
		browser.options.Note("the browser worker started with process id %d", connection.ProcessID)
		return nil
	}
	return fmt.Errorf("the browser worker would not start after %d attempts, so check that Node and Google Chrome are installed: %w",
		mostStartAttempts, lastProblem)
}

// waitBeforeAttempt is how long to wait before one attempt, doubling each time
// up to the cap.
func waitBeforeAttempt(attempt int) time.Duration {
	wait := firstRestartWait
	for step := 2; step < attempt; step++ {
		wait *= 2
		if wait >= longestRestartWait {
			return longestRestartWait
		}
	}
	return wait
}

// checkHealth asks the freshly started worker whether it can act at all. The
// caller holds the guard.
func (browser *Browser) checkHealth(ctx context.Context) error {
	healthCtx, done := context.WithTimeout(ctx, startupHealthDeadline)
	defer done()

	var health contract.BrowserHealth
	if err := browser.talker.call(healthCtx, "health", map[string]any{}, &health); err != nil {
		return fmt.Errorf("the browser worker started but would not say whether it is healthy: %w", err)
	}
	if !health.Healthy {
		return fmt.Errorf("the browser worker cannot drive a browser on this machine: %s", detailOr(health.Detail))
	}
	browser.options.Note("the browser worker is driving Chrome %s", health.ChromeVersion)
	return nil
}

// detailOr is what the worker said is wrong, or a sentence saying it said
// nothing at all.
func detailOr(detail string) string {
	if detail == "" {
		return "it did not say why, so check that google-chrome is installed and that this machine has a display"
	}
	return detail
}

// watchForIdleness starts the watcher that stops a worker nobody is using. The
// caller holds the guard.
func (browser *Browser) watchForIdleness() {
	watchCtx, stopWatching := context.WithCancel(context.Background())
	browser.stopWatch = stopWatching
	go browser.watchUntilIdle(watchCtx)
}

// watchUntilIdle looks every so often at how long it has been since the worker
// did anything, and stops it once the idle stretch has passed. It ends with the
// worker it was watching.
func (browser *Browser) watchUntilIdle(ctx context.Context) {
	for {
		if err := browser.options.Clock.Sleep(ctx, idleCheckEvery); err != nil {
			return
		}
		if browser.stopIfIdle() {
			return
		}
	}
}

// stopIfIdle stops the worker when nothing has used it for the idle stretch, and
// says whether the watcher's work is done.
func (browser *Browser) stopIfIdle() bool {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	if browser.connection == nil {
		return true
	}
	if browser.options.Clock.Now().Sub(browser.lastUsed) < browser.options.IdleStop {
		return false
	}
	pid := browser.workerProcessID()
	stopped := browser.stopWorker()
	browser.options.Note("the browser worker with process id %d was stopped after %s with nothing to do", pid, browser.options.IdleStop)
	if stopped != nil {
		browser.options.Note("stopping the idle browser worker with process id %d did not go cleanly: %v", pid, stopped)
	}
	return true
}

// stopWorker ends the running worker and forgets it, and ends every stream of
// what the person was doing in its window, because those events belonged to that
// worker's window and there is no window now. The caller holds the guard.
func (browser *Browser) stopWorker() error {
	connection := browser.connection
	if browser.events != nil {
		browser.events.closeEveryReader()
		browser.events = nil
	}
	browser.connection, browser.talker, browser.page = nil, nil, ""
	if browser.stopWatch != nil {
		browser.stopWatch()
		browser.stopWatch = nil
	}
	if connection == nil || connection.Stop == nil {
		return nil
	}
	return connection.Stop()
}

// workerProcessID is the exact process identifier of the running worker, and
// zero when there is none. The caller holds the guard.
func (browser *Browser) workerProcessID() int {
	if browser.connection == nil {
		return 0
	}
	return browser.connection.ProcessID
}
