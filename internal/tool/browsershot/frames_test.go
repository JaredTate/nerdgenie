package browsershot_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browsershot"
)

// aToolOverAPageThatDraws builds the screenshot tool over a fake browser on
// the simple fixture page whose screenshots say the page drew this many
// frames, saving into a folder of the test's own. The fake's picture is the
// same every time, which is what a still scene photographs as.
func aToolOverAPageThatDraws(t *testing.T, frames int) (*browsershot.Tool, string) {
	t.Helper()
	tool, folder, _ := aToolAndItsWorkerOverAPageThatDraws(t, frames)
	return tool, folder
}

// aToolAndItsWorkerOverAPageThatDraws is the same, handing back the fake
// worker too, for a test that hides the page.
func aToolAndItsWorkerOverAPageThatDraws(t *testing.T, frames int) (*browsershot.Tool, string, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	worker.DrawsFrames(frames)
	folder := t.TempDir()
	return browsershot.New(browsershot.Settings{Browser: worker, SavesTo: folder}), folder, worker
}

// TestASamePictureOfAPageThatDrawsSaysNothingMoved: on the night of 7
// September 2026 the sky task read "the picture is the same as the last one"
// four times and took it as proof the camera was broken, when the aircraft
// was parked and the clouds were still. A same picture of a page that drew
// frames says the page is alive and nothing on it moved, and what to do
// instead, in the one sentence brief 9.2 and brief 9.3 read word for word.
func TestASamePictureOfAPageThatDrawsSaysNothingMoved(t *testing.T) {
	tool, folder := aToolOverAPageThatDraws(t, 15)
	if _, err := run(t, tool, map[string]any{"intent": "see the sky"}); err != nil {
		t.Fatalf("the first picture failed: %v", err)
	}

	again, err := run(t, tool, map[string]any{"intent": "see the sky again"})
	if err != nil {
		t.Fatalf("the second picture failed: %v", err)
	}
	want := "the picture is the same as the last one: the page drew 15 frames in a quarter of a second, " +
		"so it is alive and nothing on it moved; change the view, move the camera or the aircraft, " +
		"or act on the page to see something new (the picture is saved at " + filepath.Join(folder, "screenshot-1.png") + ")\n"
	if again.Text != want {
		t.Errorf("the same picture of a drawing page reads:\n%q\nwant:\n%q", again.Text, want)
	}
	if again.Picture != "" {
		t.Errorf("the same picture was handed back again, %d bytes of it", len(again.Picture))
	}
}

// TestASamePictureOfAStillVisiblePageSaysNothingChanged: tic-tac-toe on 8
// September 2026 is a still page with no animation loop, so it never draws a
// frame on its own, and the model read "the page drew no frames, so its loop
// has stopped or the tab is hidden" as a hidden tab and handed the browser to
// the person. A same picture of a page that drew nothing but is visible and
// answers is a still page on which nothing changed.
func TestASamePictureOfAStillVisiblePageSaysNothingChanged(t *testing.T) {
	tool, folder := aToolOverAPageThatDraws(t, 0)
	if _, err := run(t, tool, map[string]any{"intent": "see the board"}); err != nil {
		t.Fatalf("the first picture failed: %v", err)
	}

	again, err := run(t, tool, map[string]any{"intent": "see the board again"})
	if err != nil {
		t.Fatalf("the second picture failed: %v", err)
	}
	want := "the picture is the same as the last one: the page drew no frames in a quarter of a second, " +
		"which is what a still page does; it is visible and answers, so nothing on it changed; " +
		"act on the page or change the view to see something new " +
		"(the picture is saved at " + filepath.Join(folder, "screenshot-1.png") + ")\n"
	if again.Text != want {
		t.Errorf("the same picture of a still visible page reads:\n%q\nwant:\n%q", again.Text, want)
	}
	if again.Picture != "" {
		t.Errorf("the same picture was handed back again, %d bytes of it", len(again.Picture))
	}
}

// TestASamePictureOfAHiddenPageSaysTheTabIsHidden: the reading that was
// right for the sky task's stopped loop is kept for a page that drew nothing
// and is not visible.
func TestASamePictureOfAHiddenPageSaysTheTabIsHidden(t *testing.T) {
	tool, folder, worker := aToolAndItsWorkerOverAPageThatDraws(t, 0)
	worker.HidesThePage()
	if _, err := run(t, tool, map[string]any{"intent": "see the sky"}); err != nil {
		t.Fatalf("the first picture failed: %v", err)
	}

	again, err := run(t, tool, map[string]any{"intent": "see the sky again"})
	if err != nil {
		t.Fatalf("the second picture failed: %v", err)
	}
	want := "the picture is the same as the last one: the page drew no frames in a quarter of a second " +
		"and the tab is hidden; open the page again or read its console " +
		"(the picture is saved at " + filepath.Join(folder, "screenshot-1.png") + ")\n"
	if again.Text != want {
		t.Errorf("the same picture of a hidden page reads:\n%q\nwant:\n%q", again.Text, want)
	}
}

// TestANewPictureOfAStillVisiblePageSaysSo: the new picture's frames line
// says the page is visible and answers when it drew nothing, which is what
// the done check reads to prove a photographed line on a still page.
func TestANewPictureOfAStillVisiblePageSaysSo(t *testing.T) {
	tool, folder := aToolOverAPageThatDraws(t, 0)

	output, err := run(t, tool, map[string]any{"intent": "see the board"})
	if err != nil {
		t.Fatalf("taking the picture failed: %v", err)
	}
	want := browsershot.ThePictureIsSavedAt + filepath.Join(folder, "screenshot-1.png") + "\n" +
		"the page drew no frames in a quarter of a second; it is visible and answers\n"
	if !strings.HasPrefix(output.Text, want) {
		t.Errorf("a new picture of a still visible page begins:\n%q\nwant:\n%q", output.Text, want)
	}
}

// TestANewPictureSaysHowManyFramesThePageDrew: a new picture's text carries
// one line after the saved-at line saying how many frames the page drew,
// which is the line the done check reads to prove a photographed done line.
func TestANewPictureSaysHowManyFramesThePageDrew(t *testing.T) {
	tool, folder := aToolOverAPageThatDraws(t, 15)

	output, err := run(t, tool, map[string]any{"intent": "see the sky"})
	if err != nil {
		t.Fatalf("taking the picture failed: %v", err)
	}
	want := browsershot.ThePictureIsSavedAt + filepath.Join(folder, "screenshot-1.png") + "\n" +
		"the page drew 15 frames in a quarter of a second\n"
	if !strings.HasPrefix(output.Text, want) {
		t.Errorf("a new picture's text begins:\n%q\nwant:\n%q", output.Text, want)
	}
	if output.Picture == "" {
		t.Errorf("the new picture was not handed back")
	}
}
