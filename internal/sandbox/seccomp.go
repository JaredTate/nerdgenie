// The idea of a small deny list applied to the thread that is about to become
// the command, rather than to the whole program, is borrowed from Codex's
// sandbox at docs/reference/codex/landlock.rs. Codex builds its filter with a
// library; this one is written out instruction by instruction, because the whole
// program is a dozen words and a dependency would be larger than the code.

package sandbox

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// systemCall is one system call the filter answers with "operation not
// permitted", carrying its name so that the golden file and any failure read
// plainly.
type systemCall struct {
	// name is what the system call is called, such as "mount".
	name string
	// number is what the kernel calls it on this architecture.
	number uint32
}

// filterWord is one instruction of the classic packet filter program the kernel
// runs before every system call. Its four fields are the kernel's sock_filter,
// which Go lays out the same way, so nothing here needs packing by hand.
type filterWord struct {
	code        uint16
	jumpIfTrue  uint8
	jumpIfFalse uint8
	value       uint32
}

// filterWordSize is how many bytes one instruction takes.
const filterWordSize = 8

// The instruction codes this program uses, each one a kernel constant built from
// the class of the instruction and what it does.
const (
	// loadWord reads four bytes out of the record the kernel hands the filter.
	loadWord = 0x20
	// jumpIfEqual jumps when the loaded value equals the instruction's value.
	jumpIfEqual = 0x15
	// jumpIfBitsSet jumps when the loaded value has any of those bits set.
	jumpIfBitsSet = 0x45
	// jumpAlways jumps over the given number of instructions.
	jumpAlways = 0x05
	// returnAnswer ends the program with an answer for the kernel.
	returnAnswer = 0x06
)

// Where each thing the filter reads sits in the record the kernel hands it: the
// system call number first, then the architecture, then the instruction pointer,
// then the six arguments.
const (
	systemCallNumberOffset = 0
	architectureOffset     = 4
	firstArgumentOffset    = 16
)

// The three answers this filter gives the kernel.
const (
	// allowAnswer lets the system call through.
	allowAnswer = 0x7fff0000
	// notPermittedAnswer turns the system call into an "operation not permitted"
	// error, which a program can report rather than dying on.
	notPermittedAnswer = 0x00050001
	// killAnswer ends the whole process, and is used only when the architecture
	// is not the one these numbers belong to, because there the numbers would
	// deny the wrong calls.
	killAnswer = 0x80000000
)

// newUserNamespaceFlag is the bit in the first argument of unshare that asks for
// a new user namespace. Everything else unshare can do is left alone.
const newUserNamespaceFlag = 0x10000000

// buildSeccompProgram writes out the whole filter. It checks the architecture
// first, because a system call number means something different on another one;
// then answers each denied number with "operation not permitted"; then lets an
// unshare through unless it asks for a new user namespace; then allows the rest.
func buildSeccompProgram(architecture uint32, denied []systemCall, unshareNumber uint32) []filterWord {
	program := []filterWord{
		{code: loadWord, value: architectureOffset},
		{code: jumpIfEqual, jumpIfTrue: 1, value: architecture},
		{code: returnAnswer, value: killAnswer},
		{code: loadWord, value: systemCallNumberOffset},
	}

	// The refusal is the last instruction, so a test that matches jumps from
	// where it sits to the end of the program. A jump is one byte wide, which
	// caps the deny list at about two hundred and fifty; a test holds it to the
	// fifteen the brief names.
	for index, call := range denied {
		toTheRefusal := uint8(len(denied) - index + 4)
		program = append(program, filterWord{code: jumpIfEqual, jumpIfTrue: toTheRefusal, value: call.number})
	}

	return append(program,
		filterWord{code: jumpIfEqual, jumpIfTrue: 1, value: unshareNumber},
		filterWord{code: jumpAlways, value: 2},
		filterWord{code: loadWord, value: firstArgumentOffset},
		filterWord{code: jumpIfBitsSet, jumpIfTrue: 1, value: newUserNamespaceFlag},
		filterWord{code: returnAnswer, value: allowAnswer},
		filterWord{code: returnAnswer, value: notPermittedAnswer},
	)
}

// encodeSeccompProgram lays the instructions out as the kernel reads them, which
// is eight bytes each in this machine's own byte order.
func encodeSeccompProgram(program []filterWord) []byte {
	encoded := make([]byte, 0, len(program)*filterWordSize)
	for _, word := range program {
		one := make([]byte, filterWordSize)
		binary.NativeEndian.PutUint16(one[0:2], word.code)
		one[2] = word.jumpIfTrue
		one[3] = word.jumpIfFalse
		binary.NativeEndian.PutUint32(one[4:8], word.value)
		encoded = append(encoded, one...)
	}
	return encoded
}

// setNoNewPrivilegesOption is the number of the prctl option that stops this
// process and everything it starts from ever gaining privileges again. The
// kernel insists on it before it will take a seccomp filter, and Landlock
// insists on it too.
const setNoNewPrivilegesOption = 38

// The two numbers that install a filter: the prctl option that takes one, and
// the one mode it accepts.
const (
	setSeccompFilterOption = 22
	filterMode             = 2
)

// filterProgram is what the kernel is handed: how many instructions there are
// and where they start. Go lays these two fields out the way the kernel's
// sock_fprog does on both architectures this package builds for.
type filterProgram struct {
	length      uint16
	instruction *filterWord
}

// setNoNewPrivileges stops this process and everything it starts from gaining
// privileges again, which the kernel requires before it will take either
// restriction.
func setNoNewPrivileges() error {
	_, _, errorNumber := syscall.Syscall6(syscall.SYS_PRCTL, setNoNewPrivilegesOption, 1, 0, 0, 0, 0)
	if errorNumber != 0 {
		return fmt.Errorf("the kernel refused to set the no-new-privileges flag, which every later restriction needs: %w", errorNumber)
	}
	return nil
}

// applySeccomp installs the filter on this thread and on everything it starts,
// including the program it is about to become. It can never be undone, which is
// why nothing calls it but the helper inside the fence.
func applySeccomp(program []filterWord) error {
	if len(program) == 0 {
		return errors.New("the sandbox was asked to install an empty seccomp filter, which would answer nothing, so build the filter first")
	}
	handed := filterProgram{length: uint16(len(program)), instruction: &program[0]}
	_, _, errorNumber := syscall.Syscall6(syscall.SYS_PRCTL, setSeccompFilterOption, filterMode,
		uintptr(unsafe.Pointer(&handed)), 0, 0, 0)
	if errorNumber != 0 {
		return fmt.Errorf("the kernel refused the seccomp filter of %d instructions, so check that the no-new-privileges flag was set first: %w", len(program), errorNumber)
	}
	return nil
}
