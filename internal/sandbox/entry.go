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
// ruleset and a seccomp filter to that thread, writes one line saying what it
// did, and then becomes the command it was asked to run. Both restrictions
// survive that change of program, so the command starts already fenced in.
//
// It returns only when something went wrong, because a helper that worked is no
// longer running.
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

	fmt.Fprintf(progress, "coeus %s: landlock version %d, %d folders readable, %d writable, %d seccomp instructions, running %s\n",
		EntrySubcommandName, version, len(request.readable), len(request.writable), len(filter), request.program)
	return becomeTheCommand(request)
}

// becomeTheCommand replaces this program with the command, keeping the
// restrictions that were just applied and dropping the fence marker so that the
// command never sees it.
func becomeTheCommand(request entryRequest) error {
	whole := append([]string{request.program}, request.arguments...)
	if err := syscall.Exec(request.program, whole, environmentWithoutMarker()); err != nil {
		return fmt.Errorf("the sandbox cannot start %q inside the fence, so check that the program is in a folder the fence allows: %w", request.program, err)
	}
	return nil
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
