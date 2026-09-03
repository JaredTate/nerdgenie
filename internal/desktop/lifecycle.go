package desktop

import (
	"context"
	"errors"
	"fmt"
)

// call sends one request to the worker, starting the worker first when there is
// none. A worker that dies, or that answers with one of the three codes the
// protocol's table says to restart on, is stopped by its exact process
// identifier and started again on the next call.
func (desktop *Desktop) call(ctx context.Context, method string, params map[string]any, result any) error {
	talker, err := desktop.workerReady(ctx)
	if err != nil {
		return err
	}
	callCtx, done := context.WithTimeout(ctx, deadlineFor(method))
	defer done()

	err = talker.call(callCtx, method, params, result)
	if err == nil {
		return nil
	}

	var failure *workerFailure
	if errors.As(err, &failure) && !failure.needsRestart() {
		return errors.New(failure.Message)
	}
	desktop.guard.Lock()
	pid := desktop.workerProcessID()
	stopped := desktop.stopWorker()
	desktop.guard.Unlock()
	desktop.options.Note("the desktop worker with process id %d was stopped after %s failed: %v", pid, method, err)
	if stopped != nil {
		desktop.options.Note("stopping the desktop worker with process id %d did not go cleanly: %v", pid, stopped)
	}
	return fmt.Errorf("the desktop was interrupted and will be started again on the next call: %w", err)
}

// workerReady returns the running worker, starting and checking one when there
// is none. A worker that says it is unhealthy is stopped, because a worker that
// cannot drive a desktop is worse than none.
func (desktop *Desktop) workerReady(ctx context.Context) (*client, error) {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if desktop.talker != nil {
		return desktop.talker, nil
	}

	connection, err := desktop.options.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("the desktop worker could not be started: %w", err)
	}
	if connection == nil {
		return nil, errors.New("the desktop worker started as nothing at all, which is a fault in how it was wired")
	}
	desktop.connection, desktop.talker = connection, newClient(connection)
	desktop.open = ""
	desktop.options.Note("the desktop worker started with process id %d", connection.ProcessID)

	if err := desktop.checkHealth(ctx); err != nil {
		_ = desktop.stopWorker()
		return nil, err
	}
	return desktop.talker, nil
}

// checkHealth asks the freshly started worker whether it can act at all. The
// caller holds the guard.
func (desktop *Desktop) checkHealth(ctx context.Context) error {
	healthCtx, done := context.WithTimeout(ctx, deadlineFor("health"))
	defer done()

	var health healthAnswer
	if err := desktop.talker.call(healthCtx, "health", map[string]any{}, &health); err != nil {
		return fmt.Errorf("the desktop worker started but would not say whether it is healthy: %w", err)
	}
	if !health.Healthy {
		return fmt.Errorf("the desktop worker cannot drive this machine: %s", detailOr(health.Detail))
	}
	return nil
}

// detailOr is what the worker said is wrong, or a sentence saying it said nothing.
func detailOr(detail string) string {
	if detail == "" {
		return "it did not say why, so run cua-driver doctor on this machine"
	}
	return detail
}

// stopWorker ends the running worker and forgets it. The caller holds the guard.
func (desktop *Desktop) stopWorker() error {
	connection := desktop.connection
	desktop.connection, desktop.talker, desktop.open = nil, nil, ""
	if connection == nil || connection.Stop == nil {
		return nil
	}
	return connection.Stop()
}

// workerProcessID is the exact process identifier of the running worker, and
// zero when there is none. The caller holds the guard.
func (desktop *Desktop) workerProcessID() int {
	if desktop.connection == nil {
		return 0
	}
	return desktop.connection.ProcessID
}
