package main

import (
	"context"
	"fmt"
	"io"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// doctorSubcommand prints what is there, what is missing, and what to do about
// anything that is not right. It changes nothing at all, so it is safe to run at
// any time, and it leaves with a failing code only when something is broken; a
// browser that is not installed is a warning, and Coeus runs without it.
//
// The orchestrator adds this value to the table in main.go.
var doctorSubcommand = subcommand{
	name: "doctor",
	help: "Checks that everything Coeus needs is here, and says what to do about anything that is not.",
	run: func(_ []string, output io.Writer, problems io.Writer) int {
		home, err := config.HomeFolder()
		if err != nil {
			fmt.Fprintf(problems, "nerdgenie doctor: %v\n", err)
			return contract.ExitBadConfiguration
		}
		if !command.Doctor(context.Background(), home, output) {
			return contract.ExitFailure
		}
		return contract.ExitOK
	},
}
