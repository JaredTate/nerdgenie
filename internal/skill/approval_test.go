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

// theSkillThatReadsTheNews is a skill written the way a permissions block is
// meant to be written: one host name, and one step that visits it.
func theSkillThatReadsTheNews() map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte("# read-the-story\n\nReads today's story off the site its block names.\n\n" +
			"## Permissions\n\n- site: news.example.com\n- daily limit: 50\n"),
		skill.StepsFile: []byte("1. Read today's story.\n" +
			"   tool: web\n   input: {\"url\": \"https://news.example.com/story\"}\n"),
	}
}

// configurationWhereEveryFetchAsks is the shipped ask-me-first list plus one
// rule the user wrote: show me every page this agent fetches. Every fetch in
// the tables below therefore needs a yes, so a fetch that comes back allowed
// came back allowed because the skill holds a standing approval covering it,
// and a fetch that comes back asked is one no approval covers.
func configurationWhereEveryFetchAsks() contract.Config {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolWeb, Pattern: "*", Action: contract.RulingAsk},
	}
	return configuration
}

// theThreeThingsTheListShipsWith are the three entries design section 11 says
// the ask-me-first list ships with. Not one of them may run without a yes,
// whatever standing approval a skill holds. They came here from the wave 6
// security review, whose own test file was taken out of the tree once every
// finding it named had a fix and a test of its own.
var theThreeThingsTheListShipsWith = []struct {
	name    string
	request func() contract.PermissionRequest
}{
	{"deleting many files at once", func() contract.PermissionRequest {
		return contract.PermissionRequest{ToolName: contract.ToolShell, Input: []byte(`{"command": "rm -rf /home/jared/coeus"}`)}
	}},
	{"a command with administrator powers", func() contract.PermissionRequest {
		return contract.PermissionRequest{ToolName: contract.ToolShell, Input: []byte(`{"command": "apt-get install anything", "escalate": true, "reason": "it needs them"}`)}
	}},
	{"spending money", func() contract.PermissionRequest {
		return contract.PermissionRequest{ToolName: contract.ToolWeb, Input: []byte(`{"url": "https://shop.example.com/checkout"}`)}
	}},
}

// storeHoldingTheApproval saves the news-reading skill and runs it once, which
// is what puts its standing approval into the permission function, and hands
// back the permission function to ask about other calls.
func storeHoldingTheApproval(t *testing.T) *permission.Decider {
	t.Helper()
	store, _, decider := realPermissionHarnessWith(t, configurationWhereEveryFetchAsks(),
		testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "today's story"))
	ctx := context.Background()
	if err := store.Save(ctx, "read-the-story", theSkillThatReadsTheNews()); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	if _, err := store.Run(ctx, "read-the-story", ""); err != nil {
		t.Fatalf("running the skill failed: %v", err)
	}
	return decider
}

func TestAStandingApprovalOnlyCoversTheHostTheBlockNames(t *testing.T) {
	decider := storeHoldingTheApproval(t)
	ctx := context.Background()

	for _, address := range []string{
		"https://news.example.com/story",
		"https://news.example.com",
		"http://news.example.com/story",
	} {
		decision, err := decider.Decide(ctx, webCallTo(address))
		if err != nil {
			t.Fatalf("ruling on %s failed: %v", address, err)
		}
		if decision.Ruling != contract.RulingAllow || !strings.Contains(decision.Reason, "read-the-story") {
			t.Errorf("the ruling on %s is %+v, want it allowed by the skill's standing approval", address, decision)
		}
	}

	for _, address := range []string{
		"https://news.example.com.evil.net/story",
		"https://notnews.example.com/story",
		"https://newsxexample.com/story",
		"https://news.example.com@evil.net/story",
		"https://evil.net/story?go=https://news.example.com/story",
	} {
		decision, err := decider.Decide(ctx, webCallTo(address))
		if err != nil {
			t.Fatalf("ruling on %s failed: %v", address, err)
		}
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("the call to %s was ruled allow because %q; the approval names news.example.com and this is a different website,"+
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
		"path": "/home/jared/news.example.com/notes.md",
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
			Input:    []byte(`{"command": "web --url=https://news.example.com/story ; rm -rf /home/jared/coeus"}`),
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
