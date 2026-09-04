package functional

// The fixture web site the browser flows run against, served on a loopback
// port the operating system picks so that no test has to reserve one. The site
// itself lives in internal/testkit, where "go run ./scripts/fixturesite" serves
// it for a person too.

import (
	"net/http/httptest"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// The names the flows in this package use for the site's one credential and
// the words it answers with, kept short here.
const (
	fixtureUsername     = testkit.FixtureUsername
	fixturePassword     = testkit.FixturePassword
	fixtureCode         = testkit.FixtureCode
	fixtureLoginProblem = testkit.FixtureLoginProblem
	fixtureCookieName   = testkit.FixtureCookieName
	fixtureCookieValue  = testkit.FixtureCookieValue
)

// maxFixturePageBytes bounds how much of one page a test will read, because
// every buffer has a cap.
const maxFixturePageBytes = 1 << 20

// fixtureSite is the shared site behind a server of this test's own.
type fixtureSite struct {
	*testkit.FixtureSite
	server *httptest.Server
}

// startFixtureSite serves the site on a loopback port for the life of the test.
func startFixtureSite(t *testing.T) *fixtureSite {
	t.Helper()
	site, err := testkit.NewFixtureSite(testkit.FixturePagesFolder())
	if err != nil {
		t.Fatalf("cannot read the pages of the fixture site: %v", err)
	}
	server := httptest.NewServer(site.Handler())
	t.Cleanup(server.Close)
	return &fixtureSite{FixtureSite: site, server: server}
}

// At is the whole address of one page on the running site, such as "/login".
func (site *fixtureSite) At(path string) string {
	return site.server.URL + path
}
