//go:build linux && amd64

package sandbox

// seccompArchitecture is what the kernel reports for a 64-bit Intel or AMD
// program. The filter checks it first, because every number below means
// something else on another architecture.
const seccompArchitecture = 0xc000003e

// unshareSystemCall is the number of unshare here. It is not in the list below,
// because a command may unshare anything but a new user namespace.
const unshareSystemCall = 272

// The numbers of the two calls that make a new namespace without unshare. clone
// is checked for the same flag unshare is; clone3 keeps its flags in a structure
// the filter cannot read, so it is refused whole.
const (
	cloneSystemCall  = 56
	clone3SystemCall = 435
)

// deniedSystemCalls are the system calls no tool needs, with their numbers on
// this architecture. The numbers are written out rather than taken from the
// syscall package, because that package is missing several of them here and
// there, and a missing number would quietly let a call through.
var deniedSystemCalls = []systemCall{
	{name: "mount", number: 165},
	{name: "umount2", number: 166},
	{name: "pivot_root", number: 155},
	{name: "ptrace", number: 101},
	{name: "kexec_load", number: 246},
	{name: "init_module", number: 175},
	{name: "finit_module", number: 313},
	{name: "delete_module", number: 176},
	{name: "reboot", number: 169},
	{name: "swapon", number: 167},
	{name: "swapoff", number: 168},
	{name: "setns", number: 308},
	{name: "bpf", number: 321},
	{name: "perf_event_open", number: 298},
	{name: "keyctl", number: 250},
}
