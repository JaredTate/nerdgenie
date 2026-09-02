package testkit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// FakeSecrets is the vault kept in memory: entries a test adds, a sudo password
// a test sets, and a redactor that blacks out every value it holds.
type FakeSecrets struct {
	guard         sync.Mutex
	entries       map[string]contract.Credential
	sudoPassword  string
	hasSudoSecret bool
}

// NewFakeSecrets returns an empty vault.
func NewFakeSecrets() *FakeSecrets {
	return &FakeSecrets{entries: map[string]contract.Credential{}}
}

// Add puts one credential in the vault under a name.
func (secrets *FakeSecrets) Add(name string, credential contract.Credential) {
	secrets.guard.Lock()
	defer secrets.guard.Unlock()
	secrets.entries[name] = credential
}

// SetSudoPassword sets the password a command approved to run with sudo is given.
func (secrets *FakeSecrets) SetSudoPassword(password string) {
	secrets.guard.Lock()
	defer secrets.guard.Unlock()
	secrets.sudoPassword = password
	secrets.hasSudoSecret = true
}

// Resolve turns a "secret://name" reference into the credential behind it.
func (secrets *FakeSecrets) Resolve(_ context.Context, reference string) (contract.Credential, error) {
	name, valid := contract.SecretReferenceName(reference)
	if !valid {
		return contract.Credential{}, fmt.Errorf("%q is not a secret reference, so write it as %sname", reference, contract.SecretReferencePrefix)
	}

	secrets.guard.Lock()
	defer secrets.guard.Unlock()
	credential, held := secrets.entries[name]
	if !held {
		return contract.Credential{}, fmt.Errorf("the vault holds no secret named %q, so add it with the vault command first", name)
	}
	return credential, nil
}

// SudoPassword returns the password for a command approved to run with sudo.
func (secrets *FakeSecrets) SudoPassword(_ context.Context) (string, error) {
	secrets.guard.Lock()
	defer secrets.guard.Unlock()
	if !secrets.hasSudoSecret {
		return "", errors.New("the vault holds no sudo password, so add one before approving a command that needs it")
	}
	return secrets.sudoPassword, nil
}

// Redact replaces every value the vault holds wherever it appears in the text.
// The longest values go first, so that one secret inside another is not left
// half visible.
func (secrets *FakeSecrets) Redact(text string) string {
	for _, value := range secrets.values() {
		text = strings.ReplaceAll(text, value, contract.RedactedMarker)
	}
	return text
}

// values returns every secret value the vault holds, longest first.
func (secrets *FakeSecrets) values() []string {
	secrets.guard.Lock()
	defer secrets.guard.Unlock()

	found := []string{}
	for _, credential := range secrets.entries {
		found = append(found, credential.Password, credential.TOTPSecret)
	}
	if secrets.hasSudoSecret {
		found = append(found, secrets.sudoPassword)
	}

	kept := []string{}
	for _, value := range found {
		if value != "" {
			kept = append(kept, value)
		}
	}
	sort.Slice(kept, func(left, right int) bool { return len(kept[left]) > len(kept[right]) })
	return kept
}
