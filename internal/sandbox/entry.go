// Applying the restriction in the process that is about to become the command,
// rather than in the program that starts it, is borrowed from ZeroClaw's Landlock
// backend at ~/Code/zeroclaw/crates/zeroclaw-runtime/src/security/landlock.rs,
// which does the same thing between fork and exec.

package sandbox

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// FenceMarkerVariable is the environment variable the fence sets before it
// starts the helper. The helper refuses to run without it.
//
// This is a guard against a person running the helper by hand, not a security
// boundary. The security boundary is the namespaces bwrap makes, the Landlock
// ruleset, and the seccomp filter the helper installs.
const FenceMarkerVariable = "COEUS_INSIDE_SANDBOX"

// fenceMarkerValue is what the fence sets that variable to. Its content does not
// matter; that it is there at all is the whole signal.
const fenceMarkerValue = "1"

// MaxHelperFolders is the most folders the helper will build rules for. It is
// larger than the roots cap, because the system folders are counted too.
const MaxHelperFolders = 64

// entryRequest is what the fence told the helper to do: which folders the
// command may reach, and which command to become.
type entryRequest struct {
	readable  []string
	writable  []string
	program   string
	arguments []string
}

// Entry is the helper the fence starts inside itself. It locks its
// operating-system thread, sets the no-new-privileges flag, applies a Landlock
// ruleset and a seccomp filter to that thread, and then becomes the command it
// was asked to run. Both restrictions survive that change of program, so the
// command starts already fenced in.
//
// It says nothing when all of that works, so that a command's output is the
// command's own. It returns only when something went wrong, because a helper
// that worked is no longer running, and it writes one line saying how far it got
// when the failure was in the last step.
func Entry(arguments []string, progress io.Writer) error {
	request, err := parseEntryArguments(arguments)
	if err != nil {
		return err
	}
	if os.Getenv(FenceMarkerVariable) == "" {
		return fmt.Errorf("coeus %s only runs inside the sandbox that starts it, so run your command through coeus rather than by hand", EntrySubcommandName)
	}

	runtime.LockOSThread()
	if err := setNoNewPrivileges(); err != nil {
		return err
	}
	version, err := applyLandlock(request.readable, request.writable)
	if err != nil {
		return err
	}
	filter := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)
	if err := applySeccomp(filter); err != nil {
		return err
	}

	// Nothing is said on the way through. A line on standard error here would
	// ride out with the command's own output on every single call, and the model
	// would pay to read it every time. It is written only when the helper could
	// not become the command, which is the one moment a reader needs to know how
	// far it got.
	err = becomeTheCommand(request)
	if err != nil {
		fmt.Fprintf(progress, "coeus %s: landlock version %d, %d folders readable, %d writable, %d seccomp instructions, and then it could not start the command\n",
			EntrySubcommandName, version, len(request.readable), len(request.writable), len(filter))
	}
	return err
}

// becomeTheCommand replaces this program with the command, keeping the
// restrictions that were just applied and dropping the fence marker so that the
// command never sees it.
//
// The resource bounds are set last of all, because they bind this program too,
// and its own runtime has to be free to ask the kernel for memory and for
// threads right up to the moment it becomes the command.
func becomeTheCommand(request entryRequest) error {
	program, err := programToBecome(request.program)
	if err != nil {
		return err
	}

	// The command keeps the name it was asked for as its own first argument,
	// which is what a shell would have given it and what it prints in its own
	// messages.
	whole := append([]string{request.program}, request.arguments...)
	environment := environmentWithoutMarker()
	if err := setResourceLimits(syscall.Setrlimit); err != nil {
		return err
	}
	if err := syscall.Exec(program, whole, environment); err != nil {
		return fmt.Errorf("the sandbox cannot start %q inside the fence, so check that the program is in a folder the fence allows: %w", request.program, err)
	}
	return nil
}

// programToBecome turns the name the fence was given into the full path
// syscall.Exec needs, because that call searches no path of its own.
//
// The search runs here, inside the fence and after Landlock, so it looks along
// the PATH the fence set rather than along the one this program was started
// with, and finds only what the fence allows. A name with a slash in it is
// already a path and is only checked.
func programToBecome(asked string) (string, error) {
	found, err := exec.LookPath(asked)
	if err != nil {
		return "", fmt.Errorf("the sandbox cannot find %q inside the fence, so name the program by a full path or by a program on the fence's PATH, which is %s: %w",
			asked, os.Getenv("PATH"), err)
	}
	return found, nil
}

// environmentWithoutMarker is this program's environment with the fence marker
// taken out, which is what the command inherits.
func environmentWithoutMarker() []string {
	kept := []string{}
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, FenceMarkerVariable+"=") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// parseEntryArguments reads the helper's own command line: any number of
// readable and writable folders, then a double dash, then the command.
func parseEntryArguments(arguments []string) (entryRequest, error) {
	request := entryRequest{}
	index := 0
	for ; index < len(arguments) && arguments[index] != optionsEndMarker; index += 2 {
		option := arguments[index]
		if index+1 >= len(arguments) {
			return entryRequest{}, fmt.Errorf("the option %s has no folder after it, so write the folder it allows", option)
		}
		folder := arguments[index+1]
		if !filepath.IsAbs(folder) {
			return entryRequest{}, fmt.Errorf("the folder %q is not a full path, and the helper has no working directory to read it against, so write the whole path", folder)
		}
		switch option {
		case readableOption:
			request.readable = append(request.readable, folder)
		case writableOption:
			request.writable = append(request.writable, folder)
		default:
			return entryRequest{}, fmt.Errorf("the helper does not know the option %s, so pass only %s and %s", option, readableOption, writableOption)
		}
	}
	return finishEntryRequest(request, arguments, index)
}

// finishEntryRequest reads the command that follows the double dash and holds the
// bounds the helper keeps.
func finishEntryRequest(request entryRequest, arguments []string, doubleDash int) (entryRequest, error) {
	if doubleDash >= len(arguments) {
		return entryRequest{}, errors.New("the helper was given no double dash, so end the folders with -- and then name the command")
	}
	command := arguments[doubleDash+1:]
	if len(command) == 0 {
		return entryRequest{}, errors.New("the helper was given no command after the double dash, so name the program to run")
	}
	if len(request.readable)+len(request.writable) == 0 {
		return entryRequest{}, errors.New("the helper was given no folders to allow, and a ruleset that allows nothing would deny the command its own program")
	}
	if len(request.readable)+len(request.writable) > MaxHelperFolders {
		return entryRequest{}, fmt.Errorf("the helper was given %d folders and the most it allows is %d, so pass fewer",
			len(request.readable)+len(request.writable), MaxHelperFolders)
	}

	request.program = command[0]
	request.arguments = command[1:]
	return request, nil
}
