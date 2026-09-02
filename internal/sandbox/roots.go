// The rule about which folders may be inside the fence is borrowed from Codex's
// sandbox policy, kept in this repository at docs/reference/codex/landlock.rs: a
// short list of writable roots, and everything else read-only or simply absent.

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxRoots is the most sandbox roots one fence may have. A fence with a long
// list of roots is a fence with nothing outside it, which is no fence at all.
const MaxRoots = 16

// checkRoots returns the cleaned sandbox roots, or an error naming the first
// root that cannot be allowed and saying what to do about it.
//
// The configuration package refuses a bad root as well. This package refuses it
// again, because it is the last thing standing between a bad root and a command
// that can read the vault.
func checkRoots(roots []string, userHome string) ([]string, error) {
	if userHome == "" {
		return nil, errors.New("the sandbox was given no home directory to work the forbidden paths out from, so pass the user's home directory")
	}
	if len(roots) == 0 {
		return nil, errors.New("the sandbox was given no roots, so name at least one folder a command may work in")
	}
	if len(roots) > MaxRoots {
		return nil, fmt.Errorf("the sandbox was given %d roots and the most it allows is %d, so name fewer folders", len(roots), MaxRoots)
	}

	checked := make([]string, 0, len(roots))
	for _, root := range roots {
		clean, err := checkOneRoot(root, userHome)
		if err != nil {
			return nil, err
		}
		checked = append(checked, clean)
	}
	return checked, nil
}

// checkOneRoot holds the four rules one root has to keep: it is a full path, it
// is not one of the paths that must stay outside the fence, it does not hold one
// of them, and it is a folder that is really there.
func checkOneRoot(root string, userHome string) (string, error) {
	if err := contract.CheckSandboxRoot(root, userHome); err != nil {
		return "", err
	}
	clean := filepath.Clean(root)
	if err := checkRootHoldsNothingForbidden(clean, userHome); err != nil {
		return "", err
	}

	details, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("the sandbox root %q cannot be read, so make that folder or name one that is already there: %w", clean, err)
	}
	if !details.IsDir() {
		return "", fmt.Errorf("the sandbox root %q is a file rather than a folder, so name the folder a command may work in", clean)
	}
	return clean, nil
}

// checkRootHoldsNothingForbidden refuses a root that holds one of the paths that
// must stay outside the fence. The user's whole home directory is the root this
// catches most often, because it holds the agent's own home folder and the SSH
// keys.
func checkRootHoldsNothingForbidden(clean string, userHome string) error {
	within := clean
	if !strings.HasSuffix(within, string(filepath.Separator)) {
		within += string(filepath.Separator)
	}
	for _, forbidden := range contract.ExcludedFromSandbox(userHome) {
		if strings.HasPrefix(forbidden, within) {
			return fmt.Errorf("the sandbox root %q holds %q, which must stay outside the sandbox, so name a folder that does not contain it", clean, forbidden)
		}
	}
	return nil
}
