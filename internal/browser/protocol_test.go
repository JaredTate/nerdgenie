package browser

import "testing"

// TestAnUnreachablePageDoesNotRestartTheWorker: on run 26 a refused
// connection was called a dead Chrome and the browser was restarted twice for
// a page whose server was down. The worker now answers -32004 for it, and
// the Go side leaves the worker alone and hands the words to the model.
func TestAnUnreachablePageDoesNotRestartTheWorker(t *testing.T) {
	refused := &RefusedError{Code: codePageUnreachable, Message: "The page could not be reached: page.goto: net::ERR_CONNECTION_REFUSED at http://127.0.0.1:8097/"}
	if refused.needsRestart() {
		t.Error("an unreachable page restarted the worker, and the browser is fine")
	}
	if (&RefusedError{Code: codeChromeDied, Message: "Chrome stopped working"}).needsRestart() == false {
		t.Error("a dead Chrome no longer restarts the worker")
	}
}
