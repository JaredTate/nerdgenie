package skill_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theSkillThatReadsPrices is a skill written the way a permissions block is
// meant to be written: one host name, and one step that visits it. Every
// address on that host holds the word "buy", so the shipped spending rule
// covers each of these calls and the skill's standing approval is what decides
// them.
func theSkillThatReadsPrices() map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte("# read-prices\n\nReads the prices off the shop its block names.\n\n" +
			"## Permissions\n\n- site: buy.example.com\n- daily limit: 50\n"),
		skill.StepsFile: []byte("1. Read the prices, which is a call the spending rule puts to the user.\n" +
			"   tool: web\n   input: {\"url\": \"https://buy.example.com/prices\"}\n"),
	}
}

// storeHoldingTheApproval saves the price-reading skill and runs it once, which
// is what puts its standing approval into the permission function, and hands
// back the permission function to ask about other calls.
func storeHoldingTheApproval(t *testing.T) *permission.Decider {
	t.Helper()
	store, _, decider := realPermissionHarness(t, testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "the prices are here"))
	ctx := context.Background()
	if err := store.Save(ctx, "read-prices", theSkillThatReadsPrices()); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	if _, err := store.Run(ctx, "read-prices", ""); err != nil {
		t.Fatalf("running the skill failed: %v", err)
	}
	return decider
}

func TestAStandingApprovalOnlyCoversTheHostTheBlockNames(t *testing.T) {
	decider := storeHoldingTheApproval(t)
	ctx := context.Background()

	for _, address := range []string{
		"https://buy.example.com/prices",
		"https://buy.example.com",
		"http://buy.example.com/prices",
	} {
		decision, err := decider.Decide(ctx, webCallTo(address))
		if err != nil {
			t.Fatalf("ruling on %s failed: %v", address, err)
		}
		if decision.Ruling != contract.RulingAllow || !strings.Contains(decision.Reason, "read-prices") {
			t.Errorf("the ruling on %s is %+v, want it allowed by the skill's standing approval", address, decision)
		}
	}

	for _, address := range []string{
		"https://buy.example.com.evil.net/prices",
		"https://notbuy.example.com/prices",
		"https://buyxexample.com/prices",
		"https://buy.example.com@evil.net/prices",
		"https://evil.net/buy?go=https://buy.example.com/prices",
	} {
		decision, err := decider.Decide(ctx, webCallTo(address))
		if err != nil {
			t.Fatalf("ruling on %s failed: %v", address, err)
		}
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("the call to %s was ruled allow because %q; the approval names buy.example.com and this is a different website,"+
				" so the pattern has to hold the host between the scheme in front of it and the end or the path behind it", address, decision.Reason)
		}
	}
}

func TestAStandingApprovalFromASkillNeverCoversAShellOrAFileCall(t *testing.T) {
	decider := storeHoldingTheApproval(t)
	ctx := context.Background()

	for _, one := range theThreeThingsTheListShipsWith {
		decision, err := decider.Decide(ctx, one.request())
		if err != nil {
			t.Fatalf("%s: ruling on the call failed: %v", one.name, err)
		}
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("%s: the call was ruled allow because %q; a permissions block names websites, so its approval must never"+
				" cover a shell command, a file change, or money spent somewhere else", one.name, decision.Reason)
		}
	}

	for _, one := range callsWearingTheWebsitesName(t) {
		decision, err := decider.Decide(ctx, one.request)
		if err != nil {
			t.Fatalf("%s: ruling on the call failed: %v", one.what, err)
		}
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("%s: the call was ruled allow because %q; the website's name inside a call does not make it a visit to that website", one.what, decision.Reason)
		}
	}
}

// callsWearingTheWebsitesName are the two calls that carry the approved host
// inside them without visiting it: a shell command whose first word is the name
// of the web tool, and a file change under a folder named after the website.
func callsWearingTheWebsitesName(t *testing.T) []struct {
	what    string
	request contract.PermissionRequest
} {
	t.Helper()
	emptying, err := json.Marshal(map[string]string{
		"path": "/home/jared/buy.example.com/notes.md",
		"old":  strings.Repeat("a line of the notes this change would take out\n", 200),
		"new":  "",
	})
	if err != nil {
		t.Fatalf("cannot write the input of the file change: %v", err)
	}
	return []struct {
		what    string
		request contract.PermissionRequest
	}{
		{"a shell command whose first word is the name of the web tool", contract.PermissionRequest{
			ToolName: contract.ToolShell,
			Input:    []byte(`{"command": "web --url=https://buy.example.com/prices ; rm -rf /home/jared/coeus"}`),
		}},
		{"emptying a file in a folder named after the website", contract.PermissionRequest{
			ToolName: contract.ToolEdit,
			Input:    emptying,
		}},
	}
}

// webCallTo is one fetch of a web address, which is the call a permissions
// block is written to cover.
func webCallTo(address string) contract.PermissionRequest {
	input, err := json.Marshal(map[string]string{"url": address})
	if err != nil {
		return contract.PermissionRequest{ToolName: contract.ToolWeb, Input: []byte("{}")}
	}
	return contract.PermissionRequest{ToolName: contract.ToolWeb, Input: input}
}
