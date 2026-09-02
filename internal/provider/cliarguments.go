package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JaredTate/coeus/internal/contract"
)

// instructionsFileName is where the codex program's system prompt is written,
// because that program has no flag for one and its configuration takes a file.
const instructionsFileName = "instructions.txt"

// argumentsFor builds the command line for one run. The prompt itself is never
// on it: it goes on standard input, so that no shell and no process listing ever
// sees the conversation.
func (model *commandLineModel) argumentsFor(folder, systemText string) ([]string, error) {
	if model.alias.Program == contract.CodexProgram {
		return model.codexArguments(folder, systemText)
	}
	return model.claudeArguments(systemText), nil
}

// claudeArguments is the command line for the claude program. The output format
// is the streaming one for every call, because it ends with the same result line
// as the plain one and it is the only way to pass the answer on as it is written.
func (model *commandLineModel) claudeArguments(systemText string) []string {
	return []string{
		"-p",
		"--model", model.alias.ModelName,
		"--tools", "",
		"--no-session-persistence",
		"--safe-mode",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--system-prompt", systemText,
	}
}

// codexArguments is the command line for the codex program. Its system prompt
// goes into a file in the scratch folder, because the program has no flag for
// one and its configuration has a setting that reads one.
func (model *commandLineModel) codexArguments(folder, systemText string) ([]string, error) {
	path := filepath.Join(folder, instructionsFileName)
	if err := os.WriteFile(path, []byte(systemText), contract.DataFileMode); err != nil {
		return nil, fmt.Errorf("the system prompt for %s could not be written to %s: %w", model.alias.Program, path, err)
	}
	return []string{
		"exec",
		"--json",
		"-m", model.alias.ModelName,
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--ephemeral",
		"--ignore-user-config",
		"-c", "model_instructions_file=" + path,
	}, nil
}
