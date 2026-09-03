package browserlogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserclick"
	"github.com/JaredTate/coeus/internal/tool/browserread"
)

// Credentials is where the harness gets the login to type for one site. Wave
// five wires the vault behind it. What it returns goes straight to the browser
// worker and nowhere else.
type Credentials func(ctx context.Context, site string) (contract.Credential, error)

// TwoFactorCode is where the harness gets the second code for one site, and it
// is asked only when the page has a box for one.
type TwoFactorCode func(ctx context.Context, site string) (string, error)

// Settings is what the login tool needs to do its work.
type Settings struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
	// Credentials is where the login to type comes from.
	Credentials Credentials
	// TwoFactorCode is where the second code comes from, when the page wants one.
	TwoFactorCode TwoFactorCode
}

// input is what the model writes when it calls this tool. There is no field here
// for a username or a password, and there never will be.
type input struct {
	// Intent says what this step is for.
	Intent string `json:"intent"`
	// Site is the name the user knows the login by, such as "x-account".
	Site string `json:"site"`
	// UsernameElement is the reference of the box the login name goes in.
	UsernameElement string `json:"username_element"`
	// PasswordElement is the reference of the box the secret goes in.
	PasswordElement string `json:"password_element"`
	// CodeElement is the reference of the box the second code goes in, and is
	// empty when the page asked for no second code.
	CodeElement string `json:"code_element"`
}

// Tool is the browser login tool.
type Tool struct {
	settings Settings
}

// New returns the browser login tool.
func New(settings Settings) *Tool {
	return &Tool{settings: settings}
}

// Spec is what the model is told about this tool.
func (tool *Tool) Spec() contract.ToolSpec {
	return contract.ToolSpec{
		Name: contract.ToolBrowserLogin,
		Description: "Signs in to a site the user has saved, by pointing at the boxes on the page. " +
			"You never see or write what is typed. Use it whenever a page asks who you are.",
		Fields: []contract.ToolField{
			{Name: "intent", Type: "string", Description: "What this step is for, in one line.", Required: true},
			{Name: "site", Type: "string", Description: "The name the user saved the login under.", Required: true},
			{Name: "username_element", Type: "string", Description: "The reference of the box for the login name.", Required: true},
			{Name: "password_element", Type: "string", Description: "The reference of the box the site keeps hidden.", Required: true},
			{Name: "code_element", Type: "string", Description: "The reference of the box for a second code, when the page asks for one."},
		},
		Classes: []contract.PermissionClass{contract.ClassNetwork, contract.ClassIrreversible},
	}
}

// Run fills the login form and hands back what changed on the page, which never
// carries anything that was typed.
func (tool *Tool) Run(ctx context.Context, written json.RawMessage) (contract.ToolOutput, error) {
	asked := input{}
	if len(written) > 0 {
		if err := json.Unmarshal(written, &asked); err != nil {
			return contract.ToolOutput{}, fmt.Errorf("cannot read this call's arguments as JSON, so write an object with an intent, a site, and the two boxes in it: %w", err)
		}
	}
	if err := checkCall(asked); err != nil {
		return contract.ToolOutput{}, err
	}
	if tool.settings.Browser == nil {
		return contract.ToolOutput{}, errors.New("this tool has no browser behind it, so wire the browser worker in before using it")
	}
	if tool.settings.Credentials == nil {
		return contract.ToolOutput{}, errors.New("this tool has no vault behind it, so wire the credentials in before signing in to anything")
	}

	fields, err := tool.fieldsFor(ctx, asked)
	if err != nil {
		return contract.ToolOutput{}, err
	}
	change, err := tool.settings.Browser.LoginFill(ctx, fields)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("cannot sign in to %s: %w", asked.Site, err)
	}
	return contract.ToolOutput{Text: "signed in to " + asked.Site + "\n" + browserread.ChangeText(change)}, nil
}

// fieldsFor gathers what the worker is to type. Nothing it returns is written to
// a log, an error, or a result: it goes to the worker and no further.
func (tool *Tool) fieldsFor(ctx context.Context, asked input) (contract.LoginFields, error) {
	credential, err := tool.settings.Credentials(ctx, asked.Site)
	if err != nil {
		return contract.LoginFields{}, fmt.Errorf("cannot find the login for %s: %w", asked.Site, err)
	}
	fields := contract.LoginFields{
		UsernameRef: asked.UsernameElement,
		PasswordRef: asked.PasswordElement,
		CodeRef:     asked.CodeElement,
		Username:    credential.Username,
		Password:    credential.Password,
	}
	if asked.CodeElement == "" {
		return fields, nil
	}
	if tool.settings.TwoFactorCode == nil {
		return contract.LoginFields{}, fmt.Errorf("the page asks %s for a second code and this tool has no way to get one, so add one to the vault", asked.Site)
	}
	code, err := tool.settings.TwoFactorCode(ctx, asked.Site)
	if err != nil {
		return contract.LoginFields{}, fmt.Errorf("cannot get the second code for %s: %w", asked.Site, err)
	}
	fields.Code = code
	return fields, nil
}

// checkCall holds the rules one call must satisfy before anything is typed.
func checkCall(asked input) error {
	if err := browserread.CheckIntent(asked.Intent); err != nil {
		return err
	}
	if strings.TrimSpace(asked.Site) == "" {
		return errors.New("this call names no site, so give the name the user saved the login under")
	}
	if err := browserclick.CheckElement(asked.UsernameElement); err != nil {
		return fmt.Errorf("the box for the login name is missing: %w", err)
	}
	if err := browserclick.CheckElement(asked.PasswordElement); err != nil {
		return fmt.Errorf("the box the site keeps hidden is missing: %w", err)
	}
	return nil
}
