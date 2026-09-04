// The rulebook is OpenCode's permission engine written fresh in Go: rules of
// tool, pattern, and action, and the last one that matches a call wins. The
// engine is at ~/Code/opencode/packages/opencode/src/permission/index.ts and the
// shape of a rule is at packages/core/src/v1/config/permission.ts in the same
// project.

package permission

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The caps on a rulebook. A rule list comes from a file a person wrote, so it is
// small; these numbers are there to stop a mistake, not a person.
const (
	// MaxRules is how many rules one rulebook may hold.
	MaxRules = 500
	// MaxPatternRunes is how long one pattern may be.
	MaxPatternRunes = 200
)

// Rule is one line of the rulebook: the tool it covers, the readable forms it
// matches, what to do about them, and why, in words the user and the model can
// both read.
type Rule struct {
	// Tool is a tool name, or a glob such as "browser_*" or "*" for every tool.
	Tool string
	// Pattern is a glob matched against the call's readable form.
	Pattern string
	// Action is allow, ask, or deny.
	Action contract.PermissionRuling
	// Reason says why this rule is here.
	Reason string
	// FromTheAskMeFirstList says the rule is one of the entries the user asked
	// to see first, which is the one thing a skill's standing approval may
	// never cover.
	FromTheAskMeFirstList bool
}

// Rulebook is a list of rules, compiled once, ready to be matched against a call.
type Rulebook struct {
	rules []compiledRule
}

// compiledRule is one rule with its two globs already turned into matchers, so
// that ruling on a call costs no compiling.
type compiledRule struct {
	rule    Rule
	tool    *regexp.Regexp
	pattern *regexp.Regexp
}

// NewRulebook compiles the rules once, keeping the order they were given in,
// because that order is what decides which rule wins. It returns an error naming
// the rule that cannot be used.
func NewRulebook(rules []Rule) (*Rulebook, error) {
	if len(rules) > MaxRules {
		return nil, fmt.Errorf("there are %d permission rules and the most allowed is %d, so take some out of the ask-me-first list", len(rules), MaxRules)
	}

	book := &Rulebook{rules: make([]compiledRule, 0, len(rules))}
	for _, rule := range rules {
		compiled, err := compileRule(rule)
		if err != nil {
			return nil, err
		}
		book.rules = append(book.rules, compiled)
	}
	return book, nil
}

// Match returns the last rule that covers the call, and says whether any rule
// did. A call is given in one form or in more than one — the readable form the
// user sees, and for a shell command the same form with its flags spelled out —
// and a rule covers the call when its pattern matches any of them, so that a
// rule about a flag holds however the flag was written. Nothing here decides
// what happens when no rule covers a call; that is the decider's job.
func (book *Rulebook) Match(toolName string, forms ...string) (Rule, bool) {
	matched := Rule{}
	covered := false
	for _, compiled := range book.rules {
		if !compiled.tool.MatchString(toolName) {
			continue
		}
		for _, form := range forms {
			if compiled.pattern.MatchString(form) {
				matched = compiled.rule
				covered = true
				break
			}
		}
	}
	return matched, covered
}

// compileRule checks one rule and turns its two globs into matchers.
func compileRule(rule Rule) (compiledRule, error) {
	if strings.TrimSpace(rule.Tool) == "" {
		return compiledRule{}, fmt.Errorf("a permission rule with the pattern %q names no tool, so write a tool name or * to cover every tool", rule.Pattern)
	}
	if strings.TrimSpace(rule.Pattern) == "" {
		return compiledRule{}, fmt.Errorf("the permission rule for the tool %q has no pattern, so write a pattern or * to cover every call", rule.Tool)
	}
	if !knownAction(rule.Action) {
		return compiledRule{}, fmt.Errorf("the permission rule for the tool %q says to %q, so write allow, ask, or deny instead", rule.Tool, rule.Action)
	}

	tool, err := compilePattern(rule.Tool)
	if err != nil {
		return compiledRule{}, err
	}
	pattern, err := compilePattern(rule.Pattern)
	if err != nil {
		return compiledRule{}, err
	}
	return compiledRule{rule: rule, tool: tool, pattern: pattern}, nil
}

// knownAction says whether the action is one of the three rulings.
func knownAction(action contract.PermissionRuling) bool {
	switch action {
	case contract.RulingAllow, contract.RulingAsk, contract.RulingDeny:
		return true
	default:
		return false
	}
}

// compilePattern turns one glob into a matcher. A star stands for any run of
// characters and a question mark for one, everything else stands for itself, and
// capital letters are ignored, because "rm -R" and "rm -r" are the same flag.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if letters := []rune(pattern); len(letters) > MaxPatternRunes {
		return nil, fmt.Errorf("the permission pattern %q is %d characters and the most allowed is %d, so shorten it", pattern, len(letters), MaxPatternRunes)
	}

	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")
	matcher, err := regexp.Compile("(?is)^" + escaped + "$")
	if err != nil {
		return nil, fmt.Errorf("the permission pattern %q cannot be read as a pattern, so write it with plain text, * and ?: %w", pattern, err)
	}
	return matcher, nil
}
