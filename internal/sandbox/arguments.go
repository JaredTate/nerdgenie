// The shape of the bwrap command line is borrowed from ZeroClaw's bubblewrap
// backend at ~/Code/zeroclaw/crates/zeroclaw-runtime/src/security/bubblewrap.rs,
// and the choice of which folders are read-only and which are writable, and of
// leaving the network alone, is borrowed from Codex's sandbox policy kept in
// this repository at docs/reference/codex/landlock.rs.

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The bounds on one command. Every one of them is a hard stop, not a target.
const (
	// MaxArguments is the most arguments one sandboxed command may have.
	MaxArguments = 512
	// MaxEnvironmentEntries is the most environment entries a caller may add.
	MaxEnvironmentEntries = 64
)

// sandboxPath is the PATH a sandboxed command gets. It names only the folders
// bound read-only inside the fence, because nothing else is there to run.
const sandboxPath = "/usr/local/bin:/usr/bin:/bin"

// scratchHomeName is the folder inside the first sandbox root that a command
// gets as its home directory. The user's real home directory is not inside the
// fence, and a command that writes to its home has to write somewhere.
const scratchHomeName = ".coeus-sandbox-home"

// The words the fence uses to tell the helper what the command may reach.
const (
	// readableOption names a folder the command may read and run programs from.
	readableOption = "--read"
	// writableOption names a folder the command may read, write, and run
	// programs from.
	writableOption = "--write"
	// optionsEndMarker is the double dash that ends a list of options and starts
	// the command itself.
	optionsEndMarker = "--"
)

// freshFolders are the folders bwrap makes new inside the fence rather than
// binding from the machine. They hold nothing of the user's, and a command that
// cannot write a temporary file gets very little done, so they are writable.
var freshFolders = []string{"/tmp", "/dev"}

// fencePlan is everything one run of the fence is built from, worked out once so
// that the command line and the helper's options cannot disagree.
type fencePlan struct {
	systemFolders    []string
	roots            []string
	helperProgram    string
	workingDirectory string
	environment      []string
	program          string
	arguments        []string
}

// planFor works one command into a plan, refusing anything the fence cannot
// allow. It touches nothing on disk, so that it is cheap enough to run before
// every command.
func (fence *Fence) planFor(command contract.SandboxCommand) (fencePlan, error) {
	if err := checkCommand(command); err != nil {
		return fencePlan{}, err
	}
	workingDirectory, err := fence.workingDirectoryFor(command.WorkingDirectory)
	if err != nil {
		return fencePlan{}, err
	}

	scratchHome := filepath.Join(fence.roots[0], scratchHomeName)
	return fencePlan{
		systemFolders:    fence.systemFolders,
		roots:            fence.roots,
		helperProgram:    fence.helperProgram,
		workingDirectory: workingDirectory,
		environment:      allowedEnvironment(scratchHome, os.Getenv("LANG"), os.Getenv("TERM"), command.Environment),
		program:          command.Program,
		arguments:        command.Arguments,
	}, nil
}

// buildArguments turns a plan into the arguments bwrap is given. The order
// matters: bwrap carries each one out as it reads it, so the fresh folders are
// mounted before anything is bound on top of them.
func buildArguments(plan fencePlan) []string {
	arguments := []string{
		"--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--die-with-parent", "--new-session", "--clearenv",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
	}
	for _, folder := range plan.systemFolders {
		arguments = append(arguments, "--ro-bind", folder, folder)
	}
	for _, root := range plan.roots {
		arguments = append(arguments, "--bind", root, root)
	}
	arguments = append(arguments, "--ro-bind", plan.helperProgram, plan.helperProgram)
	for _, entry := range plan.environment {
		name, value, _ := strings.Cut(entry, "=")
		arguments = append(arguments, "--setenv", name, value)
	}

	arguments = append(arguments, "--setenv", FenceMarkerVariable, fenceMarkerValue)

	arguments = append(arguments, "--chdir", plan.workingDirectory, optionsEndMarker)
	arguments = append(arguments, plan.helperProgram, EntrySubcommandName)
	arguments = append(arguments, helperOptions(plan)...)
	arguments = append(arguments, optionsEndMarker, plan.program)
	return append(arguments, plan.arguments...)
}

// helperOptions are what the fence tells the helper it may allow. The helper
// program itself is readable, because the fence starts it from inside.
func helperOptions(plan fencePlan) []string {
	options := []string{}
	for _, folder := range plan.systemFolders {
		options = append(options, readableOption, folder)
	}
	options = append(options, readableOption, plan.helperProgram)
	for _, root := range plan.roots {
		options = append(options, writableOption, root)
	}
	for _, folder := range freshFolders {
		options = append(options, writableOption, folder)
	}
	return options
}

// allowedEnvironment is the whole environment a sandboxed command sees: a short
// allow list, then whatever the caller asked for. Nothing else is inherited, so
// a secret in this program's own environment cannot leak into a command.
func allowedEnvironment(scratchHome string, language string, terminal string, extra []string) []string {
	environment := []string{"PATH=" + sandboxPath, "HOME=" + scratchHome}
	if language != "" {
		environment = append(environment, "LANG="+language)
	}
	if terminal != "" {
		environment = append(environment, "TERM="+terminal)
	}
	return append(environment, extra...)
}

// workingDirectoryFor returns where the command runs, which is the first root
// when the caller named none and has to be inside a root when it did.
func (fence *Fence) workingDirectoryFor(asked string) (string, error) {
	if asked == "" {
		return fence.roots[0], nil
	}
	clean := filepath.Clean(asked)
	for _, root := range fence.roots {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return clean, nil
		}
	}
	return "", fmt.Errorf("the working directory %q is outside every sandbox root, so run the command in one of %v", asked, fence.roots)
}

// checkCommand holds the bounds on one command: it names a program, it is within
// the caps, and none of its text holds a zero byte, which would end the string
// early for the kernel and hide whatever came after it.
func checkCommand(command contract.SandboxCommand) error {
	if strings.TrimSpace(command.Program) == "" {
		return errors.New("the sandbox was asked to run a command with no program in it, so name the program to run")
	}
	if len(command.Arguments) > MaxArguments {
		return fmt.Errorf("the command has %d arguments and the most the sandbox allows is %d, so shorten it", len(command.Arguments), MaxArguments)
	}
	for _, text := range append([]string{command.Program}, command.Arguments...) {
		if strings.ContainsRune(text, 0) {
			return fmt.Errorf("the command holds a zero byte in %q, and a zero byte hides whatever follows it, so take it out", text)
		}
	}
	return checkEnvironment(command.Environment)
}

// checkEnvironment holds the bounds on the environment entries a caller adds.
func checkEnvironment(environment []string) error {
	if len(environment) > MaxEnvironmentEntries {
		return fmt.Errorf("the command has %d environment entries and the most the sandbox allows is %d, so pass fewer", len(environment), MaxEnvironmentEntries)
	}
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if !found || name == "" {
			return fmt.Errorf("the environment entry %q has no name in it, so write it as NAME=value", entry)
		}
		if strings.ContainsRune(entry, 0) {
			return fmt.Errorf("the environment entry %q holds a zero byte, and a zero byte hides whatever follows it, so take it out", entry)
		}
	}
	return nil
}
