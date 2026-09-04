package browser

import (
	"context"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The three functions internal/tool's browser tools are wired with. They are
// here rather than in serve.go because each one carries a rule that must not be
// written twice: a credential is only ever handed over for a page on one of its
// own domains, a two-factor code is only ever handed over fresh, and a handoff
// is only ever answered by the user through the channel.

// Credentials hands the login for one site to the browser login tool, and
// refuses when the browser is not on a page the vault entry names. It is
// browserlogin.Credentials.
func (browser *Browser) Credentials(ctx context.Context, site string) (contract.Credential, error) {
	credential, err := browser.options.Secrets.Resolve(ctx, contract.SecretReferencePrefix+site)
	if err != nil {
		return contract.Credential{}, err
	}
	page, err := browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return contract.Credential{}, err
	}
	if err := checkTheDomain(page.URL, credential.Domains); err != nil {
		return contract.Credential{}, err
	}
	return credential, nil
}

// TwoFactorCode hands a fresh two-factor code for one site to the browser login
// tool, waiting for the next thirty-second window when this one is nearly over.
// It is browserlogin.TwoFactorCode.
func (browser *Browser) TwoFactorCode(ctx context.Context, site string) (string, error) {
	return browser.freshCode(ctx, site)
}

// AskUser hands the browser to the user and gives back one line saying what they
// did with it. It is browserhandoff.AskUser.
func (browser *Browser) AskUser(ctx context.Context, reason string) (string, error) {
	reply, err := browser.Handoff(ctx, reason)
	if err != nil {
		return "", err
	}
	return reply.Note, nil
}
