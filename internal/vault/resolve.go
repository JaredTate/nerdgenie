package vault

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Resolve turns a "secret://name" reference into the login behind it, for the
// harness alone: the browser's login tool and the provider's key lookup. No
// model-facing code calls this, and the tool registry is built so that none
// can.
func (vault *Vault) Resolve(_ context.Context, reference string) (contract.Credential, error) {
	name, isReference := contract.SecretReferenceName(reference)
	if !isReference {
		return contract.Credential{}, fmt.Errorf("%q is not a secret reference, so write it as %sname with the name of a vault entry", reference, contract.SecretReferencePrefix)
	}

	entry, held := vault.lookup(name)
	if !held {
		return contract.Credential{}, fmt.Errorf("the vault holds no secret named %q, so add it with /vault add %s in the terminal first", name, name)
	}
	return entry.Credential(), nil
}

// SudoPassword returns the machine password for a command the user has approved
// to run with sudo. Only the harness calls it, and the shell tool passes it to
// sudo through the askpass program rather than on a command line.
func (vault *Vault) SudoPassword(_ context.Context) (string, error) {
	entry, held := vault.lookup(SudoEntryName)
	if !held || entry.Password == "" {
		return "", errors.New("the vault holds no sudo password, so add one with /vault add sudo in the terminal before approving a command that needs it")
	}
	return entry.Password, nil
}

// lookup finds one entry by name. It returns a copy, so that a caller cannot
// change what the vault holds by holding on to what it was given.
func (vault *Vault) lookup(name string) (Entry, bool) {
	vault.guard.Lock()
	defer vault.guard.Unlock()

	at := slices.IndexFunc(vault.entries, func(held Entry) bool { return held.Name == name })
	if at < 0 {
		return Entry{}, false
	}
	found := vault.entries[at]
	found.Domains = slices.Clone(found.Domains)
	return found, true
}
