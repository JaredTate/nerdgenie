package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// The fixture pages the fake browser knows, and the references on them. A test
// names one of these rather than building a page of its own, so that every
// browser test starts from the same picture.
const (
	// FixtureSimplePage has a link and a form on it.
	FixtureSimplePage = "https://fixture.test/simple"
	// FixtureChangedPage is where the link on the simple page goes.
	FixtureChangedPage = "https://fixture.test/changed"
	// FixtureLoginPage asks for a username and a password.
	FixtureLoginPage = "https://fixture.test/login"
	// FixtureTwoFactorPage asks for a second code.
	FixtureTwoFactorPage = "https://fixture.test/two-factor"
	// FixtureCaptchaPage asks the visitor to prove they are a person.
	FixtureCaptchaPage = "https://fixture.test/captcha"

	// FixtureChangeLinkRef is the link on the simple page.
	FixtureChangeLinkRef = "e1"
	// FixtureUsernameRef is the username box.
	FixtureUsernameRef = "e2"
	// FixturePasswordRef is the password box.
	FixturePasswordRef = "e3"
)

// ErrSettleTimeout is what the fake worker returns when a test told it the page
// would never settle.
var ErrSettleTimeout = errors.New("the page did not settle before the limit, so read it again and see what it is doing")

// ErrNoSuchReference means the element the model pointed at is not on the page,
// after every way of finding it again has been tried.
var ErrNoSuchReference = errors.New("that element is not on this page, so read the page again and use a reference from the new snapshot")

// ErrNoPageOpen means nothing has been opened yet, so there is nothing to act
// on. The protocol answers it with -32002.
var ErrNoPageOpen = errors.New("no page is open in the browser, so open one before acting on it")

// ErrBrowserGone means the browser the worker was driving is not there any more.
// The protocol answers it with -32003, which tells the Go side to start the
// worker again.
var ErrBrowserGone = errors.New("the browser worker is closed, so start it again before opening a page")

// FakeBrowserWorker is a browser that never opens one: it moves between fixture
// pages, records what was typed, and does on command the six things that go
// wrong on a real page.
type FakeBrowserWorker struct {
	guard sync.Mutex
	pages map[string]contract.Snapshot
	links map[string]string

	current     string
	tabs        []contract.Tab
	previous    []contract.Element
	typed       map[string][]string
	stale       map[string]bool
	recoveries  int
	closed      bool
	nextProblem browserProblem
	openDialog  *contract.Dialog
	answers     []DialogAnswer
}

// DialogAnswer is one answer a test gave through the Dialog method.
type DialogAnswer struct {
	// Action is accept or dismiss.
	Action contract.DialogAction
	// Text is what was typed for a prompt.
	Text string
}

// browserProblem is the one thing a test told the worker to do wrong next.
type browserProblem struct {
	changeNothing bool
	neverSettle   bool
	cannotRead    bool
	dialog        *contract.Dialog
	download      *contract.Download
	newTab        string
}

// NewFakeBrowserWorker returns a worker holding the five fixture pages.
func NewFakeBrowserWorker() *FakeBrowserWorker {
	worker := &FakeBrowserWorker{
		pages: fixturePages(),
		links: map[string]string{FixtureChangeLinkRef: FixtureChangedPage},
		tabs:  []contract.Tab{{ID: "t1", Active: true}},
		typed: map[string][]string{},
		stale: map[string]bool{},
	}
	return worker
}

// MakeReferenceStale makes one reference stop working, so that a test can prove
// the worker finds the element again by its role and name.
func (worker *FakeBrowserWorker) MakeReferenceStale(ref string) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.stale[ref] = true
}

// StaleRecoveries is how many times the worker had to find an element again.
func (worker *FakeBrowserWorker) StaleRecoveries() int {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return worker.recoveries
}

// NextActionChangesNothing makes the next action leave the page as it was.
func (worker *FakeBrowserWorker) NextActionChangesNothing() {
	worker.setProblem(browserProblem{changeNothing: true})
}

// NextActionTimesOutSettling makes the page keep changing past the limit. The
// action still returns a diff, of the page as it stood, with settled false, the
// way worker/browser/PROTOCOL.md says a live page is handled.
func (worker *FakeBrowserWorker) NextActionTimesOutSettling() {
	worker.setProblem(browserProblem{neverSettle: true})
}

// NextActionCannotBeRead makes the page unreadable after the limit, which is the
// one settling failure that is an error rather than a diff, and the one the
// protocol answers with -32001.
func (worker *FakeBrowserWorker) NextActionCannotBeRead() {
	worker.setProblem(browserProblem{cannotRead: true})
}

// NextActionOpensADialog makes the next action open a dialog box.
func (worker *FakeBrowserWorker) NextActionOpensADialog(dialog contract.Dialog) {
	worker.setProblem(browserProblem{dialog: &dialog})
}

// NextActionStartsADownload makes the next action start a download.
func (worker *FakeBrowserWorker) NextActionStartsADownload(download contract.Download) {
	worker.setProblem(browserProblem{download: &download})
}

