package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JaredTate/coeus/internal/contract"
)

// bubblewrapProgram is the program that builds the outer half of the fence. It
// is named without a path, so that the machine's own PATH finds it.
const bubblewrapProgram = "bwrap"

// defaultSystemFolders are the folders bound read-only inside the fence, which
// is what a command needs to find a shell, its libraries, and the settings that
// let it look a hostname up. A machine that lacks one of them simply does not
// get it bound.
var defaultSystemFolders = []string{"/usr", "/bin", "/lib", "/lib64", "/etc"}

// Settings say what one fence may reach and how much of a command's output it
// keeps.
type Settings struct {
	// Roots are the folders a sandboxed command may read and write. They come
	// from the configuration's sandbox roots, and none of them may be, or hold,
	// the agent's home folder, the vault, the browser profile, or the SSH keys.
	Roots []string
	// UserHome is the user's own home directory, which is what the forbidden
	// paths are worked out from.
	UserHome string
	// OutputCap is the most bytes kept from each of a command's two output
	// streams. Zero means the tool output cap from the configuration's defaults.
	OutputCap int
	// HelperProgram is the program the fence starts first, which applies
	// Landlock and seccomp and then becomes the command. Empty means this
	// program, which is what production uses, because the helper is a subcommand
	// of coeus itself.
	HelperProgram string
}

// Fence runs commands inside bwrap with a Landlock ruleset and a seccomp filter.
// It implements contract.Sandbox.
type Fence struct {
	roots         []string
	systemFolders []string
	userHome      string
	outputCap     int
	helperProgram string
}

// New checks the settings and returns a fence, or an error saying which setting
// cannot be allowed and what to do about it. Everything that can be refused is
// refused here, once, rather than on every command.
func New(settings Settings) (*Fence, error) {
	roots, err := checkRoots(settings.Roots, settings.UserHome)
	if err != nil {
		return nil, err
	}
	helperProgram, err := findHelperProgram(settings.HelperProgram)
	if err != nil {
		return nil, err
	}

	outputCap := settings.OutputCap
	if outputCap <= 0 {
		outputCap = contract.DefaultConfig().Caps.ToolOutputBytes
	}

	return &Fence{
		roots:         roots,
		systemFolders: foldersThatExist(defaultSystemFolders),
		userHome:      settings.UserHome,
		outputCap:     outputCap,
		helperProgram: helperProgram,
	}, nil
}

// findHelperProgram returns the full path of the program the fence starts inside
// itself, falling back to this program when the caller named none.
func findHelperProgram(asked string) (string, error) {
	if asked == "" {
		thisProgram, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("the sandbox cannot find this program on disk to start inside the fence, so run coeus from a real file: %w", err)
		}
		return thisProgram, nil
	}
	if !filepath.IsAbs(asked) {
		return "", fmt.Errorf("the sandbox helper %q is not a full path, and the fence has a PATH of its own, so give the helper's whole path", asked)
	}
	if _, err := os.Stat(asked); err != nil {
		return "", fmt.Errorf("the sandbox helper %q cannot be read, so point at the coeus binary: %w", asked, err)
	}
	return asked, nil
}

// foldersThatExist keeps the folders that are on this machine, because binding a
// folder that is not there stops bwrap before it starts.
func foldersThatExist(folders []string) []string {
	found := make([]string, 0, len(folders))
	for _, folder := range folders {
		if details, err := os.Stat(folder); err == nil && details.IsDir() {
			found = append(found, folder)
		}
	}
	return found
}
