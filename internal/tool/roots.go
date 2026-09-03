package tool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxLinkHops is how many links the check will follow before giving up. A folder
// that points at itself is a loop, and a loop must not become a hang.
const MaxLinkHops = 40

// PathCheck says whether the file tools may touch a path, and returns the path
// with every link along it resolved. The four file tools are each given one, so
// that the rule about where the agent may read and write lives in one place.
type PathCheck func(path string) (string, error)

// WorkArea is everything the check needs to judge a path: the folders the agent
// may read and write in, the folder a path that does not start at the root of
// the filesystem is taken from, and the two homes the fence is worked out from.
type WorkArea struct {
	// Roots are the folders the agent may read and write in, which is
	// sandbox_roots in config.toml.
	Roots []string
	// WorkingFolder is where a path that does not start at the root of the
	// filesystem is taken from, which is the folder the agent works in. Empty
	// means such a path is refused, because a check with nowhere to take it from
	// cannot tell which file was meant.
	WorkingFolder string
	// UserHome is the user's own home directory, which is what the paths that
	// must stay outside the fence are worked out from and what a path beginning
	// with ~ or $HOME is read as.
	UserHome string
	// AgentHome is the agent's home folder, which is passed in as well as the
	// user's because COEUS_HOME may have moved it anywhere.
	AgentHome string
	// AlsoOutside are the folders the configuration can move anywhere in the
	// same way and which stay outside the fence wherever they land: the browser
	// profile and the backup folder. Without them the model could read the
	// cookies that are the agent's logins with the read tool, whenever the user
	// had put the profile inside a folder the agent works in.
	AlsoOutside []string
}

// NewPathCheck returns the check the file tools are given: a path is allowed
// when it sits inside one of the sandbox roots once its links are followed, and
// outside everything that must stay outside the fence, which is the agent's home
// folder, the vault, the browser profiles, and the user's SSH keys. Before any of
// that is judged the path is made a whole one: a path beginning with ~ or $HOME
// is read as the user's home folder, and a path that starts at neither the root
// of the filesystem nor the home is taken from the folder the agent works in,
// because every model writes such a path. Either is refused only when it then
// lands outside the roots, and the refusal says how the path was read.
func NewPathCheck(area WorkArea) PathCheck {
	cleanRoots := make([]string, 0, len(area.Roots))
	for _, root := range area.Roots {
		if strings.TrimSpace(root) != "" {
			cleanRoots = append(cleanRoots, resolveLinks(filepath.Clean(root)))
		}
	}
	excluded := contract.ExcludedFromSandbox(area.UserHome, area.AgentHome, area.AlsoOutside...)
	workingFolder := strings.TrimSpace(area.WorkingFolder)

	return func(path string) (string, error) {
		wanted, howItWasRead, err := wholePath(path, workingFolder, strings.TrimSpace(area.UserHome))
		if err != nil {
			return "", err
		}
		if len(cleanRoots) == 0 {
			return "", fmt.Errorf("no folder is configured for the agent to work in, so add one to sandbox_roots in config.toml before asking for %s", wanted)
		}
		resolved := resolveLinks(wanted)
		if err := insideTheFence(resolved, cleanRoots, excluded); err != nil {
			return "", alsoSayHowItWasRead(err, howItWasRead)
		}
		return resolved, nil
	}
}

// insideTheFence runs the three rules a path has to pass once it is a whole path
// with its links followed.
func insideTheFence(resolved string, roots []string, excluded []string) error {
	if err := insideOne(resolved, roots); err != nil {
		return err
	}
	if err := outsideEvery(resolved, excluded); err != nil {
		return err
	}
	return hasOneNameOnly(resolved)
}

// alsoSayHowItWasRead adds to a refusal the one clause saying how a path that
// was not written as a whole path was read, so that a model which wrote a bare
// name such as haiku.txt, or a path beginning with ~, can see where the name
// landed and why that was refused. A path that was already a whole one was read
// as it stood and gets no clause.
func alsoSayHowItWasRead(err error, howItWasRead string) error {
	if howItWasRead == "" {
		return err
	}
	return fmt.Errorf("%w; the path was read this way before it was judged: %s", err, howItWasRead)
}

// hasOneNameOnly says no to a file that is known by more than one name on the
// disk. A hard link has no target for the check above to follow, so a link made
// inside a root before the agent ever ran would otherwise let a tool read and
// write a file that sits anywhere at all, the vault and the user's keys
// included. Folders are passed over, because a folder always has more than one
// name, and a file that is not there yet has none.
func hasOneNameOnly(path string) error {
	about, err := os.Stat(path)
	if err != nil || !about.Mode().IsRegular() {
		return nil
	}
	names, known := about.Sys().(*syscall.Stat_t)
	if !known || names.Nlink <= 1 {
		return nil
	}
	return fmt.Errorf("the file %s is known by %d names on this disk, and the others may be outside the folders the agent may work in, so copy it to a file of its own and work on the copy",
		path, names.Nlink)
}

// homeMarks are the ways a model writes the user's home folder at the front of a
// path. A model that has seen a shell writes any of them, and all three mean the
// same folder.
var homeMarks = []string{"~", "$HOME", "${HOME}"}

// wholePath returns the tidied whole path of what the model wrote, together with
// one clause saying how it was read when it was not written as a whole path, so
// that a refusal can say so. A path beginning with a mark for the home folder is
// read as the user's home, and a path that starts at neither the root of the
// filesystem nor the home is taken from the folder the agent works in.
func wholePath(path string, workingFolder string, userHome string) (string, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", "", errors.New("this call names no file at all, so write the path of the file to work on")
	}
	if underHome, isUnderHome := underTheHome(trimmed, userHome); isUnderHome {
		return underHome, fmt.Sprintf("the path %q begins with ~ or $HOME, which is read as the user's home folder, %s", trimmed, userHome), nil
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed), "", nil
	}
	if workingFolder == "" {
		return "", "", fmt.Errorf("the path %q does not start at the root of the filesystem, and this tool has no folder to take it from, so write the whole path", trimmed)
	}
	return filepath.Join(workingFolder, trimmed),
		fmt.Sprintf("the path %q does not start at the root of the filesystem, so it was taken from %s, the folder the agent works in", trimmed, workingFolder),
		nil
}

// underTheHome turns a path that begins with a mark for the home folder into a
// whole path under that home, and says whether it was one. A check that was
// given no home folder leaves such a path alone, because a home nobody named
// cannot be put in front of it.
func underTheHome(path string, userHome string) (string, bool) {
	if userHome == "" {
		return "", false
	}
	for _, mark := range homeMarks {
		if path == mark {
			return filepath.Clean(userHome), true
		}
		if rest, found := strings.CutPrefix(path, mark+"/"); found {
			return filepath.Join(userHome, rest), true
		}
	}
	return "", false
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
