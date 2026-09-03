package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeDesktopRefusesAnApplicationNobodyGranted(t *testing.T) {
	desktop := testkit.NewFakeDesktop()

	err := desktop.Launch(context.Background(), "gimp", "what the test expects happens")

	if err == nil {
		t.Fatal("an application nobody granted was launched, and the desktop must refuse until the user grants it")
	}
	if !strings.Contains(err.Error(), "gimp") {
		t.Errorf("the error is %q, want it to name the application", err)
	}
}

func TestTheFakeDesktopLaunchesAnApplicationTheUserGranted(t *testing.T) {
	ctx := context.Background()
	desktop := testkit.NewFakeDesktop()
	desktop.Grant("text-editor")

	if err := desktop.Launch(ctx, "text-editor", "what the test expects happens"); err != nil {
		t.Fatalf("launching a granted application failed: %v", err)
	}
	if actions := desktop.Actions(); len(actions) != 1 || !strings.Contains(actions[0], "text-editor") {
		t.Errorf("the desktop recorded %v, want one launch of the text editor", actions)
	}
}

func TestTheFakeDesktopScreenshotHasNumberedMarksOnIt(t *testing.T) {
	ctx := context.Background()
	desktop := testkit.NewFakeDesktop()
	desktop.Grant("text-editor")
	if err := desktop.Launch(ctx, "text-editor", "what the test expects happens"); err != nil {
		t.Fatalf("launching failed: %v", err)
	}

	picture, err := desktop.Screenshot(ctx)
	if err != nil {
		t.Fatalf("taking a screenshot failed: %v", err)
	}
	if picture.PNGBase64 == "" {
		t.Error("the screenshot has no picture in it")
	}
	if len(picture.Marks) == 0 {
		t.Fatal("the screenshot has no numbered marks on it, and the model clicks by number")
	}
	if picture.Marks[0].Number != 1 {
		t.Errorf("the first mark is numbered %d, want 1", picture.Marks[0].Number)
	}
}

func TestTheFakeDesktopRecordsEveryAction(t *testing.T) {
	ctx := context.Background()
	desktop := testkit.NewFakeDesktop()
	desktop.Grant("text-editor")
	if err := desktop.Launch(ctx, "text-editor", "what the test expects happens"); err != nil {
		t.Fatalf("launching failed: %v", err)
	}

	steps := []struct {
		name string
		run  func() error
	}{
		{"click", func() error {
			return desktop.Click(ctx, 1, "what the test expects happens")
		}},
		{"type", func() error {
			return desktop.Type(ctx, "the anniversary post", "what the test expects happens")
		}},
		{"press", func() error {
			return desktop.Press(ctx, "ctrl+s", "what the test expects happens")
		}},
		{"drag", func() error {
			return desktop.Drag(ctx, 1, 2, "what the test expects happens")
		}},
		{"set the clipboard", func() error { return desktop.SetClipboard(ctx, "copied text") }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("the %s action failed: %v", step.name, err)
		}
	}

	copied, err := desktop.Clipboard(ctx)
	if err != nil {
		t.Fatalf("reading the clipboard failed: %v", err)
	}
	if copied != "copied text" {
		t.Errorf("the clipboard holds %q, want what was put on it", copied)
	}
	if len(desktop.Actions()) != len(steps)+1 {
		t.Errorf("the desktop recorded %d actions, want the launch and the five that followed", len(desktop.Actions()))
	}
}

func TestTheFakeDesktopRefusesAMarkThatIsNotOnTheScreen(t *testing.T) {
	ctx := context.Background()
	desktop := testkit.NewFakeDesktop()
	desktop.Grant("text-editor")
	if err := desktop.Launch(ctx, "text-editor", "what the test expects happens"); err != nil {
		t.Fatalf("launching failed: %v", err)
	}

	if err := desktop.Click(ctx, 99, "what the test expects happens"); err == nil {
		t.Fatal("clicking a mark that is not on the screen was reported as a success, want an error naming the number")
	}
}

func TestTheFakeDesktopKeepsTheDesktopContract(t *testing.T) {
	if err := testkit.CheckDesktop(context.Background(), testkit.NewFakeDesktop()); err != nil {
		t.Fatalf("the fake desktop does not keep the desktop contract: %v", err)
	}
}
