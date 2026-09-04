package command

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// initFlags are the answers "nerdgenie init" accepts on the command line, so that a
// container with no keyboard can set Coeus up in one line.
type initFlags struct {
	// model is the name of one of the six models, or empty to ask.
	model string
	// workFolders are the folders Coeus may work in, or empty to ask.
	workFolders []string
	// apiKeyFromEnvironment names the environment variable holding the API key,
	// so that a key never appears on a command line.
	apiKeyFromEnvironment string
	// signal is "on" or "off", or empty to ask.
	signal string
	// yes takes the default answer to every question and asks nothing.
	yes bool
	// resetConfig writes config.toml again on a home folder that already has one.
	resetConfig bool
}

// readInitFlags reads the command line "nerdgenie init" was given. Nothing on disk
// is touched until this has come back without an error, so a misspelled flag
// leaves the machine exactly as it was.
func readInitFlags(arguments []string, output io.Writer) (initFlags, error) {
	chosen := initFlags{}
	folders := ""

	set := flag.NewFlagSet("nerdgenie init", flag.ContinueOnError)
	set.SetOutput(output)
	set.StringVar(&chosen.model, "model", "", "which model to use: local, lmstudio, claude, codex, anthropic, or openai")
	set.StringVar(&folders, "work-folder", "", "the folders Coeus may work in, separated by commas")
	set.StringVar(&chosen.apiKeyFromEnvironment, "api-key-from-env", "", "the name of the environment variable holding the API key")
	set.StringVar(&chosen.signal, "signal", "", "whether to offer Signal: on or off")
	set.BoolVar(&chosen.yes, "yes", false, "take the default answer to every question and ask nothing")
	set.BoolVar(&chosen.resetConfig, "reset-config", false, "write config.toml again even though the home folder already has one")

	if err := set.Parse(arguments); err != nil {
		return initFlags{}, fmt.Errorf("nerdgenie init could not read its flags, so nothing was set up: %w", err)
	}
	if left := set.Args(); len(left) > 0 {
		return initFlags{}, fmt.Errorf("nerdgenie init takes no plain words, and was given %q, so write every answer as a flag such as --model local", strings.Join(left, " "))
	}
	if chosen.signal != "" && chosen.signal != "on" && chosen.signal != "off" {
		return initFlags{}, fmt.Errorf("--signal is %q, so write it as --signal on or --signal off", chosen.signal)
	}
	chosen.workFolders = splitFolders(folders)
	return chosen, nil
}

// splitFolders reads a comma-separated list of folders, leaving out the empty
// pieces a trailing comma leaves behind.
func splitFolders(written string) []string {
	folders := []string{}
	for _, piece := range strings.Split(written, ",") {
		if trimmed := strings.TrimSpace(piece); trimmed != "" {
			folders = append(folders, trimmed)
		}
	}
	return folders
}

// expandFolder turns a folder written with a leading tilde into a full path,
// because a shell that was given the path in quotation marks does not.
func expandFolder(written string, userHome string) string {
	trimmed := strings.TrimSpace(written)
	switch {
	case trimmed == "~":
		return userHome
	case strings.HasPrefix(trimmed, "~/"):
		return filepath.Join(userHome, trimmed[2:])
	default:
		return filepath.Clean(trimmed)
	}
}
