// The rule about which folders may be inside the fence is borrowed from Codex's
// sandbox policy, kept in this repository at docs/reference/codex/landlock.rs: a
// short list of writable roots, and everything else read-only or simply absent.

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// checkOneRoot holds the rules one root has to keep. The first three are the
// contract's: it is a full path, it is not one of the paths that must stay
// outside the fence, and it does not hold one of them. The last is this
// package's own, because only something about to run a command needs it: the
// root is a folder that is really there.
func checkOneRoot(root string, userHome string) (string, error) {
	if err := contract.CheckSandboxRoot(root, userHome); err != nil {
		return "", err
	}
	clean := filepath.Clean(root)

	details, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("the sandbox root %q cannot be read, so make that folder or name one that is already there: %w", clean, err)
	}
	if !details.IsDir() {
		return "", fmt.Errorf("the sandbox root %q is a file rather than a folder, so name the folder a command may work in", clean)
	}
	return clean, nil
}
