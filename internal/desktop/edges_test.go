package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// silentChannel is a channel that cannot reach the user at all, which is what
// happens when the terminal has gone and nobody is there to answer.
type silentChannel struct {
	*testkit.FakeChannel
}

// ShowPreview never gets to the user.
func (channel silentChannel) ShowPreview(context.Context, contract.Preview) (contract.PreviewAnswerWithReason, error) {
	return contract.PreviewAnswerWithReason{}, errors.New("the terminal has gone, so nobody can be shown anything")
}

// silentAfterTheGrant answers the grant and then cannot reach the user at all,
// which is what happens when the terminal goes away part way through a task.
type silentAfterTheGrant struct {
	*testkit.FakeChannel
	shown int
}

// ShowPreview answers the first preview and none after it.
func (channel *silentAfterTheGrant) ShowPreview(ctx context.Context, preview contract.Preview) (contract.PreviewAnswerWithReason, error) {
	channel.shown++
	if channel.shown == 1 {
		return channel.FakeChannel.ShowPreview(ctx, preview)
	}
	return contract.PreviewAnswerWithReason{}, errors.New("the terminal has gone, so nobody can be shown anything")
}

func TestAnApplicationWithNoNameIsRefusedBeforeAnybodyIsAsked(t *testing.T) {
	desk := newDesk(t)

	err := desk.desktop.Launch(context.Background(), "", "a window opens")

	if err == nil || !strings.Contains(err.Error(), "no name") {
		t.Fatalf("the error is %v, want one saying which application to open", err)
	}
	if shown := len(desk.channel.Previews()); shown != 0 {
		t.Errorf("the user was shown %d previews for an application with no name, want none", shown)
	}
}

func TestAnActionThatChangedNothingSaysSoEvenWhenTheWorkerAddedNothing(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.latestWorker(t).answer("click", aDiff(false, ""))

	err := desk.desktop.Click(context.Background(), 1, "")

	if err == nil || !strings.Contains(err.Error(), "changed nothing") {
		t.Fatalf("the error is %v, want one saying the action changed nothing", err)
	}
	if !strings.Contains(err.Error(), "said nothing more") {
		t.Errorf("the error is %v, want it to say the worker gave no more detail", err)
	}
}

func TestAPermissionRulingNobodyShipsIsRefusedRatherThanGuessedAt(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{Ruling: contract.PermissionRuling("maybe")})

	err := desk.desktop.Type(context.Background(), "nine years", "the text box holds the words")

	if err == nil || !strings.Contains(err.Error(), "maybe") {
		t.Fatalf("the error is %v, want one naming the ruling it did not understand", err)
	}
}

func TestAPreviewWithNoTextOfItsOwnStillSaysWhatIsAboutToHappen(t *testing.T) {
	cases := []struct {
		previewText string
		text        string
		want        string
	}{
		{previewText: "type into the editor", text: "", want: "type into the editor"},
		{previewText: "", text: "nine years", want: "nine years"},
		{previewText: "type into the editor", text: "nine years", want: "type into the editor\n\nnine years"},
	}
	for _, one := range cases {
		if body := previewBody(one.previewText, one.text); body != one.want {
			t.Errorf("the body for %+v is %q, want %q", one, body, one.want)
		}
	}
}

func TestAnUnhealthyWorkerThatSaysNothingStillTellsTheUserWhatToRun(t *testing.T) {
	worker := newScriptedWorker()
	worker.answer("health", map[string]any{"healthy": false})
	desktop, err := New(Options{
		Start:      func(context.Context) (*Connection, error) { return worker.start(), nil },
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}

	err = desktop.Launch(context.Background(), "zenity", "a window with a text box opens")

	if err == nil || !strings.Contains(err.Error(), "cua-driver doctor") {
		t.Fatalf("the error is %v, want one naming the command that says what is wrong", err)
	}
}

func TestReadingTheClipboardGoesThroughThePermissionFunctionAndItsPreview(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{
		Ruling:      contract.RulingAsk,
		Reason:      "the clipboard holds whatever the user copied last, which is often a password",
		PreviewText: "read what is on your clipboard",
	})

	held, err := desk.desktop.Clipboard(context.Background())

	if err != nil {
		t.Fatalf("reading the clipboard after the user said yes failed: %v", err)
	}
	if held != "nine years of DigiByte" {
		t.Errorf("the clipboard holds %q, want what the worker said was on it", held)
	}
	requests := desk.permission.Requests()
	if len(requests) != 1 || requests[0].ToolName != contract.ToolComputer {
		t.Fatalf("the permission function was asked about %+v, want the one clipboard read", requests)
	}
	previews := desk.channel.Previews()
	if len(previews) != 2 || !strings.Contains(previews[1].Body, "read what is on your clipboard") {
		t.Errorf("the user was shown %+v, want the grant and a preview of the clipboard read", previews)
	}
}

