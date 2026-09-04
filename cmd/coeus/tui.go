package main

import (
	"fmt"
	"io"
	"os"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tui"
)

// tuiSubcommand opens the terminal screen. It is what the bare "coeus" command
// runs, and it never waits on the running program: the first frame appears at
// once, and the status strip says whether the link is up and keeps trying when
// it is not, so that a person who starts the screen before the program never
// sees a crash.
var tuiSubcommand = subcommand{
	name: "tui",
	help: "Opens the terminal screen and talks to the running program.",
	run:  runTerminalScreen,
}

// runTerminalScreen finds the home folder, opens the screen on this terminal,
// and turns whatever went wrong into one plain line and an exit code.
func runTerminalScreen(arguments []string, output io.Writer, problems io.Writer) int {
	if len(arguments) > 0 {
		fmt.Fprintf(problems, "coeus tui takes no arguments, and it was given %q. Run \"coeus help\" for the list.\n", arguments[0])
		return contract.ExitUsage
	}

	home, err := config.HomeFolder()
	if err != nil {
		fmt.Fprintf(problems, "coeus tui: %v\n", err)
		return contract.ExitBadConfiguration
	}

	err = tui.Run(tui.Options{
		Dialer:      tui.NewUnixDialer(home),
		Environment: os.Getenv,
		Output:      output,
		Input:       os.Stdin,
	})
	if err != nil {
		fmt.Fprintf(problems, "coeus tui: the screen stopped: %v\n", err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}
