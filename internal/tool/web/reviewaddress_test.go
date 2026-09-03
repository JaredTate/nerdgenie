package web_test

// The addresses the wave 6 security review reached past the network guard.
// Design section 11 says the agent only fetches pages from the public web, and
// every address here is on somebody's private network and is allowed today.

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/web"
)

// privateAddressesTheGuardMisses are the ranges Go's own IsPrivate does not
// cover. The first two are the shared address space carriers and Tailscale use,
// so a machine on a tailnet is one fetch away; the rest are ranges nobody may
// route on the public web, and a name that answers with one of them is a name
// being used to get inside.
var privateAddressesTheGuardMisses = []struct {
	name    string
	address string
}{
	{"the shared address space, which is where a tailnet lives", "http://100.64.0.1/"},
	{"the shared address space again", "http://100.100.100.100/"},
	{"the range reserved for testing between networks", "http://198.18.0.1/"},
	{"the range reserved for protocol assignments", "http://192.0.0.1/"},
	{"the range reserved for the future", "http://240.0.0.1/"},
	{"the broadcast address", "http://255.255.255.255/"},
	{"this network, which is not a machine", "http://0.0.0.1/"},
	{"loopback written the old IPv6 way", "http://[::127.0.0.1]/"},
	{"loopback written the old IPv6 way in full", "http://[0:0:0:0:0:0:7f00:1]/"},
	{"loopback behind the well-known NAT64 prefix", "http://[64:ff9b::7f00:1]/"},
	{"loopback behind the 6to4 prefix", "http://[2002:7f00:0001::]/"},
}

func TestTheGuardRefusesEveryAddressThatIsNotOnThePublicWeb(t *testing.T) {
	for _, one := range privateAddressesTheGuardMisses {
		if err := web.CheckAddressAllowed(one.address, nil); err == nil {
			t.Errorf("%s: %s was allowed, and the agent only fetches pages from the public web", one.name, one.address)
		}
	}
}

func TestAHostTheSettingsAllowIsStillCheckedForWhereItLeads(t *testing.T) {
	// This is exactly what web.New puts on the list for the shipped results
	// page, because hostOf hands back url.Host and that address names no port.
	// A host on the list skips the resolve and the public-address check
	// altogether, on every port, so whoever answers for that name decides where
	// the agent connects.
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
