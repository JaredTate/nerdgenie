package permission

import (
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// AskMeFirstEntry is one line of the user's ask-me-first list: a plain name the
// user reads, and the rules that name stands for. The harness adds no rule the
// user cannot see, so every rule in the harness belongs to one of these entries
// or was written by the user.
type AskMeFirstEntry struct {
	// Name is the plain-words name, which is one of the three in the contract.
	Name string
	// Rules are what the entry matches, all of them ruling ask.
	Rules []Rule
}

// deletePatternsByTool are the forms that mean many things are about to go at
// once: a recursive rm, a find that deletes what it finds, the two git commands
// that throw away work, a write or an edit that empties a file over the size,
// and the browser or desktop intent that says to delete everything.
//
// The shell patterns are written against the form with the flags spelled out,
// which is why each flag is in brackets: they ask which flags a command was
// given and not which letters were written next to which, so that "rm -v -rf",
// "rm --force --recursive" and "rm -rf" are all one recursive delete.
var deletePatternsByTool = []Rule{
	{Tool: contract.ToolShell, Pattern: "*rm *[-r]*"},
	{Tool: contract.ToolShell, Pattern: "*rm *[--recursive]*"},
	{Tool: contract.ToolShell, Pattern: "*find *[-delete]*"},
	{Tool: contract.ToolShell, Pattern: "*git clean *[-f]*"},
	{Tool: contract.ToolShell, Pattern: "*git clean *[--force]*"},
	{Tool: contract.ToolShell, Pattern: "*git reset *[--hard]*"},
	{Tool: contract.ToolWrite, Pattern: "*emptying a file of*"},
	{Tool: contract.ToolEdit, Pattern: "*emptying a file of*"},
	{Tool: contract.ToolBrowserAct, Pattern: "*delete all*"},
	{Tool: contract.ToolComputer, Pattern: "*delete all*"},
}

// sudoPatterns are the readable forms that ask for administrator powers. The
// first covers a command that begins with sudo, which is also what the reducer
// writes when the shell call sets its escalate field, and the second covers a
// sudo that comes after a pipe.
var sudoPatterns = []Rule{
	{Tool: contract.ToolShell, Pattern: sudoProgram + "*"},
	{Tool: contract.ToolShell, Pattern: "* " + sudoProgram + "*"},
}

// spendingWords are the six words that mean money is about to leave.
var spendingWords = []string{"buy", "pay", "purchase", "checkout", "subscribe", "order"}

// spendingTools are the four tools that can spend money: the three that act on a
// page or on the screen, and the one that fetches a web address.
var spendingTools = []string{
	contract.ToolBrowserClick,
	contract.ToolBrowserAct,
	contract.ToolComputer,
	contract.ToolWeb,
}

// ShippedAskMeFirstEntries returns the three entries the ask-me-first list ships
// with, in the order contract.DefaultAskMeFirst names them. The user may add to
// this list, change it, or empty it.
func ShippedAskMeFirstEntries() []AskMeFirstEntry {
	return []AskMeFirstEntry{
		{Name: contract.AskFirstBulkDelete, Rules: askRules(contract.AskFirstBulkDelete, deletePatternsByTool)},
		{Name: contract.AskFirstSudo, Rules: askRules(contract.AskFirstSudo, sudoPatterns)},
		{Name: contract.AskFirstSpendMoney, Rules: askRules(contract.AskFirstSpendMoney, spendingRules())},
	}
}

// RulesForAskMeFirst returns the rules the named entries stand for, in the order
// the names were given, because that order decides which entry catches a call
// that two of them cover. An entry nobody shipped is refused by name.
func RulesForAskMeFirst(names []string) ([]Rule, error) {
	shipped := ShippedAskMeFirstEntries()
	rules := []Rule{}
	for _, name := range names {
		found := false
		for _, entry := range shipped {
			if entry.Name == name {
				rules = append(rules, entry.Rules...)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("the ask-me-first entry %q is not one the harness ships, so use one of these: %s", name, strings.Join(contract.DefaultAskMeFirst(), "; "))
		}
	}
	return rules, nil
}

// spendingRules pairs every spending word with every tool that can spend money.
func spendingRules() []Rule {
	rules := []Rule{}
	for _, word := range spendingWords {
		for _, toolName := range spendingTools {
			rules = append(rules, Rule{Tool: toolName, Pattern: "*" + word + "*"})
		}
	}
	return rules
}

// askRules finishes a shipped entry's rules: everything on the ask-me-first list
// asks, and the reason it gives is the entry's own plain name.
func askRules(name string, patterns []Rule) []Rule {
	rules := make([]Rule, 0, len(patterns))
	for _, pattern := range patterns {
		rules = append(rules, Rule{
			Tool:                  pattern.Tool,
			Pattern:               pattern.Pattern,
			Action:                contract.RulingAsk,
			Reason:                name,
			FromTheAskMeFirstList: true,
		})
	}
	return rules
}
