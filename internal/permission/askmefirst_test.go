package permission_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
)

// askMeFirstCall is one call put to the shipped ask-me-first list, and the name
// of the entry that has to catch it.
type askMeFirstCall struct {
	toolName string
	fields   map[string]any
	entry    string
}

// bulkDeleteCalls are the calls the first shipped entry has to catch.
var bulkDeleteCalls = []askMeFirstCall{
	{contract.ToolShell, map[string]any{"command": "rm -rf /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "rm -r /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "rm -fr /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "rm -f -r /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "rm -v -rf /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "rm --force --recursive /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "rm -R /tmp/x"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "git clean -fdx"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "find /tmp -name '*.log' -delete"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "git clean -f -d"}, contract.AskFirstBulkDelete},
	{contract.ToolShell, map[string]any{"command": "git reset --hard origin/main"}, contract.AskFirstBulkDelete},
	{contract.ToolBrowserAct, map[string]any{"intent": "delete all the messages"}, contract.AskFirstBulkDelete},
	{contract.ToolComputer, map[string]any{"intent": "delete all the screenshots"}, contract.AskFirstBulkDelete},
}

// sudoCalls are the calls the second shipped entry has to catch, including the
// escalated shell call, which asks for administrator powers through the field
// rather than through the word.
var sudoCalls = []askMeFirstCall{
	{contract.ToolShell, map[string]any{"command": "apt install ripgrep", "escalate": true}, contract.AskFirstSudo},
	{contract.ToolShell, map[string]any{"command": "sudo apt install ripgrep"}, contract.AskFirstSudo},
	{contract.ToolShell, map[string]any{"command": "sudo rm -rf /tmp/x"}, contract.AskFirstSudo},
	{contract.ToolShell, map[string]any{"command": "cat hosts | sudo tee /etc/hosts"}, contract.AskFirstSudo},
}

// spendMoneyCalls are the calls the third shipped entry has to catch, one for
// each of the six words that mean money is about to leave.
var spendMoneyCalls = []askMeFirstCall{
	{contract.ToolBrowserAct, map[string]any{"intent": "buy the blue kayak"}, contract.AskFirstSpendMoney},
	{contract.ToolBrowserClick, map[string]any{"intent": "pay now", "element": "e12"}, contract.AskFirstSpendMoney},
	{contract.ToolComputer, map[string]any{"intent": "purchase the ticket"}, contract.AskFirstSpendMoney},
	{contract.ToolBrowserClick, map[string]any{"intent": "go to checkout"}, contract.AskFirstSpendMoney},
	{contract.ToolBrowserAct, map[string]any{"intent": "subscribe to the newsletter"}, contract.AskFirstSpendMoney},
	{contract.ToolBrowserAct, map[string]any{"intent": "order the spare parts"}, contract.AskFirstSpendMoney},
	{contract.ToolWeb, map[string]any{"url": "https://example.com/checkout"}, contract.AskFirstSpendMoney},
}

// nearMisses look like the entries above and are not them, so the list has to
// leave every one of them alone.
var nearMisses = []askMeFirstCall{
	{contract.ToolShell, map[string]any{"command": "rm notes.txt"}, ""},
	{contract.ToolShell, map[string]any{"command": "rm -f notes.txt"}, ""},
	{contract.ToolShell, map[string]any{"command": "ls -lart /tmp"}, ""},
	{contract.ToolShell, map[string]any{"command": "find /tmp -name '*.log'"}, ""},
	{contract.ToolShell, map[string]any{"command": "git clean -n"}, ""},
	{contract.ToolShell, map[string]any{"command": "git clean --dry-run"}, ""},
	{contract.ToolShell, map[string]any{"command": "git reset"}, ""},
	{contract.ToolShell, map[string]any{"command": "apt install ripgrep"}, ""},
	{contract.ToolShell, map[string]any{"command": "pseudo-thing --run"}, ""},
	{contract.ToolShell, map[string]any{"command": "sh -c 'ls -la'"}, ""},
	{contract.ToolShell, map[string]any{"command": "bash -lc 'git status'"}, ""},
	{contract.ToolShell, map[string]any{"command": "/usr/bin/ls -la /tmp"}, ""},
	{contract.ToolBrowserAct, map[string]any{"intent": "delete the last message"}, ""},
	{contract.ToolBrowserClick, map[string]any{"intent": "read the news"}, ""},
	{contract.ToolWeb, map[string]any{"url": "https://example.com/about"}, ""},
	{contract.ToolRead, map[string]any{"path": "/home/jared/buy-a-kayak.md"}, ""},
}

