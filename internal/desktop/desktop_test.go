package desktop

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aDesk is one desktop under test, with everything a test wants to look at.
type aDesk struct {
	desktop    *Desktop
	worker     *scriptedWorker
	channel    *testkit.FakeChannel
	permission *testkit.FakePermission
	starts     *int
	workers    *[]*scriptedWorker
}

// newDesk builds a desktop over a scripted worker that answers everything.
func newDesk(t *testing.T) *aDesk {
	t.Helper()
	channel := testkit.NewFakeChannel("terminal")
	permission := testkit.NewFakePermission(contract.RulingAllow)
	starts := 0
	workers := []*scriptedWorker{}
	var guard sync.Mutex

	start := func(_ context.Context) (*Connection, error) {
		guard.Lock()
		defer guard.Unlock()
		starts++
		worker := newScriptedWorker()
		worker.answer("launch", aDiff(true, ""))
		worker.answer("screenshot", aScreenshot())
		worker.answer("click", aDiff(true, ""))
		worker.answer("type", aDiff(true, ""))
		worker.answer("press", aDiff(true, ""))
		worker.answer("drag", aDiff(true, ""))
		worker.answer("clipboardGet", map[string]any{"text": "nine years of DigiByte"})
		worker.answer("clipboardSet", map[string]any{"characters": 22})
		workers = append(workers, worker)
		return worker.start(), nil
	}

	desktop, err := New(Options{
		Start:      start,
		Channel:    channel,
		Permission: permission,
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}
	t.Cleanup(func() { _ = desktop.Close() })
	return &aDesk{desktop: desktop, channel: channel, permission: permission, starts: &starts, workers: &workers}
}

// latestWorker is the worker the desktop is talking to now.
func (desk *aDesk) latestWorker(t *testing.T) *scriptedWorker {
	t.Helper()
	if len(*desk.workers) == 0 {
		t.Fatal("no worker was ever started")
	}
	return (*desk.workers)[len(*desk.workers)-1]
}

// launched grants the application and opens it.
func (desk *aDesk) launched(t *testing.T) {
	t.Helper()
	if err := desk.desktop.Launch(context.Background(), "zenity"); err != nil {
		t.Fatalf("launching the fixture application failed: %v", err)
	}
}

func TestBuildingADesktopWithoutEverythingItNeedsSaysWhatIsMissing(t *testing.T) {
	whole := Options{
		Start:      func(context.Context) (*Connection, error) { return nil, nil },
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	}
	missing := map[string]func(*Options){
		"start":      func(options *Options) { options.Start = nil },
		"channel":    func(options *Options) { options.Channel = nil },
		"permission": func(options *Options) { options.Permission = nil },
		"clock":      func(options *Options) { options.Clock = nil },
	}
	for name, take := range missing {
		options := whole
		take(&options)
		if _, err := New(options); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("leaving out the %s gave the error %v, want one naming it", name, err)
		}
	}
}

func TestAnApplicationTheUserHasNotGrantedIsRefused(t *testing.T) {
	desk := newDesk(t)
	desk.channel.AnswerPreviewsWith(contract.AnswerReject)

	err := desk.desktop.Launch(context.Background(), "gimp")

	if err == nil || !strings.Contains(err.Error(), "gimp") {
		t.Fatalf("the error is %v, want one naming the application the user refused", err)
	}
	if *desk.starts != 0 {
		t.Error("a worker was started for an application the user refused, and nothing should have been")
	}
}

func TestAnApplicationIsGrantedOncePerSession(t *testing.T) {
	desk := newDesk(t)

	desk.launched(t)
	desk.launched(t)

	if shown := len(desk.channel.Previews()); shown != 1 {
		t.Errorf("the user was shown %d previews for the same application, want one for the whole session", shown)
	}
	if *desk.starts != 1 {
		t.Errorf("the worker was started %d times, want once and kept alive", *desk.starts)
	}
}

