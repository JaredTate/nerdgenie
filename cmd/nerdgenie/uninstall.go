package main

import (
	"context"
	"fmt"
	"io"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// uninstallSubcommand stops the service and takes the unit away. The home
// folder is kept, because it holds every task, memory, skill, and secret Coeus
// has; --purge removes it too, after the word "delete" has been typed.
//
// The orchestrator adds this value to the table in main.go.
var uninstallSubcommand = subcommand{
	name: "uninstall",
	help: "Stops the Coeus service and removes it, keeping your home folder unless --purge is given.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		service, code := serviceOnThisMachine(output, problems, "uninstall")
		if code != contract.ExitOK {
			return code
		}
		if err := service.Uninstall(context.Background(), arguments); err != nil {
			fmt.Fprintf(problems, "nerdgenie uninstall: %v\n", err)
			return contract.ExitFailure
		}
		return contract.ExitOK
	},
}
