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
// of the line that set the same key, and above the first table header when
// nothing set it.
//
// The two rules are what the file's own format needs. A key written after a
// "[[models]]" header belongs to that table rather than to the file, so a line
// appended to the end of a configuration "coeus init" wrote becomes
// "models.signal_account" and the whole file stops loading, which takes every
// subcommand down with it. And the line to replace is found by its key and not
// by its first letters, so a setting whose name merely begins the same way is
// left alone.
func replaceOrAddSetting(existing string, key string, line string) string {
	lines := strings.Split(existing, "\n")
	for at, one := range lines {
		// The key is matched the way the reader matches it, which folds case.
		// A file holding "sIgnal_aCcount" already sets this setting, and a
		// second line spelled the ordinary way would leave two keys that both
		// mean it; which of them the reader then believes changes from run to
		// run, so the one already there is the one to replace.
		if strings.EqualFold(keyOfSettingLine(one), key) {
			lines[at] = line
			return strings.TrimLeft(strings.Join(lines, "\n"), "\n") + "\n"
		}
	}

	at := firstTableHeaderIn(lines)
	written := append([]string{}, lines[:at]...)
	written = append(written, line, "")
	written = append(written, lines[at:]...)
	return strings.TrimLeft(strings.Join(written, "\n"), "\n") + "\n"
}

// keyOfSettingLine is the key one line of the configuration sets, and is empty
// for a comment, a blank line, and a table header.
func keyOfSettingLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "[") {
		return ""
	}
	name, _, found := strings.Cut(trimmed, "=")
	if !found {
		return ""
	}
	return strings.TrimSpace(name)
}

// firstTableHeaderIn is the line the first "[table]" or "[[table]]" header is
// on, and the end of the file when there is none. A setting written above it
// belongs to the file rather than to a table.
func firstTableHeaderIn(lines []string) int {
	for at, one := range lines {
		if strings.HasPrefix(strings.TrimSpace(one), "[") {
			return at
		}
	}
	return len(lines)
}
