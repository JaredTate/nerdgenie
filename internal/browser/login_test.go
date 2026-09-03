package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theFixtureLogin is the vault entry the login tests use. No test ever prints
// these values, and the point of most of these tests is that nothing else does
// either.
var theFixtureLogin = contract.Credential{
	Site:       "fixture-account",
	Domains:    []string{"fixture.test"},
	Username:   "jared@example.com",
	Password:   "a-password-nobody-should-see",
	TOTPSecret: "JBSWY3DPEHPK3PXP",
}

// A login on the fixture login page fills the boxes from the vault with a fresh
// two-factor code, and not one character of the credential comes back.
func TestALoginFillsTheFormFromTheVaultAndReturnsNoValue(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", theFixtureLogin)
	browser := world.browser(t, nil)
	world.worker.AddPage(loginPageWithACodeBox())

	if _, err := browser.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}
	diff, err := browser.Login(context.Background(), "fixture-account", LoginRefs{})
	if err != nil {
		t.Fatalf("the login failed: %v", err)
	}
	if world.codes.timesAsked() != 1 {
		t.Fatalf("%d codes were made, and one fresh one is what a login needs", world.codes.timesAsked())
	}
	checkTheWholeAnswerHoldsNoValue(t, diff)
	if typed := world.worker.TypedInto(testkit.FixturePasswordRef); len(typed) != 1 || typed[0] != theFixtureLogin.Password {
		t.Fatalf("the worker typed %d things into the password box, and it should have been the password once", len(typed))
	}
	if typed := world.worker.TypedInto("e5"); len(typed) != 1 || typed[0] != world.codes.code {
		t.Fatalf("the worker typed %v into the code box, and it should have been the code once", typed)
	}
}

// checkTheWholeAnswerHoldsNoValue searches every byte of the answer, not only
// the fields a reader would think to look at.
func checkTheWholeAnswerHoldsNoValue(t *testing.T, diff contract.Diff) {
	t.Helper()
	written, err := json.Marshal(diff)
	if err != nil {
		t.Fatalf("the answer could not be written back as JSON: %v", err)
	}
	for _, value := range []string{theFixtureLogin.Password, theFixtureLogin.Username, theFixtureLogin.TOTPSecret, "123456"} {
		if strings.Contains(string(written), value) {
			t.Fatalf("the whole answer holds a credential, and no field of it ever may: %s", written)
		}
	}
}

// A login on a page the vault entry does not name is refused, and the refusal
// names the hostname the browser is actually on.
func TestALoginOnTheWrongDomainIsRefusedByHostname(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", contract.Credential{
		Site: "fixture-account", Domains: []string{"bank.example"},
		Username: "jared@example.com", Password: "a-password-nobody-should-see",
	})
	browser := world.browser(t, nil)
	if _, err := browser.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	_, err := browser.Login(context.Background(), "fixture-account", LoginRefs{})
	if err == nil {
		t.Fatal("a login on a page the entry does not name should have been refused")
	}
	for _, wanted := range []string{"fixture.test", "bank.example"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Fatalf("the refusal said %q, and it should have named %q", err, wanted)
		}
	}
	if typed := world.worker.TypedInto(testkit.FixturePasswordRef); len(typed) != 0 {
		t.Fatalf("the worker typed %d things, and a refused login types nothing at all", len(typed))
	}
}

// An entry the vault does not hold, and a page with no login boxes on it, are
// both refused with what to do about it.
func TestALoginWithNothingToFillIsRefusedWithWhatToDo(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	_, err := browser.Login(context.Background(), "nobody", LoginRefs{})
	if err == nil || !strings.Contains(err.Error(), "nobody") {
		t.Fatalf("a login on an entry the vault does not hold said %v, and it should have named the entry", err)
	}

	world.secrets.Add("fixture-account", theFixtureLogin)
	_, err = browser.Login(context.Background(), "fixture-account", LoginRefs{})
	if err == nil || !strings.Contains(err.Error(), "the box the login name goes in") {
		t.Fatalf("a login on a page with no password box said %v, and it should have said what it could not find", err)
	}
}

// The model may point at the boxes itself, and a reference the page does not
// hold is refused rather than typed into.
func TestTheModelMayPointAtTheBoxesItself(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", contract.Credential{
		Site: "fixture-account", Domains: []string{"fixture.test"},
		Username: "jared@example.com", Password: "a-password-nobody-should-see",
	})
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	chosen := LoginRefs{UsernameRef: testkit.FixtureUsernameRef, PasswordRef: testkit.FixturePasswordRef}
	if _, err := browser.Login(context.Background(), "fixture-account", chosen); err != nil {
		t.Fatalf("a login into the boxes the model pointed at failed: %v", err)
	}
	if typed := world.worker.TypedInto(testkit.FixtureUsernameRef); len(typed) != 1 {
		t.Fatalf("the worker typed %v into the box the model pointed at, and it should have been the login name", typed)
	}

	_, err := browser.Login(context.Background(), "fixture-account", LoginRefs{UsernameRef: "e99", PasswordRef: "e98"})
	if err == nil || !strings.Contains(err.Error(), "e99") {
		t.Fatalf("a login into a box that is not on the page said %v, and it should have named the box", err)
	}
}

