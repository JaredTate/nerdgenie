package contract

import (
	"context"
	"time"
)

// Element is one node of the compact page tree the model reads. A few hundred
// tokens of these is the whole of what the model sees of a web page.
type Element struct {
	// Ref is the short label the model points at later, such as "e12".
	Ref string `json:"ref"`
	// Role says what kind of element it is, such as "button" or "textbox".
	Role string `json:"role"`
	// Name is what it is called on the page, such as "Compose post".
	Name string `json:"name"`
	// New says the element was not in the previous snapshot.
	New bool `json:"new,omitempty"`
}

// Dialog is a browser dialog box that is open on the page.
type Dialog struct {
	// Kind is "alert", "confirm", "prompt", or "beforeunload".
	Kind string `json:"kind"`
	// Message is the text the dialog shows.
	Message string `json:"message"`
}

// Download is a file the page started downloading.
type Download struct {
	// Filename is the name the page gave the file.
	Filename string `json:"filename"`
	// Path is where the worker saved it.
	Path string `json:"path"`
}

// Snapshot is the state of one page after an action settled.
type Snapshot struct {
	// URL is the page's address.
	URL string `json:"url"`
	// Title is the page's title.
	Title string `json:"title"`
	// TabID says which tab this is, such as "t1".
	TabID string `json:"tabId"`
	// Elements is the compact tree, in reading order.
	Elements []Element `json:"elements"`
	// Text is what the page says, as a person reads it: its visible text in
	// reading order, one line per block, a table as one line per row with the
	// cells separated by " | ", capped by the worker with a last line saying
	// how much was cut. It carries what an outline of the elements cannot,
	// such as the number in a cell of a table.
	Text string `json:"text,omitempty"`
	// BelowFold counts the elements the user would have to scroll to see.
	BelowFold int `json:"belowFold"`
	// Dialog is the open dialog box, or nil.
	Dialog *Dialog `json:"dialog,omitempty"`
	// Download is a download the page started, or nil.
	Download *Download `json:"download,omitempty"`
	// Wall is the login form, two-factor prompt, or captcha the page shows, or
	// nil; "open" and "read" report it here, an action reports it on its diff.
	Wall *Wall `json:"wall,omitempty"`
	// Errors is what went wrong on the page since it last moved to an address,
	// as a person with the console open sees it: uncaught script errors,
	// console errors, and the page, scripts and stylesheets that failed to
	// load or answered an error status. The worker lists the first five and
	// counts the rest on a last line.
	Errors []string `json:"errors,omitempty"`
}

// WallKind names the three things that stop the agent and hand the browser to
// the user.
type WallKind string

const (
	// WallLogin is a login form.
	WallLogin WallKind = "login"
	// WallTwoFactor is a prompt for a second code after the password.
	WallTwoFactor WallKind = "two-factor"
	// WallCaptcha is a puzzle meant to prove the visitor is a person.
	WallCaptcha WallKind = "captcha"
)

// Wall is what the worker saw when it hit one of the three walls.
type Wall struct {
	// Kind is which wall it was.
	Kind WallKind `json:"kind"`
	// Detail names the element or the text that gave it away.
	Detail string `json:"detail"`
}

// Diff is what changed on the page because of one action, and whether what the
// model expected actually happened.
type Diff struct {
	// URLChanged says the page moved to a new address.
	URLChanged bool `json:"urlChanged,omitempty"`
	// URL is the address after the action.
	URL string `json:"url"`
	// NewElements are the elements that were not there before.
	NewElements []Element `json:"newElements,omitempty"`
	// Dialog is a dialog box the action opened, or nil.
	Dialog *Dialog `json:"dialog,omitempty"`
	// NewTab is the id of a tab the action opened, or empty.
	NewTab string `json:"newTab,omitempty"`
	// Download is a download the action started, or nil.
	Download *Download `json:"download,omitempty"`
	// ExpectationMet says whether what the model expected actually happened.
	ExpectationMet bool `json:"expectationMet"`
	// Seen says what happened instead, when the expectation was not met.
	Seen string `json:"seen,omitempty"`
	// Wall is the login form, two-factor prompt, or captcha the action ran into,
	// or nil.
	Wall *Wall `json:"wall,omitempty"`
	// Settled says the page came to rest within the limit. When it did not, the
	// diff is still returned from the page as it stood, with Seen saying so, so
	// that a live page stays usable.
	Settled bool `json:"settled"`
	// Snapshot is the page after the action settled, or as it stood at the
	// limit.
	Snapshot Snapshot `json:"snapshot"`
}

