package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// initSubcommand sets Coeus up on a machine that has never run it: the home
// folder, the folder Coeus may work in, the model, and the API key when the
// model needs one. It asks at most six questions and takes every one of them as
// a flag instead, so a container can run it with no keyboard.
//
// An API key is asked for through the vault's masked prompt, which reads the
// terminal itself and shows an asterisk for each character, so that a key is
// never echoed and never appears on a command line.
//
// The orchestrator adds this value to the table in main.go.
var initSubcommand = subcommand{
	name: "init",
	help: "Sets Coeus up on this machine: the home folder, the folders it may work in, and the model.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		home, err := config.HomeFolder()
		if err != nil {
			fmt.Fprintf(problems, "coeus init: %v\n", err)
			return contract.ExitBadConfiguration
		}

		setup := command.Setup{
			Home:   home,
			Input:  os.Stdin,
			Output: output,
			AskSecret: func(prompt string) (string, error) {
				return vault.AskSecret(os.Stdin, prompt)
			},
		}
		if err := command.Init(context.Background(), setup, arguments); err != nil {
			fmt.Fprintf(problems, "coeus init: %v\n", err)
			return contract.ExitFailure
		}
		return contract.ExitOK
	},
}
