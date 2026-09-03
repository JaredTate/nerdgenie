package sandbox

import (
	"fmt"
	"syscall"
)

// The bounds one sandboxed command runs under. They are named here rather than
// in contract.Caps because nothing outside the fence sets them and no user has
// asked to change them; a test pins every number, so moving one is a deliberate
// act.
const (
	// MostProcesses is how many processes one sandboxed command may have at
	// once. The count is kept inside the fence's own user namespace, which
	// starts with nothing in it, so five hundred and twelve is room for a
	// parallel build and far short of the hundred and seventy thousand the
	// machine itself allows, which is what a fork bomb needs.
	MostProcesses = 512
	// TemporaryFolderBytes is the size of the fresh /tmp the fence makes. A
	// tmpfs asked for with no size is half the machine's memory, and a command
	// can fill it a byte at a time, so it is always asked for with one.
	TemporaryFolderBytes = 100 << 20
	// HomeFolderBytes is the size of the fresh home directory the fence makes at
	// the user's own home path, with the sandbox roots bound inside it. It is
	// larger than the temporary folder because a build writes its package cache
	// there, in ~/.npm and ~/.cache, and small enough beside this machine's
	// memory that filling it cannot take the machine down.
	HomeFolderBytes = 1 << 30
)

// The number the kernel knows the process bound by. It is written out because
// Go's syscall package does not name it, and it is the same on every
// architecture Linux runs on. There is no bound on address space on purpose:
// Node's engine and Go's runtime reserve tens of gigabytes of virtual space they
// never touch, and a four-gigabyte bound made a test runner abort inside the
// fence on the first live run. Memory is bounded by the machine, the process
// count, and the two folder sizes.
const processCountResource = 6

// theBoundsOnOneCommand is what the helper sets on itself just before it becomes
// the command. Each carries the words a person needs when the kernel refuses it.
var theBoundsOnOneCommand = []struct {
	name     string
	resource int
	value    uint64
}{
	{name: "how many processes it may start", resource: processCountResource, value: MostProcesses},
}

// setResourceLimits binds this program, and with it the command it is about to
// become, to the bounds above. Both the soft and the hard limit are set, so the
// command cannot raise its own.
//
// The kernel call is a parameter so that a test can watch what is asked for
// without binding the test program itself, which would leave it unable to start
// another process.
func setResourceLimits(setLimit func(resource int, limit *syscall.Rlimit) error) error {
	for _, bound := range theBoundsOnOneCommand {
		limit := syscall.Rlimit{Cur: bound.value, Max: bound.value}
		if err := setLimit(bound.resource, &limit); err != nil {
			return fmt.Errorf("the sandbox could not bound %s to %d, and a command that runs unbounded can take the whole machine down, so run coeus on a kernel that allows the limit to be set: %w",
				bound.name, bound.value, err)
		}
	}
	return nil
}