// A code with only a moment left is not typed. The login waits for the next
// thirty-second window and makes another one, because a code typed at the last
// moment is often refused by the time the form is submitted.
func TestACodeWithAMomentLeftIsThrownAwayForAFreshOne(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", theFixtureLogin)
	world.codes.secondsLeft = 2
	browser := world.browser(t, nil)
	world.worker.AddPage(loginPageWithACodeBox())
	if _, err := browser.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := browser.Login(context.Background(), "fixture-account", LoginRefs{})
		done <- err
	}()
	waitForSleepers(t, world.clock, 2)
	world.clock.Advance(3 * time.Second)

	if err := <-done; err != nil {
		t.Fatalf("the login that waited for a fresh code failed: %v", err)
	}
	if asked := world.codes.timesAsked(); asked != 2 {
		t.Fatalf("%d codes were made, and a code with two seconds left should have been thrown away for a second one", asked)
	}
}

// A vault entry with a two-factor secret and nothing wired to make codes is a
// fault in how Coeus was started, and says so.
func TestATwoFactorSecretWithNoCodeMakerSaysSo(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", theFixtureLogin)
	browser := world.browser(t, func(options *Options) { options.Codes = nil })
	world.worker.AddPage(loginPageWithACodeBox())
	if _, err := browser.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	_, err := browser.Login(context.Background(), "fixture-account", LoginRefs{})
	if err == nil || !strings.Contains(err.Error(), "fault in how Coeus was started") {
		t.Fatalf("a two-factor login with no code maker said %v, and it should have named itself a fault in the wiring", err)
	}
}

// A code maker that will not make a code stops the login rather than filling the
// form without one.
func TestACodeThatCannotBeMadeStopsTheLogin(t *testing.T) {
	world := newWorld(t)
	world.secrets.Add("fixture-account", theFixtureLogin)
	world.codes.problem = errors.New("the two-factor secret in that entry is not the base32 text a site hands out")
	browser := world.browser(t, nil)
	world.worker.AddPage(loginPageWithACodeBox())
	if _, err := browser.Open(context.Background(), testkit.FixtureLoginPage); err != nil {
		t.Fatalf("opening the login page failed: %v", err)
	}

	if _, err := browser.Login(context.Background(), "fixture-account", LoginRefs{}); err == nil {
		t.Fatal("a login whose code could not be made should have stopped rather than filling the form without one")
	}
	if typed := world.worker.TypedInto(testkit.FixturePasswordRef); len(typed) != 0 {
		t.Fatalf("the worker typed %d things, and a login that stopped types nothing at all", len(typed))
	}
}

// An answer that still held a credential is thrown away, whatever it said,
// because a credential must never reach the model's context.
func TestAnAnswerThatStillHeldACredentialIsThrownAway(t *testing.T) {
	fields := contract.LoginFields{Password: "a-password-nobody-should-see", Username: "jared@example.com"}
	leaky := contract.Diff{Seen: "the box now holds a-password-nobody-should-see"}
	if err := checkNothingLeaked(leaky, fields); err == nil {
		t.Fatal("an answer holding the password should have been thrown away")
	}
	if err := checkNothingLeaked(contract.Diff{Seen: "the box is filled"}, fields); err != nil {
		t.Fatalf("a clean answer said %v, and it should say nothing at all", err)
	}
	short := contract.LoginFields{Code: "1234", Password: "short"}
	if err := checkNothingLeaked(contract.Diff{Seen: "there are 1234 posts"}, short); err != nil {
		t.Fatalf("a value shorter than %d characters should not be hunted for in ordinary page text, and this said %v",
			shortestGuardedSecret, err)
	}
}

// The domain check reads a vault entry's domains the way a person writes them.
func TestTheDomainCheckReadsDomainsTheWayAPersonWritesThem(t *testing.T) {
	allowed := []string{"example.com"}
	for _, address := range []string{"https://example.com/in", "https://www.example.com/in", "https://mail.example.com/in"} {
		if err := checkTheDomain(address, allowed); err != nil {
			t.Fatalf("a login on %s was refused, and it should have been allowed: %v", address, err)
		}
	}
	for _, address := range []string{"https://example.com.evil.test/in", "https://notexample.com/in", "wobble"} {
		if err := checkTheDomain(address, allowed); err == nil {
			t.Fatalf("a login on %s was allowed, and it should have been refused", address)
		}
	}
	if err := checkTheDomain("https://example.com/in", nil); err == nil {
		t.Fatal("a vault entry that names no domains should refuse every login")
	}
}

// loginPageWithACodeBox is the fixture login page with a box for the second code
// on it, which is the shape a site that asks for all three at once has.
func loginPageWithACodeBox() contract.Snapshot {
	return contract.Snapshot{
		URL: testkit.FixtureLoginPage, Title: "Sign in", TabID: "t1",
		Wall: &contract.Wall{Kind: contract.WallLogin, Detail: "a password box named Password"},
		Elements: []contract.Element{
			{Ref: testkit.FixtureUsernameRef, Role: "textbox", Name: "Username"},
			{Ref: testkit.FixturePasswordRef, Role: "textbox", Name: "Password"},
			{Ref: "e5", Role: "textbox", Name: "Verification code"},
			{Ref: "e4", Role: "button", Name: "Sign in"},
		},
	}
}
