// The tests of the harness taking the photograph. The sky task of the
// flight-simulator work order spent hours steering the aircraft for a
// distinct photograph of each element, then doubting the photograph, because
// a done line that says "seen and photographed" was proved only by the
// model's own picture and its own judgment of it. Now the harness takes the
// picture itself, through the same browser tools the model has, and reads
// off the tool's own line whether the page was drawing.
package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The two shapes of the screenshot tool's frames line, word for word as
// brief 9.1 writes them: on a new picture, and on a picture that is the same
// as the last one.
const (
	aLivePicture     = "the picture is saved at /pictures/sky-1.png\nthe page drew 60 frames in a quarter of a second"
	aSameLivePicture = "the picture is the same as the last one: the page drew 58 frames in a quarter of a second, so it is alive and nothing on it moved; change the view, move the camera or the aircraft, or act on the page to see something new [/pictures/sky-1.png]"
	aStillPicture    = "the picture is saved at /pictures/sky-1.png\nthe page drew no frames in a quarter of a second; the tab is hidden"
	aStillVisiblePicture = "the picture is saved at /pictures/board-1.png\nthe page drew no frames in a quarter of a second; it is visible and answers"
)

// theNoFramesRefusal is the refusal's sentence, in the exact words the brief
// fixed.
const theNoFramesRefusal = "the harness photographed the page for this line, but the page drew no frames, so the picture proves nothing; make the page draw, then close"

// thePhotographTools are the two browser tools the photograph goes through,
// scripted: the camera answers with the pictures given, in order.
func thePhotographTools(pictures ...string) (resize, shot *testkit.ScriptedTool) {
	resize = testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserResize, Description: "A resize the test scripted."},
		"the page is 1440 wide", "the page is 390 wide", "the page is 768 wide")
	shot = testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolBrowserScreenshot, Description: "A camera the test scripted."}, pictures...)
	return resize, shot
}

// TestADoneLineThatSaysPhotographedIsProvedByTheHarnessesOwnPicture: a done
// line whose text holds the word photograph is photographed by the harness
// through the browser_screenshot tool when the model says it is done, at the
// page's current size when the line names no width, and pinned to the
// picture's result when the page drew frames.
func TestADoneLineThatSaysPhotographedIsProvedByTheHarnessesOwnPicture(t *testing.T) {
	resize, shot := thePhotographTools(aLivePicture)
	built := aTaskWhoseDoneLineIsChecked(t, "The sky and the clouds are seen and photographed.", nil, resize, shot)

	outcome := built.ask(t, "prove the sky")

	theLineIsProvedByACheck(t, built, outcome, "photograph")
	if len(shot.Inputs()) != 1 || len(resize.Inputs()) != 0 {
		t.Errorf("the page was photographed %d times and resized %d, want once and never: the line names no width", len(shot.Inputs()), len(resize.Inputs()))
	}
	keeper, err := record.Load(t.Context(), built.store, contract.RecordTask, outcome.TaskID)
	if err != nil {
		t.Fatalf("cannot load the record of task %s: %v", outcome.TaskID, err)
	}
	text, err := keeper.Read(t.Context(), keeper.Record().Goal.DoneWhen[0].ResultID)
	if err != nil || !strings.Contains(text, "the page drew 60 frames in a quarter of a second") {
		t.Errorf("the picture's result reads %q (%v), want the camera's own words in it", text, err)
	}
}

