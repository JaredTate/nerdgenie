package main

import (
	"context"
	"fmt"
	"io"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/vault"
)

// askpassSubcommand prints the sudo password out of the vault and nothing else.
// It exists for one caller: when the user has approved a command that needs
// sudo, the shell tool runs "sudo -A" with SUDO_ASKPASS pointing at this binary
// and this word, so that sudo reads the password from a program rather than
// from a command line, an environment variable, or the model's context.
//
// Anything that goes wrong is said on the error output, because whatever
// appears on the ordinary output is taken by sudo to be the password.
var askpassSubcommand = subcommand{
	name: "askpass",
	help: "Prints the sudo password from the vault, for SUDO_ASKPASS.",
	run: func(_ []string, output io.Writer, problems io.Writer) int {
		home, err := contract.DefaultHome()
		if err != nil {
			fmt.Fprintf(problems, "coeus askpass: %v\n", err)
			return contract.ExitBadConfiguration
		}

		opened, err := vault.Open(home, nil)
		if err != nil {
			fmt.Fprintf(problems, "coeus askpass: %v\n", err)
			return contract.ExitFailure
		}
		defer func() { _ = opened.Close() }()

		password, err := opened.SudoPassword(context.Background())
		if err != nil {
			fmt.Fprintf(problems, "coeus askpass: %v\n", err)
			return contract.ExitFailure
		}
		fmt.Fprintln(output, password)
		return contract.ExitOK
	},
}
