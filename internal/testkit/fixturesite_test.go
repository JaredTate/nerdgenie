package testkit_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
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
		n, err := answer.Body.Read(buffer)
		built.Write(buffer[:n])
		if err != nil {
			break
		}
	}
	return built.String()
}
