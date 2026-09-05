package contract_test

import (
	"reflect"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestEveryCrossWaveInterfaceExists is the compile-time proof that the fifteen
// interfaces every later wave codes against are declared here and nowhere else.
// The doc comment on each one is enforced separately by the style checker.
func TestEveryCrossWaveInterfaceExists(t *testing.T) {
	interfaces := map[string]reflect.Type{
		"Model":         reflect.TypeFor[contract.Model](),
		"Tool":          reflect.TypeFor[contract.Tool](),
		"ToolRegistry":  reflect.TypeFor[contract.ToolRegistry](),
		"Channel":       reflect.TypeFor[contract.Channel](),
		"Permission":    reflect.TypeFor[contract.Permission](),
		"Memory":        reflect.TypeFor[contract.Memory](),
		"Skill":         reflect.TypeFor[contract.Skill](),
		"Job":           reflect.TypeFor[contract.Job](),
		"Sandbox":       reflect.TypeFor[contract.Sandbox](),
		"Secrets":       reflect.TypeFor[contract.Secrets](),
		"BrowserWorker": reflect.TypeFor[contract.BrowserWorker](),
		"Desktop":       reflect.TypeFor[contract.Desktop](),
		"Clock":         reflect.TypeFor[contract.Clock](),
		"Ticker":        reflect.TypeFor[contract.Ticker](),
		"Store":         reflect.TypeFor[contract.Store](),
	}
	for name, kind := range interfaces {
		if kind.Kind() != reflect.Interface {
			t.Errorf("%s is a %s, want an interface", name, kind.Kind())
		}
		if kind.NumMethod() == 0 {
			t.Errorf("%s has no methods, and an empty interface is not a contract", name)
		}
	}
}

func TestTheBrowserWorkerHasTheTwelveProtocolMethodsTheEventStreamAndClose(t *testing.T) {
	worker := reflect.TypeFor[contract.BrowserWorker]()

	wanted := []string{
		"Open", "Read", "Click", "Type", "Press", "Scroll",
		"Act", "Tabs", "LoginFill", "Screenshot", "Health", "Dialog", "Events", "Close",
	}
	for _, name := range wanted {
		if _, found := worker.MethodByName(name); !found {
			t.Errorf("BrowserWorker has no %s method, and worker/browser/PROTOCOL.md names it", name)
		}
	}
	if worker.NumMethod() != len(wanted) {
		t.Errorf("BrowserWorker has %d methods, want the twelve protocol methods, the event stream, and Close", worker.NumMethod())
	}
}

func TestTheChannelHasTheSixThingsEveryChannelDoes(t *testing.T) {
	channel := reflect.TypeFor[contract.Channel]()

	wanted := []string{"Name", "Receive", "Send", "SendFile", "ShowPreview", "AskSecret", "Health"}
	for _, name := range wanted {
		if _, found := channel.MethodByName(name); !found {
			t.Errorf("Channel has no %s method, and design section 6 names it", name)
		}
	}
	if channel.NumMethod() != len(wanted) {
		t.Errorf("Channel has %d methods, want its name plus the six things a channel does", channel.NumMethod())
	}
}

func TestTheJobContractIsTheSixOperationsTheDesignNamesPlusTheSixTheLoopNeeds(t *testing.T) {
	job := reflect.TypeFor[contract.Job]()

	// The design names six operations. The loop needs six more to run a job's
	// tasks: the next due task, a finished task's report, the job's record, the
	// two that put a job down on a task and say which task it holds on, so that
	// the person's next word or answer picks that task up after a restart, and
	// Resume, which sets the put-down job running again and touches nothing
	// else, where RunNow would take a date off a task and fire a tick.
	wanted := []string{
		"Create", "AddTask", "List", "RunNow", "Pause", "SwitchOff",
		"NextTask", "FinishTask", "Load", "PutDown", "PutDownTask", "Resume",
	}
	for _, name := range wanted {
		if _, found := job.MethodByName(name); !found {
			t.Errorf("Job has no %s method, and the design or the loop needs it", name)
		}
	}
	if job.NumMethod() != len(wanted) {
		t.Errorf("Job has %d methods, want the twelve listed in this test", job.NumMethod())
	}
}

func TestTheExitCodesAreTheOnesTheServiceUnitReads(t *testing.T) {
	if contract.ExitRestartMe != 75 {
		t.Errorf("the restart-me exit code is %d, want 75", contract.ExitRestartMe)
	}
	if contract.ExitBadConfiguration != 78 {
		t.Errorf("the bad-configuration exit code is %d, want 78", contract.ExitBadConfiguration)
	}
	if contract.ExitOK != 0 {
		t.Errorf("the success exit code is %d, want 0", contract.ExitOK)
	}
	if contract.ExitFailure == contract.ExitOK || contract.ExitUsage == contract.ExitOK {
		t.Error("the failure and usage exit codes must differ from the success code")
	}
}

func TestTheAskMeFirstListShipsWithThreeEntriesInPlainWords(t *testing.T) {
	list := contract.DefaultAskMeFirst()

	if len(list) != 3 {
		t.Fatalf("the ask-me-first list ships with %d entries, want the three from design section 11", len(list))
	}
	for _, entry := range list {
		if len(entry) < 10 {
			t.Errorf("the ask-me-first entry %q is too short to read as plain words", entry)
		}
	}
}

func TestARecordCanHoldATaskAndAJobWithOneSetOfTypes(t *testing.T) {
	task := contract.Record{
		Header: contract.Header{Kind: contract.RecordTask, ID: "17", Status: contract.StatusRunning},
		Work:   contract.Work{Plan: []contract.PlanStep{{Number: 1, Text: "read the product notes", Done: true, ResultID: "r3"}}},
	}
	job := contract.Record{
		Header: contract.Header{Kind: contract.RecordJob, ID: "4", Status: contract.StatusRunning, TasksDone: 3, TasksTotal: 12},
		Work:   contract.Work{Tasks: []contract.JobTask{{TaskID: "t17", Text: "post the anniversary tweet", Done: true, ReportID: "j4.1"}}},
	}

	if task.Header.Kind != contract.RecordTask || job.Header.Kind != contract.RecordJob {
		t.Error("the kind field does not tell a task record from a job record")
	}
	if task.Work.Plan[0].ResultID != contract.ResultID(3) {
		t.Errorf("a task plan step points at %q, want %q", task.Work.Plan[0].ResultID, contract.ResultID(3))
	}
	if job.Work.Tasks[0].ReportID != contract.ReportID("4", 1) {
		t.Errorf("a job task points at %q, want %q", job.Work.Tasks[0].ReportID, contract.ReportID("4", 1))
	}
}

func TestThePermissionRulingsAreAllowAskDenyAndStop(t *testing.T) {
	for _, ruling := range []contract.PermissionRuling{contract.RulingAllow, contract.RulingAsk, contract.RulingDeny, contract.RulingStop} {
		if !contract.KnownPermissionRuling(ruling) {
			t.Errorf("the ruling %q is not known, and the loop must be able to read it", ruling)
		}
	}
	if contract.KnownPermissionRuling("maybe") {
		t.Error("the ruling \"maybe\" is known, and only the four rulings should be")
	}
	if contract.RulingStop != "stop" {
		t.Errorf("the stop ruling is %q, want \"stop\"", contract.RulingStop)
	}
}

func TestASnapshotCanReportAWallAndADiffSaysWhetherThePageSettled(t *testing.T) {
	snapshot := contract.Snapshot{URL: "https://x.com/login", Wall: &contract.Wall{Kind: contract.WallLogin, Detail: "a password field"}}
	if snapshot.Wall == nil || snapshot.Wall.Kind != contract.WallLogin {
		t.Errorf("a snapshot did not hold the wall it hit: %+v", snapshot)
	}
	diff := contract.Diff{URL: "https://x.com/home", Settled: false, Seen: "the page kept changing"}
	if diff.Settled {
		t.Error("a diff for a page that never settled reads as settled")
	}
	for _, action := range []contract.DialogAction{contract.DialogAccept, contract.DialogDismiss} {
		if !contract.KnownDialogAction(action) {
			t.Errorf("the dialog action %q is not known", action)
		}
	}
	if contract.KnownDialogAction("ignore") {
		t.Error("the dialog action \"ignore\" is known, and only accept and dismiss should be")
	}
}

func TestShowPreviewReturnsTheAnswerWithItsReason(t *testing.T) {
	method, found := reflect.TypeFor[contract.Channel]().MethodByName("ShowPreview")
	if !found {
		t.Fatal("Channel has no ShowPreview")
	}
	if method.Type.NumOut() != 2 || method.Type.Out(0) != reflect.TypeFor[contract.PreviewAnswerWithReason]() {
		t.Errorf("ShowPreview returns %v, want (PreviewAnswerWithReason, error) so a rejection carries its reason", method.Type)
	}
}
