package permission_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aUserWhoAsksAboutOneCompanysSites is a user who wrote one rule of their own:
// ask me before anything goes to that company. A standing approval is held
// against a rule like this one, because the ask-me-first list the harness ships
// is read before every approval and no approval ever covers it.
func aUserWhoAsksAboutOneCompanysSites() contract.Config {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolWeb, Pattern: "*example.com*", Action: contract.RulingAsk},
	}
	return configuration
}

// aVisitTo is one call to a page, which is the kind of call a skill's
// permissions block holds a standing approval for.
func aVisitTo(t *testing.T, address string) contract.PermissionRequest {
	t.Helper()
	return contract.PermissionRequest{
		ToolName: contract.ToolWeb,
		Input:    jsonInput(t, map[string]any{"url": address}),
	}
}

func TestAStandingApprovalAllowsUpToItsLimitAndThenAsksAgain(t *testing.T) {
	decider := newDecider(t, aUserWhoAsksAboutOneCompanysSites())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "read-the-news",
		ReducedForm: "*news.example.com*",
		Limit:       2,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := aVisitTo(t, "https://news.example.com/today")

	for use := 1; use <= 2; use++ {
		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAllow {
			t.Fatalf("use %d of the standing approval was ruled %q, want %q", use, decision.Ruling, contract.RulingAllow)
		}
		if !strings.Contains(decision.Reason, "read-the-news") {
			t.Errorf("the reason is %q, and it has to name the skill that holds the approval", decision.Reason)
		}
	}

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAsk {
		t.Errorf("the use after the limit was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestAnExpiredStandingApprovalAsksAgain(t *testing.T) {
	clock := testkit.NewFakeClock(theTestTime)
	decider, err := permission.New(aUserWhoAsksAboutOneCompanysSites(), clock)
	if err != nil {
		t.Fatalf("building the permission function failed: %v", err)
	}
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "read-the-news",
		ReducedForm: "*news.example.com*",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := aVisitTo(t, "https://news.example.com/today")

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAllow {
		t.Fatalf("before the expiry the call was ruled %q, want %q", decision.Ruling, contract.RulingAllow)
	}

	clock.Advance(2 * time.Hour)

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingAsk {
		t.Errorf("after the expiry the call was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestAStandingApprovalCoversOnlyTheFormItNames(t *testing.T) {
	decider := newDecider(t, aUserWhoAsksAboutOneCompanysSites())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "read-the-news",
		ReducedForm: "*news.example.com*",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})

	decision := decide(t, decider, aVisitTo(t, "https://other.example.com/today"))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("a call the standing approval does not name was ruled %q, want %q", decision.Ruling, contract.RulingAsk)
	}
}

func TestARejectionTheUserGaveBeatsAStandingApproval(t *testing.T) {
	decider := newDecider(t, aUserWhoAsksAboutOneCompanysSites())
	registerStanding(t, decider, permission.StandingApproval{
		Skill:       "read-the-news",
		ReducedForm: "*news.example.com*",
		Limit:       10,
		Expires:     theTestTime.Add(time.Hour),
	})
	request := aVisitTo(t, "https://news.example.com/today")

	if err := decider.Remember(request, contract.AnswerReject, "not that folder"); err != nil {
		t.Fatalf("remembering the rejection failed: %v", err)
	}

	if decision := decide(t, decider, request); decision.Ruling != contract.RulingDeny {
		t.Errorf("after the user refused it, the call was ruled %q, want %q", decision.Ruling, contract.RulingDeny)
	}
}

// theAddressesAWebsiteApprovalIsPutTo are the addresses an approval that names
// the one host example.com is asked about. It covers that site and nothing
// beside it: a subdomain is another host, a name that only begins with it is
// another host, a port is another door, and a site written inside the address of
// somewhere else is somewhere else.
var theAddressesAWebsiteApprovalIsPutTo = []struct {
	address string
	covered bool
}{
	{"https://example.com/prices", true},
	{"http://example.com/", true},
	{"https://EXAMPLE.COM/prices", true},
	{"https://example.com.evil.net/", false},
	{"https://api.example.com/", false},
	{"https://example.com:8443/", false},
	{"https://evil.net/?go=https://example.com/", false},
	{"ht tp://example.com/", false},
}

func TestAWebsiteApprovalCoversThatOneSiteAndNothingBeside(t *testing.T) {
	for _, one := range theAddressesAWebsiteApprovalIsPutTo {
		decider := newDecider(t, aUserWhoAsksAboutOneCompanysSites())
		registerStanding(t, decider, aWebsiteApproval("example.com", 10))

		wanted := contract.RulingAsk
		if one.covered {
			wanted = contract.RulingAllow
		}
		if decision := decide(t, decider, aVisitTo(t, one.address)); decision.Ruling != wanted {
			t.Errorf("a visit to %q was ruled %q, want %q; the approval names the one host example.com",
				one.address, decision.Ruling, wanted)
		}
	}
}

// theToolsThatVisitTheSite are one call to the same site through each tool whose
// call says which site it goes to. The login names its site as a bare host name,
// which is how a person writes one.
func theToolsThatVisitTheSite(t *testing.T) []contract.PermissionRequest {
	t.Helper()
	return []contract.PermissionRequest{
		aVisitTo(t, "https://example.com/prices"),
		{ToolName: contract.ToolBrowserOpen, Input: jsonInput(t, map[string]any{"url": "https://example.com/prices", "intent": "read the prices"})},
		{ToolName: contract.ToolBrowserRead, Input: jsonInput(t, map[string]any{"url": "https://example.com/prices", "intent": "read the prices"})},
		{ToolName: contract.ToolBrowserLogin, Input: jsonInput(t, map[string]any{"site": "example.com"})},
	}
}

func TestOneWebsiteApprovalServesEveryToolThatVisitsTheSite(t *testing.T) {
	configuration := aUserWhoAsksAboutOneCompanysSites()
	for _, toolName := range permission.ToolsThatVisitAWebsite() {
		configuration.PermissionRules = append(configuration.PermissionRules,
			contract.PermissionRule{Tool: toolName, Pattern: "*example.com*", Action: contract.RulingAsk})
	}
	decider := newDecider(t, configuration)
	registerStanding(t, decider, aWebsiteApproval("example.com", 10))

	for _, request := range theToolsThatVisitTheSite(t) {
		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAllow {
			t.Errorf("the %s call to the site was ruled %q because %q, want %q; one approval covers the site for every tool that visits it,"+
				" which is what makes the cap on approvals a cap on sites", request.ToolName, decision.Ruling, decision.Reason, contract.RulingAllow)
		}
	}
}

func TestAWebsiteApprovalCoversNoCallThatNamesNoSite(t *testing.T) {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolWeb, Pattern: "*", Action: contract.RulingAsk},
	}
	decider := newDecider(t, configuration)
	registerStanding(t, decider, aWebsiteApproval("example.com", 10))

	searched := contract.PermissionRequest{
		ToolName: contract.ToolWeb,
		Input:    jsonInput(t, map[string]any{"query": "the prices at example.com"}),
	}
	if decision := decide(t, decider, searched); decision.Ruling != contract.RulingAsk {
		t.Errorf("a search that names no address was ruled %q because %q, want %q; an approval for a website covers a call to that website",
			decision.Ruling, decision.Reason, contract.RulingAsk)
	}
}

