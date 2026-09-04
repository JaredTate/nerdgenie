// Redaction follows Hermes' one redaction pass, at
// ~/Code/hermes-agent/agent/redact.py and its use in
// ~/Code/hermes-agent/tools/browser_tool.py, where the function to look for is
// _redact_browser_output: one list of secret shapes compiled once, applied to
// everything that leaves the program, whether or not the value is one the
// program itself knows. The list here is shorter and the pass is a single
// scan, because everything Coeus says goes through it.

package vault

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// minRedactedValueLength is the shortest stored value the redactor blacks out.
// A value of five characters or fewer is more likely to be an ordinary word
// that happens to be in the vault than a secret worth mangling every message
// for.
const minRedactedValueLength = 6

// secretShapes are the things that look like a secret whether or not the vault
// holds them. Each one has exactly two capture groups, the text to keep before
// the secret and the text to keep after it, so that one scan can rebuild the
// line with the middle blacked out. Order matters only where two shapes could
// start at the same place, and there the longer one is written first.
var secretShapes = []string{
	// A private key block, from its header line to its footer line.
	`()-----BEGIN[A-Z ]*PRIVATE KEY-----(?s:.*?)-----END[A-Z ]*PRIVATE KEY-----()`,
	// The credential after the scheme word of an Authorization header.
	`(\b(?i:authorization)\s*:\s*(?i:bearer)\s+)[^\s"',;]+()`,
	// The value of a password or token field in a query string or a form.
	`(\b(?i:password|token)=)[^&\s"';]+()`,
	// An Anthropic or OpenAI key, which covers sk-, sk-ant- and sk-proj-.
	`()\bsk-[A-Za-z0-9_-]{8,}()`,
	// A GitHub token, personal or from an OAuth flow.
	`()\bgh[pousr]_[A-Za-z0-9]{8,}()`,
	// A Slack token of any kind.
	`()\bxox[A-Za-z]-[A-Za-z0-9-]{8,}()`,
	// An AWS access key id.
	`()\bAKIA[A-Z0-9]{16}()`,
	// A six-digit code after the word that says what it is.
	`((?i:\b(?:code|otp)\b)[^\n\d]{0,40})\b\d{6}\b()`,
	// A six-digit code before the word that says what it is.
	`()\b\d{6}\b([^\n\d]{0,40}(?i:\b(?:code|otp)\b))`,
}

// secretShapePattern is every shape joined into one expression, compiled once
// when the program starts, so that redacting a message is one scan and no
// compilation at all.
var secretShapePattern = regexp.MustCompile(strings.Join(secretShapes, "|"))

// Redact replaces every value the vault holds, and everything that looks like a
// secret whether the vault holds it or not, with "[redacted]". Every channel,
// the event log, and every tool result run their text through it on the way out
// of the program.
func (vault *Vault) Redact(text string) string {
	vault.guard.Lock()
	redactor := vault.redactor
	vault.guard.Unlock()

	if redactor != nil {
		text = redactor.Replace(text)
	}
	return redactShapes(text)
}

// redactShapes blacks out everything that looks like a secret, in one scan of
// the text, keeping the part of each match that says what was there.
func redactShapes(text string) string {
	found := secretShapePattern.FindAllStringSubmatchIndex(text, -1)
	if len(found) == 0 {
		return text
	}

	var rebuilt strings.Builder
	rebuilt.Grow(len(text))
	upTo := 0
	for _, match := range found {
		before, after := keptParts(text, match)
		rebuilt.WriteString(text[upTo:match[0]])
		rebuilt.WriteString(before)
		rebuilt.WriteString(contract.RedactedMarker)
		rebuilt.WriteString(after)
		upTo = match[1]
	}
	rebuilt.WriteString(text[upTo:])
	return rebuilt.String()
}

// keptParts returns the text the matched shape keeps on either side of the
// secret. Only the two groups of the shape that matched have positions; the
// groups of every other shape are absent.
func keptParts(text string, match []int) (string, string) {
	kept := []string{}
	for group := 1; group*2+1 < len(match); group++ {
		start, end := match[group*2], match[group*2+1]
		if start >= 0 && end >= start {
			kept = append(kept, text[start:end])
		}
	}
	if len(kept) != 2 {
		return "", ""
	}
	return kept[0], kept[1]
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
