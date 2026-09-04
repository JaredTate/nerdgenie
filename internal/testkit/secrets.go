package testkit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/JaredTate/nerdgenie/internal/contract"
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
//
// Every match is found in the text as it arrived and the pieces are spliced
// together once at the end. Replacing one secret at a time over the growing text
// would let a short secret match inside a marker an earlier pass had written,
// and the sentence would come back mangled. The longest values are claimed
// first, so that one secret inside another is not left half visible.
func (secrets *FakeSecrets) Redact(text string) string {
	claimed := secrets.spansToHide(text)
	if len(claimed) == 0 {
		return text
	}

	var built strings.Builder
	at := 0
	for _, span := range claimed {
		built.WriteString(text[at:span.from])
		built.WriteString(contract.RedactedMarker)
		at = span.to
	}
	built.WriteString(text[at:])
	return built.String()
}

// span is one stretch of the text a secret was found in.
type span struct {
	// from is where the secret starts.
	from int
	// to is one past where it ends.
	to int
}

// spansToHide finds every place a secret appears in the text, longest secret
// first, and returns them in reading order with none overlapping another.
//
// A marker already in the text is claimed before anything else, so that a secret
// which happens to be spelled inside the word "redacted" cannot match there.
// That is what makes redacting text twice change nothing the second time.
func (secrets *FakeSecrets) spansToHide(text string) []span {
	claimed := []span{}
	for at := 0; at < len(text); {
		found := strings.Index(text[at:], contract.RedactedMarker)
		if found < 0 {
			break
		}
		start := at + found
		claimed = append(claimed, span{from: start, to: start + len(contract.RedactedMarker)})
		at = start + len(contract.RedactedMarker)
	}

	for _, value := range secrets.values() {
		at := 0
		for at < len(text) {
			found := strings.Index(text[at:], value)
			if found < 0 {
				break
			}
			start := at + found
			if !overlapsAny(claimed, start, start+len(value)) {
				claimed = append(claimed, span{from: start, to: start + len(value)})
			}
			at = start + len(value)
		}
	}
	sort.Slice(claimed, func(left, right int) bool { return claimed[left].from < claimed[right].from })
	return claimed
}

// overlapsAny says whether a stretch of the text runs into one already claimed
// by a longer secret.
func overlapsAny(claimed []span, from int, to int) bool {
	for _, taken := range claimed {
		if from < taken.to && taken.from < to {
			return true
		}
	}
	return false
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
