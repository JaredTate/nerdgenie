// Repairing a name that is a near miss for a real one is OpenClaw's design, read
// from its grammar and its name resolver at
// ~/Code/openclaw/packages/tool-call-repair/src/grammar.ts and written fresh
// here.

package repair

import (
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/coeus/internal/contract"
)

// The limits on a name repair. A name longer than the cap is not a name at all;
// an edit distance of more than two stops being a repair and starts being a
// guess; and below five characters even one edit changes too much of the word to
// tell "read" from "edit".
const (
	maxToolNameLength        = 120
	maxEditDistance          = 2
	minLengthForEditDistance = 5
)

// repairName reads the name the model wrote as the name of a real tool, trying
// four rules in order: the same name, the same name in another case, the same
// name with the underscores and hyphens taken out, and a name within two edits.
// A name that matches nothing, or that matches two real tools equally well,
// comes back as a problem for the model instead.
func repairName(written string, specs []contract.ToolSpec) (string, string) {
	name := strings.TrimSpace(written)
	if name == "" || len(name) > maxToolNameLength {
		return "", problemUnknownName(written, specs)
	}
	rules := []func(string, []contract.ToolSpec) []string{exactNames, sameLetterNames, plainNames, nearestNames}
	for _, matching := range rules {
		switch found := matching(name, specs); len(found) {
		case 0:
			continue
		case 1:
			return found[0], ""
		default:
			return "", problemAmbiguousName(written, found, specs)
		}
	}
	return "", problemUnknownName(written, specs)
}

// exactNames returns the real tools whose name is the written name.
func exactNames(name string, specs []contract.ToolSpec) []string {
	return matchingNames(specs, func(real string) bool { return real == name })
}

// sameLetterNames returns the real tools whose name differs only in case.
func sameLetterNames(name string, specs []contract.ToolSpec) []string {
	return matchingNames(specs, func(real string) bool { return strings.EqualFold(real, name) })
}

// plainNames returns the real tools whose name is the same once the case, the
// underscores, and the hyphens are taken out, which is what catches
// "BrowserOpen" and "browser-open".
func plainNames(name string, specs []contract.ToolSpec) []string {
	wanted := plainForm(name)
	return matchingNames(specs, func(real string) bool { return plainForm(real) == wanted })
}

// nearestNames returns the real tools within two edits of the written name, and
// only the closest ones, so that a tie is reported rather than guessed at. A
// name under five characters is never repaired this way.
func nearestNames(name string, specs []contract.ToolSpec) []string {
	if utf8.RuneCountInString(name) < minLengthForEditDistance {
		return nil
	}
	closest := maxEditDistance + 1
	found := []string{}
	for _, spec := range specs {
		distance := editDistance(strings.ToLower(name), strings.ToLower(spec.Name))
		if distance > maxEditDistance || distance > closest {
			continue
		}
		if distance < closest {
			closest, found = distance, []string{}
		}
		found = append(found, spec.Name)
	}
	return found
}

// matchingNames returns the names of the tools the test says match.
func matchingNames(specs []contract.ToolSpec, matches func(real string) bool) []string {
	found := []string{}
	for _, spec := range specs {
		if matches(spec.Name) {
			found = append(found, spec.Name)
		}
	}
	return found
}

// plainForm is a name with nothing left but its letters and digits in one case,
// which is how two spellings of the same name are compared.
func plainForm(name string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(name))
}

// editDistance counts the single-character insertions, deletions, and
// replacements it takes to turn one name into the other.
func editDistance(left string, right string) int {
	first := []rune(left)
	second := []rune(right)
	previous := make([]int, len(second)+1)
	current := make([]int, len(second)+1)
	for index := range previous {
		previous[index] = index
	}
	for row := 1; row <= len(first); row++ {
		current[0] = row
		for column := 1; column <= len(second); column++ {
			cost := 1
			if first[row-1] == second[column-1] {
				cost = 0
			}
			current[column] = min(previous[column]+1, current[column-1]+1, previous[column-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(second)]
}

// findSpec returns the specification of the named tool, and an empty one holding
// only the name when the caller asks for a tool that is not there.
func findSpec(specs []contract.ToolSpec, name string) contract.ToolSpec {
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	return contract.ToolSpec{Name: name}
}
