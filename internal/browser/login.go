package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/vault"
)

// shortestGuardedSecret is the shortest value the leak guard looks for in an
// answer. It is six characters because that is the floor the vault's own
// redactor uses, and a shorter value would fire on ordinary page text.
const shortestGuardedSecret = 6

// LoginRefs are the boxes the model pointed at. Any of them may be left empty,
// and an empty one is found on the current page by its role and its name.
type LoginRefs struct {
	// UsernameRef is the box the login name goes in.
	UsernameRef string
	// PasswordRef is the box the password goes in.
	PasswordRef string
	// CodeRef is the box the second code goes in.
	CodeRef string
}

// Login fills the login form on the page the browser is on from the vault entry
// with that name, and reports what happened or the wall it hit.
//
// The model never sees a value. It names the entry and, if it likes, points at
// the boxes; the vault hands the credential to this package, this package checks
// that the page is on one of the entry's own domains, makes a fresh two-factor
// code, and the worker types all three itself. Nothing that comes back holds a
// value, and nothing that is logged holds one either.
func (browser *Browser) Login(ctx context.Context, site string, chosen LoginRefs) (contract.Diff, error) {
	credential, err := browser.options.Secrets.Resolve(ctx, contract.SecretReferencePrefix+site)
	if err != nil {
		return contract.Diff{}, err
	}
	page, err := browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return contract.Diff{}, err
	}
	if err := checkTheDomain(page.URL, credential.Domains); err != nil {
		return contract.Diff{}, err
	}
	fields, err := browser.loginFields(ctx, site, page, chosen, credential)
	if err != nil {
		return contract.Diff{}, err
	}

	diff, err := browser.LoginFill(ctx, fields)
	if err != nil {
		return contract.Diff{}, err
	}
	browser.noteWhatTheLoginDid(site, page.URL, diff)
	return diff, nil
}

// noteWhatTheLoginDid writes one line about the login that names the entry, the
// site, and the wall if there was one, and never a value.
func (browser *Browser) noteWhatTheLoginDid(site string, address string, diff contract.Diff) {
	if diff.Wall != nil {
		browser.options.Note("the login on %s from the vault entry %q met a %s wall: %s",
			hostnameOf(address), site, diff.Wall.Kind, diff.Wall.Detail)
		return
	}
	browser.options.Note("the login on %s from the vault entry %q was filled and submitted", hostnameOf(address), site)
}

// checkTheDomain refuses a login on any page but the ones the vault entry names,
// which is what stops a page that says it is a bank from being handed the
// password for a bank. The hostname it refused is named, so that the model can
// see what it was on.
func checkTheDomain(address string, domains []string) error {
	host := hostnameOf(address)
	if host == "" {
		return fmt.Errorf("the browser is on %q, which has no hostname, so open the site's own login page before logging in", address)
	}
	if len(domains) == 0 {
		return fmt.Errorf("that vault entry names no domains, so add them with the vault command before logging in on %s", host)
	}
	for _, domain := range domains {
		allowed := hostnameOf("https://" + strings.TrimSpace(domain))
		if allowed == "" {
			continue
		}
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return fmt.Errorf("the browser is on %s and that vault entry may only be typed into %s, so open one of those before logging in",
		host, strings.Join(domains, ", "))
}

// loginFields works out which boxes to fill and what to put in them.
func (browser *Browser) loginFields(ctx context.Context, site string, page contract.Snapshot,
	chosen LoginRefs, credential contract.Credential) (contract.LoginFields, error) {
	username, err := chooseBox(page, chosen.UsernameRef, usernameWords, "the box the login name goes in")
	if err != nil {
		return contract.LoginFields{}, err
	}
	password, err := chooseBox(page, chosen.PasswordRef, passwordWords, "the box the password goes in", username)
	if err != nil {
		return contract.LoginFields{}, err
	}

	fields := contract.LoginFields{
		UsernameRef: username,
		PasswordRef: password,
		Username:    credential.Username,
		Password:    credential.Password,
	}
	code := chosen.CodeRef
	if code == "" {
		code = findBox(page, codeWords, username, password)
	}
	if code == "" || credential.TOTPSecret == "" {
		return fields, nil
	}
	made, err := browser.freshCode(ctx, site)
	if err != nil {
		return contract.LoginFields{}, err
	}
	fields.CodeRef, fields.Code = code, made
	return fields, nil
}

// freshCode makes the two-factor code for one vault entry, waiting for the next
// thirty-second window when this one is nearly over, because a code typed at the
// last moment is often refused by the time the form is submitted.
func (browser *Browser) freshCode(ctx context.Context, site string) (string, error) {
	if browser.options.Codes == nil {
		return "", fmt.Errorf("the vault entry %q has a two-factor secret but nothing was wired to make codes, which is a fault in how Coeus was started", site)
	}
	code, secondsLeft, err := browser.options.Codes.Code(site)
	if err != nil {
		return "", err
	}
	if secondsLeft >= vault.FreshCodeSeconds {
		return code, nil
	}
	browser.options.Note("the two-factor code for %q had %d seconds left, so the browser waited for the next one", site, secondsLeft)
	if err := browser.options.Clock.Sleep(ctx, time.Duration(secondsLeft+1)*time.Second); err != nil {
		return "", fmt.Errorf("waiting for a fresh two-factor code for %q was cut short: %w", site, err)
	}
	code, _, err = browser.options.Codes.Code(site)
	if err != nil {
		return "", err
	}
	return code, nil
}

// checkNothingLeaked reads the whole answer back as text and refuses it if any
// value the worker was given survives anywhere in it. The worker promises to
// take them out, and the fake worker does the same; this is the last check
// between a credential and the model's context, and it throws the answer away
// rather than letting one through.
func checkNothingLeaked(diff contract.Diff, fields contract.LoginFields) error {
	written, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("the answer to the login could not be checked for credentials, so it was thrown away: %w", err)
	}
	for _, value := range []string{fields.Password, fields.Code, fields.Username} {
		if len(value) < shortestGuardedSecret {
			continue
		}
		if strings.Contains(string(written), value) {
			return fmt.Errorf("the browser worker's answer to the login still held a credential, so it was thrown away: read the page again to see where the login got to")
		}
	}
	return nil
}