func TestTheGrantPreviewSaysWhichApplicationAndWhatItMeans(t *testing.T) {
	desk := newDesk(t)

	desk.launched(t)

	preview := desk.channel.Previews()[0]
	if !strings.Contains(preview.Title, "zenity") && !strings.Contains(preview.Body, "zenity") {
		t.Errorf("the preview is %+v, want it to name the application", preview)
	}
	if len(preview.Body) < 20 {
		t.Errorf("the preview body is %q, want a sentence saying what the agent will be able to do", preview.Body)
	}
}

func TestNothingCanBeDoneBeforeAnApplicationIsOpen(t *testing.T) {
	desk := newDesk(t)
	ctx := context.Background()

	tries := map[string]func() error{
		"screenshot": func() error { _, err := desk.desktop.Screenshot(ctx); return err },
		"click":      func() error { return desk.desktop.Click(ctx, 1) },
		"type":       func() error { return desk.desktop.Type(ctx, "hello") },
		"press":      func() error { return desk.desktop.Press(ctx, "ctrl+s") },
		"drag":       func() error { return desk.desktop.Drag(ctx, 1, 2) },
	}
	for name, run := range tries {
		err := run()
		if err == nil || !strings.Contains(err.Error(), "launch") {
			t.Errorf("%s with nothing open gave %v, want an error saying to launch an application first", name, err)
		}
	}
	if *desk.starts != 0 {
		t.Error("a worker was started to answer a call that could not be answered, and none should have been")
	}
}

func TestAScreenshotComesBackInTheContractsOwnShape(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)

	picture, err := desk.desktop.Screenshot(context.Background())

	if err != nil {
		t.Fatalf("taking a screenshot failed: %v", err)
	}
	if picture.PNGBase64 == "" {
		t.Error("the screenshot has no picture in it")
	}
	want := []contract.DesktopMark{
		{Number: 1, Role: "text box", Name: "Type here"},
		{Number: 2, Role: "button", Name: "OK"},
	}
	if len(picture.Marks) != len(want) || picture.Marks[0] != want[0] || picture.Marks[1] != want[1] {
		t.Errorf("the marks are %+v, want %+v", picture.Marks, want)
	}
}

func TestAClickAndAKeyPressRunWithoutAPreview(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	ctx := context.Background()

	if err := desk.desktop.Click(ctx, 1); err != nil {
		t.Fatalf("clicking failed: %v", err)
	}
	if err := desk.desktop.Press(ctx, "ctrl+s"); err != nil {
		t.Fatalf("pressing a key failed: %v", err)
	}

	if asked := len(desk.permission.Requests()); asked != 0 {
		t.Errorf("the permission function was asked about %d calls, and a click and a key press can be undone", asked)
	}
}

func TestEveryActionThatCannotBeUndoneGoesThroughThePermissionFunction(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	ctx := context.Background()

	if err := desk.desktop.Type(ctx, "nine years of DigiByte"); err != nil {
		t.Fatalf("typing failed: %v", err)
	}
	if err := desk.desktop.Drag(ctx, 1, 2); err != nil {
		t.Fatalf("dragging failed: %v", err)
	}
	if err := desk.desktop.SetClipboard(ctx, "nine years of DigiByte"); err != nil {
		t.Fatalf("putting text on the clipboard failed: %v", err)
	}

	requests := desk.permission.Requests()
	if len(requests) != 3 {
		t.Fatalf("the permission function was asked about %d calls, want the typing, the drag, and the paste", len(requests))
	}
	for _, request := range requests {
		if request.ToolName != contract.ToolComputer {
			t.Errorf("the call was put as the tool %q, want %q", request.ToolName, contract.ToolComputer)
		}
		if !strings.Contains(string(request.Input), "intent") {
			t.Errorf("the call was put as %s, want an intent the permission function can read", request.Input)
		}
	}
}