// NextActionOpensATab makes the next action open another tab.
func (worker *FakeBrowserWorker) NextActionOpensATab(tabID string) {
	worker.setProblem(browserProblem{newTab: tabID})
}

// TypedInto is everything that was typed into one element, in order.
func (worker *FakeBrowserWorker) TypedInto(ref string) []string {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	copied := make([]string, len(worker.typed[ref]))
	copy(copied, worker.typed[ref])
	return copied
}

// setProblem records the one thing the next action does wrong.
func (worker *FakeBrowserWorker) setProblem(problem browserProblem) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.nextProblem = problem
}

// takeProblem takes the next problem and leaves the worker behaving well.
func (worker *FakeBrowserWorker) takeProblem() browserProblem {
	problem := worker.nextProblem
	worker.nextProblem = browserProblem{}
	return problem
}

// Open goes to a fixture page.
func (worker *FakeBrowserWorker) Open(_ context.Context, address string) (contract.Snapshot, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if worker.closed {
		return contract.Snapshot{}, ErrBrowserGone
	}
	page, found := worker.pages[address]
	if !found {
		return contract.Snapshot{}, fmt.Errorf("the fixture browser has no page at %q, so add one with AddPage first", address)
	}
	worker.current = address
	worker.previous = page.Elements
	return page, nil
}

// Read returns the page the worker is on.
func (worker *FakeBrowserWorker) Read(_ context.Context, options contract.ReadOptions) (contract.Snapshot, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	page, err := worker.page()
	if err != nil {
		return contract.Snapshot{}, err
	}
	if options.VisibleOnly {
		page.BelowFold = 0
	}
	return page, nil
}

// Click clicks one element, finding it again when its reference has gone stale.
func (worker *FakeBrowserWorker) Click(_ context.Context, ref string, expectation string) (contract.Diff, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if err := worker.findable(ref); err != nil {
		return contract.Diff{}, err
	}
	return worker.actAndSettle(worker.links[ref], expectation)
}

// Type types into one element and records what was typed.
func (worker *FakeBrowserWorker) Type(_ context.Context, ref string, text string, expectation string) (contract.Diff, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if err := worker.findable(ref); err != nil {
		return contract.Diff{}, err
	}
	worker.typed[ref] = append(worker.typed[ref], text)
	return worker.actAndSettle("", expectation)
}

// Press presses one key.
func (worker *FakeBrowserWorker) Press(_ context.Context, _ string, expectation string) (contract.Diff, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return worker.actAndSettle("", expectation)
}

// Scroll scrolls the page.
func (worker *FakeBrowserWorker) Scroll(_ context.Context, _ contract.ScrollDirection, _ int, expectation string) (contract.Diff, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return worker.actAndSettle("", expectation)
}

// Act runs a batch of steps and stops at the first one that fails.
func (worker *FakeBrowserWorker) Act(ctx context.Context, steps []contract.ActStep) ([]contract.Diff, error) {
	diffs := []contract.Diff{}
	for _, step := range steps {
		diff, err := worker.oneStep(ctx, step)
		if err != nil {
			return diffs, err
		}
		diffs = append(diffs, diff)
		if !diff.ExpectationMet {
			return diffs, nil
		}
	}
	return diffs, nil
}

// oneStep runs one step of a batch.
func (worker *FakeBrowserWorker) oneStep(ctx context.Context, step contract.ActStep) (contract.Diff, error) {
	switch step.Method {
	case "click":
		return worker.Click(ctx, step.Ref, step.Expectation)
	case "type":
		return worker.Type(ctx, step.Ref, step.Text, step.Expectation)
	case "press":
		return worker.Press(ctx, step.Key, step.Expectation)
	default:
		return contract.Diff{}, fmt.Errorf("the batch asked for the method %q, so use click, type, or press", step.Method)
	}
}

// Tabs lists, switches, or closes tabs, and always returns the list afterwards.
func (worker *FakeBrowserWorker) Tabs(_ context.Context, action contract.TabAction, tabID string) ([]contract.Tab, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	switch action {
	case contract.TabList:
	case contract.TabSwitch:
		if err := worker.switchTo(tabID); err != nil {
			return nil, err
		}
	case contract.TabClose:
		worker.closeTab(tabID)
	default:
		return nil, fmt.Errorf("the tab action %q is not one of list, switch, or close", action)
	}
	copied := make([]contract.Tab, len(worker.tabs))
	copy(copied, worker.tabs)
	return copied, nil
}

