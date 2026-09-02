package vault

import (
	"slices"
	"sort"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// minRedactedValueLength is the shortest stored value the redactor blacks out.
// A password of five characters or fewer is more likely to be an ordinary word
// that happens to be in the vault than a secret worth mangling every message
// for.
const minRedactedValueLength = 6

// Redact replaces every value the vault holds with "[redacted]". Every channel,
// the event log, and every tool result run their text through it on the way out
// of the program.
func (vault *Vault) Redact(text string) string {
	vault.guard.Lock()
	redactor := vault.redactor
	vault.guard.Unlock()

	if redactor != nil {
		text = redactor.Replace(text)
	}
	return text
}

// rebuildRedactor makes the replacer that blacks out the values the vault holds
// now. It is built once per change rather than once per message, because
// redaction runs on everything the program says. The caller already holds the
// lock.
func (vault *Vault) rebuildRedactor() {
	pairs := []string{}
	for _, value := range storedValues(vault.entries) {
		pairs = append(pairs, value, contract.RedactedMarker)
	}
	if len(pairs) == 0 {
		vault.redactor = nil
		return
	}
	vault.redactor = strings.NewReplacer(pairs...)
}

// storedValues returns every value worth blacking out, longest first, so that a
// short secret inside a longer one cannot leave half of the longer one showing.
func storedValues(entries []Entry) []string {
	found := []string{}
	for _, entry := range entries {
		for _, value := range []string{entry.Password, entry.TOTPSecret} {
			if len(value) >= minRedactedValueLength {
				found = append(found, value)
			}
		}
	}
	slices.Sort(found)
	found = slices.Compact(found)
	sort.SliceStable(found, func(left, right int) bool { return len(found[left]) > len(found[right]) })
	return found
}