func TestAnActionThePermissionFunctionAsksAboutIsPreviewedAndRemembered(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{
		Ruling:      contract.RulingAsk,
		Reason:      "typing into a field cannot be undone",
		PreviewText: "type \"nine years of DigiByte\" into the granted application",
	})

	if err := desk.desktop.Type(context.Background(), "nine years of DigiByte"); err != nil {
		t.Fatalf("typing after the user said yes failed: %v", err)
	}

	previews := desk.channel.Previews()
	if len(previews) != 2 {
		t.Fatalf("the user was shown %d previews, want the grant and the typing", len(previews))
	}
	if !strings.Contains(previews[1].Body, "nine years of DigiByte") {
		t.Errorf("the preview body is %q, want exactly what is about to be typed", previews[1].Body)
	}
	if answers := desk.permission.Answers(); len(answers) != 1 || answers[0].Answer != contract.AnswerOnce {
		t.Errorf("the answers remembered are %+v, want the one the user gave", answers)
	}
}

func TestAnActionTheUserRefusesIsNotDone(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{Ruling: contract.RulingAsk, PreviewText: "type something"})
	desk.channel.AnswerPreviewsWith(contract.AnswerReject)

	err := desk.desktop.Type(context.Background(), "nine years of DigiByte")

	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("the error is %v, want one saying the user refused it", err)
	}
	for _, asked := range desk.latestWorker(t).methodsAsked() {
		if asked == "type" {
			t.Error("the worker was asked to type after the user refused, and it must not have been")
		}
	}
}

func TestAnActionThePermissionFunctionDeniesSaysWhy(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{
		Ruling: contract.RulingDeny,
		Reason: "the user's rules refuse the desktop",
	})

	err := desk.desktop.Type(context.Background(), "nine years")

	if err == nil || !strings.Contains(err.Error(), "the user's rules refuse the desktop") {
		t.Fatalf("the error is %v, want one carrying the reason the call was refused", err)
	}
}

func TestAnActionThatWouldAskWithNobodyThereStopsTheTask(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{
		Ruling:      contract.RulingStop,
		Reason:      "nobody is there to answer",
		PreviewText: "type something",
	})

	err := desk.desktop.Type(context.Background(), "nine years")

	if err == nil || !strings.Contains(err.Error(), "nobody is there") {
		t.Fatalf("the error is %v, want one saying the task stops because nobody can answer", err)
	}
}

func TestTheClipboardGoesThereAndComesBack(t *testing.T) {
	desk := newDesk(t)
	ctx := context.Background()
	desk.launched(t)

	if err := desk.desktop.SetClipboard(ctx, "nine years of DigiByte"); err != nil {
		t.Fatalf("putting text on the clipboard failed: %v", err)
	}
	held, err := desk.desktop.Clipboard(ctx)

	if err != nil {
		t.Fatalf("reading the clipboard failed: %v", err)
	}
	if held != "nine years of DigiByte" {
		t.Errorf("the clipboard holds %q, want what was put on it", held)
	}
}

func TestAnExpectationThatWasNotMetIsReportedRatherThanPassedOver(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.latestWorker(t).answer("click", aDiff(false, "nothing changed"))

	err := desk.desktop.ClickExpecting(context.Background(), 1, "the dialog closes")

	if err == nil || !strings.Contains(err.Error(), "nothing changed") {
		t.Fatalf("the error is %v, want one saying what happened instead", err)
	}
}

func TestAControlThatIsNotOnTheScreenIsReportedWithItsNumber(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.latestWorker(t).refuse("click", &workerFailure{
		Code:    codeNoSuchMark,
		Message: "there is no control numbered 9 on the screen, so take a screenshot and use a number from it",
	})

	err := desk.desktop.Click(context.Background(), 9)

	if err == nil || !strings.Contains(err.Error(), "numbered 9") {
		t.Fatalf("the error is %v, want the worker's own message about the control", err)
	}
	if *desk.starts != 1 {
		t.Error("the worker was started again for a control that was simply not there, and it should not have been")
	}
}

