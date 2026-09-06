package browsershot_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/browsershot"
)

// newTool builds the screenshot tool over the fake browser worker, already on
// the simple fixture page, saving into a folder of the test's own.
func newTool(t *testing.T) (*browsershot.Tool, string) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	folder := t.TempDir()
	return browsershot.New(browsershot.Settings{Browser: worker, SavesTo: folder}), folder
}

func run(t *testing.T, tool *browsershot.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

// TestAScreenshotOfThePageIsSavedNumberedAndHandedBack is the eyes on the
// browser: the picture is saved as a file, its clickable elements are listed
// by number, and the picture itself rides with the result for a model that
// can see. The thirteenth nightly run's polish task spent thirty rounds
// trying to take this picture through Chrome's debugging port.
func TestAScreenshotOfThePageIsSavedNumberedAndHandedBack(t *testing.T) {
	tool, folder := newTool(t)

	output, err := run(t, tool, map[string]any{"intent": "see the page"})
	if err != nil {
		t.Fatalf("taking the screenshot failed: %v", err)
	}
	path := filepath.Join(folder, "screenshot-1.png")
	if !strings.Contains(output.Text, browsershot.ThePictureIsSavedAt+path) {
		t.Errorf("the answer does not name the file %s:\n%s", path, output.Text)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(saved, []byte("\x89PNG")) {
		t.Errorf("the file %s is not the picture (%v)", path, err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(output.Picture); err != nil || !bytes.Equal(decoded, saved) {
		t.Errorf("the picture with the result is not the file that was saved (%v)", err)
	}
	if !strings.Contains(output.Text, "1 ") || !strings.Contains(output.Text, "e1") {
		t.Errorf("the answer does not number the page's elements:\n%s", output.Text)
	}
}

// TestTheDescriptionFitsInTheCap keeps the tool's words inside the budget every
// tool description shares.
func TestTheDescriptionFitsInTheCap(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()
	if spec.Name != contract.ToolBrowserScreenshot {
		t.Errorf("the tool calls itself %q", spec.Name)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
}

// TestTheRefusalsSayWhatToDo holds the three ways a call is refused: no
// intent, arguments that are not JSON, and no browser wired in.
func TestTheRefusalsSayWhatToDo(t *testing.T) {
	tool, _ := newTool(t)
	if _, err := run(t, tool, map[string]any{"intent": "  "}); err == nil || !strings.Contains(err.Error(), "names no intent") {
		t.Errorf("a call with no intent gave %v, want a refusal asking for one", err)
	}
	if _, err := tool.Run(context.Background(), json.RawMessage(`{"intent":`)); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Errorf("unreadable arguments gave %v, want a refusal saying so", err)
	}
	unwired := browsershot.New(browsershot.Settings{})
	if _, err := run(t, unwired, map[string]any{"intent": "see the page"}); err == nil || !strings.Contains(err.Error(), "no browser is wired") {
		t.Errorf("a tool with no browser gave %v, want a refusal saying so", err)
	}
}

// TestAScreenshotWithNowhereToSaveStillHandsThePictureBack keeps the picture
// when no folder was given, and says the page has nothing to click when it
// has not.
func TestAScreenshotWithNowhereToSaveStillHandsThePictureBack(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	tool := browsershot.New(browsershot.Settings{Browser: worker})

	output, err := run(t, tool, map[string]any{"intent": "see the page"})
	if err != nil {
		t.Fatalf("taking the screenshot failed: %v", err)
	}
	if output.Picture == "" || strings.Contains(output.Text, browsershot.ThePictureIsSavedAt) {
		t.Errorf("with nowhere to save, the answer reads:\n%s\nand carries a picture: %v", output.Text, output.Picture != "")
	}
}

// TestAScreenshotThatCannotBeTakenSaysSo passes the worker's own reason on.
func TestAScreenshotThatCannotBeTakenSaysSo(t *testing.T) {
	tool := browsershot.New(browsershot.Settings{Browser: testkit.NewFakeBrowserWorker()})
	if _, err := run(t, tool, map[string]any{"intent": "see the page"}); err == nil || !strings.Contains(err.Error(), "cannot take a picture") {
		t.Errorf("a screenshot with no page open gave %v, want a refusal saying the picture cannot be taken", err)
	}
}

// TestAFolderThatCannotBeMadeCostsTheFileAndNotThePicture keeps the picture
// when the screenshots folder cannot be made, which is what a folder path
// that names a file does.
func TestAFolderThatCannotBeMadeCostsTheFileAndNotThePicture(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	file := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("cannot make the file: %v", err)
	}
	tool := browsershot.New(browsershot.Settings{Browser: worker, SavesTo: file})

	output, err := run(t, tool, map[string]any{"intent": "see the page"})
	if err != nil {
		t.Fatalf("taking the screenshot failed: %v", err)
	}
	if output.Picture == "" || strings.Contains(output.Text, browsershot.ThePictureIsSavedAt) {
		t.Errorf("with a folder that cannot be made, the answer reads:\n%s\nand carries a picture: %v", output.Text, output.Picture != "")
	}
}
