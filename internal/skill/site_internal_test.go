package skill

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/testkit"
)

// keptApprovals is a place to register standing approvals that keeps the ones
// it is given, so that a test can read what a permissions block turned into
// without going through the whole permission function.
type keptApprovals struct {
	held []permission.StandingApproval
}

// RegisterStandingApproval keeps the approval.
func (kept *keptApprovals) RegisterStandingApproval(approval permission.StandingApproval) error {
	kept.held = append(kept.held, approval)
	return nil
}

func TestTheApprovalBuiltFromASiteNamesTheHostAndTheToolsThatVisitIt(t *testing.T) {
	kept := &keptApprovals{}
	store := storeOverTheseApprovals(kept)
	folder := Folder{
		Definition: Definition{
			Name:        "read-the-news",
			Permissions: Permissions{Sites: []string{"news.example.com"}, DailyLimit: 10},
		},
		Steps: []Step{{Number: 1, Tool: contract.ToolWeb, Input: `{"url": "https://news.example.com/story"}`}},
	}

	if err := store.registerApprovals(folder); err != nil {
		t.Fatalf("registering the approvals failed: %v", err)
	}
	if len(kept.held) != 1 {
		t.Fatalf("the block turned into %d approvals, want one for the one website it names", len(kept.held))
	}
	approval := kept.held[0]
	if approval.Host != "news.example.com" || approval.ReducedForm != "" {
		t.Errorf("the approval is %+v, want it to name the host and no readable form, so that the permission function compares hosts rather than matching a pattern", approval)
	}
	if !slices.Equal(approval.Tools, permission.ToolsThatVisitAWebsite()) {
		t.Errorf("the approval covers the tools %v, want the tools that visit a website, so that a website can never approve a command run on this machine", approval.Tools)
	}
	if approval.Skill != "read-the-news" || approval.Limit != 10 || approval.Expires.IsZero() {
		t.Errorf("the approval is %+v, want it to name the skill, carry the block's daily limit, and run out at midnight", approval)
	}
}

// storeOverTheseApprovals is a store with nothing but a clock and somewhere to
// register standing approvals, which is all the registering needs.
func storeOverTheseApprovals(kept *keptApprovals) *Store {
	return &Store{
		clock:      testkit.NewFakeClock(time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)),
		standing:   kept,
		registered: map[string]string{},
	}
}

func TestASiteThatSaysNothingIsNotAHostName(t *testing.T) {
	err := checkSite("")
	if err == nil {
		t.Fatal("a site of no characters was read as a host name, and a standing approval cannot be built from nothing")
	}
	if !strings.Contains(err.Error(), "host name") {
		t.Errorf("the message is %q, and it has to say a site line names a host name", err)
	}
}

func TestAFolderWhoseSiteIsNotAHostNameGetsNoStandingApproval(t *testing.T) {
	kept := &keptApprovals{}
	store := storeOverTheseApprovals(kept)
	folder := Folder{
		Definition: Definition{
			Name:        "tidy-up",
			Permissions: Permissions{Sites: []string{"*"}, DailyLimit: 10},
		},
		Steps: []Step{{Number: 1, Tool: contract.ToolWeb, Input: `{"url": "https://example.com/"}`}},
	}

	err := store.registerApprovals(folder)
	if err == nil {
		t.Fatal("a folder whose site is a star was given standing approvals; the site is checked again here because this is where a" +
			" site line turns into permission to act, whatever built the folder")
	}
	if !strings.Contains(err.Error(), "tidy-up") || !strings.Contains(err.Error(), "host name") {
		t.Errorf("the message is %q, and it has to name the skill and say a site is one bare host name", err)
	}
	if len(kept.held) != 0 {
		t.Errorf("the approvals %v were registered before the site was refused, and none may be", kept.held)
	}
}
