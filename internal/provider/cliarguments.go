package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JaredTate/coeus/internal/contract"
)

// The two files a run's system prompt is written to, one per program. Each one
// is made in that run's own scratch folder and goes when the folder does.
const (
	// instructionsFileName is where the codex program's system prompt is
	// written, because that program has no flag for one and its configuration
	// takes a file.
	instructionsFileName = "instructions.txt"
	// systemPromptFileName is where the claude program's system prompt is
	// written. It goes in a file rather than on the command line because every
	// process on this machine can read another process's command line, and
	// because the kernel refuses a single argument longer than 128 kilobytes,
	// which a working context passes easily.
	systemPromptFileName = "system-prompt.txt"
)

// argumentsFor builds the command line for one run. Nothing of the conversation
// is on it: the messages go on standard input and the system prompt goes in a
// file inside the run's own scratch folder, so that no process listing and no
// shell history ever holds a word of it.
func (model *commandLineModel) argumentsFor(folder, systemText string) ([]string, error) {
	if model.alias.Program == contract.CodexProgram {
		return model.codexArguments(folder, systemText)
	}
	return model.claudeArguments(folder, systemText)
}

// claudeArguments is the command line for the claude program. The output format
// is the streaming one for every call, because it ends with the same result line
// as the plain one and it is the only way to pass the answer on as it is written.
func (model *commandLineModel) claudeArguments(folder, systemText string) ([]string, error) {
	path, err := model.writeSystemPrompt(folder, systemPromptFileName, systemText)
	if err != nil {
		return nil, err
	}
	return []string{
		"-p",
		"--model", model.alias.ModelName,
		"--tools", "",
		"--no-session-persistence",
		"--safe-mode",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--system-prompt-file", path,
	}, nil
}

// codexArguments is the command line for the codex program. Its system prompt
// goes into a file in the scratch folder as well, because the program has no
// flag for one and its configuration has a setting that reads one.
func (model *commandLineModel) codexArguments(folder, systemText string) ([]string, error) {
	path, err := model.writeSystemPrompt(folder, instructionsFileName, systemText)
	if err != nil {
		return nil, err
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

// writeSystemPrompt puts one run's system prompt in that run's scratch folder,
// readable by nobody but the user the agent runs as, and returns the path to
// hand the program. The folder is removed when the call ends, and this file goes
// with it.
func (model *commandLineModel) writeSystemPrompt(folder, name, systemText string) (string, error) {
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(systemText), contract.SecretFileMode); err != nil {
		return "", fmt.Errorf("the system prompt for %s could not be written to %s: %w", model.alias.Program, path, err)
	}
	return path, nil
}
