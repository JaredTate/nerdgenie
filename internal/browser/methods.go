package browser

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/JaredTate/coeus/internal/contract"
)

// Open goes to a page and returns its snapshot. It is an action, so it counts
// against the daily budget of the site it is going to.
func (browser *Browser) Open(ctx context.Context, address string) (contract.Snapshot, error) {
	if err := browser.budget.charge(hostnameOf(address), 1); err != nil {
		return contract.Snapshot{}, err
	}
	var page contract.Snapshot
	if err := browser.call(ctx, "open", map[string]any{"url": address}, &page); err != nil {
		return contract.Snapshot{}, err
	}
	browser.rememberPage(page.URL)
	return page, nil
}

// Read returns a fresh snapshot of the current page. Reading is not an action,
// so it costs nothing against the budget.
func (browser *Browser) Read(ctx context.Context, options contract.ReadOptions) (contract.Snapshot, error) {
	var page contract.Snapshot
	if err := browser.call(ctx, "read", map[string]any{"visibleOnly": options.VisibleOnly}, &page); err != nil {
		return contract.Snapshot{}, err
	}
	browser.rememberPage(page.URL)
	return page, nil
}

// Click clicks one element and checks what the model said it expected.
func (browser *Browser) Click(ctx context.Context, ref string, expectation string) (contract.Diff, error) {
	return browser.act(ctx, "click", map[string]any{"ref": ref, "expectation": expectation})
}

// Type types into one element, one key at a time, and checks the expectation.
func (browser *Browser) Type(ctx context.Context, ref string, text string, expectation string) (contract.Diff, error) {
	return browser.act(ctx, "type", map[string]any{"ref": ref, "text": text, "expectation": expectation})
}

// Press presses one key or key combination and checks the expectation.
func (browser *Browser) Press(ctx context.Context, key string, expectation string) (contract.Diff, error) {
	return browser.act(ctx, "press", map[string]any{"key": key, "expectation": expectation})
}

// Scroll scrolls the page in steps, the way a person does, and checks the
// expectation.
func (browser *Browser) Scroll(ctx context.Context, direction contract.ScrollDirection, amount int, expectation string) (contract.Diff, error) {
	return browser.act(ctx, "scroll", map[string]any{
		"direction":   string(direction),
		"amount":      amount,
		"expectation": expectation,
	})
}

// Dialog answers the open dialog box, because Chrome blocks the whole tab until
// one is answered.
func (browser *Browser) Dialog(ctx context.Context, action contract.DialogAction, text string) (contract.Diff, error) {
	if !contract.KnownDialogAction(action) {
		return contract.Diff{}, errors.New("a dialog is answered with accept or dismiss, so ask for one of those two")
	}
	return browser.act(ctx, "dialog", map[string]any{"action": string(action), "text": text})
}

// Act runs a short batch of steps and stops as soon as one expectation fails.
// Every step that ran counts against the budget, and the diffs of the steps that
// ran come back even when a later one was refused.
func (browser *Browser) Act(ctx context.Context, steps []contract.ActStep) ([]contract.Diff, error) {
	if len(steps) == 0 {
		return nil, errors.New("a batch with no steps in it does nothing, so give it at least one step")
	}
	if err := browser.budget.charge(browser.hostnameNow(), len(steps)); err != nil {
		return nil, err
	}
	var answer diffsAnswer
	err := browser.call(ctx, "act", map[string]any{"steps": steps}, &answer)
	if err != nil {
		return diffsSoFar(err), err
	}
	browser.rememberLastDiff(answer.Diffs)
	return answer.Diffs, nil
}

// diffsSoFar reads the diffs of the steps that ran out of a refusal, which is
// where worker/browser/PROTOCOL.md says a stopped batch reports them.
func diffsSoFar(err error) []contract.Diff {
	var refused *RefusedError
	if !errors.As(err, &refused) || len(refused.Data) == 0 {
		return nil
	}
	var answer diffsAnswer
	if err := json.Unmarshal(refused.Data, &answer); err != nil {
		return nil
	}
	return answer.Diffs
}

// rememberLastDiff keeps the address the last step of a batch left the browser
// on.
func (browser *Browser) rememberLastDiff(diffs []contract.Diff) {
	if len(diffs) == 0 {
		return
	}
	browser.rememberPage(diffs[len(diffs)-1].Snapshot.URL)
}

// Tabs lists, switches, or closes tabs and returns the list afterwards. Working
// with tabs is not an action on a site, so it costs nothing against the budget.
func (browser *Browser) Tabs(ctx context.Context, action contract.TabAction, tabID string) ([]contract.Tab, error) {
	var answer tabsAnswer
	params := map[string]any{"action": string(action), "tabId": tabID}
	if err := browser.call(ctx, "tabs", params, &answer); err != nil {
		return nil, err
	}
	return answer.Tabs, nil
}

// Screenshot returns the page as a picture with its clickable elements numbered.
func (browser *Browser) Screenshot(ctx context.Context) (contract.Screenshot, error) {
	var picture contract.Screenshot
	if err := browser.call(ctx, "screenshot", map[string]any{}, &picture); err != nil {
		return contract.Screenshot{}, err
	}
	return picture, nil
}

// LoginFill types a username, a password, and a code into the fields it is
// given, and returns a diff that holds none of them. Every path that fills a
// login form comes through here, so the check that no value survived the answer
// is here too. Login is the fuller thing: it finds the credential in the vault
// and the boxes on the page first.
func (browser *Browser) LoginFill(ctx context.Context, fields contract.LoginFields) (contract.Diff, error) {
	if err := browser.budget.charge(browser.hostnameNow(), 1); err != nil {
		return contract.Diff{}, err
	}
	var diff contract.Diff
	if err := browser.call(ctx, "loginFill", fields, &diff); err != nil {
		return contract.Diff{}, err
	}
	if err := checkNothingLeaked(diff, fields); err != nil {
		return contract.Diff{}, err
	}
	browser.rememberPage(diff.Snapshot.URL)
	return diff, nil
}

// Health says whether the worker is alive and what it is driving.
func (browser *Browser) Health(ctx context.Context) (contract.BrowserHealth, error) {
	var health contract.BrowserHealth
	if err := browser.call(ctx, "health", map[string]any{}, &health); err != nil {
		return contract.BrowserHealth{}, err
	}
	return health, nil
}

// act runs one action on the page: it charges the site's budget first, then
// calls the method and keeps the address the page ended on.
func (browser *Browser) act(ctx context.Context, method string, params map[string]any) (contract.Diff, error) {
	if err := browser.budget.charge(browser.hostnameNow(), 1); err != nil {
		return contract.Diff{}, err
	}
	var diff contract.Diff
	if err := browser.call(ctx, method, params, &diff); err != nil {
		return contract.Diff{}, err
	}
	browser.rememberPage(diff.Snapshot.URL)
	return diff, nil
}
