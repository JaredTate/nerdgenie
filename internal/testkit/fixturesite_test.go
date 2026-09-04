package testkit_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFixtureSiteTakesOneCredentialKeepsPostsAndShowsACaptcha is the site
// the browser flows and the wave 5 trial run against, now shared: the login
// page refuses anything but the one credential, a signed-in visitor's post is
// kept, the captcha page is there to stop at, and nothing outside the pages is
// served.
func TestTheFixtureSiteTakesOneCredentialKeepsPostsAndShowsACaptcha(t *testing.T) {
	site, err := testkit.NewFixtureSite(testkit.FixturePagesFolder())
	if err != nil {
		t.Fatalf("the fixture site could not be built: %v", err)
	}
	server := httptest.NewServer(site.Handler())
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	wrong, _ := client.PostForm(server.URL+"/login", url.Values{"username": {"x"}, "password": {"y"}, "code": {"z"}})
	if body := readAll(t, wrong); !strings.Contains(body, testkit.FixtureLoginProblem) {
		t.Errorf("a wrong credential was not refused in the site's words")
	}
	right, _ := client.PostForm(server.URL+"/login", url.Values{
		"username": {testkit.FixtureUsername}, "password": {testkit.FixturePassword}, "code": {testkit.FixtureCode},
	})
	if right.StatusCode != http.StatusSeeOther {
		t.Fatalf("the right credential answered %d, want a redirect to the compose page", right.StatusCode)
	}
	cookies := right.Cookies()
	posting, _ := http.NewRequest(http.MethodPost, server.URL+"/compose", strings.NewReader("message=hello+there"))
	posting.Header.Set("content-type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		posting.AddCookie(cookie)
	}
	if answer, _ := client.Do(posting); answer.StatusCode != http.StatusSeeOther {
		t.Errorf("a signed-in post answered %d", answer.StatusCode)
	}
	if posts := site.Posts(); len(posts) != 1 || posts[0] != "hello there" {
		t.Errorf("the site kept %v, want the one post", posts)
	}
	captcha, _ := client.Get(server.URL + "/captcha")
	if captcha.StatusCode != http.StatusOK {
		t.Errorf("the captcha page answered %d", captcha.StatusCode)
	}
	if elsewhere, _ := client.Get(server.URL + "/login.html"); elsewhere.StatusCode != http.StatusNotFound {
		t.Errorf("a file outside the routes answered %d, want not found", elsewhere.StatusCode)
	}
}

func readAll(t *testing.T, answer *http.Response) string {
	t.Helper()
	defer answer.Body.Close()
	built := strings.Builder{}
	buffer := make([]byte, 4096)
	for {
		count, err := answer.Body.Read(buffer)
		built.Write(buffer[:count])
		if err != nil {
			break
		}
	}
	return built.String()
}

// TestTheFixtureSiteFilesReportsAndRefusesAFormItCannotRead covers the quality
// form the qa skill walks, and the one answer a form that cannot be read gets.
func TestTheFixtureSiteFilesReportsAndRefusesAFormItCannotRead(t *testing.T) {
	site, err := testkit.NewFixtureSite(testkit.FixturePagesFolder())
	if err != nil {
		t.Fatalf("the fixture site could not be built: %v", err)
	}
	server := httptest.NewServer(site.Handler())
	defer server.Close()

	if form, _ := http.Get(server.URL + "/qa"); form.StatusCode != http.StatusOK {
		t.Errorf("the quality form answered %d", form.StatusCode)
	}
	filed, _ := http.PostForm(server.URL+"/qa", url.Values{"reporter": {"jared"}, "what": {"the button is off"}, "area": {"the top"}})
	if body := readAll(t, filed); !strings.Contains(body, "jared") {
		t.Errorf("the form did not thank the reporter by name")
	}
	if reports := site.Reports(); len(reports) != 1 || !strings.Contains(reports[0], "jared said the button is off about the top") {
		t.Errorf("the site kept %v, want the one report", reports)
	}
	broken, _ := http.NewRequest(http.MethodPost, server.URL+"/qa", strings.NewReader("reporter=%zz"))
	broken.Header.Set("content-type", "application/x-www-form-urlencoded")
	if answer, _ := http.DefaultClient.Do(broken); answer.StatusCode != http.StatusBadRequest {
		t.Errorf("a form that cannot be read answered %d, want a bad request", answer.StatusCode)
	}
	if home, _ := http.Get(server.URL + "/"); home.Request.URL.Path != "/login" {
		t.Errorf("the front page led to %s, want the login page", home.Request.URL.Path)
	}
}

// TestTheFixtureSiteServesARankingsTable is the page whose numbers live in its
// cells and not on any element, which the first human trial found the outline
// of a page left out, so that the browser's integration test can prove the
// text of a page arrives end to end.
func TestTheFixtureSiteServesARankingsTable(t *testing.T) {
	site, err := testkit.NewFixtureSite(testkit.FixturePagesFolder())
	if err != nil {
		t.Fatalf("the fixture site could not be built: %v", err)
	}
	server := httptest.NewServer(site.Handler())
	defer server.Close()

	answer, err := http.Get(server.URL + "/rankings")
	if err != nil || answer.StatusCode != http.StatusOK {
		t.Fatalf("the rankings page answered %v and %v, want it served", answer, err)
	}
	body := readAll(t, answer)
	for _, wanted := range []string{"<table>", "DigiByte", "91.4", "Load more"} {
		if !strings.Contains(body, wanted) {
			t.Errorf("the rankings page does not hold %q", wanted)
		}
	}
}
