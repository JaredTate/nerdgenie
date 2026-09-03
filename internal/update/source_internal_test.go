package update

import (
	"net/http"
	"testing"
)

// aRequestFor is one hop of a redirect chain, which is all the redirect rule
// looks at.
func aRequestFor(t *testing.T, address string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		t.Fatalf("cannot write a request for %s: %v", address, err)
	}
	return request
}

func TestAReleaseReadOverHttpsIsNotFollowedToAPlainAddress(t *testing.T) {
	client := Source{Address: "https://releases.example/latest"}.client()
	if client.CheckRedirect == nil {
		t.Fatalf("the release client follows a redirect wherever it leads, and the manifest carries the checksums, so a plain http hop is a whole unchecked release")
	}
	fromHttps := []*http.Request{aRequestFor(t, "https://releases.example/latest/manifest.json")}

	if err := client.CheckRedirect(aRequestFor(t, "http://releases.example/latest/manifest.json"), fromHttps); err == nil {
		t.Errorf("a release read over https was followed on to a plain http address")
	}
	if err := client.CheckRedirect(aRequestFor(t, "https://elsewhere.example/manifest.json"), fromHttps); err != nil {
		t.Errorf("a release was not followed to another https address: %v", err)
	}
}

func TestAReleaseAddressThatKeepsMovingIsGivenUpOn(t *testing.T) {
	client := Source{Address: "https://releases.example/latest"}.client()
	hops := make([]*http.Request, maxRedirects)
	for at := range hops {
		hops[at] = aRequestFor(t, "https://releases.example/one")
	}

	if err := client.CheckRedirect(aRequestFor(t, "https://releases.example/two"), hops); err == nil {
		t.Errorf("a release address that has already moved %d times was followed once more", maxRedirects)
	}
}
