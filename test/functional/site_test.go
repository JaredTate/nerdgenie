package functional

// This file proves the fixture web site the browser flows run against, through
// plain HTTP and with no browser anywhere near it. A browser test that fails
// should never leave the reader wondering whether the site itself was at fault,
// so every page is proved here first: the login page takes exactly one
// credential, the compose page keeps what it was given, the captcha page shows
// the thing that stops the agent, and the form for the QA skill files a report
// and says so.

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

func TestTheFixtureLoginPageRefusesEverythingButItsOneCredential(t *testing.T) {
	site := startFixtureSite(t)
	visitor := newVisitor(t)

	page, _ := fetch(t, visitor, site.At("/login"))
	for _, asked := range []string{"Username", "Password", "Code"} {
		if !strings.Contains(page, asked) {
			t.Errorf("the login page never asks for the %s, and it says: %s", asked, page)
		}
	}

	refused, landedOn := send(t, visitor, site.At("/login"), url.Values{
		"username": {fixtureUsername},
		"password": {"the wrong password"},
		"code":     {fixtureCode},
	})
	if !strings.Contains(refused, fixtureLoginProblem) {
		t.Errorf("a wrong password was let through, and the page said: %s", refused)
	}
	if !strings.HasSuffix(landedOn, "/login") {
		t.Errorf("a wrong password landed on %s, and it must stay on the login page", landedOn)
	}

	welcomed, landedOn := send(t, visitor, site.At("/login"), url.Values{
		"username": {fixtureUsername},
		"password": {fixturePassword},
		"code":     {fixtureCode},
	})
	if !strings.HasSuffix(landedOn, "/compose") {
		t.Fatalf("the one credential the site accepts landed on %s, want the compose page", landedOn)
	}
	if !strings.Contains(welcomed, "Compose a post") {
		t.Errorf("the page after signing in is not the compose page: %s", welcomed)
	}
}

func TestTheFixtureComposePageShowsThePostItWasGiven(t *testing.T) {
	site := startFixtureSite(t)
	visitor := newVisitor(t)
	signIn(t, visitor, site)

	posted, landedOn := send(t, visitor, site.At("/compose"), url.Values{
		"message": {"Nine years of DigiByte."},
	})
	if !strings.HasSuffix(landedOn, "/compose") {
		t.Fatalf("posting a message landed on %s, want the compose page again", landedOn)
	}
	if !strings.Contains(posted, "Nine years of DigiByte.") {
		t.Errorf("the compose page does not show the post it was given: %s", posted)
	}
	if kept := site.Posts(); len(kept) != 1 {
		t.Errorf("the site kept %d posts, want the one that was sent", len(kept))
	}
}

func TestTheFixtureComposePageSendsAVisitorWhoNeverSignedInBackToTheLoginPage(t *testing.T) {
	site := startFixtureSite(t)

	page, landedOn := fetch(t, newVisitor(t), site.At("/compose"))
	if !strings.HasSuffix(landedOn, "/login") {
		t.Fatalf("a visitor who never signed in landed on %s, want the login page", landedOn)
	}
	if !strings.Contains(page, "Sign in") {
		t.Errorf("the page a stranger is sent to is not the login page: %s", page)
	}
}

func TestTheFixtureCaptchaPageShowsTheThingThatStopsTheAgent(t *testing.T) {
	site := startFixtureSite(t)

	page, _ := fetch(t, newVisitor(t), site.At("/captcha"))
	if !strings.Contains(page, "I am not a robot") {
		t.Errorf("the captcha page shows nothing a wall detector could name: %s", page)
	}
}

func TestTheFixtureQualityFormFilesAReportAndSaysSo(t *testing.T) {
	site := startFixtureSite(t)
	visitor := newVisitor(t)

	page, _ := fetch(t, visitor, site.At("/qa"))
	for _, asked := range []string{"Your name", "What happened", "Which part"} {
		if !strings.Contains(page, asked) {
			t.Errorf("the form for the quality skill never asks for the %s: %s", asked, page)
		}
	}

	filed, _ := send(t, visitor, site.At("/qa"), url.Values{
		"reporter": {"Jared"},
		"what":     {"The post button did nothing."},
		"area":     {"browser"},
	})
	if !strings.Contains(filed, "Thank you, Jared") {
		t.Errorf("filing a report said nothing back: %s", filed)
	}
	if reports := site.Reports(); len(reports) != 1 || !strings.Contains(reports[0], "Jared") {
		t.Errorf("the site kept the reports %v, want the one that was filed", reports)
	}
}

// newVisitor is one person browsing the fixture site: an HTTP client that keeps
// the site's cookie, so that signing in on one request is remembered on the
// next, which is what a browser does.
func newVisitor(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cannot make the cookie jar this visitor browses with: %v", err)
	}
	return &http.Client{Jar: jar}
}

// signIn takes a visitor through the login page with the one credential the
// fixture site accepts, and fails the test if it does not land on the compose
// page.
func signIn(t *testing.T, visitor *http.Client, site *fixtureSite) {
	t.Helper()
	_, landedOn := send(t, visitor, site.At("/login"), url.Values{
		"username": {fixtureUsername},
		"password": {fixturePassword},
		"code":     {fixtureCode},
	})
	if !strings.HasSuffix(landedOn, "/compose") {
		t.Fatalf("signing in landed on %s, want the compose page", landedOn)
	}
}

// fetch asks for one page and gives back what came back and where the visitor
// ended up, which is not the address asked for when the site redirected.
func fetch(t *testing.T, visitor *http.Client, address string) (string, string) {
	t.Helper()
	answer, err := visitor.Get(address)
	if err != nil {
		t.Fatalf("cannot reach %s on the fixture site: %v", address, err)
	}
	return readPage(t, answer)
}

// send fills one form in and gives back the page it led to and where that page
// was.
func send(t *testing.T, visitor *http.Client, address string, form url.Values) (string, string) {
	t.Helper()
	answer, err := visitor.PostForm(address, form)
	if err != nil {
		t.Fatalf("cannot send the form to %s on the fixture site: %v", address, err)
	}
	return readPage(t, answer)
}

// readPage reads one answer to the end, bounded, and says where it came from.
func readPage(t *testing.T, answer *http.Response) (string, string) {
	t.Helper()
	defer func() { _ = answer.Body.Close() }()
	page, err := io.ReadAll(io.LimitReader(answer.Body, maxFixturePageBytes))
	if err != nil {
		t.Fatalf("cannot read the page the fixture site sent: %v", err)
	}
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("the fixture site answered %s with %d and said: %s",
			answer.Request.URL, answer.StatusCode, page)
	}
	return string(page), answer.Request.URL.Path
}
