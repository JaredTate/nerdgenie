// Resolving a name once and connecting to that exact number, so that a name
// which answers differently the second time cannot lead somewhere else, is
// OpenClaw's pinned lookup, at ~/Code/openclaw/src/infra/net/ssrf.ts. The Go
// here is written fresh.

package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// MaxLookupTime is how long a name has to resolve before the fetch gives up.
const MaxLookupTime = 5 * time.Second

// MaxHostLength is the longest a host may be, which is the length the naming
// standards allow, because a longer one is not a name anybody could register.
const MaxHostLength = 253

// refusedRange is one range of addresses the agent never reaches, together with
// the plain reason it never reaches it.
type refusedRange struct {
	// prefix is the range itself.
	prefix netip.Prefix
	// reason is the end of the sentence "the address 10.0.0.1 ...".
	reason string
}

// The reasons, written once because several ranges share one.
const (
	reasonPrivate              = "is on a private network, and the agent only fetches pages from the public web"
	reasonThisMachine          = "is on this machine, and the agent only fetches pages from outside it"
	reasonStandsForThisMachine = "stands for this machine rather than for a computer out on the public web"
	reasonLinkLocal            = "is a link-local one, and the agent only fetches pages from the public web"
	reasonMulticast            = "is a multicast one, and a page is fetched from one machine"
	reasonProtocols            = "is kept aside for the internet protocols themselves, so no page is served there"
	reasonDocuments            = "is kept aside for writing documentation, so no page is served there"
	reasonTunnel               = "carries an IPv4 address inside an IPv6 one, and that address may be anywhere at all"
)

// refusedRanges is every range of addresses that is not the ordinary public
// web. Go's own IsPrivate knows four of these and no more, which is why this is
// a table rather than a call: the shared address space in the middle of it is
// where a tailnet lives, and every machine on the user's tailnet would otherwise
// be one fetch away. The narrower ranges come first, because the first range
// that holds an address is the one whose reason the person reads.
var refusedRanges = []refusedRange{
	{netip.MustParsePrefix("169.254.169.254/32"), "is where a cloud machine keeps its own credentials, so the agent never reads it"},
	{netip.MustParsePrefix("0.0.0.0/8"), reasonStandsForThisMachine},
	{netip.MustParsePrefix("10.0.0.0/8"), reasonPrivate},
	{netip.MustParsePrefix("100.64.0.0/10"), "is on the shared address space that carriers and private networks such as a tailnet use, and the agent only fetches pages from the public web"},
	{netip.MustParsePrefix("127.0.0.0/8"), reasonThisMachine},
	{netip.MustParsePrefix("169.254.0.0/16"), reasonLinkLocal},
	{netip.MustParsePrefix("172.16.0.0/12"), reasonPrivate},
	{netip.MustParsePrefix("192.0.0.0/24"), reasonProtocols},
	{netip.MustParsePrefix("192.0.2.0/24"), reasonDocuments},
	{netip.MustParsePrefix("192.168.0.0/16"), reasonPrivate},
	{netip.MustParsePrefix("198.18.0.0/15"), "is kept aside for testing one network against another, so no page is served there"},
	{netip.MustParsePrefix("198.51.100.0/24"), reasonDocuments},
	{netip.MustParsePrefix("203.0.113.0/24"), reasonDocuments},
	{netip.MustParsePrefix("224.0.0.0/4"), reasonMulticast},
	{netip.MustParsePrefix("240.0.0.0/4"), "is kept aside for whatever comes next, so no page is served there"},
	{netip.MustParsePrefix("::/128"), reasonStandsForThisMachine},
	{netip.MustParsePrefix("::1/128"), reasonThisMachine},
	{netip.MustParsePrefix("::/96"), reasonTunnel},
	{netip.MustParsePrefix("64:ff9b::/96"), reasonTunnel},
	{netip.MustParsePrefix("64:ff9b:1::/48"), reasonTunnel},
	{netip.MustParsePrefix("100::/64"), "is one the network is meant to throw away, so no page is served there"},
	{netip.MustParsePrefix("2001::/23"), reasonProtocols},
	{netip.MustParsePrefix("2001:db8::/32"), reasonDocuments},
	{netip.MustParsePrefix("2002::/16"), reasonTunnel},
	{netip.MustParsePrefix("fc00::/7"), reasonPrivate},
	{netip.MustParsePrefix("fe80::/10"), reasonLinkLocal},
	{netip.MustParsePrefix("ff00::/8"), reasonMulticast},
}

// CheckAddressAllowed says whether the agent may reach an address: it must be an
// ordinary web address, and every number its name resolves to must be a public
// one, unless the settings name that exact number and port because it is a
// server of the user's own.
func CheckAddressAllowed(address string, allowedHosts []string) error {
	_, err := PinnedAddress(address, allowedHosts)
	return err
}

// PinnedAddress resolves the host of an address once and returns the exact host
// and port to connect to, so that the connection cannot be sent anywhere the
// check did not look at.
func PinnedAddress(address string, allowedHosts []string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return "", fmt.Errorf("cannot read %q as a web address, so write one beginning with https://: %w", address, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("the address %q is not a web address, so give one beginning with http:// or https://", address)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("the address %q names no host, so write the whole web address", address)
	}

	port := parsed.Port()
	if port == "" {
		port = defaultPortFor(parsed.Scheme)
	}
	hostAndPort := net.JoinHostPort(host, port)
	namedBySettings := allowed(hostAndPort, allowedHosts)

	// A host written as a number is checked as it stands, with no lookup, and
	// only such a number may be reached on the strength of the settings alone,
	// because a number is a choice the user made and a name is a choice whoever
	// answers for it makes.
	if written, err := netip.ParseAddr(host); err == nil {
		if namedBySettings {
			return hostAndPort, nil
		}
		if err := checkPublic(written); err != nil {
			return "", err
		}
		if err := checkPort(hostAndPort, port); err != nil {
			return "", err
		}
		return hostAndPort, nil
	}
	if err := checkName(host); err != nil {
		return "", err
	}
	pinned, err := pinOnePublicAddress(host, port)
	if err != nil {
		return "", err
	}
	if !namedBySettings {
		if err := checkPort(hostAndPort, port); err != nil {
			return "", err
		}
	}
	return pinned, nil
}

