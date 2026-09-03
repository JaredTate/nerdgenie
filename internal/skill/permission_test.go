package skill_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/testkit"
)

// browsingSkill is a skill whose permissions block names one website and whose
// two steps go to two different ones, so that a test can see the block let one
// through and stop the other.
func browsingSkill() map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte("# read-the-news\n\nReads the news site and then somewhere else.\n\n" +
			"## Permissions\n\n- site: news.example.com\n- daily limit: 7\n"),
		skill.StepsFile: []byte(readingStep +
			"2. Look at another site the block does not name.\n" +
			"   tool: browser_open\n" +
			"   input: {\"url\": \"https://other.example.com/\", \"intent\": \"look somewhere else\"}\n"),
	}
}

// readingStep reads one story off the site the block names. Reading a page is
// on none of the shipped ask-me-first entries, and the rule the user wrote in
// the harness puts every page fetch to the user, so this step runs without a
// preview only while the skill's standing approval covers its website.
const readingStep = "1. Read today's story from the news site.\n" +
	"   tool: web\n" +
	"   input: {\"url\": \"https://news.example.com/story\"}\n\n"

// configurationWithTheUsersOwnRule is the shipped ask-me-first list plus one
// rule the user wrote for themselves: show me the story pages this agent
// fetches. It has to be the user's own rule rather than one of the shipped
// entries, because nothing a skill holds may carry a call past the ask-me-first
// list, so an entry from that list could never show a standing approval doing
// anything. And it has to be narrower than the website, so that the checkout
// page of the same website is caught by the shipped spending entry and by
// nothing else.
func configurationWithTheUsersOwnRule() contract.Config {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolWeb, Pattern: "*/story*", Action: contract.RulingAsk},
	}
	return configuration
}

// realPermissionHarness builds a skill store over a real permission function,
// so that a test can prove the standing approvals really landed in it.
func realPermissionHarness(t *testing.T, tools ...contract.Tool) (*skill.Store, *testkit.FakeChannel, *permission.Decider) {
	t.Helper()
	return realPermissionHarnessWith(t, configurationWithTheUsersOwnRule(), tools...)
}

// realPermissionHarnessWith is the same harness over a configuration the test
// chose, so that a test can say what the user's own rules are.
func realPermissionHarnessWith(t *testing.T, configuration contract.Config, tools ...contract.Tool) (*skill.Store, *testkit.FakeChannel, *permission.Decider) {
	t.Helper()
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(startOfTheTests)
	decider, err := permission.New(configuration, clock)
	if err != nil {
		t.Fatalf("cannot build the permission function: %v", err)
	}
	channel := testkit.NewFakeChannel("terminal")
	store, err := skill.New(skill.Options{
		Home:       home,
		Clock:      clock,
		Tools:      testkit.NewFakeToolRegistry(tools...),
		Permission: decider,
		Standing:   decider,
		Ask:        channel.ShowPreview,
	})
	if err != nil {
		t.Fatalf("cannot build the skill store: %v", err)
	}
	return store, channel, decider
}

func TestThePermissionsBlockBecomesStandingApprovals(t *testing.T) {
	fetched := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "today's story")
	opened := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen}, "the other site")
	store, channel, decider := realPermissionHarness(t, fetched, opened)
	ctx := context.Background()
	if err := store.Save(ctx, contract.SkillSavedByPerson, "read-the-news", browsingSkill()); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	// The second step goes to a site the block does not name, so the run stops
	// there. The first step is a page fetch, which the rule the user wrote puts
	// to the user, and it runs anyway because the block covers its website.
	channel.AnswerPreviewsWith(contract.AnswerReject)
	report, err := store.Run(ctx, "read-the-news", "")
	if err == nil {
		t.Fatalf("the whole skill ran, and its second step is outside the block:\n%s", report)
	}
	if !strings.Contains(report, "today's story") {
		t.Errorf("the report is %q, want the first step to have run under the standing approval", report)
	}

	previews := channel.Previews()
	if len(previews) != 1 || !strings.Contains(previews[0].Title, "other.example.com") {
		t.Fatalf("the previews are %v, want one about the site the block does not name", previews)
	}

	// The approval is really in the permission function, and it says so by name.
	decision, err := decider.Decide(ctx, contract.PermissionRequest{
		ToolName: contract.ToolWeb,
		Input:    []byte(`{"url": "https://news.example.com/story"}`),
	})
	if err != nil {
		t.Fatalf("ruling on a call the approval covers failed: %v", err)
	}
	if decision.Ruling != contract.RulingAllow || !strings.Contains(decision.Reason, "read-the-news") {
		t.Errorf("the ruling is %+v, want it allowed because the skill holds a standing approval", decision)
	}

	// The checkout page of the very same website is on the user's ask-me-first
	// list, and nothing a skill holds may carry a call past that list.
	spending, err := decider.Decide(ctx, contract.PermissionRequest{
		ToolName: contract.ToolWeb,
		Input:    []byte(`{"url": "https://news.example.com/checkout"}`),
	})
	if err != nil {
		t.Fatalf("ruling on the checkout page failed: %v", err)
	}
	if spending.Ruling != contract.RulingAsk {
		t.Errorf("the ruling on the checkout page of the site the block names is %+v, want it put to the user,"+
			" because spending money is on the ask-me-first list and a skill's approval is not the user's word", spending)
	}
}

func TestAStepOutsideTheBlockAsksAndRunsWhenTheUserSaysYes(t *testing.T) {
	fetched := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "the paper is bought")
	opened := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserOpen}, "the other site")
	store, channel, _ := realPermissionHarness(t, fetched, opened)
	ctx := context.Background()
	if err := store.Save(ctx, contract.SkillSavedByPerson, "read-the-news", browsingSkill()); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	channel.AnswerPreviewsWith(contract.AnswerOnce)
	report, err := store.Run(ctx, "read-the-news", "")
	if err != nil {
		t.Fatalf("running the skill with the user saying yes failed: %v\n%s", err, report)
	}
	if !strings.Contains(report, "the other site") {
		t.Errorf("the report is %q, want the step the user allowed to have run", report)
	}
}

func TestTheDailyLimitIsHandedOutOnceADay(t *testing.T) {
	fetched := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb},
		"one", "two", "three", "four", "five", "six", "seven", "eight")
	store, channel, _ := realPermissionHarness(t, fetched)
	ctx := context.Background()
	files := browsingSkill()
	files[skill.StepsFile] = []byte(readingStep)
	if err := store.Save(ctx, contract.SkillSavedByPerson, "read-the-news", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	// The block's daily limit is seven, so the eighth call is the one that asks.
	channel.AnswerPreviewsWith(contract.AnswerOnce)
	for round := range 8 {
		if _, err := store.Run(ctx, "read-the-news", ""); err != nil {
			t.Fatalf("run %d failed: %v", round+1, err)
		}
	}
	if previews := channel.Previews(); len(previews) != 1 {
		t.Errorf("the user was asked %d times, want once, on the run past the daily limit", len(previews))
	}
}

func TestAStoreWithNoPlaceToRegisterApprovalsStillRuns(t *testing.T) {
	built := newHarness(t, testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "the paper is bought"))
	ctx := context.Background()
	files := browsingSkill()
	files[skill.StepsFile] = []byte(readingStep)
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "read-the-news", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	if _, err := built.store.Run(ctx, "read-the-news", ""); err != nil {
		t.Errorf("running with nowhere to register approvals failed: %v", err)
	}
}
