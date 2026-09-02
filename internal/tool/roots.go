package tool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxLinkHops is how many links the check will follow before giving up. A folder
// that points at itself is a loop, and a loop must not become a hang.
const MaxLinkHops = 40

// PathCheck says whether the file tools may touch a path, and returns the path
// with every link along it resolved. The four file tools are each given one, so
// that the rule about where the agent may read and write lives in one place.
type PathCheck func(path string) (string, error)

// NewPathCheck returns the check the file tools are given: a path is allowed
// when it is a whole path that sits inside one of the sandbox roots once its
// links are followed, and outside everything that must stay outside the fence,
// which is the agent's home folder, the vault, the browser profiles, and the
// user's SSH keys.
func NewPathCheck(roots []string, userHome string) PathCheck {
	cleanRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) != "" {
			cleanRoots = append(cleanRoots, resolveLinks(filepath.Clean(root)))
		}
	}
	excluded := contract.ExcludedFromSandbox(userHome)

	return func(path string) (string, error) {
		wanted, err := wholePath(path)
		if err != nil {
			return "", err
		}
		if len(cleanRoots) == 0 {
			return "", fmt.Errorf("no folder is configured for the agent to work in, so add one to sandbox_roots in config.toml before asking for %s", wanted)
		}
		resolved := resolveLinks(wanted)
		if err := insideOne(resolved, cleanRoots); err != nil {
			return "", err
		}
		if err := outsideEvery(resolved, excluded); err != nil {
			return "", err
		}
		return wanted, nil
	}
}

// wholePath returns the tidied form of a path the model wrote, and refuses
// anything a tool with no working directory could not read.
func wholePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("this call names no file at all, so write the whole path of the file to work on")
	}
	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("the path %q does not start at the root of the filesystem, and a tool has no working directory to read it against, so write the whole path", trimmed)
	}
	return filepath.Clean(trimmed), nil
}

// insideOne says no to a path that is in none of the folders the agent may work
// in, and names them, because the model cannot see the configuration.
func insideOne(path string, roots []string) error {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("the path %s is outside every folder the agent may work in, which are %s, so work inside one of them",
		path, strings.Join(roots, ", "))
}

// outsideEvery says no to a path that reaches something that must stay outside
// the fence however the roots are configured.
func outsideEvery(path string, excluded []string) error {
	for _, forbidden := range excluded {
		if path == forbidden || strings.HasPrefix(path, forbidden+string(filepath.Separator)) {
			return fmt.Errorf("the path %s is inside %s, which the agent never reads or writes through a tool, so leave it alone", path, forbidden)
		}
	}
	return nil
}

// resolveLinks follows the links along a path as far as the parts of it that
// exist, so that a link inside a root pointing somewhere else is judged by where
// it lands rather than by where it sits. The parts that do not exist yet are put
// back on the end, because a file about to be written has none.
func resolveLinks(path string) string {
	missing := []string{}
	at := path
	for hop := 0; hop < MaxLinkHops; hop++ {
		resolved, err := filepath.EvalSymlinks(at)
		if err == nil {
			return filepath.Join(append([]string{resolved}, reversed(missing)...)...)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return path
		}
		above := filepath.Dir(at)
		if above == at {
			return path
		}
		missing = append(missing, filepath.Base(at))
		at = above
	}
	return path
}

// reversed returns the parts of a path back in the order they were written,
// because they were collected from the end towards the front.
func reversed(parts []string) []string {
	back := make([]string, 0, len(parts))
	for at := len(parts) - 1; at >= 0; at-- {
		back = append(back, parts[at])
	}
	return back
}
