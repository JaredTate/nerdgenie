package main

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// TestTheAskpassSubcommandIsTheWordTheShellToolWrites holds the one rule that
// ties the two halves of the sudo path together. The shell tool writes this word
// into the helper it hands sudo through SUDO_ASKPASS; a subcommand of another
// name would leave sudo asking a program that answers "there is no subcommand
// named askpass", and no approved command with administrator powers could run.
func TestTheAskpassSubcommandIsTheWordTheShellToolWrites(t *testing.T) {
	if askpassSubcommand.name != shell.AskpassSubcommand {
		t.Errorf("the subcommand is %q and the shell tool writes %q, so sudo would ask a program that does not answer",
			askpassSubcommand.name, shell.AskpassSubcommand)
	}
	if _, found := lookUp(theRealTable(), shell.AskpassSubcommand); !found {
		t.Errorf("the table holds no %q subcommand at all", shell.AskpassSubcommand)
	}
}
