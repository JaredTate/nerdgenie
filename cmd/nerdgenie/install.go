package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// installSubcommand writes the systemd user unit and starts the service, so
// that Nerd Genie comes up with the user's session and is started again when it
// stops. It needs no administrator rights, because a user unit belongs to the
// account that runs it.
//
// The orchestrator adds this value to the table in main.go.
var installSubcommand = subcommand{
	name: "install",
	help: "Installs Nerd Genie as a service that starts with your session and is restarted if it stops.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		if len(arguments) > 0 {
			fmt.Fprintf(problems, "nerdgenie install: this takes no arguments, and was given %q\n", strings.Join(arguments, " "))
			return contract.ExitUsage
		}
		service, code := serviceOnThisMachine(output, problems, "install")
		if code != contract.ExitOK {
			return code
		}
		if err := service.Install(context.Background()); err != nil {
			fmt.Fprintf(problems, "nerdgenie install: %v\n", err)
			return contract.ExitFailure
		}
		return contract.ExitOK
	},
}

// serviceOnThisMachine builds the install and uninstall work against this
// machine's home folder and the binary running right now, which is what the
// current link is made to point at when there is no installed release yet.
func serviceOnThisMachine(output io.Writer, problems io.Writer, name string) (command.Service, int) {
	home, err := config.HomeFolder()
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie %s: %v\n", name, err)
		return command.Service{}, contract.ExitBadConfiguration
	}
	program, err := os.Executable()
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie %s: the path of the running program could not be found, so the service would have nothing to start: %v\n", name, err)
		return command.Service{}, contract.ExitFailure
	}
	return command.Service{Home: home, Program: program, Input: os.Stdin, Output: output}, contract.ExitOK
}