func TestAClipboardReadTheUserRefusesHandsBackNothing(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{Ruling: contract.RulingAsk, PreviewText: "read your clipboard"})
	desk.channel.AnswerPreviewsWith(contract.AnswerReject)

	held, err := desk.desktop.Clipboard(context.Background())

	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("the error is %v, want one saying the user refused the clipboard read", err)
	}
	if held != "" {
		t.Errorf("the clipboard read handed back %q after the user refused it, and it must hand back nothing", held)
	}
	for _, asked := range desk.latestWorker(t).methodsAsked() {
		if asked == "clipboardGet" {
			t.Error("the worker was asked for the clipboard after the user refused, and it must not have been")
		}
	}
}

func TestTheClipboardReportsAWorkerThatWillNotAnswer(t *testing.T) {
	desk := newDesk(t)
	ctx := context.Background()
	desk.launched(t)
	desk.latestWorker(t).refuse("clipboardGet", &workerFailure{Code: codeDriverUnavailable, Message: "the clipboard is not reachable"})

	if _, err := desk.desktop.Clipboard(ctx); err == nil {
		t.Error("reading a clipboard that is not reachable was reported as a success")
	}

	// That failure stopped the worker, so the next call starts a fresh one and
	// the fresh one has to be told to refuse as well.
	desk.launched(t)
	desk.latestWorker(t).refuse("clipboardSet", &workerFailure{Code: codeDriverUnavailable, Message: "the clipboard is not reachable"})
	if err := desk.desktop.SetClipboard(ctx, "nine years"); err == nil {
		t.Error("writing a clipboard that is not reachable was reported as a success")
	}
}

func TestAScreenshotAndAHealthCheckBothReportAWorkerThatWillNotAnswer(t *testing.T) {
	desk := newDesk(t)
	ctx := context.Background()
	desk.launched(t)
	worker := desk.latestWorker(t)
	worker.refuse("screenshot", &workerFailure{Code: codeUnreadableWindow, Message: "the window could not be read at all"})

	if _, err := desk.desktop.Screenshot(ctx); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Errorf("taking a screenshot of an unreadable window gave %v, want the worker's own message", err)
	}

	worker.refuse("health", &workerFailure{Code: codeDriverUnavailable, Message: "the driver went away"})
	if _, err := desk.desktop.Health(ctx); err == nil {
		t.Error("asking an unreachable worker whether it is healthy was reported as a success")
	}
}

func TestADragTheUserRefusesIsNotDone(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.permission.Rule(contract.ToolComputer, contract.PermissionDecision{Ruling: contract.RulingAsk, PreviewText: "drag one control onto another"})
	desk.channel.AnswerPreviewsWith(contract.AnswerReject)

	err := desk.desktop.Drag(context.Background(), 1, 2, "the file lands in the folder")

	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("the error is %v, want one saying the user refused the drag", err)
	}
}

