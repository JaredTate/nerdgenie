package browser

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The three functions the browser tools are wired with carry the rules with
// them, so that the tool path is as safe as the Login method's own path.
func TestTheFunctionsTheToolsAreWiredWithCarryTheRules(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", theFixtureLogin)
	browser := world.browser(t, nil)
	world.worker.AddPage(loginPageWithACodeBox())
	if _, err := browser.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	credential, err := browser.Credentials(context.Background(), "fixture-account")
	if err != nil || credential.Password != theFixtureLogin.Password {
		t.Fatalf("the login for the fixture site came back with %v, and it should have been the vault entry", err)
	}
	code, err := browser.TwoFactorCode(context.Background(), "fixture-account")
	if err != nil || code != world.codes.code {
		t.Fatalf("the two-factor code came back as %q and %v, and it should have been a fresh one", code, err)
	}
	if _, err := browser.Credentials(context.Background(), "nobody"); err == nil {
		t.Fatal("a login the vault does not hold should have been refused")
	}
}

// A credential is never handed to the tool for a page its own vault entry does
// not name, whichever way the login is asked for.
func TestACredentialIsNeverHandedOverForTheWrongSite(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", contract.Credential{
		Site: "fixture-account", Domains: []string{"bank.example"},
		Username: "jared@example.com", Password: "a-password-nobody-should-see",
	})
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	_, err := browser.Credentials(context.Background(), "fixture-account")
	if err == nil || !strings.Contains(err.Error(), "fixture.test") {
		t.Fatalf("the credential was handed over with %v, and a page the entry does not name must be refused by name", err)
	}
}

// The handoff function gives back one line saying what the user did, and never
// the code they sent.
func TestTheHandoffFunctionGivesBackOneLine(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)
	pushMessage(t, world, "done")

	said, err := browser.AskUser(context.Background(), "the site is asking whether I am a person")
	if err != nil {
		t.Fatalf("handing the browser to the user failed: %v", err)
	}
	if !strings.Contains(said, "finished at the browser") {
		t.Fatalf("the user's answer came back as %q, and it should say what they did", said)
	}

	world.channel.Shutdown()
	if _, err := browser.AskUser(context.Background(), "another wall"); err == nil {
		t.Fatal("a handoff with nobody left to answer should have said so rather than waiting for ever")
	}
}

// Filling a login form by any path throws the answer away when a value survived
// it, because the model's context is the one place a credential must never
// reach.
func TestFillingALoginByAnyPathThrowsALeakingAnswerAway(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(scriptedStart(healthyThen(func(line []byte) string {
			return `{"jsonrpc":"2.0","id":` + identifierIn(line) +
				`,"result":{"url":"https://a.test/","seen":"the box holds a-password-nobody-should-see",` +
				`"expectationMet":true,"settled":true,"snapshot":{"url":"https://a.test/","title":"A page","tabId":"t1"}}}` + "\n"
		})))
	})

	_, err := browser.LoginFill(context.Background(), contract.LoginFields{
		UsernameRef: "e1", PasswordRef: "e2",
		Username: "jared@example.com", Password: "a-password-nobody-should-see",
	})
	if err == nil || !strings.Contains(err.Error(), "thrown away") {
		t.Fatalf("an answer that still held the password said %v, and it should have been thrown away", err)
	}
}
