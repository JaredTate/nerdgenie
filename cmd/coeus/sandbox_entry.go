package main

import (
	"fmt"
	"io"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/sandbox"
)

// sandboxEntrySubcommand is the helper the sandbox starts inside its own fence.
// bwrap cannot apply Landlock, so the fence starts this program again as
// "coeus sandbox-entry ...", and this subcommand applies the Landlock ruleset and
// the seccomp filter and then becomes the command it was asked to run.
//
// It is not meant to be typed by a person, and it refuses to run unless the fence
// started it. The orchestrator adds this value to the table in main.go.
var sandboxEntrySubcommand = subcommand{
	name: sandbox.EntrySubcommandName,
	help: "Runs a command inside the sandbox fence. Coeus starts this itself and it is not meant to be typed.",
	run: func(arguments []string, _ io.Writer, problems io.Writer) int {
		// On a good day this call never comes back, because the helper becomes
		// the command it was asked to run.
		if err := sandbox.Entry(arguments, problems); err != nil {
			fmt.Fprintf(problems, "coeus %s: %v\n", sandbox.EntrySubcommandName, err)
			return contract.ExitFailure
		}
		return contract.ExitOK
	},
}
