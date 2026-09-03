//go:build integration

package sandbox

// Whether a name can be looked up inside the fence. On Ubuntu with
// systemd-resolved, /etc/resolv.conf is a link into /run, which the fence does
// not bind, so binding /etc alone leaves the link dangling and every lookup
// fails. These tests run against the real bwrap and this machine's own resolver.

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// theNameToLookUp is a name that exists for as long as the internet does and
// belongs to nobody who minds being asked about it.
const theNameToLookUp = "example.com"

func TestANameResolvesInsideTheFenceWhenItHasTheNetwork(t *testing.T) {
	if said, err := outsideTheFence("getent hosts " + theNameToLookUp); err != nil {
		t.Skipf("this machine cannot look %s up itself, so the fence cannot be measured against it: %v (it said %q)", theNameToLookUp, err, said)
	}
	fence, _, _ := aRealFenceWithTheNetwork(t, theToolOutputCap)

	said := insideTheFence(t, fence, "getent hosts "+theNameToLookUp+" || echo 'no name resolution inside the fence'")

	if !strings.Contains(said, theNameToLookUp) {
		t.Errorf("looking %s up inside the fence said %q; this machine keeps its resolver settings in a file that /etc/resolv.conf only links to, "+
			"so the fence has to bind that file as well as /etc", theNameToLookUp, said)
	}
}

func TestTheResolverSettingsInsideTheFenceAreThisMachinesOwn(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	said := insideTheFence(t, fence, "cat /etc/resolv.conf || echo 'the resolver settings are not there'")

	if !strings.Contains(said, "nameserver") {
		t.Errorf("the resolver settings inside the fence read %q, want the nameserver lines this machine uses", said)
	}
}

// outsideTheFence asks this machine the same question the fence is asked, so
// that a machine with no name resolution at all is told apart from a fence that
// broke it.
func outsideTheFence(question string) (string, error) {
	ctx, stopWaiting := context.WithTimeout(context.Background(), 20*time.Second)
	defer stopWaiting()

	said, err := exec.CommandContext(ctx, "/bin/sh", "-c", question).Output()
	return strings.TrimSpace(string(said)), err
}
