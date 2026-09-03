package functional

// The fixture web site the browser flows run against, and the little server
// that serves it.
//
// The pages are ordinary HTML in test/fixtures/site. What makes them a site
// rather than four files lives here: the one credential the login page accepts,
// the posts the compose page keeps, and the reports the form for the quality
// skill files. Everything is held in memory and goes away with the test.
//
// The site listens on a loopback port the operating system picks, so no test has
// to reserve a port and two tests can never collide. Nothing outside the pages
// folder is ever served, because the server has no route that names a file.

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

// The one credential the fixture login page accepts. The password is the one the
// vault test stores, so a reader meets the same made-up secret twice rather than
// two of them.
const (
	fixtureUsername = "jared"
	fixturePassword = "correct-horse-battery-staple"
	fixtureCode     = "314159"
)

// fixtureLoginProblem is what the login page says when it is given anything
// else, and what a test looks for to know the credential was refused.
const fixtureLoginProblem = "That is not the username, password, and code this site knows."

// The cookie the site hands out at sign-in. A fixture needs no real session, only
// a way to tell a visitor who signed in from one who did not.
const (
	fixtureCookieName  = "fixture-session"
	fixtureCookieValue = "signed-in"
)

// maxFixturePageBytes bounds how much of one page a test will read, because
// every buffer has a cap.
const maxFixturePageBytes = 1 << 20

// fixtureSite is the site the browser flows run against: a login page that takes
// one credential, a compose page that keeps what it is given, a captcha page, and
// a form for the quality skill.
type fixtureSite struct {
	server *httptest.Server
	pages  *template.Template

	guard   sync.Mutex
	posts   []string
	reports []string
}

// startFixtureSite serves the fixture site on a loopback port for the length of
// one test.
func startFixtureSite(t *testing.T) *fixtureSite {
	t.Helper()
	pages, err := template.ParseGlob(filepath.Join("..", "fixtures", "site", "*.html"))
	if err != nil {
		t.Fatalf("cannot read the pages of the fixture site: %v", err)
	}
	site := &fixtureSite{pages: pages}
	site.server = httptest.NewServer(site.routes())
	t.Cleanup(site.server.Close)
	return site
}

// At is the whole address of one page on the running site, such as "/login".
func (site *fixtureSite) At(path string) string {
	return site.server.URL + path
}

// Posts is every message the compose page has been given, oldest first.
func (site *fixtureSite) Posts() []string {
	site.guard.Lock()
	defer site.guard.Unlock()
	return append([]string(nil), site.posts...)
}

// Reports is every report the quality form has filed, oldest first.
func (site *fixtureSite) Reports() []string {
	site.guard.Lock()
	defer site.guard.Unlock()
	return append([]string(nil), site.reports...)
}

// routes is every address the site answers on. A method the route does not name
// is answered 405 rather than by the wrong handler.
func (site *fixtureSite) routes() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	routes.HandleFunc("GET /login", site.showLogin)
	routes.HandleFunc("POST /login", site.checkTheCredential)
	routes.HandleFunc("GET /compose", site.showCompose)
	routes.HandleFunc("POST /compose", site.keepThePost)
	routes.HandleFunc("GET /captcha", site.showCaptcha)
	routes.HandleFunc("GET /qa", site.showQualityForm)
	routes.HandleFunc("POST /qa", site.fileTheReport)
	return routes
}

// showLogin serves the login page with nothing wrong yet.
func (site *fixtureSite) showLogin(w http.ResponseWriter, r *http.Request) {
	site.show(w, "login.html", map[string]any{"Problem": ""})
}

// checkTheCredential compares what was typed with the one credential this site
// knows, and either sends the visitor on to the compose page or says plainly
// that it was wrong and stays where it is.
func (site *fixtureSite) checkTheCredential(w http.ResponseWriter, r *http.Request) {
	if !readTheForm(w, r) {
		return
	}
	right := r.PostFormValue("username") == fixtureUsername &&
		r.PostFormValue("password") == fixturePassword &&
		r.PostFormValue("code") == fixtureCode
	if !right {
		site.show(w, "login.html", map[string]any{"Problem": fixtureLoginProblem})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: fixtureCookieName, Value: fixtureCookieValue, Path: "/"})
	http.Redirect(w, r, "/compose", http.StatusSeeOther)
}

// showCompose serves the compose page with everything posted so far, and sends a
// visitor who never signed in back to the login page.
func (site *fixtureSite) showCompose(w http.ResponseWriter, r *http.Request) {
	if !signedIn(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	site.show(w, "compose.html", map[string]any{"Posts": site.Posts()})
}

// keepThePost remembers one message and shows the compose page again, which is
// how a post becomes something the next page proves.
func (site *fixtureSite) keepThePost(w http.ResponseWriter, r *http.Request) {
	if !signedIn(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !readTheForm(w, r) {
		return
	}
	site.guard.Lock()
	site.posts = append(site.posts, r.PostFormValue("message"))
	site.guard.Unlock()
	http.Redirect(w, r, "/compose", http.StatusSeeOther)
}

// showCaptcha serves the page the agent is meant to stop at.
func (site *fixtureSite) showCaptcha(w http.ResponseWriter, r *http.Request) {
	site.show(w, "captcha.html", map[string]any{})
}

// showQualityForm serves the form the quality skill walks, with nothing filed
// yet.
func (site *fixtureSite) showQualityForm(w http.ResponseWriter, r *http.Request) {
	site.show(w, "qa.html", map[string]any{"Filed": ""})
}

// fileTheReport remembers one report and thanks the person who filed it by name,
// which is the plain-words state the quality skill checks for.
func (site *fixtureSite) fileTheReport(w http.ResponseWriter, r *http.Request) {
	if !readTheForm(w, r) {
		return
	}
	reporter := r.PostFormValue("reporter")
	site.guard.Lock()
	site.reports = append(site.reports,
		reporter+" said "+r.PostFormValue("what")+" about "+r.PostFormValue("area"))
	site.guard.Unlock()
	site.show(w, "qa.html", map[string]any{"Filed": reporter})
}

// show renders one page and sends it. The page is built whole before a byte goes
// out, so a template that fails halfway answers with a plain complaint rather
// than with half a page and a wrong status.
func (site *fixtureSite) show(w http.ResponseWriter, page string, data any) {
	var built bytes.Buffer
	if err := site.pages.ExecuteTemplate(&built, page, data); err != nil {
		http.Error(w, "the fixture page could not be built: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	_, _ = w.Write(built.Bytes())
}

// readTheForm reads the fields a form sent, and answers the visitor itself when
// they cannot be read at all.
func readTheForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "the form could not be read: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// signedIn says whether this visitor went through the login page.
func signedIn(r *http.Request) bool {
	cookie, err := r.Cookie(fixtureCookieName)
	return err == nil && cookie.Value == fixtureCookieValue
}
