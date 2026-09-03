package browserlogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/tool/browserread"
	"github.com/JaredTate/coeus/internal/tool/loose"
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
	Intent string
	// Site is the name the user knows the login by, such as "x-account".
	Site string
	// UsernameElement is the reference of the box the login name goes in.
	UsernameElement string
	// PasswordElement is the reference of the box the secret goes in.
	PasswordElement string
	// CodeElement is the reference of the box the second code goes in, and is
	// empty when the page asked for no second code.
	CodeElement string
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
	asked, err := readInput(written)
	if err != nil {
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

// The names a model writes for the fields of one login call.
var (
	siteNames     = []string{"site", "account", "login", "saved_as"}
	usernameNames = []string{"username_element", "username_ref", "user_element", "login_element", "name_element"}
	passwordNames = []string{"password_element", "password_ref", "secret_element"}
	codeNames     = []string{"code_element", "code_ref", "second_code_element", "otp_element"}
)

// readInput reads the model's arguments and refuses anything this tool could not
// act on. There is no field here for a username or a password, and there never
// will be.
func readInput(written json.RawMessage) (input, error) {
	fields, err := loose.Read(written, "an intent, a site, and the two boxes")
	if err != nil {
		return input{}, err
	}
	asked := input{}
	intent, wroteIntent := fields.Text(browserread.IntentNames...)
	asked.Intent = intent
	asked.Site, _ = fields.Text(siteNames...)
	username, wroteUsername := fields.Text(usernameNames...)
	password, wrotePassword := fields.Text(passwordNames...)
	asked.UsernameElement, asked.PasswordElement = username, password
	asked.CodeElement, _ = fields.Text(codeNames...)
	if err := fields.Wrong(); err != nil {
		return input{}, err
	}
	if err := browserread.NeedIntent(fields, intent, wroteIntent); err != nil {
		return input{}, err
	}
	if strings.TrimSpace(asked.Site) == "" {
		return input{}, fields.Missing("site", "the name the user saved the login under,")
	}
	if !wroteUsername || strings.TrimSpace(username) == "" {
		return input{}, fields.Missing("username_element", "the reference of the box for the login name,")
	}
	if !wrotePassword || strings.TrimSpace(password) == "" {
		return input{}, fields.Missing("password_element", "the reference of the box the site keeps hidden,")
	}
	return asked, nil
}
