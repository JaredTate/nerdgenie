package web_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/tool/web"
)

func TestAnAddressOnThisMachineIsRefusedUnlessItIsAllowed(t *testing.T) {
	tool, server := newTool(t, "")

	if _, err := run(t, tool, map[string]any{"action": "fetch", "url": server.PageAddress("/notes")}); err != nil {
		t.Fatalf("the server the settings allow was refused: %v", err)
	}

	elsewhere := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "<html><body><p>this is on the machine too</p></body></html>")
	}))
	t.Cleanup(elsewhere.Close)

	_, err := run(t, tool, map[string]any{"action": "fetch", "url": elsewhere.URL + "/anything"})
	if err == nil {
		t.Fatalf("an address on this machine that the settings do not allow was fetched")
	}
	if !strings.Contains(err.Error(), "this machine") {
		t.Errorf("the refusal reads %q and does not say why the address was refused", err)
	}
}

func TestThePrivateRangesAreRefused(t *testing.T) {
	for _, address := range []string{
		"http://10.0.0.1/x", "http://192.168.1.1/x", "http://172.16.0.1/x",
		"http://169.254.169.254/latest/meta-data", "http://[::1]/x", "http://0.0.0.0/x",
	} {
		if err := web.CheckAddressAllowed(address, nil); err == nil {
			t.Errorf("the address %s was allowed, and it is not one the agent may reach", address)
		}
	}
}

func TestAPublicAddressIsAllowed(t *testing.T) {
	if err := web.CheckAddressAllowed("https://example.com/notes", nil); err != nil {
		t.Errorf("an ordinary public address was refused: %v", err)
	}
}

func TestARedirectToAnAddressOnThisMachineIsRefused(t *testing.T) {
	tool, server := newTool(t, "")
	elsewhere := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "<html><body><p>the secret</p></body></html>")
	}))
	t.Cleanup(elsewhere.Close)

	server.AddPage("/redirect", "")
	away := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Redirect(writer, nil, elsewhere.URL+"/secret", http.StatusFound)
	}))
	t.Cleanup(away.Close)

	tool = web.New(web.Settings{
		ResultsPageAddress: server.DuckDuckGoAddress(),
		AllowedHosts:       []string{hostOf(t, away.URL)},
		Timeout:            10 * time.Second,
	})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": away.URL + "/go"})
	if err == nil {
		t.Fatalf("a redirect to an address on this machine was followed")
	}
	if !strings.Contains(err.Error(), "this machine") {
		t.Errorf("the refusal reads %q and does not say why the redirect was refused", err)
	}
}

func TestAHostThatResolvesToNothingSaysSo(t *testing.T) {
	if err := web.CheckAddressAllowed("https://no-such-host.invalid/x", nil); err == nil {
		t.Errorf("a host that resolves to nothing was allowed")
	}
}

func TestTheAddressIsPinnedToTheOneItResolvedTo(t *testing.T) {
	tool, server := newTool(t, "")
	address, err := web.PinnedAddress(server.PageAddress("/notes"), []string{hostOf(t, server.Address())})
	if err != nil {
		t.Fatalf("cannot pin the address of the fake server: %v", err)
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("the pinned address %q is not a host and a port: %v", address, err)
	}
	if net.ParseIP(host) == nil {
		t.Errorf("the pinned address is %q, and it must be the number the name resolved to", address)
	}
	if _, err := run(t, tool, map[string]any{"action": "fetch", "url": server.PageAddress("/notes")}); err != nil {
		t.Errorf("fetching through the pinned address failed: %v", err)
	}
}

func TestTooManyRedirectsIsRefused(t *testing.T) {
	var circling *httptest.Server
	circling = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, circling.URL+"/round-again", http.StatusFound)
	}))
	t.Cleanup(circling.Close)

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, circling.URL)}, Timeout: 10 * time.Second})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": circling.URL + "/start"})
	if err == nil {
		t.Fatalf("a page that redirects for ever was followed for ever")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("the refusal reads %q and does not say what went round in circles", err)
	}
}

func TestAPageBiggerThanTheCapIsCut(t *testing.T) {
	enormous := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(writer, strings.Repeat("a very long page. ", web.MaxPageBytes/10))
	}))
	t.Cleanup(enormous.Close)

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, enormous.URL)}, Timeout: 10 * time.Second})
	output, err := run(t, tool, map[string]any{"action": "fetch", "url": enormous.URL + "/big"})
	if err != nil {
		t.Fatalf("fetching a very long page failed: %v", err)
	}
	if len(output.Text) > web.MaxPageBytes+2000 {
		t.Errorf("the result is %d bytes, and a page is capped at %d", len(output.Text), web.MaxPageBytes)
	}
}

func TestAPageThatIsNotTextIsRefused(t *testing.T) {
	pictures := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	t.Cleanup(pictures.Close)

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, pictures.URL)}, Timeout: 10 * time.Second})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": pictures.URL + "/picture.png"})
	if err == nil {
		t.Fatalf("a page that is not text was read as text")
	}
	if !strings.Contains(err.Error(), "image/png") {
		t.Errorf("the refusal reads %q and does not say what kind of thing the address holds", err)
	}
}
