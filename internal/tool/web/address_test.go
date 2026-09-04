package web_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/tool/web"
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
	// The address is written as a number rather than a name, because no test in
	// this package reaches the network, and a name would have to be looked up.
	if err := web.CheckAddressAllowed("https://93.184.216.34/notes", nil); err != nil {
		t.Errorf("an ordinary public address was refused: %v", err)
	}
}

func TestARedirectToAnAddressOnThisMachineIsRefused(t *testing.T) {
	_, server := newTool(t, "")
	elsewhere := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "<html><body><p>the secret</p></body></html>")
	}))
	t.Cleanup(elsewhere.Close)

	server.AddPage("/redirect", "")
	away := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, elsewhere.URL+"/secret", http.StatusFound)
	}))
	t.Cleanup(away.Close)

	tool := web.New(web.Settings{
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
	// The name ends in .invalid, which the standards keep aside for exactly this
	// and which therefore stands for nothing anywhere.
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

func TestEveryRedirectHopGoesThroughTheGuardAndTheRefusalNamesTheHopThatFailed(t *testing.T) {
	// No test in this package reaches the network, so the host that redirects is
	// a server on this machine that the settings name by number, which is the one
	// way past the address check. It stands in for any host the guard lets
	// through: where a page sends the agent next is checked hop after hop, and
	// the third hop here is a service on this machine that nothing names.
	service := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "<html><body><p>the service on this machine</p></body></html>")
	}))
	t.Cleanup(service.Close)

	second := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, service.URL+"/service", http.StatusFound)
	}))
	t.Cleanup(second.Close)

	first := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, second.URL+"/onwards", http.StatusFound)
	}))
	t.Cleanup(first.Close)

	tool := web.New(web.Settings{
		AllowedHosts: []string{hostOf(t, first.URL), hostOf(t, second.URL)},
		Timeout:      10 * time.Second,
	})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": first.URL + "/go"})
	if err == nil {
		t.Fatalf("the third hop of a redirect reached a service on this machine")
	}
	if !strings.Contains(err.Error(), "this machine") {
		t.Errorf("the refusal reads %q and does not say why the hop was refused", err)
	}
	if !strings.Contains(err.Error(), "redirected") || !strings.Contains(err.Error(), service.URL) {
		t.Errorf("the refusal reads %q and does not say that a page redirected the agent to %s", err, service.URL)
	}
}

func TestARedirectToTheCloudCredentialAddressIsRefusedBeforeItIsAsked(t *testing.T) {
	away := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request,
			"http://169.254.169.254/latest/meta-data/iam/security-credentials/", http.StatusFound)
	}))
	t.Cleanup(away.Close)

	tool := web.New(web.Settings{AllowedHosts: []string{hostOf(t, away.URL)}, Timeout: 10 * time.Second})
	_, err := run(t, tool, map[string]any{"action": "fetch", "url": away.URL + "/go"})
	if err == nil {
		t.Fatalf("a redirect to the address a cloud machine keeps its credentials behind was followed")
	}
	if !strings.Contains(err.Error(), "credentials") {
		t.Errorf("the refusal reads %q and does not say why the redirect was refused", err)
	}
	if !strings.Contains(err.Error(), "redirected") {
		t.Errorf("the refusal reads %q and does not say that a page sent the agent there", err)
	}
}

// The addresses the wave 6 security review reached past the network guard.
// Design section 11 says the agent only fetches pages from the public web, and
// every address below is on somebody's private network, or is a private address
// written in a way that hides it, and every one of them was allowed before the
// table in address.go was written.

