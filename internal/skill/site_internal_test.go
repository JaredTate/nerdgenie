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

// keptApprovals is a place to register standing approvals that keeps the
// readable forms it is given, so that a test can read what a permissions block
// turned into without going through the whole permission function.
type keptApprovals struct {
	forms []string
}

// RegisterStandingApproval keeps the readable form the approval covers.
func (kept *keptApprovals) RegisterStandingApproval(approval permission.StandingApproval) error {
	kept.forms = append(kept.forms, approval.ReducedForm)
	return nil
}

func TestTheFormsBuiltFromASiteHoldTheHostBetweenTheSchemeAndWhatFollowsIt(t *testing.T) {
	forms := approvalFormsFor("news.example.com", []string{contract.ToolWeb})
	want := []string{
		"web https://news.example.com",
		"web https://news.example.com/*",
		"web http://news.example.com",
		"web http://news.example.com/*",
	}
	if !slices.Equal(forms, want) {
		t.Fatalf("the forms are %v, want %v", forms, want)
	}
	for _, form := range forms {
		if strings.HasPrefix(form, "*") || strings.Contains(form, "*"+"news") {
			t.Errorf("the form %q leaves the front of the host open, so a call to somewhere else that carries this name would match it", form)
		}
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
	store := &Store{
		clock:      testkit.NewFakeClock(time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)),
		standing:   kept,
		registered: map[string]string{},
	}
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
	if len(kept.forms) != 0 {
		t.Errorf("the approvals %v were registered before the site was refused, and none may be", kept.forms)
	}
}
