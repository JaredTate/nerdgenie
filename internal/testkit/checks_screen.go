package testkit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// CheckBrowserWorker asserts what every browser worker promises: it says whether
// it is healthy, it refuses to act before a page is open, a reference that is not
// on the page is an error, every action returns a fresh snapshot, and a login
// never gives the credentials back.
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
	return checkBrowserLogin(ctx, worker)
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

	whole := diff.URL + diff.Seen + diff.Snapshot.Title
	for _, element := range diff.Snapshot.Elements {
		whole += element.Name + element.Role
	}
	if strings.Contains(whole, password) {
		return errors.New("the diff from a login holds the password, and no method may ever hand a credential back")
	}
	return nil
}

// CheckDesktop asserts what every desktop promises: an application the user has
// not granted is refused, and nothing can be done before something is open.
func CheckDesktop(ctx context.Context, desktop contract.Desktop) error {
	if err := desktop.Launch(ctx, "an-application-nobody-granted"); err == nil {
		return errors.New("an application nobody granted was launched, and the user grants an application once per session")
	}
	if err := desktop.Click(ctx, 1); err == nil {
		return errors.New("a click landed with no application open, and there is nothing to click on")
	}
	if _, err := desktop.Screenshot(ctx); err == nil {
		return errors.New("a screenshot was taken with no application open, and there is nothing to photograph")
	}
	return nil
}