func TestTheShippedListCatchesDeletingManyFilesAtOnce(t *testing.T) {
	checkAskMeFirst(t, bulkDeleteCalls)
}

func TestTheShippedListCatchesRunningACommandWithSudo(t *testing.T) {
	checkAskMeFirst(t, sudoCalls)
}

func TestTheShippedListCatchesSpendingMoney(t *testing.T) {
	checkAskMeFirst(t, spendMoneyCalls)
}

func TestTheShippedListLeavesEveryNearMissAlone(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())
	for _, call := range nearMisses {
		request := contract.PermissionRequest{ToolName: call.toolName, Input: jsonInput(t, call.fields)}
		reduced := permission.Reduce(request)
		if decision := decide(t, decider, request); decision.Ruling != contract.RulingAllow {
			t.Errorf("%q was ruled %q because %q, want %q, because a near miss runs on its own",
				reduced, decision.Ruling, decision.Reason, contract.RulingAllow)
		}
	}
}

func TestTheShippedListCatchesAWriteThatEmptiesABigFile(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big.log")
	writeFileOfSize(t, big, permission.EmptiesFileOverBytes+1)

	checkAskMeFirst(t, []askMeFirstCall{
		{contract.ToolWrite, map[string]any{"path": big, "content": ""}, contract.AskFirstBulkDelete},
	})
}

func TestTheShippedEntriesAreTheThreeTheContractNames(t *testing.T) {
	entries := permission.ShippedAskMeFirstEntries()
	if len(entries) != len(contract.DefaultAskMeFirst()) {
		t.Fatalf("the list ships with %d entries, want %d", len(entries), len(contract.DefaultAskMeFirst()))
	}
	for index, name := range contract.DefaultAskMeFirst() {
		if entries[index].Name != name {
			t.Errorf("entry %d is named %q, want %q", index, entries[index].Name, name)
		}
		if len(entries[index].Rules) == 0 {
			t.Errorf("the entry %q stands for no rules at all", name)
		}
	}
}

func TestAnAskMeFirstEntryNobodyShippedIsRefusedByName(t *testing.T) {
	_, err := permission.RulesForAskMeFirst([]string{"whatever I feel like"})
	if err == nil {
		t.Fatal("an ask-me-first entry nobody shipped was accepted, want an error naming it")
	}
	if !strings.Contains(err.Error(), "whatever I feel like") {
		t.Errorf("the error is %q, and it has to name the entry that is not known", err)
	}
}

func TestAnEmptyAskMeFirstListStandsForNoRules(t *testing.T) {
	rules, err := permission.RulesForAskMeFirst(nil)
	if err != nil {
		t.Fatalf("an empty ask-me-first list was refused: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("an empty ask-me-first list stands for %d rules, want none", len(rules))
	}
}

// checkAskMeFirst puts every call to the permission function a fresh install has
// and says which entry has to catch it. The entry's own name is the reason the
// ruling carries, because that is what the user is shown, so one ruling answers
// both questions: that the call asks, and which entry made it ask.
func checkAskMeFirst(t *testing.T, calls []askMeFirstCall) {
	t.Helper()
	decider := newDecider(t, contract.DefaultConfig())
	for _, call := range calls {
		request := contract.PermissionRequest{ToolName: call.toolName, Input: jsonInput(t, call.fields)}
		reduced := permission.Reduce(request)
		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAsk {
			t.Errorf("the ask-me-first list ruled %q on %q, want %q, because the entry %q has to catch it",
				decision.Ruling, reduced, contract.RulingAsk, call.entry)
			continue
		}
		if decision.Reason != call.entry {
			t.Errorf("%q was caught by the entry %q, want %q", reduced, decision.Reason, call.entry)
		}
	}
}
