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
		{code: jumpIfEqual, jumpIfTrue: 9, value: 165},
		{code: jumpIfEqual, jumpIfTrue: 8, value: 169},
		{code: jumpIfEqual, jumpIfTrue: 6, value: clone3SystemCall},
		{code: jumpIfEqual, jumpIfTrue: 2, value: 272},
		{code: jumpIfEqual, jumpIfTrue: 1, value: cloneSystemCall},
		{code: jumpAlways, value: 2},
		{code: loadWord, value: firstArgumentOffset},
		{code: jumpIfBitsSet, jumpIfTrue: 2, value: newUserNamespaceFlag},
		{code: returnAnswer, value: allowAnswer},
		{code: returnAnswer, value: noSuchCallAnswer},
		{code: returnAnswer, value: notPermittedAnswer},
	}
	if !slices.Equal(program, want) {
		t.Errorf("the filter program is\n%+v\nwant\n%+v", program, want)
	}
}

func TestBothWaysOfMakingANamespaceAreTestedAgainstTheSameFlag(t *testing.T) {
	program := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)

	flagTest := -1
	for index, word := range program {
		if word.code == jumpIfBitsSet && word.value == newUserNamespaceFlag {
			flagTest = index
		}
	}
	if flagTest < 1 || program[flagTest-1].code != loadWord || program[flagTest-1].value != firstArgumentOffset {
		t.Fatalf("the filter tests the new-user-namespace flag at instruction %d without loading the first argument first", flagTest)
	}

	for _, call := range []struct {
		name   string
		number uint32
	}{{"unshare", unshareSystemCall}, {"clone", cloneSystemCall}} {
		landsOnTheFlagTest := false
		for index, word := range program {
			if word.code == jumpIfEqual && word.value == call.number && index+1+int(word.jumpIfTrue) == flagTest-1 {
				landsOnTheFlagTest = true
			}
		}
		if !landsOnTheFlagTest {
			t.Errorf("%s does not lead to the test of the new-user-namespace flag, so it can make the namespace the filter refuses unshare", call.name)
		}
	}
}

func TestCloneThreeIsAnsweredAsACallThisKernelDoesNotHave(t *testing.T) {
	program := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)

	// The flags of clone3 are in a structure the filter cannot read, so the call
	// is refused whole. It has to be refused as a call that is not there, because
	// that is the one answer the C library falls back from to the older clone; an
	// "operation not permitted" would stop every fork inside the fence.
	answered := false
	for index, word := range program {
		if word.code == jumpIfEqual && word.value == clone3SystemCall {
			answered = program[index+1+int(word.jumpIfTrue)].value == noSuchCallAnswer
		}
	}
	if !answered {
		t.Errorf("clone3 is not answered with %#x, the number for a call this kernel does not have", noSuchCallAnswer)
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

func TestAnEmptyFilterIsRefusedBeforeTheKernelIsAskedToTakeIt(t *testing.T) {
	if err := applySeccomp(nil); err == nil {
		t.Fatal("an empty filter was installed, and a filter that answers nothing would let everything through")
	}
}

func TestTheWholeFilterIsWhatTheGoldenFileHolds(t *testing.T) {
	program := buildSeccompProgram(seccompArchitecture, deniedSystemCalls, unshareSystemCall)

	testkit.Golden(t, "seccomp-"+runtime.GOARCH+".golden", encodeSeccompProgram(program))
}