// TestAPhotographedLineNamingTwoWidthsIsPhotographedAtEach: a line that names
// widths, "at 1440" or "390 wide", is photographed at each through
// browser_resize first, in the line's order, and at most two widths.
func TestAPhotographedLineNamingTwoWidthsIsPhotographedAtEach(t *testing.T) {
	t.Run("two widths, in the line's order", func(t *testing.T) {
		resize, shot := thePhotographTools(aLivePicture, aSameLivePicture)
		built := aTaskWhoseDoneLineIsChecked(t, "The sky is screenshotted at 1440 and 390 wide with the horizon level.", nil, resize, shot)

		outcome := built.ask(t, "prove the sky")

		theLineIsProvedByACheck(t, built, outcome, "photograph")
		if len(resize.Inputs()) != 2 || len(shot.Inputs()) != 2 {
			t.Fatalf("the page was resized %d times and photographed %d, want twice each", len(resize.Inputs()), len(shot.Inputs()))
		}
		for at, width := range []string{`"width":1440`, `"width":390`} {
			if !strings.Contains(string(resize.Inputs()[at]), width) {
				t.Errorf("resize %d was asked for %s, want %s", at+1, resize.Inputs()[at], width)
			}
		}
	})
	t.Run("three widths are two", func(t *testing.T) {
		resize, shot := thePhotographTools(aLivePicture, aSameLivePicture, aLivePicture)
		built := aTaskWhoseDoneLineIsChecked(t, "The sky is photographed at 1440, at 768 and at 390.", nil, resize, shot)

		outcome := built.ask(t, "prove the sky")

		theLineIsProvedByACheck(t, built, outcome, "photograph")
		if len(resize.Inputs()) != 2 || len(shot.Inputs()) != 2 {
			t.Errorf("the page was resized %d times and photographed %d, want twice each: at most two widths", len(resize.Inputs()), len(shot.Inputs()))
		}
	})
}

// TestAPhotographOfAPageThatDrawsNoFramesProvesNothing: a picture of a page
// that drew no frames in a quarter of a second proves nothing, so the line is
// not pinned and the model is sent back with the refusal saying so.
func TestAPhotographOfAPageThatDrawsNoFramesProvesNothing(t *testing.T) {
	resize, shot := thePhotographTools(aStillPicture)
	built := aTaskWhoseDoneLineIsChecked(t, "The sky and the clouds are seen and photographed.",
		[]testkit.Step{answerStep("The page has stopped drawing. What should I do?")}, resize, shot)

	outcome := built.ask(t, "prove the sky")

	if outcome.Status != contract.StatusWaiting {
		t.Fatalf("the task ended %q, want waiting on the model's question after the refusal", outcome.Status)
	}
	if !strings.Contains(requestsJoined(built.model.Requests()), theNoFramesRefusal) {
		t.Errorf("the refusal does not say the picture proves nothing; the requests read:\n%s", requestsJoined(built.model.Requests()))
	}
	if line := built.held(t, outcome.TaskID).Goal.DoneWhen[0]; line.Done || line.ResultID != "" {
		t.Errorf("the done line reads %+v, and a picture of a page that drew no frames pins nothing", line)
	}
}

// TestAPhotographedLineWithNoBrowserIsLeftToTheModel: with no browser tool
// wired, the line is the model's to prove, as it was.
func TestAPhotographedLineWithNoBrowserIsLeftToTheModel(t *testing.T) {
	built := newHarness(t, closingScript("the sky is seen and photographed"), scriptedTool("read", "the notes"))

	outcome := built.ask(t, "prove the sky")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done on the model's own proof: %s", outcome.Status, outcome.Report)
	}
	held := built.held(t, outcome.TaskID)
	if line := held.Goal.DoneWhen[0]; !line.Done || line.ResultID != contract.ResultID(1) {
		t.Errorf("the done line reads %+v, want it pinned to the model's own r1", line)
	}
	for _, result := range held.Work.Results {
		if strings.Contains(result.Summary, "photograph") {
			t.Errorf("the record holds a photograph result %q, and there is no browser to take one with", result.Summary)
		}
	}
}

// TestAPhotographOfAStillVisiblePageProvesTheLine: tic-tac-toe is a still
// page with no animation loop, so its picture says it drew no frames and is
// visible and answers, and that proves the line as a live page's picture does.
func TestAPhotographOfAStillVisiblePageProvesTheLine(t *testing.T) {
	resize, shot := thePhotographTools(aStillVisiblePicture)
	built := aTaskWhoseDoneLineIsChecked(t, "The board is seen and photographed.", nil, resize, shot)

	outcome := built.ask(t, "prove the board")

	if outcome.Status != contract.StatusDone {
		t.Fatalf("the task ended %q, want done on the harness's own picture of a still visible page", outcome.Status)
	}
	if line := built.held(t, outcome.TaskID).Goal.DoneWhen[0]; !line.Done || line.ResultID == "" {
		t.Errorf("the done line reads %+v, want it pinned to the picture", line)
	}
}
