// Repairing a name that is a near miss for a real one is OpenClaw's design, read
// from its grammar and its name resolver at
// ~/Code/openclaw/packages/tool-call-repair/src/grammar.ts and written fresh
// here.

package repair

import (
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
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

// repairName reads the name the model wrote as a real tool, trying four rules in
// order: the same name, the same name in another case, the same name with the
// underscores and hyphens taken out, and a name within two edits. A name that
// matches nothing, or that matches two real tools equally well, comes back as a
// problem for the model instead.
func repairName(written string, specs []contract.ToolSpec) (contract.ToolSpec, string) {
	name := strings.TrimSpace(written)
	if name == "" || len(name) > maxToolNameLength {
		return contract.ToolSpec{}, problemUnknownName(written, specs)
	}
	rules := []func(string, []contract.ToolSpec) []contract.ToolSpec{exact, sameLetters, plain, nearest}
	for _, matching := range rules {
		switch found := matching(name, specs); len(found) {
		case 0:
			continue
		case 1:
			return found[0], ""
		default:
			return contract.ToolSpec{}, problemAmbiguousName(written, namesOf(found), specs)
		}
	}
	return contract.ToolSpec{}, problemUnknownName(written, specs)
}

// exact returns the real tools whose name is the written name.
func exact(name string, specs []contract.ToolSpec) []contract.ToolSpec {
	return matchingSpecs(specs, func(real string) bool { return real == name })
}

// sameLetters returns the real tools whose name differs only in case.
func sameLetters(name string, specs []contract.ToolSpec) []contract.ToolSpec {
	return matchingSpecs(specs, func(real string) bool { return strings.EqualFold(real, name) })
}

// plain returns the real tools whose name is the same once the case, the
// underscores, and the hyphens are taken out, which is what catches
// "BrowserOpen" and "browser-open".
func plain(name string, specs []contract.ToolSpec) []contract.ToolSpec {
	wanted := plainForm(name)
	return matchingSpecs(specs, func(real string) bool { return plainForm(real) == wanted })
}

// nearest returns the real tools within two edits of the written name, and only
// the closest of them, so that a tie is reported rather than guessed at. A name
// under five characters is never repaired this way.
func nearest(name string, specs []contract.ToolSpec) []contract.ToolSpec {
	length := utf8.RuneCountInString(name)
	if length < minLengthForEditDistance {
		return nil
	}
	lowered := strings.ToLower(name)
	closest := maxEditDistance + 1
	found := []contract.ToolSpec{}
	for _, spec := range specs {
		// A name whose length differs by more than the edits allowed cannot be
		// within them, and leaving it out keeps the comparison bounded however
		// long a tool name turns out to be.
		if difference := length - utf8.RuneCountInString(spec.Name); difference > maxEditDistance || difference < -maxEditDistance {
			continue
		}
		distance := editDistance(lowered, strings.ToLower(spec.Name))
		if distance > maxEditDistance || distance > closest {
			continue
		}
		if distance < closest {
			closest, found = distance, []contract.ToolSpec{}
		}
		found = append(found, spec)
	}
	return found
}

// matchingSpecs returns the tools whose name the test says matches.
func matchingSpecs(specs []contract.ToolSpec, matches func(real string) bool) []contract.ToolSpec {
	found := []contract.ToolSpec{}
	for _, spec := range specs {
		if matches(spec.Name) {
			found = append(found, spec)
		}
	}
	return found
}

// namesOf is the names of a set of tools, for a message that has to list them.
func namesOf(specs []contract.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

// separators are the two characters a model puts between the words of a tool
// name, and the replacer is built once because it is used on every name.
var separators = strings.NewReplacer("_", "", "-", "")

// plainForm is a name with nothing left but its letters and digits in one case,
// which is how two spellings of the same name are compared.
func plainForm(name string) string {
	return separators.Replace(strings.ToLower(name))
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
