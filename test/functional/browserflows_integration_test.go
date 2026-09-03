//go:build integration

package functional

// The browser flows against the fixture site, driven through the real browser
// worker and a real Chrome window on this machine's own display.
//
// The site is the one in test/fixtures/site, served on a loopback port, so these
// tests reach nothing outside this machine. What they prove is the half of brief
// 5.5 that needs no Go side: the worker sees the fixture login page as a wall,
// fills it with values the model never sees, and lands on the compose page.
// Everything that needs internal/browser is listed as a skipped test in
// browserflows_test.go.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestTheRealWorkerMeetsALoginWallAndFillsItWithoutEverGivingBackTheSecrets(t *testing.T) {
	site := startFixtureSite(t)
	worker := startBrowserWorker(t)

	landing := snapshotOf(t, worker.result(t, "open", map[string]any{"url": site.At("/login")}))
	if landing.Wall == nil {
		t.Fatalf("the fixture login page raised no wall, and it showed %v", namesOn(landing))
	}
	if landing.Wall.Kind != contract.WallLogin {
		t.Errorf("the fixture login page raised a %q wall, want a login wall: %s",
			landing.Wall.Kind, landing.Wall.Detail)
	}

	answer := worker.result(t, "loginFill", map[string]any{
		"usernameRef": refNamed(t, landing, "Username"),
		"passwordRef": refNamed(t, landing, "Password"),
		"codeRef":     refNamed(t, landing, "Code"),
		"username":    fixtureUsername,
		"password":    fixturePassword,
		"code":        fixtureCode,
	})
	for _, secret := range []string{fixtureUsername, fixturePassword, fixtureCode} {
		if strings.Contains(string(answer), secret) {
			t.Errorf("the answer to loginFill still holds %q, and no field of it may: %s", secret, answer)
		}
	}

	filled := diffOf(t, answer)
	if !strings.HasSuffix(filled.URL, "/compose") {
		t.Fatalf("filling the login form landed on %s, want the compose page", filled.URL)
	}
	if !filled.URLChanged {
		t.Errorf("filling the login form reached %s without reporting that the address changed", filled.URL)
	}
	if filled.Snapshot.Wall != nil {
		t.Errorf("the compose page raised a %q wall, and there is nothing on it to stop at",
			filled.Snapshot.Wall.Kind)
	}
	if filled.Snapshot.Title != "Compose a post" {
		t.Errorf("the page after signing in is titled %q, want the compose page", filled.Snapshot.Title)
	}
	refNamed(t, filled.Snapshot, "Post text")
	refNamed(t, filled.Snapshot, "Post")
}

func TestTheRealWorkerSeesTheFixtureCaptchaPageAsAWallItMustStopAt(t *testing.T) {
	site := startFixtureSite(t)
	worker := startBrowserWorker(t)

	page := snapshotOf(t, worker.result(t, "open", map[string]any{"url": site.At("/captcha")}))
	if page.Wall == nil {
		t.Fatalf("the fixture captcha page raised no wall, and it showed %v", namesOn(page))
	}
	if page.Wall.Kind != contract.WallCaptcha {
		t.Errorf("the fixture captcha page raised a %q wall, want a captcha: %s",
			page.Wall.Kind, page.Wall.Detail)
	}
}

// snapshotOf reads one page out of what the worker answered. Decoding into the
// shared contract type is half the point: a worker whose JSON stopped fitting
// internal/contract fails here rather than in wave 6.
func snapshotOf(t *testing.T, answer json.RawMessage) contract.Snapshot {
	t.Helper()
	var page contract.Snapshot
	if err := json.Unmarshal(answer, &page); err != nil {
		t.Fatalf("the worker's page does not fit contract.Snapshot: %v, and it was: %s", err, answer)
	}
	return page
}

// diffOf reads what one action changed out of what the worker answered.
func diffOf(t *testing.T, answer json.RawMessage) contract.Diff {
	t.Helper()
	var changed contract.Diff
	if err := json.Unmarshal(answer, &changed); err != nil {
		t.Fatalf("the worker's diff does not fit contract.Diff: %v, and it was: %s", err, answer)
	}
	return changed
}

// refNamed is the label the model would point at to reach the one element on the
// page with that name.
func refNamed(t *testing.T, page contract.Snapshot, name string) string {
	t.Helper()
	for _, element := range page.Elements {
		if element.Name == name {
			return element.Ref
		}
	}
	t.Fatalf("there is nothing named %q on %s, only %v", name, page.URL, namesOn(page))
	return ""
}

// namesOn is what the page showed, in the words a failure message needs.
func namesOn(page contract.Snapshot) []string {
	shown := make([]string, 0, len(page.Elements))
	for _, element := range page.Elements {
		shown = append(shown, element.Role+" "+element.Name)
	}
	return shown
}
