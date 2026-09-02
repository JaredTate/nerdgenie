package contract

// The exit codes the program uses when it quits. The service unit reads them:
// ExitRestartMe tells systemd to start the program again, and
// ExitBadConfiguration tells it not to, because restarting will not help.
const (
	// ExitOK means the program did what it was asked and quit cleanly.
	ExitOK = 0
	// ExitFailure means something went wrong that a restart might fix.
	ExitFailure = 1
	// ExitUsage means the command line was wrong, and the program printed how to
	// use it.
	ExitUsage = 2
	// ExitRestartMe means the program wants to be started again, which is what
	// it returns after an update or an unrecoverable internal state.
	ExitRestartMe = 75
	// ExitBadConfiguration means the configuration file is wrong. A restart will
	// fail the same way, so the service stops and waits for a person.
	ExitBadConfiguration = 78
)