// DialogAction is what to do with an open dialog box.
type DialogAction string

const (
	// DialogAccept presses the dialog's confirming button, with the text given
	// for a prompt.
	DialogAccept DialogAction = "accept"
	// DialogDismiss closes the dialog without confirming.
	DialogDismiss DialogAction = "dismiss"
)

// KnownDialogAction says whether the action is one of the two.
func KnownDialogAction(action DialogAction) bool {
	return action == DialogAccept || action == DialogDismiss
}

// ReadOptions says how much of the page to read.
type ReadOptions struct {
	// VisibleOnly reads only what is above the fold.
	VisibleOnly bool `json:"visibleOnly,omitempty"`
}

// ActStep is one step of a batch that aborts as soon as the page changes
// underneath it.
type ActStep struct {
	// Method is the browser method to run, such as "click" or "type".
	Method string `json:"method"`
	// Ref is the element to act on, when the method needs one.
	Ref string `json:"ref,omitempty"`
	// Text is what to type, when the method is "type".
	Text string `json:"text,omitempty"`
	// Key is the key to press, when the method is "press".
	Key string `json:"key,omitempty"`
	// Direction is which way to scroll, when the method is "scroll".
	Direction ScrollDirection `json:"direction,omitempty"`
	// Amount is how many steps to scroll, when the method is "scroll".
	Amount int `json:"amount,omitempty"`
	// Expectation is what the model expects to happen, in plain words.
	Expectation string `json:"expectation"`
}

// Tab is one open browser tab.
type Tab struct {
	// ID is the tab's label, such as "t1".
	ID string `json:"id"`
	// URL is the address it is on.
	URL string `json:"url"`
	// Title is its title.
	Title string `json:"title"`
	// Active says it is the tab the agent is acting on.
	Active bool `json:"active,omitempty"`
}

// TabAction is what to do with the tabs.
type TabAction string

const (
	// TabList only lists them.
	TabList TabAction = "list"
	// TabSwitch makes another tab the active one.
	TabSwitch TabAction = "switch"
	// TabClose closes a tab.
	TabClose TabAction = "close"
)

// ScrollDirection is which way to scroll.
type ScrollDirection string

const (
	// ScrollUp scrolls towards the top of the page.
	ScrollUp ScrollDirection = "up"
	// ScrollDown scrolls towards the bottom.
	ScrollDown ScrollDirection = "down"
)

// LoginFields tells the worker where the login boxes are and what to type into
// them. The values come from the vault, the worker types them, and no method
// ever returns them.
type LoginFields struct {
	// UsernameRef is the element to type the login name into.
	UsernameRef string `json:"usernameRef"`
	// PasswordRef is the element to type the password into.
	PasswordRef string `json:"passwordRef"`
	// CodeRef is the element to type the second code into, and is empty when the
	// page asked for no second code.
	CodeRef string `json:"codeRef,omitempty"`
	// Username is the login name to type.
	Username string `json:"username"`
	// Password is the password to type.
	Password string `json:"password"`
	// Code is the two-factor code to type, and is empty when there is none.
	Code string `json:"code,omitempty"`
}

// Mark is one numbered clickable element on a screenshot.
type Mark struct {
	// Number is the number drawn on the picture.
	Number int `json:"number"`
	// Ref is the element it points at, such as "e12".
	Ref string `json:"ref"`
	// Role says what kind of element it is.
	Role string `json:"role"`
	// Name is what it is called.
	Name string `json:"name"`
}

// Screenshot is a picture of the page with its clickable elements numbered.
type Screenshot struct {
	// PNGBase64 is the picture, encoded as base64 text.
	PNGBase64 string `json:"pngBase64"`
	// Marks lists what each number points at.
	Marks []Mark `json:"marks"`
}

// BrowserEventKind names one of the three things a person does in the browser
// window that the worker reports as it happens.
type BrowserEventKind string

