package desktop

import (
	"encoding/json"
	"fmt"
	"time"
)

// The error codes from the table in worker/desktop/PROTOCOL.md. The first four
// are JSON-RPC's own and the last five are the desktop's.
const (
	// codeParseError means the worker could not read the line as JSON.
	codeParseError = -32700
	// codeInvalidRequest means the line was not a JSON-RPC request.
	codeInvalidRequest = -32600
	// codeNoSuchMethod means the worker has no such method, which is a bug here.
	codeNoSuchMethod = -32601
	// codeBadParameters means the parameters were wrong.
	codeBadParameters = -32602
	// codeNoSuchMark means there is no control with that number on the screen.
	codeNoSuchMark = -32000
	// codeUnreadableWindow means the window could not be read after the limit.
	codeUnreadableWindow = -32001
	// codeNoApplicationOpen means no application has been launched yet.
	codeNoApplicationOpen = -32002
	// codeDriverUnavailable means the desktop driver is not there to drive.
	codeDriverUnavailable = -32003
	// codeLaunchFailed means the application could not be opened or focused.
	codeLaunchFailed = -32004
)

// workerFailure is one refusal from the worker, carrying the protocol's code so
// that the Go side knows whether to restart the worker or tell the model.
type workerFailure struct {
	// Code is one of the nine in the protocol's table.
	Code int `json:"code"`
	// Message says what went wrong, in the worker's own words.
	Message string `json:"message"`
	// Data is what the protocol says to send alongside, such as a fresh
	// screenshot when a control number is not on the screen.
	Data json.RawMessage `json:"data,omitempty"`
}

// Error says what the worker refused and why.
func (failure *workerFailure) Error() string {
	return fmt.Sprintf("the desktop worker refused the call with code %d: %s", failure.Code, failure.Message)
}

// needsRestart says whether this failure means the worker itself is broken. The
// protocol's table decides: a line the worker could not read at all, and a
// driver that is not there, both mean the worker is started again.
func (failure *workerFailure) needsRestart() bool {
	switch failure.Code {
	case codeParseError, codeInvalidRequest, codeDriverUnavailable:
		return true
	default:
		return false
	}
}

// mark is one numbered control, in the shape the protocol sends it.
type mark struct {
	// Number is what the model clicks and drags by.
	Number int `json:"number"`
	// Role says what kind of control it is.
	Role string `json:"role"`
	// Name is the label on it.
	Name string `json:"name"`
}

// diffAnswer is what every action of the protocol returns.
type diffAnswer struct {
	// TitleChanged is true when the window is not called what it was called.
	TitleChanged bool `json:"titleChanged"`
	// Title is what the window is called now.
	Title string `json:"title"`
	// NewMarks are the controls that were not there before the action.
	NewMarks []mark `json:"newMarks"`
	// GoneMarks is how many controls went away.
	GoneMarks int `json:"goneMarks"`
	// Marks is every control on the window now.
	Marks []mark `json:"marks"`
	// ExpectationMet is true when what was expected actually happened.
	ExpectationMet bool `json:"expectationMet"`
	// Seen says what happened instead, when it did not.
	Seen string `json:"seen"`
	// Settled is false when the window never came to rest.
	Settled bool `json:"settled"`
	// Application is the granted application, which only launch fills in.
	Application string `json:"application"`
}

// screenshotAnswer is what the screenshot method returns.
type screenshotAnswer struct {
	// PNGBase64 is the picture of the window, encoded as base64 text.
	PNGBase64 string `json:"pngBase64"`
	// Marks says what each number on the picture points at.
	Marks []mark `json:"marks"`
	// Application is the application the picture is of.
	Application string `json:"application"`
	// Title is what its title bar says.
	Title string `json:"title"`
	// Hidden is how many controls the worker's cap left out.
	Hidden int `json:"hidden"`
	// Windows names every window on the screen by its title.
	Windows []string `json:"windows"`
}

// clipboardAnswer is what clipboardGet returns.
type clipboardAnswer struct {
	// Text is what the clipboard holds.
	Text string `json:"text"`
}

// writtenAnswer is what clipboardSet returns.
type writtenAnswer struct {
	// Characters is how many characters were put on the clipboard.
	Characters int `json:"characters"`
}

// healthAnswer is what the health method returns.
type healthAnswer struct {
	// Healthy is true when the worker can act on the desktop.
	Healthy bool `json:"healthy"`
	// DriverVersion is what the desktop driver calls itself.
	DriverVersion string `json:"driverVersion"`
	// Display says which display server this desktop runs on.
	Display string `json:"display"`
	// Detail says what to fix when the worker is not healthy.
	Detail string `json:"detail"`
}

// The deadline for each method. Reading a window with a picture in it is the
// slow one, and launching an application waits for its window to appear.
var methodDeadlines = map[string]time.Duration{
	"launch":       30 * time.Second,
	"screenshot":   30 * time.Second,
	"click":        20 * time.Second,
	"type":         60 * time.Second,
	"press":        20 * time.Second,
	"drag":         20 * time.Second,
	"clipboardGet": 10 * time.Second,
	"clipboardSet": 10 * time.Second,
	"health":       10 * time.Second,
}

// defaultMethodDeadline is what a method the table does not name gets, so that
// no call can ever wait without a limit.
const defaultMethodDeadline = 20 * time.Second

// deadlineFor is how long one method may take before the worker is restarted.
func deadlineFor(method string) time.Duration {
	if deadline, named := methodDeadlines[method]; named {
		return deadline
	}
	return defaultMethodDeadline
}
