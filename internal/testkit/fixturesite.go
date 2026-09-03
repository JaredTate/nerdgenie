package testkit

// The fixture web site the browser flows and the human trial run against.
//
// The pages are ordinary HTML in test/fixtures/site. What makes them a site
// rather than five files lives here: the one credential the login page accepts,
// the posts the compose page keeps, and the reports the form for the quality
// skill files. Everything is held in memory. The functional suite serves it on
// a loopback port of the operating system's choosing, and "go run
// ./scripts/fixturesite" serves it for a person to watch the agent use.
// Nothing outside the pages folder is ever served, because the site has no
// route that names a file.

import (
	"bytes"
	"html/template"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
)

// The one credential the fixture login page accepts. The password is the one the
// vault test stores, so a reader meets the same made-up secret twice rather than
// two of them.
const (
	FixtureUsername = "jared"
	FixturePassword = "correct-horse-battery-staple"
	FixtureCode     = "314159"
)

// FixtureLoginProblem is what the login page says when it is given anything
// else, and what a test looks for to know the credential was refused.
const FixtureLoginProblem = "That is not the username, password, and code this site knows."

// The cookie the site hands out at sign-in. A fixture needs no real session, only
// a way to tell a visitor who signed in from one who did not.
const (
	FixtureCookieName  = "fixture-session"
	FixtureCookieValue = "signed-in"
)

// FixtureSite is the site: a login page that takes one credential, a compose
// page that keeps what it is given, a captcha page, a form for the quality
// skill, and a rankings table whose numbers live in its cells and on no
// element.
type FixtureSite struct {
	pages *template.Template

	guard   sync.Mutex
	posts   []string
	reports []string
}

// FixturePagesFolder is where the pages live in the repository, found from this
// file, so a caller anywhere in the tree can name it.
func FixturePagesFolder() string {
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "..", "..", "test", "fixtures", "site")
}

// NewFixtureSite reads the pages out of the folder and returns the site.
func NewFixtureSite(folder string) (*FixtureSite, error) {
	pages, err := template.ParseGlob(filepath.Join(folder, "*.html"))
	if err != nil {
		return nil, err
	}
	return &FixtureSite{pages: pages}, nil
}

// Posts is every message the compose page has been given, oldest first.
func (site *FixtureSite) Posts() []string {
	site.guard.Lock()
	defer site.guard.Unlock()
	return append([]string(nil), site.posts...)
}

// Reports is every report the quality form has filed, oldest first.
func (site *FixtureSite) Reports() []string {
	site.guard.Lock()
	defer site.guard.Unlock()
	return append([]string(nil), site.reports...)
}

// Handler is every address the site answers on. A method the route does not
// name is answered 405 rather than by the wrong handler.
func (site *FixtureSite) Handler() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /{$}", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/login", http.StatusSeeOther)
	})
	routes.HandleFunc("GET /login", site.showLogin)
	routes.HandleFunc("POST /login", site.checkTheCredential)
	routes.HandleFunc("GET /compose", site.showCompose)
	routes.HandleFunc("POST /compose", site.keepThePost)
	routes.HandleFunc("GET /captcha", site.showCaptcha)
	routes.HandleFunc("GET /qa", site.showQualityForm)
	routes.HandleFunc("POST /qa", site.fileTheReport)
	routes.HandleFunc("GET /rankings", site.showRankings)
	return routes
}

// showLogin serves the login page with nothing wrong yet.
func (site *FixtureSite) showLogin(writer http.ResponseWriter, request *http.Request) {
	site.show(writer, "login.html", map[string]any{"Problem": ""})
}

// checkTheCredential compares what was typed with the one credential this site
// knows, and either sends the visitor on to the compose page or says plainly
// that it was wrong and stays where it is.
func (site *FixtureSite) checkTheCredential(writer http.ResponseWriter, request *http.Request) {
	if !readTheForm(writer, request) {
		return
	}
	right := request.PostFormValue("username") == FixtureUsername &&
		request.PostFormValue("password") == FixturePassword &&
		request.PostFormValue("code") == FixtureCode
	if !right {
		site.show(writer, "login.html", map[string]any{"Problem": FixtureLoginProblem})
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: FixtureCookieName, Value: FixtureCookieValue, Path: "/"})
	http.Redirect(writer, request, "/compose", http.StatusSeeOther)
}

// showCompose serves the compose page with everything posted so far, and sends a
// visitor who never signed in back to the login page.
func (site *FixtureSite) showCompose(writer http.ResponseWriter, request *http.Request) {
	if !signedIn(request) {
		http.Redirect(writer, request, "/login", http.StatusSeeOther)
		return
	}
	site.show(writer, "compose.html", map[string]any{"Posts": site.Posts()})
}

// keepThePost remembers one message and shows the compose page again, which is
// how a post becomes something the next page proves.
func (site *FixtureSite) keepThePost(writer http.ResponseWriter, request *http.Request) {
	if !signedIn(request) {
		http.Redirect(writer, request, "/login", http.StatusSeeOther)
		return
	}
	if !readTheForm(writer, request) {
		return
	}
	site.guard.Lock()
	site.posts = append(site.posts, request.PostFormValue("message"))
	site.guard.Unlock()
	http.Redirect(writer, request, "/compose", http.StatusSeeOther)
}

// showCaptcha serves the page the agent is meant to stop at.
func (site *FixtureSite) showCaptcha(writer http.ResponseWriter, request *http.Request) {
	site.show(writer, "captcha.html", map[string]any{})
}

// showRankings serves the rankings table. Every row of it starts with an icon
// button that has no name, so the numbers live in its cells and on no element,
// which is what the browser's integration test reads the text of a page for.
func (site *FixtureSite) showRankings(writer http.ResponseWriter, request *http.Request) {
	site.show(writer, "rankings.html", map[string]any{})
}

// showQualityForm serves the form the quality skill walks, with nothing filed
// yet.
func (site *FixtureSite) showQualityForm(writer http.ResponseWriter, request *http.Request) {
	site.show(writer, "qa.html", map[string]any{"Filed": ""})
}

// fileTheReport remembers one report and thanks the person who filed it by name,
// which is the plain-words state the quality skill checks for.
func (site *FixtureSite) fileTheReport(writer http.ResponseWriter, request *http.Request) {
	if !readTheForm(writer, request) {
		return
	}
	reporter := request.PostFormValue("reporter")
	site.guard.Lock()
	site.reports = append(site.reports,
		reporter+" said "+request.PostFormValue("what")+" about "+request.PostFormValue("area"))
	site.guard.Unlock()
	site.show(writer, "qa.html", map[string]any{"Filed": reporter})
}

// show renders one page and sends it. The page is built whole before a byte goes
// out, so a template that fails halfway answers with a plain complaint rather
// than with half a page and a wrong status.
func (site *FixtureSite) show(writer http.ResponseWriter, page string, data any) {
	var built bytes.Buffer
	if err := site.pages.ExecuteTemplate(&built, page, data); err != nil {
		http.Error(writer, "the fixture page could not be built: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("content-type", "text/html; charset=utf-8")
	writer.Header().Set("cache-control", "no-store")
	_, _ = writer.Write(built.Bytes())
}

// readTheForm reads the fields a form sent, and answers the visitor itself when
// they cannot be read at all.
func readTheForm(writer http.ResponseWriter, request *http.Request) bool {
	if err := request.ParseForm(); err != nil {
		http.Error(writer, "the form could not be read: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// signedIn says whether this visitor went through the login page.
func signedIn(request *http.Request) bool {
	cookie, err := request.Cookie(FixtureCookieName)
	return err == nil && cookie.Value == FixtureCookieValue
}
