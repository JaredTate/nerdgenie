package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/JaredTate/coeus/internal/contract"
)

// version is the version string the version subcommand prints. A release build
// sets it with -ldflags "-X main.version=<the tag>"; a development build says
// "dev".
var version = "dev"

// subcommand is one thing coeus can be asked to do from the command line.
type subcommand struct {
	// name is what the user types after "coeus".
	name string
	// help is the one line the listing prints beside the name.
	help string
	// run does the work and returns the exit code, which is always one of the
	// codes in internal/contract.
	run func(arguments []string, output io.Writer, problems io.Writer) int
}

// subcommands is the whole table. A worker who adds a subcommand writes a new
// file in this folder holding one subcommand value, and the orchestrator adds
// that value to this list; no worker edits this file.
func subcommands() []subcommand {
	return []subcommand{versionSubcommand, helpSubcommand()}
}

// helpName is what the user types to see the list of subcommands.
const helpName = "help"

// helpFlags are the two ways a person asks for the list without typing the word.
// Both are the help row of the table rather than a case of their own.
var helpFlags = []string{"-h", "--help"}

// helpSubcommand prints the list of subcommands. It is a row of the table like
// every other subcommand, so that the table's own rules apply to it too, and it
// is a function rather than a value because it names the table it sits in.
func helpSubcommand() subcommand {
	return subcommand{
		name: helpName,
		help: "Shows this list.",
		run: func(_ []string, output io.Writer, _ io.Writer) int {
			writeHelp(subcommands(), output)
			return contract.ExitOK
		},
	}
}

func main() {
	os.Exit(run(subcommands(), os.Args[1:], os.Stdout, os.Stderr))
}

// run picks the subcommand out of the table and returns whatever exit code it
// gave back, unchanged. The codes come from internal/contract, because the
// service unit reads them: 75 asks systemd to start the program again, and 78
// says the configuration is wrong and a restart would fail the same way.
func run(table []subcommand, arguments []string, output io.Writer, problems io.Writer) int {
	if len(arguments) == 0 {
		writeHelp(table, output)
		return contract.ExitUsage
	}

	asked := arguments[0]
	if slices.Contains(helpFlags, asked) {
		asked = helpName
	}

	for _, command := range table {
		if command.name == asked {
			return command.run(arguments[1:], output, problems)
		}
	}

	fmt.Fprintf(problems, "coeus: there is no subcommand named %q. Run \"coeus help\" for the list.\n", asked)
	return contract.ExitUsage
}

// writeHelp prints every subcommand with its one help line.
func writeHelp(table []subcommand, output io.Writer) {
	fmt.Fprintln(output, "coeus - an assistant that runs on your own computer.")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Usage: coeus <subcommand> [arguments]")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Subcommands:")

	listing := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	for _, command := range table {
		fmt.Fprintf(listing, "  %s\t%s\n", command.name, command.help)
	}
	if err := listing.Flush(); err != nil {
		fmt.Fprintln(output, "  (the list could not be laid out)")
	}
}
