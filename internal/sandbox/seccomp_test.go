package sandbox

import (
	"runtime"
	"slices"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"
)

// theSystemCallsNoToolNeeds is the list from brief 2.3. The filter has to answer
// every one of them with "operation not permitted".
var theSystemCallsNoToolNeeds = []string{
	"mount", "umount2", "pivot_root", "ptrace", "kexec_load",
	"init_module", "finit_module", "delete_module", "reboot",
	"swapon", "swapoff", "setns", "bpf", "perf_event_open", "keyctl",
}

func TestTheFilterDeniesEverySystemCallNoToolNeeds(t *testing.T) {
	denied := []string{}
	for _, call := range deniedSystemCalls {
		denied = append(denied, call.name)
	}

	for _, wanted := range theSystemCallsNoToolNeeds {
		if !slices.Contains(denied, wanted) {
			t.Errorf("the filter lets %s through, and no tool needs it", wanted)
		}
	}
	if len(denied) != len(theSystemCallsNoToolNeeds) {
		t.Errorf("the filter denies %d system calls, want the %d in the brief", len(denied), len(theSystemCallsNoToolNeeds))
	}
	for _, call := range deniedSystemCalls {
		if call.number == 0 {
			t.Errorf("the system call %s has no number on %s", call.name, runtime.GOARCH)
		}
	}
}

func TestTheFilterForASmallListIsLaidOutInstructionByInstruction(t *testing.T) {
	program := buildSeccompProgram(0xc000003e, []systemCall{{name: "mount", number: 165}, {name: "reboot", number: 169}}, 272)

	want := []filterWord{
		{code: loadWord, value: architectureOffset},
		{code: jumpIfEqual, jumpIfTrue: 1, value: 0xc000003e},
		{code: returnAnswer, value: killAnswer},
		{code: loadWord, value: systemCallNumberOffset},
		{code: jumpIfEqual, jumpIfTrue: 6, value: 165},
		{code: jumpIfEqual, jumpIfTrue: 5, value: 169},
		{code: jumpIfEqual, jumpIfTrue: 1, value: 272},
		{code: jumpAlways, value: 2},
		{code: loadWord, value: firstArgumentOffset},
		{code: jumpIfBitsSet, jumpIfTrue: 1, value: newUserNamespaceFlag},
		{code: returnAnswer, value: allowAnswer},
		{code: returnAnswer, value: notPermittedAnswer},
	}
	if !slices.Equal(program, want) {
		t.Errorf("the filter program is\n%+v\nwant\n%+v", program, want)
	}
}

func TestTheFilterAnswersEveryDeniedCallWithTheSameInstruction(t *testing.T) {
	program := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)

	notPermittedAt := len(program) - 1
	if program[notPermittedAt].value != notPermittedAnswer {
		t.Fatalf("the last instruction answers %#x, want %#x", program[notPermittedAt].value, notPermittedAnswer)
	}
	// The first four instructions check the architecture and load the system
	// call number; the deny list starts after them and ends at the unshare test.
	for index := 4; index < 4+len(deniedSystemCalls); index++ {
		word := program[index]
		if word.code != jumpIfEqual {
			t.Fatalf("instruction %d is a %#x, want a test of the system call number", index, word.code)
		}
		landsOn := index + 1 + int(word.jumpIfTrue)
		if landsOn != notPermittedAt {
			t.Errorf("the test for system call %d jumps to instruction %d, want the refusal at %d", word.value, landsOn, notPermittedAt)
		}
	}
}

func TestEveryInstructionIsEightBytesInTheOrderTheKernelReads(t *testing.T) {
	encoded := encodeSeccompProgram([]filterWord{{code: 0x1502, jumpIfTrue: 3, jumpIfFalse: 4, value: 0x0a0b0c0d}})

	want := []byte{0x02, 0x15, 0x03, 0x04, 0x0d, 0x0c, 0x0b, 0x0a}
	if len(encoded) != filterWordSize {
		t.Fatalf("one instruction is %d bytes, want %d", len(encoded), filterWordSize)
	}
	for index := range want {
		if encoded[index] != want[index] {
			t.Fatalf("the instruction is %#v, want %#v", encoded, want)
		}
	}
}

func TestTheWholeFilterIsWhatTheGoldenFileHolds(t *testing.T) {
	program := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)

	testkit.Golden(t, "seccomp-"+runtime.GOARCH+".golden", encodeSeccompProgram(program))
}
