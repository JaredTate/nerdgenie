//go:build integration

package sandbox

// What the filter does to the two ways a command can make a new user namespace,
// and what it must not do to the ordinary fork every shell depends on. These
// tests run inside a real fence with the real seccomp filter installed.

import (
	"strings"
	"testing"
)

func TestANewUserNamespaceIsRefusedInsideTheFence(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	said := insideTheFence(t, fence, "unshare --user /bin/true && echo 'a new user namespace was made'")

	if strings.Contains(said, "a new user namespace was made") {
		t.Errorf("a command inside the fence made a new user namespace and said %q, and the filter is there to refuse one", said)
	}
	if !strings.Contains(said, "Operation not permitted") {
		t.Errorf("unshare inside the fence said %q, want the filter's own refusal; a test that passes because the program is missing proves nothing", said)
	}
}

func TestAShellInsideTheFenceCanStillStartOtherPrograms(t *testing.T) {
	fence, _, _ := aRealFence(t, theToolOutputCap)

	// Every fork on this machine's C library asks for clone3 first and falls
	// back to clone when the kernel says there is no such call, so a filter that
	// refused clone3 any other way would stop a shell starting anything at all.
	said := insideTheFence(t, fence, "echo one | cat | tr a-z A-Z")

	if !strings.Contains(said, "ONE") {
		t.Errorf("a pipeline of three programs inside the fence said %q, want ONE; the filter's answer to clone3 has to be "+
			"the one the C library falls back from", said)
	}
}