const (
	// BrowserEventClick is a person clicking something on the page.
	BrowserEventClick BrowserEventKind = "click"
	// BrowserEventType is a person typing into a box on the page.
	BrowserEventType BrowserEventKind = "type"
	// BrowserEventNavigate is a person taking the window to another address.
	BrowserEventNavigate BrowserEventKind = "navigate"
)

// KnownBrowserEventKind says whether the kind is one of the three the protocol
// defines, so that a worker sending anything else is refused rather than
// recorded.
func KnownBrowserEventKind(kind BrowserEventKind) bool {
	return kind == BrowserEventClick || kind == BrowserEventType || kind == BrowserEventNavigate
}

// BrowserEvent is one thing the person did in the browser window themselves,
// which is what "/walk record" watches to write a procedure down. What the
// person typed is never carried: a typing event says how many characters went
// into the box and nothing else, because a recording must not become a copy of
// a password.
type BrowserEvent struct {
	// Kind is click, type, or navigate.
	Kind BrowserEventKind `json:"kind"`
	// Ref is the element that was clicked or typed into, such as "e12", and is
	// empty on a navigation.
	Ref string `json:"ref,omitempty"`
	// Text is what the clicked element said, which is what a recorded step
	// expects the page to answer. It is empty on the other two kinds.
	Text string `json:"text,omitempty"`
	// Length is how many characters the box held after the person typed, and is
	// zero on the other two kinds.
	Length int `json:"length,omitempty"`
	// Address is where the window went, and is empty on the other two kinds.
	Address string `json:"address,omitempty"`
	// At is when it happened, by the worker's clock.
	At time.Time `json:"at"`
}

// BrowserHealth says whether the browser worker is alive and what it is running.
type BrowserHealth struct {
	// Healthy is true when the worker can act on a page.
	Healthy bool `json:"healthy"`
	// Detail says what is wrong when it is not.
	Detail string `json:"detail,omitempty"`
	// ChromeVersion is the version of the real Chrome it drove.
	ChromeVersion string `json:"chromeVersion,omitempty"`
}

// BrowserWorker is the Go side of the eleven methods in
// worker/browser/PROTOCOL.md, which the real worker speaks over standard input
// and output as JSON-RPC and the fake worker in internal/testkit speaks over a
// local socket.
type BrowserWorker interface {
	// Open goes to a page and returns its snapshot.
	Open(ctx context.Context, address string) (Snapshot, error)
	// Read returns a fresh snapshot of the current page.
	Read(ctx context.Context, options ReadOptions) (Snapshot, error)
	// Click clicks one element and checks the expectation.
	Click(ctx context.Context, ref string, expectation string) (Diff, error)
	// Type types into one element and checks the expectation.
	Type(ctx context.Context, ref string, text string, expectation string) (Diff, error)
	// Press presses one key and checks the expectation.
	Press(ctx context.Context, key string, expectation string) (Diff, error)
	// Scroll scrolls the page and checks the expectation.
	Scroll(ctx context.Context, direction ScrollDirection, amount int, expectation string) (Diff, error)
	// Act runs a batch of steps and stops as soon as one expectation fails.
	Act(ctx context.Context, steps []ActStep) ([]Diff, error)
	// Tabs lists, switches, or closes tabs and returns the list afterwards.
	Tabs(ctx context.Context, action TabAction, tabID string) ([]Tab, error)
	// LoginFill types a username, a password, and a code, and returns a diff
	// that never holds any of them.
	LoginFill(ctx context.Context, fields LoginFields) (Diff, error)
	// Screenshot returns the page as a numbered picture.
	Screenshot(ctx context.Context) (Screenshot, error)
	// Health says whether the worker is alive.
	Health(ctx context.Context) (BrowserHealth, error)
	// Dialog answers an open dialog box, accepting it with the text given for a
	// prompt or dismissing it, because Chrome blocks the whole tab until one is
	// answered.
	Dialog(ctx context.Context, action DialogAction, text string) (Diff, error)
	// Events is the stream of what the person did in the window themselves: their
	// clicks, their typing, and where they took the browser. The channel closes
	// when the reader's context is done or the worker goes, and no more than
	// Caps.BufferedBrowserEvents are held for a reader that has fallen behind.
	Events(ctx context.Context) (<-chan BrowserEvent, error)
	// Close shuts the worker down and closes the browser.
	Close() error
}
