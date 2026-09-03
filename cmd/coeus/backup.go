package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
)

// backupSubcommand writes one encrypted archive of everything Coeus keeps and
// removes all but the newest few. The nightly systemd timer that "coeus install"
// writes runs exactly this.
//
// The orchestrator adds this value to the table in main.go.
var backupSubcommand = subcommand{
	name: "backup",
	help: "Writes an encrypted copy of the database, the vault, and the browser profile, keeping the last seven.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		if len(arguments) > 0 {
			fmt.Fprintf(problems, "coeus backup: this takes no arguments, and was given %q\n", strings.Join(arguments, " "))
			return contract.ExitUsage
		}
		home, settings, code := homeAndSettings(problems, "backup")
		if code != contract.ExitOK {
			return code
		}

		archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{
			Home:   home,
			Clock:  clock.System(),
			Folder: settings.BackupPath,
		})
		if err != nil {
			fmt.Fprintf(problems, "coeus backup: %v\n", err)
			return contract.ExitFailure
		}

		fmt.Fprintf(output, "wrote %s, keeping the last %d.\n", archive, reliability.KeptBackups)
		fmt.Fprintf(output, "It is locked with the key in %s. Keep a copy of that key somewhere else,\n", home.VaultKeyFile())
		fmt.Fprintf(output, "because without it the archive cannot be opened again.\n")
		return contract.ExitOK
	},
}

// restoreSubcommand puts one archive back into a home folder.
//
// The orchestrator adds this value to the table in main.go.
var restoreSubcommand = subcommand{
	name: "restore",
	help: "Puts a backup archive back: the database, the vault, and the browser profile.",
	run: func(arguments []string, output io.Writer, problems io.Writer) int {
		set := flag.NewFlagSet("coeus restore", flag.ContinueOnError)
		set.SetOutput(problems)
		force := set.Bool("force", false, "write over a home folder that is already in use")
		keyFile := set.String("key", "", "the age key file that opens the archive, when it is not the one in the home folder")
		if err := set.Parse(arguments); err != nil {
			fmt.Fprintf(problems, "coeus restore: the flags could not be read, so nothing was changed: %v\n", err)
			return contract.ExitUsage
		}
		if len(set.Args()) != 1 {
			fmt.Fprintf(problems, "coeus restore: name exactly one archive to put back, as in \"coeus restore ~/.coeus/backups/%s\"\n",
				reliability.ArchivePrefix+"2026-09-02-030000"+reliability.ArchiveSuffix)
			return contract.ExitUsage
		}
		home, _, code := homeAndSettings(problems, "restore")
		if code != contract.ExitOK {
			return code
		}

		archive := set.Args()[0]
		err := reliability.Restore(context.Background(), reliability.RestoreSettings{
			Home:    home,
			Archive: archive,
			KeyFile: *keyFile,
			Force:   *force,
		})
		if err != nil {
			fmt.Fprintf(problems, "coeus restore: %v\n", err)
			return contract.ExitFailure
		}

		fmt.Fprintf(output, "put %s back into %s.\n", archive, home.Root)
		fmt.Fprintf(output, "Start Coeus again to work from what it holds.\n")
		return contract.ExitOK
	},
}

// homeAndSettings works out which home folder to act on and reads its
// configuration, which is where the backup folder is named.
func homeAndSettings(problems io.Writer, name string) (contract.Home, contract.Config, int) {
	home, err := config.HomeFolder()
	if err != nil {
		fmt.Fprintf(problems, "coeus %s: %v\n", name, err)
		return contract.Home{}, contract.Config{}, contract.ExitBadConfiguration
	}
	settings, err := config.Load(home)
	if err != nil {
		fmt.Fprintf(problems, "coeus %s: %v\n", name, err)
		return contract.Home{}, contract.Config{}, contract.ExitBadConfiguration
	}
	return home, settings, contract.ExitOK
}
