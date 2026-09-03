package browserlogin_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/browserlogin"
)

// theCredential is the login the harness hands the tool in these tests. Nothing
// in it may ever come back out of the tool.
var theCredential = contract.Credential{
	Site: "the-board", Domains: []string{"fixture.test"},
	Username: "jared", Password: "a-very-secret-password", TOTPSecret: "SECRETSEED",
}

// newTool builds the login tool over the fake browser worker, on the login page.
func newTool(t *testing.T) (*browserlogin.Tool, *testkit.FakeBrowserWorker) {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("cannot open the login page: %v", err)
	}
	tool := browserlogin.New(browserlogin.Settings{
		Browser: worker,
		Credentials: func(_ context.Context, site string) (contract.Credential, error) {
			if site != theCredential.Site {
				return contract.Credential{}, errors.New("the vault holds no login for that site, so add one first")
			}
			return theCredential, nil
		},
		TwoFactorCode: func(context.Context, string) (string, error) { return "123456", nil },
	})
	return tool, worker
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *browserlogin.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolBrowserLogin {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolBrowserLogin)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "intent,site,username_element,password_element,code_element" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a login by site", names)
	}
	for _, field := range spec.Fields {
		if strings.Contains(strings.ToLower(field.Description), "password") && field.Name != "password_element" {
			t.Errorf("the field %s talks about a password, and the model never writes one", field.Name)
		}
	}
}

func TestTheLoginIsTypedAndNothingOfItComesBack(t *testing.T) {
	tool, worker := newTool(t)

	output, err := run(t, tool, map[string]any{
		"intent": "sign in to the board", "site": "the-board",
		"username_element": testkit.FixtureUsernameRef, "password_element": testkit.FixturePasswordRef,
	})
	if err != nil {
		t.Fatalf("filling the login form failed: %v", err)
	}
	for _, secret := range []string{theCredential.Username, theCredential.Password, theCredential.TOTPSecret, "123456"} {
		if strings.Contains(output.Text, secret) {
			t.Errorf("the result carries %q, and nothing of a login may ever come back to the model: %q", secret, output.Text)
		}
	}
	if typed := worker.TypedInto(testkit.FixturePasswordRef); len(typed) != 1 || typed[0] != theCredential.Password {
		t.Errorf("the password box holds %v, want the password the harness supplied", typed)
	}
	testkit.Golden(t, "a_login.txt", []byte(output.Text))
}

func TestACodeIsAskedForOnlyWhenThePageHasABoxForIt(t *testing.T) {
	asked := 0
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("cannot open the login page: %v", err)
	}
	tool := browserlogin.New(browserlogin.Settings{
		Browser:     worker,
		Credentials: func(context.Context, string) (contract.Credential, error) { return theCredential, nil },
		TwoFactorCode: func(context.Context, string) (string, error) {
			asked++
			return "123456", nil
		},
	})

	if _, err := run(t, tool, map[string]any{
		"intent": "sign in", "site": "the-board",
		"username_element": testkit.FixtureUsernameRef, "password_element": testkit.FixturePasswordRef,
	}); err != nil {
		t.Fatalf("filling the login form failed: %v", err)
	}
	if asked != 0 {
		t.Errorf("a two-factor code was asked for %d times when the page had no box for one", asked)
	}

	if _, err := run(t, tool, map[string]any{
		"intent": "sign in with a code", "site": "the-board",
		"username_element": testkit.FixtureUsernameRef, "password_element": testkit.FixturePasswordRef,
		"code_element": "e4",
	}); err != nil {
		t.Fatalf("filling the login form with a code failed: %v", err)
	}
	if asked != 1 {
		t.Errorf("a two-factor code was asked for %d times when the page had a box for one", asked)
	}
}

func TestASiteTheVaultHasNoLoginForIsRefused(t *testing.T) {
	tool, worker := newTool(t)

	_, err := run(t, tool, map[string]any{
		"intent": "sign in", "site": "a-site-nobody-added",
		"username_element": testkit.FixtureUsernameRef, "password_element": testkit.FixturePasswordRef,
	})
	if err == nil {
		t.Fatalf("a login was filled for a site the vault has nothing for")
	}
	if len(worker.TypedInto(testkit.FixturePasswordRef)) != 0 {
		t.Errorf("something was typed into the password box for a site with no login")
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"site": "the-board", "username_element": "e2", "password_element": "e3"},
		{"intent": "sign in", "username_element": "e2", "password_element": "e3"},
		{"intent": "sign in", "site": "the-board", "password_element": "e3"},
		{"intent": "sign in", "site": "the-board", "username_element": "e2"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoBrowserOrNoVaultSaysSo(t *testing.T) {
	fields := map[string]any{
		"intent": "sign in", "site": "the-board", "username_element": "e2", "password_element": "e3",
	}
	if _, err := run(t, browserlogin.New(browserlogin.Settings{}), fields); err == nil {
		t.Errorf("a login was filled with nothing behind the tool")
	}
	noVault := browserlogin.New(browserlogin.Settings{Browser: testkit.NewFakeBrowserWorker()})
	_, err := run(t, noVault, fields)
	if err == nil {
		t.Fatalf("a login was filled with no vault behind the tool")
	}
	if !strings.Contains(err.Error(), "vault") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
