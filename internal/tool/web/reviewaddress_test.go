package web_test

// The addresses the wave 6 security review reached past the network guard.
// Design section 11 says the agent only fetches pages from the public web, and
// every address here is on somebody's private network, or is a private address
// written in a way that hides it, and every one of them was allowed before this
// table was written.

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/web"
)

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
