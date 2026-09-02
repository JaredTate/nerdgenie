package main

import (
	"fmt"
	"io"

	"github.com/JaredTate/coeus/internal/contract"
)

// versionSubcommand prints the version this binary was built as. The updater
// reads it to decide whether a release is newer, and a person reads it when
// something goes wrong.
var versionSubcommand = subcommand{
	name: "version",
	help: "Prints the version of this build.",
	run: func(_ []string, output io.Writer, _ io.Writer) int {
		fmt.Fprintln(output, version)
		return contract.ExitOK
	},
}
