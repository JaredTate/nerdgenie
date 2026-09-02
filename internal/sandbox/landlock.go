// The Landlock ruleset, the fall back to the version the kernel reports, and the
// note that the restriction has to be applied by the process that is about to
// become the command, are borrowed from ZeroClaw's Landlock backend at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/security/landlock.rs. Which folders
// are read-only and which are writable is borrowed from Codex's sandbox policy at
// docs/reference/codex/landlock.rs. Both use a library; this is written against
// the three system calls by hand, because the agent takes no dependency it can
// write itself in eighty lines.

package sandbox

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// The three Landlock system calls. The numbers are the same on amd64 and arm64,
// because Landlock arrived after the two architectures stopped disagreeing about
// new system call numbers.
const (
	createRulesetCall = 444
	addRuleCall       = 445
	restrictSelfCall  = 446
)

// versionRequest asks landlock_create_ruleset for the version the kernel speaks
// rather than for a ruleset.
const versionRequest = 1 << 0

// pathBeneathRuleKind is the one kind of rule this package adds: everything
// beneath one folder.
const pathBeneathRuleKind = 1

// pathOnlyOpenFlag opens a folder for its name alone, without asking to read it,
// which is all Landlock needs to hang a rule on.
const pathOnlyOpenFlag = 0x200000

// The access rights Landlock can arbitrate, in the order the kernel numbers them.
const (
	accessExecute             = 1 << 0
	accessWriteFile           = 1 << 1
	accessReadFile            = 1 << 2
	accessReadDirectory       = 1 << 3
	accessRemoveDirectory     = 1 << 4
	accessRemoveFile          = 1 << 5
	accessMakeCharacterDevice = 1 << 6
	accessMakeDirectory       = 1 << 7
	accessMakeRegularFile     = 1 << 8
	accessMakeSocket          = 1 << 9
	accessMakeNamedPipe       = 1 << 10
	accessMakeBlockDevice     = 1 << 11
	accessMakeSymbolicLink    = 1 << 12
	accessMoveBetweenFolders  = 1 << 13
	accessTruncate            = 1 << 14
	accessDeviceControl       = 1 << 15
)

// The rights each Landlock version added, as a mask of everything up to and
// including that version. Handing the kernel a right it has never heard of makes
// it refuse the whole ruleset, so the mask is chosen from what it reports.
const (
	rightsThroughVersionOne   = accessExecute | accessWriteFile | accessReadFile | accessReadDirectory | accessRemoveDirectory | accessRemoveFile | accessMakeCharacterDevice | accessMakeDirectory | accessMakeRegularFile | accessMakeSocket | accessMakeNamedPipe | accessMakeBlockDevice | accessMakeSymbolicLink
	rightsThroughVersionTwo   = rightsThroughVersionOne | accessMoveBetweenFolders
	rightsThroughVersionThree = rightsThroughVersionTwo | accessTruncate
	rightsThroughVersionFive  = rightsThroughVersionThree | accessDeviceControl
)

// The sizes of the two structures the kernel reads. The rule is packed, so it is
// twelve bytes rather than the sixteen Go would lay out on its own.
const (
	rulesetAttributeSize = 8
	pathBeneathRuleSize  = 12
)

// handledAccess returns the rights a ruleset arbitrates on a kernel reporting
// this Landlock version. A version older than the one this package knows falls
// back to version one, which every Landlock kernel has.
func handledAccess(version int) uint64 {
	switch {
	case version >= 5:
		return rightsThroughVersionFive
	case version >= 3:
		return rightsThroughVersionThree
	case version == 2:
		return rightsThroughVersionTwo
	default:
		return rightsThroughVersionOne
	}
}

// readableAccess is what a folder bound read-only is given: read a file, list a
// folder, and run a program. It is the same on every version, because all three
// rights arrived with version one.
func readableAccess() uint64 {
	return accessExecute | accessReadFile | accessReadDirectory
}

// writableAccess is what a sandbox root is given: everything the ruleset
// arbitrates, so that a command can do inside a root whatever it could do
// outside the fence.
func writableAccess(version int) uint64 {
	return handledAccess(version)
}

// encodeRulesetAttribute lays out struct landlock_ruleset_attr by hand. Only its
// first field is written, so the kernel arbitrates the filesystem and leaves the
// network alone, which is what lets a sandboxed command reach the internet.
func encodeRulesetAttribute(handled uint64) [rulesetAttributeSize]byte {
	encoded := [rulesetAttributeSize]byte{}
	binary.NativeEndian.PutUint64(encoded[0:8], handled)
	return encoded
}