func TestAWorkerIsStoppedByTheExactProcessIDItWasStartedWith(t *testing.T) {
	notes := []string{}
	worker := workerAnsweringEverything()
	worker.refuse("click", &workerFailure{Code: codeDriverUnavailable, Message: "the desktop driver went away"})
	desktop, err := New(Options{
		Start:      func(context.Context) (*Connection, error) { return worker.start(), nil },
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
		Note:       func(format string, arguments ...any) { notes = append(notes, fmt.Sprintf(format, arguments...)) },
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}
	if err := desktop.Launch(context.Background(), "zenity", "a window with a text box opens"); err != nil {
		t.Fatalf("launching the fixture application failed: %v", err)
	}

	_ = desktop.Click(context.Background(), 1, "the dialog closes")

	// The scripted worker hands back process id 4242, and the note has to name
	// that one, because an exact process id is the only thing this package ever
	// kills.
	var named bool
	for _, note := range notes {
		if strings.Contains(note, "was stopped") && strings.Contains(note, "4242") {
			named = true
		}
	}
	if !named {
		t.Errorf("the notes are %v, want one naming the exact process id of the worker that was stopped", notes)
	}
}

func TestClosingADesktopThatNeverStartedAWorkerIsHarmless(t *testing.T) {
	desk := newDesk(t)

	if err := desk.desktop.Close(); err != nil {
		t.Fatalf("closing a desktop that never started a worker failed: %v", err)
	}
	if *desk.starts != 0 {
		t.Errorf("closing a desktop started %d workers, and it must start none", *desk.starts)
	}
}

func TestAStopOfAWorkerThatIsAlreadyGoneIsHarmless(t *testing.T) {
	start, err := ProcessStart([]string{"sh", "-c", "exit 0"}, nil)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}
	connection, err := start(context.Background())
	if err != nil {
		t.Fatalf("starting the stand-in worker failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := connection.Stop(); err != nil {
		t.Fatalf("stopping a worker that had already gone failed: %v", err)
	}
}

func TestAWorkerThatWillNotTakeAStopSignalIsMadeToGo(t *testing.T) {
	start, err := ProcessStart([]string{"sh", "-c", "trap '' TERM; sleep 30"}, nil)
	if err != nil {
		t.Fatalf("building the start function failed: %v", err)
	}
	connection, err := start(context.Background())
	if err != nil {
		t.Fatalf("starting the stubborn stand-in failed: %v", err)
	}

	if err := connection.Stop(); err != nil {
		t.Fatalf("stopping the stubborn stand-in failed: %v", err)
	}
	if err := syscall.Kill(connection.ProcessID, 0); err == nil {
		t.Errorf("the process %d is still there after it was made to go", connection.ProcessID)
	}
}

func TestARequestThatCannotBeWrittenDownIsReported(t *testing.T) {
	worker := newScriptedWorker()
	talker := newClient(worker.start())

	err := talker.call(context.Background(), "click", map[string]any{"mark": make(chan int)}, &diffAnswer{})

	if err == nil || !strings.Contains(err.Error(), "could not be written") {
		t.Fatalf("the error is %v, want one saying the request could not be written down", err)
	}
}

func TestALongLineInAnErrorMessageIsCutShort(t *testing.T) {
	shown := firstPart([]byte(strings.Repeat("x", 400)))

	if len(shown) > 130 || !strings.HasSuffix(shown, "...") {
		t.Errorf("the shortened line is %d characters ending %q, want it cut short", len(shown), shown)
	}
}

func TestTheStartFunctionThatHandsBackNothingIsReported(t *testing.T) {
	desktop, err := New(Options{
		Start:      func(context.Context) (*Connection, error) { return nil, nil },
		Channel:    testkit.NewFakeChannel("terminal"),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}

	err = desktop.Launch(context.Background(), "zenity", "a window with a text box opens")

	if err == nil || !strings.Contains(err.Error(), "wired") {
		t.Fatalf("the error is %v, want one saying the worker was wired wrongly", err)
	}
}

func TestAUserWhoCannotBeAskedIsReportedRatherThanTakenAsAYes(t *testing.T) {
	desktop, err := New(Options{
		Start:      func(context.Context) (*Connection, error) { return newScriptedWorker().start(), nil },
		Channel:    silentChannel{FakeChannel: testkit.NewFakeChannel("terminal")},
		Permission: testkit.NewFakePermission(contract.RulingAllow),
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}

	err = desktop.Launch(context.Background(), "zenity", "a window with a text box opens")

	if err == nil || !strings.Contains(err.Error(), "could not be asked") {
		t.Fatalf("the error is %v, want one saying the user could not be asked", err)
	}
}

func TestAnActionThePermissionFunctionAsksAboutWithNobodyToAskIsReported(t *testing.T) {
	permission := testkit.NewFakePermission(contract.RulingAllow)
	desktop, err := New(Options{
		Start:      func(context.Context) (*Connection, error) { return workerAnsweringEverything().start(), nil },
		Channel:    &silentAfterTheGrant{FakeChannel: testkit.NewFakeChannel("terminal")},
		Permission: permission,
		Clock:      testkit.NewFakeClock(time.Unix(0, 0).UTC()),
	})
	if err != nil {
		t.Fatalf("building the desktop failed: %v", err)
	}
	if err := desktop.Launch(context.Background(), "zenity", "a window with a text box opens"); err != nil {
		t.Fatalf("opening the fixture application failed: %v", err)
	}
	permission.Rule(contract.ToolComputer, contract.PermissionDecision{Ruling: contract.RulingAsk, PreviewText: "paste something"})

	err = desktop.SetClipboard(context.Background(), "nine years")

	if err == nil || !strings.Contains(err.Error(), "could not be shown") {
		t.Fatalf("the error is %v, want one saying the user could not be shown the preview", err)
	}
}
