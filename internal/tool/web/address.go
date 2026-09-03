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
	"net/url"
	"strings"
	"time"
)

// MaxLookupTime is how long a name has to resolve before the fetch gives up.
const MaxLookupTime = 5 * time.Second

// cloudCredentialAddress is the address a cloud machine keeps its own
// credentials behind. It is a public number and it is nobody's business but the
// machine's, so it is refused by name as well as by range.
const cloudCredentialAddress = "169.254.169.254"

// CheckAddressAllowed says whether the agent may reach an address: it must be an
// ordinary web address, and every number its name resolves to must be a public
// one, unless the settings allow that host by name because it is a server of the
// user's own.
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
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("the address %q names no host, so write the whole web address", address)
	}

	port := parsed.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[parsed.Scheme]
	}
	hostAndPort := net.JoinHostPort(parsed.Hostname(), port)
	if allowed(hostAndPort, parsed.Hostname(), allowedHosts) {
		return hostAndPort, nil
	}
	return pinOnePublicAddress(parsed.Hostname(), port)
}

// allowed says whether the settings named this host as one the agent may reach
// whatever its number is, which is how a search server or a test server of the
// user's own on this machine stays reachable.
func allowed(hostAndPort string, host string, allowedHosts []string) bool {
	for _, named := range allowedHosts {
		if named == hostAndPort || named == host {
			return true
		}
	}
	return false
}

// pinOnePublicAddress resolves a name and returns the first public number it
// answers with, refusing the whole address when any of them is private, because
// a name that answers with one of each is a name being used to get inside.
func pinOnePublicAddress(host string, port string) (string, error) {
	ctx, stopLooking := context.WithTimeout(context.Background(), MaxLookupTime)
	defer stopLooking()

	found, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", fmt.Errorf("cannot find out what %q stands for, so check the address: %w", host, err)
	}
	if len(found) == 0 {
		return "", fmt.Errorf("the host %q stands for no address at all, so check it", host)
	}
	for _, one := range found {
		if err := checkPublic(one.IP); err != nil {
			return "", err
		}
	}
	return net.JoinHostPort(found[0].IP.String(), port), nil
}

// checkPublic says no to every number the agent must not reach: this machine
// itself, the private ranges, the link-local range and the address a cloud
// machine keeps its credentials behind, and the ranges that are nobody's.
func checkPublic(address net.IP) error {
	switch {
	case address == nil:
		return errors.New("this address is not a number at all, so check the address")
	case address.String() == cloudCredentialAddress:
		return fmt.Errorf("the address %s is where a cloud machine keeps its own credentials, so the agent never reads it", address)
	case address.IsLoopback():
		return fmt.Errorf("the address %s is on this machine, and the agent only fetches pages from outside it", address)
	case address.IsUnspecified():
		return fmt.Errorf("the address %s stands for this machine, and the agent only fetches pages from outside it", address)
	case address.IsPrivate():
		return fmt.Errorf("the address %s is on a private network, and the agent only fetches pages from the public web", address)
	case address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast():
		return fmt.Errorf("the address %s is a link-local one, and the agent only fetches pages from the public web", address)
	case address.IsMulticast() || address.IsInterfaceLocalMulticast():
		return fmt.Errorf("the address %s is a multicast one, and a page is fetched from one machine", address)
	default:
		return nil
	}
}
