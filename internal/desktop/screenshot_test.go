package desktop

import (
	"context"
	"testing"
)

// A picture of the screen needs no launched application. The first human trial
// asked the computer tool for a screenshot to read a page the browser had
// opened, and was told to launch an application first; the rule that an
// application must be launched belongs to clicking and typing into one, not to
// looking. These tests hold the Go side to that, over the scripted worker.

func TestAScreenshotAloneNeedsNoLaunchAndAsksNobodyAnything(t *testing.T) {
	desk := newDesk(t)

	picture, err := desk.desktop.Screenshot(context.Background())

	if err != nil {
		t.Fatalf("taking a screenshot with no application open failed: %v, and looking at the screen needs no application", err)
	}
	if len(picture.Windows) == 0 {
		t.Error("the screenshot names no window, and with nothing open the names are what the model reads")
	}
	if shown := len(desk.channel.Previews()); shown != 0 {
		t.Errorf("the user was shown %d previews for a screenshot alone, want none: the grant question belongs to launching an application", shown)
	}
	if asked := len(desk.permission.Requests()); asked != 0 {
		t.Errorf("the permission function was asked about %d calls for a screenshot alone, want none", asked)
	}
	if asked := desk.latestWorker(t).methodsAsked(); len(asked) != 2 || asked[0] != "health" || asked[1] != "screenshot" {
		t.Errorf("the worker was asked %v, want the health check and then the screenshot, with no launch in between", asked)
	}
}

func TestAScreenshotSaysWhichWindowsAreOnTheScreenAndWhichApplicationItIsOf(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)

	picture, err := desk.desktop.Screenshot(context.Background())

	if err != nil {
		t.Fatalf("taking a screenshot failed: %v", err)
	}
	if len(picture.Windows) != 2 || picture.Windows[0] != "Coeus fixture window" || picture.Windows[1] != "Firefox" {
		t.Errorf("the windows are %q, want the two titles the worker sent, so that the model knows what it is looking at", picture.Windows)
	}
	if picture.Application != "zenity" {
		t.Errorf("the picture is of %q, want the application the worker said it photographed", picture.Application)
	}
}