// addressesThatAreNotOnThePublicWeb is one address out of every range the guard
// must refuse. Go's own IsPrivate knows four of these ranges and no more, so the
// rest were reachable: the shared address space is where a tailnet lives, the
// tunnel prefixes carry a private IPv4 number inside an IPv6 one, and the last
// few are a private number written in octal, in hexadecimal, or as one long
// integer, which the machine's own name lookup turns back into an address.
var addressesThatAreNotOnThePublicWeb = []struct {
	name    string
	address string
}{
	{"this network, which is not a machine", "http://0.0.0.1/"},
	{"this network again", "http://0.1.0.0/"},
	{"the first private range", "http://10.0.0.1/"},
	{"the shared address space, which is where a tailnet lives", "http://100.64.0.1/"},
	{"the shared address space again", "http://100.100.100.100/"},
	{"loopback", "http://127.0.0.1/"},
	{"loopback on another number of the same range", "http://127.9.9.9/"},
	{"the link-local range", "http://169.254.1.1/"},
	{"the address a cloud machine keeps its credentials behind", "http://169.254.169.254/latest/meta-data"},
	{"the second private range", "http://172.16.0.1/"},
	{"the range reserved for protocol assignments", "http://192.0.0.1/"},
	{"the first range reserved for documentation", "http://192.0.2.1/"},
	{"the third private range", "http://192.168.1.1/"},
	{"the range reserved for testing between networks", "http://198.18.0.1/"},
	{"the second range reserved for documentation", "http://198.51.100.1/"},
	{"the third range reserved for documentation", "http://203.0.113.1/"},
	{"the multicast range", "http://224.0.0.1/"},
	{"the range reserved for the future", "http://240.0.0.1/"},
	{"the broadcast address", "http://255.255.255.255/"},
	{"loopback the IPv6 way", "http://[::1]/"},
	{"the address that stands for nothing at all", "http://[::]/"},
	{"the unique local range", "http://[fc00::1]/"},
	{"the unique local range as a real machine writes it", "http://[fd12:3456:789a::1]/"},
	{"the IPv6 link-local range", "http://[fe80::1]/"},
	{"the IPv6 multicast range", "http://[ff02::1]/"},
	{"loopback written the old IPv6 way", "http://[::127.0.0.1]/"},
	{"loopback written the old IPv6 way in full", "http://[0:0:0:0:0:0:7f00:1]/"},
	{"loopback behind the well-known NAT64 prefix", "http://[64:ff9b::7f00:1]/"},
	{"loopback behind the 6to4 prefix", "http://[2002:7f00:0001::]/"},
	{"loopback as an IPv4-mapped IPv6 address", "http://[::ffff:127.0.0.1]/"},
	{"a private address as an IPv4-mapped IPv6 address", "http://[::ffff:10.0.0.1]/"},
	{"a private address as an IPv4-mapped IPv6 address in hexadecimal", "http://[::ffff:a00:1]/"},
	{"loopback written in octal", "http://0177.0.0.1/"},
	{"loopback written in hexadecimal", "http://0x7f000001/"},
	{"loopback written in hexadecimal one part at a time", "http://0x7f.0x0.0x0.0x1/"},
	{"loopback written as one long integer", "http://2130706433/"},
	{"loopback written with parts left out", "http://127.1/"},
}

func TestTheGuardRefusesEveryAddressThatIsNotOnThePublicWeb(t *testing.T) {
	for _, one := range addressesThatAreNotOnThePublicWeb {
		if err := web.CheckAddressAllowed(one.address, nil); err == nil {
			t.Errorf("%s: %s was allowed, and the agent only fetches pages from the public web", one.name, one.address)
		}
	}
}

func TestAnOrdinaryPublicAddressIsStillAllowedAfterTheTable(t *testing.T) {
	// Written as numbers rather than as names, because no test in this package
	// reaches the network and a name would have to be looked up.
	for _, address := range []string{
		"https://93.184.216.34/notes",
		"https://8.8.8.8/",
		"https://100.63.255.255/",
		"https://100.128.0.1/",
		"https://[2606:2800:220:1:248:1893:25c8:1946]/",
	} {
		if err := web.CheckAddressAllowed(address, nil); err != nil {
			t.Errorf("the ordinary public address %s was refused: %v", address, err)
		}
	}
}

func TestAHostTheSettingsAllowIsStillCheckedForWhereItLeads(t *testing.T) {
	// This is exactly what web.New used to put on the list for the shipped
	// results page, because hostOf handed back url.Host and that address names
	// no port. A host on the list skipped the resolve and the public-address
	// check altogether, on every port, so whoever answered for that name decided
	// where the agent connected.
	allowed := []string{"html.duckduckgo.com"}

	for _, address := range []string{
		"http://html.duckduckgo.com:19091/v1/models",
		"http://html.duckduckgo.com:22/",
	} {
		err := web.CheckAddressAllowed(address, allowed)
		if err == nil {
			t.Errorf("%s was allowed on a port the settings never named; a host the settings allow is one address and one port, not a whole machine", address)
			continue
		}
		if !strings.Contains(err.Error(), "port") && !strings.Contains(err.Error(), "private") {
			t.Logf("%s was refused with %q", address, err)
		}
	}
}

func TestAHostTheSettingsAllowByNameIsResolvedAndItsAddressesChecked(t *testing.T) {
	// A name on the list, port and all, still goes through the address check,
	// because the settings named a name and whoever answers for that name is the
	// one choosing the number. Only a number the settings name is taken as the
	// user's own choice.
	if err := web.CheckAddressAllowed("http://localhost:80/", []string{"localhost:80"}); err == nil {
		t.Errorf("a name on the allow list skipped the address check, and it stands for this machine")
	}
	if err := web.CheckAddressAllowed("http://127.0.0.1:80/", []string{"127.0.0.1:80"}); err != nil {
		t.Errorf("a number the settings name was refused, and that is how a server of the user's own stays reachable: %v", err)
	}
	if err := web.CheckAddressAllowed("http://127.0.0.1:81/", []string{"127.0.0.1:80"}); err == nil {
		t.Errorf("a number the settings name was reached on a port they never named")
	}
}
