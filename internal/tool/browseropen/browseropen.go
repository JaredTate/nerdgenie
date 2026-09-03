package browseropen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserread"
	"github.com/JaredTate/coeus/internal/tool/loose"
)

// Settings is what the browser open tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
}

// input is what the model writes when it calls this tool.
type input struct {
	// Intent says what this step is for, in the model's own words.
	Intent string
	// URL is the address to go to.
	URL string
}

// Tool is the browser open tool.
type Tool struct {
	settings Settings
}

// New returns the browser open tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserOpen,
		Description: "Takes the agent's own Chrome to a web address and hands back the page as an outline. " +
			"Use web fetch instead for a page that needs no login and no clicking.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "url", Type: "string", Description: "The whole web address to go to.", Required: true},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassExecute},
	}
}

// Run takes the browser to the address and hands back the page.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked, err := readInput(written)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}

	page, err := tool.settings.Browser.Open(ctx, asked.URL)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot open the page %s: %w", asked.URL, err)
	}
	return contract.ToolOutput{Text: browserread.PageText(page)}, nil
}

// addressNames are the names a model writes for the page to go to.
var addressNames = []string{"url", "address", "link", "page", "web_address", "href"}

// readInput reads the model's arguments and refuses anything this tool could not
// act on.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent and a url")
	if err != nil {
		return input{}, err
	}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	address, wroteAddress := fields.Text(addressNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if !wroteAddress {
		return input{}, fields.Missing("url", "the whole web address to go to, beginning with https://,")
	}
	if err := checkAddress(address); err != nil {
		return input{}, err
	}
	return input{Intent: intent, URL: address}, nil
}

// checkAddress refuses anything that is not an ordinary web address, so that the
// browser is never asked to open a file or a program on this machine.
func checkAddress(address string) error {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return errors.New("this call names no page, so give the whole web address to go to")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("cannot read %q as a web address, so write one beginning with https://: %w", address, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("the address %q is not a web address, so give one beginning with http:// or https://", address)
	}
	if parsed.Host == "" {
		return fmt.Errorf("the address %q names no host, so write the whole web address", address)
	}
	return nil
}