func TestAWebsiteApprovalNeverCoversACommandOnThisMachine(t *testing.T) {
	configuration := contract.DefaultConfig()
	configuration.PermissionRules = []contract.PermissionRule{
		{Tool: contract.ToolShell, Pattern: "*curl*", Action: contract.RulingAsk},
	}
	decider := newDecider(t, configuration)
	registerStanding(t, decider, aWebsiteApproval("example.com", 10))

	decision := decide(t, decider, shellRequest(t, "curl https://example.com/prices"))
	if decision.Ruling != contract.RulingAsk {
		t.Errorf("a command was ruled %q because %q, want %q; an approval for a website covers the tools that visit a website and nothing else",
			decision.Ruling, decision.Reason, contract.RulingAsk)
	}
}

// aWebsiteApproval is the approval a skill's permissions block turns into: one
// site, the tools that visit a site, and the block's daily limit.
func aWebsiteApproval(host string, limit int) permission.StandingApproval {
	return permission.StandingApproval{
		Skill:   "read-the-news",
		Tools:   permission.ToolsThatVisitAWebsite(),
		Host:    host,
		Limit:   limit,
		Expires: theTestTime.Add(time.Hour),
	}
}

// badStandingApprovals are the standing approvals that must be refused, each
// with a word the error has to carry.
var badStandingApprovals = []struct {
	named    string
	approval permission.StandingApproval
}{
	{"skill", permission.StandingApproval{ReducedForm: "rm -rf", Limit: 1, Expires: theTestTime.Add(time.Hour)}},
	{"readable form", permission.StandingApproval{Skill: "a-skill", Limit: 1, Expires: theTestTime.Add(time.Hour)}},
	{"a-skill", permission.StandingApproval{Skill: "a-skill", ReducedForm: "rm -rf", Limit: 0, Expires: theTestTime.Add(time.Hour)}},
	{"a-skill", permission.StandingApproval{Skill: "a-skill", ReducedForm: "rm -rf", Limit: 1}},
	{"both", permission.StandingApproval{
		Skill: "a-skill", ReducedForm: "web *", Tools: permission.ToolsThatVisitAWebsite(), Host: "example.com",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{"*", permission.StandingApproval{
		Skill: "a-skill", Tools: permission.ToolsThatVisitAWebsite(), Host: "*",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{"example.com:8443", permission.StandingApproval{
		Skill: "a-skill", Tools: permission.ToolsThatVisitAWebsite(), Host: "example.com:8443",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{"https://example.com/prices", permission.StandingApproval{
		Skill: "a-skill", Tools: permission.ToolsThatVisitAWebsite(), Host: "https://example.com/prices",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{"tool", permission.StandingApproval{
		Skill: "a-skill", Host: "example.com", Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{contract.ToolShell, permission.StandingApproval{
		Skill: "a-skill", Tools: []string{contract.ToolShell}, Host: "example.com",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{"example..com", permission.StandingApproval{
		Skill: "a-skill", Tools: permission.ToolsThatVisitAWebsite(), Host: "example..com",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
	{"longer than a host name", permission.StandingApproval{
		Skill: "a-skill", Tools: permission.ToolsThatVisitAWebsite(),
		Host:  strings.Repeat("a", permission.MaxHostRunes+1) + ".com",
		Limit: 1, Expires: theTestTime.Add(time.Hour),
	}},
}

func TestAStandingApprovalTheHarnessCannotUseIsRefusedByName(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, bad := range badStandingApprovals {
		err := decider.RegisterStandingApproval(bad.approval)
		if err == nil {
			t.Errorf("the standing approval %+v was accepted, want an error saying what is missing", bad.approval)
			continue
		}
		if !strings.Contains(err.Error(), bad.named) {
			t.Errorf("the error for %+v is %q, and it has to name %q", bad.approval, err, bad.named)
		}
	}
}

func TestMoreStandingApprovalsThanTheCapAreRefused(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	var lastErr error
	for index := 0; index <= permission.MaxStandingApprovals; index++ {
		lastErr = decider.RegisterStandingApproval(permission.StandingApproval{
			Skill:       "a-skill",
			ReducedForm: "rm -rf",
			Limit:       1,
			Expires:     theTestTime.Add(time.Hour),
		})
	}

	if lastErr == nil {
		t.Fatalf("more than %d standing approvals were accepted, and the cap has to hold", permission.MaxStandingApprovals)
	}
}

// theShippedEntriesAndACallEachCatches is one call for each of the three entries
// the ask-me-first list ships with. A skill's standing approval is not the user's
// yes, so it may not cover any of them, however wide the readable form it names.
var theShippedEntriesAndACallEachCatches = []struct {
	entry    string
	toolName string
	fields   map[string]any
}{
	{contract.AskFirstBulkDelete, contract.ToolShell, map[string]any{"command": "rm -rf /home/user/nerdgenie"}},
	{contract.AskFirstSudo, contract.ToolShell, map[string]any{"command": "sudo apt install ripgrep"}},
	{contract.AskFirstSpendMoney, contract.ToolWeb, map[string]any{"url": "https://shop.example.com/checkout"}},
}

func TestAStandingApprovalNeverCoversTheAskMeFirstList(t *testing.T) {
	for _, one := range theShippedEntriesAndACallEachCatches {
		decider := newDecider(t, contract.DefaultConfig())
		registerStanding(t, decider, permission.StandingApproval{
			Skill:       "tidy-up",
			ReducedForm: "*",
			Limit:       1000,
			Expires:     theTestTime.Add(time.Hour),
		})
		request := contract.PermissionRequest{ToolName: one.toolName, Input: jsonInput(t, one.fields)}

		decision := decide(t, decider, request)
		if decision.Ruling != contract.RulingAsk {
			t.Errorf("%s: %q was ruled %q because %q, want %q; the ask-me-first list is read before any standing approval,"+
				" so a skill cannot hand itself the three things the user always sees first",
				one.entry, permission.Reduce(request), decision.Ruling, decision.Reason, contract.RulingAsk)
		}
	}
}

// registerStanding gives the permission function one standing approval and fails
// the test when it will not take it.
func registerStanding(t *testing.T, decider *permission.Decider, approval permission.StandingApproval) {
	t.Helper()
	if err := decider.RegisterStandingApproval(approval); err != nil {
		t.Fatalf("registering the standing approval for %q failed: %v", approval.Skill, err)
	}
}
