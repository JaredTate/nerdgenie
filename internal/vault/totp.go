package vault

import (
	"fmt"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// The shape of a two-factor code, which is what every site that hands out a
// shared secret expects unless it says otherwise.
const (
	// CodeDigits is how many digits a code has.
	CodeDigits = 6
	// CodeSeconds is how long one code lasts.
	CodeSeconds = 30
	// FreshCodeSeconds is the fewest seconds a code may have left before the
	// login tool should wait for the next one rather than type this one.
	FreshCodeSeconds = 5
)

// Code makes the two-factor code for one entry and says how many seconds are
// left before it changes. The login tool waits for the next code when fewer
// than FreshCodeSeconds remain, because a code typed at the last moment is
// often refused by the time the form is submitted.
func (vault *Vault) Code(name string) (string, int, error) {
	entry, held := vault.lookup(name)
	if !held {
		return "", 0, fmt.Errorf("the vault holds no entry named %q, so run /vault list to see what it does hold", name)
	}
	if entry.TOTPSecret == "" {
		return "", 0, fmt.Errorf("the entry %q has no two-factor secret, so add one with /vault add %s if the site asks for a code", name, name)
	}

	now := vault.now()
	code, err := totp.GenerateCodeCustom(entry.TOTPSecret, now, totp.ValidateOpts{
		Period:    CodeSeconds,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", 0, fmt.Errorf("the two-factor secret in the entry %q is not the base32 text a site hands out, so enter it again with /vault add %s: %w", name, name, err)
	}
	return code, CodeSeconds - int(now.Unix()%CodeSeconds), nil
}
