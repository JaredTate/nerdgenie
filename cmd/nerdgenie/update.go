package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/update"
)

// updateSubcommand installs a newer Coeus beside the running one, switches the
// link, and switches it back if the new version does not come up. It is the only
// way a machine changes version, and the model never runs it.
//
// The orchestrator adds this value to the table in main.go.
var updateSubcommand = subcommand{
	name: "update",
	help: "Installs the newest version, keeping the one before it to go back to.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		chosen, code := readUpdateFlags(arguments, problems)
		if code != contract.ExitOK {
			return code
		}
		home, settings, code := homeAndSettings(problems, "update")
		if code != contract.ExitOK {
			return code
		}
		if chosen.migrate {
			return bringTheDatabaseForward(home, settings, output, problems)
		}

		updater, err := update.New(update.Settings{
			Home: home, Clock: clock.System(), Version: version,
			Address: chosen.from, BackupFolder: settings.BackupPath,
			Announce: func(line string) { fmt.Fprintf(output, "%s\n", line) },
		})
		if err != nil {
			fmt.Fprintf(problems, "nerdgenie update: %v\n", err)
			return contract.ExitFailure
		}
		if chosen.check {
			return reportWhatIsOnOffer(updater, output, problems)
		}
		return installOrRollBack(updater, chosen, output, problems)
	},
}

// updateFlags is what the person asked for on the command line.
type updateFlags struct {
	// check prints what is available and installs nothing.
	check bool
	// rollback goes back to the version before the running one.
	rollback bool
	// migrate brings the database forward, which is the step the updater runs
	// with the newly installed program once it has come up.
	migrate bool
	// to is the version to install, and is empty for whatever is newest.
	to string
	// from is the release address, and is empty for the published one.
	from string
}

// readUpdateFlags reads the command line and refuses the combinations that
// contradict each other, because an update that guesses at what was meant is
// worse than one that asks again.
func readUpdateFlags(arguments []string, problems io.Writer) (updateFlags, int) {
	chosen := updateFlags{}
	set := flag.NewFlagSet("nerdgenie update", flag.ContinueOnError)
	set.SetOutput(problems)
	set.BoolVar(&chosen.check, "check", false, "say what is available and install nothing")
	set.BoolVar(&chosen.rollback, "rollback", false, "go back to the version installed before this one")
	set.BoolVar(&chosen.migrate, "migrate", false, "bring the database forward, which the updater does for itself")
	set.StringVar(&chosen.to, "to", "", "the version to install, when it is not the newest")
	set.StringVar(&chosen.from, "from", "", "the folder or web address to read the release from")

	if err := set.Parse(arguments); err != nil {
		fmt.Fprintf(problems, "nerdgenie update: the flags could not be read, so nothing was changed: %v\n", err)
		return chosen, contract.ExitUsage
	}
	if len(set.Args()) > 0 {
		fmt.Fprintf(problems, "nerdgenie update: this takes flags rather than words, and was given %q\n", strings.Join(set.Args(), " "))
		return chosen, contract.ExitUsage
	}
	return chosen, checkUpdateFlags(chosen, problems)
}

// checkUpdateFlags refuses two flags that cannot both be meant.
func checkUpdateFlags(chosen updateFlags, problems io.Writer) int {
	asked := 0
	for _, one := range []bool{chosen.check, chosen.rollback, chosen.migrate} {
		if one {
			asked++
		}
	}
	if asked > 1 {
		fmt.Fprintf(problems, "nerdgenie update: --check, --rollback, and --migrate each do a different thing, so ask for one of them at a time\n")
		return contract.ExitUsage
	}
	if chosen.rollback && chosen.to != "" {
		fmt.Fprintf(problems, "nerdgenie update: --rollback goes back to the version before this one, so it cannot also be given --to %s\n", chosen.to)
		return contract.ExitUsage
	}
	return contract.ExitOK
}

// reportWhatIsOnOffer prints what the release address is serving and changes
// nothing at all.
func reportWhatIsOnOffer(updater *update.Updater, output io.Writer, problems io.Writer) int {
	available, err := updater.Check(context.Background())
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie update: %v\n", err)
		return contract.ExitFailure
	}
	fmt.Fprintf(output, "running version %s.\n", available.Running)
	fmt.Fprintf(output, "%s offers version %s, built %s.\n", updater.Address(), available.Offered, available.Date)
	if available.Newer {
		fmt.Fprintf(output, "Run \"nerdgenie update\" to install it. The version running now is kept to go back to.\n")
		return contract.ExitOK
	}
	fmt.Fprintf(output, "There is nothing newer to install.\n")
	return contract.ExitOK
}

// installOrRollBack does the update itself, or the rollback, and prints what
// happened either way.
func installOrRollBack(updater *update.Updater, chosen updateFlags, output io.Writer, problems io.Writer) int {
	outcome := update.Outcome{}
	var err error
	if chosen.rollback {
		outcome, err = updater.Rollback(context.Background())
	} else {
		outcome, err = updater.Install(context.Background(), chosen.to)
	}
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie update: %v\n", err)
		return contract.ExitFailure
	}
	switch {
	case outcome.Installed:
		fmt.Fprintf(output, "version %s is installed and running, and version %s is kept to go back to.\n", outcome.To, outcome.From)
	case outcome.To != outcome.From:
		fmt.Fprintf(output, "version %s is running now.\n", outcome.To)
	default:
		fmt.Fprintf(output, "%s\n", outcome.Reason)
	}
	return contract.ExitOK
}

// bringTheDatabaseForward runs this version's own migrations, which is what the
// updater asks the newly installed program to do once it has come up.
func bringTheDatabaseForward(home contract.Home, settings contract.Config, output io.Writer, problems io.Writer) int {
	applied, err := update.Migrate(context.Background(), update.MigrateSettings{
		Home: home, Clock: clock.System(), BackupFolder: settings.BackupPath, Version: version,
	})
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie update: %v\n", err)
		return contract.ExitFailure
	}
	fmt.Fprintf(output, "the database is at schema version %d, and %d migrations were applied.\n",
		update.SchemaVersion(), applied)
	return contract.ExitOK
}