// defaultPortFor is the port a scheme is served on when the address leaves the
// port out.
func defaultPortFor(scheme string) string {
	return map[string]string{"http": "80", "https": "443"}[scheme]
}

// webPorts are the two doors a web page is served behind.
var webPorts = map[string]bool{"80": true, "443": true}

// checkPort refuses a port that is not a web port, because what answers on any
// other port is a service rather than a page: the model asking for port 22 or
// for the port a local model listens on is the agent being pointed at somebody's
// machinery. The settings may name a host and a port together to allow one.
func checkPort(hostAndPort string, port string) error {
	if webPorts[port] {
		return nil
	}
	return fmt.Errorf("the address is on port %s, and a page is served on port 80 or 443, so write %q on the allowed hosts in the settings if the agent is meant to reach that port",
		port, hostAndPort)
}

// allowed says whether the settings named this exact host and port as one the
// agent may reach whatever the address table says, which is how a search server
// of the user's own on this machine stays reachable. The whole host and port
// must match: a bare name on the list would be allowed on every port, and a
// whole machine is not what the settings meant to name.
func allowed(hostAndPort string, allowedHosts []string) bool {
	for _, named := range allowedHosts {
		if strings.TrimSpace(named) == hostAndPort {
			return true
		}
	}
	return false
}

// checkName refuses a host that is not an ordinary name. A host written in
// octal, in hexadecimal, or as one long integer is an address in disguise: the
// address reader above does not read it, but the machine's own name lookup does,
// and it turns 0x7f000001 back into the loopback address. Refusing it here means
// the guard reads the address the same way the connection will.
func checkName(host string) error {
	if len(host) > MaxHostLength {
		return fmt.Errorf("the host is %d characters and the longest a name may be is %d, so check the address",
			len(host), MaxHostLength)
	}
	for _, letter := range host {
		if unicode.IsLetter(letter) || unicode.IsDigit(letter) || strings.ContainsRune("-_.", letter) {
			continue
		}
		return fmt.Errorf("the host %q holds %q, which no name holds, so write the name or the address of the server plainly",
			host, letter)
	}
	if isAddressInDisguise(host) {
		return fmt.Errorf("the host %q is an address written in another base rather than a name, so write it the ordinary way, as four numbers with dots between them",
			host)
	}
	return nil
}

// isAddressInDisguise says whether a host is really an address written in
// another base. The machine's own name lookup reads up to four parts, each in
// decimal, in octal with a nought in front of it, or in hexadecimal with 0x in
// front of it, so 0177.0.0.1, 0x7f000001 and 2130706433 all reach the loopback
// address without ever being read as a name.
func isAddressInDisguise(host string) bool {
	parts := strings.Split(strings.TrimSuffix(host, "."), ".")
	if len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if !isNumberPart(part) {
			return false
		}
	}
	return true
}

// isNumberPart says whether one part of a host is a number in any of the three
// bases a name lookup reads.
func isNumberPart(part string) bool {
	digits, digitsAllowed := part, "0123456789"
	switch {
	case len(part) > 2 && (strings.HasPrefix(part, "0x") || strings.HasPrefix(part, "0X")):
		digits, digitsAllowed = part[2:], "0123456789abcdefABCDEF"
	case len(part) > 1 && strings.HasPrefix(part, "0"):
		digits, digitsAllowed = part[1:], "01234567"
	}
	if digits == "" {
		return false
	}
	for _, letter := range digits {
		if !strings.ContainsRune(digitsAllowed, letter) {
			return false
		}
	}
	return true
}

// pinOnePublicAddress resolves a name and returns the first public number it
// answers with, refusing the whole address when any of them is not public,
// because a name that answers with one of each is a name being used to get
// inside.
func pinOnePublicAddress(host string, port string) (string, error) {
	ctx, stopLooking := context.WithTimeout(context.Background(), MaxLookupTime)
	defer stopLooking()

	found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return "", fmt.Errorf("cannot find out what %q stands for, so check the address: %w", host, err)
	}
	if len(found) == 0 {
		return "", fmt.Errorf("the host %q stands for no address at all, so check it", host)
	}
	for _, one := range found {
		if err := checkPublic(one); err != nil {
			return "", err
		}
	}
	return net.JoinHostPort(found[0].Unmap().String(), port), nil
}

// checkPublic says no to every number the agent must not reach, by looking the
// number up in the table of ranges that are not the public web.
func checkPublic(address netip.Addr) error {
	if !address.IsValid() {
		return errors.New("this address is not a number at all, so check the address")
	}
	// An IPv4 address written as an IPv6 one, such as ::ffff:127.0.0.1, is the
	// same machine as the IPv4 address inside it, and a name lookup hands back
	// that form for every IPv4 address, so it is unwrapped before the table. A
	// zone goes with it, because a range holds no zone.
	plain := address.Unmap().WithZone("")
	for _, refused := range refusedRanges {
		if refused.prefix.Contains(plain) {
			return fmt.Errorf("the address %s %s", address, refused.reason)
		}
	}
	return nil
}
