package browsershot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// ThePictureIsSavedAt opens the line naming the file the picture was saved as.
const ThePictureIsSavedAt = "the picture is saved at "

// Settings is what the tool needs: the browser, and the folder screenshots
// are saved under, which may be empty.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
	// SavesTo is the folder screenshots are saved under, or empty.
	SavesTo string
}

// Tool is the browser_screenshot tool.
type Tool struct {
	settings Settings
	saved    int
}

// New builds the tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserScreenshot,
		Description: "Takes a picture of the page the browser is on, with its clickable elements numbered, " +
			"and shows it to you; use it to check what a page or a canvas really looks like.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this look is for, in one line.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork},
	}
}

// Run takes the picture, saves it, and hands it back with its numbered marks.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := struct {
		Intent string `json:"intent"`
	}{}
	if err := json.Unmarshal(written, &asked); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the arguments could not be read, so write intent as one line: %w", err)
	}
	if strings.TrimSpace(asked.Intent) == "" {
		return contract.ToolOutput{}, errors.New("this call names no intent, so say in one line what this look is for")
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("no browser is wired to this tool, so it cannot take a picture")
	}
	picture, err := tool.settings.Browser.Screenshot(ctx)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot take a picture of the page: %w", err)
	}
	text := &strings.Builder{}
	if path, err := tool.savePicture(picture.PNGBase64); err == nil && path != "" {
		text.WriteString(ThePictureIsSavedAt + path + "\n")
	}
	if len(picture.Marks) == 0 {
		text.WriteString("nothing on the page is numbered, because nothing on it can be clicked\n")
	} else {
		text.WriteString("numbered on the picture:\n")
		for _, mark := range picture.Marks {
			fmt.Fprintf(text, "%d %s %q (%s)\n", mark.Number, mark.Role, mark.Name, mark.Ref)
		}
	}
	return contract.ToolOutput{Text: text.String(), Picture: picture.PNGBase64}, nil
}

// savePicture writes the picture as the next numbered file under the folder
// the tool saves to, and hands back its path, or nothing when there is no
// folder or no picture.
func (tool *Tool) savePicture(pngBase64 string) (string, error) {
	if tool.settings.SavesTo == "" || pngBase64 == "" {
		return "", nil
	}
	bytes, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return "", fmt.Errorf("the picture is not base64, so it cannot be saved: %w", err)
	}
	if err := os.MkdirAll(tool.settings.SavesTo, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("cannot make the folder for screenshots, so the picture is not saved: %w", err)
	}
	tool.saved++
	path := filepath.Join(tool.settings.SavesTo, fmt.Sprintf("screenshot-%d.png", tool.saved))
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		return "", fmt.Errorf("cannot write the screenshot, so the picture is not saved: %w", err)
	}
	return path, nil
}
