package testkit_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// undecidedPermission rules with a word that is not allow, ask, or deny.
type undecidedPermission struct{ *testkit.FakePermission }

// Decide returns a ruling nobody defined.
func (undecidedPermission) Decide(context.Context, contract.PermissionRequest) (contract.PermissionDecision, error) {
	return contract.PermissionDecision{Ruling: "perhaps"}, nil
}

// silentAsker asks the user without saying what is about to happen.
type silentAsker struct{ *testkit.FakePermission }

// Decide asks, and gives the user nothing to look at.
func (silentAsker) Decide(context.Context, contract.PermissionRequest) (contract.PermissionDecision, error) {
	return contract.PermissionDecision{Ruling: contract.RulingAsk}, nil
}

// agreeablePermission accepts any answer at all.
type agreeablePermission struct{ *testkit.FakePermission }

// Remember accepts a word that is not one of the three answers.
func (agreeablePermission) Remember(contract.PermissionRequest, contract.PreviewAnswer, string) error {
	return nil
}

// forgetfulPermission files every answer it understands as an always, so a
// reject the user gave with a reason is allowed anyway and the reason is thrown
// away. It is the mutation the reviewer named.
type forgetfulPermission struct{ *testkit.FakePermission }

// Remember files an answer it understands as an always, and passes an answer it
// does not understand through so that the refusal still happens.
func (forgetful forgetfulPermission) Remember(request contract.PermissionRequest, answer contract.PreviewAnswer, reason string) error {
	if !contract.KnownPreviewAnswer(answer) {
		return forgetful.FakePermission.Remember(request, answer, reason)
	}
	return forgetful.FakePermission.Remember(request, contract.AnswerAlways, "")
}

func TestThePermissionCheckCatchesADeciderThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		permission contract.Permission
	}{
		{"a ruling nobody defined", undecidedPermission{testkit.NewFakePermission(contract.RulingAllow)}},
		{"an ask with nothing to show the user", silentAsker{testkit.NewFakePermission(contract.RulingAllow)}},
		{"an answer of maybe that was accepted", agreeablePermission{testkit.NewFakePermission(contract.RulingAllow)}},
		{"a reject filed as an always", forgetfulPermission{testkit.NewFakePermission(contract.RulingAllow)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckPermission(ctx, test.permission); err == nil {
				t.Fatal("the permission check passed, and it was given a decider that breaks a promise")
			}
		})
	}
}

// imaginativeRegistry finds a tool nobody ever registered.
type imaginativeRegistry struct{ *testkit.FakeToolRegistry }

// Lookup finds anything it is asked for.
func (imaginativeRegistry) Lookup(string) (contract.Tool, bool) {
	return testkit.NewScriptedTool(contract.ToolSpec{Name: "invented", Description: "Invented."}, "nothing"), true
}

func TestTheToolRegistryCheckCatchesARegistryThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()

	wordy := testkit.NewFakeToolRegistry(testkit.NewScriptedTool(contract.ToolSpec{
		Name:        contract.ToolRead,
		Description: strings.Repeat("word ", contract.MaxToolDescriptionWords+1),
	}, "read"))
	if err := testkit.CheckToolRegistry(ctx, wordy); err == nil {
		t.Error("the registry check passed a description longer than the forty-word cap")
	}

	nameless := testkit.NewFakeToolRegistry(testkit.NewScriptedTool(contract.ToolSpec{Description: "No name."}, "read"))
	if err := testkit.CheckToolRegistry(ctx, nameless); err == nil {
		t.Error("the registry check passed a tool with no name")
	}

	if err := testkit.CheckToolRegistry(ctx, imaginativeRegistry{testkit.NewFakeToolRegistry()}); err == nil {
		t.Error("the registry check passed a registry that finds a tool nobody registered")
	}
}

// agreeableSkillStore saves a skill folder with nothing in it.
type agreeableSkillStore struct{ *testkit.FakeSkill }

// Save accepts a folder with no files.
func (agreeableSkillStore) Save(context.Context, string, map[string][]byte) error { return nil }

// forgetfulSkillStore saves a skill and then does not list it.
type forgetfulSkillStore struct{ *testkit.FakeSkill }

// List never mentions what was saved.
func (forgetfulSkillStore) List(context.Context) ([]contract.SkillSummary, error) { return nil, nil }

func TestTheSkillCheckCatchesAStoreThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		skills contract.Skill
	}{
		{"a store that saves a folder with no files", agreeableSkillStore{testkit.NewFakeSkill()}},
		{"a store that saves a skill and does not list it", forgetfulSkillStore{testkit.NewFakeSkill()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckSkill(ctx, test.skills); err == nil {
				t.Fatal("the skill check passed, and it was given a store that breaks a promise")
			}
		})
	}
}