// LoginFill types the credentials and returns a diff that holds none of them.
// The diff describes the whole login rather than the last keystroke, so an
// element the page grew while the form was being filled is reported as new, the
// way the real worker reports it.
func (worker *FakeBrowserWorker) LoginFill(ctx context.Context, fields contract.LoginFields) (contract.Diff, error) {
	worker.guard.Lock()
	beforeTheLogin := worker.previous
	worker.guard.Unlock()

	if _, err := worker.Type(ctx, fields.UsernameRef, fields.Username, "the username box holds the login name"); err != nil {
		return contract.Diff{}, err
	}
	if _, err := worker.Type(ctx, fields.PasswordRef, fields.Password, "the password box is filled"); err != nil {
		return contract.Diff{}, err
	}
	if fields.CodeRef != "" {
		if _, err := worker.Type(ctx, fields.CodeRef, fields.Code, "the code box is filled"); err != nil {
			return contract.Diff{}, err
		}
	}

	worker.guard.Lock()
	defer worker.guard.Unlock()
	diff, err := worker.actAndSettle("", "the login is accepted")
	if err != nil {
		return contract.Diff{}, err
	}
	diff.NewElements = elementsNotIn(diff.Snapshot.Elements, beforeTheLogin)
	return scrubCredentials(diff, fields), nil
}

// scrubCredentials takes every value that was typed back out of the diff, so
// that no method of the browser worker can ever hand a credential to the model.
//
// It works on the whole diff at once rather than on a list of fields, because a
// list of fields is a list somebody will forget to add to. The diff is turned
// into plain values, every piece of text in it is rewritten, and it is read back
// as a diff. A diff that cannot be written or read that way comes back holding
// nothing but the marker, because a credential must never escape a failure here.
func scrubCredentials(diff contract.Diff, fields contract.LoginFields) contract.Diff {
	hide := func(text string) string {
		for _, secret := range []string{fields.Password, fields.Code, fields.Username} {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, contract.RedactedMarker)
			}
		}
		return text
	}

	written, err := json.Marshal(diff)
	if err != nil {
		return contract.Diff{Seen: scrubFailureNote}
	}
	var opened any
	if err := json.Unmarshal(written, &opened); err != nil {
		return contract.Diff{Seen: scrubFailureNote}
	}
	rewritten, err := json.Marshal(rewriteEveryString(opened, hide))
	if err != nil {
		return contract.Diff{Seen: scrubFailureNote}
	}
	var scrubbed contract.Diff
	if err := json.Unmarshal(rewritten, &scrubbed); err != nil {
		return contract.Diff{Seen: scrubFailureNote}
	}
	return scrubbed
}

// scrubFailureNote is what a diff says when the scrubber could not read it. It
// is deliberately the only thing that comes back, because a diff that could not
// be scrubbed may still hold a credential.
const scrubFailureNote = "the browser worker could not check this diff for credentials, so it was thrown away"

// rewriteEveryString rewrites every piece of text inside a decoded piece of
// JSON, however deeply it is buried, and leaves the shape alone.
func rewriteEveryString(value any, rewrite func(text string) string) any {
	switch typed := value.(type) {
	case string:
		return rewrite(typed)
	case []any:
		for at, item := range typed {
			typed[at] = rewriteEveryString(item, rewrite)
		}
		return typed
	case map[string]any:
		for key, item := range typed {
			typed[key] = rewriteEveryString(item, rewrite)
		}
		return typed
	default:
		return value
	}
}

// Screenshot returns a picture with the clickable elements numbered.
func (worker *FakeBrowserWorker) Screenshot(_ context.Context) (contract.Screenshot, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	page, err := worker.page()
	if err != nil {
		return contract.Screenshot{}, err
	}
	marks := make([]contract.Mark, 0, len(page.Elements))
	for at, element := range page.Elements {
		marks = append(marks, contract.Mark{Number: at + 1, Ref: element.Ref, Role: element.Role, Name: element.Name})
	}
	return contract.Screenshot{PNGBase64: fixturePicture, Marks: marks}, nil
}

// Health says whether the worker can act on a page.
func (worker *FakeBrowserWorker) Health(_ context.Context) (contract.BrowserHealth, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if worker.closed {
		return contract.BrowserHealth{Healthy: false, Detail: "the fixture browser was closed"}, nil
	}
	return contract.BrowserHealth{Healthy: true, ChromeVersion: "fixture"}, nil
}

// Dialog answers the open dialog box, or refuses when none is open or the
// action is not one of the two.
func (worker *FakeBrowserWorker) Dialog(_ context.Context, action contract.DialogAction, text string) (contract.Diff, error) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	if !contract.KnownDialogAction(action) {
		return contract.Diff{}, fmt.Errorf("the dialog action %q is not one of accept or dismiss, so use one of those two", action)
	}
	if worker.openDialog == nil {
		return contract.Diff{}, fmt.Errorf("no dialog is open, so there is nothing to %s", action)
	}
	worker.openDialog = nil
	worker.answers = append(worker.answers, DialogAnswer{Action: action, Text: text})
	return worker.actAndSettle("", "")
}

// DialogAnswers is every answer given through Dialog, which a test reads.
func (worker *FakeBrowserWorker) DialogAnswers() []DialogAnswer {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	return append([]DialogAnswer(nil), worker.answers...)
}

// Close shuts the fixture browser down.
func (worker *FakeBrowserWorker) Close() error {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.closed = true
	return nil
}
