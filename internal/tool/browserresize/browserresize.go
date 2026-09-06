package browserresize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
)

// The sizes a page may be set to, in pixels: from a small phone to a 4K
// screen. The worker holds the same bounds.
const (
	LeastWidth  = 320
	MostWidth   = 3840
	LeastHeight = 240
	MostHeight  = 2160
)

// Settings is what the tool needs: the browser.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// Tool is the browser_resize tool.
type Tool struct {
	settings Settings
}

// New builds the tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserResize,
		Description: "Sets the page's size in pixels and reads it again; use it to check a page at a phone's, " +
			"a tablet's or a wide screen's size. It does not move the window.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this size is for, in one line.", Required: true},
			{Name: "width", Type: "number", Description: "The page's width in pixels, 320 to 3840.", Required: true},
			{Name: "height", Type: "number", Description: "The page's height in pixels, 240 to 2160.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork},
	}
}

// Run sets the size and hands back the page as it reads at that size.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := struct {
		Intent string  `json:"intent"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}{}
	if err := json.Unmarshal(written, &asked); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the arguments could not be read, so write intent as one line and width and height as numbers: %w", err)
	}
	if strings.TrimSpace(asked.Intent) == "" {
		return contract.ToolOutput{}, errors.New("this call names no intent, so say in one line what this size is for")
	}
	width, err := wholePixels("width", asked.Width, LeastWidth, MostWidth)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	height, err := wholePixels("height", asked.Height, LeastHeight, MostHeight)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("no browser is wired to this tool, so it cannot set the page's size")
	}
	page, err := tool.settings.Browser.Resize(ctx, width, height)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot set the page's size: %w", err)
	}
	return contract.ToolOutput{Text: fmt.Sprintf("the page is now %d by %d pixels\n", width, height) + browserread.PageText(page)}, nil
}

// wholePixels reads one side of the size, refusing anything that is not a
// whole number of pixels within the bounds.
func wholePixels(name string, value float64, least int, most int) (int, error) {
	whole := int(value)
	if float64(whole) != value || whole < least || whole > most {
		return 0, fmt.Errorf("the %s must be a whole number of pixels between %d and %d, and this was %v", name, least, most, value)
	}
	return whole, nil
}