func TestAWorkerThatDiesIsStartedAgainAndTheModelIsTold(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	first := desk.latestWorker(t)
	first.refuse("click", &workerFailure{Code: codeDriverUnavailable, Message: "the desktop driver went away"})

	err := desk.desktop.Click(context.Background(), 1)

	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("the error is %v, want one saying the desktop was interrupted", err)
	}
	if !first.wasStopped() {
		t.Error("the worker that died was not stopped, and it must be stopped by its exact process id")
	}
	if err := desk.desktop.Launch(context.Background(), "zenity"); err != nil {
		t.Fatalf("launching again after the restart failed: %v", err)
	}
	if *desk.starts != 2 {
		t.Errorf("the worker was started %d times, want a second one after the first died", *desk.starts)
	}
}

func TestAfterARestartTheApplicationHasToBeOpenedAgain(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.latestWorker(t).refuse("click", &workerFailure{Code: codeDriverUnavailable, Message: "the desktop driver went away"})
	_ = desk.desktop.Click(context.Background(), 1)

	err := desk.desktop.Click(context.Background(), 1)

	if err == nil || !strings.Contains(err.Error(), "launch") {
		t.Fatalf("the error is %v, want one saying the application has to be opened again", err)
	}
}

func TestAWorkerThatSaysItIsUnhealthyIsStoppedAndTheReasonIsGivenBack(t *testing.T) {
	channel := testkit.NewFakeChannel("terminal")
	worker := newScriptedWorker()
	worker.answer("health", map[string]any{"healthy": false, "detail": "there is no display to drive"})
	desktop, err := New(Options{
		Start:      func(context.Context) (*Connection, error) { return worker.start(), nil },
		Channel:    channel,
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}

	err = desktop.Launch(context.Background(), "zenity")

	if err == nil || !strings.Contains(err.Error(), "no display") {
		t.Fatalf("the error is %v, want the reason the worker gave", err)
	}
	if !worker.wasStopped() {
		t.Error("an unhealthy worker was left running, and it must be stopped")
	}
}

func TestAWorkerThatWillNotStartAtAllIsReported(t *testing.T) {
	desktop, err := New(Options{
		Start: func(context.Context) (*Connection, error) {
			return nil, errors.New("node is not installed on this machine")
		},
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}

	err = desktop.Launch(context.Background(), "zenity")

	if err == nil || !strings.Contains(err.Error(), "node is not installed") {
		t.Fatalf("the error is %v, want the reason the worker could not be started", err)
	}
}

func TestHealthSaysWhatTheWorkerSaid(t *testing.T) {
	desk := newDesk(t)

	health, err := desk.desktop.Health(context.Background())

	if err != nil {
		t.Fatalf("asking whether the desktop is healthy failed: %v", err)
	}
	if !health.Healthy || health.DriverVersion == "" {
		t.Errorf("the health is %+v, want a healthy worker naming its driver", health)
	}
}

func TestClosingStopsTheWorkerAndClosingTwiceIsHarmless(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)

	if err := desk.desktop.Close(); err != nil {
		t.Fatalf("closing the desktop failed: %v", err)
	}
	if err := desk.desktop.Close(); err != nil {
		t.Fatalf("closing the desktop twice failed: %v", err)
	}

	if !desk.latestWorker(t).wasStopped() {
		t.Error("the worker was left running after the desktop was closed")
	}
}

func TestTheRealDesktopKeepsTheDesktopContract(t *testing.T) {
	desk := newDesk(t)
	desk.channel.AnswerPreviewsWith(contract.AnswerReject)

	if err := testkit.CheckDesktop(context.Background(), desk.desktop); err != nil {
		t.Fatalf("the desktop does not keep the desktop contract: %v", err)
	}
}
