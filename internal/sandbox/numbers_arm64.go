//go:build linux && arm64

package sandbox

// seccompArchitecture is what the kernel reports for a 64-bit Arm program. The
// filter checks it first, because every number below means something else on
// another architecture.
const seccompArchitecture = 0xc00000b7

// unshareSystemCall is the number of unshare here. It is not in the list below,
// because a command may unshare anything but a new user namespace.
const unshareSystemCall = 97

// The numbers of the two calls that make a new namespace without unshare. clone
// is checked for the same flag unshare is; clone3 keeps its flags in a structure
// the filter cannot read, so it is refused whole.
const (
	cloneSystemCall  = 220
	clone3SystemCall = 435
)

// deniedSystemCalls are the system calls no tool needs, with their numbers on
// this architecture. The numbers are written out rather than taken from the
// syscall package, because that package is missing several of them here and
// there, and a missing number would quietly let a call through.
var deniedSystemCalls = []systemCall{
	{name: "mount", number: 40},
	{name: "umount2", number: 39},
	{name: "pivot_root", number: 41},
	{name: "ptrace", number: 117},
	{name: "kexec_load", number: 104},
	{name: "init_module", number: 105},
	{name: "finit_module", number: 273},
	{name: "delete_module", number: 106},
	{name: "reboot", number: 142},
	{name: "swapon", number: 224},
	{name: "swapoff", number: 225},
	{name: "setns", number: 268},
	{name: "bpf", number: 280},
	{name: "perf_event_open", number: 241},
	{name: "keyctl", number: 219},
}