// workingJobStore is the fake job store the broken ones below are built from.
func workingJobStore() *testkit.FakeJob {
	return testkit.NewFakeJob(testkit.NewFakeClock(zeroTime()))
}

// agreeableJobStore creates a job with no ask at all.
type agreeableJobStore struct{ *testkit.FakeJob }

// Create accepts a job with no ask.
func (agreeableJobStore) Create(context.Context, contract.NewJob) (string, error) { return "1", nil }

// forgivingJobStore pauses a job that does not exist.
type forgivingJobStore struct{ *testkit.FakeJob }

// Pause never says there is no such job.
func (forgivingJobStore) Pause(context.Context, string) error { return nil }

// oddlyNumberedJobStore gives its tasks identifiers of the wrong shape.
type oddlyNumberedJobStore struct{ *testkit.FakeJob }

// AddTask returns an identifier the design does not use.
func (oddlyNumberedJobStore) AddTask(context.Context, contract.NewTask) (string, error) {
	return "task-one", nil
}

func TestTheJobCheckCatchesAStoreThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		jobs contract.Job
	}{
		{"a store that creates a job with no ask", agreeableJobStore{workingJobStore()}},
		{"a store that pauses a job that is not there", forgivingJobStore{workingJobStore()}},
		{"a store whose task identifiers are the wrong shape", oddlyNumberedJobStore{workingJobStore()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := testkit.CheckJob(ctx, test.jobs); err == nil {
				t.Fatal("the job check passed, and it was given a store that breaks a promise")
			}
		})
	}
}

// permissiveDesktop opens any application at all.
type permissiveDesktop struct{ *testkit.FakeDesktop }

// Launch opens whatever it is given, granted or not.
func (permissiveDesktop) Launch(context.Context, string) error { return nil }

func TestTheDesktopCheckCatchesADesktopThatOpensAnythingAtAll(t *testing.T) {
	if err := testkit.CheckDesktop(context.Background(), permissiveDesktop{testkit.NewFakeDesktop()}); err == nil {
		t.Fatal("the desktop check passed, and an application nobody granted was opened")
	}
}

// leakyBrowser hands the password back inside the diff.
type leakyBrowser struct{ *testkit.FakeBrowserWorker }

// LoginFill returns a diff holding the password it was given.
func (worker leakyBrowser) LoginFill(_ context.Context, fields contract.LoginFields) (contract.Diff, error) {
	return contract.Diff{Seen: "typed " + fields.Password, ExpectationMet: true}, nil
}

// echoingBrowser hands the password back in an element that appeared, which is
// the one place the old scrubber did not look.
type echoingBrowser struct{ *testkit.FakeBrowserWorker }

// LoginFill fills the form properly and then adds a message naming the password.
func (worker echoingBrowser) LoginFill(ctx context.Context, fields contract.LoginFields) (contract.Diff, error) {
	diff, err := worker.FakeBrowserWorker.LoginFill(ctx, fields)
	if err != nil {
		return diff, err
	}
	diff.NewElements = append(diff.NewElements, contract.Element{
		Ref: "e9", Role: "alert", Name: "we could not sign you in with " + fields.Password, New: true,
	})
	return diff, nil
}

// eagerBrowser clicks before any page is open.
type eagerBrowser struct{ *testkit.FakeBrowserWorker }

// Click acts with nothing on the screen.
func (eagerBrowser) Click(context.Context, string, string) (contract.Diff, error) {
	return contract.Diff{ExpectationMet: true}, nil
}

func TestTheBrowserCheckCatchesAWorkerThatBreaksOnePromise(t *testing.T) {
	ctx := context.Background()

	leaky := leakyBrowser{testkit.NewFakeBrowserWorker()}
	defer leaky.Close()
	if err := testkit.CheckBrowserWorker(ctx, leaky); err == nil {
		t.Error("the browser check passed a worker that hands the password back")
	}

	echoing := echoingBrowser{testkit.NewFakeBrowserWorker()}
	defer echoing.Close()
	if err := testkit.CheckBrowserWorker(ctx, echoing); err == nil {
		t.Error("the browser check passed a worker that hands the password back in an element that appeared")
	}

	eager := eagerBrowser{testkit.NewFakeBrowserWorker()}
	defer eager.Close()
	if err := testkit.CheckBrowserWorker(ctx, eager); err == nil {
		t.Error("the browser check passed a worker that clicks with no page open")
	}
}

// zeroTime is the moment every fake clock in these tests starts from.
func zeroTime() time.Time {
	return time.Unix(0, 0).UTC()
}
