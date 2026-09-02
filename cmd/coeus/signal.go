package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/signal"
)

// signalProgramName is the program that talks to Signal, looked for on the PATH.
const signalProgramName = "signal-cli"

// accountSetting is the key in config.toml that holds the linked account.
const accountSetting = "signal_account"

// signalSubcommand is "coeus signal". Its one action in this wave is "link",
// which links this computer to the user's Signal account as another device, the
// same way the Signal desktop application is linked to a phone.
//
// The orchestrator adds this value to the table in main.go.
var signalSubcommand = subcommand{
	name: "signal",
	help: "Links this computer to your Signal account. Use: coeus signal link.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		if len(arguments) == 0 || arguments[0] != "link" {
			fmt.Fprintln(problems, "coeus signal: the only action is link. Use: coeus signal link")
			return contract.ExitUsage
		}
		return runSignalLink(output, problems)
	},
}

// runSignalLink finds signal-cli, links the account, and writes it into the
// configuration.
func runSignalLink(output io.Writer, problems io.Writer) int {
	program, err := exec.LookPath(signalProgramName)
	if err != nil {
		fmt.Fprintf(problems, "coeus signal link: cannot find %s on your PATH. Install it from https://github.com/AsamK/signal-cli/releases and try again.\n", signalProgramName)
		return contract.ExitFailure
	}
	home, err := config.HomeFolder()
	if err != nil {
		fmt.Fprintf(problems, "coeus signal link: %v\n", err)
		return contract.ExitBadConfiguration
	}

	_, err = signal.Link(context.Background(), signal.LinkOptions{
		Program:     program,
		Out:         output,
		Clock:       clock.System(),
		SaveAccount: func(account string) error { return saveSignalAccount(home, account) },
	})
	if err != nil {
		fmt.Fprintf(problems, "coeus signal link: %v\n", err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// saveSignalAccount writes the linked account into config.toml, replacing the
// line that was there and leaving every other line alone. This is the function
// the Signal package calls once the phone has scanned; serve.go supplies its own
// when the agent is running.
func saveSignalAccount(home contract.Home, account string) error {
	path := home.ConfigFile()
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read the configuration file %s to write the Signal account into it: %w", path, err)
	}

	line := fmt.Sprintf("%s = %q", accountSetting, account)
	written := replaceOrAddSetting(string(existing), accountSetting, line)
	if err := os.WriteFile(path, []byte(written), contract.DataFileMode); err != nil {
		return fmt.Errorf("cannot write the configuration file %s, so check who owns it: %w", path, err)
	}
	return nil
}

// replaceOrAddSetting puts one setting line into a configuration file, in place
// of the line that set the same key, or at the end when nothing set it.
func replaceOrAddSetting(existing string, key string, line string) string {
	lines := strings.Split(existing, "\n")
	replaced := false
	for at, one := range lines {
		if strings.HasPrefix(strings.TrimSpace(one), key) {
			lines[at] = line
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")
	return strings.TrimLeft(joined, "\n") + "\n"
}
