//go:build integration

package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// These tests drive the real browser worker from worker/browser, which launches
// a real Google Chrome on this machine's own display against a profile folder
// made for the test and thrown away afterwards. The user's daily profile is
// never touched, and the worker is only ever ended by the exact process
// identifier it was started with.

// theRealLogin is the vault entry the fixture site accepts. It is a fixture, and
// the point of the test is that not one character of it comes back.
var theRealLogin = contract.Credential{
	Site:     "fixture-account",
	Username: "jared@example.com",
	Password: "a-password-nobody-should-see",
}

// The real worker starts against a temporary profile, says it is healthy, opens
// a page served on a loopback port, and is stopped by its exact process
// identifier.
func TestTheRealWorkerOpensAFixturePageAndIsStoppedByItsProcessID(t *testing.T) {
	site := httptest.NewServer(fixtureSite(t))
	defer site.Close()
	browser := realBrowser(t)
	ctx, giveUp := context.WithTimeout(context.Background(), 2*time.Minute)
	defer giveUp()

	health, err := browser.Health(ctx)
	if err != nil || !health.Healthy || health.ChromeVersion == "" {
		t.Fatalf("the real worker answered health with %+v and %v, and it should be driving a real Chrome", health, err)
	}
	pid := browser.ProcessID()
	if pid <= 0 {
		t.Fatalf("the worker is running with process id %d, and it should be a real one", pid)
	}

	page, err := browser.Open(ctx, site.URL+"/login")
	if err != nil {
		t.Fatalf("opening the fixture page failed: %v", err)
	}
	if page.Title != "Sign in" || page.Wall == nil || page.Wall.Kind != contract.WallLogin {
		t.Fatalf("the page came back as %+v, and it should be the login page behind a login wall", page)
	}
	if findBox(page, passwordWords) == "" {
		t.Fatalf("the page has %+v on it, and one of them should be the box the password goes in", page.Elements)
	}

	if err := browser.Close(); err != nil {
		t.Fatalf("stopping the real worker failed: %v", err)
	}
	waitUntil(t, "the worker to go", func() bool { return syscall.Kill(pid, 0) != nil })
}

// A login on the fixture site is filled by the worker from the vault, and not
// one character of the credential comes back in the answer.
func TestTheRealWorkerFillsALoginAndReturnsNoValue(t *testing.T) {
	site := httptest.NewServer(fixtureSite(t))
	defer site.Close()
	browser := realBrowser(t)
	ctx, giveUp := context.WithTimeout(context.Background(), 2*time.Minute)
	defer giveUp()

	credential := theRealLogin
	credential.Domains = []string{"127.0.0.1"}
	browser.options.Secrets.(*testkit.FakeSecrets).Add("fixture-account", credential)
	if _, err := browser.Open(ctx, site.URL+"/login"); err != nil {
		t.Fatalf("opening the fixture login page failed: %v", err)
	}

	diff, err := browser.Login(ctx, "fixture-account", LoginRefs{})
	if err != nil {
		t.Fatalf("the login on the real worker failed: %v", err)
	}
	written, err := json.Marshal(diff)
	if err != nil {
		t.Fatalf("the answer could not be written back as JSON: %v", err)
	}
	for _, value := range []string{theRealLogin.Password, theRealLogin.Username} {
		if strings.Contains(string(written), value) {
			t.Fatalf("the whole answer holds a credential, and no field of it ever may: %s", written)
		}
	}
	if !strings.Contains(diff.Snapshot.URL, "/welcome") {
		t.Fatalf("the login left the browser on %s, and the fixture site sends a good login to the welcome page", diff.Snapshot.URL)
	}
}

// realBrowser builds a browser on the real worker, with a profile folder made
// for this test and taken away afterwards.
func realBrowser(t *testing.T) *Browser {
	t.Helper()
	command := []string{nodeProgram(t), builtWorkerEntry(t)}
	profile := throwawayProfile(t)
	start, err := ProcessStart(command, profile, PacingFast, t.Logf)
	if err != nil {
		t.Fatalf("building the start function for the real worker failed: %v", err)
	}

	built, err := New(Options{
		Start:               start,
		Channel:             testkit.NewFakeChannel(contract.TerminalChannelName),
		Secrets:             testkit.NewFakeSecrets(),
		Clock:               clock.System(),
		DailyActionsPerSite: 50,
		Note:                t.Logf,
	})
	if err != nil {
		t.Fatalf("building the browser on the real worker failed: %v", err)
	}
	t.Cleanup(func() { _ = built.Close() })
	return built
}

// fixtureSite is two pages on a loopback port: a login form that accepts exactly
// one credential, and the page a good login lands on. Nothing is served from
// disk and no route is served that is not written here.
func fixtureSite(t *testing.T) http.Handler {
	t.Helper()
	pages := http.NewServeMux()
	pages.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.FormValue("username") == theRealLogin.Username &&
			r.FormValue("password") == theRealLogin.Password {
			http.Redirect(w, r, "/welcome", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loginPageMarkup))
	})
	pages.HandleFunc("/welcome", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>Welcome</title><h1>You are signed in</h1>"))
	})
	return pages
}

// loginPageMarkup is the fixture login form, written so that the worker's own
// wall detector sees a login page and its snapshot names the three boxes.
const loginPageMarkup = `<!doctype html><title>Sign in</title>
<h1>Sign in</h1>
<form method="post" action="/login">
<label for="username">Username</label><input id="username" name="username" type="text">
<label for="password">Password</label><input id="password" name="password" type="password">
<button type="submit">Sign in</button>
</form>`

// throwawayProfile is a Chrome profile folder for the length of one test. Chrome
// writes its last files as it is going, so the removal is tried a few times
// rather than once.
func throwawayProfile(t *testing.T) string {
	t.Helper()
	folder, err := os.MkdirTemp("", "coeus-browser-profile-")
	if err != nil {
		t.Fatalf("making a throwaway profile folder failed: %v", err)
	}
	t.Cleanup(func() {
		for attempt := 0; attempt < 20; attempt++ {
			if err := os.RemoveAll(folder); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Logf("the throwaway profile folder %s could not be taken away and is left behind", folder)
	})
	return folder
}

// nodeProgram is where Node lives, on the path or in the folder the development
// machine keeps it in, because a shell that never read the user's profile has no
// node on its path.
func nodeProgram(t *testing.T) string {
	t.Helper()
	if found, err := exec.LookPath("node"); err == nil {
		return found
	}
	fallback := filepath.Join(os.Getenv("HOME"), ".nvm", "versions", "node", "v24.18.0", "bin", "node")
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	t.Skipf("node is not on the path or at %s, so the real browser worker cannot be started", fallback)
	return ""
}

// builtWorkerEntry is the file Node runs, and says how to build it when it is
// not there. A machine with no display has no browser to drive, so the test says
// so rather than failing on a Chrome that cannot draw a window.
func builtWorkerEntry(t *testing.T) string {
	t.Helper()
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("there is no display on this machine, so there is no window for Chrome to draw in")
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("finding the working folder failed: %v", err)
	}
	entry := filepath.Join(filepath.Dir(filepath.Dir(here)), "worker", "browser", "dist", "main.js")
	if _, err := os.Stat(entry); err != nil {
		t.Skipf("the browser worker is not built at %s, so run: npm --prefix worker/browser ci && npm --prefix worker/browser run build", entry)
	}
	return entry
}
