package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
	"github.com/JaredTate/nerdgenie/internal/vault"
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
	name: shell.AskpassSubcommand,
	help: "Prints the sudo password from the vault, for SUDO_ASKPASS.",
	run:  printTheSudoPassword,
}

// printTheSudoPassword finds the vault, reads the machine password out of it,
// and writes it on the ordinary output for sudo to read.
func printTheSudoPassword(_ []string, output io.Writer, problems io.Writer) int {
	home, err := theHomeAskpassMustRead()
	if err != nil {
		fmt.Fprintf(problems, "coeus askpass: %v\n", err)
		return contract.ExitBadConfiguration
	}

	// The vault is opened only when it is already there. Opening one that is not
	// makes an age private key, and this helper is run by sudo, under another
	// account's environment, possibly many times: a key nothing manages must
	// never be left behind by a program that only wanted to read.
	if _, err := os.Stat(home.VaultFile()); err != nil {
		fmt.Fprintf(problems, "coeus askpass: there is no vault at %s, so run \"coeus init\" and then \"/vault add sudo\" in the terminal: %v\n",
			home.VaultFile(), err)
		return contract.ExitFailure
	}

	opened, err := vault.Open(home, clock.System())
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
}

// theHomeAskpassMustRead finds the agent's home folder from inside sudo, where
// the environment is not the one the agent runs under.
//
// Three things make this different from every other subcommand. COEUS_HOME may
// have moved the home folder, and it is passed through, so it is read first, the
// way config.HomeFolder reads it. The shell tool sets HOME to the agent's own
// home folder before it runs sudo, so HOME may already be the home rather than
// the folder holding it. And sudo may have replaced HOME with root's, in which
// case the account that asked is named in SUDO_USER and its home is where the
// vault is. Each candidate is judged by whether it really holds a vault, so a
// guess never becomes a new empty home.
func theHomeAskpassMustRead() (contract.Home, error) {
	if named := strings.TrimSpace(os.Getenv(config.HomeVariable)); named != "" {
		return config.HomeFolder()
	}
	for _, candidate := range whereTheVaultCouldBe() {
		if _, err := os.Stat(candidate.VaultFile()); err == nil {
			return candidate, nil
		}
	}
	return config.HomeFolder()
}

// whereTheVaultCouldBe lists the folders that could be the agent's home, in the
// order they are tried: the home the ordinary rules give, the folder HOME names
// when the shell tool has already pointed it at the agent's home, and the home
// of the account that asked for sudo.
func whereTheVaultCouldBe() []contract.Home {
	could := []contract.Home{}
	if ordinary, err := config.HomeFolder(); err == nil {
		could = append(could, ordinary)
	}
	if named := strings.TrimSpace(os.Getenv("HOME")); named != "" {
		could = append(could, contract.NewHome(named))
	}
	if asked := strings.TrimSpace(os.Getenv("SUDO_USER")); asked != "" {
		if found, err := user.Lookup(asked); err == nil && found.HomeDir != "" {
			could = append(could, contract.NewHome(filepath.Join(found.HomeDir, contract.HomeFolderName)))
		}
	}
	return could
}
