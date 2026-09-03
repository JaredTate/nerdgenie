package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// CheckBrowserWorker asserts what every browser worker promises: it says whether
// it is healthy, it refuses to act before a page is open, a reference that is
// not on the page is an error, every action returns a fresh snapshot and says
// whether the page settled, a page that is a wall says so on the snapshot before
// any action, a dialog action nobody defined is refused, and a login never gives
// the credentials back.
func CheckBrowserWorker(ctx context.Context, worker contract.BrowserWorker) error {
	health, err := worker.Health(ctx)
	if err != nil {
		return fmt.Errorf("asking the worker whether it is healthy failed: %w", err)
	}
	if !health.Healthy && health.Detail == "" {
		return errors.New("the worker says it is unhealthy and does not say why, and the user has to be told what to fix")
	}

	if _, err := worker.Click(ctx, "e1", "anything at all"); err == nil {
		return errors.New("clicking before a page was open returned no error, and there is nothing to click on")
	}

	snapshot, err := worker.Open(ctx, FixtureSimplePage)
	if err != nil {
		return fmt.Errorf("opening the simple fixture page failed: %w", err)
	}
	if snapshot.URL == "" || len(snapshot.Elements) == 0 {
		return fmt.Errorf("the page came back as %+v, want an address and some elements", snapshot)
	}
	for _, element := range snapshot.Elements {
		if _, valid := contract.ParseResultID(strings.Replace(element.Ref, "e", "r", 1)); !valid {
			return fmt.Errorf("the element reference %q is not the shape the protocol shows, such as e12", element.Ref)
		}
	}

	if _, err := worker.Click(ctx, "e999", "anything at all"); err == nil {
		return errors.New("clicking a reference that is not on the page returned no error, and it must name the reference")
	}
	if err := checkBrowserWalls(ctx, worker); err != nil {
		return err
	}
	if err := checkBrowserDialog(ctx, worker); err != nil {
		return err
	}
	return checkBrowserLogin(ctx, worker)
}

// checkBrowserWalls asserts that a page which is a wall says so on the snapshot,
// before any action is taken, which is what lets the harness hand the browser to
// the user rather than typing into a login form it did not recognise.
func checkBrowserWalls(ctx context.Context, worker contract.BrowserWorker) error {
	for page, kind := range map[string]contract.WallKind{
		FixtureLoginPage:     contract.WallLogin,
		FixtureTwoFactorPage: contract.WallTwoFactor,
		FixtureCaptchaPage:   contract.WallCaptcha,
	} {
		opened, err := worker.Open(ctx, page)
		if err != nil {
			return fmt.Errorf("opening the %s fixture page failed: %w", kind, err)
		}
		if opened.Wall == nil || opened.Wall.Kind != kind {
			return fmt.Errorf("opening %s reported the wall %+v, and a page that is a wall says so on its snapshot", page, opened.Wall)
		}
		read, err := worker.Read(ctx, contract.ReadOptions{})
		if err != nil {
			return fmt.Errorf("reading the %s fixture page failed: %w", kind, err)
		}
		if read.Wall == nil || read.Wall.Kind != kind {
			return fmt.Errorf("reading %s reported the wall %+v, and a read says what open said", page, read.Wall)
		}
	}
	return nil
}

// checkBrowserDialog asserts the twelfth method: an action nobody defined is
// refused, and answering a dialog nobody opened is refused too, because a tab
// with a dialog on it is blocked until the dialog is answered.
func checkBrowserDialog(ctx context.Context, worker contract.BrowserWorker) error {
	if _, err := worker.Open(ctx, FixtureSimplePage); err != nil {
		return fmt.Errorf("opening the simple fixture page failed: %w", err)
	}
	if _, err := worker.Dialog(ctx, "maybe", ""); err == nil {
		return errors.New("a dialog action of \"maybe\" was accepted, and the two actions are accept and dismiss")
	}
	if _, err := worker.Dialog(ctx, contract.DialogAccept, ""); err == nil {
		return errors.New("answering a dialog nobody opened returned no error, and there is nothing to answer")
	}

	diff, err := worker.Click(ctx, FixtureChangeLinkRef, "the page changed")
	if err != nil {
		return fmt.Errorf("clicking the link on the simple fixture page failed: %w", err)
	}
	if !diff.Settled {
		return fmt.Errorf("a click on a page that comes to rest says it did not settle: %+v", diff)
	}
	if diff.Snapshot.URL == "" {
		return fmt.Errorf("a click came back with no snapshot of the page it left behind: %+v", diff)
	}
	return nil
}

// checkBrowserLogin asserts the one promise that matters most: no method of the
// browser worker ever hands a credential back.
func checkBrowserLogin(ctx context.Context, worker contract.BrowserWorker) error {
	if _, err := worker.Open(ctx, FixtureLoginPage); err != nil {
		return fmt.Errorf("opening the login fixture page failed: %w", err)
	}
	password := "the contract check's password"
	diff, err := worker.LoginFill(ctx, contract.LoginFields{
		UsernameRef: FixtureUsernameRef,
		PasswordRef: FixturePasswordRef,
		Username:    "contract-check",
		Password:    password,
	})
	if err != nil {
		return fmt.Errorf("filling the login form failed: %w", err)
	}

	// The whole diff as JSON is the only haystack that cannot miss a field
	// somebody forgot to scrub, which is exactly how the password used to get out
	// through the list of elements that appeared.
	whole, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("cannot read the diff from a login back as JSON to check it for credentials: %w", err)
	}
	if strings.Contains(string(whole), password) {
		return errors.New("the diff from a login holds the password, and no method may ever hand a credential back")
	}
	return nil
}

// CheckDesktop asserts what every desktop promises: an application the user has
// not granted is refused, and nothing can be done before something is open.
func CheckDesktop(ctx context.Context, desktop contract.Desktop) error {
	if err := desktop.Launch(ctx, "an-application-nobody-granted", "the application opens"); err == nil {
		return errors.New("an application nobody granted was launched, and the user grants an application once per session")
	}
	if err := desktop.Click(ctx, 1, "the control is pressed"); err == nil {
		return errors.New("a click landed with no application open, and there is nothing to click on")
	}
	if _, err := desktop.Screenshot(ctx); err == nil {
		return errors.New("a screenshot was taken with no application open, and there is nothing to photograph")
	}
	return nil
}