// encodePathBeneathRule lays out struct landlock_path_beneath_attr by hand. The
// kernel's structure is packed, so the file number follows the rights with no
// padding between them, which is not how Go would lay the same fields out.
func encodePathBeneathRule(allowed uint64, parentFile int32) [pathBeneathRuleSize]byte {
	encoded := [pathBeneathRuleSize]byte{}
	binary.NativeEndian.PutUint64(encoded[0:8], allowed)
	binary.NativeEndian.PutUint32(encoded[8:12], uint32(parentFile))
	return encoded
}

// landlockVersion asks the kernel which Landlock it speaks.
func landlockVersion() (int, error) {
	version, _, errorNumber := syscall.Syscall(createRulesetCall, 0, 0, versionRequest)
	if errorNumber != 0 {
		return 0, fmt.Errorf("the kernel does not answer about Landlock, so check that it is among the security modules in %s: %w", securityModulesFile, errorNumber)
	}
	return int(version), nil
}

// buildLandlockRuleset makes a ruleset holding one rule per folder and returns
// it, without applying it. Applying it is a separate step, because it can never
// be undone and this half can be checked by a test.
func buildLandlockRuleset(readable []string, writable []string) (int, int, error) {
	version, err := landlockVersion()
	if err != nil {
		return 0, 0, err
	}
	rulesetFile, err := createLandlockRuleset(handledAccess(version))
	if err != nil {
		return 0, 0, err
	}

	rules := map[string]uint64{}
	for _, folder := range readable {
		rules[folder] |= readableAccess()
	}
	for _, folder := range writable {
		rules[folder] |= writableAccess(version)
	}
	for folder, allowed := range rules {
		if err := addLandlockRule(rulesetFile, folder, allowed); err != nil {
			_ = syscall.Close(rulesetFile)
			return 0, 0, err
		}
	}
	return rulesetFile, version, nil
}

// createLandlockRuleset makes an empty ruleset that arbitrates the given rights.
func createLandlockRuleset(handled uint64) (int, error) {
	attribute := encodeRulesetAttribute(handled)
	rulesetFile, _, errorNumber := syscall.Syscall(createRulesetCall,
		uintptr(unsafe.Pointer(&attribute[0])), rulesetAttributeSize, 0)
	if errorNumber != 0 {
		return 0, fmt.Errorf("the kernel refused a Landlock ruleset arbitrating %#x, so check the kernel's Landlock version: %w", handled, errorNumber)
	}
	return int(rulesetFile), nil
}

// addLandlockRule allows everything beneath one folder. A folder that is not on
// this machine is skipped rather than refused, because a rule can only ever take
// access away and a machine may lay its system folders out differently.
func addLandlockRule(rulesetFile int, folder string, allowed uint64) error {
	if strings.ContainsRune(folder, 0) {
		return fmt.Errorf("the folder name %q holds a zero byte, and a zero byte hides whatever follows it, so take it out", folder)
	}
	parentFile, err := syscall.Open(folder, pathOnlyOpenFlag|syscall.O_CLOEXEC, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("the sandbox cannot open the folder %q to hang a Landlock rule on it: %w", folder, err)
	}
	defer func() { _ = syscall.Close(parentFile) }()

	rule := encodePathBeneathRule(allowed, int32(parentFile))
	_, _, errorNumber := syscall.Syscall6(addRuleCall, uintptr(rulesetFile), pathBeneathRuleKind,
		uintptr(unsafe.Pointer(&rule[0])), 0, 0, 0)
	if errorNumber != 0 {
		return fmt.Errorf("the kernel refused a Landlock rule allowing %#x beneath %q: %w", allowed, folder, errorNumber)
	}
	return nil
}

// restrictWithLandlock applies a ruleset to this thread and to everything it
// starts, including the program it is about to become. It can never be undone,
// which is why nothing calls it but the helper inside the fence.
func restrictWithLandlock(rulesetFile int) error {
	if _, _, errorNumber := syscall.Syscall(restrictSelfCall, uintptr(rulesetFile), 0, 0); errorNumber != 0 {
		return fmt.Errorf("the kernel refused to apply the Landlock ruleset, so check that the no-new-privileges flag was set first: %w", errorNumber)
	}
	return nil
}

// applyLandlock builds the ruleset and applies it, and returns the Landlock
// version it was built against so that the helper can say so in its one line.
func applyLandlock(readable []string, writable []string) (int, error) {
	rulesetFile, version, err := buildLandlockRuleset(readable, writable)
	if err != nil {
		return 0, err
	}
	defer func() { _ = syscall.Close(rulesetFile) }()

	if err := restrictWithLandlock(rulesetFile); err != nil {
		return 0, err
	}
	return version, nil
}
