package permission_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
)

// bookForMatching is the rulebook the matching table is written against. The
// rules are in the order a user would write them, and the last one that covers a
// call is the one that wins.
var bookForMatching = []permission.Rule{
	{Tool: "*", Pattern: "*", Action: contract.RulingAsk, Reason: "everything asks by default here"},
	{Tool: contract.ToolRead, Pattern: "*", Action: contract.RulingAllow, Reason: "reading is always allowed"},
	{Tool: contract.ToolShell, Pattern: "*rm -r*", Action: contract.RulingAsk, Reason: "deleting many files at once"},
	{Tool: contract.ToolShell, Pattern: "*rm -r /home*", Action: contract.RulingDeny, Reason: "never delete a home directory"},
	{Tool: "browser_*", Pattern: "*checkout*", Action: contract.RulingAsk, Reason: "spending money"},
}

// matchingCalls are the calls the rulebook is asked about and the reason of the
// rule that has to win, or an empty reason when no rule covers the call.
var matchingCalls = []struct {
	toolName string
	reduced  string
	reason   string
}{
	{contract.ToolRead, "read /home/jared/notes.md", "reading is always allowed"},
	{contract.ToolShell, "rm -rf /tmp/x", "deleting many files at once"},
	{contract.ToolShell, "rm -rf /home/jared", "never delete a home directory"},
	{contract.ToolShell, "git commit", "everything asks by default here"},
	{contract.ToolBrowserClick, "browser_click go to checkout", "spending money"},
	{contract.ToolBrowserClick, "browser_click read the news", "everything asks by default here"},
}

func TestARulebookGivesEveryCallTheLastRuleThatCoversIt(t *testing.T) {
	book, err := permission.NewRulebook(bookForMatching)
	if err != nil {
		t.Fatalf("compiling the rulebook failed: %v", err)
	}

	for _, call := range matchingCalls {
		matched, covered := book.Match(call.toolName, call.reduced)
		if !covered {
			t.Errorf("no rule covered %q, want the rule that says %q", call.reduced, call.reason)
			continue
		}
		if matched.Reason != call.reason {
			t.Errorf("the rule that won for %q says %q, want %q", call.reduced, matched.Reason, call.reason)
		}
	}
}

func TestARulebookWithNoRulesCoversNothing(t *testing.T) {
	book, err := permission.NewRulebook(nil)
	if err != nil {
		t.Fatalf("compiling an empty rulebook failed: %v", err)
	}

	if _, covered := book.Match(contract.ToolShell, "rm -rf /tmp/x"); covered {
		t.Error("an empty rulebook covered a call, and a book with no rules covers nothing")
	}
}

func TestARulebookMatchesWithoutCaringAboutCapitalLetters(t *testing.T) {
	book, err := permission.NewRulebook([]permission.Rule{
		{Tool: contract.ToolShell, Pattern: "*rm -r*", Action: contract.RulingAsk, Reason: "deleting many files at once"},
	})
	if err != nil {
		t.Fatalf("compiling the rulebook failed: %v", err)
	}

	if _, covered := book.Match(contract.ToolShell, "rm -R /tmp/x"); !covered {
		t.Error("the pattern \"*rm -r*\" missed \"rm -R\", and the two are the same flag")
	}
}

// badRules are the rulebooks that must be refused, each with the word the error
// has to carry so that the user can find the rule that is wrong.
var badRules = []struct {
	named string
	rule  permission.Rule
}{
	{"tool", permission.Rule{Tool: "", Pattern: "*", Action: contract.RulingAsk}},
	{"pattern", permission.Rule{Tool: "*", Pattern: "", Action: contract.RulingAsk}},
	{"maybe", permission.Rule{Tool: "*", Pattern: "*", Action: "maybe"}},
	{strings.Repeat("x", 40), permission.Rule{Tool: "*", Pattern: strings.Repeat("x", permission.MaxPatternRunes+1), Action: contract.RulingAsk}},
}

func TestARulebookRefusesARuleItCannotUseAndSaysWhichOne(t *testing.T) {
	for _, bad := range badRules {
		_, err := permission.NewRulebook([]permission.Rule{bad.rule})
		if err == nil {
			t.Errorf("the rule %+v was accepted, want an error naming what is wrong with it", bad.rule)
			continue
		}
		if !strings.Contains(err.Error(), bad.named) {
			t.Errorf("the error for %+v is %q, and it has to name %q", bad.rule, err, bad.named)
		}
	}
}

func TestARulebookRefusesMoreRulesThanTheCap(t *testing.T) {
	tooMany := make([]permission.Rule, permission.MaxRules+1)
	for index := range tooMany {
		tooMany[index] = permission.Rule{Tool: "*", Pattern: "*", Action: contract.RulingAsk}
	}

	if _, err := permission.NewRulebook(tooMany); err == nil {
		t.Fatalf("a rulebook of %d rules was accepted, and the cap is %d", len(tooMany), permission.MaxRules)
	}
}
